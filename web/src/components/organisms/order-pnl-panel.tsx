"use client";

import { ChevronLeft, ChevronRight, RefreshCw, Search } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { listOrderTradePnL, type OrderTradePnL } from "@/lib/api-client";
import type { PaginationMeta, ResourceConfig } from "@/types/api";

type OrderPnLPanelProps = {
  resource: ResourceConfig;
};

type ReportFilters = {
  startTime: string;
  endTime: string;
  strategyID: string;
  symbol: string;
};

export function OrderPnLPanel({ resource }: OrderPnLPanelProps) {
  const [filters, setFilters] = useState<ReportFilters>(() => defaultFilters());
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<OrderTradePnL[]>([]);
  const [meta, setMeta] = useState<PaginationMeta | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");

  const loadItems = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      const result = await listOrderTradePnL({
        start_time: toAPITime(filters.startTime),
        end_time: toAPITime(filters.endTime),
        strategy_id: filters.strategyID.trim(),
        symbol: filters.symbol.trim(),
        page,
        limit: 25,
      });
      setItems(result.items);
      setMeta(result.meta);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load order PnL.");
    } finally {
      setIsLoading(false);
    }
  }, [filters, page]);

  useEffect(() => {
    void loadItems();
  }, [loadItems]);

  return (
    <section className="grid gap-5">
      <ReportToolbar
        filters={filters}
        isLoading={isLoading}
        onChange={(next) => {
          setFilters(next);
          setPage(1);
        }}
        onRefresh={loadItems}
      />

      {error ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-3">
        <Metric label="Total rows" value={String(meta?.totalItems ?? 0)} />
        <Metric label="Page profit" value={formatMoney(sum(items.map((item) => item.profit)))} tone={sum(items.map((item) => item.profit)) < 0 ? "loss" : "gain"} />
        <Metric label="Latest running PnL" value={formatMoney(Number(items[0]?.running_profit ?? 0))} tone={Number(items[0]?.running_profit ?? 0) < 0 ? "loss" : "gain"} />
      </div>

      <div className="overflow-hidden rounded-md border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1060px] text-left text-sm">
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
                  <tr key={item.entry_order_id} className="border-t">
                    <td className="max-w-48 truncate px-3 py-3 font-mono text-xs" title={item.entry_order_id}>{item.entry_order_id}</td>
                    <td className="max-w-48 truncate px-3 py-3">{item.strategy_id}</td>
                    <td className="px-3 py-3">{item.symbol}</td>
                    <td className="px-3 py-3"><Badge variant={["BUY", "LONG"].includes(item.side.toUpperCase()) ? "success" : "warning"}>{item.side}</Badge></td>
                    <td className="px-3 py-3 tabular-nums">{formatNumber(item.entry_price)}</td>
                    <td className="px-3 py-3 tabular-nums">{formatNumber(item.exit_price)}</td>
                    <td className="px-3 py-3 tabular-nums">{formatNumber(item.qty)}</td>
                    <td className={moneyCellClass(item.profit)}>{formatMoney(Number(item.profit))}</td>
                    <td className={moneyCellClass(item.running_profit)}>{formatMoney(Number(item.running_profit))}</td>
                    <td className="px-3 py-3">{formatDateTime(item.exit_time)}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td className="px-3 py-8 text-center text-muted-foreground" colSpan={resource.columns.length}>
                    No closed paired trades
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-between border-t px-3 py-3 text-sm text-muted-foreground">
          <span>
            Page {meta?.page ?? 1} of {meta?.totalPages ?? 0} - {meta?.totalItems ?? 0} rows
          </span>
          <div className="flex gap-2">
            <Button type="button" variant="outline" size="sm" disabled={!meta || meta.page <= 1 || isLoading} onClick={() => setPage((current) => current - 1)}>
              <ChevronLeft className="h-4 w-4" />
              Previous
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={!meta || meta.page >= meta.totalPages || isLoading} onClick={() => setPage((current) => current + 1)}>
              Next
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
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
      <div className={`mt-1 text-xl font-semibold tabular-nums ${toneClass}`}>{value}</div>
    </div>
  );
}

function defaultFilters(): ReportFilters {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - 7);
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

function sum(values: string[]) {
  return values.reduce((total, value) => total + Number(value || 0), 0);
}

function moneyCellClass(value: string) {
  return `px-3 py-3 tabular-nums ${Number(value) < 0 ? "text-destructive" : "text-primary"}`;
}

function formatNumber(value: string) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return value;
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 }).format(number);
}

function formatMoney(value: number) {
  return new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value);
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
