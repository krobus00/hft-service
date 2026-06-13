"use client";

import { useRouter } from "next/navigation";
import { Loader2, ShieldAlert } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { ResourceNav } from "@/components/molecules/resource-nav";
import { ResourcePanel } from "@/components/organisms/resource-panel";
import { DashboardTemplate } from "@/components/templates/dashboard-template";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { getMe, logoutCurrentSession } from "@/lib/auth-api";
import { canReadResource } from "@/lib/rbac";
import { firstReadableResource, resources } from "@/lib/resources";
import { readSession } from "@/lib/auth-store";
import type { ResourceKey } from "@/types/api";
import type { Session } from "@/types/auth";

export function DashboardApp() {
  const router = useRouter();
  const [session, setSession] = useState<Session | null>(null);
  const [activeKey, setActiveKey] = useState<ResourceKey>("orders");
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

  const readableKeys = useMemo(() => {
    const keys = new Set<string>();
    if (!session) {
      return keys;
    }
    for (const resource of resources) {
      if (canReadResource(session.user, resource)) {
        keys.add(resource.key);
      }
    }
    return keys;
  }, [session]);

  useEffect(() => {
    const first = firstReadableResource(readableKeys);
    if (first && !readableKeys.has(activeKey)) {
      setActiveKey(first.key);
    }
  }, [activeKey, readableKeys]);

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
    resources.find((resource) => resource.key === activeKey) ??
    firstReadableResource(readableKeys);

  return (
    <DashboardTemplate user={session.user} onLogout={handleLogout}>
      <div className="grid gap-6">
        <section className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-normal">Dashboard API</h1>
            <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
              Standardized workspace for auth-protected API endpoints.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {session.user.roles.map((role) => (
              <Badge key={role} variant="secondary">
                {role}
              </Badge>
            ))}
          </div>
        </section>

        <div className="grid gap-6 lg:grid-cols-[240px_minmax(0,1fr)]">
          <aside className="lg:sticky lg:top-20 lg:self-start">
            <ResourceNav
              resources={resources}
              activeKey={activeResource?.key ?? activeKey}
              readableKeys={readableKeys}
              onSelect={setActiveKey}
            />
          </aside>

          {activeResource ? (
            <ResourcePanel key={activeResource.key} resource={activeResource} user={session.user} />
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
      </div>
    </DashboardTemplate>
  );
}
