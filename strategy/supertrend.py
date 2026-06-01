import asyncio
from typing import Dict, Optional

import uvloop

from core.common import cfg_value, load_full_config
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
SUPERTREND_CONFIG = CONFIG.get("supertrend", {})


class SupertrendStrategy(StrategyBase):
    __slots__ = ("states", "atr_period", "multiplier")

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)
        self.states: Dict[str, dict] = {}
        self.atr_period = max(1, int(section.get("atr_period", 10)))
        self.multiplier = max(0.1, float(section.get("multiplier", 3.0)))

    def _symbol_key(self, candle: Candle) -> str:
        symbol = str(candle.symbol or self.config.symbol or "").strip().upper()
        return symbol or str(self.config.symbol or "UNKNOWN").strip().upper()

    def _get_state(self, symbol: str) -> dict:
        state = self.states.get(symbol)
        if state is not None:
            return state

        state = {
            "atr": ATR(self.atr_period),
            "prev_close": None,
            "prev_final_upper": None,
            "prev_final_lower": None,
            "prev_supertrend": None,
            "trend": 0,
            "position_side": None,
            "entry_price": None,
        }
        self.states[symbol] = state
        return state

    def _update_supertrend(self, state: dict, high_px: float, low_px: float, close_px: float) -> Optional[int]:
        atr_value = state["atr"].update(high_px, low_px, close_px)
        if atr_value is None:
            state["prev_close"] = close_px
            return None

        hl2 = (high_px + low_px) / 2.0
        basic_upper = hl2 + self.multiplier * atr_value
        basic_lower = hl2 - self.multiplier * atr_value

        prev_close = state.get("prev_close")
        prev_final_upper = state.get("prev_final_upper")
        prev_final_lower = state.get("prev_final_lower")
        prev_supertrend = state.get("prev_supertrend")

        if prev_final_upper is None or prev_final_lower is None or prev_close is None:
            final_upper = basic_upper
            final_lower = basic_lower
        else:
            if basic_upper < prev_final_upper or prev_close > prev_final_upper:
                final_upper = basic_upper
            else:
                final_upper = prev_final_upper

            if basic_lower > prev_final_lower or prev_close < prev_final_lower:
                final_lower = basic_lower
            else:
                final_lower = prev_final_lower

        if prev_supertrend is None:
            if close_px <= final_upper:
                supertrend = final_upper
                trend = -1
            else:
                supertrend = final_lower
                trend = 1
        elif prev_supertrend == prev_final_upper:
            if close_px <= final_upper:
                supertrend = final_upper
                trend = -1
            else:
                supertrend = final_lower
                trend = 1
        else:
            if close_px >= final_lower:
                supertrend = final_lower
                trend = 1
            else:
                supertrend = final_upper
                trend = -1

        state["prev_final_upper"] = final_upper
        state["prev_final_lower"] = final_lower
        state["prev_supertrend"] = supertrend
        state["prev_close"] = close_px
        state["trend"] = trend
        return trend

    def _enter_long(self, state: dict, candle: Candle, metadata: dict):
        previous_side = str(state.get("position_side") or "").strip().upper()
        state["position_side"] = "LONG"
        state["entry_price"] = candle.close

        if previous_side == "SHORT":
            reason = "REVERSE_TO_LONG_SUPERTREND"
        else:
            reason = "ENTER_LONG_SUPERTREND"

        metadata["trade_condition"] = "ENTRY"
        metadata["order_reason"] = reason
        metadata["exit_type"] = ""
        metadata["previous_side"] = previous_side or "FLAT"
        return self.buy(candle.close, reason, metadata)

    def _enter_short(self, state: dict, candle: Candle, metadata: dict):
        previous_side = str(state.get("position_side") or "").strip().upper()
        state["position_side"] = "SHORT"
        state["entry_price"] = candle.close

        if previous_side == "LONG":
            reason = "REVERSE_TO_SHORT_SUPERTREND"
        else:
            reason = "ENTER_SHORT_SUPERTREND"

        metadata["trade_condition"] = "ENTRY"
        metadata["order_reason"] = reason
        metadata["exit_type"] = ""
        metadata["previous_side"] = previous_side or "FLAT"
        return self.sell(candle.close, reason, metadata)

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        symbol = self._symbol_key(candle)
        state = self._get_state(symbol)

        high_px = candle.high if candle.high is not None else candle.close
        low_px = candle.low if candle.low is not None else candle.close
        trend = self._update_supertrend(state, high_px, low_px, candle.close)
        if trend is None:
            return None

        if is_warmup:
            return None

        metadata = {
            "symbol": symbol,
            "trend": "UP" if trend > 0 else "DOWN",
        }

        if trend > 0 and state["position_side"] != "LONG":
            return self._enter_long(state, candle, metadata)

        if trend < 0 and state["position_side"] != "SHORT":
            return self._enter_short(state, candle, metadata)

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
        source=section.get("source", "python-supertrend"),
        strategy_id=section.get("strategy_id", "python-supertrend"),
        need_notification=bool(section.get("need_notification", False)),
        is_paper_trading=bool(section.get("is_paper_trading", True)),
        order_type=section.get("order_type", "MARKET"),
        order_qty=float(section.get("order_qty", 10)),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
        # Supertrend strategy intentionally uses no local risk-control exits.
        enable_intrabar_risk_exit=bool(cfg_value(section, GLOBAL_CONFIG, "enable_intrabar_risk_exit", False)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "SUPERTREND"),
        symbol="*",
        interval="*",
        warmup_limit=int(section.get("historical_limit", 600)),
    )


async def run():
    runtime = build_runtime_config(SUPERTREND_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = SupertrendStrategy(build_strategy_config(SUPERTREND_CONFIG), SUPERTREND_CONFIG)
    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
