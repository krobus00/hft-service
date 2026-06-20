import asyncio

import uvloop

from core.common import cfg_value, load_full_config, pct_to_frac, runtime_options
from core.framework import StrategyBase, StrategyRunner
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
MICRO_GRID_CONFIG = CONFIG.get("micro_grid", {})


class MicroGridStrategy(StrategyBase):
    __slots__ = ("anchor_price", "grid_step_pct", "cooldown", "cooldown_bars")

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)
        self.anchor_price = None
        self.grid_step_pct = float(section.get("grid_step_pct", 0.10))
        self.cooldown = 0
        self.cooldown_bars = int(cfg_value(section, GLOBAL_CONFIG, "cooldown_bars", 0))

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        # ponytail: one order per closed candle; use order-book events if sub-candle execution matters.
        if not self.allow_new_candle(candle):
            return None

        if is_warmup or self.anchor_price is None:
            self.anchor_price = candle.close
            return None

        if self.cooldown > 0:
            self.cooldown -= 1
            return None

        step = pct_to_frac(self.grid_step_pct)
        lower = self.anchor_price * (1.0 - step)
        upper = self.anchor_price * (1.0 + step)
        metadata = {
            "anchor_price": round(self.anchor_price, 8),
            "grid_step_pct": self.grid_step_pct,
            "lower_grid": round(lower, 8),
            "upper_grid": round(upper, 8),
        }

        if candle.close <= lower:
            self.anchor_price = candle.close
            self.cooldown = self.cooldown_bars
            return self.buy(candle.close, "GRID_BUY", metadata)

        if candle.close >= upper:
            self.anchor_price = candle.close
            self.cooldown = self.cooldown_bars
            return self.sell(candle.close, "GRID_SELL", metadata)

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
        source="python-micro-grid",
        strategy_id="python-micro-grid",
        **runtime_options(GLOBAL_CONFIG, section),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "MICRO_GRID"),
        symbol="*",
        interval="*",
        warmup_limit=int(section.get("historical_limit", 100)),
    )


async def run():
    strategy = MicroGridStrategy(build_strategy_config(MICRO_GRID_CONFIG), MICRO_GRID_CONFIG)
    if strategy.grid_step_pct <= 0 or strategy.grid_step_pct >= 100:
        raise ValueError("grid_step_pct must be between 0 and 100")
    if strategy.cooldown_bars < 0:
        raise ValueError("cooldown_bars must be >= 0")
    await StrategyRunner(strategy=strategy, runtime=build_runtime_config(MICRO_GRID_CONFIG)).run()


if __name__ == "__main__":
    asyncio.run(run())
