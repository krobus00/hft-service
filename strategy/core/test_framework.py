import unittest

from core.framework import RedisStateStore, StoredPairState, StrategyBase
from core.models import StrategyConfig


class FakeRedis:
    def __init__(self):
        self.values = {}

    async def get(self, key):
        return self.values.get(key)

    async def set(self, key, value, **kwargs):
        self.values[key] = value

    async def delete(self, key):
        self.values.pop(key, None)


class ExternalCloseStrategy(StrategyBase):
    __slots__ = ("states", "position_side", "entry_price", "cooldown", "cooldown_bars")

    def __init__(self):
        super().__init__(StrategyConfig("test", "BTCUSDT", "1m", 1))
        self.states = {"BTCUSDT": {"position_side": "LONG", "entry_price": 100, "trail_armed": True, "reentry_lock_side": None}}
        self.position_side = "LONG"
        self.entry_price = 100
        self.cooldown = 0
        self.cooldown_bars = 2

    def on_closed_candle(self, candle, is_warmup=False):
        return None


class ExternalCloseTest(unittest.TestCase):
    def test_clears_direct_and_per_symbol_position_state(self):
        strategy = ExternalCloseStrategy()
        strategy.on_external_close()

        self.assertIsNone(strategy.position_side)
        self.assertIsNone(strategy.entry_price)
        self.assertEqual(strategy.cooldown, 2)
        self.assertIsNone(strategy.states["BTCUSDT"]["position_side"])
        self.assertIsNone(strategy.states["BTCUSDT"]["entry_price"])
        self.assertFalse(strategy.states["BTCUSDT"]["trail_armed"])
        self.assertEqual(strategy.states["BTCUSDT"]["reentry_lock_side"], "LONG")


class RedisStateStoreTest(unittest.IsolatedAsyncioTestCase):
    async def test_round_trip(self):
        store = RedisStateStore(FakeRedis(), "test", 0)
        expected = StoredPairState({"cooldown": 2}, 100.5, "order-1")

        await store.save("pair", expected)

        self.assertEqual(await store.load("pair"), expected)


if __name__ == "__main__":
    unittest.main()
