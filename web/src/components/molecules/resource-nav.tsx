import {
  Activity,
  BarChart3,
  BellRing,
  CandlestickChart,
  Database,
  History,
  KeyRound,
  LayoutDashboard,
  LayoutList,
  Radio,
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
  const groups = groupResources(resources);
  return (
    <nav className="grid gap-3">
      {groups.map((group) => (
        <div key={group.name} className="grid gap-1.5">
          {group.name && !collapsed ? (
            <p className="px-3 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
              {group.name}
            </p>
          ) : null}
          {group.items.map((resource) => {
            const allowed = readableKeys.has(resource.key);
            const active = activeKey === resource.key;
            const Icon = resourceIcon[resource.key] ?? Database;
            if (!allowed) {
              return null;
            }
            return (
              <button
                key={resource.key}
                type="button"
                className={cn(
                  "group relative flex h-11 w-full items-center gap-3 rounded-md px-3 text-left text-sm transition-colors",
                  "border border-transparent text-muted-foreground hover:border-border hover:bg-background hover:text-foreground",
                  active && "border-primary/25 bg-primary/10 text-foreground shadow-sm",
                  collapsed && "justify-center px-0",
                )}
                onClick={() => onSelect(resource.key)}
                title={resource.description}
              >
                {active ? (
                  <span className="absolute left-0 top-2 h-7 w-1 rounded-r-full bg-primary" />
                ) : null}
                <span
                  className={cn(
                    "flex h-8 w-8 shrink-0 items-center justify-center rounded-md border bg-card transition-colors",
                    active && "border-primary/30 bg-primary text-primary-foreground",
                  )}
                >
                  <Icon className="h-4 w-4" />
                </span>
                <span className={cn("min-w-0 flex-1", collapsed && "sr-only")}>
                  <span className="block truncate font-medium">{resource.label}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {resource.shortDescription}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      ))}
    </nav>
  );
}

const resourceIcon: Partial<Record<ResourceKey, typeof Database>> = {
  overview: LayoutDashboard,
  orders: Activity,
  orderPnL: BarChart3,
  dailyReports: BarChart3,
  strategyPerformance: BarChart3,
  strategyMonitors: Activity,
  marketKlines: CandlestickChart,
  priceReferences: Radio,
  marketBackfills: History,
  symbolMappings: SlidersHorizontal,
  klineSubscriptions: BellRing,
  strategyConfigs: BarChart3,
  settings: Settings,
  users: Users,
  roles: Shield,
  permissions: KeyRound,
  dashboardPages: LayoutList,
};

function groupResources(resources: ResourceConfig[]) {
  const groups = new Map<string, ResourceConfig[]>();
  for (const resource of resources) {
    const name = resource.navParent ?? "";
    groups.set(name, [...(groups.get(name) ?? []), resource]);
  }
  const preferred = ["", "Trading", "Automation", "Market Data", "Administration"];
  const names = [
    ...preferred.filter((name) => groups.has(name)),
    ...Array.from(groups.keys()).filter((name) => !preferred.includes(name)),
  ];
  return names
    .map((name) => ({
      name,
      items: (groups.get(name) ?? []).sort((left, right) => (left.navOrder ?? 0) - (right.navOrder ?? 0)),
    }));
}
