import asyncio
import time
import uuid
from collections import deque
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Optional

import orjson
import uvloop
import yaml
from nats.aio.client import Client as NATS

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

BASE_DIR = Path(__file__).resolve().parent
CONFIG_PATH = BASE_DIR / "config.yml"


def load_config() -> dict:
    if not CONFIG_PATH.exists():
        raise FileNotFoundError(
            f"Config file not found: {CONFIG_PATH}. Copy strategy/config.yml.example to strategy/config.yml"
        )
    with CONFIG_PATH.open("r", encoding="utf-8") as file:
        return yaml.safe_load(file) or {}


CONFIG = load_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
VWAP_CONFIG = CONFIG.get("vwap", {})

NATS_URL = GLOBAL_CONFIG.get("nats_url", "nats://localhost:4222")
NATS_ALLOW_RECONNECT = GLOBAL_CONFIG.get("nats_allow_reconnect", True)
NATS_MAX_RECONNECT_ATTEMPTS = GLOBAL_CONFIG.get("nats_max_reconnect_attempts", -1)
NATS_RECONNECT_TIME_WAIT_SEC = GLOBAL_CONFIG.get("nats_reconnect_time_wait_sec", 2)
NATS_CONNECT_TIMEOUT_SEC = GLOBAL_CONFIG.get("nats_connect_timeout_sec", 5)
NATS_PING_INTERVAL_SEC = GLOBAL_CONFIG.get("nats_ping_interval_sec", 30)
NATS_MAX_OUTSTANDING_PINGS = GLOBAL_CONFIG.get("nats_max_outstanding_pings", 3)

KLINE_SUBJECT = VWAP_CONFIG.get("kline_subject", "KLINE.TOKOCRYPTO.>")
ORDER_SUBJECT = VWAP_CONFIG.get("order_subject", "order_engine.place_order")
QUEUE_NAME = VWAP_CONFIG.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_VWAP")

SYMBOL = VWAP_CONFIG.get("symbol", "SOLUSDT")
INTERVAL = VWAP_CONFIG.get("interval", "1m")

USER_ID = VWAP_CONFIG.get("user_id", "paper-1")
EXCHANGE = VWAP_CONFIG.get("exchange", "tokocrypto")
SOURCE = VWAP_CONFIG.get("source", "python-vwap")
STRATEGY_ID = VWAP_CONFIG.get("strategy_id", "python-vwap-mean-revert-hf-v1")
IS_PAPER_TRADING = VWAP_CONFIG.get("is_paper_trading", True)

ORDER_TYPE = VWAP_CONFIG.get("order_type", "LIMIT")
ORDER_QTY = VWAP_CONFIG.get("order_qty", 10)
ORDER_SYMBOL = VWAP_CONFIG.get("order_symbol", "SOLUSDT")
LIMIT_SLIPPAGE_BPS = VWAP_CONFIG.get("limit_slippage_bps", 2)

VWAP_WINDOW = VWAP_CONFIG.get("vwap_window", 30)
ENTRY_THRESHOLD_BPS = VWAP_CONFIG.get("entry_threshold_bps", 5)
TP_BPS = VWAP_CONFIG.get("tp_bps", 2)
SL_BPS = VWAP_CONFIG.get("sl_bps", 8)
MAX_HOLD_BARS = VWAP_CONFIG.get("max_hold_bars", 8)
COOLDOWN_BARS = VWAP_CONFIG.get("cooldown_bars", 1)


def bps_to_frac(bps: float) -> float:
    return bps / 10_000.0


def fmt_num(x: float) -> str:
    return f"{x:.8f}".rstrip("0").rstrip(".")


def parse_iso_to_ms(s: str) -> int:
    dt = datetime.fromisoformat(s.replace("Z", "+00:00"))
    return int(dt.timestamp() * 1000)


@dataclass
class Candle:
    close_time_ms: int
    close: float
    quote_volume: float


class RollingVWAP:
    __slots__ = ("window", "buf", "sum_pv", "sum_v")

    def __init__(self, window: int):
        self.window = max(2, int(window))
        self.buf = deque()
        self.sum_pv = 0.0
        self.sum_v = 0.0

    def update(self, price: float, volume: float) -> Optional[float]:
        v = max(0.0, float(volume))
        pv = price * v
        self.buf.append((pv, v))
        self.sum_pv += pv
        self.sum_v += v

        if len(self.buf) > self.window:
            old_pv, old_v = self.buf.popleft()
            self.sum_pv -= old_pv
            self.sum_v -= old_v

        if self.sum_v <= 0:
            return None
        return self.sum_pv / self.sum_v


class VWAPMeanReversionHF:
    __slots__ = (
        "vwap",
        "last_close_time_ms",
        "pos",
        "entry_price",
        "bars_in_pos",
        "cooldown",
    )

    def __init__(self):
        self.vwap = RollingVWAP(VWAP_WINDOW)
        self.last_close_time_ms = 0

        self.pos = "FLAT"  # FLAT | LONG | SHORT
        self.entry_price: Optional[float] = None
        self.bars_in_pos = 0
        self.cooldown = 0

    def _entry_band(self, vwap_px: float):
        band = vwap_px * bps_to_frac(ENTRY_THRESHOLD_BPS)
        return vwap_px - band, vwap_px + band

    def on_closed_candle(self, c: Candle):
        if c.close_time_ms <= self.last_close_time_ms:
            return None
        self.last_close_time_ms = c.close_time_ms

        if self.cooldown > 0:
            self.cooldown -= 1

        vwap_px = self.vwap.update(c.close, c.quote_volume)
        if vwap_px is None:
            return None

        lower, upper = self._entry_band(vwap_px)

        if self.pos == "LONG":
            self.bars_in_pos += 1
            take_px = self.entry_price * (1.0 + bps_to_frac(TP_BPS))
            stop_px = self.entry_price * (1.0 - bps_to_frac(SL_BPS))
            exit_signal = c.close >= take_px or c.close <= stop_px or c.close >= vwap_px or self.bars_in_pos >= MAX_HOLD_BARS
            if exit_signal:
                if c.close >= take_px:
                    reason = "TP_LONG"
                elif c.close <= stop_px:
                    reason = "SL_LONG"
                elif c.close >= vwap_px:
                    reason = "MEAN_REVERT_LONG"
                else:
                    reason = "TIME_EXIT_LONG"
                self.pos = "FLAT"
                self.entry_price = None
                self.bars_in_pos = 0
                self.cooldown = COOLDOWN_BARS
                return "SELL", c.close, reason, vwap_px, lower, upper

        elif self.pos == "SHORT":
            self.bars_in_pos += 1
            take_px = self.entry_price * (1.0 - bps_to_frac(TP_BPS))
            stop_px = self.entry_price * (1.0 + bps_to_frac(SL_BPS))
            exit_signal = c.close <= take_px or c.close >= stop_px or c.close <= vwap_px or self.bars_in_pos >= MAX_HOLD_BARS
            if exit_signal:
                if c.close <= take_px:
                    reason = "TP_SHORT"
                elif c.close >= stop_px:
                    reason = "SL_SHORT"
                elif c.close <= vwap_px:
                    reason = "MEAN_REVERT_SHORT"
                else:
                    reason = "TIME_EXIT_SHORT"
                self.pos = "FLAT"
                self.entry_price = None
                self.bars_in_pos = 0
                self.cooldown = COOLDOWN_BARS
                return "BUY", c.close, reason, vwap_px, lower, upper

        if self.pos == "FLAT" and self.cooldown == 0:
            if c.close > upper:
                self.pos = "SHORT"
                self.entry_price = c.close
                self.bars_in_pos = 0
                return "SELL", c.close, "ENTER_SHORT", vwap_px, lower, upper
            if c.close < lower:
                self.pos = "LONG"
                self.entry_price = c.close
                self.bars_in_pos = 0
                return "BUY", c.close, "ENTER_LONG", vwap_px, lower, upper

        return None


def gen_id() -> str:
    return uuid.uuid4().hex


def now_ms() -> int:
    return int(time.time() * 1000)


def build_order_payload(side: str, price: float) -> dict:
    px = price
    if ORDER_TYPE == "LIMIT":
        if side == "BUY":
            px = price * (1.0 + bps_to_frac(LIMIT_SLIPPAGE_BPS))
        else:
            px = price * (1.0 - bps_to_frac(LIMIT_SLIPPAGE_BPS))

    return {
        "retry": 0,
        "data": {
            "request_id": gen_id(),
            "user_id": USER_ID,
            "order_id": gen_id(),
            "exchange": EXCHANGE,
            "symbol": ORDER_SYMBOL,
            "type": ORDER_TYPE,
            "side": side,
            "price": fmt_num(px),
            "quantity": fmt_num(float(ORDER_QTY)),
            "requested_at": now_ms(),
            "expired_at": None,
            "source": SOURCE,
            "strategy_id": STRATEGY_ID,
            "is_paper_trading": IS_PAPER_TRADING,
        },
    }


async def run():
    if ORDER_QTY <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = VWAPMeanReversionHF()

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
        NATS_URL,
        allow_reconnect=NATS_ALLOW_RECONNECT,
        max_reconnect_attempts=NATS_MAX_RECONNECT_ATTEMPTS,
        reconnect_time_wait=NATS_RECONNECT_TIME_WAIT_SEC,
        connect_timeout=NATS_CONNECT_TIMEOUT_SEC,
        ping_interval=NATS_PING_INTERVAL_SEC,
        max_outstanding_pings=NATS_MAX_OUTSTANDING_PINGS,
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

            if data.get("Symbol") != SYMBOL:
                await msg.ack()
                return

            if data.get("Interval") != INTERVAL:
                await msg.ack()
                return

            candle = Candle(
                close_time_ms=parse_iso_to_ms(data["CloseTime"]),
                close=float(data["ClosePrice"]),
                quote_volume=float(data.get("QuoteVolume", "0") or "0"),
            )

            signal = strategy.on_closed_candle(candle)
            if signal:
                side, px, reason, vwap_px, lower, upper = signal
                out = build_order_payload(side, px)
                await js.publish(ORDER_SUBJECT, orjson.dumps(out))
                print(
                    f"[VWAP-HF] {reason} side={side} symbol={ORDER_SYMBOL} close={px:.4f} "
                    f"vwap={vwap_px:.4f} band=({lower:.4f},{upper:.4f})",
                    flush=True,
                )

            await msg.ack()
        except Exception as exc:
            # Keep message unacked so transient failures can be retried.
            print(f"handler error: {exc}", flush=True)
            return

    await js.subscribe(
        KLINE_SUBJECT,
        manual_ack=True,
        queue=QUEUE_NAME,
        cb=handler,
    )

    print("VWAP mean-reversion HF strategy running...", flush=True)
    while True:
        if nc.is_closed:
            raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
        await asyncio.sleep(1)


if __name__ == "__main__":
    asyncio.run(run())
