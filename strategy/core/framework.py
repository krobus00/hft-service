import asyncio
from abc import ABC, abstractmethod
from typing import Any, Dict, Optional

import asyncpg
import orjson
from nats.aio.client import Client as NATS

from .common import fmt_num, gen_id, now_ms, parse_iso_to_ms, pct_to_frac
from .models import Candle, RuntimeConfig, Signal, StrategyConfig


class StrategyBase(ABC):
    __slots__ = ("config", "last_close_time_ms")

    def __init__(self, config: StrategyConfig):
        self.config = config
        self.last_close_time_ms = 0

    def allow_new_candle(self, candle: Candle) -> bool:
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

    @abstractmethod
    def on_closed_candle(self, candle: Candle, is_warmup: bool = False) -> Optional[Signal]:
        raise NotImplementedError


class StrategyRunner:
    __slots__ = ("strategy", "runtime")

    def __init__(self, strategy: StrategyBase, runtime: RuntimeConfig):
        self.strategy = strategy
        self.runtime = runtime

    def build_order_payload(self, side: str, price: float) -> dict:
        px = float(price)
        if self.runtime.order_type == "LIMIT":
            if side == "BUY":
                px = price * (1.0 + pct_to_frac(self.runtime.limit_slippage_pct))
            else:
                px = price * (1.0 - pct_to_frac(self.runtime.limit_slippage_pct))

        return {
            "retry": 0,
            "data": {
                "request_id": gen_id(),
                "user_id": self.runtime.user_id,
                "order_id": gen_id(),
                "exchange": self.runtime.exchange,
                "market_type": self.runtime.market_type,
                "position_side": self.runtime.position_side,
                "symbol": self.runtime.order_symbol,
                "type": self.runtime.order_type,
                "side": side,
                "price": fmt_num(px),
                "quantity": fmt_num(float(self.runtime.order_qty)),
                "requested_at": now_ms(),
                "expired_at": None,
                "source": self.runtime.source,
                "strategy_id": self.runtime.strategy_id,
                "is_paper_trading": self.runtime.is_paper_trading,
            },
        }

    async def warmup_from_postgres(self) -> None:
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
                WHERE symbol = $1
                  AND interval = $2
                  AND is_closed IS TRUE
                ORDER BY close_time DESC
                LIMIT $3
                """,
                self.strategy.config.symbol,
                self.strategy.config.interval,
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
                high=float(row["high_price"]),
                low=float(row["low_price"]),
                taker_quote_volume=float(row["taker_quote_volume"]),
                trade_count=int(row["trade_count"]),
            )
            self.strategy.on_closed_candle(candle, is_warmup=True)

        print(f"Warmup completed with {len(rows)} candles", flush=True)

    async def run(self) -> None:
        if self.runtime.order_qty <= 0:
            raise ValueError("order_qty must be > 0")

        try:
            await self.warmup_from_postgres()
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

        async def handler(msg):
            try:
                payload = orjson.loads(msg.data)
                data = payload.get("data", {})

                if not data.get("IsClosed"):
                    await msg.ack()
                    return

                if data.get("Symbol") != self.strategy.config.symbol:
                    await msg.ack()
                    return

                if data.get("Interval") != self.strategy.config.interval:
                    await msg.ack()
                    return

                candle = Candle(
                    close_time_ms=parse_iso_to_ms(data["CloseTime"]),
                    close=float(data["ClosePrice"]),
                    quote_volume=float(data.get("QuoteVolume", "0") or "0"),
                    high=float(data.get("HighPrice", data.get("ClosePrice", "0")) or "0"),
                    low=float(data.get("LowPrice", data.get("ClosePrice", "0")) or "0"),
                    taker_quote_volume=float(data.get("TakerQuoteVolume", "0") or "0"),
                    trade_count=int(data.get("TradeCount", 0) or 0),
                )

                snapshot = self.strategy.snapshot_state()
                signal = self.strategy.on_closed_candle(candle, is_warmup=False)
                if signal is not None:
                    try:
                        out = self.build_order_payload(signal.side, signal.price)
                        await js.publish(self.runtime.order_subject, orjson.dumps(out))
                    except Exception:
                        self.strategy.restore_state(snapshot)
                        raise
                    metadata = " ".join(f"{k}={v}" for k, v in signal.metadata.items())
                    print(
                        f"[{self.strategy.config.name}] {signal.reason} side={signal.side} symbol={self.runtime.order_symbol} close={signal.price:.6f} {metadata}".strip(),
                        flush=True,
                    )

                await msg.ack()
            except Exception as exc:
                print(f"handler error: {exc}", flush=True)

        await js.subscribe(
            self.runtime.kline_subject,
            manual_ack=True,
            queue=self.runtime.queue_name,
            cb=handler,
        )

        print(f"{self.strategy.config.name} strategy running...", flush=True)
        while True:
            if nc.is_closed:
                raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
            await asyncio.sleep(1)
