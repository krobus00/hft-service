import time
import uuid
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Optional

import yaml

BASE_DIR = Path(__file__).resolve().parent.parent
CONFIG_PATH = BASE_DIR / "config.yml"


def load_full_config() -> Dict[str, Any]:
    if not CONFIG_PATH.exists():
        raise FileNotFoundError(
            f"Config file not found: {CONFIG_PATH}. Copy strategy/config.yml.example to strategy/config.yml"
        )
    with CONFIG_PATH.open("r", encoding="utf-8") as file:
        return yaml.safe_load(file) or {}


def cfg_value(section: Dict[str, Any], global_config: Dict[str, Any], key: str, default: Any) -> Any:
    if key in section and section.get(key) is not None:
        return section.get(key)

    strategy_configs = global_config.get("strategy_configs", {}) if isinstance(global_config, dict) else {}
    if isinstance(strategy_configs, dict) and key in strategy_configs and strategy_configs.get(key) is not None:
        return strategy_configs.get(key)

    risk_controls = global_config.get("risk_controls", {}) if isinstance(global_config, dict) else {}
    if isinstance(risk_controls, dict) and key in risk_controls and risk_controls.get(key) is not None:
        return risk_controls.get(key)

    if key in global_config and global_config.get(key) is not None:
        return global_config.get(key)

    return default


def runtime_options(global_config: Dict[str, Any], section: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """Shared non-strategy runtime features.

    Strategy files pass this into RuntimeConfig so monitoring, config refresh,
    and state persistence stay consistent across all strategy runners.
    """

    section = section or {}
    return {
        "strategy_config_refresh_interval_sec": int(global_config.get("strategy_config_refresh_interval_sec", 10)),
        "monitor_enabled": bool(section.get("monitor_enabled", global_config.get("monitor_enabled", False))),
        "monitor_host": str(section.get("monitor_host", global_config.get("monitor_host", "127.0.0.1"))),
        "monitor_port": int(section.get("monitor_port", global_config.get("monitor_port", 8088))),
        "redis_url": str(global_config.get("redis_url", "") or ""),
        "redis_key_prefix": str(global_config.get("redis_key_prefix", "hft:strategy")),
        "redis_state_ttl_sec": int(global_config.get("redis_state_ttl_sec", 86400)),
        "redis_delete_removed_state": bool(global_config.get("redis_delete_removed_state", False)),
        "db_pool_min_size": int(global_config.get("db_pool_min_size", 1)),
        "db_pool_max_size": int(global_config.get("db_pool_max_size", 4)),
        "message_queue_size": int(global_config.get("message_queue_size", 1000)),
    }


def parse_iso_to_ms(value: str) -> int:
    dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
    return int(dt.timestamp() * 1000)


def now_ms() -> int:
    return int(time.time() * 1000)


def gen_id() -> str:
    return uuid.uuid4().hex


def pct_to_frac(pct: float) -> float:
    return pct / 100.0


def fmt_num(value: float) -> str:
    return f"{value:.8f}".rstrip("0").rstrip(".")
