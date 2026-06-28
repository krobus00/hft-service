"use client";

import { Eye, EyeOff, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  buildStrategyMetricDetailWebSocketURL,
  getStrategyMetricDetail,
  type StrategyMetricDetail,
  type StrategyMetricKline,
  type StrategyMetricOrder,
} from "@/lib/api-client";

type Filters = {
  exchange: string;
  marketType: string;
  symbol: string;
  interval: string;
  strategy: string;
  startTime: string;
  endTime: string;
  limit: number;
  indicator: string;
  showIndicator: boolean;
  refreshSeconds: number;
};

const symbols = ["BNB_USDT.P", "ETH_USDT.P", "HYPE_USDT.P", "SOL_USDT.P", "TRX_USDT.P"];
const indicators = ["ema_21", "ema_50", "ema_200", "vwap_100", "vwap_120", "rsi_14"];

export function StrategyMetricDetailPanel() {
  const [filters, setFilters] = useState<Filters>(() => defaultFilters());
  const [detail, setDetail] = useState<StrategyMetricDetail | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");
  const [streamState, setStreamState] = useState("Disconnected");

  const load = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      const next = await getStrategyMetricDetail(metricQuery(filters, false));
      setDetail(next);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load strategy detail.");
    } finally {
      setIsLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    if (filters.refreshSeconds === 0) {
      setStreamState("Manual");
      void load();
      return;
    }
    setIsLoading(true);
    setError("");
    setStreamState("Connecting");
    let socket: WebSocket;
    try {
      socket = new WebSocket(buildStrategyMetricDetailWebSocketURL(metricQuery(filters, true)));
    } catch (caught) {
      setIsLoading(false);
      setStreamState("Disconnected");
      setError(caught instanceof Error ? caught.message : "Unable to connect realtime stream.");
      return;
    }
    socket.onopen = () => setStreamState("Live");
    socket.onmessage = (event) => {
      const payload = JSON.parse(event.data) as StrategyMetricDetail | { error?: string };
      if ("error" in payload && payload.error) {
        setError(payload.error);
        return;
      }
      setDetail(payload as StrategyMetricDetail);
      setIsLoading(false);
    };
    socket.onerror = () => {
      setError("Realtime stream disconnected.");
      setStreamState("Disconnected");
      setIsLoading(false);
    };
    socket.onclose = () => {
      setStreamState("Disconnected");
      setIsLoading(false);
    };
    return () => socket.close();
  }, [filters, load]);

  const availableIndicators = useMemo(() => {
    const names = new Set<string>(indicators);
    for (const kline of detail?.klines ?? []) {
      Object.keys(kline.indicators).forEach((name) => names.add(name));
    }
    return [...names].sort();
  }, [detail]);

  return (
    <section className="grid gap-5">
      <div className="grid gap-2 md:grid-cols-3 xl:grid-cols-9">
        <Select value={filters.symbol} onChange={(event) => setFilters({ ...filters, symbol: event.target.value })}>
          {symbols.map((symbol) => <option key={symbol}>{symbol}</option>)}
        </Select>
        <Input value={filters.strategy} onChange={(event) => setFilters({ ...filters, strategy: event.target.value })} placeholder="Strategy" />
        <Input value={filters.interval} onChange={(event) => setFilters({ ...filters, interval: event.target.value })} placeholder="Interval" />
        <Input type="datetime-local" value={filters.startTime} onChange={(event) => setFilters({ ...filters, startTime: event.target.value })} />
        <Input type="datetime-local" value={filters.endTime} onChange={(event) => setFilters({ ...filters, endTime: event.target.value })} />
        <Input type="number" min={50} max={500} value={filters.limit} onChange={(event) => setFilters({ ...filters, limit: Number(event.target.value) || 200 })} />
        <Select value={filters.indicator} onChange={(event) => setFilters({ ...filters, indicator: event.target.value })}>
          {availableIndicators.map((name) => <option key={name}>{name}</option>)}
        </Select>
        <Button
          type="button"
          variant={filters.showIndicator ? "default" : "outline"}
          onClick={() => setFilters({ ...filters, showIndicator: !filters.showIndicator })}
        >
          {filters.showIndicator ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
          Indicator
        </Button>
        <Select value={String(filters.refreshSeconds)} onChange={(event) => setFilters({ ...filters, refreshSeconds: Number(event.target.value) })}>
          <option value="0">Manual</option>
          <option value="5">5s</option>
          <option value="15">15s</option>
          <option value="30">30s</option>
        </Select>
        <Button type="button" variant="outline" onClick={load} disabled={isLoading}>
          <RefreshCw className={isLoading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          Refresh
        </Button>
      </div>

      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p> : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <Metric label="Candles" value={detail?.summary.kline_count ?? 0} />
        <Metric label="Missing indicators" value={detail?.summary.missing_indicator_results ?? 0} />
        <Metric label="Orders" value={detail?.summary.order_count ?? 0} />
        <Metric label="Entries" value={detail?.summary.entry_count ?? 0} />
        <Metric label="Exits" value={detail?.summary.exit_count ?? 0} />
      </div>
      <div className="text-sm text-muted-foreground">Stream: {streamState}</div>

      <PriceChart klines={detail?.klines ?? []} orders={detail?.orders ?? []} indicator={filters.indicator} showIndicator={filters.showIndicator} />

      <div className="overflow-hidden rounded-md border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[980px] text-left text-sm">
            <thead className="bg-muted text-xs uppercase text-muted-foreground">
              <tr>
                {["time", "strategy", "side", "condition", "status", "price", "pnl", "reason"].map((column) => (
                  <th key={column} className="px-3 py-3 font-medium">{column}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(detail?.orders ?? []).map((order) => (
                <tr key={order.id} className="border-t">
                  <td className="px-3 py-3 text-muted-foreground">{formatDateTime(order.created_at)}</td>
                  <td className="max-w-60 truncate px-3 py-3">{order.strategy_id ?? "-"}</td>
                  <td className="px-3 py-3">{order.side}</td>
                  <td className="px-3 py-3">{order.trade_condition}</td>
                  <td className="px-3 py-3">{order.status}</td>
                  <td className="px-3 py-3 tabular-nums">{order.entry_price ?? order.exit_price ?? "-"}</td>
                  <td className={Number(order.pnl ?? 0) < 0 ? "px-3 py-3 tabular-nums text-destructive" : "px-3 py-3 tabular-nums text-primary"}>{order.pnl ?? "-"}</td>
                  <td className="max-w-72 truncate px-3 py-3 text-muted-foreground">{order.order_reason}</td>
                </tr>
              ))}
              {detail && detail.orders.length === 0 ? (
                <tr><td className="px-3 py-8 text-center text-muted-foreground" colSpan={8}>No orders in range</td></tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function PriceChart({ klines, orders, indicator, showIndicator }: { klines: StrategyMetricKline[]; orders: StrategyMetricOrder[]; indicator: string; showIndicator: boolean }) {
  const width = 1120;
  const height = 520;
  const padX = 54;
  const priceTop = 24;
  const priceBottom = 380;
  const volumeTop = 410;
  const volumeBottom = 496;
  const candles = klines.map((kline, index) => ({
    index,
    time: new Date(kline.close_time).getTime(),
    open: Number(kline.open_price),
    high: Number(kline.high_price),
    low: Number(kline.low_price),
    close: Number(kline.close_price),
    volume: Number(kline.volume),
    indicator: kline.indicators[indicator],
  }));
  const priceValues = candles.flatMap((candle) => showIndicator && Number.isFinite(candle.indicator)
    ? [candle.high, candle.low, candle.indicator as number]
    : [candle.high, candle.low]);
  const min = priceValues.length > 0 ? Math.min(...priceValues) : 0;
  const max = priceValues.length > 0 ? Math.max(...priceValues) : 1;
  const maxVolume = Math.max(...candles.map((candle) => candle.volume), 1);
  const step = (width - padX * 2) / Math.max(candles.length, 1);
  const bodyWidth = Math.max(3, Math.min(12, step * 0.62));
  const x = (index: number) => padX + step * index + step / 2;
  const y = (value: number) => priceBottom - ((value - min) / Math.max(max - min, 1)) * (priceBottom - priceTop);
  const volumeY = (value: number) => volumeBottom - (value / maxVolume) * (volumeBottom - volumeTop);
  const indicatorPath = candles
    .filter((candle) => Number.isFinite(candle.indicator))
    .map((candle, index) => `${index === 0 ? "M" : "L"} ${x(candle.index)} ${y(candle.indicator as number)}`)
    .join(" ");
  const orderMarkers = orders.map((order) => {
    const orderTime = new Date(order.created_at).getTime();
    const nearest = nearestKlineIndex(klines, orderTime);
    const price = Number(order.entry_price ?? order.exit_price ?? klines[nearest]?.close_price ?? 0);
    return { order, cx: x(nearest), cy: y(price) };
  });
  const gridLines = [0, 1, 2, 3, 4].map((item) => priceTop + ((priceBottom - priceTop) / 4) * item);

  return (
    <div className="overflow-hidden rounded-md border bg-card">
      <div className="flex items-center justify-between border-b px-4 py-3 text-sm">
        <span className="font-medium">Candles, volume, orders{showIndicator ? `, ${indicator}` : ""}</span>
        <span className="text-muted-foreground">{klines.length} candles</span>
      </div>
      <div className="overflow-x-auto p-3">
        <svg viewBox={`0 0 ${width} ${height}`} className="h-[520px] min-w-[1120px]">
          <rect x="0" y="0" width={width} height={height} className="fill-background" />
          {gridLines.map((line) => (
            <line key={line} x1={padX} x2={width - padX} y1={line} y2={line} className="stroke-border" strokeDasharray="4 6" />
          ))}
          <line x1={padX} x2={width - padX} y1={volumeTop} y2={volumeTop} className="stroke-border" />
          {candles.map((candle) => {
            const bullish = candle.close >= candle.open;
            const colorClass = bullish ? "stroke-primary fill-primary/70" : "stroke-destructive fill-destructive/70";
            const bodyTop = Math.min(y(candle.open), y(candle.close));
            const bodyHeight = Math.max(1, Math.abs(y(candle.open) - y(candle.close)));
            return (
              <g key={`${candle.time}-${candle.index}`}>
                <line x1={x(candle.index)} x2={x(candle.index)} y1={y(candle.high)} y2={y(candle.low)} className={colorClass} />
                <rect x={x(candle.index) - bodyWidth / 2} y={bodyTop} width={bodyWidth} height={bodyHeight} rx="1" className={colorClass} />
                <rect x={x(candle.index) - bodyWidth / 2} y={volumeY(candle.volume)} width={bodyWidth} height={volumeBottom - volumeY(candle.volume)} className={bullish ? "fill-primary/25" : "fill-destructive/25"} />
              </g>
            );
          })}
          {showIndicator && indicatorPath ? <path d={indicatorPath} fill="none" stroke="hsl(var(--ring))" strokeWidth="2" /> : null}
          {orderMarkers.map(({ order, cx, cy }) => (
            <circle key={order.id} cx={cx} cy={cy} r="5" className={order.side === "SHORT" ? "fill-destructive" : "fill-primary"}>
              <title>{`${order.side} ${order.trade_condition} ${formatDateTime(order.created_at)}`}</title>
            </circle>
          ))}
          {candles.length === 0 ? (
            <text x={width / 2} y={height / 2} textAnchor="middle" className="fill-muted-foreground text-sm">No candles in range</text>
          ) : null}
        </svg>
      </div>
    </div>
  );
}

function nearestKlineIndex(klines: StrategyMetricKline[], time: number) {
  let best = 0;
  let bestDelta = Number.POSITIVE_INFINITY;
  klines.forEach((kline, index) => {
    const delta = Math.abs(new Date(kline.close_time).getTime() - time);
    if (delta < bestDelta) {
      best = index;
      bestDelta = delta;
    }
  });
  return best;
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border bg-card px-4 py-3">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-xl font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function defaultFilters(): Filters {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - 1);
  return { exchange: "binance", marketType: "futures", symbol: "BNB_USDT.P", interval: "15m", strategy: "", startTime: toDateTimeInput(start), endTime: toDateTimeInput(end), limit: 200, indicator: "ema_21", showIndicator: true, refreshSeconds: 5 };
}

function metricQuery(filters: Filters, realtime: boolean) {
  return {
    exchange: filters.exchange,
    market_type: filters.marketType,
    symbol: filters.symbol,
    interval: filters.interval,
    strategy: filters.strategy.trim(),
    start_time: toAPITime(filters.startTime),
    end_time: realtime ? undefined : toAPITime(filters.endTime),
    limit: filters.limit,
    refresh: filters.refreshSeconds,
  };
}

function toDateTimeInput(date: Date) {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

function toAPITime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toISOString();
}

function formatDateTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(undefined, { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(date);
}
