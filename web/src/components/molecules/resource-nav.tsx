import { Database, Lock } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ResourceConfig, ResourceKey } from "@/types/api";

type ResourceNavProps = {
  resources: ResourceConfig[];
  activeKey: ResourceKey;
  readableKeys: Set<string>;
  onSelect: (key: ResourceKey) => void;
};

export function ResourceNav({
  resources,
  activeKey,
  readableKeys,
  onSelect,
}: ResourceNavProps) {
  return (
    <nav className="flex gap-2 overflow-x-auto border-b pb-3 lg:grid lg:gap-1 lg:overflow-visible lg:border-b-0 lg:pb-0">
      {resources.map((resource) => {
        const allowed = readableKeys.has(resource.key);
        return (
          <Button
            key={resource.key}
            type="button"
            variant={activeKey === resource.key ? "default" : "outline"}
            size="sm"
            disabled={!allowed}
            className={cn("h-9 shrink-0 justify-start lg:w-full", !allowed && "opacity-50")}
            onClick={() => onSelect(resource.key)}
            title={allowed ? resource.description : `Requires ${resource.readPermission}`}
          >
            {allowed ? <Database className="h-4 w-4" /> : <Lock className="h-4 w-4" />}
            {resource.label}
          </Button>
        );
      })}
    </nav>
  );
}
