import asyncio
from statistics import median
from typing import Optional

import uvloop

from core.common import load_full_config
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, AnchoredVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
AVWAP_CONFIG = CONFIG.get("anchored_vwap", {})


def percentile(xs, p: float) -> float:
    if not xs:
        return 0.0
    ys = sorted(xs)
    k = int(round((len(ys) - 1) * p))
    return float(ys[max(0, min(len(ys) - 1, k))])


class AnchoredVWAPStrategy(StrategyBase):
    __slots__ = (
        "vwap",
        "atr",
        "prev_close",
        "pos",
        "cooldown",
        "buy_ok",
        "entry_armed",
        "entry_arm_left",
        "vol_buf",
        "trades_buf",
        "last_anchor_reset_ms",
        "atr_k",
        "min_band_pct",
        "slope_min_pct",
        "vol_med_n",
        "vol_shock_x",
        "range_min_pct",
        "anchor_reset_cooldown_ms",
        "participation_n",
        "min_trades_pctl",
        "taker_ratio_long_min",
        "confirm_bars",
        "cooldown_bars",
        "entry_arm_bars",
        "long_only",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        slope_lookback = int(section.get("slope_lookback", 20))

        self.vwap = AnchoredVWAP(slope_lookback=slope_lookback)
        self.atr = ATR(int(section.get("atr_n", 14)))
        self.prev_close: Optional[float] = None

        self.pos = "FLAT"
        self.cooldown = 0
        self.buy_ok = 0
        self.entry_armed = False
        self.entry_arm_left = 0

        self.vol_buf = []
        self.trades_buf = []
        self.last_anchor_reset_ms = 0

        self.atr_k = float(section.get("atr_k", 1.2))
        self.min_band_pct = float(section.get("min_band_pct", 0.0008))
        self.slope_min_pct = float(section.get("slope_min_pct", section.get("slope_min_bps", 8) / 100.0))
        self.vol_med_n = int(section.get("vol_med_n", 60))
        self.vol_shock_x = float(section.get("vol_shock_x", 2.5))
        self.range_min_pct = float(section.get("range_min_pct", section.get("range_min_bps", 15) / 100.0))
        self.anchor_reset_cooldown_ms = int(section.get("anchor_reset_cooldown_ms", 10 * 60_000))
        self.participation_n = int(section.get("participation_n", 120))
        self.min_trades_pctl = float(section.get("min_trades_pctl", 0.30))
        self.taker_ratio_long_min = float(section.get("taker_ratio_long_min", 0.52))
        self.confirm_bars = int(section.get("confirm_bars", 2))
        self.cooldown_bars = int(section.get("cooldown_bars", 3))
        self.entry_arm_bars = int(section.get("entry_arm_bars", 3))
        self.long_only = bool(section.get("long_only", True))

    def snapshot_state(self):
        return (
            self.last_close_time_ms,
            self.prev_close,
            self.pos,
            self.cooldown,
            self.buy_ok,
            self.entry_armed,
            self.entry_arm_left,
            self.last_anchor_reset_ms,
            list(self.vol_buf),
            list(self.trades_buf),
            self.vwap.anchor_ms,
            self.vwap.cum_pv,
            self.vwap.cum_v,
            list(self.vwap.avwap_hist),
            self.atr.prev_close,
            list(self.atr.buf),
            self.atr.sum,
        )

    def restore_state(self, snapshot) -> None:
        (
            self.last_close_time_ms,
            self.prev_close,
            self.pos,
            self.cooldown,
            self.buy_ok,
            self.entry_armed,
            self.entry_arm_left,
            self.last_anchor_reset_ms,
            vol_buf,
            trades_buf,
            self.vwap.anchor_ms,
            self.vwap.cum_pv,
            self.vwap.cum_v,
            avwap_hist,
            self.atr.prev_close,
            atr_buf,
            self.atr.sum,
        ) = snapshot

        self.vol_buf = vol_buf
        self.trades_buf = trades_buf

        self.vwap.avwap_hist.clear()
        self.vwap.avwap_hist.extend(avwap_hist)

        self.atr.buf.clear()
        self.atr.buf.extend(atr_buf)

    def _update_hist(self, candle: Candle) -> None:
        self.vol_buf.append(candle.quote_volume)
        if len(self.vol_buf) > self.vol_med_n:
            self.vol_buf.pop(0)

        self.trades_buf.append(candle.trade_count)
        if len(self.trades_buf) > self.participation_n:
            self.trades_buf.pop(0)

    def _should_reset_anchor(self, candle: Candle) -> bool:
        if self.pos == "LONG":
            return False

        if candle.close_time_ms - self.last_anchor_reset_ms < self.anchor_reset_cooldown_ms:
            return False

        med_vol = median(self.vol_buf) if self.vol_buf else 0.0
        if med_vol <= 0:
            return False

        range_pct = 0.0
        if candle.high is not None and candle.low is not None and candle.close > 0:
            range_pct = (candle.high - candle.low) / candle.close * 100.0

        return candle.quote_volume >= med_vol * self.vol_shock_x and range_pct >= self.range_min_pct

    def _participation_ok(self, candle: Candle) -> bool:
        if len(self.trades_buf) < 30:
            return True
        threshold = percentile(self.trades_buf, self.min_trades_pctl)
        return candle.trade_count >= threshold

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        if self.cooldown > 0:
            self.cooldown -= 1

        self._update_hist(candle)

        if self._should_reset_anchor(candle):
            self.vwap.reset(candle.close_time_ms)
            self.last_anchor_reset_ms = candle.close_time_ms

        high = candle.high if candle.high is not None else candle.close
        low = candle.low if candle.low is not None else candle.close
        typical = (high + low + candle.close) / 3.0

        avwap = self.vwap.update(candle.close_time_ms, typical, candle.quote_volume)
        atr = self.atr.update(high, low, candle.close)

        if self.prev_close is None:
            self.prev_close = candle.close
            return None

        if avwap is not None and self.pos == "LONG":
            crossed_down = self.prev_close >= avwap and candle.close < avwap
            if crossed_down:
                self.pos = "FLAT"
                self.cooldown = self.cooldown_bars
                self.buy_ok = 0
                self.entry_armed = False
                self.entry_arm_left = 0
                self.prev_close = candle.close
                if is_warmup:
                    return None
                return self.sell(
                    candle.close,
                    "EXIT_LONG",
                    {
                        "avwap": round(avwap, 8),
                        "taker_ratio": round(candle.taker_quote_volume / candle.quote_volume, 8)
                        if candle.quote_volume > 0
                        else 0.0,
                    },
                )

        if avwap is None or atr is None:
            self.prev_close = candle.close
            return None

        slope = self.vwap.slope_pct()
        if slope is None or slope < self.slope_min_pct:
            self.buy_ok = 0
            self.entry_armed = False
            self.entry_arm_left = 0
            self.prev_close = candle.close
            return None

        if not self._participation_ok(candle):
            self.prev_close = candle.close
            return None

        band = max(self.atr_k * atr, self.min_band_pct * avwap)
        upper = avwap + band

        crossed_up = self.prev_close <= avwap and candle.close > avwap

        if crossed_up:
            self.entry_armed = True
            self.entry_arm_left = self.entry_arm_bars
        elif self.entry_arm_left > 0:
            self.entry_arm_left -= 1
        else:
            self.entry_armed = False

        taker_ratio = candle.taker_quote_volume / candle.quote_volume if candle.quote_volume > 0 else 0.0
        buy_candidate = candle.close >= upper and taker_ratio >= self.taker_ratio_long_min
        self.buy_ok = self.buy_ok + 1 if buy_candidate else 0

        self.prev_close = candle.close

        if is_warmup:
            return None

        if self.cooldown == 0 and self.pos == "FLAT" and self.entry_armed and self.buy_ok >= self.confirm_bars:
            self.pos = "LONG"
            self.cooldown = self.cooldown_bars
            self.buy_ok = 0
            self.entry_armed = False
            self.entry_arm_left = 0

            return self.buy(
                candle.close,
                "ENTER_LONG",
                {
                    "avwap": round(avwap, 8),
                    "upper": round(upper, 8),
                    "slope_pct": round(slope, 8),
                    "taker_ratio": round(taker_ratio, 8),
                },
            )

        if not self.long_only:
            return None

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
        order_subject=section.get("order_subject", section.get("place_subject", "order_engine.place_order")),
        queue_name=section.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_AVWAP_IMPROVED_LONGONLY"),
        user_id=section.get("user_id", "paper-1"),
        exchange=section.get("exchange", "tokocrypto"),
        market_type=section.get("market_type", "spot"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-avwap"),
        strategy_id=section.get("strategy_id", "python-avwap-improved-longonly-v1"),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        order_symbol=section.get("symbol_out", section.get("symbol", "SOLUSDT")),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 3) / 100.0)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name="AVWAP+",
        symbol=section.get("symbol_in", section.get("symbol", "SOLUSDT")),
        interval=section.get("interval", "1m"),
        warmup_limit=int(section.get("historical_limit", 500)),
    )


async def run():
    runtime = build_runtime_config(AVWAP_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = AnchoredVWAPStrategy(build_strategy_config(AVWAP_CONFIG), AVWAP_CONFIG)

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
