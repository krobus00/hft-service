import { ArrowDown, ArrowUp, ChevronsUpDown, ChevronLeft, ChevronRight, Eye, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { PaginationMeta, ResourceConfig, SortDirection } from "@/types/api";

type ResourceTableProps = {
  resource: ResourceConfig;
  items: Array<Record<string, unknown>>;
  meta: PaginationMeta | null;
  canWrite: boolean;
  sortField: string;
  sortDirection: SortDirection;
  onDetail: (id: string) => void;
  onDelete: (id: string) => void;
  onPageChange: (page: number) => void;
  onSortChange: (field: string) => void;
};

export function ResourceTable({
  resource,
  items,
  meta,
  canWrite,
  sortField,
  sortDirection,
  onDetail,
  onDelete,
  onPageChange,
  onSortChange,
}: ResourceTableProps) {
  return (
    <div className="overflow-hidden rounded-md border bg-card">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] text-left text-sm">
          <thead className="bg-muted text-xs uppercase text-muted-foreground">
            <tr>
              {resource.columns.map((column) => (
                <th key={column} className="px-3 py-3 font-medium">
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 rounded-sm text-left hover:text-foreground"
                    onClick={() => onSortChange(column)}
                  >
                    {humanize(column)}
                    {sortField === column ? (
                      sortDirection === "asc" ? (
                        <ArrowUp className="h-3.5 w-3.5" />
                      ) : (
                        <ArrowDown className="h-3.5 w-3.5" />
                      )
                    ) : (
                      <ChevronsUpDown className="h-3.5 w-3.5 opacity-50" />
                    )}
                  </button>
                </th>
              ))}
              <th className="w-28 px-3 py-3 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.length > 0 ? (
              items.map((item, index) => {
                const rawID = item[resource.idField];
                const id = rawID == null ? "" : String(rawID);
                return (
                  <tr key={`${resource.key}-${id}-${index}`} className="border-t">
                    {resource.columns.map((column) => (
                      <td key={column} className="max-w-64 truncate px-3 py-3">
                        {formatCell(item[column], column)}
                      </td>
                    ))}
                    <td className="px-3 py-3">
                      <div className="flex justify-end gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          onClick={() => onDetail(id)}
                          disabled={!id}
                          title="Open detail"
                        >
                          <Eye className="h-4 w-4" />
                        </Button>
                        {canWrite ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            onClick={() => onDelete(id)}
                            disabled={!id}
                            title="Delete"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                );
              })
            ) : (
              <tr>
                <td className="px-3 py-8 text-center text-muted-foreground" colSpan={resource.columns.length + 1}>
                  No data
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between border-t px-3 py-3 text-sm text-muted-foreground">
        <span>
          Page {meta?.page ?? 1} of {meta?.totalPages ?? 0} - {meta?.totalItems ?? 0} rows
        </span>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!meta || meta.page <= 1}
            onClick={() => onPageChange((meta?.page ?? 1) - 1)}
          >
            <ChevronLeft className="h-4 w-4" />
            Previous
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!meta || meta.page >= meta.totalPages}
            onClick={() => onPageChange((meta?.page ?? 1) + 1)}
          >
            Next
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

function humanize(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatCell(value: unknown, column: string) {
  if (value == null) {
    return "";
  }
  if (column === "id" && typeof value === "string") {
    return (
      <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs" title={value}>
        {compactID(value)}
      </span>
    );
  }
  if (isTimestampColumn(column) && (typeof value === "string" || typeof value === "number")) {
    return formatLocalTime(value);
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

function compactID(value: string) {
  if (value.length <= 14) {
    return value;
  }
  return `${value.slice(0, 8)}...${value.slice(-4)}`;
}

function isTimestampColumn(column: string) {
  return column.endsWith("_at") || column.endsWith("_time");
}

function formatLocalTime(value: string | number) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}
