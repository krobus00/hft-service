import { Plus, RefreshCw, Search } from "lucide-react";
import type React from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";

type ResourceToolbarProps = {
  keyword: string;
  filters: Record<string, string | { op: string; value: string | string[] }>;
  filterFields: string[];
  enumFields?: Record<string, string>;
  enums: Record<string, string[]>;
  canWrite: boolean;
  isLoading: boolean;
  onKeywordChange: (value: string) => void;
  onFilterChange: (field: string, value: string) => void;
  onRefresh: () => void;
  onCreate: () => void;
  extraAction?: React.ReactNode;
};

export function ResourceToolbar({
  keyword,
  filters,
  filterFields,
  enumFields,
  enums,
  canWrite,
  isLoading,
  onKeywordChange,
  onFilterChange,
  onRefresh,
  onCreate,
  extraAction,
}: ResourceToolbarProps) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div className="grid w-full gap-2 sm:grid-cols-2 xl:max-w-5xl xl:grid-cols-6">
          <label className="relative sm:col-span-2 xl:col-span-2">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={keyword}
              onChange={(event) => onKeywordChange(event.target.value)}
              placeholder="Search"
              className="pl-9"
            />
          </label>
          {filterFields.map((field) => {
            const enumKey = enumFields?.[field];
            const options = enumKey ? enums[enumKey] ?? [] : [];
            if (options.length > 0) {
              return (
                <Select
                  key={field}
                  value={filterValue(filters[field])}
                  onChange={(event) => onFilterChange(field, event.target.value)}
                  aria-label={humanize(field)}
                >
                  <option value="">{humanize(field)}</option>
                  {options.map((option) => (
                    <option key={`${field}-${option}`} value={option}>
                      {option}
                    </option>
                  ))}
                </Select>
              );
            }
            return (
              <Input
                key={field}
                value={filterValue(filters[field])}
                onChange={(event) => onFilterChange(field, event.target.value)}
                placeholder={humanize(field)}
              />
            );
          })}
        </div>
      <div className="flex gap-2">
        {extraAction}
        <Button type="button" variant="outline" size="sm" onClick={onRefresh} disabled={isLoading}>
          <RefreshCw className={isLoading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          Refresh
        </Button>
        {canWrite ? (
          <Button type="button" size="sm" onClick={onCreate}>
            <Plus className="h-4 w-4" />
            Create
          </Button>
        ) : null}
      </div>
      </div>
    </div>
  );
}

function filterValue(value: string | { op: string; value: string | string[] } | undefined) {
  if (typeof value === "string") {
    return value;
  }
  return "";
}

function humanize(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
