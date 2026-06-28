"use client";

import { RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
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
  refreshSeconds: number;
};

const symbols = ["BNB_USDT.P", "ETH_USDT.P", "HYPE_USDT.P", "SOL_USDT.P", "TRX_USDT.P"];
const indicators = ["ema_21", "ema_50", "ema_200", "vwap_100", "vwap_120", "rsi_14"];

export function StrategyMetricDetailPanel() {
  const [filters, setFilters] = useState<Filters>(() => defaultFilters());
  const [detail, setDetail] = useState<StrategyMetricDetail | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      const endTime = filters.refreshSeconds > 0 ? toDateTimeInput(new Date()) : filters.endTime;
      const next = await getStrategyMetricDetail({
        exchange: filters.exchange,
        market_type: filters.marketType,
        symbol: filters.symbol,
        interval: filters.interval,
        strategy: filters.strategy.trim(),
        start_time: toAPITime(filters.startTime),
        end_time: toAPITime(endTime),
        limit: filters.limit,
      });
      setDetail(next);
      if (filters.refreshSeconds > 0) {
        setFilters((current) => ({ ...current, endTime }));
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load strategy detail.");
    } finally {
      setIsLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (filters.refreshSeconds <= 0) {
      return;
    }
    const interval = window.setInterval(() => {
      void load();
    }, filters.refreshSeconds * 1000);
    return () => window.clearInterval(interval);
  }, [filters.refreshSeconds, load]);

  const availableIndicators = useMemo(() => {
    const names = new Set<string>(indicators);
    for (const kline of detail?.klines ?? []) {
      Object.keys(kline.indicators).forEach((name) => names.add(name));
    }
    return [...names].sort();
  }, [detail]);

  return (
    <section className="grid gap-5">
      <div className="grid gap-2 md:grid-cols-3 xl:grid-cols-8">
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

      <PriceChart klines={detail?.klines ?? []} orders={detail?.orders ?? []} indicator={filters.indicator} />

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

function PriceChart({ klines, orders, indicator }: { klines: StrategyMetricKline[]; orders: StrategyMetricOrder[]; indicator: string }) {
  const width = 960;
  const height = 360;
  const pad = 28;
  const points = klines.map((kline, index) => ({ index, close: Number(kline.close_price), indicator: kline.indicators[indicator] }));
  const values = points.flatMap((point) => Number.isFinite(point.indicator) ? [point.close, point.indicator] : [point.close]);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const x = (index: number) => pad + (index / Math.max(points.length - 1, 1)) * (width - pad * 2);
  const y = (value: number) => height - pad - ((value - min) / Math.max(max - min, 1)) * (height - pad * 2);
  const pricePath = points.map((point, index) => `${index === 0 ? "M" : "L"} ${x(point.index)} ${y(point.close)}`).join(" ");
  const indicatorPath = points.filter((point) => Number.isFinite(point.indicator)).map((point, index) => `${index === 0 ? "M" : "L"} ${x(point.index)} ${y(point.indicator as number)}`).join(" ");
  const orderMarkers = orders.map((order) => {
    const orderTime = new Date(order.created_at).getTime();
    const nearest = nearestKlineIndex(klines, orderTime);
    const price = Number(order.entry_price ?? order.exit_price ?? klines[nearest]?.close_price ?? 0);
    return { order, cx: x(nearest), cy: y(price) };
  });

  return (
    <div className="overflow-hidden rounded-md border bg-card">
      <div className="border-b px-4 py-3 text-sm font-medium">Price, {indicator}, and orders</div>
      <div className="overflow-x-auto p-3">
        <svg viewBox={`0 0 ${width} ${height}`} className="h-[360px] min-w-[960px]">
          <rect x="0" y="0" width={width} height={height} className="fill-background" />
          <path d={pricePath} fill="none" stroke="hsl(var(--primary))" strokeWidth="2" />
          <path d={indicatorPath} fill="none" stroke="hsl(var(--destructive))" strokeWidth="1.5" strokeDasharray="6 5" />
          {orderMarkers.map(({ order, cx, cy }) => (
            <circle key={order.id} cx={cx} cy={cy} r="5" className={order.side === "SHORT" ? "fill-destructive" : "fill-primary"}>
              <title>{`${order.side} ${order.trade_condition} ${formatDateTime(order.created_at)}`}</title>
            </circle>
          ))}
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
  return { exchange: "binance", marketType: "futures", symbol: "BNB_USDT.P", interval: "15m", strategy: "", startTime: toDateTimeInput(start), endTime: toDateTimeInput(end), limit: 200, indicator: "ema_21", refreshSeconds: 5 };
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
