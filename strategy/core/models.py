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
    order_subject: str
    position_side: str
    source: str
    strategy_id: str
    need_notification: bool
    is_paper_trading: bool
    order_type: str
    order_qty: float
    limit_slippage_pct: float
    enable_intrabar_risk_exit: bool = False
