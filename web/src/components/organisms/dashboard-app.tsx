"use client";

import { useRouter } from "next/navigation";
import { Loader2, ShieldAlert } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { ResourceNav } from "@/components/molecules/resource-nav";
import { BackfillPanel } from "@/components/organisms/backfill-panel";
import { DailyReportPanel } from "@/components/organisms/daily-report-panel";
import { DashboardOverviewPanel } from "@/components/organisms/dashboard-overview-panel";
import { OrderPnLPanel } from "@/components/organisms/order-pnl-panel";
import { ResourcePanel } from "@/components/organisms/resource-panel";
import { StrategyMetricDetailPanel } from "@/components/organisms/strategy-metric-detail-panel";
import { StrategyPerformancePanel } from "@/components/organisms/strategy-performance-panel";
import { DashboardTemplate } from "@/components/templates/dashboard-template";
import { Button } from "@/components/ui/button";
import { getDashboardPagesMenu } from "@/lib/api-client";
import { getMe, logoutCurrentSession } from "@/lib/auth-api";
import { canReadResource } from "@/lib/rbac";
import { resources } from "@/lib/resources";
import { readSession } from "@/lib/auth-store";
import type { DashboardPageConfig, ResourceConfig, ResourceKey } from "@/types/api";
import type { AuthUser, Session } from "@/types/auth";

type DashboardAppProps = {
  initialResourceKey?: ResourceKey;
};

export function DashboardApp({ initialResourceKey = "orders" }: DashboardAppProps) {
  const router = useRouter();
  const [session, setSession] = useState<Session | null>(null);
  const [activeKey, setActiveKey] = useState<ResourceKey>(initialResourceKey);
  const [managedPages, setManagedPages] = useState<DashboardPageConfig[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    async function loadSession() {
      const storedSession = readSession();
      if (!storedSession) {
        router.replace("/login");
        return;
      }

      try {
        const nextSession = await getMe(storedSession);
        if (isMounted) {
          setSession(nextSession);
          try {
            const pages = await getDashboardPagesMenu();
            if (isMounted) {
              setManagedPages(pages);
            }
          } catch {
            if (isMounted) {
              setManagedPages([]);
            }
          }
        }
      } catch {
        await logoutCurrentSession();
        router.replace("/login");
      } finally {
        if (isMounted) {
          setIsLoading(false);
        }
      }
    }

    void loadSession();

    return () => {
      isMounted = false;
    };
  }, [router]);

  useEffect(() => {
    setActiveKey(initialResourceKey);
  }, [initialResourceKey]);

  useEffect(() => {
    function handlePopState() {
      const key = resourceKeyFromPath(window.location.pathname);
      if (key) {
        setActiveKey(key);
      }
    }

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const dashboardResources = useMemo(
    () => mergeManagedPages(resources, managedPages),
    [managedPages],
  );

  const readableKeys = useMemo(() => {
    const keys = new Set<string>();
    if (!session) {
      return keys;
    }
    for (const resource of dashboardResources) {
      if (canReadResource(session.user, resource)) {
        keys.add(resource.key);
      }
    }
    return keys;
  }, [dashboardResources, session]);

  useEffect(() => {
    const first = firstReadableResource(dashboardResources, readableKeys);
    if (first && !readableKeys.has(activeKey)) {
      setActiveKey(first.key);
      replaceDashboardPath(first.key);
    }
  }, [activeKey, dashboardResources, readableKeys]);

  async function handleLogout() {
    await logoutCurrentSession();
    router.replace("/login");
  }

  if (isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background text-muted-foreground">
        <Loader2 className="mr-2 h-5 w-5 animate-spin" />
        Loading Krobot
      </main>
    );
  }

  if (!session) {
    return null;
  }

  const activeResource =
    dashboardResources.find((resource) => resource.key === activeKey) ??
    firstReadableResource(dashboardResources, readableKeys);

  return (
    <DashboardTemplate
      user={session.user}
      pageTitle={activeResource?.label ?? "Dashboard"}
      pageDescription={activeResource?.description ?? "Authenticated API workspace"}
      onLogout={handleLogout}
      onUserChange={(user) => setSession((current) => updateSessionUser(current, user))}
      sidebar={({ collapsed, onNavigate }) => (
        <ResourceNav
          resources={dashboardResources}
          activeKey={activeResource?.key ?? activeKey}
          readableKeys={readableKeys}
          collapsed={collapsed}
          onSelect={(key) => {
            setActiveKey(key);
            pushDashboardPath(key);
            onNavigate?.();
          }}
        />
      )}
    >
      <div className="grid gap-6">
        {activeResource ? (
          activeResource.key === "overview" ? (
            <DashboardOverviewPanel />
          ) : activeResource.key === "marketBackfills" ? (
            <BackfillPanel key={activeResource.key} resource={activeResource} user={session.user} />
          ) : activeResource.key === "orderPnL" ? (
            <OrderPnLPanel key={activeResource.key} resource={activeResource} />
          ) : activeResource.key === "dailyReports" ? (
            <DailyReportPanel key={activeResource.key} resource={activeResource} />
          ) : activeResource.key === "strategyPerformance" ? (
            <StrategyPerformancePanel key={activeResource.key} resource={activeResource} />
          ) : activeResource.key === "strategyMetrics" ? (
            <StrategyMetricDetailPanel key={activeResource.key} />
          ) : (
            <ResourcePanel key={activeResource.key} resource={activeResource} user={session.user} />
          )
        ) : (
          <section className="flex min-h-80 flex-col items-center justify-center gap-3 rounded-md border bg-card p-8 text-center">
            <ShieldAlert className="h-8 w-8 text-muted-foreground" />
            <h2 className="text-lg font-semibold">No dashboard permissions</h2>
            <p className="max-w-md text-sm text-muted-foreground">
              This account can authenticate but does not have read access to any dashboard
              endpoint.
            </p>
            <Button type="button" variant="outline" onClick={handleLogout}>
              Logout
            </Button>
          </section>
        )}
      </div>
    </DashboardTemplate>
  );
}

function mergeManagedPages(
  baseResources: ResourceConfig[],
  pages: DashboardPageConfig[],
): ResourceConfig[] {
  if (pages.length === 0) {
    return baseResources;
  }
  const byKey = new Map(baseResources.map((resource) => [resource.key, resource]));
  const result: ResourceConfig[] = baseResources
    .filter((resource) => resource.key === "overview")
    .map((resource) => ({ ...resource }));
  for (const page of pages) {
    const base = byKey.get(page.resource_key);
    if (!base || !page.visible) {
      continue;
    }
    result.push({
      ...base,
      label: page.label || base.label,
      description: page.description || base.description,
      shortDescription: page.short_description || base.shortDescription,
      readPermission: page.read_permission || base.readPermission,
      writePermission: page.write_permission || base.writePermission,
      navParent: page.parent_key,
      navOrder: page.sort_order,
    });
  }
  return result.sort((left, right) => (left.navOrder ?? 0) - (right.navOrder ?? 0));
}

function updateSessionUser(session: Session | null, user: AuthUser) {
  if (!session) {
    return session;
  }
  return { ...session, user };
}

function firstReadableResource(items: ResourceConfig[], readableKeys: Set<string>) {
  return items.find((resource) => readableKeys.has(resource.key));
}

function pushDashboardPath(key: ResourceKey) {
  const path = `/dashboard/${key}`;
  if (window.location.pathname !== path) {
    window.history.pushState(null, "", path);
  }
}

function replaceDashboardPath(key: ResourceKey) {
  const path = `/dashboard/${key}`;
  if (window.location.pathname !== path) {
    window.history.replaceState(null, "", path);
  }
}

function resourceKeyFromPath(pathname: string): ResourceKey | null {
  const value = pathname.split("/").filter(Boolean).at(-1);
  return resources.find((resource) => resource.key === value)?.key ?? null;
}
