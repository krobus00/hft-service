import asyncio
import uvloop
import orjson
import uuid
import time
import math
from pathlib import Path
from dataclasses import dataclass
from datetime import datetime
from typing import Optional, Dict, Any, Tuple
import yaml
from nats.aio.client import Client as NATS

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

# =========================
# CONFIG
# =========================
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
AVWAP_CONFIG = CONFIG.get("anchored_vwap", {})

NATS_URL = GLOBAL_CONFIG.get("nats_url", "nats://localhost:4222")

KLINE_SUBJECT = AVWAP_CONFIG.get("kline_subject", "KLINE.TOKOCRYPTO.>")
PLACE_SUBJECT = AVWAP_CONFIG.get("place_subject", "order_engine.place_order")

EXCHANGE = AVWAP_CONFIG.get("exchange", "tokocrypto")
SYMBOL_IN = AVWAP_CONFIG.get("symbol_in", "SOLUSDT")
SYMBOL_OUT = AVWAP_CONFIG.get("symbol_out", "SOLUSDT")

USER_ID = AVWAP_CONFIG.get("user_id", "paper-1")
SOURCE = AVWAP_CONFIG.get("source", "python-avwap")
STRATEGY_ID = AVWAP_CONFIG.get("strategy_id", "python-avwap-improved-longonly-v1")
IS_PAPER_TRADING = AVWAP_CONFIG.get("is_paper_trading", True)

# LONG-ONLY spot behavior
LONG_ONLY = AVWAP_CONFIG.get("long_only", True)

# Execution
ORDER_TYPE = AVWAP_CONFIG.get("order_type", "LIMIT")         # "LIMIT" or "MARKET"
ORDER_QTY = AVWAP_CONFIG.get("order_qty", 10.0)
LIMIT_SLIPPAGE_BPS = AVWAP_CONFIG.get("limit_slippage_bps", 3)       # BUY: price*(1+slip), SELL: price*(1-slip)

# =========================
# SIGNAL PARAMS (tune)
# =========================
# ATR bands
ATR_N = AVWAP_CONFIG.get("atr_n", 14)
ATR_K = AVWAP_CONFIG.get("atr_k", 1.2)
MIN_BAND_PCT = AVWAP_CONFIG.get("min_band_pct", 0.0008)       # 0.08%

# AVWAP slope filter (avoid chop)
SLOPE_LOOKBACK = AVWAP_CONFIG.get("slope_lookback", 20)
SLOPE_MIN_BPS = AVWAP_CONFIG.get("slope_min_bps", 8)

# Volume shock anchor reset
VOL_MED_N = AVWAP_CONFIG.get("vol_med_n", 60)
VOL_SHOCK_X = AVWAP_CONFIG.get("vol_shock_x", 2.5)
RANGE_MIN_BPS = AVWAP_CONFIG.get("range_min_bps", 15)
ANCHOR_RESET_COOLDOWN_MS = AVWAP_CONFIG.get("anchor_reset_cooldown_ms", 10 * 60_000)

# Participation filters
PARTICIPATION_N = AVWAP_CONFIG.get("participation_n", 120)
MIN_TRADES_PCTL = AVWAP_CONFIG.get("min_trades_pctl", 0.30)

# Taker flow filter (long entries)
TAKER_RATIO_LONG_MIN = AVWAP_CONFIG.get("taker_ratio_long_min", 0.52)

# Signal smoothing
CONFIRM_BARS = AVWAP_CONFIG.get("confirm_bars", 2)
COOLDOWN_BARS = AVWAP_CONFIG.get("cooldown_bars", 3)

QUEUE_NAME = AVWAP_CONFIG.get(
    "queue_name", "KLINE_STRATEGY_TOKOCRYPTO_AVWAP_IMPROVED_LONGONLY"
)


# =========================
# HELPERS
# =========================
def now_ms() -> int:
    return int(time.time() * 1000)

def gen_request_id() -> str:
    return uuid.uuid4().hex

def bps_to_frac(bps: float) -> float:
    return bps / 10_000.0

def fmt_num(x: float) -> str:
    return f"{x:.8f}".rstrip("0").rstrip(".")

def parse_iso_to_ms(s: str) -> int:
    dt = datetime.fromisoformat(s.replace("Z", "+00:00"))
    return int(dt.timestamp() * 1000)

def median(xs):
    if not xs:
        return 0.0
    ys = sorted(xs)
    n = len(ys)
    mid = n // 2
    if n % 2:
        return float(ys[mid])
    return (ys[mid - 1] + ys[mid]) / 2.0

def percentile(xs, p: float) -> float:
    if not xs:
        return 0.0
    ys = sorted(xs)
    k = int(round((len(ys) - 1) * p))
    return float(ys[max(0, min(len(ys) - 1, k))])


@dataclass
class Candle:
    open_time_ms: int
    close_time_ms: int
    o: float
    h: float
    l: float
    c: float
    quote_volume: float
    taker_quote_volume: float
    trade_count: int

    @property
    def typical(self) -> float:
        return (self.h + self.l + self.c) / 3.0

    @property
    def range_bps(self) -> float:
        if self.c <= 0:
            return 0.0
        return (self.h - self.l) / self.c * 10_000.0

    @property
    def taker_ratio(self) -> float:
        if self.quote_volume <= 0:
            return 0.0
        r = self.taker_quote_volume / self.quote_volume
        return max(0.0, min(1.0, r))


class ATR:
    __slots__ = ("n", "prev_close", "buf", "sum")

    def __init__(self, n: int):
        self.n = n
        self.prev_close: Optional[float] = None
        self.buf = []
        self.sum = 0.0

    def update(self, h: float, l: float, c: float) -> Optional[float]:
        if self.prev_close is None:
            tr = h - l
        else:
            tr = max(h - l, abs(h - self.prev_close), abs(l - self.prev_close))
        self.prev_close = c

        self.buf.append(tr)
        self.sum += tr
        if len(self.buf) > self.n:
            old = self.buf.pop(0)
            self.sum -= old

        if len(self.buf) < self.n:
            return None
        return self.sum / len(self.buf)


class AnchoredVWAP:
    __slots__ = ("anchor_ms", "cum_pv", "cum_v", "avwap_hist")

    def __init__(self):
        self.anchor_ms: Optional[int] = None
        self.cum_pv = 0.0
        self.cum_v = 0.0
        self.avwap_hist = []

    def reset(self, anchor_ms: int):
        self.anchor_ms = anchor_ms
        self.cum_pv = 0.0
        self.cum_v = 0.0
        self.avwap_hist.clear()

    def update(self, candle: Candle) -> Optional[float]:
        if self.anchor_ms is None:
            self.reset(candle.close_time_ms)

        v = candle.quote_volume
        if v <= 0:
            return None

        self.cum_pv += candle.typical * v
        self.cum_v += v
        if self.cum_v <= 0:
            return None

        avwap = self.cum_pv / self.cum_v
        self.avwap_hist.append(avwap)
        if len(self.avwap_hist) > max(200, SLOPE_LOOKBACK + 5):
            self.avwap_hist.pop(0)
        return avwap

    def slope_bps(self) -> Optional[float]:
        if len(self.avwap_hist) <= SLOPE_LOOKBACK:
            return None
        a0 = self.avwap_hist[-SLOPE_LOOKBACK - 1]
        a1 = self.avwap_hist[-1]
        if a0 <= 0:
            return None
        return (a1 - a0) / a0 * 10_000.0


class Strategy:
    __slots__ = (
        "vwap",
        "atr",
        "prev_close",
        "pos",
        "cooldown",
        "buy_ok",
        "vol_buf",
        "trades_buf",
        "last_anchor_reset_ms",
    )

    def __init__(self):
        self.vwap = AnchoredVWAP()
        self.atr = ATR(ATR_N)
        self.prev_close: Optional[float] = None

        self.pos = "FLAT"  # FLAT | LONG
        self.cooldown = 0
        self.buy_ok = 0

        self.vol_buf = []
        self.trades_buf = []
        self.last_anchor_reset_ms = 0

    def update_hist(self, candle: Candle):
        self.vol_buf.append(candle.quote_volume)
        if len(self.vol_buf) > VOL_MED_N:
            self.vol_buf.pop(0)

        self.trades_buf.append(candle.trade_count)
        if len(self.trades_buf) > PARTICIPATION_N:
            self.trades_buf.pop(0)

    def should_reset_anchor(self, candle: Candle) -> bool:
        if candle.close_time_ms - self.last_anchor_reset_ms < ANCHOR_RESET_COOLDOWN_MS:
            return False
        med_vol = median(self.vol_buf)
        if med_vol <= 0:
            return False
        return (candle.quote_volume >= med_vol * VOL_SHOCK_X) and (candle.range_bps >= RANGE_MIN_BPS)

    def participation_ok(self, candle: Candle) -> bool:
        if len(self.trades_buf) < 30:
            return True
        thr = percentile(self.trades_buf, MIN_TRADES_PCTL)
        return candle.trade_count >= thr

    def on_closed_candle(self, candle: Candle):
        if self.cooldown > 0:
            self.cooldown -= 1

        self.update_hist(candle)

        if self.should_reset_anchor(candle):
            self.vwap.reset(candle.close_time_ms)
            self.last_anchor_reset_ms = candle.close_time_ms

        avwap = self.vwap.update(candle)
        atr = self.atr.update(candle.h, candle.l, candle.c)

        if avwap is None or atr is None:
            self.prev_close = candle.c
            return None

        slope = self.vwap.slope_bps()
        if slope is None or abs(slope) < SLOPE_MIN_BPS:
            self.prev_close = candle.c
            return None

        if not self.participation_ok(candle):
            self.prev_close = candle.c
            return None

        band = max(ATR_K * atr, MIN_BAND_PCT * avwap)
        upper = avwap + band

        close = candle.c
        if self.prev_close is None:
            self.prev_close = close
            return None

        crossed_up = self.prev_close <= avwap and close > avwap
        crossed_down = self.prev_close >= avwap and close < avwap

        # Exit LONG when cross down through AVWAP
        if self.pos == "LONG" and crossed_down:
            self.pos = "FLAT"
            self.cooldown = COOLDOWN_BARS
            self.prev_close = close
            return ("SELL", close, avwap, upper, slope, candle.taker_ratio, "EXIT_LONG")

        # Enter LONG: cross up + close above upper band + taker filter + confirm
        buy_candidate = (close >= upper) and (candle.taker_ratio >= TAKER_RATIO_LONG_MIN)
        self.buy_ok = self.buy_ok + 1 if buy_candidate else 0

        if self.cooldown == 0 and self.pos == "FLAT":
            if crossed_up and self.buy_ok >= CONFIRM_BARS:
                self.pos = "LONG"
                self.cooldown = COOLDOWN_BARS
                self.buy_ok = 0
                self.prev_close = close
                return ("BUY", close, avwap, upper, slope, candle.taker_ratio, "ENTER_LONG")

        self.prev_close = close
        return None


def make_order_payload(side: str, ref_px: float, qty: float) -> Dict[str, Any]:
    px = ref_px
    if ORDER_TYPE == "LIMIT":
        if side == "BUY":
            px = px * (1.0 + bps_to_frac(LIMIT_SLIPPAGE_BPS))
        else:
            px = px * (1.0 - bps_to_frac(LIMIT_SLIPPAGE_BPS))

    # NOTE: keys are snake_case here to match your MACD script.
    return {
        "retry": 0,
        "data": {
            "request_id": gen_request_id(),
            "user_id": USER_ID,
            "order_id": gen_request_id(),
            "exchange": EXCHANGE,
            "symbol": SYMBOL_OUT,
            "type": ORDER_TYPE,
            "side": side,
            "price": fmt_num(px),
            "quantity": fmt_num(qty),
            "requested_at": now_ms(),
            "expired_at": None,
            "source": SOURCE,
            "strategy_id": STRATEGY_ID,
            "is_paper_trading": IS_PAPER_TRADING,
        },
    }


async def run():
    strat = Strategy()

    nc = NATS()
    await nc.connect(NATS_URL)
    js = nc.jetstream()

    async def handler(msg):
        payload = orjson.loads(msg.data)
        d = payload["data"]

        if d.get("Symbol") != SYMBOL_IN:
            await msg.ack()
            return
        if d.get("Interval") != "1m":
            await msg.ack()
            return
        if not d.get("IsClosed"):
            await msg.ack()
            return

        candle = Candle(
            open_time_ms=parse_iso_to_ms(d["OpenTime"]),
            close_time_ms=parse_iso_to_ms(d["CloseTime"]),
            o=float(d["OpenPrice"]),
            h=float(d["HighPrice"]),
            l=float(d["LowPrice"]),
            c=float(d["ClosePrice"]),
            quote_volume=float(d.get("QuoteVolume", "0") or "0"),
            taker_quote_volume=float(d.get("TakerQuoteVolume", "0") or "0"),
            trade_count=int(d.get("TradeCount", 0)),
        )

        sig = strat.on_closed_candle(candle)
        if sig:
            side, close, avwap, upper, slope_bps, taker_ratio, reason = sig
            out = make_order_payload(side, close, ORDER_QTY)

            print(
                f"[AVWAP+ LO] {reason} side={side} close={close:.4f} avwap={avwap:.4f} "
                f"upper={upper:.4f} slope={slope_bps:.1f}bps taker={taker_ratio:.2f} "
                f"trades={candle.trade_count} qv={candle.quote_volume:.2f}",
                flush=True,
            )

            await js.publish(PLACE_SUBJECT, orjson.dumps(out))

        await msg.ack()

    await js.subscribe(
        KLINE_SUBJECT,
        manual_ack=True,
        queue=QUEUE_NAME,
        cb=handler,
    )

    print("Anchored VWAP improved (long-only) running...", flush=True)
    while True:
        await asyncio.sleep(1)


if __name__ == "__main__":
    asyncio.run(run())
