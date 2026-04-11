import { NextResponse } from "next/server";

import {
  isDashboardProxyError,
  requestDashboardWithAuth,
} from "@/lib/server/dashboard-auth-request";
import type {
  DashboardUser,
  UpdateUserRequest,
} from "@/lib/types/dashboard-user";

function applyRefreshedCookies(target: NextResponse, source?: NextResponse) {
  if (!source) {
    return;
  }

  for (const cookie of source.cookies.getAll()) {
    target.cookies.set(cookie);
  }
}

export async function GET(_: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;

  try {
    const result = await requestDashboardWithAuth<DashboardUser>(
      `/dashboard/v1/users/${id}`,
      { method: "GET" },
    );

    const response = NextResponse.json({ data: result.data }, { status: 200 });
    applyRefreshedCookies(response, result.refreshedResponse);
    return response;
  } catch (error) {
    if (isDashboardProxyError(error)) {
      return NextResponse.json(
        { error: { code: error.code, message: error.message } },
        { status: error.status },
      );
    }

    return NextResponse.json(
      { error: { message: "failed to fetch user" } },
      { status: 500 },
    );
  }
}

export async function PATCH(request: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;

  let payload: UpdateUserRequest;
  try {
    payload = (await request.json()) as UpdateUserRequest;
  } catch {
    return NextResponse.json(
      { error: { message: "invalid json body" } },
      { status: 400 },
    );
  }

  try {
    const result = await requestDashboardWithAuth<DashboardUser>(
      `/dashboard/v1/users/${id}`,
      {
        method: "PATCH",
        body: JSON.stringify(payload),
      },
    );

    const response = NextResponse.json({ data: result.data }, { status: 200 });
    applyRefreshedCookies(response, result.refreshedResponse);
    return response;
  } catch (error) {
    if (isDashboardProxyError(error)) {
      return NextResponse.json(
        { error: { code: error.code, message: error.message } },
        { status: error.status },
      );
    }

    return NextResponse.json(
      { error: { message: "failed to update user" } },
      { status: 500 },
    );
  }
}

export async function DELETE(_: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;

  try {
    const result = await requestDashboardWithAuth<{ id: string }>(
      `/dashboard/v1/users/${id}`,
      { method: "DELETE" },
    );

    const response = NextResponse.json({ data: result.data }, { status: 200 });
    applyRefreshedCookies(response, result.refreshedResponse);
    return response;
  } catch (error) {
    if (isDashboardProxyError(error)) {
      return NextResponse.json(
        { error: { code: error.code, message: error.message } },
        { status: error.status },
      );
    }

    return NextResponse.json(
      { error: { message: "failed to delete user" } },
      { status: 500 },
    );
  }
}
