"use client";

import {
  Activity,
  AlertTriangle,
  Bot,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Loader2,
  Radio,
  RefreshCw,
  Target,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { getDashboardOverview, type DashboardOrder, type DashboardOverview } from "@/lib/api-client";
import { cn } from "@/lib/utils";

export function DashboardOverviewPanel() {
  const [data, setData] = useState<DashboardOverview | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState("");
  const [recentPage, setRecentPage] = useState(1);

  const loadOverview = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      setData(await getDashboardOverview());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load dashboard overview.");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadOverview();
    const interval = globalThis.setInterval(() => {
      if (document.visibilityState === "visible") {
        void loadOverview();
      }
    }, 15_000);
    return () => globalThis.clearInterval(interval);
  }, [loadOverview]);

  const insight = useMemo(() => buildInsight(data), [data]);
  const recentPageCount = Math.max(1, Math.ceil((data?.recent_orders.length ?? 0) / 10));
  const currentRecentPage = Math.min(recentPage, recentPageCount);
  const recentOrders = data?.recent_orders.slice((currentRecentPage - 1) * 10, currentRecentPage * 10) ?? [];

  return (
    <section className="grid gap-5">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-sm font-medium text-muted-foreground">Last 24 hours</p>
          <h2 className="mt-1 text-2xl font-semibold">Trading bot overview</h2>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={loadOverview} disabled={isLoading}>
          <RefreshCw className={cn("h-4 w-4", isLoading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p> : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
        <MetricCard label="Realized PnL" value={money(data?.realized_pnl)} icon={CircleDollarSign} tone={number(data?.realized_pnl) < 0 ? "loss" : "gain"} />
        <MetricCard label="Running PnL" value={money(data?.running_pnl)} icon={Activity} tone={number(data?.running_pnl) < 0 ? "loss" : "gain"} />
        <MetricCard label="Win rate" value={percent(data ? data.winning_trades / Math.max(data.closed_trades, 1) : 0)} icon={Target} />
        <MetricCard label="Open positions" value={integer(data?.running_trades)} icon={Radio} />
        <MetricCard label="Strategies" value={`${data?.enabled_strategies ?? 0}/${data?.total_strategies ?? 0}`} icon={Bot} />
        <MetricCard label="Order activity" value={integer(data?.orders_24h)} icon={Activity} />
      </div>

      <div className="grid min-w-0 gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-w-0 overflow-hidden rounded-md">
          <CardHeader className="flex-row items-center justify-between border-b p-4">
            <CardTitle className="text-base">Recent positions</CardTitle>
            <span className="text-xs text-muted-foreground">Latest 20 entries</span>
          </CardHeader>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[820px] text-left text-sm">
                <thead className="bg-muted text-xs uppercase text-muted-foreground">
                  <tr>
                    <th className="px-3 py-3">Symbol</th><th className="px-3 py-3">Strategy</th>
                    <th className="px-3 py-3">Side</th><th className="px-3 py-3">State</th>
                    <th className="px-3 py-3">Qty</th><th className="px-3 py-3">Entry</th>
                    <th className="px-3 py-3">Exit</th><th className="px-3 py-3">PnL</th>
                    <th className="px-3 py-3">Time</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoading && !data ? (
                    <tr><td colSpan={9} className="px-3 py-10 text-center text-muted-foreground"><Loader2 className="mr-2 inline h-4 w-4 animate-spin" />Loading overview</td></tr>
                  ) : recentOrders.length ? recentOrders.map((order) => <OrderRow key={order.id} order={order} />) : (
                    <tr><td colSpan={9} className="px-3 py-10 text-center text-muted-foreground">No order history yet</td></tr>
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex items-center justify-between border-t px-3 py-3 text-sm text-muted-foreground">
              <span>Page {currentRecentPage} of {recentPageCount} · {data?.recent_orders.length ?? 0} positions</span>
              <div className="flex gap-2">
                <Button type="button" variant="outline" size="sm" disabled={currentRecentPage <= 1} onClick={() => setRecentPage((page) => page - 1)}>
                  <ChevronLeft className="h-4 w-4" /> Previous
                </Button>
                <Button type="button" variant="outline" size="sm" disabled={currentRecentPage >= recentPageCount} onClick={() => setRecentPage((page) => page + 1)}>
                  Next <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-md">
          <CardHeader className="border-b p-4"><CardTitle className="text-base">Bot status</CardTitle></CardHeader>
          <CardContent className="grid gap-3 p-4">
            <StatusRow label="Market feed" value={insight.marketLabel} healthy={insight.marketHealthy} detail={data?.last_price_at ? `Last price ${relativeTime(data.last_price_at)}` : "No price received"} />
            <StatusRow label="Strategies" value={insight.strategyLabel} healthy={insight.strategyHealthy} detail={`${data?.enabled_strategies ?? 0} enabled`} />
            <StatusRow label="Order errors" value={data?.problem_orders_24h ? "Needs attention" : "Clear"} healthy={!data?.problem_orders_24h} detail={`${data?.problem_orders_24h ?? 0} rejected or expired`} />
            <div className="border-t pt-3 text-xs text-muted-foreground">
              Updated {data?.generated_at ? relativeTime(data.generated_at) : "-"}
            </div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

function OrderRow({ order }: { order: DashboardOrder }) {
  return (
    <tr className="border-t">
      <td className="px-3 py-3 font-medium">{order.symbol}</td>
      <td className="max-w-48 truncate px-3 py-3 text-muted-foreground">{order.strategy_id || "-"}</td>
      <td className="px-3 py-3"><Badge variant={order.side.toLowerCase() === "buy" ? "success" : "warning"}>{order.side}</Badge></td>
      <td className="px-3 py-3"><Badge variant={order.state === "running" ? "success" : "outline"}>{order.state}</Badge></td>
      <td className="px-3 py-3 tabular-nums">{decimal(number(order.filled_quantity) || order.quantity)}</td>
      <td className="px-3 py-3 tabular-nums">{decimal(order.entry_price)}</td>
      <td className="px-3 py-3 tabular-nums">{decimal(order.exit_price)}</td>
      <td className={cn("px-3 py-3 font-medium tabular-nums", number(order.pnl) < 0 ? "text-destructive" : "text-primary")}>{order.pnl == null ? "-" : money(order.pnl)}</td>
      <td className="px-3 py-3 text-muted-foreground">{dateTime(order.created_at)}</td>
    </tr>
  );
}

function MetricCard({ label, value, icon: Icon, tone }: { label: string; value: string; icon: typeof Activity; tone?: "gain" | "loss" }) {
  return <Card className="overflow-hidden rounded-lg transition-transform hover:-translate-y-0.5 hover:shadow-md"><div className={cn("h-1 bg-gradient-to-r from-primary to-emerald-400", tone === "loss" && "from-destructive to-orange-400")} /><CardContent className="p-4"><div className="flex items-center justify-between text-xs font-medium uppercase text-muted-foreground"><span>{label}</span><span className="rounded-md bg-primary/10 p-1.5 text-primary"><Icon className="h-4 w-4" /></span></div><p className={cn("mt-3 text-xl font-semibold tabular-nums", tone === "loss" ? "text-destructive" : tone === "gain" ? "text-primary" : "")}>{value}</p></CardContent></Card>;
}

function StatusRow({ label, value, detail, healthy }: { label: string; value: string; detail: string; healthy: boolean }) {
  return <div className="rounded-md border bg-muted/30 p-3"><div className="flex items-center justify-between gap-2"><span className="text-sm font-medium">{label}</span><Badge variant={healthy ? "success" : "destructive"}>{healthy ? <Radio className="mr-1 h-3 w-3" /> : <AlertTriangle className="mr-1 h-3 w-3" />}{value}</Badge></div><p className="mt-1 text-xs text-muted-foreground">{detail}</p></div>;
}

function buildInsight(data: DashboardOverview | null) {
  const marketAge = data?.last_price_at ? Date.now() - new Date(data.last_price_at).getTime() : Infinity;
  return {
    marketHealthy: marketAge < 5 * 60 * 1000,
    marketLabel: marketAge < 5 * 60 * 1000 ? "Live" : "Stale",
    strategyHealthy: Boolean(data?.enabled_strategies),
    strategyLabel: data?.enabled_strategies ? "Running" : "Disabled",
  };
}

function number(value: string | number | undefined) { const parsed = Number(value ?? 0); return Number.isFinite(parsed) ? parsed : 0; }
function integer(value: number | undefined) { return new Intl.NumberFormat().format(value ?? 0); }
function money(value: string | number | undefined) { return new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(number(value)); }
function percent(value: number) { return new Intl.NumberFormat(undefined, { style: "percent", maximumFractionDigits: 1 }).format(value); }
function decimal(value: string | number | undefined) { return value == null || value === "" ? "-" : new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 }).format(number(value)); }
function dateTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? "-" : new Intl.DateTimeFormat(undefined, { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(date); }
function relativeTime(value: string) { const seconds = Math.round((new Date(value).getTime() - Date.now()) / 1000); const unit = Math.abs(seconds) < 60 ? "second" : Math.abs(seconds) < 3600 ? "minute" : "hour"; const divisor = unit === "second" ? 1 : unit === "minute" ? 60 : 3600; return new Intl.RelativeTimeFormat(undefined, { numeric: "auto" }).format(Math.round(seconds / divisor), unit); }
