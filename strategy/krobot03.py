import asyncio
from typing import Dict, Optional

import uvloop

from core.common import cfg_value, load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, BollingerBands, EMA, RSI, Stochastic
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
KROBOT03_CONFIG = CONFIG.get("krobot03", {})


class Krobot03EMABBRSIMomentum(StrategyBase):
    __slots__ = (
        "states",
        "ema_fast_period",
        "ema_slow_period",
        "use_bollinger",
        "bb_window",
        "bb_std_mult",
        "use_atr_filter",
        "atr_n",
        "atr_min_pct",
        "use_stochastic",
        "stoch_k_period",
        "stoch_d_period",
        "stoch_low",
        "stoch_high",
        "rsi_period",
        "rsi_buy_min",
        "rsi_sell_max",
        "cooldown_bars",
        "sl_cooldown_bars",
        "max_consecutive_stop_losses",
        "sl_pause_bars",
        "take_profit_pct",
        "stop_loss_pct",
        "trailing_stop_pct",
        "trailing_stop_trigger_pct",
        "max_hold_bars",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        self.states: Dict[str, dict] = {}

        self.ema_fast_period = int(section.get("ema_fast", 21))
        self.ema_slow_period = int(section.get("ema_slow", 50))

        self.use_bollinger = bool(section.get("use_bollinger", True))
        self.bb_window = int(section.get("bb_window", 20))
        self.bb_std_mult = float(section.get("bb_std_mult", 2.0))

        self.use_atr_filter = bool(section.get("use_atr_filter", False))
        self.atr_n = int(section.get("atr_n", 14))
        self.atr_min_pct = float(section.get("atr_min_pct", 0.10))

        self.use_stochastic = bool(section.get("use_stochastic", False))
        self.stoch_k_period = int(section.get("stoch_k_period", 14))
        self.stoch_d_period = int(section.get("stoch_d_period", 3))
        self.stoch_low = float(section.get("stoch_low", 20.0))
        self.stoch_high = float(section.get("stoch_high", 80.0))

        self.rsi_period = int(section.get("rsi_period", 14))
        self.rsi_buy_min = float(section.get("rsi_buy_min", 52.0))
        self.rsi_sell_max = float(section.get("rsi_sell_max", 48.0))

        self.cooldown_bars = int(cfg_value(section, GLOBAL_CONFIG, "cooldown_bars", 2))
        self.sl_cooldown_bars = int(
            cfg_value(
                section,
                GLOBAL_CONFIG,
                "sl_cooldown_bars",
                cfg_value(section, GLOBAL_CONFIG, "stop_loss_cooldown_bars", 3),
            )
        )
        self.max_consecutive_stop_losses = int(cfg_value(section, GLOBAL_CONFIG, "max_consecutive_stop_losses", 2))
        self.sl_pause_bars = int(cfg_value(section, GLOBAL_CONFIG, "sl_pause_bars", 10))
        self.take_profit_pct = float(cfg_value(section, GLOBAL_CONFIG, "take_profit_pct", 0.5))
        self.stop_loss_pct = float(cfg_value(section, GLOBAL_CONFIG, "stop_loss_pct", 0.3))
        self.trailing_stop_pct = float(cfg_value(section, GLOBAL_CONFIG, "trailing_stop_pct", 0.2))
        self.trailing_stop_trigger_pct = float(cfg_value(section, GLOBAL_CONFIG, "trailing_stop_trigger_pct", 0.0))
        self.max_hold_bars = int(cfg_value(section, GLOBAL_CONFIG, "max_hold_bars", 24))

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
            "bb": BollingerBands(self.bb_window, self.bb_std_mult),
            "atr": ATR(self.atr_n),
            "rsi": RSI(self.rsi_period),
            "stoch": Stochastic(self.stoch_k_period, self.stoch_d_period),
            "prev_ema_spread": None,
            "position_side": None,
            "entry_price": None,
            "highest_since_entry": None,
            "lowest_since_entry": None,
            "trail_armed": False,
            "bars_in_pos": 0,
            "cooldown": 0,
            "pause_bars": 0,
            "stop_loss_streak": 0,
        }
        self.states[symbol] = state
        return state

    def _reset_position(self, state: dict, exit_type: str = "") -> None:
        state["position_side"] = None
        state["entry_price"] = None
        state["highest_since_entry"] = None
        state["lowest_since_entry"] = None
        state["trail_armed"] = False
        state["bars_in_pos"] = 0

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

    def _momentum_ok(self, side: str, rsi: Optional[float], stoch_k: Optional[float], stoch_d: Optional[float]) -> bool:
        if self.use_stochastic:
            if stoch_k is None or stoch_d is None:
                return False
            if side == "LONG":
                return stoch_k > stoch_d and stoch_k > self.stoch_low
            return stoch_k < stoch_d and stoch_k < self.stoch_high

        if rsi is None:
            return False
        if side == "LONG":
            return rsi >= self.rsi_buy_min
        return rsi <= self.rsi_sell_max

    def _volatility_ok(self, atr_pct: Optional[float]) -> bool:
        if not self.use_atr_filter:
            return True
        if atr_pct is None:
            return False
        return atr_pct >= self.atr_min_pct

    def on_price_update(self, candle: Candle):
        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)

        if state["position_side"] is None or state["entry_price"] is None:
            return None

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close
        entry = float(state["entry_price"])

        metadata = {
            "symbol": symbol,
            "entry_price": round(entry, 8),
            "intrabar": True,
        }

        if state["position_side"] == "LONG":
            state["highest_since_entry"] = max(state["highest_since_entry"] or high_px, high_px)
            sl_px = entry * (1.0 - pct_to_frac(self.stop_loss_pct))
            tp_px = entry * (1.0 + pct_to_frac(self.take_profit_pct))
            trailing_trigger_pct = max(0.0, float(self.trailing_stop_trigger_pct))
            trailing_trigger_px = entry * (1.0 + pct_to_frac(trailing_trigger_pct))
            if trailing_trigger_pct <= 0 or high_px >= trailing_trigger_px:
                state["trail_armed"] = True

            trail_px = None
            if self.trailing_stop_pct > 0 and state.get("trail_armed"):
                trail_px = (state["highest_since_entry"] or high_px) * (1.0 - pct_to_frac(self.trailing_stop_pct))

            if self.stop_loss_pct > 0 and low_px <= sl_px:
                metadata.update({"trade_condition": "STOP_LOSS", "exit_type": "STOP_LOSS", "order_reason": "STOP_LOSS_LONG"})
                self._reset_position(state, "STOP_LOSS")
                exit_px = min(candle.close, sl_px)
                return self.sell(exit_px, "STOP_LOSS_LONG", metadata)

            if self.take_profit_pct > 0 and high_px >= tp_px:
                metadata.update({"trade_condition": "TAKE_PROFIT", "exit_type": "TAKE_PROFIT", "order_reason": "TAKE_PROFIT_LONG"})
                self._reset_position(state, "TAKE_PROFIT")
                exit_px = max(candle.close, tp_px)
                return self.sell(exit_px, "TAKE_PROFIT_LONG", metadata)

            if trail_px is not None and low_px <= trail_px:
                metadata.update({"trade_condition": "TRAILING_STOP", "exit_type": "TRAILING_STOP", "order_reason": "TRAILING_STOP_LONG"})
                self._reset_position(state, "TRAILING_STOP")
                exit_px = min(candle.close, trail_px)
                return self.sell(exit_px, "TRAILING_STOP_LONG", metadata)

        if state["position_side"] == "SHORT":
            state["lowest_since_entry"] = min(state["lowest_since_entry"] or low_px, low_px)
            sl_px = entry * (1.0 + pct_to_frac(self.stop_loss_pct))
            tp_px = entry * (1.0 - pct_to_frac(self.take_profit_pct))
            trailing_trigger_pct = max(0.0, float(self.trailing_stop_trigger_pct))
            trailing_trigger_px = entry * (1.0 - pct_to_frac(trailing_trigger_pct))
            if trailing_trigger_pct <= 0 or low_px <= trailing_trigger_px:
                state["trail_armed"] = True

            trail_px = None
            if self.trailing_stop_pct > 0 and state.get("trail_armed"):
                trail_px = (state["lowest_since_entry"] or low_px) * (1.0 + pct_to_frac(self.trailing_stop_pct))

            if self.stop_loss_pct > 0 and high_px >= sl_px:
                metadata.update({"trade_condition": "STOP_LOSS", "exit_type": "STOP_LOSS", "order_reason": "STOP_LOSS_SHORT"})
                self._reset_position(state, "STOP_LOSS")
                exit_px = max(candle.close, sl_px)
                return self.buy(exit_px, "STOP_LOSS_SHORT", metadata)

            if self.take_profit_pct > 0 and low_px <= tp_px:
                metadata.update({"trade_condition": "TAKE_PROFIT", "exit_type": "TAKE_PROFIT", "order_reason": "TAKE_PROFIT_SHORT"})
                self._reset_position(state, "TAKE_PROFIT")
                exit_px = min(candle.close, tp_px)
                return self.buy(exit_px, "TAKE_PROFIT_SHORT", metadata)

            if trail_px is not None and high_px >= trail_px:
                metadata.update({"trade_condition": "TRAILING_STOP", "exit_type": "TRAILING_STOP", "order_reason": "TRAILING_STOP_SHORT"})
                self._reset_position(state, "TRAILING_STOP")
                exit_px = max(candle.close, trail_px)
                return self.buy(exit_px, "TRAILING_STOP_SHORT", metadata)

        return None

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)

        if not self.allow_new_candle(candle):
            return None

        if state["pause_bars"] > 0:
            state["pause_bars"] -= 1
            return None

        entry_cooldown_active = state["cooldown"] > 0
        if entry_cooldown_active:
            state["cooldown"] -= 1

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close

        ema_fast = state["ema_fast"].update(candle.close)
        ema_slow = state["ema_slow"].update(candle.close)
        bb = state["bb"].update(candle.close)
        atr = state["atr"].update(high_px, low_px, candle.close)
        rsi = state["rsi"].update(candle.close)
        stoch_k, stoch_d = state["stoch"].update(high_px, low_px, candle.close)

        atr_pct = (atr / candle.close * 100.0) if atr is not None and candle.close > 0 else None
        spread = ema_fast - ema_slow
        prev_spread = state.get("prev_ema_spread")
        crossed_up = prev_spread is not None and prev_spread <= 0 and spread > 0
        crossed_down = prev_spread is not None and prev_spread >= 0 and spread < 0
        state["prev_ema_spread"] = spread

        ready = state["ema_slow"].count >= self.ema_slow_period
        if self.use_bollinger and bb is None:
            ready = False
        if rsi is None and not self.use_stochastic:
            ready = False
        if self.use_stochastic and (stoch_k is None or stoch_d is None):
            ready = False
        if self.use_atr_filter and atr_pct is None:
            ready = False

        if not ready or is_warmup:
            return None

        bb_lower, bb_mid, bb_upper = bb if bb is not None else (None, None, None)
        metadata = {
            "symbol": symbol,
            "ema_fast": round(ema_fast, 8),
            "ema_slow": round(ema_slow, 8),
            "rsi": round(rsi, 4) if rsi is not None else None,
            "stoch_k": round(stoch_k, 4) if stoch_k is not None else None,
            "stoch_d": round(stoch_d, 4) if stoch_d is not None else None,
            "atr_pct": round(atr_pct, 6) if atr_pct is not None else None,
            "bb_mid": round(bb_mid, 8) if bb_mid is not None else None,
            "bb_upper": round(bb_upper, 8) if bb_upper is not None else None,
            "bb_lower": round(bb_lower, 8) if bb_lower is not None else None,
        }

        if state["position_side"] in {"LONG", "SHORT"} and state["entry_price"] is not None:
            state["bars_in_pos"] = int(state["bars_in_pos"]) + 1
            if self.max_hold_bars > 0 and state["bars_in_pos"] >= self.max_hold_bars:
                entry_side = state["position_side"]
                self._reset_position(state, "TIMEOUT")
                metadata.update({"trade_condition": "EXIT", "exit_type": "TIMEOUT", "order_reason": "TIMEOUT_EXIT"})
                if entry_side == "LONG":
                    return self.sell(candle.close, "TIMEOUT_EXIT_LONG", metadata)
                return self.buy(candle.close, "TIMEOUT_EXIT_SHORT", metadata)

        if state["position_side"] == "LONG" and crossed_down:
            self._reset_position(state, "SIGNAL")
            metadata.update({"trade_condition": "EXIT", "exit_type": "SIGNAL", "order_reason": "EMA_CROSSDOWN_EXIT_LONG"})
            return self.sell(candle.close, "EMA_CROSSDOWN_EXIT_LONG", metadata)

        if state["position_side"] == "SHORT" and crossed_up:
            self._reset_position(state, "SIGNAL")
            metadata.update({"trade_condition": "EXIT", "exit_type": "SIGNAL", "order_reason": "EMA_CROSSUP_EXIT_SHORT"})
            return self.buy(candle.close, "EMA_CROSSUP_EXIT_SHORT", metadata)

        if entry_cooldown_active:
            return None

        if state["position_side"] is not None:
            return None

        long_momentum = self._momentum_ok("LONG", rsi, stoch_k, stoch_d)
        short_momentum = self._momentum_ok("SHORT", rsi, stoch_k, stoch_d)
        vol_ok = self._volatility_ok(atr_pct)

        bb_long_ok = True
        bb_short_ok = True
        if self.use_bollinger and bb_mid is not None and bb_upper is not None and bb_lower is not None:
            bb_long_ok = candle.close > bb_mid and candle.close < bb_upper
            bb_short_ok = candle.close < bb_mid and candle.close > bb_lower

        if crossed_up and long_momentum and vol_ok and bb_long_ok:
            state["position_side"] = "LONG"
            state["entry_price"] = candle.close
            state["highest_since_entry"] = high_px
            state["lowest_since_entry"] = low_px
            state["trail_armed"] = False
            state["bars_in_pos"] = 0
            state["cooldown"] = self.cooldown_bars
            metadata.update({"trade_condition": "ENTRY", "order_reason": "EMA_CROSSUP_ENTER_LONG", "exit_type": ""})
            return self.buy(candle.close, "EMA_CROSSUP_ENTER_LONG", metadata)

        if crossed_down and short_momentum and vol_ok and bb_short_ok:
            state["position_side"] = "SHORT"
            state["entry_price"] = candle.close
            state["highest_since_entry"] = high_px
            state["lowest_since_entry"] = low_px
            state["trail_armed"] = False
            state["bars_in_pos"] = 0
            state["cooldown"] = self.cooldown_bars
            metadata.update({"trade_condition": "ENTRY", "order_reason": "EMA_CROSSDOWN_ENTER_SHORT", "exit_type": ""})
            return self.sell(candle.close, "EMA_CROSSDOWN_ENTER_SHORT", metadata)

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
        source=section.get("source", "python-krobot03"),
        strategy_id=section.get("strategy_id", "python-krobot03-ema-bb-rsi"),
        need_notification=bool(section.get("need_notification", False)),
        is_paper_trading=bool(section.get("is_paper_trading", True)),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
        enable_intrabar_risk_exit=bool(cfg_value(section, GLOBAL_CONFIG, "enable_intrabar_risk_exit", False)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "KROBOT03_EMA_BB_RSI"),
        symbol="*",
        interval="*",
        warmup_limit=int(section.get("historical_limit", 800)),
    )


async def run():
    runtime = build_runtime_config(KROBOT03_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = Krobot03EMABBRSIMomentum(build_strategy_config(KROBOT03_CONFIG), KROBOT03_CONFIG)

    if strategy.take_profit_pct < 0:
        raise ValueError("take_profit_pct must be >= 0")
    if strategy.stop_loss_pct < 0:
        raise ValueError("stop_loss_pct must be >= 0")
    if strategy.trailing_stop_pct < 0:
        raise ValueError("trailing_stop_pct must be >= 0")

    if strategy.trailing_stop_trigger_pct < 0:
        raise ValueError("trailing_stop_trigger_pct must be >= 0")

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
