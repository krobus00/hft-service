import time
import uuid
from datetime import datetime
from pathlib import Path
from typing import Any, Dict

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
