import { Plus, RefreshCw, Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type ResourceToolbarProps = {
  keyword: string;
  canWrite: boolean;
  isLoading: boolean;
  onKeywordChange: (value: string) => void;
  onRefresh: () => void;
  onCreate: () => void;
};

export function ResourceToolbar({
  keyword,
  canWrite,
  isLoading,
  onKeywordChange,
  onRefresh,
  onCreate,
}: ResourceToolbarProps) {
  return (
    <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <label className="relative w-full md:max-w-sm">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={keyword}
          onChange={(event) => onKeywordChange(event.target.value)}
          placeholder="Search"
          className="pl-9"
        />
      </label>
      <div className="flex gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onRefresh} disabled={isLoading}>
          <RefreshCw className={isLoading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          Refresh
        </Button>
        <Button type="button" size="sm" onClick={onCreate} disabled={!canWrite}>
          <Plus className="h-4 w-4" />
          Create
        </Button>
      </div>
    </div>
  );
}
