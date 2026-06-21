import unittest
from unittest.mock import patch

from ai import AIHybridStrategy
from core.models import Candle, StrategyConfig


class AIIntervalTest(unittest.IsolatedAsyncioTestCase):
    async def test_queries_only_on_configured_interval(self):
        strategy = AIHybridStrategy(
            StrategyConfig("test", "BTC_USDT", "1m", 40),
            {"use_ai": False, "ai_interval_bars": 3, "ema_slow": 2, "macd_slow": 2, "macd_signal": 1},
        )
        candle = Candle(1, 100, 100, "BTC_USDT", "1m", 101, 99, 50, 1)

        for close_time in range(1, 51):
            candle.close_time_ms = close_time
            await strategy.on_closed_candle(candle, is_warmup=True)

        with patch.object(AIHybridStrategy, "_ai_signal", return_value=("HOLD", 1.0, "test", {})) as ai_signal:
            for close_time in range(51, 54):
                candle.close_time_ms = close_time
                await strategy.on_closed_candle(candle)

        self.assertEqual(ai_signal.call_count, 1)


if __name__ == "__main__":
    unittest.main()
