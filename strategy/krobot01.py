import asyncio
from typing import Optional

import uvloop

from core.common import load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import EMA, MACD, RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
KROBOT01_CONFIG = CONFIG.get("krobot01", {})


class Krobot01Strategy(StrategyBase):
    __slots__ = (
        "ema",
        "vwap",
        "macd",
        "cooldown",
        "position_side",
        "entry_price",
        "ema_period",
        "macd_ready_min",
        "cooldown_bars",
        "take_profit_pct",
        "stop_loss_pct",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        ema_period = int(section.get("ema_period", 200))
        vwap_window = int(section.get("vwap_window", 200))
        macd_fast = int(section.get("macd_fast", 12))
        macd_slow = int(section.get("macd_slow", 26))
        macd_signal = int(section.get("macd_signal", 9))

        self.ema = EMA(ema_period)
        self.vwap = RollingVWAP(vwap_window)
        self.macd = MACD(macd_fast, macd_slow, macd_signal)

        self.cooldown = 0
        self.position_side: Optional[str] = None
        self.entry_price: Optional[float] = None

        self.ema_period = ema_period
        self.macd_ready_min = max(50, macd_slow + macd_signal)
        self.cooldown_bars = int(section.get("cooldown_bars", 1))
        self.take_profit_pct = float(section.get("take_profit_pct", section.get("tp_pct", 0.0)))
        self.stop_loss_pct = float(section.get("stop_loss_pct", section.get("sl_pct", 0.0)))

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        if self.cooldown > 0:
            self.cooldown -= 1

        ema = self.ema.update(candle.close)
        vwap_px = self.vwap.update(candle.close, candle.quote_volume)
        _, _, crossed_up, crossed_down = self.macd.update(candle.close)

        if self.ema.count < self.ema_period or vwap_px is None or self.macd.count < self.macd_ready_min:
            return None

        if is_warmup:
            return None

        metadata = {
            "ema": round(ema, 8),
            "vwap": round(vwap_px, 8),
        }

        if self.position_side == "LONG" and self.entry_price is not None:
            if self.stop_loss_pct > 0 and candle.close <= self.entry_price * (1.0 - pct_to_frac(self.stop_loss_pct)):
                self.position_side = None
                self.entry_price = None
                self.cooldown = self.cooldown_bars
                return self.sell(candle.close, "STOP_LOSS_LONG", metadata)

            if self.take_profit_pct > 0 and candle.close >= self.entry_price * (1.0 + pct_to_frac(self.take_profit_pct)):
                self.position_side = None
                self.entry_price = None
                self.cooldown = self.cooldown_bars
                return self.sell(candle.close, "TAKE_PROFIT_LONG", metadata)

        if self.position_side == "SHORT" and self.entry_price is not None:
            if self.stop_loss_pct > 0 and candle.close >= self.entry_price * (1.0 + pct_to_frac(self.stop_loss_pct)):
                self.position_side = None
                self.entry_price = None
                self.cooldown = self.cooldown_bars
                return self.buy(candle.close, "STOP_LOSS_SHORT", metadata)

            if self.take_profit_pct > 0 and candle.close <= self.entry_price * (1.0 - pct_to_frac(self.take_profit_pct)):
                self.position_side = None
                self.entry_price = None
                self.cooldown = self.cooldown_bars
                return self.buy(candle.close, "TAKE_PROFIT_SHORT", metadata)

        if self.cooldown > 0:
            return None

        long_cond = candle.close > ema and candle.close > vwap_px and crossed_up
        short_cond = candle.close < ema and candle.close < vwap_px and crossed_down

        print(
            f"close={candle.close:.6f} ema={ema:.6f} vwap={vwap_px:.6f} crossed_up={crossed_up} crossed_down={crossed_down} long_cond={long_cond} short_cond={short_cond}",
            flush=True,
        )

        if long_cond:
            if self.position_side == "LONG":
                return None
            self.position_side = "LONG"
            self.entry_price = candle.close
            self.cooldown = self.cooldown_bars
            return self.buy(candle.close, "ENTER_LONG", metadata)

        if short_cond:
            if self.position_side == "SHORT":
                return None
            self.position_side = "SHORT"
            self.entry_price = candle.close
            self.cooldown = self.cooldown_bars
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
        queue_name=section.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_KROBOT01"),
        user_id=section.get("user_id", "paper-1"),
        exchange=section.get("exchange", "tokocrypto"),
        market_type=section.get("market_type", "spot"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-krobot01"),
        strategy_id=section.get("strategy_id", "python-krobot01-ema-vwap-macd"),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        order_symbol=section.get("order_symbol", section.get("symbol", "SOLUSDT")),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name="KROBOT01",
        symbol=section.get("symbol", "SOLUSDT"),
        interval=section.get("interval", "1m"),
        warmup_limit=int(section.get("historical_limit", 800)),
    )


async def run():
    runtime = build_runtime_config(KROBOT01_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = Krobot01Strategy(build_strategy_config(KROBOT01_CONFIG), KROBOT01_CONFIG)

    if strategy.take_profit_pct < 0:
        raise ValueError("take_profit_pct must be >= 0")

    if strategy.stop_loss_pct < 0:
        raise ValueError("stop_loss_pct must be >= 0")

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
