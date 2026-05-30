import asyncio
from collections import deque
from contextlib import suppress
import json
import os
from typing import Any, Dict, Optional, Tuple

import httpx
from openai import OpenAI
import uvloop

from core.common import cfg_value, load_full_config, pct_to_frac
from core.framework import StrategyBase, StrategyRunner
from core.indicators import ATR, EMA, MACD, RollingVWAP
from core.models import Candle, RuntimeConfig, StrategyConfig

asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

CONFIG = load_full_config()
GLOBAL_CONFIG = CONFIG.get("global", {})
AI_CONFIG = CONFIG.get("ai", {})

AI_SYSTEM_PROMPT = (
    "You are an advanced quantitative crypto trading assistant.\n\n"
    "Make a trading decision based ONLY on the provided data.\n\n"
    "Kline feature mapping (use this grouping when reasoning):\n"
    "- momentum: rsi, macd.line, macd.signal, macd.hist\n"
    "- volatility: atr, atr_pct\n"
    "- price_action: o, h, l, c, v, vwap.value, vwap.gap_pct, distance_from_ema\n\n"
    "--------------------------------------------------\n"
    "DECISION FRAMEWORK (STRICT ORDER)\n"
    "--------------------------------------------------\n"
    "1. Trend alignment (EMA fast vs slow)\n"
    "2. Momentum confirmation (MACD hist direction + RSI regime)\n"
    "3. Price location (VWAP + distance_from_ema)\n"
    "4. Volatility filter (ATR%)\n"
    "5. Risk-reward vs risk_rules\n\n"
    "If any of (1-3) are not aligned -> prefer HOLD\n\n"
    "--------------------------------------------------\n"
    "ENTRY RULES (ALL MUST BE TRUE)\n"
    "--------------------------------------------------\n"
    "- Trend and momentum aligned\n"
    "- MACD histogram expanding in trade direction\n"
    "- RSI > 55 for BUY, RSI < 45 for SELL\n"
    "- distance_from_ema within +/-0.3% (not overextended)\n"
    "- Risk-reward >= 1:2 based on risk_rules\n\n"
    "--------------------------------------------------\n"
    "NO-TRADE ZONE (CHOP FILTER)\n"
    "--------------------------------------------------\n"
    "- RSI between 45-55 AND\n"
    "- |MACD hist| < 0.02\n"
    "-> MUST return HOLD\n\n"
    "--------------------------------------------------\n"
    "OVEREXTENSION RULE (STRICT)\n"
    "--------------------------------------------------\n"
    "- distance_from_ema > 0.5% -> overbought -> no BUY\n"
    "- distance_from_ema < -0.5% -> oversold -> no SELL\n\n"
    "--------------------------------------------------\n"
    "EXIT RULES (WHEN IN POSITION)\n"
    "--------------------------------------------------\n"
    "- MACD histogram reverses against position\n"
    "- RSI crosses 50 against position\n"
    "- Price crosses VWAP against position\n"
    "- Trailing stop hit\n\n"
    "If already in position:\n"
    "- Decide HOLD or EXIT\n"
    "- SELL exits BUY/LONG\n"
    "- BUY exits SELL/SHORT\n\n"
    "If TP already reached:\n"
    "- If confidence remains high AND trend still valid:\n"
    "  -> keep position\n"
    "  -> move SL to previous TP\n"
    "  -> set next TP = previous TP + TP %\n\n"
    "--------------------------------------------------\n"
    "CONFIDENCE SCALE\n"
    "--------------------------------------------------\n"
    "0.0-0.4 -> weak (avoid trading)\n"
    "0.4-0.6 -> neutral (prefer HOLD)\n"
    "0.6-0.8 -> valid setup\n"
    "0.8-1.0 -> strong setup\n\n"
    "--------------------------------------------------\n"
    "RISK CONTROL RULES\n"
    "--------------------------------------------------\n"
    "- Respect risk_rules strictly\n"
    "- Adjust SL / TP / trailing based on volatility (ATR%)\n"
    "- Do NOT output static values blindly\n"
    "- Higher ATR -> wider SL, tighter trailing\n"
    "- Lower ATR -> tighter SL, standard TP\n\n"
    "--------------------------------------------------\n"
    "BEHAVIOR CONSTRAINTS\n"
    "--------------------------------------------------\n"
    "- Do NOT chase price\n"
    "- Prefer HOLD over low-quality trades\n"
    "- Ignore previous AI decisions (last_ai)\n"
    "- Be deterministic and consistent\n\n"
    "--------------------------------------------------\n"
    "OUTPUT FORMAT (STRICT JSON ONLY)\n"
    "--------------------------------------------------\n"
    "{\n"
    "  \"action\": \"BUY | SELL | HOLD | EXIT\",\n"
    "  \"confidence\": 0.0,\n"
    "  \"reason\": \"<trend>, <momentum>, <location>. <decision (max 255 chars)>\",\n"
    "  \"risk_control\": {\n"
    "    \"keep_position_on_tp\": true|false,\n"
    "    \"suggested_sl_pct\": number >= 0,\n"
    "    \"suggested_trailing_stop_pct\": number >= 0,\n"
    "    \"suggested_tp_pct\": number >= 0\n"
    "  }\n"
    "}\n\n"
    "Rules:\n"
    "- Reason MUST include: trend + momentum + price location\n"
    "- Reason MUST be <= 255 characters\n"
    "- Return ONLY valid JSON"
)


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
        "http_client",
        "base_url",
        "use_ai",
        "model_name",
        "max_tokens",
        "ai_interval_bars",
        "entry_confidence",
        "override_confidence",
        "take_profit_pct",
        "stop_loss_pct",
        "trailing_stop_pct",
        "max_hold_bars",
        "cooldown_bars",
        "sl_cooldown_bars",
        "max_consecutive_stop_losses",
        "sl_pause_bars",
        "pause_bars",
        "stop_loss_streak",
        "max_positions",
        "macd_ready_min",
        "rsi_period",
        "rsi_avg_gain",
        "rsi_avg_loss",
        "rsi_count",
        "prev_close",
        "prev_macd_hist",
        "prev_ema_spread_pct",
        "prev_vwap_gap_pct",
        "prev_atr_pct",
        "recent_returns_pct",
        "recent_macd_hist",
        "recent_atr_pct",
        "recent_klines",
        "previous_trade_conditions",
        "last_ai_action",
        "last_ai_confidence",
        "last_ai_reason",
        "ai_keep_position_on_tp",
        "ai_suggested_sl_pct",
        "ai_suggested_trailing_stop_pct",
        "ai_suggested_tp_pct",
        "active_take_profit_pct",
        "active_stop_loss_pct",
        "active_trailing_stop_pct",
        "tp_anchor_price",
        "tp_ladder_steps",
        "intrabar_risk_guard",
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
        self.pause_bars = 0
        self.stop_loss_streak = 0
        self.bar_count = 0

        self.use_ai = bool(section.get("use_ai", True))
        self.model_name = section.get("ai_model", "gpt-4o-mini")
        self.max_tokens = int(GLOBAL_CONFIG.get("max_tokens", 1200))
        self.ai_interval_bars = max(1, int(section.get("ai_interval_bars", 3)))
        self.entry_confidence = float(section.get("entry_confidence", 0.62))
        self.override_confidence = float(section.get("override_confidence", 0.75))

        self.take_profit_pct = float(cfg_value(section, GLOBAL_CONFIG, "take_profit_pct", 0.40))
        self.stop_loss_pct = float(cfg_value(section, GLOBAL_CONFIG, "stop_loss_pct", 0.25))
        self.trailing_stop_pct = float(cfg_value(section, GLOBAL_CONFIG, "trailing_stop_pct", 0.20))
        self.max_hold_bars = int(cfg_value(section, GLOBAL_CONFIG, "max_hold_bars", 24))
        self.cooldown_bars = int(cfg_value(section, GLOBAL_CONFIG, "cooldown_bars", 2))
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
        self.max_positions = int(cfg_value(section, GLOBAL_CONFIG, "max_positions", 1))

        self.macd_ready_min = max(50, macd_slow + macd_signal)
        self.rsi_period = max(2, int(section.get("rsi_period", 14)))
        self.rsi_avg_gain = 0.0
        self.rsi_avg_loss = 0.0
        self.rsi_count = 0

        self.prev_close: Optional[float] = None
        self.prev_macd_hist: Optional[float] = None
        self.prev_ema_spread_pct: Optional[float] = None
        self.prev_vwap_gap_pct: Optional[float] = None
        self.prev_atr_pct: Optional[float] = None
        self.recent_returns_pct = deque(maxlen=5)
        self.recent_macd_hist = deque(maxlen=5)
        self.recent_atr_pct = deque(maxlen=5)
        self.recent_klines = deque(maxlen=24)
        self.previous_trade_conditions = deque(maxlen=5)
        self.last_ai_action = "HOLD"
        self.last_ai_confidence = 0.0
        self.last_ai_reason = "INIT"
        self.ai_keep_position_on_tp = False
        self.ai_suggested_sl_pct: Optional[float] = None
        self.ai_suggested_trailing_stop_pct: Optional[float] = None
        self.ai_suggested_tp_pct: Optional[float] = None
        self.active_take_profit_pct = self.take_profit_pct
        self.active_stop_loss_pct = self.stop_loss_pct
        self.active_trailing_stop_pct = self.trailing_stop_pct
        self.tp_anchor_price: Optional[float] = None
        self.tp_ladder_steps = 0
        self.intrabar_risk_guard = False

        api_key = (
            section.get("api_key")
            or GLOBAL_CONFIG.get("api_key")
            or os.getenv("API_KEY")
            or section.get("openai_api_key")
            or GLOBAL_CONFIG.get("openai_api_key")
            or os.getenv("OPENAI_API_KEY")
            or section.get("anthropic_api_key")
            or GLOBAL_CONFIG.get("anthropic_api_key")
            or os.getenv("ANTHROPIC_API_KEY")
        )
        self.base_url = (
            section.get("proxy_host")
            or GLOBAL_CONFIG.get("proxy_host")
            or os.getenv("PROXY_HOST")
            or section.get("openai_base_url")
            or GLOBAL_CONFIG.get("openai_base_url")
            or os.getenv("OPENAI_BASE_URL")
            or section.get("anthropic_base_url")
            or GLOBAL_CONFIG.get("anthropic_base_url")
            or os.getenv("ANTHROPIC_BASE_URL")
            or "https://llm.proxy"
        )
        client_kwargs = {"api_key": api_key}
        if self.base_url:
            client_kwargs["base_url"] = self.base_url

        self.http_client: Optional[httpx.Client] = None
        if self.use_ai and api_key:
            self.http_client = httpx.Client()
            self.ai_client = OpenAI(http_client=self.http_client, **client_kwargs)
        else:
            self.ai_client = None

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
    def _extract_text(response) -> str:
        def _coerce_text(value: Any) -> str:
            if isinstance(value, str):
                return value.strip()
            if isinstance(value, list):
                parts = []
                for item in value:
                    text = _coerce_text(item)
                    if text:
                        parts.append(text)
                return "\n".join(parts).strip()
            if isinstance(value, dict):
                # Common OpenAI-compatible payload variants.
                for key in ("text", "value", "content", "output_text", "reasoning_content"):
                    text = _coerce_text(value.get(key))
                    if text:
                        return text
            return ""

        choices = getattr(response, "choices", None) or []
        if not choices:
            output_text = getattr(response, "output_text", None)
            if isinstance(output_text, str):
                return output_text.strip()
            return ""

        first_choice = choices[0]
        direct_text = getattr(first_choice, "text", None)
        if isinstance(first_choice, dict):
            direct_text = first_choice.get("text", direct_text)
        if isinstance(direct_text, str) and direct_text.strip():
            return direct_text.strip()

        message = getattr(first_choice, "message", None)
        if isinstance(first_choice, dict):
            message = first_choice.get("message")

        content = getattr(message, "content", None)
        if isinstance(message, dict):
            content = message.get("content")

        if isinstance(content, str):
            return content.strip()
        if isinstance(content, list):
            texts = []
            for part in content:
                part_type = getattr(part, "type", None)
                part_text = getattr(part, "text", None)
                if isinstance(part, dict):
                    part_type = part.get("type")
                    part_text = part.get("text")
                if part_type in {"text", "output_text"} and part_text:
                    candidate = _coerce_text(part_text)
                    if candidate:
                        texts.append(candidate)
                elif isinstance(part, str) and part.strip():
                    texts.append(part.strip())
                elif isinstance(part, dict):
                    candidate = _coerce_text(part)
                    if candidate:
                        texts.append(candidate)
            return "\n".join(texts).strip()

        # Fallbacks for OpenAI-compatible providers that return non-standard fields.
        fallback = _coerce_text(message)
        if fallback:
            return fallback
        fallback = _coerce_text(first_choice)
        if fallback:
            return fallback
        return ""

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
    def _window_stats(values: deque) -> Dict[str, float]:
        if not values:
            return {"mean": 0.0, "min": 0.0, "max": 0.0}
        arr = list(values)
        return {
            "mean": sum(arr) / len(arr),
            "min": min(arr),
            "max": max(arr),
        }

    @staticmethod
    def _normalize_pct(raw: Any) -> Optional[float]:
        if raw is None:
            return None
        try:
            value = float(raw)
        except Exception:
            return None
        if value < 0:
            return None
        return value

    @staticmethod
    def _normalize_bool(raw: Any, default: bool = False) -> bool:
        if isinstance(raw, bool):
            return raw
        if isinstance(raw, (int, float)):
            return bool(raw)
        if isinstance(raw, str):
            return raw.strip().lower() in {"1", "true", "yes", "y", "on"}
        return default

    @staticmethod
    def _truncate_text(value: Any, limit: int = 512) -> str:
        text = str(value or "").strip()
        if len(text) <= limit:
            return text
        return f"{text[:limit]}...<truncated:{len(text)-limit}>"

    @staticmethod
    def _macd_condition(macd_hist: float) -> str:
        if macd_hist > 0:
            return "bullish"
        if macd_hist < 0:
            return "bearish"
        return "neutral"

    @staticmethod
    def _vwap_condition(vwap_gap_pct: float) -> str:
        if vwap_gap_pct > 0:
            return "above_vwap"
        if vwap_gap_pct < 0:
            return "below_vwap"
        return "at_vwap"

    @staticmethod
    def _atr_condition(atr_pct: float) -> str:
        if atr_pct <= 0:
            return "flat"
        if atr_pct < 1.8:
            return "normal"
        return "high"

    def _build_kline_snapshot(
        self,
        candle: Candle,
        open_price: Optional[float] = None,
        macd_line: Optional[float] = None,
        macd_signal: Optional[float] = None,
        macd_hist: Optional[float] = None,
        vwap_px: Optional[float] = None,
        vwap_gap_pct: Optional[float] = None,
        atr_pct: Optional[float] = None,
        atr: Optional[float] = None,
        rsi: Optional[float] = None,
        distance_from_ema: Optional[float] = None,
    ) -> Dict[str, Any]:
        high = candle.high if candle.high is not None else candle.close
        low = candle.low if candle.low is not None else candle.close
        opened = open_price if open_price is not None else candle.close
        snapshot = {
            "t": int(max(0, int(candle.close_time_ms)) // 1000),
            "o": round(opened, 8),
            "h": round(high, 8),
            "l": round(low, 8),
            "c": round(candle.close, 8),
            "v": round(candle.quote_volume, 8),
        }
        if (
            macd_line is not None
            and macd_signal is not None
            and macd_hist is not None
            and vwap_px is not None
            and vwap_gap_pct is not None
            and atr_pct is not None
        ):
            snapshot["condition"] = {
                "macd": self._macd_condition(macd_hist),
                "vwap": self._vwap_condition(vwap_gap_pct),
                "atr": self._atr_condition(atr_pct),
            }
            snapshot["metrics"] = {
                "macd": {
                    "line": round(macd_line, 8),
                    "signal": round(macd_signal, 8),
                    "hist": round(macd_hist, 8),
                },
                "vwap": {
                    "value": round(vwap_px, 8),
                    "gap_pct": round(vwap_gap_pct, 6),
                },
                "atr": round(atr, 8) if atr is not None else None,
                "atr_pct": round(atr_pct, 6),
                "rsi": round(rsi, 4) if rsi is not None else None,
                "distance_from_ema": round(distance_from_ema, 6) if distance_from_ema is not None else None,
            }
        return snapshot

    def _build_previous_trade_condition_snapshot(self) -> Dict[str, Any]:
        return {
            "prev_close": round(self.prev_close, 8) if self.prev_close is not None else None,
            "prev_macd_hist": round(self.prev_macd_hist, 8) if self.prev_macd_hist is not None else None,
            "prev_ema_spread_pct": round(self.prev_ema_spread_pct, 6) if self.prev_ema_spread_pct is not None else None,
            "prev_vwap_gap_pct": round(self.prev_vwap_gap_pct, 6) if self.prev_vwap_gap_pct is not None else None,
            "prev_atr_pct": round(self.prev_atr_pct, 6) if self.prev_atr_pct is not None else None,
            "recent_returns_pct": self._window_stats(self.recent_returns_pct),
            "recent_macd_hist": self._window_stats(self.recent_macd_hist),
            "recent_atr_pct": self._window_stats(self.recent_atr_pct),
        }

    @staticmethod
    def _build_user_input_prompt(payload: Dict[str, Any]) -> str:
        kline_src = payload.get("kline") if isinstance(payload.get("kline"), list) else []
        kline: list = []
        for item in kline_src:
            if not isinstance(item, dict):
                continue
            metrics = item.get("metrics") if isinstance(item.get("metrics"), dict) else {}
            macd_metrics = metrics.get("macd") if isinstance(metrics.get("macd"), dict) else {}
            vwap_metrics = metrics.get("vwap") if isinstance(metrics.get("vwap"), dict) else {}
            kline.append(
                {
                    "t": item.get("t"),
                    "o": item.get("o"),
                    "h": item.get("h"),
                    "l": item.get("l"),
                    "c": item.get("c"),
                    "v": item.get("v"),
                    "rsi": metrics.get("rsi"),
                    "macd_hist": macd_metrics.get("hist"),
                    "macd_line": macd_metrics.get("line"),
                    "macd_signal": macd_metrics.get("signal"),
                    "vwap_gap_pct": vwap_metrics.get("gap_pct"),
                    "ema_dist_pct": metrics.get("distance_from_ema"),
                    "atr_pct": metrics.get("atr_pct"),
                }
            )

        trend_data = payload.get("trend") if isinstance(payload.get("trend"), dict) else {}
        previous_trade_condition = (
            payload.get("previous_trade_condition")
            if isinstance(payload.get("previous_trade_condition"), list)
            else []
        )
        prev_snapshot = previous_trade_condition[-1] if previous_trade_condition and isinstance(previous_trade_condition[-1], dict) else {}
        current_position = payload.get("current_position") if isinstance(payload.get("current_position"), dict) else {}
        risk_rules = payload.get("risk_rules") if isinstance(payload.get("risk_rules"), dict) else {}

        ema_fast = trend_data.get("ema_fast")
        ema_slow = trend_data.get("ema_slow")
        ema_spread_pct = 0.0
        if isinstance(ema_fast, (int, float)) and isinstance(ema_slow, (int, float)) and ema_slow != 0:
            ema_spread_pct = ((float(ema_fast) - float(ema_slow)) / float(ema_slow)) * 100.0

        rsi_values = [
            float(entry.get("rsi"))
            for entry in kline
            if isinstance(entry, dict) and isinstance(entry.get("rsi"), (int, float))
        ]
        rsi_mean_5 = sum(rsi_values[-5:]) / len(rsi_values[-5:]) if rsi_values else 0.0

        macd_hist_values = [
            float(entry.get("macd_hist"))
            for entry in kline
            if isinstance(entry, dict) and isinstance(entry.get("macd_hist"), (int, float))
        ]
        macd_hist_trend = 0
        if len(macd_hist_values) >= 2:
            delta = macd_hist_values[-1] - macd_hist_values[-2]
            macd_hist_trend = 1 if delta > 0 else (-1 if delta < 0 else 0)
        elif len(macd_hist_values) == 1:
            macd_hist_trend = 1 if macd_hist_values[-1] > 0 else (-1 if macd_hist_values[-1] < 0 else 0)

        atr_pct_now = kline[-1].get("atr_pct") if kline and isinstance(kline[-1], dict) else None
        volatility_regime = "normal"
        if isinstance(atr_pct_now, (int, float)):
            if atr_pct_now <= 0:
                volatility_regime = "flat"
            elif atr_pct_now >= 1.8:
                volatility_regime = "high"

        return_5 = 0.0
        recent_returns = prev_snapshot.get("recent_returns_pct") if isinstance(prev_snapshot.get("recent_returns_pct"), dict) else {}
        if isinstance(recent_returns.get("mean"), (int, float)):
            return_5 = float(recent_returns.get("mean"))

        user_input = {
            "symbol": payload.get("symbol"),
            "timeframe": payload.get("timeframe"),
            "kline": kline,
            "features": {
                "trend": str(trend_data.get("trend", "sideways")),
                "ema_spread_pct": round(ema_spread_pct, 6),
                "macd_hist_trend": macd_hist_trend,
                "rsi_mean_5": round(rsi_mean_5, 4),
                "volatility_regime": volatility_regime,
                "return_5": round(return_5, 6),
            },
            "risk_rules": {
                "max_loss_pct": abs(float(risk_rules.get("max_loss", 0.0))),
                "target_profit_pct": abs(float(risk_rules.get("target_profit", 0.0))),
                "max_positions": int(risk_rules.get("max_positions", 1) or 1),
            },
            "position": {
                "is_open": bool(current_position.get("is_open", False)),
                "side": str(current_position.get("side", "FLAT")),
                "entry_price": current_position.get("entry_price"),
                "bars_in_position": int(current_position.get("bars_in_position", 0) or 0),
            },
        }

        return (
            "Analyze the following market data and return a trading decision.\n\n"
            "Input:\n"
            f"{json.dumps(user_input, separators=(',', ':'))}\n\n"
            "Instructions:\n"
            "- Follow the system decision framework strictly\n"
            "- Use ONLY the provided data\n"
            "- Do NOT assume missing data\n"
            "- Do NOT explain outside JSON\n"
            "- Be consistent and deterministic\n\n"
            "Output:\n"
            "Return ONLY valid JSON matching the required schema without any additional text."
        )

    def _ai_signal(self, payload: Dict[str, Any]) -> Tuple[str, float, str, Dict[str, Any]]:
        if self.intrabar_risk_guard:
            raise RuntimeError("LLM call is blocked during intrabar risk checks")
        if self.ai_client is None:
            raise RuntimeError("AI client is not initialized. Set api_key or API_KEY.")

        payload_symbol = str(payload.get("symbol", self.config.symbol) or self.config.symbol)
        payload_timeframe = str(payload.get("timeframe", self.config.interval) or self.config.interval)

        user_payload_text = self._build_user_input_prompt(payload)
        print(
            (
                f"[AI_REQUEST] symbol={payload_symbol} interval={payload_timeframe} "
                f"model={self.model_name} "
                f"prompt_len={len(AI_SYSTEM_PROMPT)} payload_len={len(user_payload_text)}"
            ),
            flush=True,
        )

        try:
            response = self.ai_client.chat.completions.create(
                model=self.model_name,
                max_tokens=max(1, self.max_tokens),
                messages=[
                    {
                        "role": "system",
                        "content": AI_SYSTEM_PROMPT,
                    },
                    {
                        "role": "user",
                        "content": user_payload_text,
                    }
                ],
                temperature=0,
            )

            raw_text = self._extract_text(response)
            finish_reason = None
            choices = getattr(response, "choices", None) or []
            if choices:
                first_choice = choices[0]
                finish_reason = getattr(first_choice, "finish_reason", None)
                if isinstance(first_choice, dict):
                    finish_reason = first_choice.get("finish_reason")
            print(
                (
                    f"[AI_RESPONSE_RAW] symbol={payload_symbol} interval={payload_timeframe} "
                    f"model={self.model_name} finish_reason={finish_reason} "
                    f"response={self._truncate_text(raw_text, 600)}"
                ),
                flush=True,
            )
            if not raw_text:
                raise RuntimeError(f"No text in response (finish_reason={finish_reason})")
            parsed = self._extract_json(raw_text)

            action = str(parsed.get("action", "HOLD")).upper()
            if action not in {"BUY", "SELL", "HOLD", "EXIT"}:
                action = "HOLD"

            confidence = float(parsed.get("confidence", 0.0))
            confidence = max(0.0, min(1.0, confidence))

            reason = " ".join(str(parsed.get("reason", "AI_DECISION")).split())[:255]

            risk_control = parsed.get("risk_control")
            if not isinstance(risk_control, dict):
                risk_control = {}

            keep_position_on_tp = self._normalize_bool(risk_control.get("keep_position_on_tp"), default=False)
            suggested_sl_pct = self._normalize_pct(risk_control.get("suggested_sl_pct"))
            suggested_trailing_stop_pct = self._normalize_pct(risk_control.get("suggested_trailing_stop_pct"))
            suggested_tp_pct = self._normalize_pct(risk_control.get("suggested_tp_pct"))

            self.ai_keep_position_on_tp = keep_position_on_tp
            self.ai_suggested_sl_pct = suggested_sl_pct
            self.ai_suggested_trailing_stop_pct = suggested_trailing_stop_pct
            self.ai_suggested_tp_pct = suggested_tp_pct

            risk_feedback = {
                "keep_position_on_tp": keep_position_on_tp,
                "suggested_sl_pct": suggested_sl_pct,
                "suggested_trailing_stop_pct": suggested_trailing_stop_pct,
                "suggested_tp_pct": suggested_tp_pct,
            }
            print(
                (
                    f"[AI_RESPONSE_PARSED] symbol={payload_symbol} interval={payload_timeframe} "
                    f"model={self.model_name} action={action} confidence={confidence:.4f} "
                    f"reason={reason} risk_control={risk_feedback}"
                ),
                flush=True,
            )
            return action, confidence, reason, risk_feedback
        except Exception as exc:
            print(
                (
                    f"[AI_ERROR] symbol={payload_symbol} interval={payload_timeframe} "
                    f"model={self.model_name} error={type(exc).__name__}:{exc}"
                ),
                flush=True,
            )
            raise RuntimeError(f"LLM request failed: {type(exc).__name__}:{exc}") from exc

    @staticmethod
    def _trend_label(ema_fast: float, ema_slow: float) -> str:
        if ema_fast > ema_slow:
            return "bullish"
        if ema_fast < ema_slow:
            return "bearish"
        return "sideways"

    @staticmethod
    def _interval_to_minutes(interval: str) -> int:
        raw = str(interval or "").strip().lower()
        if not raw:
            return 1
        if raw.endswith("m") and raw[:-1].isdigit():
            return max(1, int(raw[:-1]))
        if raw.endswith("h") and raw[:-1].isdigit():
            return max(1, int(raw[:-1]) * 60)
        if raw.endswith("d") and raw[:-1].isdigit():
            return max(1, int(raw[:-1]) * 1440)
        return 1

    def _update_rsi(self, close: float) -> Optional[float]:
        if self.prev_close is None:
            return None

        delta = close - self.prev_close
        gain = max(delta, 0.0)
        loss = max(-delta, 0.0)

        self.rsi_count += 1
        if self.rsi_count <= self.rsi_period:
            scale = float(self.rsi_count)
            self.rsi_avg_gain = ((self.rsi_avg_gain * (scale - 1.0)) + gain) / scale
            self.rsi_avg_loss = ((self.rsi_avg_loss * (scale - 1.0)) + loss) / scale
            if self.rsi_count < self.rsi_period:
                return None
        else:
            period = float(self.rsi_period)
            self.rsi_avg_gain = ((self.rsi_avg_gain * (period - 1.0)) + gain) / period
            self.rsi_avg_loss = ((self.rsi_avg_loss * (period - 1.0)) + loss) / period

        if self.rsi_avg_loss == 0.0:
            return 100.0
        rs = self.rsi_avg_gain / self.rsi_avg_loss
        return 100.0 - (100.0 / (1.0 + rs))

    def _build_ai_payload(
        self,
        candle: Candle,
        ema_fast: float,
        ema_slow: float,
        macd_line: float,
        macd_signal: float,
        macd_hist: float,
        vwap_px: float,
        atr: float,
        rsi: Optional[float],
    ) -> Dict[str, Any]:
        distance_from_ema = ((candle.close - ema_slow) / ema_slow * 100.0) if ema_slow else 0.0
        vwap_gap_pct = (candle.close - vwap_px) / vwap_px * 100.0 if vwap_px else 0.0
        atr_pct = atr / candle.close * 100.0 if candle.close else 0.0
        symbol = str(candle.symbol or self.config.symbol or "").strip().upper() or str(self.config.symbol)
        timeframe = str(candle.interval or self.config.interval or "").strip() or str(self.config.interval)
        current_open = self.prev_close if self.prev_close is not None else candle.close

        kline = list(self.recent_klines)
        kline.append(
            self._build_kline_snapshot(
                candle,
                open_price=current_open,
                macd_line=macd_line,
                macd_signal=macd_signal,
                macd_hist=macd_hist,
                vwap_px=vwap_px,
                vwap_gap_pct=vwap_gap_pct,
                atr_pct=atr_pct,
                atr=atr,
                rsi=rsi,
                distance_from_ema=distance_from_ema,
            )
        )
        if len(kline) > 5:
            kline = kline[-5:]

        previous_trade_condition = list(self.previous_trade_conditions)
        previous_trade_condition.append(self._build_previous_trade_condition_snapshot())
        if len(previous_trade_condition) > 5:
            previous_trade_condition = previous_trade_condition[-5:]

        payload: Dict[str, Any] = {
            "symbol": symbol,
            "timeframe": timeframe,
            "last_ai": {
                "action": self.last_ai_action,
                "confidence": round(self.last_ai_confidence, 4),
                "reason": self.last_ai_reason,
            },
            "kline": kline,
            "trend": {
                "ema_fast": round(ema_fast, 8),
                "ema_slow": round(ema_slow, 8),
                "trend": self._trend_label(ema_fast, ema_slow),
            },
            "risk_rules": {
                "max_loss": -abs(round(self.stop_loss_pct, 6)),
                "target_profit": abs(round(self.take_profit_pct, 6)),
                "max_positions": max(1, self.max_positions),
            },
            "previous_trade_condition": previous_trade_condition,
            "current_position": {
                "is_open": self.position_side in {"LONG", "SHORT"} and self.entry_price is not None and self.entry_price != 0,
                "side": self.position_side or "FLAT",
                "entry_price": round(self.entry_price, 8) if self.entry_price is not None else None,
                "bars_in_position": max(0, int(self.bars_in_pos)),
                "tp_anchor_price": round(self.tp_anchor_price, 8) if self.tp_anchor_price is not None else None,
                "tp_ladder_steps": max(0, int(self.tp_ladder_steps)),
                "highest_since_entry": round(self.highest_since_entry, 8) if self.highest_since_entry is not None else None,
                "lowest_since_entry": round(self.lowest_since_entry, 8) if self.lowest_since_entry is not None else None,
                "risk": {
                    "active_take_profit_pct": round(float(self.active_take_profit_pct), 6),
                    "active_stop_loss_pct": round(float(self.active_stop_loss_pct), 6),
                    "active_trailing_stop_pct": round(float(self.active_trailing_stop_pct), 6),
                },
            },
        }

        if self.position_side in {"LONG", "SHORT"} and self.entry_price is not None and self.entry_price != 0:
            if self.position_side == "LONG":
                pnl_pct = (candle.close - self.entry_price) / self.entry_price * 100.0
                side = "BUY"
            else:
                pnl_pct = (self.entry_price - candle.close) / self.entry_price * 100.0
                side = "SELL"

            hold_minutes = max(1, self.bars_in_pos * self._interval_to_minutes(self.config.interval))
            payload["position"] = {
                "side": side,
                "entry": round(self.entry_price, 8),
                "pnl": round(pnl_pct, 4),
                "holding_time": f"{hold_minutes}m",
            }
            payload["current_position"]["unrealized_pnl_pct"] = round(pnl_pct, 4)
            payload["current_position"]["holding_time_minutes"] = hold_minutes

        return payload

    def _update_bar_state(
        self,
        candle: Candle,
        macd_line: float,
        macd_signal: float,
        macd_hist: float,
        vwap_px: float,
        atr: float,
        rsi: Optional[float],
        distance_from_ema: float,
        ema_spread_pct: float,
        vwap_gap_pct: float,
        atr_pct: float,
        ret_1_pct: float,
    ) -> None:
        previous_close = self.prev_close
        self.prev_close = candle.close
        self.prev_macd_hist = macd_hist
        self.prev_ema_spread_pct = ema_spread_pct
        self.prev_vwap_gap_pct = vwap_gap_pct
        self.prev_atr_pct = atr_pct
        self.recent_returns_pct.append(ret_1_pct)
        self.recent_macd_hist.append(macd_hist)
        self.recent_atr_pct.append(atr_pct)
        self.recent_klines.append(
            self._build_kline_snapshot(
                candle,
                open_price=previous_close or candle.close,
                macd_line=macd_line,
                macd_signal=macd_signal,
                macd_hist=macd_hist,
                vwap_px=vwap_px,
                vwap_gap_pct=vwap_gap_pct,
                atr_pct=atr_pct,
                atr=atr,
                rsi=rsi,
                distance_from_ema=distance_from_ema,
            )
        )
        self.previous_trade_conditions.append(self._build_previous_trade_condition_snapshot())

    def _test_llm_on_init(self) -> None:
        if self.ai_client is None:
            raise RuntimeError("AI client is not initialized. Set api_key or API_KEY.")

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
                f"model={self.model_name} payload={test_payload_text}"
            ),
            flush=True,
        )

        try:
            init_max_tokens = max(64, min(max(1, self.max_tokens), 256))
            response = self.ai_client.chat.completions.create(
                model=self.model_name,
                max_tokens=init_max_tokens,
                messages=[
                    {
                        "role": "system",
                        "content": system_prompt,
                    },
                    {
                        "role": "user",
                        "content": test_payload_text,
                    }
                ],
                temperature=0,
            )
            raw_text = self._extract_text(response)
            finish_reason = None
            choices = getattr(response, "choices", None) or []
            if choices:
                first_choice = choices[0]
                finish_reason = getattr(first_choice, "finish_reason", None)
                if isinstance(first_choice, dict):
                    finish_reason = first_choice.get("finish_reason")
            if not raw_text:
                if choices:
                    print(
                        (
                            f"[AI_INIT_TEST_RESPONSE_NO_TEXT] symbol={self.config.symbol} interval={self.config.interval} "
                            f"model={self.model_name} finish_reason={finish_reason} "
                            "status=ok warning=empty_text_with_nonempty_choices"
                        ),
                        flush=True,
                    )
                    return
                raise RuntimeError("empty response from LLM init test")

            print(
                (
                    f"[AI_INIT_TEST_RESPONSE] symbol={self.config.symbol} interval={self.config.interval} "
                    f"model={self.model_name} response={raw_text}"
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

    def _position_risk_exit(self, candle: Candle, metadata: Dict[str, Any], advance_bar: bool = True):
        if self.position_side == "LONG" and self.entry_price is not None:
            if advance_bar:
                self.bars_in_pos += 1

            high_px = candle.high if candle.high is not None else candle.close
            low_px = candle.low if candle.low is not None else candle.close
            self.highest_since_entry = max(self.highest_since_entry or high_px, high_px)
            entry_price = float(self.entry_price)

            if self.tp_anchor_price is None:
                self.tp_anchor_price = entry_price

            tp_base = float(self.tp_anchor_price)
            tp_step_pct = max(0.0, float(self.active_take_profit_pct))
            if tp_step_pct > 0:
                tp_px = tp_base * (1.0 + pct_to_frac(tp_step_pct))
            else:
                tp_px = entry_price

            sl_px = entry_price * (1.0 - pct_to_frac(max(0.0, float(self.active_stop_loss_pct))))
            trail_px = (self.highest_since_entry or high_px) * (
                1.0 - pct_to_frac(max(0.0, float(self.active_trailing_stop_pct)))
            )
            sl_px = max(sl_px, trail_px)

            if high_px >= tp_px:
                can_roll_tp = (
                    bool(metadata.get("tp_roll_eligible"))
                    and tp_step_pct > 0
                )
                if can_roll_tp:
                    self.tp_anchor_price = tp_px
                    self.tp_ladder_steps += 1
                    self.active_stop_loss_pct = 0.0
                    metadata["tp_roll"] = True
                    metadata["tp_roll_anchor"] = round(tp_px, 8)
                    metadata["tp_roll_steps"] = self.tp_ladder_steps
                    metadata["trade_condition"] = "HOLD"
                    return None
                return self._close_long(candle.close, "TAKE_PROFIT_LONG", metadata)
            if low_px <= sl_px:
                return self._close_long(candle.close, "STOP_LOSS_LONG", metadata)
            if low_px <= trail_px:
                return self._close_long(candle.close, "TRAILING_STOP_LONG", metadata)
            if advance_bar and self.bars_in_pos >= self.max_hold_bars:
                return self._close_long(candle.close, "EXIT_LONG_TIMEOUT", metadata)

        if self.position_side == "SHORT" and self.entry_price is not None:
            if advance_bar:
                self.bars_in_pos += 1

            high_px = candle.high if candle.high is not None else candle.close
            low_px = candle.low if candle.low is not None else candle.close
            self.lowest_since_entry = min(self.lowest_since_entry or low_px, low_px)
            entry_price = float(self.entry_price)

            if self.tp_anchor_price is None:
                self.tp_anchor_price = entry_price

            tp_base = float(self.tp_anchor_price)
            tp_step_pct = max(0.0, float(self.active_take_profit_pct))
            if tp_step_pct > 0:
                tp_px = tp_base * (1.0 - pct_to_frac(tp_step_pct))
            else:
                tp_px = entry_price

            sl_px = entry_price * (1.0 + pct_to_frac(max(0.0, float(self.active_stop_loss_pct))))
            trail_px = (self.lowest_since_entry or low_px) * (
                1.0 + pct_to_frac(max(0.0, float(self.active_trailing_stop_pct)))
            )
            sl_px = min(sl_px, trail_px)

            if low_px <= tp_px:
                can_roll_tp = (
                    bool(metadata.get("tp_roll_eligible"))
                    and tp_step_pct > 0
                )
                if can_roll_tp:
                    self.tp_anchor_price = tp_px
                    self.tp_ladder_steps += 1
                    self.active_stop_loss_pct = 0.0
                    metadata["tp_roll"] = True
                    metadata["tp_roll_anchor"] = round(tp_px, 8)
                    metadata["tp_roll_steps"] = self.tp_ladder_steps
                    metadata["trade_condition"] = "HOLD"
                    return None
                return self._close_short(candle.close, "TAKE_PROFIT_SHORT", metadata)
            if high_px >= sl_px:
                return self._close_short(candle.close, "STOP_LOSS_SHORT", metadata)
            if high_px >= trail_px:
                return self._close_short(candle.close, "TRAILING_STOP_SHORT", metadata)
            if advance_bar and self.bars_in_pos >= self.max_hold_bars:
                return self._close_short(candle.close, "EXIT_SHORT_TIMEOUT", metadata)

        return None

    def on_price_update(self, candle: Candle):
        if self.position_side is None:
            return None

        self.intrabar_risk_guard = True
        try:
            metadata = {
                "intrabar": True,
                "close": round(candle.close, 8),
                "high": round(candle.high if candle.high is not None else candle.close, 8),
                "low": round(candle.low if candle.low is not None else candle.close, 8),
            }
            return self._position_risk_exit(candle, metadata, advance_bar=False)
        finally:
            self.intrabar_risk_guard = False

    def _close_long(self, price: float, reason: str, metadata: Dict[str, Any]):
        tagged_metadata = dict(metadata)
        if "trade_condition" not in tagged_metadata:
            if reason.startswith("TAKE_PROFIT"):
                tagged_metadata["trade_condition"] = "TAKE_PROFIT"
            elif reason.startswith("STOP_LOSS"):
                tagged_metadata["trade_condition"] = "STOP_LOSS"
            elif reason.startswith("TRAILING_STOP"):
                tagged_metadata["trade_condition"] = "TRAILING_STOP"
            else:
                tagged_metadata["trade_condition"] = "EXIT"
        tagged_metadata["order_reason"] = reason[:255]
        if reason.startswith("TAKE_PROFIT"):
            tagged_metadata["exit_type"] = "TAKE_PROFIT"
        elif reason.startswith("STOP_LOSS"):
            tagged_metadata["exit_type"] = "STOP_LOSS"
        elif reason.startswith("TRAILING_STOP"):
            tagged_metadata["exit_type"] = "TRAILING_STOP"
        else:
            tagged_metadata["exit_type"] = ""
        self.position_side = None
        self.entry_price = None
        self.highest_since_entry = None
        self.lowest_since_entry = None
        self.tp_anchor_price = None
        self.tp_ladder_steps = 0
        self.active_take_profit_pct = self.take_profit_pct
        self.active_stop_loss_pct = self.stop_loss_pct
        self.active_trailing_stop_pct = self.trailing_stop_pct
        self.bars_in_pos = 0
        if reason.startswith("STOP_LOSS"):
            self.stop_loss_streak += 1
            self.cooldown = max(self.cooldown_bars, self.sl_cooldown_bars)
            if (
                self.max_consecutive_stop_losses > 0
                and self.stop_loss_streak >= self.max_consecutive_stop_losses
                and self.sl_pause_bars > 0
            ):
                self.pause_bars = max(self.pause_bars, self.sl_pause_bars)
                self.stop_loss_streak = 0
        else:
            self.cooldown = self.cooldown_bars
            self.stop_loss_streak = 0
        return self.sell(price, reason, tagged_metadata)

    def _close_short(self, price: float, reason: str, metadata: Dict[str, Any]):
        tagged_metadata = dict(metadata)
        if "trade_condition" not in tagged_metadata:
            if reason.startswith("TAKE_PROFIT"):
                tagged_metadata["trade_condition"] = "TAKE_PROFIT"
            elif reason.startswith("STOP_LOSS"):
                tagged_metadata["trade_condition"] = "STOP_LOSS"
            elif reason.startswith("TRAILING_STOP"):
                tagged_metadata["trade_condition"] = "TRAILING_STOP"
            else:
                tagged_metadata["trade_condition"] = "EXIT"
        tagged_metadata["order_reason"] = reason[:255]
        if reason.startswith("TAKE_PROFIT"):
            tagged_metadata["exit_type"] = "TAKE_PROFIT"
        elif reason.startswith("STOP_LOSS"):
            tagged_metadata["exit_type"] = "STOP_LOSS"
        elif reason.startswith("TRAILING_STOP"):
            tagged_metadata["exit_type"] = "TRAILING_STOP"
        else:
            tagged_metadata["exit_type"] = ""
        self.position_side = None
        self.entry_price = None
        self.highest_since_entry = None
        self.lowest_since_entry = None
        self.tp_anchor_price = None
        self.tp_ladder_steps = 0
        self.active_take_profit_pct = self.take_profit_pct
        self.active_stop_loss_pct = self.stop_loss_pct
        self.active_trailing_stop_pct = self.trailing_stop_pct
        self.bars_in_pos = 0
        if reason.startswith("STOP_LOSS"):
            self.stop_loss_streak += 1
            self.cooldown = max(self.cooldown_bars, self.sl_cooldown_bars)
            if (
                self.max_consecutive_stop_losses > 0
                and self.stop_loss_streak >= self.max_consecutive_stop_losses
                and self.sl_pause_bars > 0
            ):
                self.pause_bars = max(self.pause_bars, self.sl_pause_bars)
                self.stop_loss_streak = 0
        else:
            self.cooldown = self.cooldown_bars
            self.stop_loss_streak = 0
        return self.buy(price, reason, tagged_metadata)

    def on_closed_candle(self, candle: Candle, is_warmup: bool = False):
        if not self.allow_new_candle(candle):
            return None

        if self.pause_bars > 0:
            self.pause_bars -= 1
            return None

        self.bar_count += 1
        if self.cooldown > 0:
            self.cooldown -= 1
            return None

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

        ema_spread_pct = (ema_fast - ema_slow) / ema_slow * 100.0 if ema_slow != 0 else 0.0
        vwap_gap_pct = (candle.close - vwap_px) / vwap_px * 100.0 if vwap_px != 0 else 0.0
        atr_pct = atr / candle.close * 100.0 if candle.close != 0 else 0.0
        distance_from_ema = (candle.close - ema_slow) / ema_slow * 100.0 if ema_slow != 0 else 0.0
        ret_1_pct = ((candle.close - self.prev_close) / self.prev_close * 100.0) if self.prev_close else 0.0
        rsi = self._update_rsi(candle.close)

        if is_warmup:
            self._update_bar_state(
                candle,
                macd_line,
                signal_line,
                macd_hist,
                vwap_px,
                atr,
                rsi,
                distance_from_ema,
                ema_spread_pct,
                vwap_gap_pct,
                atr_pct,
                ret_1_pct,
            )
            return None

        ai_payload = self._build_ai_payload(candle, ema_fast, ema_slow, macd_line, signal_line, macd_hist, vwap_px, atr, rsi)

        ai_action, ai_confidence, ai_reason, risk_feedback = self._ai_signal(ai_payload)
        queried = True

        if ai_action == "EXIT":
            if self.position_side == "LONG":
                ai_action = "SELL"
            elif self.position_side == "SHORT":
                ai_action = "BUY"
            else:
                ai_action = "HOLD"

        final_action = ai_action
        final_confidence = ai_confidence
        self.last_ai_action = ai_action
        self.last_ai_confidence = ai_confidence
        self.last_ai_reason = ai_reason

        trend_state = str(ai_payload.get("trend", {}).get("trend", "")).strip().lower()
        trend_same = (
            (self.position_side == "LONG" and trend_state == "bullish")
            or (self.position_side == "SHORT" and trend_state == "bearish")
        )

        if self.position_side in {"LONG", "SHORT"} and ai_confidence >= self.override_confidence:
            sl_pct = risk_feedback.get("suggested_sl_pct")
            if sl_pct is not None:
                self.active_stop_loss_pct = float(sl_pct)

            trailing_pct = risk_feedback.get("suggested_trailing_stop_pct")
            if trailing_pct is not None:
                self.active_trailing_stop_pct = float(trailing_pct)

            tp_pct = risk_feedback.get("suggested_tp_pct")
            if tp_pct is not None:
                self.active_take_profit_pct = float(tp_pct)

        print(
            (
                f"[AI_DECISION] symbol={candle.symbol or self.config.symbol} "
                f"interval={candle.interval or self.config.interval} "
                f"close={candle.close:.6f} ai={ai_action}:{ai_confidence:.4f} "
                f"final={final_action}:{final_confidence:.4f} "
                f"queried={queried} reason={ai_reason}"
            ),
            flush=True,
        )

        metadata = {
            "local_action": "DISABLED",
            "local_confidence": 0.0,
            "ai_action": ai_action,
            "ai_confidence": round(ai_confidence, 4),
            "ai_reason": ai_reason,
            "ai_risk_feedback": risk_feedback,
            "tp_roll_eligible": bool(self.ai_keep_position_on_tp and ai_confidence >= self.override_confidence and trend_same),
            "trend_same": trend_same,
            "active_take_profit_pct": round(self.active_take_profit_pct, 6),
            "active_stop_loss_pct": round(self.active_stop_loss_pct, 6),
            "active_trailing_stop_pct": round(self.active_trailing_stop_pct, 6),
            "tp_ladder_steps": self.tp_ladder_steps,
            "previous_trade_condition": ai_payload.get("previous_trade_condition", []),
            "ema_fast": round(ema_fast, 8),
            "ema_slow": round(ema_slow, 8),
            "vwap": round(vwap_px, 8),
            "macd_hist": round(macd_hist, 8),
            "atr": round(atr, 8),
            "rsi": round(rsi if rsi is not None else 50.0, 4),
        }

        signal = None
        risk_exit_signal = self._position_risk_exit(candle, metadata)
        if risk_exit_signal is not None:
            signal = risk_exit_signal
        elif self.cooldown == 0:
            if self.position_side == "LONG" and final_action == "SELL" and final_confidence >= self.entry_confidence:
                signal = self._close_long(candle.close, "EXIT_LONG_AI", metadata)
            elif self.position_side == "SHORT" and final_action == "BUY" and final_confidence >= self.entry_confidence:
                signal = self._close_short(candle.close, "EXIT_SHORT_AI", metadata)
            elif self.position_side is None and final_confidence >= self.entry_confidence:
                if final_action == "BUY":
                    self.position_side = "LONG"
                    self.entry_price = candle.close
                    self.highest_since_entry = candle.close
                    self.lowest_since_entry = candle.close
                    self.tp_anchor_price = candle.close
                    self.tp_ladder_steps = 0
                    self.active_take_profit_pct = self.take_profit_pct
                    self.active_stop_loss_pct = self.stop_loss_pct
                    self.active_trailing_stop_pct = self.trailing_stop_pct
                    if ai_confidence >= self.override_confidence:
                        if self.ai_suggested_tp_pct is not None:
                            self.active_take_profit_pct = float(self.ai_suggested_tp_pct)
                        if self.ai_suggested_sl_pct is not None:
                            self.active_stop_loss_pct = float(self.ai_suggested_sl_pct)
                        if self.ai_suggested_trailing_stop_pct is not None:
                            self.active_trailing_stop_pct = float(self.ai_suggested_trailing_stop_pct)
                    self.bars_in_pos = 0
                    self.cooldown = self.cooldown_bars
                    metadata["trade_condition"] = "ENTRY"
                    metadata["order_reason"] = "ENTER_LONG_AI"
                    metadata["exit_type"] = ""
                    signal = self.buy(candle.close, "ENTER_LONG_AI", metadata)
                elif final_action == "SELL":
                    self.position_side = "SHORT"
                    self.entry_price = candle.close
                    self.highest_since_entry = candle.close
                    self.lowest_since_entry = candle.close
                    self.tp_anchor_price = candle.close
                    self.tp_ladder_steps = 0
                    self.active_take_profit_pct = self.take_profit_pct
                    self.active_stop_loss_pct = self.stop_loss_pct
                    self.active_trailing_stop_pct = self.trailing_stop_pct
                    if ai_confidence >= self.override_confidence:
                        if self.ai_suggested_tp_pct is not None:
                            self.active_take_profit_pct = float(self.ai_suggested_tp_pct)
                        if self.ai_suggested_sl_pct is not None:
                            self.active_stop_loss_pct = float(self.ai_suggested_sl_pct)
                        if self.ai_suggested_trailing_stop_pct is not None:
                            self.active_trailing_stop_pct = float(self.ai_suggested_trailing_stop_pct)
                    self.bars_in_pos = 0
                    self.cooldown = self.cooldown_bars
                    metadata["trade_condition"] = "ENTRY"
                    metadata["order_reason"] = "ENTER_SHORT_AI"
                    metadata["exit_type"] = ""
                    signal = self.sell(candle.close, "ENTER_SHORT_AI", metadata)

        self._update_bar_state(
            candle,
            macd_line,
            signal_line,
            macd_hist,
            vwap_px,
            atr,
            rsi,
            distance_from_ema,
            ema_spread_pct,
            vwap_gap_pct,
            atr_pct,
            ret_1_pct,
        )
        return signal


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
        source=section.get("source", "python-ai-minimax-m2-7"),
        strategy_id=section.get("strategy_id", "python-ai-minimax-m2-7-hybrid"),
        need_notification=bool(section.get("need_notification", False)),
        is_paper_trading=section.get("is_paper_trading", True),
        order_type=section.get("order_type", "LIMIT"),
        order_qty=float(section.get("order_qty", 10)),
        limit_slippage_pct=float(section.get("limit_slippage_pct", section.get("limit_slippage_bps", 2) / 100.0)),
        enable_intrabar_risk_exit=bool(cfg_value(section, GLOBAL_CONFIG, "enable_intrabar_risk_exit", True)),
        monitor_enabled=bool(cfg_value(section, GLOBAL_CONFIG, "monitor_enabled", True)),
        monitor_host=str(section.get("monitor_host") or GLOBAL_CONFIG.get("monitor_host") or "127.0.0.1"),
        monitor_port=int(cfg_value(section, GLOBAL_CONFIG, "monitor_port", 19013)),
    )


def build_strategy_config(section: dict) -> StrategyConfig:
    return StrategyConfig(
        name=section.get("name", "AI-M2.7"),
        symbol="*",
        interval="*",
        warmup_limit=int(section.get("historical_limit", 800)),
    )


async def run():
    runtime = build_runtime_config(AI_CONFIG)

    if runtime.order_qty <= 0:
        raise ValueError("order_qty must be > 0")

    strategy = AIHybridStrategy(build_strategy_config(AI_CONFIG), AI_CONFIG)
    if strategy.ai_client is None:
        raise ValueError("LLM is required. Set api_key or API_KEY.")

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

    # Run one-time connectivity check before the runner starts.
    strategy._test_llm_on_init()

    runner = StrategyRunner(strategy=strategy, runtime=runtime)
    try:
        await runner.run()
    finally:
        if strategy.ai_client is not None:
            with suppress(Exception):
                strategy.ai_client.close()
        if strategy.http_client is not None:
            with suppress(Exception):
                strategy.http_client.close()


if __name__ == "__main__":
    asyncio.run(run())
