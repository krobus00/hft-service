import {
  Activity,
  BarChart3,
  BellRing,
  CandlestickChart,
  Database,
  Lock,
  Settings,
  Shield,
  SlidersHorizontal,
  Users,
} from "lucide-react";

import { cn } from "@/lib/utils";
import type { ResourceConfig, ResourceKey } from "@/types/api";

type ResourceNavProps = {
  resources: ResourceConfig[];
  activeKey: ResourceKey;
  readableKeys: Set<string>;
  collapsed?: boolean;
  onSelect: (key: ResourceKey) => void;
};

export function ResourceNav({
  resources,
  activeKey,
  readableKeys,
  collapsed,
  onSelect,
}: ResourceNavProps) {
  return (
    <nav className="grid gap-1.5">
      {resources.map((resource) => {
        const allowed = readableKeys.has(resource.key);
        const active = activeKey === resource.key;
        const Icon = resourceIcon[resource.key] ?? Database;
        if (!allowed) {
            return <></>;
        }
        return (
          <button
            key={resource.key}
            type="button"
            disabled={!allowed}
            className={cn(
              "group relative flex h-11 w-full items-center gap-3 rounded-md px-3 text-left text-sm transition-colors",
              "border border-transparent text-muted-foreground hover:border-border hover:bg-background hover:text-foreground",
              active && "border-primary/25 bg-primary/10 text-foreground shadow-sm",
              !allowed && "cursor-not-allowed opacity-45 hover:border-transparent hover:bg-transparent hover:text-muted-foreground",
              collapsed && "justify-center px-0",
            )}
            onClick={() => onSelect(resource.key)}
            title={allowed ? resource.description : `Requires ${resource.readPermission}`}
          >
            {active ? (
              <span className="absolute left-0 top-2 h-7 w-1 rounded-r-full bg-primary" />
            ) : null}
            <span
              className={cn(
                "flex h-8 w-8 shrink-0 items-center justify-center rounded-md border bg-card transition-colors",
                active && "border-primary/30 bg-primary text-primary-foreground",
                !allowed && "bg-muted",
              )}
            >
              {allowed ? <Icon className="h-4 w-4" /> : <Lock className="h-4 w-4" />}
            </span>
            <span className={cn("min-w-0 flex-1", collapsed && "sr-only")}>
              <span className="block truncate font-medium">{resource.label}</span>
              <span className="block truncate text-xs text-muted-foreground">
                {allowed ? resource.shortDescription : "No access"}
              </span>
            </span>
          </button>
        );
      })}
    </nav>
  );
}

const resourceIcon: Partial<Record<ResourceKey, typeof Database>> = {
  orders: Activity,
  marketKlines: CandlestickChart,
  symbolMappings: SlidersHorizontal,
  klineSubscriptions: BellRing,
  strategyConfigs: BarChart3,
  settings: Settings,
  users: Users,
  roles: Shield,
};
