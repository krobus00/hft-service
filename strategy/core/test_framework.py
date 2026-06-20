import pickle
import threading
import unittest
from types import SimpleNamespace
from unittest.mock import patch

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

    async def ping(self):
        return True


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


class ClientStrategy(StrategyBase):
    __slots__ = ("ai_client", "_http_client", "cooldown")

    def __init__(self):
        super().__init__(StrategyConfig("test", "BTCUSDT", "1m", 1))
        self.ai_client = threading.RLock()
        self._http_client = threading.RLock()
        self.cooldown = 2

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

    def test_snapshot_excludes_runtime_clients(self):
        snapshot = ClientStrategy().snapshot_state()

        self.assertEqual(snapshot["cooldown"], 2)
        self.assertNotIn("ai_client", snapshot)
        self.assertNotIn("_http_client", snapshot)
        pickle.dumps(snapshot)


class RedisStateStoreTest(unittest.IsolatedAsyncioTestCase):
    async def test_connect_uses_binary_responses(self):
        client = FakeRedis()
        options = {}

        def from_url(url, **kwargs):
            options.update(kwargs)
            return client

        redis_module = SimpleNamespace(from_url=from_url)
        runtime = SimpleNamespace(
            redis_url="redis://localhost:6379/0",
            redis_key_prefix="test",
            redis_state_ttl_sec=0,
        )

        with patch("core.framework.redis_async", redis_module):
            store = await RedisStateStore.connect(runtime)

        self.assertIs(store.client, client)
        self.assertEqual(options, {"decode_responses": False})

    async def test_round_trip(self):
        store = RedisStateStore(FakeRedis(), "test", 0)
        expected = StoredPairState({"cooldown": 2}, 100.5, "order-1")

        await store.save("pair", expected)

        self.assertEqual(await store.load("pair"), expected)

    async def test_invalid_state_is_fatal(self):
        client = FakeRedis()
        store = RedisStateStore(client, "test", 0)
        client.values[store.key("pair")] = pickle.dumps([])

        with self.assertRaises(RuntimeError):
            await store.load("pair")


if __name__ == "__main__":
    unittest.main()
