import asyncio
from collections import deque
from typing import Optional

import uvloop

from core.common import load_full_config
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, EMA
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
SECTION = CONFIG.get("trend_following_atr", {})


class RSI:
	__slots__ = ("n", "prev_close", "avg_gain", "avg_loss", "count")

	def __init__(self, period: int):
		self.n = max(2, int(period))
		self.prev_close: Optional[float] = None
		self.avg_gain = 0.0
		self.avg_loss = 0.0
		self.count = 0

	def update(self, close: float) -> Optional[float]:
		if self.prev_close is None:
			self.prev_close = close
			return None

		change = close - self.prev_close
		gain = max(change, 0.0)
		loss = max(-change, 0.0)
		self.prev_close = close

		self.count += 1
		if self.count <= self.n:
			self.avg_gain += gain
			self.avg_loss += loss
			if self.count < self.n:
				return None
			self.avg_gain /= self.n
			self.avg_loss /= self.n
		else:
			self.avg_gain = ((self.avg_gain * (self.n - 1)) + gain) / self.n
			self.avg_loss = ((self.avg_loss * (self.n - 1)) + loss) / self.n

		if self.avg_loss == 0:
			return 100.0
		rs = self.avg_gain / self.avg_loss
		return 100.0 - (100.0 / (1.0 + rs))


class TrendFollowingATRStrategy(StrategyBase):
	__slots__ = (
		"ema_fast",
		"ema_mid",
		"ema_slow",
		"atr",
		"rsi",
		"prev_close",
		"cooldown",
		"cooldown_bars",
		"pullback_tolerance_pct",
		"atr_stop_mult",
		"atr_trail_mult",
		"take_profit_r",
		"use_rsi_filter",
		"rsi_min",
		"rsi_max",
		"use_volume_filter",
		"volume_window",
		"volume_mult",
		"volume_buf",
		"volume_sum",
		"position_side",
		"entry_price",
		"initial_stop",
		"trailing_stop",
		"highest_since_entry",
		"lowest_since_entry",
		"risk_per_unit",
		"warmup_min",
	)

	def __init__(self, strategy_config: StrategyConfig, section: dict):
		super().__init__(strategy_config)

		ema_fast_n = int(section.get("ema_fast", 20))
		ema_mid_n = int(section.get("ema_mid", 50))
		ema_slow_n = int(section.get("ema_slow", 200))
		atr_n = int(section.get("atr_n", 14))
		rsi_n = int(section.get("rsi_n", 14))

		self.ema_fast = EMA(ema_fast_n)
		self.ema_mid = EMA(ema_mid_n)
		self.ema_slow = EMA(ema_slow_n)
		self.atr = ATR(atr_n)
		self.rsi = RSI(rsi_n)

		self.prev_close: Optional[float] = None
		self.cooldown = 0
		self.cooldown_bars = int(section.get("cooldown_bars", 1))
		self.pullback_tolerance_pct = float(section.get("pullback_tolerance_pct", 0.25))
		self.atr_stop_mult = float(section.get("atr_stop_mult", 2.0))
		self.atr_trail_mult = float(section.get("atr_trail_mult", 2.0))
		self.take_profit_r = float(section.get("take_profit_r", 0.0))

		self.use_rsi_filter = bool(section.get("use_rsi_filter", False))
		self.rsi_min = float(section.get("rsi_min", 40.0))
		self.rsi_max = float(section.get("rsi_max", 60.0))

		self.use_volume_filter = bool(section.get("use_volume_filter", False))
		self.volume_window = max(2, int(section.get("volume_window", 20)))
		self.volume_mult = float(section.get("volume_mult", 1.0))
		self.volume_buf = deque()
		self.volume_sum = 0.0

		self.position_side: Optional[str] = None
		self.entry_price: Optional[float] = None
		self.initial_stop: Optional[float] = None
		self.trailing_stop: Optional[float] = None
		self.highest_since_entry: Optional[float] = None
		self.lowest_since_entry: Optional[float] = None
		self.risk_per_unit: Optional[float] = None

		self.warmup_min = max(ema_slow_n, atr_n + 2, rsi_n + 2)

	def _update_volume_avg(self, value: float) -> Optional[float]:
		v = max(0.0, float(value))
		self.volume_buf.append(v)
		self.volume_sum += v
		if len(self.volume_buf) > self.volume_window:
			self.volume_sum -= self.volume_buf.popleft()
		if len(self.volume_buf) < self.volume_window:
			return None
		return self.volume_sum / len(self.volume_buf)

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
		self.position_side = "LONG"
		self.entry_price = candle.close
		self.highest_since_entry = candle.high if candle.high is not None else candle.close
		self.lowest_since_entry = candle.low if candle.low is not None else candle.close
		self.initial_stop = candle.close - (self.atr_stop_mult * atr_value)
		self.trailing_stop = self.initial_stop
		self.risk_per_unit = max(1e-12, candle.close - self.initial_stop)
		self.cooldown = self.cooldown_bars
		metadata["trade_condition"] = "ENTRY"
		return self.buy(candle.close, "ENTER_LONG_TREND_ATR", metadata)

	def _enter_short(self, candle: Candle, atr_value: float, metadata: dict):
		self.position_side = "SHORT"
		self.entry_price = candle.close
		self.highest_since_entry = candle.high if candle.high is not None else candle.close
		self.lowest_since_entry = candle.low if candle.low is not None else candle.close
		self.initial_stop = candle.close + (self.atr_stop_mult * atr_value)
		self.trailing_stop = self.initial_stop
		self.risk_per_unit = max(1e-12, self.initial_stop - candle.close)
		self.cooldown = self.cooldown_bars
		metadata["trade_condition"] = "ENTRY"
		return self.sell(candle.close, "ENTER_SHORT_TREND_ATR", metadata)

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
		ema_mid = self.ema_mid.update(close)
		ema_slow = self.ema_slow.update(close)
		atr_value = self.atr.update(high, low, close)
		rsi_value = self.rsi.update(close)
		avg_volume = self._update_volume_avg(candle.quote_volume)

		if self.ema_slow.count < self.warmup_min or atr_value is None:
			return None

		if is_warmup:
			return None

		pullback_gap_pct = abs(close - ema_fast) / max(1e-12, ema_fast) * 100.0
		long_trend = ema_fast > ema_mid > ema_slow
		short_trend = ema_fast < ema_mid < ema_slow
		long_pullback = low <= ema_fast or pullback_gap_pct <= self.pullback_tolerance_pct
		short_pullback = high >= ema_fast or pullback_gap_pct <= self.pullback_tolerance_pct
		bullish_momentum = prev_close is not None and close > prev_close
		bearish_momentum = prev_close is not None and close < prev_close
		rsi_ok = (not self.use_rsi_filter) or (
			rsi_value is not None and self.rsi_min <= rsi_value <= self.rsi_max
		)
		volume_ok = (not self.use_volume_filter) or (
			avg_volume is not None and candle.quote_volume >= avg_volume * self.volume_mult
		)

		metadata = {
			"ema20": round(ema_fast, 8),
			"ema50": round(ema_mid, 8),
			"ema200": round(ema_slow, 8),
			"atr": round(atr_value, 8),
			"pullback_gap_pct": round(pullback_gap_pct, 6),
		}
		if rsi_value is not None:
			metadata["rsi"] = round(rsi_value, 4)

		long_setup = long_trend and long_pullback and bullish_momentum and rsi_ok and volume_ok
		short_setup = short_trend and short_pullback and bearish_momentum and rsi_ok and volume_ok

		if self.position_side == "LONG" and self.entry_price is not None:
			self.highest_since_entry = max(self.highest_since_entry or high, high)
			trail_candidate = (self.highest_since_entry or high) - (self.atr_trail_mult * atr_value)
			self.trailing_stop = max(self.trailing_stop or trail_candidate, trail_candidate)
			active_stop = max(self.initial_stop or trail_candidate, self.trailing_stop)

			metadata.update(
				{
					"entry_price": round(self.entry_price, 8),
					"highest_since_entry": round(self.highest_since_entry or high, 8),
					"initial_stop": round(self.initial_stop or active_stop, 8),
					"trailing_stop": round(self.trailing_stop, 8),
					"active_stop": round(active_stop, 8),
				}
			)

			if low <= active_stop:
				metadata["trade_condition"] = "TRAILING_STOP" if active_stop > (self.initial_stop or active_stop) else "STOP_LOSS"
				self._reset_position()
				return self.sell(close, "EXIT_LONG_STOP", metadata)

			if self.take_profit_r > 0 and self.risk_per_unit is not None:
				tp_price = self.entry_price + (self.risk_per_unit * self.take_profit_r)
				metadata["tp_price"] = round(tp_price, 8)
				if high >= tp_price:
					metadata["trade_condition"] = "TAKE_PROFIT"
					self._reset_position()
					return self.sell(close, "EXIT_LONG_TP", metadata)

			if close < ema_fast:
				metadata["trade_condition"] = "EXIT"
				self._reset_position()
				return self.sell(close, "EXIT_LONG_EMA20_LOST", metadata)

			if short_setup:
				metadata["trade_condition"] = "EXIT"
				self._reset_position()
				return self.sell(close, "EXIT_LONG_OPPOSITE_SIGNAL", metadata)

			return None

		if self.position_side == "SHORT" and self.entry_price is not None:
			self.lowest_since_entry = min(self.lowest_since_entry or low, low)
			trail_candidate = (self.lowest_since_entry or low) + (self.atr_trail_mult * atr_value)
			self.trailing_stop = min(self.trailing_stop or trail_candidate, trail_candidate)
			active_stop = min(self.initial_stop or trail_candidate, self.trailing_stop)

			metadata.update(
				{
					"entry_price": round(self.entry_price, 8),
					"lowest_since_entry": round(self.lowest_since_entry or low, 8),
					"initial_stop": round(self.initial_stop or active_stop, 8),
					"trailing_stop": round(self.trailing_stop, 8),
					"active_stop": round(active_stop, 8),
				}
			)

			if high >= active_stop:
				metadata["trade_condition"] = "TRAILING_STOP" if active_stop < (self.initial_stop or active_stop) else "STOP_LOSS"
				self._reset_position()
				return self.buy(close, "EXIT_SHORT_STOP", metadata)

			if self.take_profit_r > 0 and self.risk_per_unit is not None:
				tp_price = self.entry_price - (self.risk_per_unit * self.take_profit_r)
				metadata["tp_price"] = round(tp_price, 8)
				if low <= tp_price:
					metadata["trade_condition"] = "TAKE_PROFIT"
					self._reset_position()
					return self.buy(close, "EXIT_SHORT_TP", metadata)

			if close > ema_fast:
				metadata["trade_condition"] = "EXIT"
				self._reset_position()
				return self.buy(close, "EXIT_SHORT_EMA20_LOST", metadata)

			if long_setup:
				metadata["trade_condition"] = "EXIT"
				self._reset_position()
				return self.buy(close, "EXIT_SHORT_OPPOSITE_SIGNAL", metadata)

			return None

		if self.cooldown > 0:
			return None

		if long_setup:
			return self._enter_long(candle, atr_value, metadata)

		if short_setup:
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
		queue_name=section.get("queue_name", "KLINE_STRATEGY_BINANCE_TREND_FOLLOWING_ATR"),
		user_id=section.get("user_id", "paper-1"),
		exchange=section.get("exchange", "binance"),
		market_type=section.get("market_type", "futures"),
		position_side=section.get("position_side", "ONE_WAY"),
		source=section.get("source", "python-trend-following-atr"),
		strategy_id=section.get("strategy_id", "python-trend-following-atr"),
		is_paper_trading=section.get("is_paper_trading", True),
		order_type=section.get("order_type", "MARKET"),
		order_qty=float(section.get("order_qty", 1)),
		order_symbol=section.get("order_symbol", section.get("symbol", "SOL_USDT.P")),
		limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
	)


def build_strategy_config(section: dict) -> StrategyConfig:
	return StrategyConfig(
		name=section.get("name", "TREND_FOLLOWING_ATR"),
		symbol=section.get("symbol", "SOL_USDT.P"),
		interval=section.get("interval", "15m"),
		warmup_limit=int(section.get("historical_limit", 800)),
	)


async def run():
	runtime = build_runtime_config(SECTION)

	if runtime.order_qty <= 0:
		raise ValueError("order_qty must be > 0")

	strategy = TrendFollowingATRStrategy(build_strategy_config(SECTION), SECTION)

	if strategy.atr_stop_mult <= 0:
		raise ValueError("atr_stop_mult must be > 0")
	if strategy.atr_trail_mult <= 0:
		raise ValueError("atr_trail_mult must be > 0")

	runner = StrategyRunner(strategy=strategy, runtime=runtime)
	await runner.run()


if __name__ == "__main__":
	asyncio.run(run())
