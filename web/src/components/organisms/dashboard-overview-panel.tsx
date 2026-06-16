"use client";

import {
  Activity,
  ArrowDownRight,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  Loader2,
  RefreshCw,
  Target,
  TrendingUp,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { listResource } from "@/lib/api-client";
import { resources } from "@/lib/resources";
import { cn } from "@/lib/utils";

type OrderHistory = {
  id?: string;
  exchange?: string;
  market_type?: string;
  symbol?: string;
  order_id?: string;
  entry_order_id?: string;
  side?: string;
  quantity?: string | number;
  filled_quantity?: string | number;
  avg_fill_price?: string | number | null;
  price?: string | number | null;
  realized_pnl?: string | number | null;
  status?: string;
  strategy_id?: string | null;
  trade_condition?: string;
  created_at?: string;
  filled_at?: string | null;
};

type MarketKline = {
  close_price?: string | number;
  open_time?: string;
};

type DashboardTrade = {
  id: string;
  strategyID: string;
  symbol: string;
  side: string;
  qty: number;
  entryPrice: number;
  exitPrice?: number;
  markPrice?: number;
  pnl?: number;
  status: "closed" | "running";
  time: string;
};

type OverviewData = {
  orders: OrderHistory[];
  trades: DashboardTrade[];
  runningTrades: DashboardTrade[];
};

export function DashboardOverviewPanel() {
  const [data, setData] = useState<OverviewData>({
    orders: [],
    trades: [],
    runningTrades: [],
  });
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState("");
  const [tradePage, setTradePage] = useState(1);

  const loadOverview = useCallback(async () => {
    setIsLoading(true);
    setError("");

    const end = new Date();
    const start = new Date(end.getTime() - 24 * 60 * 60 * 1000);

    try {
      const orders = await listLast24hOrders(start, end);
      const runningEntries = findRunningEntries(orders);
      const markPrices = await loadMarkPrices(runningEntries);
      const trades = buildTradeRows(orders, markPrices);

      setData({
        orders,
        trades,
        runningTrades: trades.filter((trade) => trade.status === "running"),
      });
      setTradePage(1);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load dashboard overview.");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadOverview();
  }, [loadOverview]);

  const stats = useMemo(() => buildStats(data), [data]);
  const totalTradePages = Math.max(1, Math.ceil(data.trades.length / tradePageSize));
  const currentTradePage = Math.min(tradePage, totalTradePages);
  const pagedTrades = data.trades.slice(
    (currentTradePage - 1) * tradePageSize,
    currentTradePage * tradePageSize,
  );

  return (
    <section className="grid gap-5">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-sm font-medium text-muted-foreground">Last 24 hours</p>
          <h2 className="mt-1 text-2xl font-semibold tracking-normal">Trading dashboard</h2>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={loadOverview} disabled={isLoading}>
          <RefreshCw className={cn("h-4 w-4", isLoading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {error ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          label="24h profit"
          value={formatMoney(stats.closedProfit)}
          tone={stats.closedProfit < 0 ? "loss" : "gain"}
          icon={stats.closedProfit < 0 ? ArrowDownRight : ArrowUpRight}
          detail="Realized PnL from closed trades"
        />
        <MetricCard
          label="Total trade"
          value={formatInteger(data.orders.length)}
          icon={Activity}
          detail="Order history rows in range"
        />
        <MetricCard
          label="Win rate"
          value={formatPercent(stats.winRate)}
          icon={Target}
          detail={`${formatInteger(stats.closedTrades)} closed trades`}
        />
        <MetricCard
          label="Running PnL"
          value={formatMoney(stats.runningPnl)}
          tone={stats.runningPnl < 0 ? "loss" : "gain"}
          icon={TrendingUp}
          detail={`${formatInteger(data.runningTrades.length)} running trades`}
        />
      </div>

      <div className="grid min-w-0 gap-5 xl:grid-cols-[minmax(0,1fr)_300px]">
        <Card className="min-w-0 overflow-hidden rounded-md">
          <CardHeader className="flex-row items-center justify-between gap-3 border-b p-4">
            <CardTitle className="text-base">Recent order history with PnL</CardTitle>
            <span className="shrink-0 text-xs text-muted-foreground">
              {formatInteger(data.trades.length)} rows
            </span>
          </CardHeader>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[860px] text-left text-sm">
                <thead className="bg-muted text-xs uppercase text-muted-foreground">
                  <tr>
                    <th className="px-3 py-3 font-medium">Symbol</th>
                    <th className="px-3 py-3 font-medium">Strategy</th>
                    <th className="px-3 py-3 font-medium">Side</th>
                    <th className="px-3 py-3 font-medium">State</th>
                    <th className="px-3 py-3 font-medium">Qty</th>
                    <th className="px-3 py-3 font-medium">Entry</th>
                    <th className="px-3 py-3 font-medium">Exit / Mark</th>
                    <th className="px-3 py-3 font-medium">PnL</th>
                    <th className="px-3 py-3 font-medium">Time</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoading ? (
                    <tr>
                      <td className="px-3 py-10 text-center text-muted-foreground" colSpan={9}>
                        <span className="inline-flex items-center gap-2">
                          <Loader2 className="h-4 w-4 animate-spin" />
                          Loading overview
                        </span>
                      </td>
                    </tr>
                  ) : pagedTrades.length > 0 ? (
                    pagedTrades.map((trade) => (
                      <tr key={trade.id} className="border-t">
                        <td className="px-3 py-3 font-medium">{trade.symbol}</td>
                        <td className="max-w-48 truncate px-3 py-3 text-muted-foreground">
                          {trade.strategyID || "-"}
                        </td>
                        <td className="px-3 py-3">
                          <Badge variant="outline">{trade.side}</Badge>
                        </td>
                        <td className="px-3 py-3">
                          <Badge variant={trade.status === "running" ? "secondary" : "outline"}>
                            {trade.status}
                          </Badge>
                        </td>
                        <td className="px-3 py-3 tabular-nums">{formatNumberValue(trade.qty)}</td>
                        <td className="px-3 py-3 tabular-nums">{formatNumberValue(trade.entryPrice)}</td>
                        <td className="px-3 py-3 tabular-nums">
                          {formatNumberValue(trade.exitPrice ?? trade.markPrice)}
                        </td>
                        <td className={moneyCellClass(trade.pnl)}>
                          {trade.pnl == null ? "-" : formatMoney(trade.pnl)}
                        </td>
                        <td className="px-3 py-3 text-muted-foreground">
                          {formatDateTime(trade.time)}
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td className="px-3 py-10 text-center text-muted-foreground" colSpan={9}>
                        No order history rows in the last 24 hours
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex flex-col gap-2 border-t px-3 py-3 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
              <span>
                Page {currentTradePage} of {totalTradePages} - {formatInteger(data.trades.length)} rows
              </span>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={isLoading || currentTradePage <= 1}
                  onClick={() => setTradePage((page) => Math.max(1, page - 1))}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={isLoading || currentTradePage >= totalTradePages}
                  onClick={() => setTradePage((page) => Math.min(totalTradePages, page + 1))}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="min-w-0 self-start rounded-md">
          <CardHeader className="flex-row items-center justify-between gap-3 border-b p-4">
            <CardTitle className="text-base">Running trades</CardTitle>
            <span className="shrink-0 text-xs text-muted-foreground">
              {formatInteger(data.runningTrades.length)}
            </span>
          </CardHeader>
          <CardContent className="max-h-[420px] min-w-0 overflow-y-auto p-3">
            {isLoading ? (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading orders
              </p>
            ) : data.runningTrades.length > 0 ? (
              <div className="grid min-w-0 gap-2">
                {data.runningTrades.map((trade) => (
                  <div key={trade.id} className="min-w-0 rounded-md border bg-background px-3 py-2">
                    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2">
                      <p className="min-w-0 truncate text-sm font-medium" title={trade.symbol}>
                        {trade.symbol}
                      </p>
                      <span className={cn("text-xs font-semibold tabular-nums", pnlTone(trade.pnl))}>
                        {trade.pnl == null ? "-" : formatMoney(trade.pnl)}
                      </span>
                    </div>
                    <div className="mt-1 grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 text-xs text-muted-foreground">
                      <span className="min-w-0 truncate" title={trade.strategyID || "-"}>
                        {trade.strategyID || "-"} - {trade.side}
                      </span>
                      <span className="shrink-0 tabular-nums">
                        entry {formatNumberValue(trade.entryPrice)}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="rounded-md border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
                No unpaired entry orders
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

function buildStats(data: OverviewData) {
  const closedTrades = data.trades.filter((trade) => trade.status === "closed");
  const closedProfit = closedTrades.reduce((total, trade) => total + (trade.pnl ?? 0), 0);
  const winningTrades = closedTrades.filter((trade) => (trade.pnl ?? 0) > 0).length;
  const runningPnl = data.runningTrades.reduce((total, trade) => total + (trade.pnl ?? 0), 0);
  return {
    closedProfit,
    closedTrades: closedTrades.length,
    runningPnl,
    winRate: closedTrades.length > 0 ? winningTrades / closedTrades.length : 0,
  };
}

function MetricCard({
  label,
  value,
  detail,
  tone,
  icon: Icon,
}: {
  label: string;
  value: string;
  detail: string;
  tone?: "gain" | "loss";
  icon: typeof Activity;
}) {
  return (
    <Card className="rounded-md">
      <CardContent className="flex min-h-32 items-start justify-between gap-3 p-4">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase text-muted-foreground">{label}</p>
          <p className={cn("mt-3 truncate text-2xl font-semibold tabular-nums", toneClass(tone))}>
            {value}
          </p>
          <p className="mt-2 truncate text-xs text-muted-foreground">{detail}</p>
        </div>
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground">
          <Icon className="h-4 w-4" />
        </span>
      </CardContent>
    </Card>
  );
}

const orderResource = resources.find((resource) => resource.key === "orders") ?? resources[0];
const marketKlineResource =
  resources.find((resource) => resource.key === "marketKlines") ?? resources[0];
const tradePageSize = 10;

async function listLast24hOrders(start: Date, end: Date) {
  const firstPage = await listResource(orderResource, {
    page: 1,
    limit: 100,
    keyword: "",
    filters: { created_at: { op: "between", value: [start.toISOString(), end.toISOString()] } },
    sortField: "created_at",
    sortDirection: "desc",
  });
  const items = firstPage.items.map((item) => item as OrderHistory);
  for (let page = 2; page <= firstPage.meta.totalPages; page += 1) {
    const result = await listResource(orderResource, {
      page,
      limit: 100,
      keyword: "",
      filters: { created_at: { op: "between", value: [start.toISOString(), end.toISOString()] } },
      sortField: "created_at",
      sortDirection: "desc",
    });
    items.push(...result.items.map((item) => item as OrderHistory));
  }
  return items;
}

function buildTradeRows(orders: OrderHistory[], markPrices: Map<string, number>) {
  const groups = groupOrders(orders);
  const rows: DashboardTrade[] = [];

  for (const group of groups.values()) {
    const entry = newest(group.entries) ?? inferEntryOrder(group.orders);
    if (!entry) {
      continue;
    }

    const exit = newest(group.orders.filter((order) => order !== entry && isExitOrder(order)));
    const entryPrice = orderPrice(entry);
    const qty = orderQuantity(entry);
    if (!entryPrice || !qty) {
      continue;
    }

    if (exit) {
      const exitPrice = orderPrice(exit);
      const pnl = orderRealizedPnl(exit) ?? calculatePnl(entry.side, entryPrice, exitPrice, qty);
      rows.push({
        id: `closed:${entry.id ?? group.key}`,
        strategyID: entry.strategy_id ?? exit.strategy_id ?? "",
        symbol: entry.symbol ?? "-",
        side: entry.side ?? "-",
        qty,
        entryPrice,
        exitPrice,
        pnl,
        status: "closed",
        time: exit.filled_at ?? exit.created_at ?? entry.created_at ?? "",
      });
      continue;
    }

    const markPrice = markPrices.get(marketKey(entry));
    rows.push({
      id: `running:${entry.id ?? group.key}`,
      strategyID: entry.strategy_id ?? "",
      symbol: entry.symbol ?? "-",
      side: entry.side ?? "-",
      qty,
      entryPrice,
      markPrice,
      pnl: calculatePnl(entry.side, entryPrice, markPrice, qty),
      status: "running",
      time: entry.filled_at ?? entry.created_at ?? "",
    });
  }

  return rows.sort((left, right) => dateValue(right.time) - dateValue(left.time));
}

function findRunningEntries(orders: OrderHistory[]) {
  return Array.from(groupOrders(orders).values())
    .filter((group) => group.orders.length > 0 && !group.orders.some(isExitOrder))
    .map((group) => newest(group.entries) ?? inferEntryOrder(group.orders))
    .filter((order): order is OrderHistory => Boolean(order));
}

function groupOrders(orders: OrderHistory[]) {
  const groups = new Map<string, { key: string; orders: OrderHistory[]; entries: OrderHistory[] }>();
  for (const order of orders) {
    const key = groupKey(order);
    if (!key) {
      continue;
    }
    const group = groups.get(key) ?? { key, orders: [], entries: [] };
    group.orders.push(order);
    if (isEntryOrder(order)) {
      group.entries.push(order);
    }
    groups.set(key, group);
  }
  return groups;
}

async function loadMarkPrices(entries: OrderHistory[]) {
  const prices = new Map<string, number>();
  await Promise.all(
    Array.from(new Set(entries.map(marketKey))).map(async (key) => {
      const [exchange, marketType, symbol] = key.split("|");
      if (!exchange || !marketType || !symbol) {
        return;
      }
      try {
        const result = await listResource(marketKlineResource, {
          page: 1,
          limit: 1,
          keyword: "",
          filters: { exchange, market_type: marketType, symbol },
          sortField: "open_time",
          sortDirection: "desc",
        });
        const kline = result.items[0] as MarketKline | undefined;
        const closePrice = toNumber(kline?.close_price);
        if (closePrice != null) {
          prices.set(key, closePrice);
        }
      } catch {
        // Running PnL can be blank when market data is unavailable or not permitted.
      }
    }),
  );
  return prices;
}

function groupKey(order: OrderHistory) {
  if (order.entry_order_id) {
    return order.entry_order_id;
  }
  if (isEntryOrder(order) && order.order_id) {
    return order.order_id;
  }
  return order.id ?? "";
}

function isEntryOrder(order: OrderHistory) {
  return (order.trade_condition ?? "").toUpperCase() === "ENTRY";
}

function isExitOrder(order: OrderHistory) {
  return new Set(["EXIT", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"]).has(
    (order.trade_condition ?? "").toUpperCase(),
  );
}

function inferEntryOrder(orders: OrderHistory[]) {
  const nonExitOrders = orders.filter((order) => !isExitOrder(order));
  if (nonExitOrders.length === 0) {
    return undefined;
  }
  return newest(nonExitOrders);
}

function newest(orders: OrderHistory[]) {
  return [...orders].sort((left, right) => dateValue(right.created_at) - dateValue(left.created_at))[0];
}

function marketKey(order: OrderHistory) {
  return `${order.exchange ?? ""}|${order.market_type ?? ""}|${order.symbol ?? ""}`;
}

function orderPrice(order: OrderHistory | undefined) {
  return toNumber(order?.avg_fill_price) ?? toNumber(order?.price);
}

function orderQuantity(order: OrderHistory) {
  return toNumber(order.filled_quantity) ?? toNumber(order.quantity) ?? 0;
}

function orderRealizedPnl(order: OrderHistory) {
  return toNumber(order.realized_pnl);
}

function calculatePnl(
  side: string | undefined,
  entryPrice: number | undefined,
  exitPrice: number | undefined,
  qty: number,
) {
  if (entryPrice == null || exitPrice == null || !qty) {
    return undefined;
  }
  const normalizedSide = (side ?? "").toUpperCase();
  if (normalizedSide === "SELL" || normalizedSide === "SHORT") {
    return (entryPrice - exitPrice) * qty;
  }
  return (exitPrice - entryPrice) * qty;
}

function dateValue(value: string | null | undefined) {
  if (!value) {
    return 0;
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? 0 : date.getTime();
}

function toNumber(value: string | number | null | undefined) {
  if (value == null || value === "") {
    return undefined;
  }
  const number = Number(value);
  return Number.isFinite(number) ? number : undefined;
}

function moneyCellClass(value: number | undefined) {
  return `px-3 py-3 tabular-nums ${pnlTone(value)}`;
}

function pnlTone(value: number | undefined) {
  if (value == null) {
    return "text-muted-foreground";
  }
  return value < 0 ? "text-destructive" : "text-primary";
}

function toneClass(tone: "gain" | "loss" | undefined) {
  if (tone === "gain") {
    return "text-primary";
  }
  if (tone === "loss") {
    return "text-destructive";
  }
  return "";
}

function formatInteger(value: number) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
}

function formatNumberValue(value: number | undefined) {
  if (value == null || !Number.isFinite(value)) {
    return "-";
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 }).format(value);
}

function formatMoney(value: number) {
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function formatPercent(value: number) {
  return new Intl.NumberFormat(undefined, {
    style: "percent",
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(value);
}

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value || "-";
  }
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
