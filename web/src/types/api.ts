import type { AuthTokens } from "@/types/auth";

export type Permission = string;

export type ApiEnvelope<T> = {
  message: string;
  code: string;
  data?: T;
  validation_errors?: Array<{
    field: string;
    tag: string;
    message: string;
  }>;
};

export type PaginationMeta = {
  page: number;
  limit: number;
  totalItems: number;
  totalPages: number;
};

export type PaginationResponse<T> = {
  meta: PaginationMeta;
  items: T[];
};

export type SortDirection = "asc" | "desc";

export type ResourceKey =
  | "overview"
  | "orders"
  | "orderPnL"
  | "dailyReports"
  | "strategyPerformance"
  | "marketKlines"
  | "priceReferences"
  | "marketBackfills"
  | "symbolMappings"
  | "klineSubscriptions"
  | "strategyConfigs"
  | "settings"
  | "users"
  | "roles"
  | "permissions"
  | "dashboardPages";

export type ResourceMode = "readonly" | "mutable";

export type ResourceConfig = {
  key: ResourceKey;
  label: string;
  description: string;
  shortDescription?: string;
  path: string;
  idField: string;
  mode: ResourceMode;
  readPermission: Permission;
  writePermission?: Permission;
  columns: string[];
  searchableFields: string[];
  filterFields?: string[];
  enumFields?: Record<string, string>;
  multiEnumFields?: string[];
  defaultSort: {
    field: string;
    direction: SortDirection;
  };
  sampleBody?: Record<string, unknown>;
  navParent?: string;
  navOrder?: number;
};

export type ListQuery = {
  page: number;
  limit: number;
  keyword: string;
  filters: Record<string, string | ListFilterValue>;
  sortField: string;
  sortDirection: SortDirection;
};

export type ListFilterValue = {
  op: "eq" | "neq" | "in" | "contain" | "between" | "gte" | "lte" | "gt" | "lt";
  value: string | string[];
};

export type DashboardPageConfig = {
  id: string;
  resource_key: ResourceKey;
  parent_key: string;
  label: string;
  description: string;
  short_description: string;
  icon: string;
  path: string;
  read_permission: Permission;
  write_permission: Permission | "";
  sort_order: number;
  visible: boolean;
};

export type SessionTokens = Pick<AuthTokens, "access_token" | "refresh_token">;
