import asyncio
import json
import os
from typing import Any, Dict, Optional, Tuple

import anthropic
import uvloop

from core.common import load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, EMA, MACD, RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
AI_CONFIG = CONFIG.get("ai", {})


class AIHybridStrategy(StrategyBase):
    __slots__ = (
        "ema_fast",
        "ema_slow",
        "macd",
        "vwap",
        "atr",
        "position_side",
        "entry_price",
        "highest_since_entry",
        "lowest_since_entry",
        "bars_in_pos",
        "cooldown",
        "bar_count",
        "ai_client",
        "base_url",
        "use_ai",
        "model_name",
        "ai_interval_bars",
        "entry_confidence",
        "override_confidence",
        "take_profit_pct",
        "stop_loss_pct",
        "trailing_stop_pct",
        "max_hold_bars",
        "cooldown_bars",
        "macd_ready_min",
    )

    def __init__(self, strategy_config: StrategyConfig, section: dict):
        super().__init__(strategy_config)

        ema_fast_period = int(section.get("ema_fast", 50))
        ema_slow_period = int(section.get("ema_slow", 200))
        macd_fast = int(section.get("macd_fast", 12))
        macd_slow = int(section.get("macd_slow", 26))
        macd_signal = int(section.get("macd_signal", 9))
        vwap_window = int(section.get("vwap_window", 120))
        atr_period = int(section.get("atr_n", 14))

        self.ema_fast = EMA(ema_fast_period)
        self.ema_slow = EMA(ema_slow_period)
        self.macd = MACD(macd_fast, macd_slow, macd_signal)
        self.vwap = RollingVWAP(vwap_window)
        self.atr = ATR(atr_period)

        self.position_side: Optional[str] = None
        self.entry_price: Optional[float] = None
        self.highest_since_entry: Optional[float] = None
        self.lowest_since_entry: Optional[float] = None
        self.bars_in_pos = 0
        self.cooldown = 0
        self.bar_count = 0

        self.use_ai = bool(section.get("use_ai", True))
        self.model_name = section.get("ai_model", "MiniMax-M2.7")
        self.ai_interval_bars = max(1, int(section.get("ai_interval_bars", 3)))
        self.entry_confidence = float(section.get("entry_confidence", 0.62))
        self.override_confidence = float(section.get("override_confidence", 0.75))

        self.take_profit_pct = float(section.get("take_profit_pct", 0.40))
        self.stop_loss_pct = float(section.get("stop_loss_pct", 0.25))
        self.trailing_stop_pct = float(section.get("trailing_stop_pct", 0.20))
        self.max_hold_bars = int(section.get("max_hold_bars", 24))
        self.cooldown_bars = int(section.get("cooldown_bars", 2))

        self.macd_ready_min = max(50, macd_slow + macd_signal)

        api_key = (
            section.get("anthropic_api_key")
            or GLOBAL_CONFIG.get("anthropic_api_key")
            or os.getenv("ANTHROPIC_API_KEY")
        )
        self.base_url = (
            section.get("anthropic_base_url")
            or GLOBAL_CONFIG.get("anthropic_base_url")
            or os.getenv("ANTHROPIC_BASE_URL")
        )
        client_kwargs = {"api_key": api_key}
        if self.base_url:
            client_kwargs["base_url"] = self.base_url

        self.ai_client = anthropic.Anthropic(**client_kwargs) if self.use_ai and api_key else None

    def _local_signal(self, candle: Candle, ema_fast: float, ema_slow: float, vwap_px: float, macd_hist: float, atr: float):
        trend_up = ema_fast > ema_slow and candle.close > ema_slow
        trend_down = ema_fast < ema_slow and candle.close < ema_slow
        vwap_gap_pct = (candle.close - vwap_px) / vwap_px * 100.0 if vwap_px > 0 else 0.0
        atr_pct = atr / candle.close * 100.0 if candle.close > 0 else 0.0

        long_score = 0.0
        short_score = 0.0

        if trend_up:
            long_score += 0.35
        if trend_down:
            short_score += 0.35

        if macd_hist > 0:
            long_score += min(0.25, abs(macd_hist) * 3.0)
        elif macd_hist < 0:
            short_score += min(0.25, abs(macd_hist) * 3.0)

        if vwap_gap_pct > 0:
            long_score += min(0.20, abs(vwap_gap_pct) / 200.0)
        elif vwap_gap_pct < 0:
            short_score += min(0.20, abs(vwap_gap_pct) / 200.0)

        if 0 < atr_pct < 1.8:
            long_score += 0.08
            short_score += 0.08

        long_score = min(1.0, long_score)
        short_score = min(1.0, short_score)

        if long_score > short_score and long_score >= self.entry_confidence:
            return "BUY", long_score
        if short_score > long_score and short_score >= self.entry_confidence:
            return "SELL", short_score
        return "HOLD", max(long_score, short_score)

    @staticmethod
    def _extract_text(message) -> str:
        texts = []
        for block in message.content or []:
            block_type = getattr(block, "type", None)
            block_text = getattr(block, "text", None)
            if isinstance(block, dict):
                block_type = block.get("type")
                block_text = block.get("text")
            if block_type == "text" and block_text:
                texts.append(str(block_text))
        return "\n".join(texts).strip()

    @staticmethod
    def _extract_block_types(message) -> str:
        block_types = []
        for block in message.content or []:
            block_type = getattr(block, "type", None)
            if isinstance(block, dict):
                block_type = block.get("type")
            block_types.append(str(block_type or "unknown"))
        return ",".join(block_types) if block_types else "none"

    @staticmethod
    def _extract_json(raw_text: str) -> Dict[str, Any]:
        text = raw_text.strip()
        if "```" in text:
            text = text.replace("```json", "").replace("```", "").strip()
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            text = text[start : end + 1]
        return json.loads(text)

    @staticmethod
    def _truncate_for_log(value: str, max_len: int = 1200) -> str:
        if len(value) <= max_len:
            return value
        return f"{value[:max_len]}...<truncated:{len(value) - max_len}>"

    def _ai_signal(self, payload: Dict[str, Any], local_action: str, local_confidence: float) -> Tuple[str, float, str]:
        if self.ai_client is None:
            raise RuntimeError("AI client is not initialized. Set anthropic_api_key or ANTHROPIC_API_KEY.")

        system_prompt = (
            "You are an advanced quantitative crypto trading assistant. "
            "action must be BUY, SELL, or HOLD. confidence must be between 0 and 1."
            "Return ONLY valid JSON. No markdown, no explanation. "
            'Example: {"action":"SELL","confidence":0.72,"reason":"trend down"}'
        )

        user_payload = {
            "symbol": self.config.symbol,
            "interval": self.config.interval,
            "features": payload,
            "local_signal": {
                "action": local_action,
                "confidence": round(local_confidence, 4),
            },
            "constraints": {
                "prefer_trend_following": True,
                "avoid_low_confidence_entries": True,
                "risk_first": True,
            },
        }

        user_payload_text = json.dumps(user_payload, separators=(",", ":"))
        print(
            (
                f"[AI_REQUEST] symbol={self.config.symbol} interval={self.config.interval} "
                f"model={self.model_name} local={local_action}:{local_confidence:.4f} "
                f"system_prompt={self._truncate_for_log(system_prompt)} "
                f"payload={self._truncate_for_log(user_payload_text)}"
            ),
            flush=True,
        )

        try:
            message = self.ai_client.messages.create(
                model=self.model_name,
                max_tokens=1200,
                system=system_prompt,
                messages=[
                    {
                        "role": "user",
                        "content": user_payload_text,
                    }
                ],
            )

            raw_text = self._extract_text(message)
            stop_reason = getattr(message, "stop_reason", None)
            block_types = self._extract_block_types(message)
            print(
                (
                    f"[AI_RESPONSE_RAW] symbol={self.config.symbol} interval={self.config.interval} "
                    f"model={self.model_name} stop_reason={stop_reason} block_types={block_types} "
                    f"response={self._truncate_for_log(raw_text)}"
                ),
                flush=True,
            )
            if not raw_text:
                raise RuntimeError(
                    f"No text block in response (stop_reason={stop_reason}, block_types={block_types})"
                )
            parsed = self._extract_json(raw_text)

            action = str(parsed.get("action", "HOLD")).upper()
            if action not in {"BUY", "SELL", "HOLD"}:
                action = "HOLD"

            confidence = float(parsed.get("confidence", local_confidence))
            confidence = max(0.0, min(1.0, confidence))

            reason = str(parsed.get("reason", "AI_DECISION"))[:160]
            print(
                (
                    f"[AI_RESPONSE_PARSED] symbol={self.config.symbol} interval={self.config.interval} "
                    f"model={self.model_name} action={action} confidence={confidence:.4f} "
                    f"reason={reason}"
                ),
                flush=True,
            )
            return action, confidence, reason
        except Exception as exc:
            print(
                (
                    f"[AI_ERROR] symbol={self.config.symbol} interval={self.config.interval} "
                    f"model={self.model_name} error={type(exc).__name__}:{exc}"
                ),
                flush=True,
            )
            raise RuntimeError(f"LLM request failed: {type(exc).__name__}:{exc}") from exc

    def _test_llm_on_init(self) -> None:
        if self.ai_client is None:
            raise RuntimeError("AI client is not initialized. Set anthropic_api_key or ANTHROPIC_API_KEY.")

        system_prompt = "You are a health check endpoint. Reply with plain text: OK"
        test_payload = {
            "symbol": self.config.symbol,
            "interval": self.config.interval,
            "check": "init_healthcheck",
        }
        test_payload_text = json.dumps(test_payload, separators=(",", ":"))

        print(
            (
                f"[AI_INIT_TEST_REQUEST] symbol={self.config.symbol} interval={self.config.interval} "
                f"model={self.model_name} payload={self._truncate_for_log(test_payload_text)}"
            ),
            flush=True,
        )

        try:
            message = self.ai_client.messages.create(
                model=self.model_name,
                max_tokens=64,
                system=system_prompt,
                messages=[
                    {
                        "role": "user",
                        "content": test_payload_text,
                    }
                ],
            )
            raw_text = self._extract_text(message)
            if not raw_text:
                raise RuntimeError("empty response from LLM init test")

            print(
                (
                    f"[AI_INIT_TEST_RESPONSE] symbol={self.config.symbol} interval={self.config.interval} "
                    f"model={self.model_name} response={self._truncate_for_log(raw_text)}"
                ),
                flush=True,
            )
        except Exception as exc:
            print(
                (
                    f"[AI_INIT_TEST_ERROR] symbol={self.config.symbol} interval={self.config.interval} "
                    f"model={self.model_name} error={type(exc).__name__}:{exc}"
                ),
                flush=True,
            )
            raise RuntimeError(f"LLM init test failed: {type(exc).__name__}:{exc}") from exc

    def _position_risk_exit(self, candle: Candle, metadata: Dict[str, Any]):
        if self.position_side == "LONG" and self.entry_price is not None:
            self.bars_in_pos += 1
            self.highest_since_entry = max(self.highest_since_entry or candle.close, candle.close)

            tp_px = self.entry_price * (1.0 + pct_to_frac(self.take_profit_pct))
            sl_px = self.entry_price * (1.0 - pct_to_frac(self.stop_loss_pct))
            trail_px = (self.highest_since_entry or candle.close) * (1.0 - pct_to_frac(self.trailing_stop_pct))

            if candle.close >= tp_px:
                return self._close_long(candle.close, "TP_LONG", metadata)
            if candle.close <= sl_px:
                return self._close_long(candle.close, "SL_LONG", metadata)
            if candle.close <= trail_px:
                return self._close_long(candle.close, "TRAIL_LONG", metadata)
            if self.bars_in_pos >= self.max_hold_bars:
                return self._close_long(candle.close, "TIME_LONG", metadata)

        if self.position_side == "SHORT" and self.entry_price is not None:
            self.bars_in_pos += 1
            self.lowest_since_entry = min(self.lowest_since_entry or candle.close, candle.close)

            tp_px = self.entry_price * (1.0 - pct_to_frac(self.take_profit_pct))
            sl_px = self.entry_price * (1.0 + pct_to_frac(self.stop_loss_pct))
            trail_px = (self.lowest_since_entry or candle.close) * (1.0 + pct_to_frac(self.trailing_stop_pct))

            if candle.close <= tp_px:
                return self._close_short(candle.close, "TP_SHORT", metadata)
            if candle.close >= sl_px:
                return self._close_short(candle.close, "SL_SHORT", metadata)
            if candle.close >= trail_px:
                return self._close_short(candle.close, "TRAIL_SHORT", metadata)
            if self.bars_in_pos >= self.max_hold_bars:
                return self._close_short(candle.close, "TIME_SHORT", metadata)

        return None

    def _close_long(self, price: float, reason: str, metadata: Dict[str, Any]):
        self.position_side = None
        self.entry_price = None
        self.highest_since_entry = None
        self.lowest_since_entry = None
        self.bars_in_pos = 0
        self.cooldown = self.cooldown_bars
        return self.sell(price, reason, metadata)

    def _close_short(self, price: float, reason: str, metadata: Dict[str, Any]):
        self.position_side = None
        self.entry_price = None
        self.highest_since_entry = None
        self.lowest_since_entry = None
        self.bars_in_pos = 0
        self.cooldown = self.cooldown_bars
        return self.buy(price, reason, metadata)

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        self.bar_count += 1
        if self.cooldown > 0:
            self.cooldown -= 1

        ema_fast = self.ema_fast.update(candle.close)
        ema_slow = self.ema_slow.update(candle.close)
        macd_line, signal_line, _, _ = self.macd.update(candle.close)
        macd_hist = macd_line - signal_line
        vwap_px = self.vwap.update(candle.close, candle.quote_volume)

        high = candle.high if candle.high is not None else candle.close
        low = candle.low if candle.low is not None else candle.close
        atr = self.atr.update(high, low, candle.close)

        if self.macd.count < self.macd_ready_min or vwap_px is None or atr is None:
            return None

        if is_warmup:
            return None

        local_action, local_confidence = self._local_signal(candle, ema_fast, ema_slow, vwap_px, macd_hist, atr)

        features = {
            "close": round(candle.close, 8),
            "ema_fast": round(ema_fast, 8),
            "ema_slow": round(ema_slow, 8),
            "vwap": round(vwap_px, 8),
            "macd": round(macd_line, 8),
            "signal": round(signal_line, 8),
            "macd_hist": round(macd_hist, 8),
            "atr": round(atr, 8),
            "quote_volume": round(candle.quote_volume, 8),
            "taker_quote_volume": round(candle.taker_quote_volume, 8),
            "trade_count": int(candle.trade_count),
        }

        ai_action, ai_confidence, ai_reason = self._ai_signal(features, local_action, local_confidence)

        final_action = ai_action
        final_confidence = ai_confidence

        print(
            (
                f"[AI_DECISION] symbol={self.config.symbol} interval={self.config.interval} "
                f"close={candle.close:.6f} local={local_action}:{local_confidence:.4f} "
                f"ai={ai_action}:{ai_confidence:.4f} final={final_action}:{final_confidence:.4f} "
                f"queried=True reason={ai_reason}"
            ),
            flush=True,
        )

        metadata = {
            "local_action": local_action,
            "local_confidence": round(local_confidence, 4),
            "ai_action": ai_action,
            "ai_confidence": round(ai_confidence, 4),
            "ai_reason": ai_reason,
            "ema_fast": round(ema_fast, 8),
            "ema_slow": round(ema_slow, 8),
            "vwap": round(vwap_px, 8),
            "macd_hist": round(macd_hist, 8),
            "atr": round(atr, 8),
        }

        risk_exit_signal = self._position_risk_exit(candle, metadata)
        if risk_exit_signal is not None:
            return risk_exit_signal

        if self.cooldown > 0:
            return None

        if self.position_side is None and final_confidence >= self.entry_confidence:
            if final_action == "BUY":
                self.position_side = "LONG"
                self.entry_price = candle.close
                self.highest_since_entry = candle.close
                self.lowest_since_entry = candle.close
                self.bars_in_pos = 0
                self.cooldown = self.cooldown_bars
                return self.buy(candle.close, "ENTER_LONG_AI", metadata)

            if final_action == "SELL":
                self.position_side = "SHORT"
                self.entry_price = candle.close
                self.highest_since_entry = candle.close
                self.lowest_since_entry = candle.close
                self.bars_in_pos = 0
                self.cooldown = self.cooldown_bars
                return self.sell(candle.close, "ENTER_SHORT_AI", metadata)

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
        order_subject=section.get("order_subject", "order_engine.place_order"),
        queue_name=section.get("queue_name", "KLINE_STRATEGY_TOKOCRYPTO_AI"),
        user_id=section.get("user_id", "paper-1"),
        exchange=section.get("exchange", "tokocrypto"),
        market_type=section.get("market_type", "spot"),
        position_side=section.get("position_side", "BOTH"),
        source=section.get("source", "python-ai-minimax-m2-7"),
        strategy_id=section.get("strategy_id", "python-ai-minimax-m2-7-hybrid"),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        order_symbol=section.get("order_symbol", section.get("symbol", "SOLUSDT")),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "AI-M2.7"),
        symbol=section.get("symbol", "SOLUSDT"),
        interval=section.get("interval", "1m"),
        warmup_limit=int(section.get("historical_limit", 800)),
    )


async def run():
    runtime = build_runtime_config(AI_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = AIHybridStrategy(build_strategy_config(AI_CONFIG), AI_CONFIG)
    if strategy.ai_client is None:
        raise ValueError("LLM is required. Set anthropic_api_key or ANTHROPIC_API_KEY.")
    strategy._test_llm_on_init()

    if strategy.take_profit_pct < 0:
        raise ValueError("take_profit_pct must be >= 0")
    if strategy.stop_loss_pct < 0:
        raise ValueError("stop_loss_pct must be >= 0")
    if strategy.trailing_stop_pct < 0:
        raise ValueError("trailing_stop_pct must be >= 0")

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    await runner.run()


if __name__ == "__main__":
    asyncio.run(run())
