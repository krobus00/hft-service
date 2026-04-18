import asyncio
from typing import Optional

import uvloop

from core.common import load_full_config
from core.framework import StrategyBase, StrategyRunner
from core.indicators import EMA, MACD
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
MACD_CONFIG = CONFIG.get("macd", {})


class MACDStrategy(StrategyBase):
    __slots__ = (
        "macd",
        "trend_fast",
        "trend_slow",
        "macd_ready_min",
        "hist_band",
        "confirm_bars",
        "cooldown_bars",
        "buy_ok_count",
        "sell_ok_count",
        "cooldown",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        self.macd = MACD(
            int(section.get("macd_fast", 12)),
            int(section.get("macd_slow", 26)),
            int(section.get("macd_signal", 9)),
        )
        self.trend_fast = EMA(int(section.get("trend_ema_fast", 50)))
        self.trend_slow = EMA(int(section.get("trend_ema_slow", 200)))
        macd_slow = int(section.get("macd_slow", 26))
        macd_signal = int(section.get("macd_signal", 9))
        self.macd_ready_min = max(50, macd_slow + macd_signal)

        self.hist_band = float(section.get("hist_band", 0.05))
        self.confirm_bars = int(section.get("confirm_bars", 2))
        self.cooldown_bars = int(section.get("cooldown_bars", 3))

        self.buy_ok_count = 0
        self.sell_ok_count = 0
        self.cooldown = 0

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        if self.cooldown > 0:
            self.cooldown -= 1

        trend_fast = self.trend_fast.update(candle.close)
        trend_slow = self.trend_slow.update(candle.close)

        macd_line, signal_line, crossed_up, crossed_down = self.macd.update(candle.close)
        hist = macd_line - signal_line

        if self.macd.count < self.macd_ready_min:
            return None

        trend_up = trend_fast > trend_slow
        trend_down = trend_fast < trend_slow

        buy_candidate = hist >= self.hist_band
        sell_candidate = hist <= -self.hist_band

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

        if is_warmup:
            return None

        metadata = {
            "macd": round(macd_line, 8),
            "signal": round(signal_line, 8),
            "hist": round(hist, 8),
            "trend_fast": round(trend_fast, 8),
            "trend_slow": round(trend_slow, 8),
        }

        if self.cooldown == 0 and crossed_up and self.buy_ok_count >= self.confirm_bars:
            self.cooldown = self.cooldown_bars
            self.buy_ok_count = 0
            return self.buy(candle.close, "ENTER_LONG", metadata)

        if self.cooldown == 0 and crossed_down and self.sell_ok_count >= self.confirm_bars:
            self.cooldown = self.cooldown_bars
            self.sell_ok_count = 0
            return self.sell(candle.close, "ENTER_SHORT", metadata)

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
        queue_name=section.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_MACD"),
        user_id=section.get("user_id", "paper-1"),
        exchange=section.get("exchange", "tokocrypto"),
        market_type=section.get("market_type", "spot"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-macd"),
        strategy_id=section.get("strategy_id", "python-macd"),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        order_symbol=section.get("order_symbol", section.get("symbol", "SOLUSDT")),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name="MACD",
        symbol=section.get("symbol", "SOLUSDT"),
        interval=section.get("interval", "1m"),
        warmup_limit=int(section.get("historical_limit", 200)),
    )


async def run():
    runtime = build_runtime_config(MACD_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = MACDStrategy(build_strategy_config(MACD_CONFIG), MACD_CONFIG)
    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
