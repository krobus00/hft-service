import asyncio

import uvloop

from core.common import cfg_value, load_full_config
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
        self.cooldown = int(cfg_value(section, GLOBAL_CONFIG, "cooldown_bars", 0))

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        if self.cooldown > 0:
            self.cooldown -= 1
            return None

        # Update reusable indicators here during both warmup and live data.
        if is_warmup:
            return None

        # Implement real entry/exit rules here.
        # return self.buy(candle.close, "ENTER_LONG", {"example": 1})
        # self.cooldown = int(cfg_value(SECTION, GLOBAL_CONFIG, "cooldown_bars", 0))
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
        order_subject=section.get("order_subject", "order_engine.place_order"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-template"),
        strategy_id=section.get("strategy_id", "python-template"),
        need_notification=bool(section.get("need_notification", False)),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
        enable_intrabar_risk_exit=bool(cfg_value(section, GLOBAL_CONFIG, "enable_intrabar_risk_exit", False)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    # Symbol/interval routing is driven by kline_subscriptions in StrategyRunner.
    # Keep placeholders only to satisfy StrategyConfig shape.
    return StrategyConfig(
        name=section.get("name", "TEMPLATE"),
        symbol="*",
        interval="*",
        warmup_limit=int(section.get("historical_limit", 500)),
    )


async def run():
    runtime = build_runtime_config(SECTION)
    strategy = TemplateStrategy(build_strategy_config(SECTION), SECTION)
    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
