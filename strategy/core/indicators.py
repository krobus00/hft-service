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
