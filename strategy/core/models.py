from dataclasses import dataclass, field
from typing import Any, Dict, Optional


@dataclass
class Candle:
    close_time_ms: int
    close: float
    quote_volume: float
    symbol: str = ""
    interval: str = ""
    high: Optional[float] = None
    low: Optional[float] = None
    taker_quote_volume: float = 0.0
    trade_count: int = 0


@dataclass
class Signal:
    side: str
    price: float
    reason: str
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class StrategyConfig:
    name: str
    symbol: str
    interval: str
    warmup_limit: int


@dataclass
class RuntimeConfig:
    db_dsn: str
    nats_url: str
    nats_allow_reconnect: bool
    nats_max_reconnect_attempts: int
    nats_reconnect_time_wait_sec: int
    nats_connect_timeout_sec: int
    nats_ping_interval_sec: int
    nats_max_outstanding_pings: int
    position_side: str = "BOTH"
    source: str = "python-strategy"
    strategy_id: str = ""
    need_notification: bool = False
    is_paper_trading: bool = True
    order_type: str = "LIMIT"
    order_qty: float = 0.0
    limit_slippage_pct: float = 0.0
    enable_intrabar_risk_exit: bool = False
    strategy_config_refresh_interval_sec: int = 10
    monitor_enabled: bool = False
    monitor_host: str = "127.0.0.1"
    monitor_port: int = 8088
    redis_url: str = ""
    redis_key_prefix: str = "hft:strategy"
    redis_state_ttl_sec: int = 86400
    redis_delete_removed_state: bool = False
    db_pool_min_size: int = 1
    db_pool_max_size: int = 4
    message_queue_size: int = 1000
