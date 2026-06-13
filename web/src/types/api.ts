import type { AuthTokens } from "@/types/auth";

export type Permission =
  | "order:read"
  | "order:write"
  | "market:read"
  | "market:write"
  | "market_config:write"
  | "strategy_config:read"
  | "strategy_config:write"
  | "settings:read"
  | "settings:write"
  | "user:read"
  | "user:write";

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
  | "orders"
  | "marketKlines"
  | "symbolMappings"
  | "klineSubscriptions"
  | "strategyConfigs"
  | "settings"
  | "users"
  | "roles";

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
  enumFields?: Record<string, string>;
  multiEnumFields?: string[];
  defaultSort: {
    field: string;
    direction: SortDirection;
  };
  sampleBody?: Record<string, unknown>;
};

export type ListQuery = {
  page: number;
  limit: number;
  keyword: string;
  sortField: string;
  sortDirection: SortDirection;
};

export type SessionTokens = Pick<AuthTokens, "access_token" | "refresh_token">;
