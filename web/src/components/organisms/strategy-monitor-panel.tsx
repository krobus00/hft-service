"use client";

import { Activity, Power, RefreshCw, RotateCcw } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  listStrategyMonitors,
  runStrategyMonitorAction,
  type StrategyMonitor,
} from "@/lib/api-client";
import type { ResourceConfig } from "@/types/api";

type StrategyMonitorPanelProps = {
  resource: ResourceConfig;
};

export function StrategyMonitorPanel({ resource }: StrategyMonitorPanelProps) {
  const [items, setItems] = useState<StrategyMonitor[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [busyKey, setBusyKey] = useState("");
  const [error, setError] = useState("");

  const loadItems = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      setItems(await listStrategyMonitors());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load strategy monitors.");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadItems();
  }, [loadItems]);

  const summary = useMemo(() => summarize(items), [items]);

  async function runAction(name: string, action: "reset" | "restart") {
    setBusyKey(`${name}:${action}`);
    setError("");
    try {
      await runStrategyMonitorAction(name, action);
      await loadItems();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : `Unable to ${action} strategy.`);
    } finally {
      setBusyKey("");
    }
  }

  return (
    <section className="grid gap-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="grid gap-3 sm:grid-cols-3">
          <Metric label="Configured" value={String(items.length)} />
          <Metric label="Online" value={String(summary.online)} />
          <Metric label="Active pairs" value={String(summary.activePairs)} />
        </div>
        <Button type="button" variant="outline" size="sm" onClick={loadItems} disabled={isLoading}>
          <RefreshCw className={isLoading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          Refresh
        </Button>
      </div>

      {error ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}

      <div className="overflow-hidden rounded-md border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1040px] text-left text-sm">
            <thead className="bg-muted text-xs uppercase text-muted-foreground">
              <tr>
                {resource.columns.map((column) => (
                  <th key={column} className="px-3 py-3 font-medium">
                    {humanize(column)}
                  </th>
                ))}
                <th className="px-3 py-3 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {items.length > 0 ? (
                items.map((item) => {
                  const view = monitorView(item);
                  return (
                    <tr key={item.name} className="border-t">
                      <td className="px-3 py-3 font-medium">{item.name}</td>
                      <td className="px-3 py-3">
                        <Badge variant={item.online ? "success" : "destructive"}>
                          <Activity className="mr-1 h-3 w-3" />
                          {item.online ? "Online" : "Offline"}
                        </Badge>
                      </td>
                      <td className="max-w-72 truncate px-3 py-3" title={view.strategyID}>
                        {view.strategyID || "-"}
                      </td>
                      <td className="px-3 py-3 tabular-nums">{view.activePairs}</td>
                      <td className="max-w-96 truncate px-3 py-3 text-muted-foreground" title={view.stateSummary}>
                        {view.stateSummary || "-"}
                      </td>
                      <td className="px-3 py-3 tabular-nums">{view.ordersPublished}</td>
                      <td className={view.messagesFailed > 0 ? "px-3 py-3 tabular-nums text-destructive" : "px-3 py-3 tabular-nums"}>
                        {view.messagesFailed}
                      </td>
                      <td className="px-3 py-3 tabular-nums">{formatDuration(view.uptimeSec)}</td>
                      <td className="max-w-80 truncate px-3 py-3 text-muted-foreground" title={view.lastError || item.error || ""}>
                        {view.lastError || item.error || "-"}
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex gap-2">
                          <Button type="button" variant="outline" size="sm" onClick={() => runAction(item.name, "reset")} disabled={Boolean(busyKey)}>
                            <RotateCcw className={busyKey === `${item.name}:reset` ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
                            Reset
                          </Button>
                          <Button type="button" variant="destructive" size="sm" onClick={() => runAction(item.name, "restart")} disabled={Boolean(busyKey)}>
                            <Power className="h-4 w-4" />
                            Restart
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })
              ) : (
                <tr>
                  <td className="px-3 py-8 text-center text-muted-foreground" colSpan={resource.columns.length + 1}>
                    No strategy monitors configured
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function monitorView(item: StrategyMonitor) {
  const payload = item.payload ?? {};
  const strategy = asRecord(payload.strategy);
  const health = asRecord(payload.health);
  const metrics = asRecord(payload.metrics);
  return {
    strategyID: stringValue(strategy.strategy_id),
    activePairs: numberValue(payload.active_pairs_count),
    stateSummary: stateSummary(asRecord(payload.strategy_state_by_pair)),
    ordersPublished: numberValue(metrics.orders_published),
    messagesFailed: numberValue(metrics.messages_failed),
    uptimeSec: numberValue(health.uptime_sec),
    lastError: stringValue(metrics.last_error),
  };
}

function stateSummary(states: Record<string, unknown>) {
  return Object.entries(states).slice(0, 3).map(([pair, value]) => {
    const values = asRecord(asRecord(value).all_variables);
    const risk = asRecord(asRecord(value).risk_config);
    const side = stringValue(values.position_side ?? risk.position_side) || "FLAT";
    const entry = values.entry_price ?? risk.entry_price;
    const cooldown = values.cooldown ?? risk.cooldown;
    const parts = [pair, side];
    if (entry != null && entry !== "") {
      parts.push(`entry=${String(entry)}`);
    }
    if (cooldown != null && cooldown !== "") {
      parts.push(`cooldown=${String(cooldown)}`);
    }
    return parts.join(" ");
  }).join(" | ");
}

function summarize(items: StrategyMonitor[]) {
  return items.reduce(
    (acc, item) => {
      acc.online += item.online ? 1 : 0;
      acc.activePairs += monitorView(item).activePairs;
      return acc;
    },
    { online: 0, activePairs: 0 },
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border bg-card px-4 py-3">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-xl font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
}

function numberValue(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function formatDuration(seconds: number) {
  if (seconds < 60) {
    return `${seconds}s`;
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}m`;
  }
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}

function humanize(value: string) {
  return value.replaceAll("_", " ");
}
