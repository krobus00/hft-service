import asyncio
import uvloop
import asyncpg
import orjson
import uuid
import time
from pathlib import Path
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
MACD_CONFIG = CONFIG.get("macd", {})

DB_DSN = GLOBAL_CONFIG.get("db_dsn", "postgres://root:root@localhost:5432/market_data?sslmode=disable")
NATS_URL = GLOBAL_CONFIG.get("nats_url", "nats://localhost:4222")

SYMBOL = MACD_CONFIG.get("symbol", "SOLUSDT")
INTERVAL = MACD_CONFIG.get("interval", "1m")

HISTORICAL_LIMIT = MACD_CONFIG.get("historical_limit", 200)

# =========================
# NOISE FILTER CONFIG
# =========================
HIST_BAND = MACD_CONFIG.get("hist_band", 0.05)
CONFIRM_BARS = MACD_CONFIG.get("confirm_bars", 2)
COOLDOWN_BARS = MACD_CONFIG.get("cooldown_bars", 3)

TREND_EMA_FAST = MACD_CONFIG.get("trend_ema_fast", 50)
TREND_EMA_SLOW = MACD_CONFIG.get("trend_ema_slow", 200)

EXCHANGE = MACD_CONFIG.get("exchange", "tokocrypto")
USER_ID = MACD_CONFIG.get("user_id", "paper-1")
SOURCE = MACD_CONFIG.get("source", "python-macd")
STRATEGY_ID = MACD_CONFIG.get("strategy_id", "python-macd")
IS_PAPER_TRADING = MACD_CONFIG.get("is_paper_trading", True)
ORDER_SUBJECT = MACD_CONFIG.get("order_subject", "order_engine.place_order")
KLINE_SUBJECT = MACD_CONFIG.get("kline_subject", "KLINE.TOKOCRYPTO.>")
QUEUE_NAME = MACD_CONFIG.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_MACD")
ORDER_TYPE = MACD_CONFIG.get("order_type", "LIMIT")
ORDER_QTY = MACD_CONFIG.get("order_qty", 10)
ORDER_SYMBOL = MACD_CONFIG.get("order_symbol", "SOLUSDT")


def gen_request_id() -> str:
    return uuid.uuid4().hex


def gen_order_id() -> str:
    return f"paper-{uuid.uuid4().hex}"


def now_ms() -> int:
    return int(time.time() * 1000)


# =========================
# INDICATORS (HFT SAFE)
# =========================
class FastEMA:
    __slots__ = ("alpha", "value", "initialized")

    def __init__(self, period: int):
        self.alpha = 2.0 / (period + 1)
        self.value = 0.0
        self.initialized = False

    def update(self, price: float):
        if not self.initialized:
            self.value = price
            self.initialized = True
        else:
            self.value = self.alpha * price + (1 - self.alpha) * self.value
        return self.value


class FastMACD:
    __slots__ = ("ema_fast", "ema_slow", "ema_signal", "ready_count")

    def __init__(self):
        self.ema_fast = FastEMA(12)
        self.ema_slow = FastEMA(26)
        self.ema_signal = FastEMA(9)
        self.ready_count = 0

    def update(self, price: float):
        fast = self.ema_fast.update(price)
        slow = self.ema_slow.update(price)

        macd = fast - slow
        signal = self.ema_signal.update(macd)
        histogram = macd - signal

        self.ready_count += 1
        if self.ready_count < 50:
            return None

        return macd, signal, histogram


# =========================
# ENGINE + NOISE FILTER
# =========================
class HFTEngine:
    __slots__ = (
        "macd",
        "prev_hist",
        "buy_ok_count",
        "sell_ok_count",
        "cooldown",
        "trend_fast",
        "trend_slow",
    )

    def __init__(self):
        self.macd = FastMACD()
        self.prev_hist = None

        self.buy_ok_count = 0
        self.sell_ok_count = 0
        self.cooldown = 0

        self.trend_fast = FastEMA(TREND_EMA_FAST)
        self.trend_slow = FastEMA(TREND_EMA_SLOW)

    def process_price(self, price: float):
        tf = self.trend_fast.update(price)
        ts = self.trend_slow.update(price)

        if self.cooldown > 0:
            self.cooldown -= 1

        result = self.macd.update(price)
        if result is None:
            return None

        macd, signal, hist = result

        if self.prev_hist is None:
            self.prev_hist = hist
            return None

        crossed_up = self.prev_hist <= 0 and hist > 0
        crossed_down = self.prev_hist >= 0 and hist < 0
        self.prev_hist = hist

        buy_candidate = hist >= HIST_BAND
        sell_candidate = hist <= -HIST_BAND

        trend_up = tf > ts
        trend_down = tf < ts

        if buy_candidate and trend_up:
            self.buy_ok_count += 1
        else:
            self.buy_ok_count = 0

        if sell_candidate and trend_down:
            self.sell_ok_count += 1
        else:
            self.sell_ok_count = 0

        if crossed_up:
            self.sell_ok_count = 0
        if crossed_down:
            self.buy_ok_count = 0

        if self.cooldown == 0 and crossed_up and self.buy_ok_count >= CONFIRM_BARS:
            self.cooldown = COOLDOWN_BARS
            self.buy_ok_count = 0
            return "BUY", price

        if self.cooldown == 0 and crossed_down and self.sell_ok_count >= CONFIRM_BARS:
            self.cooldown = COOLDOWN_BARS
            self.sell_ok_count = 0
            return "SELL", price

        return None


# =========================
# LOAD HISTORICAL DATA
# =========================
async def warmup_from_postgres(engine: HFTEngine):
    conn = await asyncpg.connect(DB_DSN)

    rows = await conn.fetch(
        """
        SELECT close_price as close
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

    await conn.close()

    rows = list(reversed(rows))
    for row in rows:
        engine.process_price(float(row["close"]))

    print(f"Warmup completed with {len(rows)} candles")


# =========================
# LIVE STREAM
# =========================
async def run():
    engine = HFTEngine()
    await warmup_from_postgres(engine)

    nc = NATS()
    await nc.connect(NATS_URL)
    js = nc.jetstream()

    async def handler(msg):
        payload = orjson.loads(msg.data)
        d = payload["data"]

        # New convention (your kline payload is PascalCase)
        # - IsClosed: bool
        # - Symbol: str
        # - ClosePrice: str
        if not d.get("IsClosed"):
            await msg.ack()
            return

        if d.get("Symbol") != SYMBOL:
            await msg.ack()
            return

        if d.get("Interval") != INTERVAL:
            await msg.ack()
            return
        price = float(d["ClosePrice"])
        signal = engine.process_price(price)

        if signal:
            side, px = signal

            # Keep order payload in snake_case to match your "new convention" used in strategies
            out = {
                "retry": 0,
                "data": {
                    "request_id": gen_request_id(),
                    "user_id": USER_ID,
                    "order_id": gen_order_id(),
                    "exchange": EXCHANGE,
                    "symbol": ORDER_SYMBOL,
                    "type": ORDER_TYPE,
                    "side": side,
                    "price": str(px),
                    "quantity": str(ORDER_QTY),
                    "requested_at": now_ms(),
                    "expired_at": None,
                    "source": SOURCE,
                    "strategy_id": STRATEGY_ID,
                    "is_paper_trading": IS_PAPER_TRADING,
                },
            }

            print("order payload:", out)

            await js.publish(ORDER_SUBJECT, orjson.dumps(out))

        await msg.ack()

    await js.subscribe(
        KLINE_SUBJECT,
        manual_ack=True,
        queue=QUEUE_NAME,
        cb=handler,
    )

    print("Live analyzer running...")
    while True:
        await asyncio.sleep(1)


if __name__ == "__main__":
    asyncio.run(run())
