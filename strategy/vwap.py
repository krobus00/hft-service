import asyncio
from typing import Optional

import uvloop

from core.common import load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
VWAP_CONFIG = CONFIG.get("vwap", {})


class VWAPMeanReversionStrategy(StrategyBase):
    __slots__ = (
        "vwap",
        "last_vwap",
        "pos",
        "entry_price",
        "bars_in_pos",
        "cooldown",
        "entry_threshold_pct",
        "tp_pct",
        "sl_pct",
        "max_hold_bars",
        "cooldown_bars",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        self.vwap = RollingVWAP(int(section.get("vwap_window", 30)))
        self.last_vwap: Optional[float] = None

        self.pos = "FLAT"
        self.entry_price: Optional[float] = None
        self.bars_in_pos = 0
        self.cooldown = 0

        self.entry_threshold_pct = float(section.get("entry_threshold_pct", section.get("entry_threshold_bps", 5) / 100.0))
        self.tp_pct = float(section.get("tp_pct", section.get("tp_bps", 2) / 100.0))
        self.sl_pct = float(section.get("sl_pct", section.get("sl_bps", 8) / 100.0))
        self.max_hold_bars = int(section.get("max_hold_bars", 8))
        self.cooldown_bars = int(section.get("cooldown_bars", 1))

    def _entry_band(self, vwap_px: float):
        band = vwap_px * pct_to_frac(self.entry_threshold_pct)
        return vwap_px - band, vwap_px + band

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        if self.cooldown > 0:
            self.cooldown -= 1

        vwap_px = self.vwap.update(candle.close, candle.quote_volume)
        if vwap_px is None:
            return None

        self.last_vwap = vwap_px
        lower, upper = self._entry_band(vwap_px)

        if is_warmup:
            return None

        metadata = {
            "vwap": round(vwap_px, 8),
            "lower": round(lower, 8),
            "upper": round(upper, 8),
        }

        if self.pos == "LONG" and self.entry_price is not None:
            self.bars_in_pos += 1
            take_px = self.entry_price * (1.0 + pct_to_frac(self.tp_pct))
            stop_px = self.entry_price * (1.0 - pct_to_frac(self.sl_pct))

            if candle.close >= take_px:
                reason = "TP_LONG"
            elif candle.close <= stop_px:
                reason = "SL_LONG"
            elif candle.close >= vwap_px:
                reason = "MEAN_REVERT_LONG"
            elif self.bars_in_pos >= self.max_hold_bars:
                reason = "TIME_EXIT_LONG"
            else:
                reason = ""

            if reason:
                self.pos = "FLAT"
                self.entry_price = None
                self.bars_in_pos = 0
                self.cooldown = self.cooldown_bars
                return self.sell(candle.close, reason, metadata)

        if self.pos == "SHORT" and self.entry_price is not None:
            self.bars_in_pos += 1
            take_px = self.entry_price * (1.0 - pct_to_frac(self.tp_pct))
            stop_px = self.entry_price * (1.0 + pct_to_frac(self.sl_pct))

            if candle.close <= take_px:
                reason = "TP_SHORT"
            elif candle.close >= stop_px:
                reason = "SL_SHORT"
            elif candle.close <= vwap_px:
                reason = "MEAN_REVERT_SHORT"
            elif self.bars_in_pos >= self.max_hold_bars:
                reason = "TIME_EXIT_SHORT"
            else:
                reason = ""

            if reason:
                self.pos = "FLAT"
                self.entry_price = None
                self.bars_in_pos = 0
                self.cooldown = self.cooldown_bars
                return self.buy(candle.close, reason, metadata)

        if self.pos == "FLAT" and self.cooldown == 0:
            if candle.close > upper:
                self.pos = "SHORT"
                self.entry_price = candle.close
                self.bars_in_pos = 0
                return self.sell(candle.close, "ENTER_SHORT", metadata)

            if candle.close < lower:
                self.pos = "LONG"
                self.entry_price = candle.close
                self.bars_in_pos = 0
                return self.buy(candle.close, "ENTER_LONG", metadata)

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
        queue_name=section.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_VWAP"),
        user_id=section.get("user_id", "paper-1"),
        exchange=section.get("exchange", "tokocrypto"),
        market_type=section.get("market_type", "spot"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-vwap"),
        strategy_id=section.get("strategy_id", "python-vwap-mean-revert-hf-v1"),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        order_symbol=section.get("order_symbol", section.get("symbol", "SOLUSDT")),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name="VWAP-HF",
        symbol=section.get("symbol", "SOLUSDT"),
        interval=section.get("interval", "1m"),
        warmup_limit=int(section.get("historical_limit", 500)),
    )


async def run():
    runtime = build_runtime_config(VWAP_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = VWAPMeanReversionStrategy(build_strategy_config(VWAP_CONFIG), VWAP_CONFIG)

    if strategy.entry_threshold_pct < 0:
        raise ValueError("entry_threshold_pct must be >= 0")

    if strategy.tp_pct < 0:
        raise ValueError("tp_pct must be >= 0")

    if strategy.sl_pct < 0:
        raise ValueError("sl_pct must be >= 0")

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
