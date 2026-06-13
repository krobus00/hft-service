import { clearSession, readSession, writeSession } from "@/lib/auth-store";
import type {
  ApiEnvelope,
  DashboardPageConfig,
  ListQuery,
  PaginationResponse,
  ResourceConfig,
} from "@/types/api";
import type { AuthResult, AuthUser, Session } from "@/types/auth";

export type BackfillRequest = {
  exchange: string;
  market_type: string;
  symbol: string;
  interval: string;
  start_time: string;
  end_time: string;
};

export type BackfillJob = {
  id: string;
  status: "pending" | "running" | "succeeded" | "failed";
  request: BackfillRequest;
  inserted_count: number;
  error?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  updated_at: string;
};

export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:9804/api/v1";

export class ApiError extends Error {
  code: string;
  status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

type RequestOptions = RequestInit & {
  accessToken?: string;
};

let refreshPromise: Promise<Session> | null = null;
const inFlightRequests = new Map<string, Promise<unknown>>();

export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { accessToken, headers, ...requestOptions } = options;
  const method = (requestOptions.method ?? "GET").toUpperCase();
  const requestKey = buildRequestKey(method, path, accessToken, requestOptions.body);
  const existing = inFlightRequests.get(requestKey);
  if (existing) {
    return existing as Promise<T>;
  }

  const requestPromise = fetch(`${API_BASE_URL}${path}`, {
    ...requestOptions,
    method,
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...headers,
    },
  }).then(async (response) => {
    const envelope = (await response.json()) as ApiEnvelope<T>;

    if (!response.ok || envelope.code !== "SUCCESS") {
      throw new ApiError(
        envelope.message || "Request failed",
        envelope.code || "REQUEST_FAILED",
        response.status,
      );
    }

    return envelope.data as T;
  }).finally(() => {
    globalThis.setTimeout(() => inFlightRequests.delete(requestKey), method === "GET" ? 150 : 600);
  });

  inFlightRequests.set(requestKey, requestPromise);
  return requestPromise;
}

function buildRequestKey(
  method: string,
  path: string,
  accessToken: string | undefined,
  body: BodyInit | null | undefined,
) {
  const tokenKey = accessToken ? accessToken.slice(-16) : "public";
  const bodyKey = typeof body === "string" ? body : "";
  return `${method}:${path}:${tokenKey}:${bodyKey}`;
}

export async function rawApiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { accessToken, headers, ...requestOptions } = options;
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...requestOptions,
    headers: {
      "Content-Type": "application/json",
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...headers,
    },
  });

  const envelope = (await response.json()) as ApiEnvelope<T>;

  if (!response.ok || envelope.code !== "SUCCESS") {
    throw new ApiError(
      envelope.message || "Request failed",
      envelope.code || "REQUEST_FAILED",
      response.status,
    );
  }

  return envelope.data as T;
}

export function persistAuthResult(result: AuthResult): Session {
  const session = {
    user: result.user,
    tokens: result.tokens,
  };
  writeSession(session);
  return session;
}

export async function refreshSession(session: Session) {
  const currentSession = readSession();
  if (
    currentSession &&
    currentSession.tokens.access_token !== session.tokens.access_token
  ) {
    return currentSession;
  }

  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = rotateRefreshToken(session).finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}

async function rotateRefreshToken(session: Session) {
  const result = await apiRequest<AuthResult>("/auth/refresh", {
    method: "POST",
    body: JSON.stringify({ refresh_token: session.tokens.refresh_token }),
  });
  return persistAuthResult(result);
}

export async function withSession<T>(
  callback: (session: Session) => Promise<T>,
): Promise<T> {
  const session = readSession();
  if (!session) {
    throw new ApiError("Session is required", "UNAUTHORIZED", 401);
  }

  try {
    return await callback(session);
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401) {
      throw error;
    }

    const refreshed = await refreshSession(session);
    return callback(refreshed);
  }
}

export async function getCurrentUser(session: Session) {
  const user = await apiRequest<AuthUser>("/auth/me", {
    method: "GET",
    accessToken: session.tokens.access_token,
  });
  const nextSession = { ...session, user };
  writeSession(nextSession);
  return nextSession;
}

export async function logoutSession() {
  const session = readSession();
  if (!session) {
    return;
  }

  try {
    await apiRequest<{ status: string }>("/auth/logout", {
      method: "POST",
      body: JSON.stringify({ refresh_token: session.tokens.refresh_token }),
      accessToken: session.tokens.access_token,
    });
  } catch {
    // Local cleanup still needs to happen if the server token is already invalid.
  } finally {
    clearSession();
  }
}

export async function getAuthConfig() {
  return apiRequest<{ show_setup_link: boolean }>("/auth/config", {
    method: "GET",
  });
}

export async function getFormEnums() {
  return withSession((session) =>
    apiRequest<Record<string, string[]>>("/form/enums", {
      method: "GET",
      accessToken: session.tokens.access_token,
    }),
  );
}

export function buildListPath(resource: ResourceConfig, query: ListQuery) {
  const params = new URLSearchParams();
  params.set("pagination", JSON.stringify({ page: query.page, limit: query.limit }));
  params.set(
    "sort",
    JSON.stringify({ field: query.sortField, direction: query.sortDirection }),
  );
  if (query.keyword.trim()) {
    params.set("keyword", query.keyword.trim());
  }
  const filters = Object.entries(query.filters)
    .filter(([, value]) => value.trim() !== "")
    .map(([field, value]) => ({ field, op: "eq", value: [value.trim()] }));
  if (filters.length > 0) {
    params.set("filter", JSON.stringify(filters));
  }
  return `${resource.path}?${params.toString()}`;
}

export async function listResource(
  resource: ResourceConfig,
  query: ListQuery,
) {
  return withSession((session) =>
    apiRequest<PaginationResponse<Record<string, unknown>>>(buildListPath(resource, query), {
      method: "GET",
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function getResourceDetail(resource: ResourceConfig, id: string) {
  return withSession((session) =>
    apiRequest<Record<string, unknown>>(`${resource.path}/${encodeURIComponent(id)}`, {
      method: "GET",
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function createResource(resource: ResourceConfig, body: unknown) {
  return withSession((session) =>
    apiRequest<Record<string, unknown>>(resource.path, {
      method: "POST",
      body: JSON.stringify(body),
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function updateResource(resource: ResourceConfig, id: string, body: unknown) {
  return withSession((session) =>
    apiRequest<Record<string, unknown>>(`${resource.path}/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function deleteResource(resource: ResourceConfig, id: string) {
  return withSession((session) =>
    apiRequest<{ status: string }>(`${resource.path}/${encodeURIComponent(id)}`, {
      method: "DELETE",
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function getDashboardPagesMenu() {
  return withSession((session) =>
    apiRequest<DashboardPageConfig[]>("/dashboard/pages/menu", {
      method: "GET",
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function startMarketBackfill(body: BackfillRequest) {
  return withSession((session) =>
    apiRequest<BackfillJob>("/market/backfills", {
      method: "POST",
      body: JSON.stringify(body),
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function getMarketBackfill(id: string, waitSeconds = 0) {
  const wait = Math.max(0, Math.min(30, waitSeconds));
  const path = wait > 0 ? `/market/backfills/${encodeURIComponent(id)}?wait=${wait}` : `/market/backfills/${encodeURIComponent(id)}`;
  return withSession((session) =>
    apiRequest<BackfillJob>(path, {
      method: "GET",
      accessToken: session.tokens.access_token,
    }),
  );
}

export async function updateMyProfile(body: { name: string; password?: string }) {
  return withSession(async (session) => {
    const user = await apiRequest<AuthUser>("/auth/profile", {
      method: "PATCH",
      body: JSON.stringify(body),
      accessToken: session.tokens.access_token,
    });
    writeSession({ ...session, user });
    return user;
  });
}
