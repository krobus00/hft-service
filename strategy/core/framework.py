import asyncio
import copy
import json
import re
from abc import ABC, abstractmethod
from typing import Any, Dict, List, Optional

import asyncpg
import orjson
from nats.aio.client import Client as NATS

from .common import fmt_num, gen_id, now_ms, parse_iso_to_ms, pct_to_frac
from .models import Candle, RuntimeConfig, Signal, StrategyConfig


class StrategyBase(ABC):
    __slots__ = ("config", "last_close_time_ms", "last_close_time_ms_by_symbol")

    def __init__(self, config: StrategyConfig):
        self.config = config
        self.last_close_time_ms = 0
        self.last_close_time_ms_by_symbol: Dict[str, int] = {}

    def allow_new_candle(self, candle: Candle) -> bool:
        symbol = str(getattr(candle, "symbol", "") or "").strip().upper()
        if symbol:
            prev = self.last_close_time_ms_by_symbol.get(symbol, 0)
            if candle.close_time_ms <= prev:
                return False
            self.last_close_time_ms_by_symbol[symbol] = candle.close_time_ms
            return True

        if candle.close_time_ms <= self.last_close_time_ms:
            return False
        self.last_close_time_ms = candle.close_time_ms
        return True

    def buy(self, price: float, reason: str, metadata: Optional[Dict[str, Any]] = None) -> Signal:
        return Signal(side="BUY", price=price, reason=reason, metadata=metadata or {})

    def sell(self, price: float, reason: str, metadata: Optional[Dict[str, Any]] = None) -> Signal:
        return Signal(side="SELL", price=price, reason=reason, metadata=metadata or {})

    @staticmethod
    def _safe_deepcopy(value: Any) -> Any:
        try:
            return copy.deepcopy(value)
        except Exception:
            return value

    @classmethod
    def _iter_slots(cls):
        seen = set()
        for klass in cls.mro():
            slots = getattr(klass, "__slots__", ())
            if isinstance(slots, str):
                slots = (slots,)
            for slot in slots:
                if slot in seen:
                    continue
                seen.add(slot)
                yield slot

    def snapshot_state(self):
        snapshot: Dict[str, Any] = {}
        for slot in self._iter_slots():
            if slot == "config":
                continue
            if not hasattr(self, slot):
                continue
            snapshot[slot] = self._safe_deepcopy(getattr(self, slot))
        return snapshot

    def restore_state(self, snapshot) -> None:
        if snapshot is None:
            return
        if not isinstance(snapshot, dict):
            return
        for slot in self._iter_slots():
            if slot == "config":
                continue
            if slot not in snapshot:
                continue
            setattr(self, slot, self._safe_deepcopy(snapshot[slot]))
        return None

    def on_price_update(self, candle: Candle) -> Optional[Signal]:
        return None

    @abstractmethod
    def on_closed_candle(self, candle: Candle, is_warmup: bool = False) -> Optional[Signal]:
        raise NotImplementedError


class StrategyRunner:
    __slots__ = ("strategy", "runtime")

    def __init__(self, strategy: StrategyBase, runtime: RuntimeConfig):
        self.strategy = strategy
        self.runtime = runtime

    @staticmethod
    def _state_key(strategy: str, exchange: str, symbol: str, market_type: str, interval: str) -> str:
        return (
            f"{str(strategy or '').strip()}|"
            f"{str(exchange or '').strip().upper()}|"
            f"{str(symbol or '').strip().upper()}|"
            f"{str(market_type or '').strip().lower()}|"
            f"{str(interval or '').strip()}"
        )

    @staticmethod
    def _pair_key(strategy: str, exchange: str, symbol: str, market_type: str, interval: str) -> str:
        return StrategyRunner._state_key(strategy, exchange, symbol, market_type, interval)

    @staticmethod
    def infer_trade_condition(reason: str, metadata: Optional[Dict[str, Any]] = None) -> str:
        if metadata:
            raw = str(metadata.get("trade_condition", "")).strip().upper()
            if raw:
                return raw

        normalized = str(reason or "").strip().upper()
        if not normalized:
            return "UNKNOWN"

        if "TRAIL" in normalized:
            return "TRAILING_STOP"
        if "STOP_LOSS" in normalized or normalized.startswith("SL") or "_SL_" in normalized or normalized.endswith("_SL"):
            return "STOP_LOSS"
        if "TAKE_PROFIT" in normalized or normalized.startswith("TP") or "_TP_" in normalized or normalized.endswith("_TP"):
            return "TAKE_PROFIT"
        if normalized.startswith("ENTER") or normalized.startswith("OPEN"):
            return "ENTRY"
        if normalized.startswith("EXIT") or normalized.startswith("CLOSE") or normalized.startswith("TIME"):
            return "EXIT"

        return "SIGNAL"

    @staticmethod
    def infer_order_reason(reason: str, metadata: Optional[Dict[str, Any]] = None) -> str:
        if metadata:
            raw = str(metadata.get("order_reason", "")).strip()
            if raw:
                return raw

        raw_reason = str(reason or "").strip()
        if raw_reason:
            return raw_reason

        return "UNKNOWN"

    @staticmethod
    def infer_exit_type(reason: str, metadata: Optional[Dict[str, Any]] = None) -> str:
        if metadata:
            raw = str(metadata.get("exit_type", "")).strip().upper()
            if raw in {"TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"}:
                return raw

            tc = str(metadata.get("trade_condition", "")).strip().upper()
            if tc in {"TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"}:
                return tc

        normalized = str(reason or "").strip().upper()
        if "TRAIL" in normalized:
            return "TRAILING_STOP"
        if "STOP_LOSS" in normalized or normalized.startswith("SL") or "_SL_" in normalized or normalized.endswith("_SL"):
            return "STOP_LOSS"
        if "TAKE_PROFIT" in normalized or normalized.startswith("TP") or "_TP_" in normalized or normalized.endswith("_TP"):
            return "TAKE_PROFIT"

        return ""

    @staticmethod
    def _metadata_float(metadata: Optional[Dict[str, Any]], key: str) -> Optional[float]:
        if not metadata:
            return None
        raw = metadata.get(key)
        if raw is None:
            return None
        if isinstance(raw, str) and not raw.strip():
            return None
        try:
            return float(raw)
        except Exception:
            return None

    @staticmethod
    def _format_optional_num(value: Optional[float]) -> str:
        if value is None:
            return ""
        return fmt_num(float(value))

    def infer_entry_exit_and_pnl(
        self,
        side: str,
        signal_price: float,
        trade_condition: str,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> tuple[str, str, str]:
        metadata = metadata or {}

        entry_price = self._metadata_float(metadata, "entry_price")
        if entry_price is None:
            entry_price = self._metadata_float(metadata, "entry")

        exit_price = self._metadata_float(metadata, "exit_price")
        if exit_price is None and trade_condition != "ENTRY":
            exit_price = float(signal_price)

        if trade_condition == "ENTRY" and entry_price is None:
            entry_price = float(signal_price)

        pnl_percentage = self._metadata_float(metadata, "pnl_percentage")
        if pnl_percentage is None:
            pnl_percentage = self._metadata_float(metadata, "pnl_pct")

        if (
            pnl_percentage is None
            and entry_price is not None
            and exit_price is not None
            and entry_price > 0
            and trade_condition in {"EXIT", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"}
        ):
            normalized_side = str(side or "").strip().upper()
            if normalized_side == "SELL":
                pnl_percentage = ((exit_price - entry_price) / entry_price) * 100.0
            elif normalized_side == "BUY":
                pnl_percentage = ((entry_price - exit_price) / entry_price) * 100.0

        return (
            self._format_optional_num(entry_price),
            self._format_optional_num(exit_price),
            self._format_optional_num(pnl_percentage),
        )

    @staticmethod
    def _is_exit_trade_condition(trade_condition: str) -> bool:
        normalized = str(trade_condition or "").strip().upper()
        return normalized in {"EXIT", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"}

    def _safe_token(self, value: str, fallback: str = "NA") -> str:
        token = re.sub(r"[^A-Za-z0-9]+", "_", str(value or "").strip())
        token = token.strip("_")
        return (token or fallback).upper()

    def _resolve_queue_name(self, exchange: str, market_type: str, scope: str) -> str:
        queue_base = "KLINE_STRATEGY"
        strategy_key = str(self.runtime.strategy_id or self.strategy.config.name or "STRATEGY").strip()
        return (
            f"{self._safe_token(queue_base)}_"
            f"{self._safe_token(exchange)}_"
            f"{self._safe_token(market_type)}_"
            f"{self._safe_token(strategy_key)}_"
            f"{self._safe_token(scope)}"
        )

    def _resolve_subject_for_exchange(self, exchange: str) -> str:
        normalized_exchange = str(exchange or "").strip().upper()
        return f"KLINE.{normalized_exchange}.>"

    @staticmethod
    def _order_config_key(exchange: str, symbol: str, market_type: str, interval: str) -> str:
        return (
            f"{str(exchange or '').strip().upper()}|"
            f"{str(symbol or '').strip().upper()}|"
            f"{str(market_type or '').strip().lower()}|"
            f"{str(interval or '').strip()}"
        )

    def _strategy_lookup_keys(self) -> List[str]:
        keys: List[str] = []
        for candidate in (self.runtime.strategy_id, self.strategy.config.name, self.runtime.source):
            value = str(candidate or "").strip()
            if not value:
                continue
            if value in keys:
                continue
            keys.append(value)
        return keys

    async def load_strategy_targets_from_order_configs(self) -> Dict[str, Dict[str, Any]]:
        strategy_keys = self._strategy_lookup_keys()
        if not strategy_keys:
            return {}

        conn = await asyncpg.connect(self.runtime.db_dsn)
        try:
            rows = await conn.fetch(
                """
                SELECT
                    strategy,
                    exchange,
                    symbol,
                    market_type,
                    interval,
                    need_notification,
                    is_paper_trading,
                    order_type,
                    order_qty,
                    limit_slippage_pct
                FROM strategy_order_configs
                WHERE strategy = ANY($1::text[])
                """,
                strategy_keys,
            )
        finally:
            await conn.close()

        targets: Dict[str, Dict[str, Any]] = {}

        for row in rows:
            row_strategy = str(row["strategy"] or "").strip()
            exchange = str(row["exchange"] or "").strip().upper()
            symbol = str(row["symbol"] or "").strip().upper()
            market_type = str(row["market_type"] or "spot").strip().lower() or "spot"
            interval = str(row["interval"] or "").strip() or str(self.strategy.config.interval or "").strip()
            if not symbol:
                continue
            if not exchange:
                continue
            if not market_type:
                continue
            if not interval:
                continue
            if not row_strategy:
                continue

            pair_key = self._pair_key(row_strategy, exchange, symbol, market_type, interval)
            targets[pair_key] = {
                "strategy": row_strategy,
                "exchange": exchange,
                "market_type": market_type,
                "symbol": symbol,
                "interval": interval,
                "order_config": {
                    "strategy": row_strategy,
                    "order_type": str(row["order_type"] or "").strip().upper() or self.runtime.order_type,
                    "order_qty": float(row["order_qty"]),
                    "limit_slippage_pct": float(row["limit_slippage_pct"]),
                    "need_notification": bool(row["need_notification"]),
                    "is_paper_trading": bool(row["is_paper_trading"]),
                },
            }

        return targets

    def build_order_payload(
        self,
        side: str,
        price: float,
        reason: str,
        exchange: str,
        market_type: str,
        symbol: str,
        interval: str,
        metadata: Optional[Dict[str, Any]] = None,
        order_config: Optional[Dict[str, Any]] = None,
    ) -> dict:
        metadata = metadata or {}
        order_config = order_config or {}

        order_type = str(order_config.get("order_type", self.runtime.order_type)).strip().upper() or self.runtime.order_type
        order_qty = float(order_config.get("order_qty", self.runtime.order_qty))
        limit_slippage_pct = float(order_config.get("limit_slippage_pct", self.runtime.limit_slippage_pct))
        need_notification = bool(order_config.get("need_notification", self.runtime.need_notification))
        is_paper_trading = bool(order_config.get("is_paper_trading", self.runtime.is_paper_trading))

        px = float(price)
        if order_type == "LIMIT":
            if side == "BUY":
                px = price * (1.0 + pct_to_frac(limit_slippage_pct))
            else:
                px = price * (1.0 - pct_to_frac(limit_slippage_pct))

        trade_condition = self.infer_trade_condition(reason, metadata)
        order_reason = self.infer_order_reason(reason, metadata)
        exit_type = self.infer_exit_type(reason, metadata)
        entry_price, exit_price, pnl_percentage = self.infer_entry_exit_and_pnl(
            side=side,
            signal_price=px,
            trade_condition=trade_condition,
            metadata=metadata,
        )
        entry_order_id = str(metadata.get("entry_order_id", "")).strip()

        internal_payload: Dict[str, Any] = {}
        raw_internal = metadata.get("internal")
        if isinstance(raw_internal, dict):
            internal_payload.update(raw_internal)
        elif isinstance(raw_internal, str):
            raw_internal_text = raw_internal.strip()
            if raw_internal_text:
                try:
                    loaded = json.loads(raw_internal_text)
                    if isinstance(loaded, dict):
                        internal_payload.update(loaded)
                except Exception:
                    pass

        if entry_order_id:
            internal_payload["entry_order_id"] = entry_order_id

        pair_key = str(metadata.get("pair_key", "")).strip()
        if pair_key:
            internal_payload["pair_key"] = pair_key

        internal_text = json.dumps(internal_payload, separators=(",", ":")) if internal_payload else ""

        return {
            "retry": 0,
            "data": {
                "request_id": gen_id(),
                "user_id": self.runtime.user_id,
                "order_id": gen_id(),
                "exchange": exchange,
                "market_type": market_type,
                "position_side": self.runtime.position_side,
                "symbol": symbol,
                "type": order_type,
                "side": side,
                "price": fmt_num(px),
                "quantity": fmt_num(order_qty),
                "requested_at": now_ms(),
                "expired_at": None,
                "source": self.runtime.source,
                "strategy_id": self.runtime.strategy_id,
                "strategy_name": self.strategy.config.name,
                "interval": interval,
                "internal": internal_text,
                "entry_order_id": entry_order_id,
                "entry_price": entry_price,
                "exit_price": exit_price,
                "pnl_percentage": pnl_percentage,
                "trade_condition": trade_condition,
                "order_reason": order_reason,
                "exit_type": exit_type,
                "need_notification": need_notification,
                "is_paper_trading": is_paper_trading,
            },
        }

    async def warmup_from_postgres(self, exchange: str, market_type: str, symbol: str, interval: str) -> None:
        conn = await asyncpg.connect(self.runtime.db_dsn)
        try:
            rows = await conn.fetch(
                """
                SELECT
                    close_time,
                    close_price,
                    quote_volume,
                    high_price,
                    low_price,
                    taker_quote_volume,
                    trade_count
                FROM market_klines
                                WHERE lower(exchange) = lower($1)
                                    AND lower(market_type) = lower($2)
                                    AND symbol = $3
                                    AND interval = $4
                  AND is_closed IS TRUE
                ORDER BY close_time DESC
                                LIMIT $5
                """,
                                exchange,
                                market_type,
                                symbol,
                interval,
                self.strategy.config.warmup_limit,
            )
        finally:
            await conn.close()

        rows = list(reversed(rows))
        for row in rows:
            candle = Candle(
                close_time_ms=int(row["close_time"].timestamp() * 1000),
                close=float(row["close_price"]),
                quote_volume=float(row["quote_volume"]),
                symbol=symbol,
                interval=interval,
                high=float(row["high_price"]),
                low=float(row["low_price"]),
                taker_quote_volume=float(row["taker_quote_volume"]),
                trade_count=int(row["trade_count"]),
            )
            self.strategy.on_closed_candle(candle, is_warmup=True)

        print(f"Warmup completed for {exchange}:{market_type}:{symbol}:{interval} with {len(rows)} candles", flush=True)

    async def run(self) -> None:
        if self.runtime.order_qty <= 0:
            raise ValueError("order_qty must be > 0")

        print(
            f"[{self.strategy.config.name}] start_config strategy_id={self.runtime.strategy_id} source={self.runtime.source} user_id={self.runtime.user_id} interval={self.strategy.config.interval} warmup_limit={self.strategy.config.warmup_limit} order_subject={self.runtime.order_subject} order_type={self.runtime.order_type} order_qty={self.runtime.order_qty} limit_slippage_pct={self.runtime.limit_slippage_pct} position_side={self.runtime.position_side} need_notification={self.runtime.need_notification} is_paper_trading={self.runtime.is_paper_trading} intrabar_risk_exit={self.runtime.enable_intrabar_risk_exit}",
            flush=True,
        )

        initial_state = self.strategy.snapshot_state()
        state_store: Dict[str, Any] = {}
        entry_price_store: Dict[str, float] = {}
        entry_order_id_store: Dict[str, str] = {}

        nc = NATS()

        async def on_disconnected():
            print("NATS disconnected, reconnecting...", flush=True)

        async def on_reconnected():
            print(f"NATS reconnected, server={nc.connected_url}", flush=True)

        async def on_error(exc):
            print(f"NATS async error: {exc}", flush=True)

        async def on_closed():
            print("NATS connection closed", flush=True)

        await nc.connect(
            self.runtime.nats_url,
            allow_reconnect=self.runtime.nats_allow_reconnect,
            max_reconnect_attempts=self.runtime.nats_max_reconnect_attempts,
            reconnect_time_wait=self.runtime.nats_reconnect_time_wait_sec,
            connect_timeout=self.runtime.nats_connect_timeout_sec,
            ping_interval=self.runtime.nats_ping_interval_sec,
            max_outstanding_pings=self.runtime.nats_max_outstanding_pings,
            disconnected_cb=on_disconnected,
            reconnected_cb=on_reconnected,
            error_cb=on_error,
            closed_cb=on_closed,
        )
        js = nc.jetstream()
        state_lock = asyncio.Lock()
        active_pairs: Dict[str, Dict[str, Any]] = {}
        refresh_interval_sec = 10
        no_pairs_logged = False
        last_active_summary = ""

        def build_handler(pair_key: str):
            async def handler(msg):
                try:
                    async with state_lock:
                        pair_ctx = active_pairs.get(pair_key)
                        if pair_ctx is None:
                            await msg.ack()
                            return

                        target = pair_ctx["target"]
                        expected_strategy = str(target["strategy"]).strip()
                        expected_exchange = str(target["exchange"]).strip().upper()
                        expected_symbol = str(target["symbol"]).strip().upper()
                        expected_interval = str(target["interval"]).strip()
                        expected_market_type = str(target.get("market_type", "spot") or "spot").strip().lower()

                        payload = orjson.loads(msg.data)
                        data = payload.get("data", {})

                        incoming_symbol = str(data.get("Symbol", "")).strip().upper()
                        incoming_interval = str(data.get("Interval", "")).strip()
                        incoming_market_type = str(data.get("MarketType", "")).strip().lower()
                        is_closed = bool(data.get("IsClosed"))

                        if incoming_symbol != expected_symbol:
                            await msg.ack()
                            return

                        if incoming_interval != expected_interval:
                            await msg.ack()
                            return

                        effective_market_type = incoming_market_type or expected_market_type
                        if effective_market_type != expected_market_type:
                            await msg.ack()
                            return

                        candle = Candle(
                            close_time_ms=parse_iso_to_ms(data["CloseTime"]),
                            close=float(data["ClosePrice"]),
                            quote_volume=float(data.get("QuoteVolume", "0") or "0"),
                            symbol=incoming_symbol,
                            interval=incoming_interval,
                            high=float(data.get("HighPrice", data.get("ClosePrice", "0")) or "0"),
                            low=float(data.get("LowPrice", data.get("ClosePrice", "0")) or "0"),
                            taker_quote_volume=float(data.get("TakerQuoteVolume", "0") or "0"),
                            trade_count=int(data.get("TradeCount", 0) or 0),
                        )

                        state_key = self._state_key(
                            expected_strategy,
                            expected_exchange,
                            candle.symbol,
                            expected_market_type,
                            candle.interval,
                        )
                        previous_snapshot = state_store.get(state_key)
                        self.strategy.restore_state(previous_snapshot if previous_snapshot is not None else initial_state)

                        snapshot = self.strategy.snapshot_state()
                        if is_closed:
                            signal = self.strategy.on_closed_candle(candle, is_warmup=False)
                        elif self.runtime.enable_intrabar_risk_exit:
                            signal = self.strategy.on_price_update(candle)
                        else:
                            signal = None

                        if signal is not None:
                            try:
                                trade_condition = self.infer_trade_condition(signal.reason, signal.metadata)
                                signal_metadata = dict(signal.metadata or {})

                                if trade_condition == "ENTRY":
                                    if self._metadata_float(signal_metadata, "entry_price") is None:
                                        signal_metadata["entry_price"] = float(signal.price)

                                    entry_order_id = str(signal_metadata.get("entry_order_id", "")).strip()
                                    if not entry_order_id:
                                        entry_order_id = gen_id()
                                        signal_metadata["entry_order_id"] = entry_order_id
                                    entry_order_id_store[state_key] = entry_order_id
                                elif self._is_exit_trade_condition(trade_condition):
                                    if self._metadata_float(signal_metadata, "entry_price") is None:
                                        remembered_entry = entry_price_store.get(state_key)
                                        if remembered_entry is not None:
                                            signal_metadata["entry_price"] = float(remembered_entry)

                                    exit_entry_order_id = str(signal_metadata.get("entry_order_id", "")).strip()
                                    if not exit_entry_order_id:
                                        remembered_entry_order_id = entry_order_id_store.get(state_key)
                                        if remembered_entry_order_id:
                                            exit_entry_order_id = remembered_entry_order_id
                                            signal_metadata["entry_order_id"] = remembered_entry_order_id

                                    if self._metadata_float(signal_metadata, "exit_price") is None:
                                        signal_metadata["exit_price"] = float(signal.price)

                                signal_metadata["pair_key"] = pair_key

                                out = self.build_order_payload(
                                    signal.side,
                                    signal.price,
                                    signal.reason,
                                    exchange=expected_exchange,
                                    market_type=effective_market_type,
                                    symbol=candle.symbol,
                                    interval=candle.interval,
                                    metadata=signal_metadata,
                                    order_config=target.get("order_config"),
                                )
                                out_bytes = orjson.dumps(out)
                                print(
                                    f"[{self.strategy.config.name}] publish subject={self.runtime.order_subject} payload={out_bytes.decode('utf-8')}",
                                    flush=True,
                                )
                                await js.publish(self.runtime.order_subject, out_bytes)

                                if trade_condition == "ENTRY":
                                    persisted_entry = self._metadata_float(signal_metadata, "entry_price")
                                    if persisted_entry is not None:
                                        entry_price_store[state_key] = float(persisted_entry)
                                elif self._is_exit_trade_condition(trade_condition):
                                    entry_price_store.pop(state_key, None)
                                    entry_order_id_store.pop(state_key, None)
                            except Exception:
                                self.strategy.restore_state(snapshot)
                                raise
                            metadata = " ".join(f"{k}={v}" for k, v in signal.metadata.items())
                            print(
                                f"[{self.strategy.config.name}] {signal.reason} trade_condition={trade_condition} side={signal.side} symbol={candle.symbol} close={signal.price:.6f} {metadata}".strip(),
                                flush=True,
                            )

                        state_store[state_key] = self.strategy.snapshot_state()

                    await msg.ack()
                except Exception as exc:
                    print(f"handler error: {exc}", flush=True)

            return handler

        async def reconcile_pairs() -> None:
            nonlocal no_pairs_logged, last_active_summary

            try:
                desired_pairs = await self.load_strategy_targets_from_order_configs()
            except Exception as exc:
                print(f"Failed loading strategy_order_configs: {exc}", flush=True)
                return

            active_keys = set(active_pairs.keys())
            desired_keys = set(desired_pairs.keys())

            changed_keys = {
                key for key in (active_keys & desired_keys) if active_pairs[key]["target"] != desired_pairs[key]
            }
            remove_keys = sorted((active_keys - desired_keys) | changed_keys)
            add_keys = sorted((desired_keys - active_keys) | changed_keys)
            has_changes = bool(remove_keys or add_keys)

            for key in remove_keys:
                pair_ctx = active_pairs.pop(key, None)
                if pair_ctx is None:
                    continue

                target = pair_ctx["target"]
                sub = pair_ctx.get("subscription")
                if sub is not None:
                    try:
                        await sub.unsubscribe()
                    except Exception as exc:
                        print(f"unsubscribe error for pair={key}: {exc}", flush=True)

                state_key = self._state_key(
                    target["strategy"],
                    target["exchange"],
                    target["symbol"],
                    target["market_type"],
                    target["interval"],
                )
                state_store.pop(state_key, None)
                entry_price_store.pop(state_key, None)
                entry_order_id_store.pop(state_key, None)
                print(f"Removed pair={key}", flush=True)

            for key in add_keys:
                target = desired_pairs[key]
                state_key = self._state_key(
                    target["strategy"],
                    target["exchange"],
                    target["symbol"],
                    target["market_type"],
                    target["interval"],
                )

                try:
                    async with state_lock:
                        previous = state_store.get(state_key)
                        self.strategy.restore_state(previous if previous is not None else initial_state)
                        await self.warmup_from_postgres(
                            target["exchange"],
                            target["market_type"],
                            target["symbol"],
                            target["interval"],
                        )
                        state_store[state_key] = self.strategy.snapshot_state()
                except Exception as exc:
                    print(f"Warmup skipped for pair={key} due to DB error: {exc}", flush=True)

                subject = self._resolve_subject_for_exchange(target["exchange"])
                queue_scope = f"{target['strategy']}_{target['symbol']}_{target['market_type']}_{target['interval']}"
                queue_name = self._resolve_queue_name(target["exchange"], target["market_type"], queue_scope)
                sub = await js.subscribe(
                    subject,
                    manual_ack=True,
                    queue=queue_name,
                    cb=build_handler(key),
                )
                active_pairs[key] = {
                    "target": target,
                    "subscription": sub,
                }
                print(
                    f"Subscribed pair={key} subject={subject} queue={queue_name}",
                    flush=True,
                )

            if active_pairs:
                no_pairs_logged = False
                active_summary = ",".join(sorted(active_pairs.keys()))
                if has_changes or active_summary != last_active_summary:
                    print(f"{self.strategy.config.name} strategy running for pairs={active_summary}", flush=True)
                    last_active_summary = active_summary
            elif not no_pairs_logged:
                print(
                    f"{self.strategy.config.name} no strategy_order_configs rows, waiting and refreshing every {refresh_interval_sec}s",
                    flush=True,
                )
                no_pairs_logged = True
                last_active_summary = ""

        while True:
            if nc.is_closed:
                raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
            await reconcile_pairs()
            await asyncio.sleep(refresh_interval_sec)
