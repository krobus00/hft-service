"use client";

import { RefreshCw, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  listStrategyPerformanceReports,
  type StrategyPerformanceReport,
} from "@/lib/api-client";
import type { ResourceConfig } from "@/types/api";

type StrategyPerformancePanelProps = {
  resource: ResourceConfig;
};

type ReportFilters = {
  startTime: string;
  endTime: string;
  strategyID: string;
  symbol: string;
};

export function StrategyPerformancePanel({ resource }: StrategyPerformancePanelProps) {
  const [filters, setFilters] = useState<ReportFilters>(() => defaultFilters());
  const [items, setItems] = useState<StrategyPerformanceReport[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");

  const loadItems = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      const result = await listStrategyPerformanceReports({
        start_time: toAPITime(filters.startTime),
        end_time: toAPITime(filters.endTime),
        strategy_id: filters.strategyID.trim(),
        symbol: filters.symbol.trim(),
      });
      setItems(result);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load strategy performance.");
    } finally {
      setIsLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    void loadItems();
  }, [loadItems]);

  const summary = useMemo(() => summarize(items), [items]);

  return (
    <section className="grid gap-5">
      <ReportToolbar
        filters={filters}
        isLoading={isLoading}
        onChange={setFilters}
        onRefresh={loadItems}
      />

      {error ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Metric
          label="Total profit"
          value={formatMoney(summary.totalProfit)}
          tone={summary.totalProfit < 0 ? "loss" : "gain"}
        />
        <Metric label="Total trades" value={formatInteger(summary.totalTrades)} />
        <Metric label="Weighted win rate" value={formatPercent(summary.winRate)} />
        <Metric
          label="Best strategy"
          value={summary.bestStrategy || "-"}
          tone={summary.bestProfit < 0 ? "loss" : summary.bestStrategy ? "gain" : undefined}
        />
      </div>

      <div className="overflow-hidden rounded-md border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1120px] text-left text-sm">
            <thead className="bg-muted text-xs uppercase text-muted-foreground">
              <tr>
                {resource.columns.map((column) => (
                  <th key={column} className="px-3 py-3 font-medium">
                    {humanize(column)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {items.length > 0 ? (
                items.map((item) => (
                  <tr key={item.strategy_id} className="border-t">
                    <td className="max-w-80 truncate px-3 py-3 font-medium" title={item.strategy_id}>
                      {item.strategy_id}
                    </td>
                    <td className={moneyCellClass(item.total_profit)}>
                      {formatMoney(Number(item.total_profit))}
                    </td>
                    <td className="px-3 py-3 tabular-nums">{formatPercent(Number(item.win_rate))}</td>
                    <td className={moneyCellClass(item.avg_profit)}>
                      {formatMoney(Number(item.avg_profit))}
                    </td>
                    <td className="px-3 py-3 tabular-nums">{formatRatio(item.profit_factor)}</td>
                    <td className="px-3 py-3 tabular-nums">{item.total_trades}</td>
                    <td className="px-3 py-3 tabular-nums">{item.winning_trades}</td>
                    <td className="px-3 py-3 tabular-nums">{item.losing_trades}</td>
                    <td className={moneyCellClass(item.best_trade)}>
                      {formatMoney(Number(item.best_trade))}
                    </td>
                    <td className={moneyCellClass(item.worst_trade)}>
                      {formatMoney(Number(item.worst_trade))}
                    </td>
                    <td className="px-3 py-3 text-muted-foreground">
                      {formatDateTime(item.last_trade_at)}
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td className="px-3 py-8 text-center text-muted-foreground" colSpan={resource.columns.length}>
                    No strategy performance rows
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="border-t px-3 py-3 text-sm text-muted-foreground">
          Showing {items.length} strategies
        </div>
      </div>
    </section>
  );
}

function ReportToolbar({
  filters,
  isLoading,
  onChange,
  onRefresh,
}: {
  filters: ReportFilters;
  isLoading: boolean;
  onChange: (filters: ReportFilters) => void;
  onRefresh: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
      <div className="grid w-full gap-2 sm:grid-cols-2 xl:max-w-5xl xl:grid-cols-4">
        <Input type="datetime-local" value={filters.startTime} onChange={(event) => onChange({ ...filters, startTime: event.target.value })} aria-label="Start time" />
        <Input type="datetime-local" value={filters.endTime} onChange={(event) => onChange({ ...filters, endTime: event.target.value })} aria-label="End time" />
        <label className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={filters.strategyID} onChange={(event) => onChange({ ...filters, strategyID: event.target.value })} placeholder="Strategy" className="pl-9" />
        </label>
        <Input value={filters.symbol} onChange={(event) => onChange({ ...filters, symbol: event.target.value })} placeholder="Symbol" />
      </div>
      <Button type="button" variant="outline" size="sm" onClick={onRefresh} disabled={isLoading}>
        <RefreshCw className={isLoading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
        Refresh
      </Button>
    </div>
  );
}

function Metric({ label, value, tone }: { label: string; value: string; tone?: "gain" | "loss" }) {
  const toneClass = tone === "loss" ? "text-destructive" : tone === "gain" ? "text-primary" : "text-foreground";
  return (
    <div className="rounded-md border bg-card px-4 py-3">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className={`mt-1 truncate text-xl font-semibold tabular-nums ${toneClass}`}>{value}</div>
    </div>
  );
}

function summarize(items: StrategyPerformanceReport[]) {
  const totalProfit = items.reduce((total, item) => total + Number(item.total_profit || 0), 0);
  const totalTrades = items.reduce((total, item) => total + item.total_trades, 0);
  const winningTrades = items.reduce((total, item) => total + item.winning_trades, 0);
  const best = [...items].sort((left, right) => Number(right.total_profit) - Number(left.total_profit))[0];
  return {
    totalProfit,
    totalTrades,
    winRate: totalTrades > 0 ? winningTrades / totalTrades : 0,
    bestStrategy: best?.strategy_id ?? "",
    bestProfit: Number(best?.total_profit ?? 0),
  };
}

function defaultFilters(): ReportFilters {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - 30);
  return {
    startTime: toDateTimeInput(start),
    endTime: toDateTimeInput(end),
    strategyID: "",
    symbol: "",
  };
}

function toDateTimeInput(date: Date) {
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function toAPITime(value: string) {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toISOString();
}

function moneyCellClass(value: string) {
  return `px-3 py-3 tabular-nums ${Number(value) < 0 ? "text-destructive" : "text-primary"}`;
}

function formatInteger(value: number) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
}

function formatMoney(value: number) {
  return new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value);
}

function formatPercent(value: number) {
  return new Intl.NumberFormat(undefined, { style: "percent", minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(value);
}

function formatRatio(value: string) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return value;
  }
  return new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(number);
}

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function humanize(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
