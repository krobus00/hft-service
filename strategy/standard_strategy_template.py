import asyncio

import uvloop

from core.common import load_full_config
from core.framework import StrategyBase, StrategyRunner
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
SECTION = CONFIG.get("template", {})


class TemplateStrategy(StrategyBase):
    __slots__ = ("cooldown",)

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)
        self.cooldown = int(section.get("cooldown_bars", 0))

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        # Update reusable indicators here during both warmup and live data.
        if is_warmup:
            return None

        # Implement real entry/exit rules here.
        # return self.buy(candle.close, "ENTER_LONG", {"example": 1})
        # return self.sell(candle.close, "EXIT_LONG", {"example": 1})
        return None


def build_runtime_config(section: dict) -> RuntimeConfig:
    return RuntimeConfig(
        db_dsn=GLOBAL_CONFIG.get("db_dsn", "postgres://root:root@localhost:5432/market_data?sslmode=disable"),
        nats_url=GLOBAL_CONFIG.get("nats_url", "nats://localhost:4222"),
        nats_allow_reconnect=GLOBAL_CONFIG.get("nats_allow_reconnect", True),
        nats_max_reconnect_attempts=GLOBAL_CONFIG.get("nats_max_reconnect_attempts", -1),
        nats_reconnect_time_wait_sec=GLOBAL_CONFIG.get("nats_reconnect_time_wait_sec", 2),
        nats_connect_timeout_sec=GLOBAL_CONFIG.get("nats_connect_timeout_sec", 5),
        nats_ping_interval_sec=GLOBAL_CONFIG.get("nats_ping_interval_sec", 30),
        nats_max_outstanding_pings=GLOBAL_CONFIG.get("nats_max_outstanding_pings", 3),
        kline_subject=section.get("kline_subject", "KLINE.TOKOCRYPTO.>"),
        order_subject=section.get("order_subject", "order_engine.place_order"),
        queue_name=section.get("queue_name", "KLINE_STRATEGY_TEMPLATE"),
        user_id=section.get("user_id", "paper-1"),
        exchange=section.get("exchange", "tokocrypto"),
        market_type=section.get("market_type", "spot"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-template"),
        strategy_id=section.get("strategy_id", "python-template"),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        order_symbol=section.get("order_symbol", section.get("symbol", "SOLUSDT")),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "TEMPLATE"),
        symbol=section.get("symbol", "SOLUSDT"),
        interval=section.get("interval", "1m"),
        warmup_limit=int(section.get("historical_limit", 500)),
    )


async def run():
    runtime = build_runtime_config(SECTION)
    strategy = TemplateStrategy(build_strategy_config(SECTION), SECTION)
    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
