import asyncio
import contextlib
import copy
import json
import logging
import pickle
import re
import zlib
from abc import ABC, abstractmethod
from collections import deque
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable, Dict, List, Optional, Sequence

import asyncpg
import orjson
from nats.aio.client import Client as NATS
from nats.js.api import DeliverPolicy

from .common import fmt_num, gen_id, now_ms, parse_iso_to_ms, pct_to_frac
from .models import Candle, RuntimeConfig, Signal, StrategyConfig

try:
    import redis.asyncio as redis_async
except Exception:  # pragma: no cover - redis is optional at runtime.
    redis_async = None


LOGGER = logging.getLogger("strategy.framework")
KLINE_STREAM_NAME = "KLINE"
DEFAULT_ORDER_SUBJECT = "order_engine.place_order"
if not logging.getLogger().handlers:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )


@dataclass
class StrategyMetrics:
    started_at_ms: int = field(default_factory=now_ms)
    last_reconcile_ms: int = 0
    messages_seen: int = 0
    messages_enqueued: int = 0
    messages_acked: int = 0
    messages_failed: int = 0
    signals_emitted: int = 0
    orders_published: int = 0
    config_resync_count: int = 0
    nats_disconnects: int = 0
    nats_reconnects: int = 0
    last_error: str = ""


@dataclass
class StoredPairState:
    strategy_state: Optional[Dict[str, Any]] = None
    entry_price: Optional[float] = None
    entry_order_id: str = ""


class RedisStateStore:
    def __init__(self, client: Any, prefix: str, ttl_sec: int):
        self.client = client
        self.prefix = prefix.strip(":") or "hft:strategy"
        self.ttl_sec = max(0, int(ttl_sec or 0))

    @classmethod
    async def connect(cls, runtime: RuntimeConfig) -> Optional["RedisStateStore"]:
        redis_url = str(runtime.redis_url or "").strip()
        if not redis_url:
            LOGGER.info("redis state store disabled")
            return None
        if redis_async is None:
            LOGGER.warning("redis package is not installed; state persistence disabled")
            return None

        client = redis_async.from_url(redis_url, encoding=None, decode_responses=False)
        try:
            await client.ping()
        except Exception as exc:
            LOGGER.warning("redis state store unavailable: %s", exc)
            with contextlib.suppress(Exception):
                await client.aclose()
            return None

        LOGGER.info("redis state store connected prefix=%s ttl_sec=%s", runtime.redis_key_prefix, runtime.redis_state_ttl_sec)
        return cls(client, runtime.redis_key_prefix, runtime.redis_state_ttl_sec)

    def key(self, state_key: str) -> str:
        return f"{self.prefix}:state:{state_key}"

    async def load(self, state_key: str) -> StoredPairState:
        raw = await self.client.get(self.key(state_key))
        if not raw:
            return StoredPairState()

        payload = pickle.loads(raw)
        if not isinstance(payload, dict):
            return StoredPairState()

        return StoredPairState(
            strategy_state=payload.get("strategy_state") if isinstance(payload.get("strategy_state"), dict) else None,
            entry_price=payload.get("entry_price"),
            entry_order_id=str(payload.get("entry_order_id") or ""),
        )

    async def save(self, state_key: str, state: StoredPairState) -> None:
        payload = pickle.dumps(
            {
                "strategy_state": state.strategy_state,
                "entry_price": state.entry_price,
                "entry_order_id": state.entry_order_id,
                "updated_at_ms": now_ms(),
            },
            protocol=pickle.HIGHEST_PROTOCOL,
        )
        if self.ttl_sec > 0:
            await self.client.set(self.key(state_key), payload, ex=self.ttl_sec)
        else:
            await self.client.set(self.key(state_key), payload)

    async def delete(self, state_key: str) -> None:
        await self.client.delete(self.key(state_key))

    async def close(self) -> None:
        with contextlib.suppress(Exception):
            await self.client.aclose()


class StrategyMonitorServer:
    def __init__(self, host: str, port: int, payload_factory: Callable[[], Awaitable[Dict[str, Any]]]):
        self.host = host
        self.port = port
        self.payload_factory = payload_factory
        self.server: Optional[asyncio.AbstractServer] = None

    async def start(self) -> None:
        self.server = await asyncio.start_server(self._handle_client, self.host, self.port)
        LOGGER.info("strategy monitor listening on http://%s:%s", self.host, self.port)

    async def close(self) -> None:
        if self.server is None:
            return
        self.server.close()
        await self.server.wait_closed()

    async def _handle_client(self, reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        try:
            request_line = await reader.readline()
            while True:
                line = await reader.readline()
                if line in {b"\r\n", b"\n", b""}:
                    break

            path = "/"
            parts = request_line.decode("utf-8", errors="ignore").split()
            if len(parts) >= 2:
                path = parts[1]

            if path not in {"/", "/health", "/metrics", "/state"}:
                await self._write_json(writer, 404, {"status": "not_found"})
                return

            payload = await self.payload_factory()
            if path == "/health":
                payload = {"status": payload.get("status", "ok"), "health": payload.get("health", {})}
            elif path == "/metrics":
                payload = {"status": payload.get("status", "ok"), "metrics": payload.get("metrics", {})}

            await self._write_json(writer, 200, payload)
        except Exception as exc:
            LOGGER.warning("monitor request failed: %s", exc)
        finally:
            writer.close()
            with contextlib.suppress(Exception):
                await writer.wait_closed()

    async def _write_json(self, writer: asyncio.StreamWriter, status_code: int, payload: Dict[str, Any]) -> None:
        body = orjson.dumps(payload)
        reason = "OK" if status_code == 200 else "Not Found"
        writer.write(
            (
                f"HTTP/1.1 {status_code} {reason}\r\n"
                "Content-Type: application/json\r\n"
                f"Content-Length: {len(body)}\r\n"
                "Connection: close\r\n\r\n"
            ).encode("ascii")
            + body
        )
        await writer.drain()


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
    def on_closed_candle(self, candle: Candle, is_warmup: bool = False) -> Optional[Signal] | Sequence[Signal]:
        raise NotImplementedError

    @staticmethod
    def normalize_signals(result: Optional[Signal] | Sequence[Signal]) -> List[Signal]:
        if result is None:
            return []
        if isinstance(result, Signal):
            return [result]
        if isinstance(result, (list, tuple)):
            return [signal for signal in result if isinstance(signal, Signal)]
        return []


class StrategyRunner:
    __slots__ = ("strategy", "runtime", "db_pool")

    RISK_CONFIG_KEYS = (
        "cooldown_bars",
        "sl_cooldown_bars",
        "max_consecutive_stop_losses",
        "sl_pause_bars",
        "take_profit_pct",
        "stop_loss_pct",
        "trailing_stop_pct",
        "trailing_stop_trigger_pct",
        "max_hold_bars",
        "max_positions",
        "enable_intrabar_risk_exit",
    )

    def __init__(self, strategy: StrategyBase, runtime: RuntimeConfig):
        self.strategy = strategy
        self.runtime = runtime
        self.db_pool: Optional[asyncpg.Pool] = None

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

    @staticmethod
    def _infer_pnl_direction(
        side: str,
        reason: str,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> str:
        # Prefer explicit trade direction when present.
        if metadata:
            for key in ("position_side", "position", "side", "trade_side"):
                raw = str(metadata.get(key, "")).strip().upper()
                if raw in {"LONG", "BUY"}:
                    return "LONG"
                if raw in {"SHORT", "SELL"}:
                    return "SHORT"

        normalized_reason = str(reason or "").strip().upper()
        if "SHORT" in normalized_reason:
            return "SHORT"
        if "LONG" in normalized_reason:
            return "LONG"

        normalized_side = str(side or "").strip().upper()
        if normalized_side in {"BUY", "LONG"}:
            return "SHORT"
        if normalized_side in {"SELL", "SHORT"}:
            return "LONG"
        return ""

    def infer_entry_exit_and_pnl(
        self,
        side: str,
        reason: str,
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
            pnl_direction = self._infer_pnl_direction(side=side, reason=reason, metadata=metadata)
            if pnl_direction == "LONG":
                pnl_percentage = ((exit_price - entry_price) / entry_price) * 100.0
            elif pnl_direction == "SHORT":
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

    @classmethod
    def _to_jsonable(cls, value: Any) -> Any:
        if value is None or isinstance(value, (bool, int, float, str)):
            return value

        if isinstance(value, dict):
            return {str(k): cls._to_jsonable(v) for k, v in value.items()}

        if isinstance(value, (list, tuple, set, deque)):
            return [cls._to_jsonable(v) for v in value]

        if hasattr(value, "isoformat"):
            try:
                return value.isoformat()
            except Exception:
                pass

        slots = getattr(value, "__slots__", None)
        if slots:
            if isinstance(slots, str):
                slots = (slots,)
            out: Dict[str, Any] = {}
            for slot in slots:
                if hasattr(value, slot):
                    out[str(slot)] = cls._to_jsonable(getattr(value, slot))
            if out:
                return out

        if hasattr(value, "__dict__"):
            try:
                raw = vars(value)
                if raw:
                    return {str(k): cls._to_jsonable(v) for k, v in raw.items()}
            except Exception:
                pass

        return str(value)

    def _safe_token(self, value: str, fallback: str = "NA") -> str:
        token = re.sub(r"[^A-Za-z0-9]+", "_", str(value or "").strip())
        token = token.strip("_")
        return (token or fallback).upper()

    def _resolve_queue_name(self, exchange: str, market_type: str, symbol: str, interval: str) -> str:
        strategy_key = str(self.runtime.strategy_id or self.strategy.config.name or "STRATEGY").strip()
        full_key = "|".join(
            (
                self._safe_token(exchange),
                self._safe_token(market_type),
                self._safe_token(strategy_key),
                self._safe_token(symbol),
                self._safe_token(interval),
            )
        )
        checksum = f"{zlib.crc32(full_key.encode('utf-8')) & 0xFFFFFFFF:08X}"[:4]
        market_token = self._safe_token(market_type)[:1] or "M"
        return (
            f"KS_"
            f"{self._safe_token(exchange)[:3]}_"
            f"{market_token}_"
            f"{self._safe_token(symbol)[:8]}_"
            f"{self._safe_token(interval)[:4]}_"
            f"{checksum}"
        )

    def _resolve_subject_for_exchange(self, exchange: str) -> str:
        normalized_exchange = str(exchange or "").strip().upper()
        return f"KLINE.{normalized_exchange}.>"

    def _resolve_subject_for_pair(self, exchange: str, symbol: str, interval: str) -> str:
        normalized_exchange = str(exchange or "").strip().upper()
        normalized_symbol = str(symbol or "").strip().upper()
        normalized_interval = str(interval or "").strip().upper()
        return f"KLINE.{normalized_exchange}.{normalized_symbol}.{normalized_interval}"

    @staticmethod
    def _order_config_key(exchange: str, symbol: str, market_type: str, interval: str) -> str:
        return (
            f"{str(exchange or '').strip().upper()}|"
            f"{str(symbol or '').strip().upper()}|"
            f"{str(market_type or '').strip().lower()}|"
            f"{str(interval or '').strip()}"
        )

    @staticmethod
    def _kline_field(data: Dict[str, Any], *keys: str, default: Any = "") -> Any:
        for key in keys:
            if key in data:
                return data[key]
        return default

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

    def _apply_strategy_overrides(self, order_config: Optional[Dict[str, Any]], strategy: Optional[StrategyBase] = None) -> None:
        if not isinstance(order_config, dict):
            return

        target_strategy = strategy or self.strategy
        for key in self.RISK_CONFIG_KEYS:
            if key not in order_config:
                continue
            value = order_config.get(key)
            if value is None:
                continue
            if not hasattr(target_strategy, key):
                continue
            setattr(target_strategy, key, value)

    async def load_strategy_targets_from_order_configs(self) -> Dict[str, Dict[str, Any]]:
        strategy_keys = self._strategy_lookup_keys()
        if not strategy_keys:
            return {}

        if self.db_pool is not None:
            async with self.db_pool.acquire() as conn:
                rows = await conn.fetch(
                    """
                    SELECT
                        strategy,
                        exchange,
                        symbol,
                        market_type,
                        interval,
                        user_id,
                        position_side,
                        source,
                        need_notification,
                        is_paper_trading,
                        order_type,
                        order_qty,
                        limit_slippage_pct,
                        cooldown_bars,
                        sl_cooldown_bars,
                        max_consecutive_stop_losses,
                        sl_pause_bars,
                        take_profit_pct,
                        stop_loss_pct,
                        trailing_stop_pct,
                        trailing_stop_trigger_pct,
                        max_hold_bars,
                        max_positions,
                        enable_intrabar_risk_exit
                    FROM strategy_configs
                    WHERE strategy = ANY($1::text[])
                    """,
                    strategy_keys,
                )
        else:
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
                        user_id,
                        position_side,
                        source,
                        need_notification,
                        is_paper_trading,
                        order_type,
                        order_qty,
                        limit_slippage_pct,
                        cooldown_bars,
                        sl_cooldown_bars,
                        max_consecutive_stop_losses,
                        sl_pause_bars,
                        take_profit_pct,
                        stop_loss_pct,
                        trailing_stop_pct,
                        trailing_stop_trigger_pct,
                        max_hold_bars,
                        max_positions,
                        enable_intrabar_risk_exit
                    FROM strategy_configs
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
            user_id = str(row["user_id"] or "").strip()
            if not symbol:
                continue
            if not exchange:
                continue
            if not market_type:
                continue
            if not interval:
                continue
            if not user_id:
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
                    "user_id": user_id,
                    "position_side": str(row["position_side"] or self.runtime.position_side).strip().upper()
                    or self.runtime.position_side,
                    "source": str(row["source"] or self.runtime.source).strip() or self.runtime.source,
                    "order_type": str(row["order_type"] or "").strip().upper() or self.runtime.order_type,
                    "order_qty": float(row["order_qty"]),
                    "limit_slippage_pct": float(row["limit_slippage_pct"]),
                    "need_notification": bool(row["need_notification"]),
                    "is_paper_trading": bool(row["is_paper_trading"]),
                    "cooldown_bars": int(row["cooldown_bars"]) if row["cooldown_bars"] is not None else None,
                    "sl_cooldown_bars": int(row["sl_cooldown_bars"]) if row["sl_cooldown_bars"] is not None else None,
                    "max_consecutive_stop_losses": int(row["max_consecutive_stop_losses"])
                    if row["max_consecutive_stop_losses"] is not None
                    else None,
                    "sl_pause_bars": int(row["sl_pause_bars"]) if row["sl_pause_bars"] is not None else None,
                    "take_profit_pct": float(row["take_profit_pct"]) if row["take_profit_pct"] is not None else None,
                    "stop_loss_pct": float(row["stop_loss_pct"]) if row["stop_loss_pct"] is not None else None,
                    "trailing_stop_pct": float(row["trailing_stop_pct"])
                    if row["trailing_stop_pct"] is not None
                    else None,
                    "trailing_stop_trigger_pct": float(row["trailing_stop_trigger_pct"])
                    if row["trailing_stop_trigger_pct"] is not None
                    else None,
                    "max_hold_bars": int(row["max_hold_bars"]) if row["max_hold_bars"] is not None else None,
                    "max_positions": int(row["max_positions"]) if row["max_positions"] is not None else None,
                    "enable_intrabar_risk_exit": bool(row["enable_intrabar_risk_exit"])
                    if row["enable_intrabar_risk_exit"] is not None
                    else None,
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
        user_id = str(order_config.get("user_id", "")).strip()
        position_side = str(order_config.get("position_side", self.runtime.position_side)).strip().upper()
        source = str(order_config.get("source", self.runtime.source)).strip()
        strategy_id = str(order_config.get("strategy", self.runtime.strategy_id)).strip()

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
            reason=reason,
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
                "user_id": user_id,
                "order_id": gen_id(),
                "exchange": exchange,
                "market_type": market_type,
                "position_side": position_side,
                "symbol": symbol,
                "type": order_type,
                "side": side,
                "price": fmt_num(px),
                "quantity": fmt_num(order_qty),
                "requested_at": now_ms(),
                "expired_at": None,
                "source": source,
                "strategy_id": strategy_id,
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
        query = """
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
        """
        args = (exchange, market_type, symbol, interval, self.strategy.config.warmup_limit)
        if self.db_pool is not None:
            async with self.db_pool.acquire() as conn:
                rows = await conn.fetch(query, *args)
        else:
            conn = await asyncpg.connect(self.runtime.db_dsn)
            try:
                rows = await conn.fetch(query, *args)
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

        LOGGER.info("warmup completed pair=%s:%s:%s:%s candles=%s", exchange, market_type, symbol, interval, len(rows))

    async def run(self) -> None:
        LOGGER.info(
            "[%s] start strategy_id=%s source=%s interval=%s warmup_limit=%s order_subject=%s "
            "config_refresh_sec=%s queue_size=%s",
            self.strategy.config.name,
            self.runtime.strategy_id,
            self.runtime.source,
            self.strategy.config.interval,
            self.strategy.config.warmup_limit,
            DEFAULT_ORDER_SUBJECT,
            self.runtime.strategy_config_refresh_interval_sec,
            self.runtime.message_queue_size,
        )

        initial_state = self.strategy.snapshot_state()
        state_store: Dict[str, Any] = {}
        entry_price_store: Dict[str, float] = {}
        entry_order_id_store: Dict[str, str] = {}
        metrics = StrategyMetrics()
        redis_store = await RedisStateStore.connect(self.runtime)
        db_pool_min_size = max(1, int(self.runtime.db_pool_min_size or 1))
        db_pool_max_size = max(db_pool_min_size, int(self.runtime.db_pool_max_size or db_pool_min_size))
        self.db_pool = await asyncpg.create_pool(
            self.runtime.db_dsn,
            min_size=db_pool_min_size,
            max_size=db_pool_max_size,
        )
        LOGGER.info(
            "postgres pool ready min_size=%s max_size=%s",
            db_pool_min_size,
            db_pool_max_size,
        )

        nc = NATS()

        async def on_disconnected():
            metrics.nats_disconnects += 1
            LOGGER.warning("NATS disconnected; waiting for reconnect")

        async def on_reconnected():
            metrics.nats_reconnects += 1
            LOGGER.info("NATS reconnected server=%s", nc.connected_url)

        async def on_error(exc):
            metrics.last_error = f"NATS async error: {exc}"
            LOGGER.warning("NATS async error: %s", exc)

        async def on_closed():
            LOGGER.warning("NATS connection closed")

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
        active_pairs: Dict[str, Dict[str, Any]] = {}
        shared_strategy_lock = asyncio.Lock()
        refresh_interval_sec = max(1, int(self.runtime.strategy_config_refresh_interval_sec or 10))
        no_pairs_logged = False
        last_active_summary = ""
        last_reconcile_ms = 0
        monitor_server: Optional[StrategyMonitorServer] = None

        def clone_strategy_for_pair(pair_key: str) -> tuple[StrategyBase, asyncio.Lock]:
            try:
                pair_strategy = copy.deepcopy(self.strategy)
                return pair_strategy, asyncio.Lock()
            except Exception as exc:
                LOGGER.warning(
                    "strategy clone failed pair=%s error=%s; falling back to shared strategy lock",
                    pair_key,
                    exc,
                )
                return self.strategy, shared_strategy_lock

        async def process_message(pair_key: str, msg) -> None:
            try:
                metrics.messages_seen += 1
                pair_ctx = active_pairs.get(pair_key)
                if pair_ctx is None:
                    await msg.ack()
                    metrics.messages_acked += 1
                    return

                async with pair_ctx["lock"]:

                        target = pair_ctx["target"]
                        pair_strategy = pair_ctx["strategy"]
                        expected_strategy = str(target["strategy"]).strip()
                        expected_exchange = str(target["exchange"]).strip().upper()
                        expected_symbol = str(target["symbol"]).strip().upper()
                        expected_interval = str(target["interval"]).strip()
                        expected_market_type = str(target.get("market_type", "spot") or "spot").strip().lower()

                        payload = orjson.loads(msg.data)
                        data = payload.get("data", {})
                        if not isinstance(data, dict):
                            LOGGER.debug(
                                "[%s] nats_market_kline_ignored subject=%s pair=%s reason=invalid_data payload=%r",
                                self.strategy.config.name,
                                getattr(msg, "subject", ""),
                                pair_key,
                                msg.data[:1000],
                            )
                            await msg.ack()
                            metrics.messages_acked += 1
                            return

                        LOGGER.debug(
                            "[%s] nats_market_kline_raw subject=%s pair=%s keys=%s",
                            self.strategy.config.name,
                            getattr(msg, "subject", ""),
                            pair_key,
                            sorted(str(key) for key in data.keys()),
                        )

                        incoming_symbol = str(self._kline_field(data, "Symbol", "symbol")).strip().upper()
                        incoming_interval = str(self._kline_field(data, "Interval", "interval")).strip()
                        incoming_market_type = str(
                            self._kline_field(data, "MarketType", "market_type")
                        ).strip().lower()
                        is_closed = bool(self._kline_field(data, "IsClosed", "is_closed", default=False))

                        if incoming_symbol != expected_symbol:
                            LOGGER.debug(
                                "[%s] nats_market_kline_ignored subject=%s pair=%s reason=symbol_mismatch incoming=%s expected=%s",
                                self.strategy.config.name,
                                getattr(msg, "subject", ""),
                                pair_key,
                                incoming_symbol,
                                expected_symbol,
                            )
                            await msg.ack()
                            metrics.messages_acked += 1
                            return

                        if incoming_interval != expected_interval:
                            LOGGER.debug(
                                "[%s] nats_market_kline_ignored subject=%s pair=%s reason=interval_mismatch incoming=%s expected=%s",
                                self.strategy.config.name,
                                getattr(msg, "subject", ""),
                                pair_key,
                                incoming_interval,
                                expected_interval,
                            )
                            await msg.ack()
                            metrics.messages_acked += 1
                            return

                        effective_market_type = incoming_market_type or expected_market_type
                        if effective_market_type != expected_market_type:
                            LOGGER.debug(
                                "[%s] nats_market_kline_ignored subject=%s pair=%s reason=market_type_mismatch incoming=%s expected=%s",
                                self.strategy.config.name,
                                getattr(msg, "subject", ""),
                                pair_key,
                                effective_market_type,
                                expected_market_type,
                            )
                            await msg.ack()
                            metrics.messages_acked += 1
                            return

                        close_time = self._kline_field(data, "CloseTime", "close_time")
                        close_price = self._kline_field(data, "ClosePrice", "close_price")
                        quote_volume = self._kline_field(data, "QuoteVolume", "quote_volume", default="0")
                        high_price = self._kline_field(data, "HighPrice", "high_price", default=close_price)
                        low_price = self._kline_field(data, "LowPrice", "low_price", default=close_price)
                        taker_quote_volume = self._kline_field(
                            data,
                            "TakerQuoteVolume",
                            "taker_quote_volume",
                            default="0",
                        )
                        trade_count = self._kline_field(data, "TradeCount", "trade_count", default=0)

                        LOGGER.info(
                            "[%s] nats_market_kline_received subject=%s pair=%s exchange=%s market_type=%s "
                            "symbol=%s interval=%s close_time=%s is_closed=%s close=%s",
                            self.strategy.config.name,
                            getattr(msg, "subject", ""),
                            pair_key,
                            expected_exchange,
                            effective_market_type,
                            incoming_symbol,
                            incoming_interval,
                            close_time,
                            is_closed,
                            close_price,
                        )

                        candle = Candle(
                            close_time_ms=parse_iso_to_ms(close_time),
                            close=float(close_price),
                            quote_volume=float(quote_volume or "0"),
                            symbol=incoming_symbol,
                            interval=incoming_interval,
                            high=float(high_price or "0"),
                            low=float(low_price or "0"),
                            taker_quote_volume=float(taker_quote_volume or "0"),
                            trade_count=int(trade_count or 0),
                        )

                        state_key = self._state_key(
                            expected_strategy,
                            expected_exchange,
                            candle.symbol,
                            expected_market_type,
                            candle.interval,
                        )
                        previous_snapshot = state_store.get(state_key)
                        pair_strategy.restore_state(previous_snapshot if previous_snapshot is not None else initial_state)
                        self._apply_strategy_overrides(target.get("order_config"), pair_strategy)

                        snapshot = pair_strategy.snapshot_state()
                        intrabar_risk_exit_enabled = bool(
                            target.get("order_config", {}).get(
                                "enable_intrabar_risk_exit",
                                self.runtime.enable_intrabar_risk_exit,
                            )
                        )
                        if is_closed:
                            signal_result = pair_strategy.on_closed_candle(candle, is_warmup=False)
                        elif intrabar_risk_exit_enabled:
                            signal_result = pair_strategy.on_price_update(candle)
                        else:
                            signal_result = None

                        signals = StrategyBase.normalize_signals(signal_result)
                        if signals:
                            metrics.signals_emitted += len(signals)
                            try:
                                published_summaries = []
                                for signal in signals:
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
                                    LOGGER.info(
                                        "[%s] publish_order subject=%s pair=%s side=%s reason=%s entry_order_id=%s",
                                        self.strategy.config.name,
                                        DEFAULT_ORDER_SUBJECT,
                                        pair_key,
                                        signal.side,
                                        signal.reason,
                                        out["data"].get("entry_order_id", ""),
                                    )
                                    await js.publish(DEFAULT_ORDER_SUBJECT, out_bytes)
                                    metrics.orders_published += 1

                                    if trade_condition == "ENTRY":
                                        persisted_entry = self._metadata_float(signal_metadata, "entry_price")
                                        if persisted_entry is not None:
                                            entry_price_store[state_key] = float(persisted_entry)
                                    elif self._is_exit_trade_condition(trade_condition):
                                        entry_price_store.pop(state_key, None)
                                        entry_order_id_store.pop(state_key, None)

                                    published_summaries.append(
                                        (signal.reason, trade_condition, signal.side, signal.price)
                                    )
                            except Exception:
                                pair_strategy.restore_state(snapshot)
                                raise

                            for reason, trade_condition, side, price in published_summaries:
                                LOGGER.info(
                                    "[%s] signal pair=%s reason=%s trade_condition=%s side=%s symbol=%s close=%.6f",
                                    self.strategy.config.name,
                                    pair_key,
                                    reason,
                                    trade_condition,
                                    side,
                                    candle.symbol,
                                    price,
                                )

                        latest_state = pair_strategy.snapshot_state()
                        state_store[state_key] = latest_state
                        if redis_store is not None:
                            await redis_store.save(
                                state_key,
                                StoredPairState(
                                    strategy_state=latest_state,
                                    entry_price=entry_price_store.get(state_key),
                                    entry_order_id=entry_order_id_store.get(state_key, ""),
                                ),
                            )

                await msg.ack()
                metrics.messages_acked += 1
            except Exception as exc:
                metrics.messages_failed += 1
                metrics.last_error = f"handler error: {type(exc).__name__}: {exc}"
                LOGGER.exception("handler error pair=%s", pair_key)
                with contextlib.suppress(Exception):
                    await msg.nak(delay=1)

        async def pair_worker(pair_key: str) -> None:
            while True:
                pair_ctx = active_pairs.get(pair_key)
                if pair_ctx is None:
                    return
                msg = await pair_ctx["message_queue"].get()
                try:
                    await process_message(pair_key, msg)
                finally:
                    pair_ctx["message_queue"].task_done()

        def build_handler(pair_key: str):
            async def handler(msg):
                pair_ctx = active_pairs.get(pair_key)
                if pair_ctx is None:
                    await msg.ack()
                    metrics.messages_acked += 1
                    return
                try:
                    pair_ctx["message_queue"].put_nowait(msg)
                    metrics.messages_enqueued += 1
                    LOGGER.debug(
                        "[%s] nats_market_kline_enqueued subject=%s pair=%s pending=%s",
                        self.strategy.config.name,
                        getattr(msg, "subject", ""),
                        pair_key,
                        pair_ctx["message_queue"].qsize(),
                    )
                except asyncio.QueueFull:
                    metrics.messages_failed += 1
                    metrics.last_error = f"message queue full pair={pair_key}"
                    LOGGER.warning("message queue full pair=%s", pair_key)
                    await msg.nak(delay=1)

            return handler

        async def reconcile_pairs() -> None:
            nonlocal no_pairs_logged, last_active_summary, last_reconcile_ms

            try:
                desired_pairs = await self.load_strategy_targets_from_order_configs()
            except Exception as exc:
                metrics.last_error = f"failed loading strategy_configs: {exc}"
                LOGGER.warning("failed loading strategy_configs: %s", exc)
                return

            last_reconcile_ms = now_ms()
            metrics.last_reconcile_ms = last_reconcile_ms

            active_keys = set(active_pairs.keys())
            desired_keys = set(desired_pairs.keys())

            changed_keys = {
                key for key in (active_keys & desired_keys) if active_pairs[key]["target"] != desired_pairs[key]
            }
            remove_keys = sorted((active_keys - desired_keys) | changed_keys)
            add_keys = sorted((desired_keys - active_keys) | changed_keys)
            has_changes = bool(remove_keys or add_keys)
            if has_changes:
                metrics.config_resync_count += 1
                LOGGER.info(
                    "strategy config changed add=%s remove=%s active=%s desired=%s",
                    len(add_keys),
                    len(remove_keys),
                    len(active_keys),
                    len(desired_keys),
                )

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
                        LOGGER.warning("unsubscribe error pair=%s error=%s", key, exc)
                queue_name = str(pair_ctx.get("queue", "")).strip()
                if queue_name:
                    with contextlib.suppress(Exception):
                        await js.delete_consumer(KLINE_STREAM_NAME, queue_name)
                worker = pair_ctx.get("worker")
                if worker is not None:
                    worker.cancel()
                    with contextlib.suppress(asyncio.CancelledError):
                        await worker
                message_queue = pair_ctx.get("message_queue")
                if message_queue is not None:
                    while not message_queue.empty():
                        queued_msg = message_queue.get_nowait()
                        with contextlib.suppress(Exception):
                            await queued_msg.nak(delay=1)
                        message_queue.task_done()

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
                if redis_store is not None and self.runtime.redis_delete_removed_state:
                    await redis_store.delete(state_key)
                LOGGER.info("removed strategy pair=%s", key)

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
                    restored = StoredPairState()
                    if redis_store is not None:
                        restored = await redis_store.load(state_key)

                    if restored.strategy_state is not None:
                        state_store[state_key] = restored.strategy_state
                        if restored.entry_price is not None:
                            entry_price_store[state_key] = float(restored.entry_price)
                        if restored.entry_order_id:
                            entry_order_id_store[state_key] = restored.entry_order_id
                        LOGGER.info("restored strategy state from redis pair=%s", key)
                    else:
                        previous = state_store.get(state_key)
                        temp_strategy, _ = clone_strategy_for_pair(key)
                        temp_strategy.restore_state(previous if previous is not None else initial_state)
                        self._apply_strategy_overrides(target.get("order_config"), temp_strategy)
                        original_strategy = self.strategy
                        try:
                            self.strategy = temp_strategy
                            await self.warmup_from_postgres(
                                target["exchange"],
                                target["market_type"],
                                target["symbol"],
                                target["interval"],
                            )
                        finally:
                            self.strategy = original_strategy
                        latest_state = temp_strategy.snapshot_state()
                        state_store[state_key] = latest_state
                        if redis_store is not None:
                            await redis_store.save(state_key, StoredPairState(strategy_state=latest_state))
                except Exception as exc:
                    metrics.last_error = f"warmup skipped for pair={key}: {exc}"
                    LOGGER.warning("warmup skipped pair=%s error=%s", key, exc)

                subject = self._resolve_subject_for_pair(target["exchange"], target["symbol"], target["interval"])
                queue_name = self._resolve_queue_name(
                    target["exchange"],
                    target["market_type"],
                    target["symbol"],
                    target["interval"],
                )
                queue: asyncio.Queue = asyncio.Queue(maxsize=max(1, int(self.runtime.message_queue_size or 1000)))
                pair_strategy, pair_lock = clone_strategy_for_pair(key)
                active_pairs[key] = {
                    "target": target,
                    "strategy": pair_strategy,
                    "subscription": None,
                    "subject": subject,
                    "queue": queue_name,
                    "message_queue": queue,
                    "lock": pair_lock,
                    "worker": None,
                }
                try:
                    try:
                        sub = await js.subscribe(
                            subject,
                            manual_ack=True,
                            queue=queue_name,
                            durable=queue_name,
                            stream=KLINE_STREAM_NAME,
                            deliver_policy=DeliverPolicy.NEW,
                            cb=build_handler(key),
                        )
                    except Exception as subscribe_exc:
                        LOGGER.info(
                            "recreating kline consumer pair=%s stream=%s durable=%s after subscribe error=%s:%s",
                            key,
                            KLINE_STREAM_NAME,
                            queue_name,
                            type(subscribe_exc).__name__,
                            subscribe_exc,
                        )
                        with contextlib.suppress(Exception):
                            await js.delete_consumer(KLINE_STREAM_NAME, queue_name)
                        sub = await js.subscribe(
                            subject,
                            manual_ack=True,
                            queue=queue_name,
                            durable=queue_name,
                            stream=KLINE_STREAM_NAME,
                            deliver_policy=DeliverPolicy.NEW,
                            cb=build_handler(key),
                        )
                except Exception as exc:
                    metrics.last_error = f"subscribe error pair={key}: {exc}"
                    LOGGER.warning(
                        "subscribe error pair=%s subject=%s stream=%s queue=%s durable=%s error=%s:%s; retrying on next reconcile",
                        key,
                        subject,
                        KLINE_STREAM_NAME,
                        queue_name,
                        queue_name,
                        type(exc).__name__,
                        exc,
                    )
                    active_pairs.pop(key, None)
                    continue
                active_pairs[key]["subscription"] = sub
                active_pairs[key]["worker"] = asyncio.create_task(pair_worker(key))
                LOGGER.info(
                    "subscribed strategy pair=%s stream=%s subject=%s queue=%s durable=%s",
                    key,
                    KLINE_STREAM_NAME,
                    subject,
                    queue_name,
                    queue_name,
                )

            if active_pairs:
                no_pairs_logged = False
                active_summary = ",".join(sorted(active_pairs.keys()))
                if has_changes or active_summary != last_active_summary:
                    LOGGER.info("%s strategy running pairs=%s", self.strategy.config.name, active_summary)
                    last_active_summary = active_summary
            elif not no_pairs_logged:
                LOGGER.info(
                    "%s no strategy_configs rows; refreshing every %ss",
                    self.strategy.config.name,
                    refresh_interval_sec,
                )
                no_pairs_logged = True
                last_active_summary = ""

        async def monitor_payload() -> Dict[str, Any]:
            def _slice_by_tokens(values: Dict[str, Any], tokens: tuple[str, ...]) -> Dict[str, Any]:
                out: Dict[str, Any] = {}
                for key, value in values.items():
                    normalized = str(key).strip().lower()
                    if any(token in normalized for token in tokens):
                        out[key] = value
                return out

            active_pair_map = {key: ctx.get("target", {}) for key, ctx in active_pairs.items()}
            state_by_pair = {key: self._to_jsonable(value) for key, value in state_store.items()}
            subscriptions = {
                key: {
                    "subject": str(ctx.get("subject", "")),
                    "queue": str(ctx.get("queue", "")),
                    "target": self._to_jsonable(ctx.get("target", {})),
                    "pending_messages": ctx.get("message_queue").qsize() if ctx.get("message_queue") is not None else 0,
                }
                for key, ctx in active_pairs.items()
            }

            risk_tokens = (
                "risk",
                "stop",
                "sl",
                "tp",
                "take_profit",
                "trailing",
                "max_hold",
                "cooldown",
                "pause",
                "loss",
                "entry",
                "position",
            )
            indicator_tokens = (
                "ema",
                "vwap",
                "macd",
                "atr",
                "rsi",
                "vol",
                "volume",
                "mean",
                "hist",
                "spread",
            )

            strategy_state_by_pair: Dict[str, Any] = {}
            for pair_key, state_value in state_by_pair.items():
                as_dict = state_value if isinstance(state_value, dict) else {"raw": state_value}
                strategy_state_by_pair[pair_key] = {
                    "all_variables": as_dict,
                    "risk_config": _slice_by_tokens(as_dict, risk_tokens),
                    "indicator_state": _slice_by_tokens(as_dict, indicator_tokens),
                }

            return {
                "status": "ok",
                "strategy": {
                    "name": self.strategy.config.name,
                    "strategy_id": self.runtime.strategy_id,
                    "source": self.runtime.source,
                },
                "strategy_config": self._to_jsonable(vars(self.strategy.config)),
                "runtime": {
                    "order_subject": DEFAULT_ORDER_SUBJECT,
                    "enable_intrabar_risk_exit": self.runtime.enable_intrabar_risk_exit,
                },
                "health": {
                    "started_at_ms": metrics.started_at_ms,
                    "now_ms": now_ms(),
                    "uptime_sec": max(0, (now_ms() - metrics.started_at_ms) // 1000),
                    "nats_connected": bool(nc.is_connected),
                    "nats_closed": bool(nc.is_closed),
                    "last_reconcile_ms": last_reconcile_ms,
                    "redis_enabled": redis_store is not None,
                },
                "metrics": {
                    "messages_seen": metrics.messages_seen,
                    "messages_enqueued": metrics.messages_enqueued,
                    "messages_acked": metrics.messages_acked,
                    "messages_failed": metrics.messages_failed,
                    "signals_emitted": metrics.signals_emitted,
                    "orders_published": metrics.orders_published,
                    "config_resync_count": metrics.config_resync_count,
                    "nats_disconnects": metrics.nats_disconnects,
                    "nats_reconnects": metrics.nats_reconnects,
                    "last_error": metrics.last_error,
                },
                "active_pairs_count": len(active_pair_map),
                "active_pairs": active_pair_map,
                "subscriptions": subscriptions,
                "strategy_state_by_pair": strategy_state_by_pair,
            }

        try:
            if self.runtime.monitor_enabled:
                monitor_server = StrategyMonitorServer(
                    self.runtime.monitor_host,
                    self.runtime.monitor_port,
                    monitor_payload,
                )
                await monitor_server.start()

            while True:
                if nc.is_closed:
                    raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
                await reconcile_pairs()
                await asyncio.sleep(refresh_interval_sec)
        finally:
            for pair_ctx in list(active_pairs.values()):
                worker = pair_ctx.get("worker")
                if worker is not None:
                    worker.cancel()
                    with contextlib.suppress(asyncio.CancelledError):
                        await worker
            if monitor_server is not None:
                await monitor_server.close()
            if redis_store is not None:
                await redis_store.close()
            if self.db_pool is not None:
                await self.db_pool.close()
                self.db_pool = None
            if not nc.is_closed:
                await nc.close()
