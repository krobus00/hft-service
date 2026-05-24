import asyncio
from collections import deque
from typing import Optional

import uvloop

from core.common import cfg_value, load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import EMA, RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
KROBOT02_CONFIG = CONFIG.get("krobot02", {})


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


class Krobot02VWAPVolumeStrategy(StrategyBase):
	__slots__ = (
		"ema",
		"vwap",
		"vol_mean",
		"cooldown",
		"pause_bars",
		"stop_loss_streak",
		"position_side",
		"entry_price",
		"highest_since_entry",
		"lowest_since_entry",
		"ema_period",
		"vwap_window",
		"volume_window",
		"volume_mult",
		"cooldown_bars",
		"sl_cooldown_bars",
		"max_consecutive_stop_losses",
		"sl_pause_bars",
		"take_profit_pct",
		"stop_loss_pct",
		"trailing_stop_pct",
		"warmup_min",
	)

	def __init__(self, strategy_config: StrategyConfig, section: dict):
		super().__init__(strategy_config)

		self.ema_period = int(section.get("ema_period", 200))
		self.vwap_window = int(section.get("vwap_window", 100))
		self.volume_window = int(section.get("volume_window", 20))

		self.ema = EMA(self.ema_period)
		self.vwap = RollingVWAP(self.vwap_window)
		self.vol_mean = RollingMean(self.volume_window)

		self.cooldown = 0
		self.pause_bars = 0
		self.stop_loss_streak = 0
		self.position_side: Optional[str] = None
		self.entry_price: Optional[float] = None
		self.highest_since_entry: Optional[float] = None
		self.lowest_since_entry: Optional[float] = None

		self.volume_mult = float(section.get("volume_mult", 1.15))
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
		self.take_profit_pct = float(cfg_value(section, GLOBAL_CONFIG, "take_profit_pct", section.get("tp_pct", 0.4)))
		self.stop_loss_pct = float(cfg_value(section, GLOBAL_CONFIG, "stop_loss_pct", section.get("sl_pct", 0.25)))
		self.trailing_stop_pct = float(
			cfg_value(section, GLOBAL_CONFIG, "trailing_stop_pct", section.get("ts_pct", 0.0))
		)

		self.warmup_min = max(self.ema_period, self.vwap_window, self.volume_window)

	def _reset_position(self, exit_type: str = "") -> None:
		self.position_side = None
		self.entry_price = None
		self.highest_since_entry = None
		self.lowest_since_entry = None

		normalized_exit = str(exit_type or "").strip().upper()
		if normalized_exit == "STOP_LOSS":
			self.stop_loss_streak += 1
			self.cooldown = max(self.cooldown_bars, self.sl_cooldown_bars)
			if (
				self.max_consecutive_stop_losses > 0
				and self.stop_loss_streak >= self.max_consecutive_stop_losses
				and self.sl_pause_bars > 0
			):
				self.pause_bars = max(self.pause_bars, self.sl_pause_bars)
				self.stop_loss_streak = 0
			return

		self.cooldown = self.cooldown_bars
		self.stop_loss_streak = 0

	def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
		if not self.allow_new_candle(candle):
			return None

		if self.pause_bars > 0:
			self.pause_bars -= 1
			return None

		if self.cooldown > 0:
			self.cooldown -= 1
			return None

		ema_px = self.ema.update(candle.close)
		vwap_px = self.vwap.update(candle.close, candle.quote_volume)
		vol_avg = self.vol_mean.update(candle.quote_volume)

		if self.ema.count < self.warmup_min or vwap_px is None or vol_avg is None:
			return None

		if is_warmup:
			return None

		high_px = candle.high if candle.high is not None else candle.close
		low_px = candle.low if candle.low is not None else candle.close
		vol_ok = candle.quote_volume >= (vol_avg * self.volume_mult)

		metadata = {
			"ema": round(ema_px, 8),
			"vwap": round(vwap_px, 8),
			"volume": round(candle.quote_volume, 8),
			"volume_avg": round(vol_avg, 8),
			"volume_mult": round(self.volume_mult, 8),
		}

		if self.position_side == "LONG" and self.entry_price is not None:
			self.highest_since_entry = max(self.highest_since_entry or high_px, high_px)
			sl_px = self.entry_price * (1.0 - pct_to_frac(self.stop_loss_pct))
			tp_px = self.entry_price * (1.0 + pct_to_frac(self.take_profit_pct))
			trail_px = (self.highest_since_entry or high_px) * (1.0 - pct_to_frac(self.trailing_stop_pct))

			metadata.update(
				{
					"entry_price": round(self.entry_price, 8),
					"tp_px": round(tp_px, 8),
					"sl_px": round(sl_px, 8),
					"trail_px": round(trail_px, 8),
				}
			)

			if self.stop_loss_pct > 0 and low_px <= sl_px:
				metadata["trade_condition"] = "STOP_LOSS"
				metadata["order_reason"] = "STOP_LOSS_LONG"
				metadata["exit_type"] = "STOP_LOSS"
				self._reset_position("STOP_LOSS")
				return self.sell(candle.close, "STOP_LOSS_LONG", metadata)

			if self.take_profit_pct > 0 and high_px >= tp_px:
				metadata["trade_condition"] = "TAKE_PROFIT"
				metadata["order_reason"] = "TAKE_PROFIT_LONG"
				metadata["exit_type"] = "TAKE_PROFIT"
				self._reset_position("TAKE_PROFIT")
				return self.sell(candle.close, "TAKE_PROFIT_LONG", metadata)

			if self.trailing_stop_pct > 0 and low_px <= trail_px:
				metadata["trade_condition"] = "TRAILING_STOP"
				metadata["order_reason"] = "TRAILING_STOP_LONG"
				metadata["exit_type"] = "TRAILING_STOP"
				self._reset_position("TRAILING_STOP")
				return self.sell(candle.close, "TRAILING_STOP_LONG", metadata)

		if self.position_side == "SHORT" and self.entry_price is not None:
			self.lowest_since_entry = min(self.lowest_since_entry or low_px, low_px)
			sl_px = self.entry_price * (1.0 + pct_to_frac(self.stop_loss_pct))
			tp_px = self.entry_price * (1.0 - pct_to_frac(self.take_profit_pct))
			trail_px = (self.lowest_since_entry or low_px) * (1.0 + pct_to_frac(self.trailing_stop_pct))

			metadata.update(
				{
					"entry_price": round(self.entry_price, 8),
					"tp_px": round(tp_px, 8),
					"sl_px": round(sl_px, 8),
					"trail_px": round(trail_px, 8),
				}
			)

			if self.stop_loss_pct > 0 and high_px >= sl_px:
				metadata["trade_condition"] = "STOP_LOSS"
				metadata["order_reason"] = "STOP_LOSS_SHORT"
				metadata["exit_type"] = "STOP_LOSS"
				self._reset_position("STOP_LOSS")
				return self.buy(candle.close, "STOP_LOSS_SHORT", metadata)

			if self.take_profit_pct > 0 and low_px <= tp_px:
				metadata["trade_condition"] = "TAKE_PROFIT"
				metadata["order_reason"] = "TAKE_PROFIT_SHORT"
				metadata["exit_type"] = "TAKE_PROFIT"
				self._reset_position("TAKE_PROFIT")
				return self.buy(candle.close, "TAKE_PROFIT_SHORT", metadata)

			if self.trailing_stop_pct > 0 and high_px >= trail_px:
				metadata["trade_condition"] = "TRAILING_STOP"
				metadata["order_reason"] = "TRAILING_STOP_SHORT"
				metadata["exit_type"] = "TRAILING_STOP"
				self._reset_position("TRAILING_STOP")
				return self.buy(candle.close, "TRAILING_STOP_SHORT", metadata)

		if self.cooldown > 0:
			return None

		long_cond = candle.close > vwap_px and candle.close > ema_px and vol_ok
		short_cond = candle.close < vwap_px and candle.close < ema_px and vol_ok

		if long_cond:
			if self.position_side == "LONG":
				return None
			self.position_side = "LONG"
			self.entry_price = candle.close
			self.highest_since_entry = high_px
			self.lowest_since_entry = low_px
			self.cooldown = self.cooldown_bars
			metadata["trade_condition"] = "ENTRY"
			metadata["order_reason"] = "ENTER_LONG_VWAP_VOLUME"
			metadata["exit_type"] = ""
			return self.buy(candle.close, "ENTER_LONG_VWAP_VOLUME", metadata)

		if short_cond:
			if self.position_side == "SHORT":
				return None
			self.position_side = "SHORT"
			self.entry_price = candle.close
			self.highest_since_entry = high_px
			self.lowest_since_entry = low_px
			self.cooldown = self.cooldown_bars
			metadata["trade_condition"] = "ENTRY"
			metadata["order_reason"] = "ENTER_SHORT_VWAP_VOLUME"
			metadata["exit_type"] = ""
			return self.sell(candle.close, "ENTER_SHORT_VWAP_VOLUME", metadata)

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
		source=section.get("source", "python-krobot02"),
		strategy_id=section.get("strategy_id", "python-krobot02-vwap-volume"),
		need_notification=bool(section.get("need_notification", False)),
		is_paper_trading=section.get("is_paper_trading", True),
		order_type=section.get("order_type", "LIMIT"),
		order_qty=float(section.get("order_qty", 10)),
		limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
		enable_intrabar_risk_exit=bool(cfg_value(section, GLOBAL_CONFIG, "enable_intrabar_risk_exit", False)),
	)


def build_strategy_config(section: dict) -> StrategyConfig:
	return StrategyConfig(
		name=section.get("name", "KROBOT02_VWAP_VOLUME"),
		symbol="*",
		interval="*",
		warmup_limit=int(section.get("historical_limit", 800)),
	)


async def run():
	runtime = build_runtime_config(KROBOT02_CONFIG)

	if runtime.order_qty <= 0:
		raise ValueError("order_qty must be > 0")

	strategy = Krobot02VWAPVolumeStrategy(build_strategy_config(KROBOT02_CONFIG), KROBOT02_CONFIG)

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

	if strategy.volume_mult <= 0:
		raise ValueError("volume_mult must be > 0")

	runner = StrategyRunner(strategy=strategy, runtime=runtime)
	await runner.run()


if __name__ == "__main__":
	asyncio.run(run())
