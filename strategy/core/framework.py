import asyncio
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

    def snapshot_state(self):
        return None

    def restore_state(self, snapshot) -> None:
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

    def _safe_token(self, value: str, fallback: str = "NA") -> str:
        token = re.sub(r"[^A-Za-z0-9]+", "_", str(value or "").strip())
        token = token.strip("_")
        return (token or fallback).upper()

    def _resolve_queue_name(self, exchange: str, market_type: str, symbol: str) -> str:
        queue_base = "KLINE_STRATEGY"
        strategy_key = str(self.runtime.strategy_id or self.strategy.config.name or "STRATEGY").strip()
        return (
            f"{self._safe_token(queue_base)}_"
            f"{self._safe_token(exchange)}_"
            f"{self._safe_token(market_type)}_"
            f"{self._safe_token(strategy_key)}_"
            f"{self._safe_token(symbol)}"
        )

    def _resolve_subject_for_symbol(self, exchange: str, symbol: str, interval: str) -> str:
        normalized_exchange = str(exchange or "").strip().upper()
        normalized_symbol = str(symbol or "").strip().upper()
        normalized_interval = str(interval or "").strip()
        return f"KLINE.{normalized_exchange}.{normalized_symbol}.{normalized_interval}".upper()

    async def load_symbols_from_subscriptions(self) -> List[Dict[str, str]]:
        conn = await asyncpg.connect(self.runtime.db_dsn)
        try:
            rows = await conn.fetch(
                """
                SELECT DISTINCT exchange, market_type, symbol, interval
                FROM kline_subscriptions
                WHERE lower(interval) = lower($1)
                ORDER BY exchange ASC, market_type ASC, symbol ASC
                """,
                self.strategy.config.interval,
            )
        finally:
            await conn.close()

        targets: List[Dict[str, str]] = []
        for row in rows:
            symbol = str(row["symbol"]).strip().upper()
            interval = str(row["interval"]).strip() or str(self.strategy.config.interval or "").strip()
            exchange_raw = str(row["exchange"]).strip()
            exchange = exchange_raw.upper()
            market_type = str(row["market_type"]).strip().lower()
            if not symbol:
                continue
            if not exchange:
                continue
            if not market_type:
                continue
            targets.append({"exchange": exchange, "market_type": market_type, "symbol": symbol, "interval": interval})

        if targets:
            return targets
        return []

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
    ) -> dict:
        metadata = metadata or {}
        px = float(price)
        if self.runtime.order_type == "LIMIT":
            if side == "BUY":
                px = price * (1.0 + pct_to_frac(self.runtime.limit_slippage_pct))
            else:
                px = price * (1.0 - pct_to_frac(self.runtime.limit_slippage_pct))

        trade_condition = self.infer_trade_condition(reason, metadata)
        order_reason = self.infer_order_reason(reason, metadata)
        exit_type = self.infer_exit_type(reason, metadata)

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
                "type": self.runtime.order_type,
                "side": side,
                "price": fmt_num(px),
                "quantity": fmt_num(float(self.runtime.order_qty)),
                "requested_at": now_ms(),
                "expired_at": None,
                "source": self.runtime.source,
                "strategy_id": self.runtime.strategy_id,
                "strategy_name": self.strategy.config.name,
                "interval": interval,
                "internal": symbol,
                "trade_condition": trade_condition,
                "order_reason": order_reason,
                "exit_type": exit_type,
                "need_notification": self.runtime.need_notification,
                "is_paper_trading": self.runtime.is_paper_trading,
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

        try:
            targets = await self.load_symbols_from_subscriptions()
        except Exception as exc:
            raise RuntimeError(f"failed loading symbols from kline_subscriptions: {exc}") from exc

        if not targets:
            raise RuntimeError("No matching rows found in kline_subscriptions for strategy interval")

        try:
            for target in targets:
                await self.warmup_from_postgres(
                    target["exchange"],
                    target["market_type"],
                    target["symbol"],
                    target["interval"],
                )
        except Exception as exc:
            print(f"Warmup skipped due to DB error: {exc}", flush=True)

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

        def build_handler(expected_exchange: str, expected_market_type: str, expected_symbol: str, expected_interval: str):
            normalized_expected = str(expected_symbol).strip().upper()
            normalized_interval = str(expected_interval).strip()
            normalized_exchange = str(expected_exchange).strip().upper()
            normalized_market_type = str(expected_market_type).strip().lower()

            async def handler(msg):
                try:
                    payload = orjson.loads(msg.data)
                    data = payload.get("data", {})

                    incoming_symbol = str(data.get("Symbol", "")).strip().upper() or normalized_expected
                    incoming_interval = str(data.get("Interval", "")).strip() or normalized_interval
                    is_closed = bool(data.get("IsClosed"))

                    print(
                        f"[{self.strategy.config.name}] event_received subject={getattr(msg, 'subject', '')} expected_exchange={normalized_exchange} expected_market_type={normalized_market_type} expected_symbol={normalized_expected} expected_interval={normalized_interval} incoming_symbol={incoming_symbol} incoming_interval={incoming_interval} is_closed={is_closed}",
                        flush=True,
                    )

                    if incoming_symbol != normalized_expected:
                        print(
                            f"[{self.strategy.config.name}] event_skipped reason=symbol_mismatch expected={normalized_expected} got={incoming_symbol}",
                            flush=True,
                        )
                        await msg.ack()
                        return

                    if incoming_interval != normalized_interval:
                        print(
                            f"[{self.strategy.config.name}] event_skipped reason=interval_mismatch expected={normalized_interval} got={incoming_interval}",
                            flush=True,
                        )
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

                    snapshot = self.strategy.snapshot_state()
                    if is_closed:
                        signal = self.strategy.on_closed_candle(candle, is_warmup=False)
                    elif self.runtime.enable_intrabar_risk_exit:
                        signal = self.strategy.on_price_update(candle)
                    else:
                        signal = None

                    if signal is not None:
                        try:
                            out = self.build_order_payload(
                                signal.side,
                                signal.price,
                                signal.reason,
                                exchange=normalized_exchange,
                                market_type=normalized_market_type,
                                symbol=candle.symbol,
                                interval=candle.interval,
                                metadata=signal.metadata,
                            )
                            await js.publish(self.runtime.order_subject, orjson.dumps(out))
                        except Exception:
                            self.strategy.restore_state(snapshot)
                            raise
                        metadata = " ".join(f"{k}={v}" for k, v in signal.metadata.items())
                        trade_condition = self.infer_trade_condition(signal.reason, signal.metadata)
                        print(
                            f"[{self.strategy.config.name}] {signal.reason} trade_condition={trade_condition} side={signal.side} symbol={candle.symbol} close={signal.price:.6f} {metadata}".strip(),
                            flush=True,
                        )

                    await msg.ack()
                except Exception as exc:
                    print(f"handler error: {exc}", flush=True)

            return handler

        stream_bindings: List[str] = []
        for target in targets:
            exchange = target["exchange"]
            market_type = target["market_type"]
            symbol = target["symbol"]
            interval = target["interval"]
            subject = self._resolve_subject_for_symbol(exchange, symbol, interval)
            queue_name = self._resolve_queue_name(exchange, market_type, symbol)
            stream_bindings.append(f"{subject}|{queue_name}")

        print(
            f"[{self.strategy.config.name}] kline_stream_bindings_count={len(stream_bindings)} bindings={';'.join(stream_bindings)}",
            flush=True,
        )

        for target in targets:
            exchange = target["exchange"]
            market_type = target["market_type"]
            symbol = target["symbol"]
            interval = target["interval"]
            subject = self._resolve_subject_for_symbol(exchange, symbol, interval)
            queue_name = self._resolve_queue_name(exchange, market_type, symbol)
            await js.subscribe(
                subject,
                manual_ack=True,
                queue=queue_name,
                cb=build_handler(exchange, market_type, symbol, interval),
            )
            print(f"Subscribed subject={subject} queue={queue_name}", flush=True)

        symbols_for_log = ",".join(f"{t['exchange']}:{t['market_type']}:{t['symbol']}:{t['interval']}" for t in targets)
        print(f"{self.strategy.config.name} strategy running for symbols={symbols_for_log}", flush=True)
        while True:
            if nc.is_closed:
                raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
            await asyncio.sleep(1)
