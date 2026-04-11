import asyncio
import time
import uuid
from pathlib import Path
from dataclasses import dataclass
from datetime import datetime
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
EXAMPLE_CONFIG = CONFIG.get("example", {})

NATS_URL = GLOBAL_CONFIG.get("nats_url", "nats://localhost:4222")
NATS_ALLOW_RECONNECT = GLOBAL_CONFIG.get("nats_allow_reconnect", True)
NATS_MAX_RECONNECT_ATTEMPTS = GLOBAL_CONFIG.get("nats_max_reconnect_attempts", -1)
NATS_RECONNECT_TIME_WAIT_SEC = GLOBAL_CONFIG.get("nats_reconnect_time_wait_sec", 2)
NATS_CONNECT_TIMEOUT_SEC = GLOBAL_CONFIG.get("nats_connect_timeout_sec", 5)
NATS_PING_INTERVAL_SEC = GLOBAL_CONFIG.get("nats_ping_interval_sec", 30)
NATS_MAX_OUTSTANDING_PINGS = GLOBAL_CONFIG.get("nats_max_outstanding_pings", 3)

KLINE_SUBJECT = EXAMPLE_CONFIG.get("kline_subject", "KLINE.TOKOCRYPTO.>")
ORDER_SUBJECT = EXAMPLE_CONFIG.get("order_subject", "order_engine.place_order")
QUEUE_NAME = EXAMPLE_CONFIG.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_EXAMPLE")

SYMBOL = EXAMPLE_CONFIG.get("symbol", "SOLUSDT")
INTERVAL = EXAMPLE_CONFIG.get("interval", "1m")

USER_ID = EXAMPLE_CONFIG.get("user_id", "paper-1")
EXCHANGE = EXAMPLE_CONFIG.get("exchange", "tokocrypto")
MARKET_TYPE = EXAMPLE_CONFIG.get("market_type", "spot")
POSITION_SIDE = EXAMPLE_CONFIG.get("position_side", "BOTH")
SOURCE = EXAMPLE_CONFIG.get("source", "python-example")
STRATEGY_ID = EXAMPLE_CONFIG.get("strategy_id", "python-example")
IS_PAPER_TRADING = EXAMPLE_CONFIG.get("is_paper_trading", True)

ORDER_TYPE = EXAMPLE_CONFIG.get("order_type", "LIMIT")
ORDER_QTY = EXAMPLE_CONFIG.get("order_qty", 10)
ORDER_SYMBOL = EXAMPLE_CONFIG.get("order_symbol", "SOLUSDT")
LIMIT_SLIPPAGE_PCT = EXAMPLE_CONFIG.get("limit_slippage_pct", EXAMPLE_CONFIG.get("limit_slippage_bps", 3) / 100.0)

# HFT signal params
FAST_EMA_N = EXAMPLE_CONFIG.get("fast_ema_n", 8)
SLOW_EMA_N = EXAMPLE_CONFIG.get("slow_ema_n", 21)
RET_LOOKBACK = EXAMPLE_CONFIG.get("ret_lookback", 3)
RET_ENTRY_PCT = EXAMPLE_CONFIG.get("ret_entry_pct", EXAMPLE_CONFIG.get("ret_entry_bps", 7) / 100.0)
MIN_TAKER_RATIO = EXAMPLE_CONFIG.get("min_taker_ratio", 0.53)

TRADE_COUNT_WINDOW = EXAMPLE_CONFIG.get("trade_count_window", 80)
MIN_TRADE_COUNT_PCTL = EXAMPLE_CONFIG.get("min_trade_count_pctl", 0.35)

ATR_N = EXAMPLE_CONFIG.get("atr_n", 14)
STOP_ATR = EXAMPLE_CONFIG.get("stop_atr", 1.0)
TAKE_ATR = EXAMPLE_CONFIG.get("take_atr", 1.6)

MIN_EMA_SPREAD_PCT = EXAMPLE_CONFIG.get("min_ema_spread_pct", EXAMPLE_CONFIG.get("min_ema_spread_bps", 4) / 100.0)
MIN_ATR_PCT = EXAMPLE_CONFIG.get("min_atr_pct", EXAMPLE_CONFIG.get("min_atr_bps", 8) / 100.0)
MIN_RANGE_PCT = EXAMPLE_CONFIG.get("min_range_pct", EXAMPLE_CONFIG.get("min_range_bps", 10) / 100.0)
ENTRY_CONFIRM_BARS = EXAMPLE_CONFIG.get("entry_confirm_bars", 2)
MIN_HOLD_BARS = EXAMPLE_CONFIG.get("min_hold_bars", 3)
ROUND_TRIP_COST_PCT = EXAMPLE_CONFIG.get("round_trip_cost_pct", EXAMPLE_CONFIG.get("round_trip_cost_bps", 12) / 100.0)
MIN_EXPECTED_EDGE_MULT = EXAMPLE_CONFIG.get("min_expected_edge_mult", 1.3)

COOLDOWN_BARS = EXAMPLE_CONFIG.get("cooldown_bars", 2)
MAX_HOLD_BARS = EXAMPLE_CONFIG.get("max_hold_bars", 20)
LONG_ONLY = EXAMPLE_CONFIG.get("long_only", True)


def pct_to_frac(pct: float) -> float:
    return pct / 100.0


def fmt_num(x: float) -> str:
    return f"{x:.8f}".rstrip("0").rstrip(".")


def parse_iso_to_ms(s: str) -> int:
    dt = datetime.fromisoformat(s.replace("Z", "+00:00"))
    return int(dt.timestamp() * 1000)


def percentile(xs, p: float) -> float:
    if not xs:
        return 0.0
    ys = sorted(xs)
    k = int(round((len(ys) - 1) * p))
    return float(ys[max(0, min(len(ys) - 1, k))])


@dataclass
class Candle:
    close_time_ms: int
    close: float
    high: float
    low: float
    quote_volume: float
    taker_quote_volume: float
    trade_count: int

    @property
    def taker_ratio(self) -> float:
        if self.quote_volume <= 0:
            return 0.0
        r = self.taker_quote_volume / self.quote_volume
        return max(0.0, min(1.0, r))


class EMA:
    __slots__ = ("alpha", "value", "ready")

    def __init__(self, n: int):
        self.alpha = 2.0 / (n + 1)
        self.value = 0.0
        self.ready = False

    def update(self, x: float) -> float:
        if not self.ready:
            self.value = x
            self.ready = True
        else:
            self.value = self.alpha * x + (1.0 - self.alpha) * self.value
        return self.value


class ATR:
    __slots__ = ("n", "prev_close", "buf", "sum")

    def __init__(self, n: int):
        self.n = n
        self.prev_close: Optional[float] = None
        self.buf = []
        self.sum = 0.0

    def update(self, high: float, low: float, close: float) -> Optional[float]:
        if self.prev_close is None:
            tr = high - low
        else:
            tr = max(high - low, abs(high - self.prev_close), abs(low - self.prev_close))
        self.prev_close = close

        self.buf.append(tr)
        self.sum += tr
        if len(self.buf) > self.n:
            self.sum -= self.buf.pop(0)

        if len(self.buf) < self.n:
            return None
        return self.sum / len(self.buf)


class HFTMomentumStrategy:
    __slots__ = (
        "ema_fast",
        "ema_slow",
        "atr",
        "close_hist",
        "trade_count_hist",
        "last_close_time_ms",
        "pos",
        "entry_price",
        "entry_atr",
        "bars_in_pos",
        "cooldown",
        "signal_streak",
    )

    def __init__(self):
        self.ema_fast = EMA(FAST_EMA_N)
        self.ema_slow = EMA(SLOW_EMA_N)
        self.atr = ATR(ATR_N)

        self.close_hist = []
        self.trade_count_hist = []
        self.last_close_time_ms = 0

        self.pos = "FLAT"  # FLAT | LONG
        self.entry_price: Optional[float] = None
        self.entry_atr: Optional[float] = None
        self.bars_in_pos = 0
        self.cooldown = 0
        self.signal_streak = 0

    def on_closed_candle(self, c: Candle):
        if c.close_time_ms <= self.last_close_time_ms:
            return None
        self.last_close_time_ms = c.close_time_ms

        if self.cooldown > 0:
            self.cooldown -= 1

        ema_fast = self.ema_fast.update(c.close)
        ema_slow = self.ema_slow.update(c.close)
        atr = self.atr.update(c.high, c.low, c.close)

        self.close_hist.append(c.close)
        if len(self.close_hist) > max(RET_LOOKBACK + 5, 120):
            self.close_hist.pop(0)

        self.trade_count_hist.append(c.trade_count)
        if len(self.trade_count_hist) > TRADE_COUNT_WINDOW:
            self.trade_count_hist.pop(0)

        if atr is None or len(self.close_hist) <= RET_LOOKBACK:
            return None

        ret = (c.close - self.close_hist[-RET_LOOKBACK - 1]) / max(1e-12, self.close_hist[-RET_LOOKBACK - 1])
        ret_pct = ret * 100.0
        ema_spread_pct = (ema_fast - ema_slow) / max(1e-12, c.close) * 100.0
        atr_pct = atr / max(1e-12, c.close) * 100.0
        range_pct = (c.high - c.low) / max(1e-12, c.close) * 100.0
        trend_up = ema_fast > ema_slow and ema_spread_pct >= MIN_EMA_SPREAD_PCT
        vol_ok = atr_pct >= MIN_ATR_PCT and range_pct >= MIN_RANGE_PCT
        edge_ok = (TAKE_ATR * atr_pct) >= (ROUND_TRIP_COST_PCT * MIN_EXPECTED_EDGE_MULT)

        trades_ok = True
        if len(self.trade_count_hist) >= 30:
            thr = percentile(self.trade_count_hist, MIN_TRADE_COUNT_PCTL)
            trades_ok = c.trade_count >= thr

        raw_buy_signal = (
            trend_up
            and ret_pct >= RET_ENTRY_PCT
            and c.taker_ratio >= MIN_TAKER_RATIO
            and trades_ok
            and vol_ok
            and edge_ok
        )

        if raw_buy_signal:
            self.signal_streak += 1
        else:
            self.signal_streak = 0

        buy_signal = self.signal_streak >= max(1, ENTRY_CONFIRM_BARS)

        if self.pos == "LONG":
            self.bars_in_pos += 1
            stop_price = self.entry_price - STOP_ATR * self.entry_atr
            take_price = self.entry_price + TAKE_ATR * self.entry_atr
            trend_reversal = self.bars_in_pos >= MIN_HOLD_BARS and ema_fast < ema_slow
            exit_signal = (
                c.close <= stop_price
                or c.close >= take_price
                or trend_reversal
                or self.bars_in_pos >= MAX_HOLD_BARS
            )
            if exit_signal:
                if c.close <= stop_price:
                    reason = "STOP_LONG"
                elif c.close >= take_price:
                    reason = "TAKE_LONG"
                elif trend_reversal:
                    reason = "TREND_REVERSAL"
                else:
                    reason = "TIME_EXIT"
                self.pos = "FLAT"
                self.entry_price = None
                self.entry_atr = None
                self.bars_in_pos = 0
                self.cooldown = COOLDOWN_BARS
                self.signal_streak = 0
                return "SELL", c.close, reason

        if self.pos == "FLAT" and self.cooldown == 0 and buy_signal:
            self.pos = "LONG"
            self.entry_price = c.close
            self.entry_atr = atr
            self.bars_in_pos = 0
            self.signal_streak = 0
            return "BUY", c.close, "ENTER_LONG"

        if not LONG_ONLY:
            # Reserved for future short-side logic.
            return None

        return None


def gen_id() -> str:
    return uuid.uuid4().hex


def now_ms() -> int:
    return int(time.time() * 1000)


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
            "market_type": MARKET_TYPE,
            "position_side": POSITION_SIDE,
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

    strategy = HFTMomentumStrategy()

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
                high=float(data["HighPrice"]),
                low=float(data["LowPrice"]),
                quote_volume=float(data.get("QuoteVolume", "0") or "0"),
                taker_quote_volume=float(data.get("TakerQuoteVolume", "0") or "0"),
                trade_count=int(data.get("TradeCount", 0)),
            )

            signal = strategy.on_closed_candle(candle)
            if signal:
                side, px, reason = signal
                out = build_order_payload(side, px)
                await js.publish(ORDER_SUBJECT, orjson.dumps(out))
                print(
                    f"[HFT] {reason} side={side} symbol={ORDER_SYMBOL} close={px:.4f} "
                    f"taker={candle.taker_ratio:.2f} trades={candle.trade_count}",
                    flush=True,
                )

            await msg.ack()
        except Exception as exc:
            # Keep message unacked for redelivery when failures are transient.
            print(f"handler error: {exc}", flush=True)
            return

    await js.subscribe(
        KLINE_SUBJECT,
        manual_ack=True,
        queue=QUEUE_NAME,
        cb=handler,
    )

    print("HFT momentum strategy running...", flush=True)
    while True:
        if nc.is_closed:
            raise RuntimeError("NATS connection closed; strategy exiting for supervisor restart")
        await asyncio.sleep(1)


if __name__ == "__main__":
    asyncio.run(run())
