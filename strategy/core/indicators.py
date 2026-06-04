from collections import deque
from typing import Optional, Tuple


class EMA:
    __slots__ = ("alpha", "value", "count")

    def __init__(self, period: int):
        self.alpha = 2.0 / (max(1, int(period)) + 1)
        self.value = 0.0
        self.count = 0

    def update(self, price: float) -> float:
        self.count += 1
        if self.count == 1:
            self.value = price
        else:
            self.value = self.alpha * price + (1.0 - self.alpha) * self.value
        return self.value


class RollingVWAP:
    __slots__ = ("window", "buf", "sum_pv", "sum_v")

    def __init__(self, window: int):
        self.window = max(2, int(window))
        self.buf = deque()
        self.sum_pv = 0.0
        self.sum_v = 0.0

    def update(self, price: float, volume: float) -> Optional[float]:
        v = max(0.0, float(volume))
        pv = price * v

        self.buf.append((pv, v))
        self.sum_pv += pv
        self.sum_v += v

        if len(self.buf) > self.window:
            old_pv, old_v = self.buf.popleft()
            self.sum_pv -= old_pv
            self.sum_v -= old_v

        if self.sum_v <= 0:
            return None
        return self.sum_pv / self.sum_v


class MACD:
    __slots__ = (
        "ema_fast",
        "ema_slow",
        "ema_signal",
        "prev_diff",
        "count",
        "crossed_up_active",
        "crossed_down_active",
    )

    def __init__(self, fast: int, slow: int, signal: int):
        self.ema_fast = EMA(fast)
        self.ema_slow = EMA(slow)
        self.ema_signal = EMA(signal)
        self.prev_diff: Optional[float] = None
        self.count = 0
        self.crossed_up_active = False
        self.crossed_down_active = False

    def update(self, price: float) -> Tuple[float, float, bool, bool]:
        self.count += 1
        fast = self.ema_fast.update(price)
        slow = self.ema_slow.update(price)
        macd_line = fast - slow
        signal_line = self.ema_signal.update(macd_line)
        diff = macd_line - signal_line

        if self.prev_diff is not None:
            if self.prev_diff <= 0 and diff > 0:
                self.crossed_up_active = True
                self.crossed_down_active = False
            elif self.prev_diff >= 0 and diff < 0:
                self.crossed_up_active = False
                self.crossed_down_active = True
        else:
            self.crossed_up_active = diff > 0
            self.crossed_down_active = diff < 0
        self.prev_diff = diff

        return macd_line, signal_line, self.crossed_up_active, self.crossed_down_active


class ATR:
    __slots__ = ("n", "prev_close", "buf", "sum")

    def __init__(self, n: int):
        self.n = max(1, int(n))
        self.prev_close: Optional[float] = None
        self.buf = deque()
        self.sum = 0.0

    def update(self, high: float, low: float, close: float) -> Optional[float]:
        if self.prev_close is None:
            tr = high - low
        else:
            tr = max(high - low, abs(high - self.prev_close), abs(low - self.prev_close))
        self.prev_close = close

        self.buf.append(tr)
        self.sum += tr

        if len(self.buf) > self.n:
            self.sum -= self.buf.popleft()

        if len(self.buf) < self.n:
            return None
        return self.sum / len(self.buf)


class AnchoredVWAP:
    __slots__ = ("anchor_ms", "cum_pv", "cum_v", "avwap_hist", "slope_lookback")

    def __init__(self, slope_lookback: int):
        self.anchor_ms: Optional[int] = None
        self.cum_pv = 0.0
        self.cum_v = 0.0
        self.avwap_hist = deque()
        self.slope_lookback = max(2, int(slope_lookback))

    def reset(self, anchor_ms: int) -> None:
        self.anchor_ms = anchor_ms
        self.cum_pv = 0.0
        self.cum_v = 0.0
        self.avwap_hist.clear()

    def update(self, close_time_ms: int, typical_price: float, quote_volume: float) -> Optional[float]:
        if self.anchor_ms is None:
            self.reset(close_time_ms)

        volume = max(0.0, float(quote_volume))
        if volume <= 0:
            return None

        self.cum_pv += typical_price * volume
        self.cum_v += volume

        if self.cum_v <= 0:
            return None

        avwap = self.cum_pv / self.cum_v
        self.avwap_hist.append(avwap)

        max_hist = max(200, self.slope_lookback + 5)
        if len(self.avwap_hist) > max_hist:
            self.avwap_hist.popleft()

        return avwap

    def slope_pct(self) -> Optional[float]:
        if len(self.avwap_hist) <= self.slope_lookback:
            return None

        old = self.avwap_hist[-self.slope_lookback - 1]
        latest = self.avwap_hist[-1]

        if old <= 0:
            return None
        return (latest - old) / old * 100.0


class BollingerBands:
    __slots__ = ("window", "std_mult", "buf", "sum", "sum_sq")

    def __init__(self, window: int, std_mult: float = 2.0):
        self.window = max(2, int(window))
        self.std_mult = float(std_mult)
        self.buf = deque()
        self.sum = 0.0
        self.sum_sq = 0.0

    def update(self, price: float) -> Optional[Tuple[float, float, float]]:
        px = float(price)
        self.buf.append(px)
        self.sum += px
        self.sum_sq += px * px

        if len(self.buf) > self.window:
            old = self.buf.popleft()
            self.sum -= old
            self.sum_sq -= old * old

        if len(self.buf) < self.window:
            return None

        n = float(len(self.buf))
        mean = self.sum / n
        variance = max((self.sum_sq / n) - (mean * mean), 0.0)
        std = variance ** 0.5
        upper = mean + (self.std_mult * std)
        lower = mean - (self.std_mult * std)
        return lower, mean, upper


class RSI:
    __slots__ = ("period", "prev_close", "avg_gain", "avg_loss", "count")

    def __init__(self, period: int):
        self.period = max(2, int(period))
        self.prev_close: Optional[float] = None
        self.avg_gain = 0.0
        self.avg_loss = 0.0
        self.count = 0

    def update(self, close: float) -> Optional[float]:
        px = float(close)
        if self.prev_close is None:
            self.prev_close = px
            return None

        delta = px - self.prev_close
        self.prev_close = px

        gain = max(delta, 0.0)
        loss = max(-delta, 0.0)

        self.count += 1
        if self.count <= self.period:
            scale = float(self.count)
            self.avg_gain = ((self.avg_gain * (scale - 1.0)) + gain) / scale
            self.avg_loss = ((self.avg_loss * (scale - 1.0)) + loss) / scale
            if self.count < self.period:
                return None
        else:
            period = float(self.period)
            self.avg_gain = ((self.avg_gain * (period - 1.0)) + gain) / period
            self.avg_loss = ((self.avg_loss * (period - 1.0)) + loss) / period

        if self.avg_loss == 0.0:
            return 100.0
        rs = self.avg_gain / self.avg_loss
        return 100.0 - (100.0 / (1.0 + rs))


class Stochastic:
    __slots__ = ("k_period", "d_period", "high_buf", "low_buf", "k_buf")

    def __init__(self, k_period: int, d_period: int = 3):
        self.k_period = max(2, int(k_period))
        self.d_period = max(1, int(d_period))
        self.high_buf = deque()
        self.low_buf = deque()
        self.k_buf = deque()

    def update(self, high: float, low: float, close: float) -> Tuple[Optional[float], Optional[float]]:
        hi = float(high)
        lo = float(low)
        px = float(close)

        self.high_buf.append(hi)
        self.low_buf.append(lo)

        if len(self.high_buf) > self.k_period:
            self.high_buf.popleft()
        if len(self.low_buf) > self.k_period:
            self.low_buf.popleft()

        if len(self.high_buf) < self.k_period or len(self.low_buf) < self.k_period:
            return None, None

        period_high = max(self.high_buf)
        period_low = min(self.low_buf)
        denom = period_high - period_low
        if denom <= 0:
            k = 50.0
        else:
            k = ((px - period_low) / denom) * 100.0

        self.k_buf.append(k)
        if len(self.k_buf) > self.d_period:
            self.k_buf.popleft()

        d = None
        if len(self.k_buf) == self.d_period:
            d = sum(self.k_buf) / float(self.d_period)

        return k, d
