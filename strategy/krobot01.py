import asyncio
from typing import Dict, Optional

import uvloop

from core.common import cfg_value, load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import EMA, MACD, RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
KROBOT01_CONFIG = CONFIG.get("krobot01", {})


class Krobot01Strategy(StrategyBase):
    __slots__ = (
        "states",
        "ema_period",
        "vwap_window",
        "macd_fast",
        "macd_slow",
        "macd_signal",
        "macd_ready_min",
        "cooldown_bars",
        "sl_cooldown_bars",
        "max_consecutive_stop_losses",
        "sl_pause_bars",
        "take_profit_pct",
        "stop_loss_pct",
        "trailing_stop_pct",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        ema_period = int(section.get("ema_period", 200))
        vwap_window = int(section.get("vwap_window", 200))
        macd_fast = int(section.get("macd_fast", 12))
        macd_slow = int(section.get("macd_slow", 26))
        macd_signal = int(section.get("macd_signal", 9))

        self.states: Dict[str, dict] = {}

        self.ema_period = ema_period
        self.vwap_window = vwap_window
        self.macd_fast = macd_fast
        self.macd_slow = macd_slow
        self.macd_signal = macd_signal
        self.macd_ready_min = max(50, macd_slow + macd_signal)
        self.cooldown_bars = int(cfg_value(section, GLOBAL_CONFIG, "cooldown_bars", 1))
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
        self.take_profit_pct = float(cfg_value(section, GLOBAL_CONFIG, "take_profit_pct", section.get("tp_pct", 0.0)))
        self.stop_loss_pct = float(cfg_value(section, GLOBAL_CONFIG, "stop_loss_pct", section.get("sl_pct", 0.0)))
        self.trailing_stop_pct = float(
            cfg_value(section, GLOBAL_CONFIG, "trailing_stop_pct", section.get("ts_pct", 0.0))
        )

    def _symbol_key(self, candle: Candle) -> str:
        symbol = str(candle.symbol or self.config.symbol or "").strip().upper()
        return symbol or str(self.config.symbol or "UNKNOWN").strip().upper()

    def _get_state(self, symbol: str) -> dict:
        state = self.states.get(symbol)
        if state is not None:
            return state

        state = {
            "ema": EMA(self.ema_period),
            "vwap": RollingVWAP(self.vwap_window),
            "macd": MACD(self.macd_fast, self.macd_slow, self.macd_signal),
            "cooldown": 0,
            "pause_bars": 0,
            "stop_loss_streak": 0,
            "reentry_lock_side": None,
            "position_side": None,
            "entry_price": None,
            "highest_since_entry": None,
            "lowest_since_entry": None,
        }
        self.states[symbol] = state
        return state

    def _reset_position(self, state: dict, exit_type: str = "", exited_side: str = "") -> None:
        state["position_side"] = None
        state["entry_price"] = None
        state["highest_since_entry"] = None
        state["lowest_since_entry"] = None

        normalized_exited_side = str(exited_side or "").strip().upper()
        if normalized_exited_side in {"LONG", "SHORT"}:
            state["reentry_lock_side"] = normalized_exited_side

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
            return
        if lock_side == "SHORT" and crossed_up:
            state["reentry_lock_side"] = None

    def on_price_update(self, candle: Candle):
        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)

        if state["position_side"] is None or state["entry_price"] is None:
            return None

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close

        metadata = {
            "intrabar": True,
            "symbol": symbol,
            "entry_price": round(float(state["entry_price"]), 8),
            "high": round(high_px, 8),
            "low": round(low_px, 8),
        }

        if state["position_side"] == "LONG":
            state["highest_since_entry"] = max(state["highest_since_entry"] or high_px, high_px)
            entry_price = float(state["entry_price"])
            sl_px = entry_price * (1.0 - pct_to_frac(self.stop_loss_pct))
            tp_px = entry_price * (1.0 + pct_to_frac(self.take_profit_pct))
            trail_px = (state["highest_since_entry"] or high_px) * (1.0 - pct_to_frac(self.trailing_stop_pct))

            metadata.update(
                {
                    "trade_condition": "EXIT",
                    "sl_px": round(sl_px, 8),
                    "tp_px": round(tp_px, 8),
                    "trail_px": round(trail_px, 8),
                }
            )

            if self.stop_loss_pct > 0 and low_px <= sl_px:
                metadata["trade_condition"] = "STOP_LOSS"
                metadata["order_reason"] = "STOP_LOSS_LONG"
                metadata["exit_type"] = "STOP_LOSS"
                self._reset_position(state, "STOP_LOSS", "LONG")
                return self.sell(candle.close, "STOP_LOSS_LONG", metadata)

            if self.take_profit_pct > 0 and high_px >= tp_px:
                metadata["trade_condition"] = "TAKE_PROFIT"
                metadata["order_reason"] = "TAKE_PROFIT_LONG"
                metadata["exit_type"] = "TAKE_PROFIT"
                self._reset_position(state, "TAKE_PROFIT", "LONG")
                return self.sell(candle.close, "TAKE_PROFIT_LONG", metadata)

            if self.trailing_stop_pct > 0 and low_px <= trail_px:
                metadata["trade_condition"] = "TRAILING_STOP"
                metadata["order_reason"] = "TRAILING_STOP_LONG"
                metadata["exit_type"] = "TRAILING_STOP"
                self._reset_position(state, "TRAILING_STOP", "LONG")
                return self.sell(candle.close, "TRAILING_STOP_LONG", metadata)

        if state["position_side"] == "SHORT":
            state["lowest_since_entry"] = min(state["lowest_since_entry"] or low_px, low_px)
            entry_price = float(state["entry_price"])
            sl_px = entry_price * (1.0 + pct_to_frac(self.stop_loss_pct))
            tp_px = entry_price * (1.0 - pct_to_frac(self.take_profit_pct))
            trail_px = (state["lowest_since_entry"] or low_px) * (1.0 + pct_to_frac(self.trailing_stop_pct))

            metadata.update(
                {
                    "trade_condition": "EXIT",
                    "sl_px": round(sl_px, 8),
                    "tp_px": round(tp_px, 8),
                    "trail_px": round(trail_px, 8),
                }
            )

            if self.stop_loss_pct > 0 and high_px >= sl_px:
                metadata["trade_condition"] = "STOP_LOSS"
                metadata["order_reason"] = "STOP_LOSS_SHORT"
                metadata["exit_type"] = "STOP_LOSS"
                self._reset_position(state, "STOP_LOSS", "SHORT")
                return self.buy(candle.close, "STOP_LOSS_SHORT", metadata)

            if self.take_profit_pct > 0 and low_px <= tp_px:
                metadata["trade_condition"] = "TAKE_PROFIT"
                metadata["order_reason"] = "TAKE_PROFIT_SHORT"
                metadata["exit_type"] = "TAKE_PROFIT"
                self._reset_position(state, "TAKE_PROFIT", "SHORT")
                return self.buy(candle.close, "TAKE_PROFIT_SHORT", metadata)

            if self.trailing_stop_pct > 0 and high_px >= trail_px:
                metadata["trade_condition"] = "TRAILING_STOP"
                metadata["order_reason"] = "TRAILING_STOP_SHORT"
                metadata["exit_type"] = "TRAILING_STOP"
                self._reset_position(state, "TRAILING_STOP", "SHORT")
                return self.buy(candle.close, "TRAILING_STOP_SHORT", metadata)

        return None

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)

        if not self.allow_new_candle(candle):
            return None

        ema = state["ema"].update(candle.close)
        vwap_px = state["vwap"].update(candle.close, candle.quote_volume)
        _, _, crossed_up, crossed_down = state["macd"].update(candle.close)
        self._update_reentry_lock(state, crossed_up, crossed_down)

        if state["ema"].count < self.ema_period or vwap_px is None or state["macd"].count < self.macd_ready_min:
            return None

        if is_warmup:
            return None

        metadata = {
            "symbol": symbol,
            "ema": round(ema, 8),
            "vwap": round(vwap_px, 8),
        }

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close

        if state["position_side"] == "LONG" and state["entry_price"] is not None:
            state["highest_since_entry"] = max(state["highest_since_entry"] or high_px, high_px)

            entry_price = float(state["entry_price"])
            sl_px = entry_price * (1.0 - pct_to_frac(self.stop_loss_pct))
            tp_px = entry_price * (1.0 + pct_to_frac(self.take_profit_pct))
            trail_px = (state["highest_since_entry"] or high_px) * (1.0 - pct_to_frac(self.trailing_stop_pct))

            metadata.update(
                {
                    "entry_price": round(entry_price, 8),
                    "high_since_entry": round(state["highest_since_entry"] or high_px, 8),
                    "tp_px": round(tp_px, 8),
                    "sl_px": round(sl_px, 8),
                    "trail_px": round(trail_px, 8),
                }
            )

            if self.stop_loss_pct > 0 and low_px <= sl_px:
                metadata["trade_condition"] = "STOP_LOSS"
                metadata["order_reason"] = "STOP_LOSS_LONG"
                metadata["exit_type"] = "STOP_LOSS"
                self._reset_position(state, "STOP_LOSS", "LONG")
                return self.sell(candle.close, "STOP_LOSS_LONG", metadata)

            if self.take_profit_pct > 0 and high_px >= tp_px:
                metadata["trade_condition"] = "TAKE_PROFIT"
                metadata["order_reason"] = "TAKE_PROFIT_LONG"
                metadata["exit_type"] = "TAKE_PROFIT"
                self._reset_position(state, "TAKE_PROFIT", "LONG")
                return self.sell(candle.close, "TAKE_PROFIT_LONG", metadata)

            if self.trailing_stop_pct > 0 and low_px <= trail_px:
                metadata["trade_condition"] = "TRAILING_STOP"
                metadata["order_reason"] = "TRAILING_STOP_LONG"
                metadata["exit_type"] = "TRAILING_STOP"
                self._reset_position(state, "TRAILING_STOP", "LONG")
                return self.sell(candle.close, "TRAILING_STOP_LONG", metadata)

        if state["position_side"] == "SHORT" and state["entry_price"] is not None:
            state["lowest_since_entry"] = min(state["lowest_since_entry"] or low_px, low_px)

            entry_price = float(state["entry_price"])
            sl_px = entry_price * (1.0 + pct_to_frac(self.stop_loss_pct))
            tp_px = entry_price * (1.0 - pct_to_frac(self.take_profit_pct))
            trail_px = (state["lowest_since_entry"] or low_px) * (1.0 + pct_to_frac(self.trailing_stop_pct))

            metadata.update(
                {
                    "entry_price": round(entry_price, 8),
                    "low_since_entry": round(state["lowest_since_entry"] or low_px, 8),
                    "tp_px": round(tp_px, 8),
                    "sl_px": round(sl_px, 8),
                    "trail_px": round(trail_px, 8),
                }
            )

            if self.stop_loss_pct > 0 and high_px >= sl_px:
                metadata["trade_condition"] = "STOP_LOSS"
                metadata["order_reason"] = "STOP_LOSS_SHORT"
                metadata["exit_type"] = "STOP_LOSS"
                self._reset_position(state, "STOP_LOSS", "SHORT")
                return self.buy(candle.close, "STOP_LOSS_SHORT", metadata)

            if self.take_profit_pct > 0 and low_px <= tp_px:
                metadata["trade_condition"] = "TAKE_PROFIT"
                metadata["order_reason"] = "TAKE_PROFIT_SHORT"
                metadata["exit_type"] = "TAKE_PROFIT"
                self._reset_position(state, "TAKE_PROFIT", "SHORT")
                return self.buy(candle.close, "TAKE_PROFIT_SHORT", metadata)

            if self.trailing_stop_pct > 0 and high_px >= trail_px:
                metadata["trade_condition"] = "TRAILING_STOP"
                metadata["order_reason"] = "TRAILING_STOP_SHORT"
                metadata["exit_type"] = "TRAILING_STOP"
                self._reset_position(state, "TRAILING_STOP", "SHORT")
                return self.buy(candle.close, "TRAILING_STOP_SHORT", metadata)

        if state["pause_bars"] > 0:
            state["pause_bars"] -= 1
            return None

        if state["cooldown"] > 0:
            state["cooldown"] -= 1
            return None

        # Keep current position until explicit exit conditions are met.
        if state["position_side"] is not None:
            return None

        long_cond = candle.close > ema and candle.close > vwap_px and crossed_up
        short_cond = candle.close < ema and candle.close < vwap_px and crossed_down

        print(
            f"symbol={symbol} close={candle.close:.6f} ema={ema:.6f} vwap={vwap_px:.6f} crossed_up={crossed_up} crossed_down={crossed_down} long_cond={long_cond} short_cond={short_cond}",
            flush=True,
        )

        if long_cond:
            if state["position_side"] == "LONG":
                return None
            if state.get("reentry_lock_side") == "LONG":
                return None
            state["position_side"] = "LONG"
            state["entry_price"] = candle.close
            state["highest_since_entry"] = high_px
            state["lowest_since_entry"] = low_px
            state["cooldown"] = self.cooldown_bars
            metadata["trade_condition"] = "ENTRY"
            metadata["order_reason"] = "ENTER_LONG"
            metadata["exit_type"] = ""
            return self.buy(candle.close, "ENTER_LONG", metadata)

        if short_cond:
            if state["position_side"] == "SHORT":
                return None
            if state.get("reentry_lock_side") == "SHORT":
                return None
            state["position_side"] = "SHORT"
            state["entry_price"] = candle.close
            state["highest_since_entry"] = high_px
            state["lowest_since_entry"] = low_px
            state["cooldown"] = self.cooldown_bars
            metadata["trade_condition"] = "ENTRY"
            metadata["order_reason"] = "ENTER_SHORT"
            metadata["exit_type"] = ""
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
        order_subject=section.get("order_subject", "order_engine.place_order"),
        user_id=section.get("user_id", "paper-1"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-krobot01"),
        strategy_id=section.get("strategy_id", "python-krobot01-ema-vwap-macd"),
        need_notification=bool(section.get("need_notification", False)),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
        enable_intrabar_risk_exit=bool(cfg_value(section, GLOBAL_CONFIG, "enable_intrabar_risk_exit", True)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name="KROBOT01",
        symbol="*",
        interval="*",
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

    if strategy.trailing_stop_pct < 0:
        raise ValueError("trailing_stop_pct must be >= 0")

    if strategy.cooldown_bars < 0:
        raise ValueError("cooldown_bars must be >= 0")

    if strategy.sl_cooldown_bars < 0:
        raise ValueError("sl_cooldown_bars must be >= 0")

    if strategy.max_consecutive_stop_losses < 0:
        raise ValueError("max_consecutive_stop_losses must be >= 0")

    if strategy.sl_pause_bars < 0:
        raise ValueError("sl_pause_bars must be >= 0")

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
