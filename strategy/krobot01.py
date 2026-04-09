import asyncio
import time
import uuid
from collections import deque
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Optional

import asyncpg
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
KROBOT01_CONFIG = CONFIG.get("krobot01", {})

DB_DSN = GLOBAL_CONFIG.get("db_dsn", "postgres://root:root@localhost:5432/market_data?sslmode=disable")
NATS_URL = GLOBAL_CONFIG.get("nats_url", "nats://localhost:4222")
NATS_ALLOW_RECONNECT = GLOBAL_CONFIG.get("nats_allow_reconnect", True)
NATS_MAX_RECONNECT_ATTEMPTS = GLOBAL_CONFIG.get("nats_max_reconnect_attempts", -1)
NATS_RECONNECT_TIME_WAIT_SEC = GLOBAL_CONFIG.get("nats_reconnect_time_wait_sec", 2)
NATS_CONNECT_TIMEOUT_SEC = GLOBAL_CONFIG.get("nats_connect_timeout_sec", 5)
NATS_PING_INTERVAL_SEC = GLOBAL_CONFIG.get("nats_ping_interval_sec", 30)
NATS_MAX_OUTSTANDING_PINGS = GLOBAL_CONFIG.get("nats_max_outstanding_pings", 3)

KLINE_SUBJECT = KROBOT01_CONFIG.get("kline_subject", "KLINE.TOKOCRYPTO.>")
ORDER_SUBJECT = KROBOT01_CONFIG.get("order_subject", "order_engine.place_order")
QUEUE_NAME = KROBOT01_CONFIG.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_KROBOT01")

SYMBOL = KROBOT01_CONFIG.get("symbol", "SOLUSDT")
INTERVAL = KROBOT01_CONFIG.get("interval", "1m")

USER_ID = KROBOT01_CONFIG.get("user_id", "paper-1")
EXCHANGE = KROBOT01_CONFIG.get("exchange", "tokocrypto")
SOURCE = KROBOT01_CONFIG.get("source", "python-krobot01")
STRATEGY_ID = KROBOT01_CONFIG.get("strategy_id", "python-krobot01-ema-vwap-macd")
IS_PAPER_TRADING = KROBOT01_CONFIG.get("is_paper_trading", True)

ORDER_TYPE = KROBOT01_CONFIG.get("order_type", "LIMIT")
ORDER_QTY = KROBOT01_CONFIG.get("order_qty", 10)
ORDER_SYMBOL = KROBOT01_CONFIG.get("order_symbol", "SOLUSDT")
LIMIT_SLIPPAGE_PCT = KROBOT01_CONFIG.get(
    "limit_slippage_pct", KROBOT01_CONFIG.get("limit_slippage_bps", 2) / 100.0
)

EMA_PERIOD = int(KROBOT01_CONFIG.get("ema_period", 200))
VWAP_WINDOW = int(KROBOT01_CONFIG.get("vwap_window", 200))
MACD_FAST = int(KROBOT01_CONFIG.get("macd_fast", 12))
MACD_SLOW = int(KROBOT01_CONFIG.get("macd_slow", 26))
MACD_SIGNAL = int(KROBOT01_CONFIG.get("macd_signal", 9))
COOLDOWN_BARS = int(KROBOT01_CONFIG.get("cooldown_bars", 1))
HISTORICAL_LIMIT = int(KROBOT01_CONFIG.get("historical_limit", 800))


def pct_to_frac(pct: float) -> float:
    return pct / 100.0


def fmt_num(x: float) -> str:
    return f"{x:.8f}".rstrip("0").rstrip(".")


def parse_iso_to_ms(s: str) -> int:
    dt = datetime.fromisoformat(s.replace("Z", "+00:00"))
    return int(dt.timestamp() * 1000)


def now_ms() -> int:
    return int(time.time() * 1000)


def gen_id() -> str:
    return uuid.uuid4().hex


@dataclass
class Candle:
    close_time_ms: int
    close: float
    quote_volume: float


class EMA:
    __slots__ = ("alpha", "value", "count")

    def __init__(self, period: int):
        self.alpha = 2.0 / (max(1, period) + 1)
        self.value = 0.0
        self.count = 0

    def update(self, price: float) -> float:
        self.count += 1
        if self.count == 1:
            self.value = price
        else:
            self.value = self.alpha * price + (1.0 - self.alpha) * self.value
        return self.value


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


class MACD:
    __slots__ = (
        "ema_fast",
        "ema_slow",
        "ema_signal",
        "prev_diff",
        "count",
        "crossed_up_active",
        "crossed_down_active",
    )

    def __init__(self, fast: int, slow: int, signal: int):
        self.ema_fast = EMA(fast)
        self.ema_slow = EMA(slow)
        self.ema_signal = EMA(signal)
        self.prev_diff: Optional[float] = None
        self.count = 0
        self.crossed_up_active = False
        self.crossed_down_active = False

    def update(self, price: float):
        self.count += 1
        fast = self.ema_fast.update(price)
        slow = self.ema_slow.update(price)
        macd_line = fast - slow
        signal_line = self.ema_signal.update(macd_line)
        diff = macd_line - signal_line

        if self.prev_diff is not None:
            if self.prev_diff <= 0 and diff > 0:
                self.crossed_up_active = True
                self.crossed_down_active = False
            elif self.prev_diff >= 0 and diff < 0:
                self.crossed_up_active = False
                self.crossed_down_active = True
        else:
            self.crossed_up_active = diff > 0
            self.crossed_down_active = diff < 0
        self.prev_diff = diff

        return macd_line, signal_line, self.crossed_up_active, self.crossed_down_active


class Krobot01Strategy:
    __slots__ = ("ema", "vwap", "macd", "last_close_time_ms", "cooldown")

    def __init__(self):
        self.ema = EMA(EMA_PERIOD)
        self.vwap = RollingVWAP(VWAP_WINDOW)
        self.macd = MACD(MACD_FAST, MACD_SLOW, MACD_SIGNAL)
        self.last_close_time_ms = 0
        self.cooldown = 0

    def on_closed_candle(self, c: Candle):
        if c.close_time_ms <= self.last_close_time_ms:
            return None
        self.last_close_time_ms = c.close_time_ms

        if self.cooldown > 0:
            self.cooldown -= 1

        ema = self.ema.update(c.close)
        vwap_px = self.vwap.update(c.close, c.quote_volume)
        _, _, crossed_up, crossed_down = self.macd.update(c.close)

        if self.ema.count < EMA_PERIOD or vwap_px is None or self.macd.count < max(50, MACD_SLOW + MACD_SIGNAL):
            return None

        if self.cooldown > 0:
            return None

        long_cond = c.close > ema and c.close > vwap_px and crossed_up
        short_cond = c.close < ema and c.close < vwap_px and crossed_down

        print(
            f"close= {c.close:.6f} EMA={ema:.6f} VWAP={vwap_px:.6f} MACD_crossed_up={crossed_up} crossed_down={crossed_down} long_cond={long_cond} short_cond={short_cond}",
            flush=True,)

        if long_cond:
            self.cooldown = COOLDOWN_BARS
            return "BUY", c.close, "ENTER_LONG", ema, vwap_px

        if short_cond:
            self.cooldown = COOLDOWN_BARS
            return "SELL", c.close, "ENTER_SHORT", ema, vwap_px

        return None


def build_order_payload(side: str, price: float) -> dict:
    px = price
    if ORDER_TYPE == "LIMIT":
        if side == "BUY":
            px = price * (1.0 + pct_to_frac(LIMIT_SLIPPAGE_PCT))
        else:
            px = price * (1.0 - pct_to_frac(LIMIT_SLIPPAGE_PCT))

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


async def warmup_from_postgres(strategy: Krobot01Strategy):
    conn = await asyncpg.connect(DB_DSN)
    try:
        rows = await conn.fetch(
            """
            SELECT close_time, close_price, quote_volume
            FROM market_klines
            WHERE symbol = $1
              AND interval = $2
              AND is_closed IS TRUE
            ORDER BY close_time DESC
            LIMIT $3
            """,
            SYMBOL,
            INTERVAL,
            HISTORICAL_LIMIT,
        )
    finally:
        await conn.close()

    rows = list(reversed(rows))
    for row in rows:
        candle = Candle(
            close_time_ms=int(row["close_time"].timestamp() * 1000),
            close=float(row["close_price"]),
            quote_volume=float(row["quote_volume"]),
        )
        strategy.on_closed_candle(candle)

    print(f"Warmup completed with {len(rows)} candles", flush=True)


async def run():
    if ORDER_QTY <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = Krobot01Strategy()

    try:
        await warmup_from_postgres(strategy)
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
            print(f"Received closed candle: close_time={data.get('CloseTime')} close_price={data.get('ClosePrice')}", flush=True)
            candle = Candle(
                close_time_ms=parse_iso_to_ms(data["CloseTime"]),
                close=float(data["ClosePrice"]),
                quote_volume=float(data.get("QuoteVolume", "0") or "0"),
            )

            signal = strategy.on_closed_candle(candle)
            if signal:
                side, px, reason, ema, vwap_px = signal
                out = build_order_payload(side, px)
                await js.publish(ORDER_SUBJECT, orjson.dumps(out))
                print(
                    f"[KROBOT01] {reason} side={side} symbol={ORDER_SYMBOL} close={px:.6f} ema={ema:.6f} vwap={vwap_px:.6f}",
                    flush=True,
                )

            await msg.ack()
        except Exception as exc:
            print(f"handler error: {exc}", flush=True)
            return

    await js.subscribe(
        KLINE_SUBJECT,
        manual_ack=True,
        queue=QUEUE_NAME,
        cb=handler,
    )

    print("Krobot01 strategy running...", flush=True)
    while True:
        if nc.is_closed:
            raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
        await asyncio.sleep(1)


if __name__ == "__main__":
    asyncio.run(run())
