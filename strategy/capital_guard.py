import asyncio
from collections import deque
from typing import Dict, Optional

import uvloop

from core.common import cfg_value, load_full_config, pct_to_frac, runtime_options
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, EMA, MACD, RSI, RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
CAPITAL_GUARD_CONFIG = CONFIG.get("capital_guard", {})


class RollingMean:
    __slots__ = ("window", "buf", "total")

    def __init__(self, window: int):
        self.window = max(2, int(window))
        self.buf = deque()
        self.total = 0.0

    def update(self, value: float) -> Optional[float]:
        v = max(0.0, float(value))
        self.buf.append(v)
        self.total += v

        if len(self.buf) > self.window:
            self.total -= self.buf.popleft()

        if len(self.buf) < self.window:
            return None
        return self.total / float(len(self.buf))


class CapitalGuardStrategy(StrategyBase):
    __slots__ = (
        "states",
        "ema_fast_period",
        "ema_slow_period",
        "ema_trend_period",
        "vwap_window",
        "volume_window",
        "volume_mult",
        "atr_n",
        "atr_min_pct",
        "atr_max_pct",
        "rsi_period",
        "rsi_long_min",
        "rsi_long_max",
        "rsi_short_min",
        "rsi_short_max",
        "macd_fast",
        "macd_slow",
        "macd_signal",
        "macd_ready_min",
        "max_bar_range_pct",
        "cooldown_bars",
        "sl_cooldown_bars",
        "max_consecutive_stop_losses",
        "sl_pause_bars",
        "take_profit_pct",
        "stop_loss_pct",
        "trailing_stop_pct",
        "trailing_stop_trigger_pct",
        "max_hold_bars",
        "warmup_min",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        self.states: Dict[str, dict] = {}

        self.ema_fast_period = int(section.get("ema_fast", 21))
        self.ema_slow_period = int(section.get("ema_slow", 55))
        self.ema_trend_period = int(section.get("ema_trend", 200))
        self.vwap_window = int(section.get("vwap_window", 120))
        self.volume_window = int(section.get("volume_window", 30))
        self.volume_mult = float(section.get("volume_mult", 1.10))

        self.atr_n = int(section.get("atr_n", 14))
        self.atr_min_pct = float(section.get("atr_min_pct", 0.05))
        self.atr_max_pct = float(section.get("atr_max_pct", 1.25))
        self.max_bar_range_pct = float(section.get("max_bar_range_pct", 1.50))

        self.rsi_period = int(section.get("rsi_period", 14))
        self.rsi_long_min = float(section.get("rsi_long_min", 52.0))
        self.rsi_long_max = float(section.get("rsi_long_max", 72.0))
        self.rsi_short_min = float(section.get("rsi_short_min", 28.0))
        self.rsi_short_max = float(section.get("rsi_short_max", 48.0))

        self.macd_fast = int(section.get("macd_fast", 12))
        self.macd_slow = int(section.get("macd_slow", 26))
        self.macd_signal = int(section.get("macd_signal", 9))
        self.macd_ready_min = max(50, self.macd_slow + self.macd_signal)

        self.cooldown_bars = int(cfg_value(section, GLOBAL_CONFIG, "cooldown_bars", 2))
        self.sl_cooldown_bars = int(
            cfg_value(
                section,
                GLOBAL_CONFIG,
                "sl_cooldown_bars",
                cfg_value(section, GLOBAL_CONFIG, "stop_loss_cooldown_bars", 5),
            )
        )
        self.max_consecutive_stop_losses = int(cfg_value(section, GLOBAL_CONFIG, "max_consecutive_stop_losses", 2))
        self.sl_pause_bars = int(cfg_value(section, GLOBAL_CONFIG, "sl_pause_bars", 30))
        self.take_profit_pct = float(cfg_value(section, GLOBAL_CONFIG, "take_profit_pct", 0.35))
        self.stop_loss_pct = float(cfg_value(section, GLOBAL_CONFIG, "stop_loss_pct", 0.18))
        self.trailing_stop_pct = float(cfg_value(section, GLOBAL_CONFIG, "trailing_stop_pct", 0.12))
        self.trailing_stop_trigger_pct = float(cfg_value(section, GLOBAL_CONFIG, "trailing_stop_trigger_pct", 0.18))
        self.max_hold_bars = int(cfg_value(section, GLOBAL_CONFIG, "max_hold_bars", 18))

        self.warmup_min = max(
            self.ema_trend_period,
            self.vwap_window,
            self.volume_window,
            self.atr_n,
            self.rsi_period + 1,
            self.macd_ready_min,
        )

    def _symbol_key(self, candle: Candle) -> str:
        symbol = str(candle.symbol or self.config.symbol or "").strip().upper()
        return symbol or str(self.config.symbol or "UNKNOWN").strip().upper()

    def _get_state(self, symbol: str) -> dict:
        state = self.states.get(symbol)
        if state is not None:
            return state

        state = {
            "ema_fast": EMA(self.ema_fast_period),
            "ema_slow": EMA(self.ema_slow_period),
            "ema_trend": EMA(self.ema_trend_period),
            "vwap": RollingVWAP(self.vwap_window),
            "vol_mean": RollingMean(self.volume_window),
            "atr": ATR(self.atr_n),
            "rsi": RSI(self.rsi_period),
            "macd": MACD(self.macd_fast, self.macd_slow, self.macd_signal),
            "position_side": None,
            "entry_price": None,
            "highest_since_entry": None,
            "lowest_since_entry": None,
            "trail_armed": False,
            "bars_in_pos": 0,
            "cooldown": 0,
            "pause_bars": 0,
            "stop_loss_streak": 0,
            "reentry_lock_side": None,
        }
        self.states[symbol] = state
        return state

    def _reset_position(self, state: dict, exit_type: str = "", exited_side: str = "") -> None:
        state["position_side"] = None
        state["entry_price"] = None
        state["highest_since_entry"] = None
        state["lowest_since_entry"] = None
        state["trail_armed"] = False
        state["bars_in_pos"] = 0

        normalized_side = str(exited_side or "").strip().upper()
        if normalized_side in {"LONG", "SHORT"}:
            state["reentry_lock_side"] = normalized_side

        normalized_exit = str(exit_type or "").strip().upper()
        if normalized_exit == "STOP_LOSS":
            state["stop_loss_streak"] = int(state.get("stop_loss_streak", 0)) + 1
            state["cooldown"] = max(self.cooldown_bars, self.sl_cooldown_bars)
            if (
                self.max_consecutive_stop_losses > 0
                and state["stop_loss_streak"] >= self.max_consecutive_stop_losses
                and self.sl_pause_bars > 0
            ):
                state["pause_bars"] = max(int(state.get("pause_bars", 0)), self.sl_pause_bars)
                state["stop_loss_streak"] = 0
            return

        state["cooldown"] = self.cooldown_bars
        state["stop_loss_streak"] = 0

    def _update_reentry_lock(self, state: dict, crossed_up: bool, crossed_down: bool) -> None:
        lock_side = str(state.get("reentry_lock_side") or "").strip().upper()
        if lock_side == "LONG" and crossed_down:
            state["reentry_lock_side"] = None
        elif lock_side == "SHORT" and crossed_up:
            state["reentry_lock_side"] = None

    def _exit_metadata(self, symbol: str, state: dict, candle: Candle, intrabar: bool) -> dict:
        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close
        entry = float(state["entry_price"])
        return {
            "symbol": symbol,
            "intrabar": intrabar,
            "entry_price": round(entry, 8),
            "high": round(high_px, 8),
            "low": round(low_px, 8),
            "bars_in_pos": int(state.get("bars_in_pos", 0)),
        }

    def _risk_exit(self, symbol: str, state: dict, candle: Candle, intrabar: bool = False):
        if state["position_side"] is None or state["entry_price"] is None:
            return None

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close
        entry = float(state["entry_price"])
        metadata = self._exit_metadata(symbol, state, candle, intrabar)

        if state["position_side"] == "LONG":
            state["highest_since_entry"] = max(state["highest_since_entry"] or high_px, high_px)
            sl_px = entry * (1.0 - pct_to_frac(self.stop_loss_pct))
            tp_px = entry * (1.0 + pct_to_frac(self.take_profit_pct))
            trigger_px = entry * (1.0 + pct_to_frac(max(0.0, self.trailing_stop_trigger_pct)))
            if self.trailing_stop_trigger_pct <= 0 or high_px >= trigger_px:
                state["trail_armed"] = True

            trail_px = None
            if self.trailing_stop_pct > 0 and state.get("trail_armed"):
                trail_px = (state["highest_since_entry"] or high_px) * (1.0 - pct_to_frac(self.trailing_stop_pct))

            metadata.update(
                {
                    "tp_px": round(tp_px, 8),
                    "sl_px": round(sl_px, 8),
                    "trail_px": round(trail_px, 8) if trail_px is not None else None,
                    "trail_trigger_px": round(trigger_px, 8),
                    "trail_armed": bool(state.get("trail_armed")),
                }
            )

            if self.stop_loss_pct > 0 and low_px <= sl_px:
                metadata.update({"trade_condition": "STOP_LOSS", "order_reason": "STOP_LOSS_LONG", "exit_type": "STOP_LOSS"})
                self._reset_position(state, "STOP_LOSS", "LONG")
                return self.sell(min(candle.close, sl_px), "STOP_LOSS_LONG", metadata)

            if self.take_profit_pct > 0 and high_px >= tp_px:
                metadata.update({"trade_condition": "TAKE_PROFIT", "order_reason": "TAKE_PROFIT_LONG", "exit_type": "TAKE_PROFIT"})
                self._reset_position(state, "TAKE_PROFIT", "LONG")
                return self.sell(max(candle.close, tp_px), "TAKE_PROFIT_LONG", metadata)

            if trail_px is not None and low_px <= trail_px:
                metadata.update({"trade_condition": "TRAILING_STOP", "order_reason": "TRAILING_STOP_LONG", "exit_type": "TRAILING_STOP"})
                self._reset_position(state, "TRAILING_STOP", "LONG")
                return self.sell(min(candle.close, trail_px), "TRAILING_STOP_LONG", metadata)

        if state["position_side"] == "SHORT":
            state["lowest_since_entry"] = min(state["lowest_since_entry"] or low_px, low_px)
            sl_px = entry * (1.0 + pct_to_frac(self.stop_loss_pct))
            tp_px = entry * (1.0 - pct_to_frac(self.take_profit_pct))
            trigger_px = entry * (1.0 - pct_to_frac(max(0.0, self.trailing_stop_trigger_pct)))
            if self.trailing_stop_trigger_pct <= 0 or low_px <= trigger_px:
                state["trail_armed"] = True

            trail_px = None
            if self.trailing_stop_pct > 0 and state.get("trail_armed"):
                trail_px = (state["lowest_since_entry"] or low_px) * (1.0 + pct_to_frac(self.trailing_stop_pct))

            metadata.update(
                {
                    "tp_px": round(tp_px, 8),
                    "sl_px": round(sl_px, 8),
                    "trail_px": round(trail_px, 8) if trail_px is not None else None,
                    "trail_trigger_px": round(trigger_px, 8),
                    "trail_armed": bool(state.get("trail_armed")),
                }
            )

            if self.stop_loss_pct > 0 and high_px >= sl_px:
                metadata.update({"trade_condition": "STOP_LOSS", "order_reason": "STOP_LOSS_SHORT", "exit_type": "STOP_LOSS"})
                self._reset_position(state, "STOP_LOSS", "SHORT")
                return self.buy(max(candle.close, sl_px), "STOP_LOSS_SHORT", metadata)

            if self.take_profit_pct > 0 and low_px <= tp_px:
                metadata.update({"trade_condition": "TAKE_PROFIT", "order_reason": "TAKE_PROFIT_SHORT", "exit_type": "TAKE_PROFIT"})
                self._reset_position(state, "TAKE_PROFIT", "SHORT")
                return self.buy(min(candle.close, tp_px), "TAKE_PROFIT_SHORT", metadata)

            if trail_px is not None and high_px >= trail_px:
                metadata.update({"trade_condition": "TRAILING_STOP", "order_reason": "TRAILING_STOP_SHORT", "exit_type": "TRAILING_STOP"})
                self._reset_position(state, "TRAILING_STOP", "SHORT")
                return self.buy(max(candle.close, trail_px), "TRAILING_STOP_SHORT", metadata)

        return None

    def on_price_update(self, candle: Candle):
        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)
        return self._risk_exit(symbol, state, candle, intrabar=True)

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)

        if not self.allow_new_candle(candle):
            return None

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close

        ema_fast = state["ema_fast"].update(candle.close)
        ema_slow = state["ema_slow"].update(candle.close)
        ema_trend = state["ema_trend"].update(candle.close)
        vwap_px = state["vwap"].update(candle.close, candle.quote_volume)
        vol_avg = state["vol_mean"].update(candle.quote_volume)
        atr = state["atr"].update(high_px, low_px, candle.close)
        rsi = state["rsi"].update(candle.close)
        macd_line, signal_line, crossed_up, crossed_down = state["macd"].update(candle.close)
        self._update_reentry_lock(state, crossed_up, crossed_down)

        atr_pct = (atr / candle.close * 100.0) if atr is not None and candle.close > 0 else None
        bar_range_pct = ((high_px - low_px) / candle.close * 100.0) if candle.close > 0 else None

        ready = (
            state["ema_trend"].count >= self.warmup_min
            and vwap_px is not None
            and vol_avg is not None
            and atr_pct is not None
            and rsi is not None
            and state["macd"].count >= self.macd_ready_min
        )
        if not ready or is_warmup:
            return None

        risk_signal = self._risk_exit(symbol, state, candle, intrabar=False)
        if risk_signal is not None:
            return risk_signal

        metadata = {
            "symbol": symbol,
            "ema_fast": round(ema_fast, 8),
            "ema_slow": round(ema_slow, 8),
            "ema_trend": round(ema_trend, 8),
            "vwap": round(vwap_px, 8),
            "volume": round(candle.quote_volume, 8),
            "volume_avg": round(vol_avg, 8),
            "volume_mult": round(self.volume_mult, 8),
            "atr_pct": round(atr_pct, 6),
            "bar_range_pct": round(bar_range_pct, 6) if bar_range_pct is not None else None,
            "rsi": round(rsi, 4),
            "macd": round(macd_line, 8),
            "macd_signal": round(signal_line, 8),
        }

        if state["position_side"] in {"LONG", "SHORT"} and state["entry_price"] is not None:
            state["bars_in_pos"] = int(state["bars_in_pos"]) + 1
            metadata["entry_price"] = round(float(state["entry_price"]), 8)
            metadata["bars_in_pos"] = int(state["bars_in_pos"])

            if self.max_hold_bars > 0 and state["bars_in_pos"] >= self.max_hold_bars:
                side = state["position_side"]
                self._reset_position(state, "TIMEOUT", side)
                metadata.update({"trade_condition": "EXIT", "order_reason": "TIMEOUT_EXIT", "exit_type": "TIMEOUT"})
                if side == "LONG":
                    return self.sell(candle.close, "TIMEOUT_EXIT_LONG", metadata)
                return self.buy(candle.close, "TIMEOUT_EXIT_SHORT", metadata)

            if state["position_side"] == "LONG" and (crossed_down or candle.close < vwap_px or ema_fast < ema_slow):
                self._reset_position(state, "SIGNAL", "LONG")
                metadata.update({"trade_condition": "EXIT", "order_reason": "PROTECTIVE_EXIT_LONG", "exit_type": "SIGNAL"})
                return self.sell(candle.close, "PROTECTIVE_EXIT_LONG", metadata)

            if state["position_side"] == "SHORT" and (crossed_up or candle.close > vwap_px or ema_fast > ema_slow):
                self._reset_position(state, "SIGNAL", "SHORT")
                metadata.update({"trade_condition": "EXIT", "order_reason": "PROTECTIVE_EXIT_SHORT", "exit_type": "SIGNAL"})
                return self.buy(candle.close, "PROTECTIVE_EXIT_SHORT", metadata)

            return None

        if state["pause_bars"] > 0:
            state["pause_bars"] -= 1
            return None

        entry_cooldown_active = state["cooldown"] > 0
        if entry_cooldown_active:
            state["cooldown"] -= 1
            return None

        vol_ok = candle.quote_volume >= (vol_avg * self.volume_mult)
        atr_ok = self.atr_min_pct <= atr_pct <= self.atr_max_pct
        range_ok = bar_range_pct is not None and bar_range_pct <= self.max_bar_range_pct
        long_trend = candle.close > vwap_px and ema_fast > ema_slow > ema_trend
        short_trend = candle.close < vwap_px and ema_fast < ema_slow < ema_trend
        long_momentum = crossed_up and self.rsi_long_min <= rsi <= self.rsi_long_max
        short_momentum = crossed_down and self.rsi_short_min <= rsi <= self.rsi_short_max

        if vol_ok and atr_ok and range_ok and long_trend and long_momentum and state.get("reentry_lock_side") != "LONG":
            state["position_side"] = "LONG"
            state["entry_price"] = candle.close
            state["highest_since_entry"] = high_px
            state["lowest_since_entry"] = low_px
            state["trail_armed"] = False
            state["bars_in_pos"] = 0
            state["cooldown"] = self.cooldown_bars
            metadata.update({"trade_condition": "ENTRY", "order_reason": "CAPITAL_GUARD_ENTER_LONG", "exit_type": ""})
            return self.buy(candle.close, "CAPITAL_GUARD_ENTER_LONG", metadata)

        if vol_ok and atr_ok and range_ok and short_trend and short_momentum and state.get("reentry_lock_side") != "SHORT":
            state["position_side"] = "SHORT"
            state["entry_price"] = candle.close
            state["highest_since_entry"] = high_px
            state["lowest_since_entry"] = low_px
            state["trail_armed"] = False
            state["bars_in_pos"] = 0
            state["cooldown"] = self.cooldown_bars
            metadata.update({"trade_condition": "ENTRY", "order_reason": "CAPITAL_GUARD_ENTER_SHORT", "exit_type": ""})
            return self.sell(candle.close, "CAPITAL_GUARD_ENTER_SHORT", metadata)

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
        source="python-capital-guard",
        strategy_id="python-capital-guard",
        **runtime_options(GLOBAL_CONFIG, section),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "CAPITAL_GUARD"),
        symbol="*",
        interval="*",
        warmup_limit=int(section.get("historical_limit", 1000)),
    )


async def run():
    runtime = build_runtime_config(CAPITAL_GUARD_CONFIG)
    strategy = CapitalGuardStrategy(build_strategy_config(CAPITAL_GUARD_CONFIG), CAPITAL_GUARD_CONFIG)

    if strategy.volume_mult <= 0:
        raise ValueError("volume_mult must be > 0")
    if strategy.atr_min_pct < 0:
        raise ValueError("atr_min_pct must be >= 0")
    if strategy.atr_max_pct <= 0:
        raise ValueError("atr_max_pct must be > 0")
    if strategy.atr_min_pct > strategy.atr_max_pct:
        raise ValueError("atr_min_pct must be <= atr_max_pct")
    if strategy.max_bar_range_pct <= 0:
        raise ValueError("max_bar_range_pct must be > 0")
    if strategy.take_profit_pct < 0:
        raise ValueError("take_profit_pct must be >= 0")
    if strategy.stop_loss_pct < 0:
        raise ValueError("stop_loss_pct must be >= 0")
    if strategy.trailing_stop_pct < 0:
        raise ValueError("trailing_stop_pct must be >= 0")
    if strategy.trailing_stop_trigger_pct < 0:
        raise ValueError("trailing_stop_trigger_pct must be >= 0")
    if strategy.cooldown_bars < 0:
        raise ValueError("cooldown_bars must be >= 0")
    if strategy.sl_cooldown_bars < 0:
        raise ValueError("sl_cooldown_bars must be >= 0")
    if strategy.max_consecutive_stop_losses < 0:
        raise ValueError("max_consecutive_stop_losses must be >= 0")
    if strategy.sl_pause_bars < 0:
        raise ValueError("sl_pause_bars must be >= 0")
    if strategy.max_hold_bars < 0:
        raise ValueError("max_hold_bars must be >= 0")

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
