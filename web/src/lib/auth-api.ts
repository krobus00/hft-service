import {
  ApiError,
  apiRequest,
  getAuthConfig,
  getCurrentUser,
  logoutSession,
  persistAuthResult,
  refreshSession,
} from "@/lib/api-client";
import type {
  AuthResult,
  LoginPayload,
  Session,
  SetupPayload,
} from "@/types/auth";

export async function setup(payload: SetupPayload) {
  const result = await apiRequest<AuthResult>("/auth/setup", {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return persistAuthResult(result);
}

export async function login(payload: LoginPayload) {
  const result = await apiRequest<AuthResult>("/auth/login", {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return persistAuthResult(result);
}

export async function getMe(session: Session) {
  try {
    return await getCurrentUser(session);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      const refreshed = await refreshSession(session);
      return getCurrentUser(refreshed);
    }
    throw error;
  }
}

export async function logoutCurrentSession() {
  await logoutSession();
}

export { ApiError };
export { getAuthConfig };
