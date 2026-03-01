import asyncio
import time
import uuid
from pathlib import Path

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

KLINE_SUBJECT = EXAMPLE_CONFIG.get("kline_subject", "KLINE.TOKOCRYPTO.>")
ORDER_SUBJECT = EXAMPLE_CONFIG.get("order_subject", "order_engine.place_order")
QUEUE_NAME = EXAMPLE_CONFIG.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_EXAMPLE")

SYMBOL = EXAMPLE_CONFIG.get("symbol", "SOLUSDT")
INTERVAL = EXAMPLE_CONFIG.get("interval", "1m")

USER_ID = EXAMPLE_CONFIG.get("user_id", "paper-1")
EXCHANGE = EXAMPLE_CONFIG.get("exchange", "tokocrypto")
SOURCE = EXAMPLE_CONFIG.get("source", "python-example")
STRATEGY_ID = EXAMPLE_CONFIG.get("strategy_id", "python-example")
IS_PAPER_TRADING = EXAMPLE_CONFIG.get("is_paper_trading", True)

ORDER_TYPE = EXAMPLE_CONFIG.get("order_type", "LIMIT")
ORDER_QTY = EXAMPLE_CONFIG.get("order_qty", 10)
ORDER_SYMBOL = EXAMPLE_CONFIG.get("order_symbol", "SOLUSDT")


def gen_id() -> str:
    return uuid.uuid4().hex


def now_ms() -> int:
    return int(time.time() * 1000)


def build_order_payload(side: str, price: float) -> dict:
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
            "price": str(price),
            "quantity": str(ORDER_QTY),
            "requested_at": now_ms(),
            "expired_at": None,
            "source": SOURCE,
            "strategy_id": STRATEGY_ID,
            "is_paper_trading": IS_PAPER_TRADING,
        },
    }


def process_closed_candle(data: dict):
    close_price = float(data["ClosePrice"])
    return None, close_price


async def run():
    nc = NATS()
    await nc.connect(NATS_URL)
    js = nc.jetstream()

    async def handler(msg):
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

        side, close_price = process_closed_candle(data)

        if side in ("BUY", "SELL"):
            out = build_order_payload(side, close_price)
            await js.publish(ORDER_SUBJECT, orjson.dumps(out))
            print(f"published order side={side} symbol={ORDER_SYMBOL} close={close_price}")

        await msg.ack()

    await js.subscribe(
        KLINE_SUBJECT,
        manual_ack=True,
        queue=QUEUE_NAME,
        cb=handler,
    )

    print("Example strategy running...")
    while True:
        await asyncio.sleep(1)


if __name__ == "__main__":
    asyncio.run(run())
