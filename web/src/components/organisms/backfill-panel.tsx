"use client";

import { CheckCircle2, Loader2, Play, RefreshCw, XCircle } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import {
  getFormEnums,
  getMarketBackfill,
  listMarketBackfills,
  startMarketBackfill,
} from "@/lib/api-client";
import type { BackfillJob } from "@/lib/api-client";
import { canWriteResource } from "@/lib/rbac";
import type { ResourceConfig } from "@/types/api";
import type { AuthUser } from "@/types/auth";

type BackfillPanelProps = {
  resource: ResourceConfig;
  user: AuthUser;
};

type BackfillFormState = {
  exchange: string;
  market_type: string;
  symbol: string;
  interval: string;
  start_time: string;
  end_time: string;
};

export function BackfillPanel({ resource, user }: BackfillPanelProps) {
  const canWrite = useMemo(() => canWriteResource(user, resource), [resource, user]);
  const [form, setForm] = useState<BackfillFormState>(() => defaultBackfillForm());
  const [enums, setEnums] = useState<Record<string, string[]>>({});
  const [job, setJob] = useState<BackfillJob | null>(null);
  const [history, setHistory] = useState<BackfillJob[]>([]);
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const isActive = job?.status === "pending" || job?.status === "running";

  useEffect(() => {
    let mounted = true;
    async function loadEnums() {
      try {
        const result = await getFormEnums();
        if (mounted) {
          setEnums(result);
          setForm((current) => ({
            ...current,
            exchange: current.exchange || result.exchange?.[0] || "binance",
            market_type: current.market_type || result.market_type?.[0] || "futures",
            interval: current.interval || result.interval?.[0] || "1m",
          }));
        }
      } catch {
        if (mounted) {
          setEnums({});
        }
      }
    }
    void loadEnums();
    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    void loadHistory();
  }, []);

  useEffect(() => {
    if (!job || !isActive) {
      return;
    }
    const jobId = job.id;
    let cancelled = false;
    async function poll() {
      try {
        const result = await getMarketBackfill(jobId, 20);
        if (!cancelled) {
          setJob(result);
          void loadHistory();
          setError("");
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : "Unable to refresh backfill status.");
        }
      }
    }
    const timeout = globalThis.setTimeout(() => void poll(), 1000);
    return () => {
      cancelled = true;
      globalThis.clearTimeout(timeout);
    };
  }, [job, isActive]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canWrite || isSubmitting) {
      return;
    }
    setIsSubmitting(true);
    setError("");
    try {
      const nextJob = await startMarketBackfill({
        exchange: form.exchange,
        market_type: form.market_type,
        symbol: form.symbol,
        interval: form.interval,
        start_time: datetimeLocalToISO(form.start_time),
        end_time: datetimeLocalToISO(form.end_time),
      });
      setJob(nextJob);
      await loadHistory();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to start backfill.");
    } finally {
      setIsSubmitting(false);
    }
  }

  async function refreshJob() {
    if (!job) {
      return;
    }
    try {
      setJob(await getMarketBackfill(job.id));
      await loadHistory();
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to refresh backfill status.");
    }
  }

  async function loadHistory() {
    try {
      setHistory(await listMarketBackfills());
    } catch {
      setHistory([]);
    }
  }

  return (
    <section className="grid gap-5">
      <Card className="rounded-md">
        <CardHeader>
          <CardTitle className="text-base">Market kline backfill</CardTitle>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={handleSubmit}>
            <div className="grid gap-4 md:grid-cols-4">
              <SelectField
                label="Exchange"
                value={form.exchange}
                options={enums.exchange ?? ["binance", "tokocrypto"]}
                onChange={(exchange) => setForm((current) => ({ ...current, exchange }))}
              />
              <SelectField
                label="Market type"
                value={form.market_type}
                options={enums.market_type ?? ["futures", "spot"]}
                onChange={(market_type) => setForm((current) => ({ ...current, market_type }))}
              />
              <InputField
                label="Symbol"
                value={form.symbol}
                onChange={(symbol) => setForm((current) => ({ ...current, symbol }))}
              />
              <SelectField
                label="Interval"
                value={form.interval}
                options={enums.interval ?? ["1m", "5m", "15m", "1h", "4h", "1d"]}
                onChange={(interval) => setForm((current) => ({ ...current, interval }))}
              />
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <InputField
                label="Start time"
                type="datetime-local"
                value={form.start_time}
                onChange={(start_time) => setForm((current) => ({ ...current, start_time }))}
              />
              <InputField
                label="End time"
                type="datetime-local"
                value={form.end_time}
                onChange={(end_time) => setForm((current) => ({ ...current, end_time }))}
              />
            </div>

            {error ? (
              <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </p>
            ) : null}

            <div className="flex flex-wrap items-center gap-2">
              <Button type="submit" disabled={!canWrite || isSubmitting || isActive}>
                {isSubmitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                Start backfill
              </Button>
              <Button type="button" variant="outline" onClick={() => setForm(defaultBackfillForm())}>
                Reset window
              </Button>
              {job ? (
                <Button type="button" variant="outline" onClick={refreshJob}>
                  <RefreshCw className="h-4 w-4" />
                  Refresh status
                </Button>
              ) : null}
            </div>
          </form>
        </CardContent>
      </Card>

      {job ? (
        <Card className="rounded-md">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <StatusIcon status={job.status} />
              {statusLabel(job.status)}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3 text-sm md:grid-cols-3">
              <Readout label="Job ID" value={job.id} />
              <Readout label="Inserted" value={String(job.inserted_count ?? 0)} />
              <Readout label="Updated" value={formatDateTime(job.updated_at)} />
              <Readout label="Exchange" value={job.request.exchange} />
              <Readout label="Symbol" value={job.request.symbol} />
              <Readout label="Interval" value={job.request.interval} />
            </div>
            {job.error ? (
              <p className="mt-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {job.error}
              </p>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <Card className="rounded-md">
        <CardHeader>
          <CardTitle className="text-base">Backfill history</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] text-left text-sm">
              <thead className="text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-3 py-2">Status</th>
                  <th className="px-3 py-2">Market</th>
                  <th className="px-3 py-2">Window</th>
                  <th className="px-3 py-2">Inserted</th>
                  <th className="px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {history.map((item) => (
                  <tr key={item.id} className="border-t">
                    <td className="px-3 py-2">{statusLabel(item.status)}</td>
                    <td className="px-3 py-2">{item.request.exchange} {item.request.symbol} {item.request.interval}</td>
                    <td className="px-3 py-2">{formatDateTime(item.request.start_time)} - {formatDateTime(item.request.end_time)}</td>
                    <td className="px-3 py-2">{item.inserted_count}</td>
                    <td className="px-3 py-2">{formatDateTime(item.updated_at)}</td>
                  </tr>
                ))}
                {history.length === 0 ? (
                  <tr><td className="px-3 py-6 text-center text-muted-foreground" colSpan={5}>No backfill history</td></tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </section>
  );
}

function SelectField({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
}) {
  return (
    <div className="grid gap-2">
      <Label>{label}</Label>
      <Select
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </Select>
    </div>
  );
}

function InputField({
  label,
  value,
  onChange,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: string;
}) {
  return (
    <div className="grid gap-2">
      <Label>{label}</Label>
      <Input type={type} value={value} onChange={(event) => onChange(event.target.value)} />
    </div>
  );
}

function Readout({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border bg-background px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 break-all font-medium">{value || "-"}</p>
    </div>
  );
}

function StatusIcon({ status }: { status: BackfillJob["status"] }) {
  if (status === "succeeded") {
    return <CheckCircle2 className="h-5 w-5 text-emerald-600" />;
  }
  if (status === "failed") {
    return <XCircle className="h-5 w-5 text-destructive" />;
  }
  return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />;
}

function statusLabel(status: BackfillJob["status"]) {
  if (status === "succeeded") {
    return "Backfill completed";
  }
  if (status === "failed") {
    return "Backfill failed";
  }
  return "Backfill running";
}

function defaultBackfillForm(): BackfillFormState {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - 30);
  return {
    exchange: "binance",
    market_type: "futures",
    symbol: "BTC_USDT",
    interval: "1m",
    start_time: toDatetimeLocal(start),
    end_time: toDatetimeLocal(end),
  };
}

function toDatetimeLocal(date: Date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function datetimeLocalToISO(value: string) {
  return new Date(value).toISOString();
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString();
}
