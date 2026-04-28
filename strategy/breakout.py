import asyncio
from collections import deque
from typing import Optional, Tuple

import uvloop

from core.common import load_full_config
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, EMA
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
SECTION = CONFIG.get("breakout", {})


class DonchianWindow:
	__slots__ = ("window", "highs", "lows")

	def __init__(self, window: int):
		self.window = max(2, int(window))
		self.highs = deque(maxlen=self.window + 1)
		self.lows = deque(maxlen=self.window + 1)

	def update(self, high: float, low: float) -> Tuple[Optional[float], Optional[float]]:
		self.highs.append(high)
		self.lows.append(low)

		if len(self.highs) <= self.window:
			return None, None

		highs = list(self.highs)
		lows = list(self.lows)
		prev_upper = max(highs[:-1])
		prev_lower = min(lows[:-1])
		return prev_upper, prev_lower


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
		return self.total / len(self.buf)


class BreakoutPullbackStrategy(StrategyBase):
	__slots__ = (
		"ema_fast",
		"ema_slow",
		"atr",
		"donchian",
		"vol_mean",
		"prev_close",
		"position_side",
		"entry_price",
		"initial_stop",
		"trailing_stop",
		"highest_since_entry",
		"lowest_since_entry",
		"risk_per_unit",
		"pending_side",
		"pending_level",
		"pending_age",
		"pending_extreme",
		"cooldown",
		"cooldown_bars",
		"pullback_wait_bars",
		"pullback_tolerance_pct",
		"stop_atr_mult",
		"trail_atr_mult",
		"take_profit_r",
		"min_atr_pct",
		"volume_mult",
		"max_distance_ema200_pct",
		"warmup_min",
	)

	def __init__(self, strategy_config: StrategyConfig, section: dict):
		super().__init__(strategy_config)

		ema_fast_n = int(section.get("ema_fast", 20))
		ema_slow_n = int(section.get("ema_slow", 200))
		atr_n = int(section.get("atr_n", 14))
		donchian_n = int(section.get("donchian_n", 20))
		volume_window = int(section.get("volume_window", 20))

		self.ema_fast = EMA(ema_fast_n)
		self.ema_slow = EMA(ema_slow_n)
		self.atr = ATR(atr_n)
		self.donchian = DonchianWindow(donchian_n)
		self.vol_mean = RollingMean(volume_window)

		self.prev_close: Optional[float] = None

		self.position_side: Optional[str] = None
		self.entry_price: Optional[float] = None
		self.initial_stop: Optional[float] = None
		self.trailing_stop: Optional[float] = None
		self.highest_since_entry: Optional[float] = None
		self.lowest_since_entry: Optional[float] = None
		self.risk_per_unit: Optional[float] = None

		self.pending_side: Optional[str] = None
		self.pending_level: Optional[float] = None
		self.pending_age = 0
		self.pending_extreme: Optional[float] = None

		self.cooldown = 0
		self.cooldown_bars = int(section.get("cooldown_bars", 1))
		self.pullback_wait_bars = int(section.get("pullback_wait_bars", 5))
		self.pullback_tolerance_pct = float(section.get("pullback_tolerance_pct", 0.2))
		self.stop_atr_mult = float(section.get("stop_atr_mult", 1.5))
		self.trail_atr_mult = float(section.get("trail_atr_mult", 1.0))
		self.take_profit_r = float(section.get("take_profit_r", 1.5))
		self.min_atr_pct = float(section.get("min_atr_pct", 0.08))
		self.volume_mult = float(section.get("volume_mult", 1.05))
		self.max_distance_ema200_pct = float(section.get("max_distance_ema200_pct", 5.0))

		self.warmup_min = max(ema_slow_n, atr_n + 2, donchian_n + 2, volume_window)

	def _clear_pending(self) -> None:
		self.pending_side = None
		self.pending_level = None
		self.pending_age = 0
		self.pending_extreme = None

	def _reset_position(self) -> None:
		self.position_side = None
		self.entry_price = None
		self.initial_stop = None
		self.trailing_stop = None
		self.highest_since_entry = None
		self.lowest_since_entry = None
		self.risk_per_unit = None
		self.cooldown = self.cooldown_bars

	def _enter_long(self, candle: Candle, atr_value: float, metadata: dict):
		close = candle.close
		low = candle.low if candle.low is not None else close
		stop_by_level = (self.pending_extreme if self.pending_extreme is not None else low) - (0.1 * atr_value)
		stop_by_atr = close - (self.stop_atr_mult * atr_value)
		initial_stop = min(stop_by_level, stop_by_atr)

		self.position_side = "LONG"
		self.entry_price = close
		self.initial_stop = initial_stop
		self.trailing_stop = initial_stop
		self.highest_since_entry = candle.high if candle.high is not None else close
		self.lowest_since_entry = low
		self.risk_per_unit = max(1e-12, close - initial_stop)

		metadata["trade_condition"] = "ENTRY"
		metadata["initial_stop"] = round(initial_stop, 8)
		self._clear_pending()
		return self.buy(close, "ENTER_LONG_BREAKOUT_PULLBACK", metadata)

	def _enter_short(self, candle: Candle, atr_value: float, metadata: dict):
		close = candle.close
		high = candle.high if candle.high is not None else close
		stop_by_level = (self.pending_extreme if self.pending_extreme is not None else high) + (0.1 * atr_value)
		stop_by_atr = close + (self.stop_atr_mult * atr_value)
		initial_stop = max(stop_by_level, stop_by_atr)

		self.position_side = "SHORT"
		self.entry_price = close
		self.initial_stop = initial_stop
		self.trailing_stop = initial_stop
		self.highest_since_entry = high
		self.lowest_since_entry = candle.low if candle.low is not None else close
		self.risk_per_unit = max(1e-12, initial_stop - close)

		metadata["trade_condition"] = "ENTRY"
		metadata["initial_stop"] = round(initial_stop, 8)
		self._clear_pending()
		return self.sell(close, "ENTER_SHORT_BREAKOUT_PULLBACK", metadata)

	def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
		if not self.allow_new_candle(candle):
			return None

		if self.cooldown > 0:
			self.cooldown -= 1

		close = candle.close
		high = candle.high if candle.high is not None else close
		low = candle.low if candle.low is not None else close
		prev_close = self.prev_close
		self.prev_close = close

		ema_fast = self.ema_fast.update(close)
		ema_slow = self.ema_slow.update(close)
		atr_value = self.atr.update(high, low, close)
		prev_upper, prev_lower = self.donchian.update(high, low)
		avg_volume = self.vol_mean.update(candle.quote_volume)

		if self.ema_slow.count < self.warmup_min or atr_value is None or prev_upper is None or prev_lower is None:
			return None

		if is_warmup:
			return None

		atr_pct = atr_value / max(1e-12, close) * 100.0
		distance_ema200_pct = abs(close - ema_slow) / max(1e-12, ema_slow) * 100.0
		vol_ok = avg_volume is not None and candle.quote_volume >= avg_volume * self.volume_mult
		volatility_ok = atr_pct >= self.min_atr_pct
		extension_ok = distance_ema200_pct <= self.max_distance_ema200_pct
		bullish_momentum = prev_close is not None and close > prev_close
		bearish_momentum = prev_close is not None and close < prev_close

		metadata = {
			"ema20": round(ema_fast, 8),
			"ema200": round(ema_slow, 8),
			"atr": round(atr_value, 8),
			"atr_pct": round(atr_pct, 6),
			"donchian_upper": round(prev_upper, 8),
			"donchian_lower": round(prev_lower, 8),
			"distance_ema200_pct": round(distance_ema200_pct, 6),
		}

		if self.position_side == "LONG" and self.entry_price is not None:
			self.highest_since_entry = max(self.highest_since_entry or high, high)
			trail_candidate = ema_fast - (self.trail_atr_mult * atr_value)
			self.trailing_stop = max(self.trailing_stop or trail_candidate, trail_candidate)
			active_stop = max(self.initial_stop or trail_candidate, self.trailing_stop)

			metadata.update(
				{
					"entry_price": round(self.entry_price, 8),
					"active_stop": round(active_stop, 8),
					"trailing_stop": round(self.trailing_stop, 8),
				}
			)

			if low <= active_stop:
				metadata["trade_condition"] = "TRAILING_STOP" if active_stop > (self.initial_stop or active_stop) else "STOP_LOSS"
				self._reset_position()
				return self.sell(close, "EXIT_LONG_BREAKOUT_STOP", metadata)

			if self.take_profit_r > 0 and self.risk_per_unit is not None:
				tp_price = self.entry_price + (self.take_profit_r * self.risk_per_unit)
				metadata["tp_price"] = round(tp_price, 8)
				if high >= tp_price:
					metadata["trade_condition"] = "TAKE_PROFIT"
					self._reset_position()
					return self.sell(close, "EXIT_LONG_BREAKOUT_TP", metadata)

			if close < ema_fast:
				metadata["trade_condition"] = "EXIT"
				self._reset_position()
				return self.sell(close, "EXIT_LONG_BREAKOUT_EMA20", metadata)

			return None

		if self.position_side == "SHORT" and self.entry_price is not None:
			self.lowest_since_entry = min(self.lowest_since_entry or low, low)
			trail_candidate = ema_fast + (self.trail_atr_mult * atr_value)
			self.trailing_stop = min(self.trailing_stop or trail_candidate, trail_candidate)
			active_stop = min(self.initial_stop or trail_candidate, self.trailing_stop)

			metadata.update(
				{
					"entry_price": round(self.entry_price, 8),
					"active_stop": round(active_stop, 8),
					"trailing_stop": round(self.trailing_stop, 8),
				}
			)

			if high >= active_stop:
				metadata["trade_condition"] = "TRAILING_STOP" if active_stop < (self.initial_stop or active_stop) else "STOP_LOSS"
				self._reset_position()
				return self.buy(close, "EXIT_SHORT_BREAKOUT_STOP", metadata)

			if self.take_profit_r > 0 and self.risk_per_unit is not None:
				tp_price = self.entry_price - (self.take_profit_r * self.risk_per_unit)
				metadata["tp_price"] = round(tp_price, 8)
				if low <= tp_price:
					metadata["trade_condition"] = "TAKE_PROFIT"
					self._reset_position()
					return self.buy(close, "EXIT_SHORT_BREAKOUT_TP", metadata)

			if close > ema_fast:
				metadata["trade_condition"] = "EXIT"
				self._reset_position()
				return self.buy(close, "EXIT_SHORT_BREAKOUT_EMA20", metadata)

			return None

		if self.pending_side is None and self.cooldown == 0 and volatility_ok and extension_ok and vol_ok:
			if close > prev_upper:
				self.pending_side = "LONG"
				self.pending_level = prev_upper
				self.pending_age = 0
				self.pending_extreme = low
				return None

			if close < prev_lower:
				self.pending_side = "SHORT"
				self.pending_level = prev_lower
				self.pending_age = 0
				self.pending_extreme = high
				return None

		if self.pending_side is not None and self.pending_level is not None:
			self.pending_age += 1
			if self.pending_side == "LONG":
				self.pending_extreme = min(self.pending_extreme or low, low)
			else:
				self.pending_extreme = max(self.pending_extreme or high, high)

			if self.pending_age > self.pullback_wait_bars:
				self._clear_pending()
				return None

			tolerance = self.pending_level * (self.pullback_tolerance_pct / 100.0)

			if self.pending_side == "LONG":
				touched = low <= self.pending_level + tolerance
				hold_level = close >= self.pending_level
				setup_ok = touched and hold_level and close >= ema_fast and bullish_momentum
				if setup_ok:
					metadata["pullback_level"] = round(self.pending_level, 8)
					return self._enter_long(candle, atr_value, metadata)

			if self.pending_side == "SHORT":
				touched = high >= self.pending_level - tolerance
				hold_level = close <= self.pending_level
				setup_ok = touched and hold_level and close <= ema_fast and bearish_momentum
				if setup_ok:
					metadata["pullback_level"] = round(self.pending_level, 8)
					return self._enter_short(candle, atr_value, metadata)

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
		kline_subject=section.get("kline_subject", "KLINE.BINANCE.>"),
		order_subject=section.get("order_subject", "order_engine.place_order"),
		queue_name=section.get("queue_name", "KLINE_STRATEGY_BINANCE_BREAKOUT"),
		user_id=section.get("user_id", "paper-1"),
		exchange=section.get("exchange", "binance"),
		market_type=section.get("market_type", "futures"),
		position_side=section.get("position_side", "ONE_WAY"),
		source=section.get("source", "python-breakout"),
		strategy_id=section.get("strategy_id", "python-breakout-pullback"),
		is_paper_trading=section.get("is_paper_trading", True),
		order_type=section.get("order_type", "MARKET"),
		order_qty=float(section.get("order_qty", 1)),
		order_symbol=section.get("order_symbol", section.get("symbol", "SOL_USDT.P")),
		limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
	)


def build_strategy_config(section: dict) -> StrategyConfig:
	return StrategyConfig(
		name=section.get("name", "BREAKOUT_PULLBACK"),
		symbol=section.get("symbol", "SOL_USDT.P"),
		interval=section.get("interval", "15m"),
		warmup_limit=int(section.get("historical_limit", 800)),
	)


async def run():
	runtime = build_runtime_config(SECTION)

	if runtime.order_qty <= 0:
		raise ValueError("order_qty must be > 0")

	strategy = BreakoutPullbackStrategy(build_strategy_config(SECTION), SECTION)
	if strategy.stop_atr_mult <= 0:
		raise ValueError("stop_atr_mult must be > 0")
	if strategy.trail_atr_mult <= 0:
		raise ValueError("trail_atr_mult must be > 0")

	runner = StrategyRunner(strategy=strategy, runtime=runtime)
	await runner.run()


if __name__ == "__main__":
	asyncio.run(run())
