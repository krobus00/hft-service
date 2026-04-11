import { NextRequest, NextResponse } from "next/server";

import {
  isDashboardProxyError,
  requestDashboardWithAuth,
} from "@/lib/server/dashboard-auth-request";
import type {
  CreateUserRequest,
  ListUsersPayload,
} from "@/lib/types/dashboard-user";

function applyRefreshedCookies(target: NextResponse, source?: NextResponse) {
  if (!source) {
    return;
  }

  for (const cookie of source.cookies.getAll()) {
    target.cookies.set(cookie);
  }
}

export async function GET(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  const page = searchParams.get("page") ?? "1";
  const pageSize = searchParams.get("page_size") ?? "10";
  const search = searchParams.get("search") ?? "";

  const query = new URLSearchParams({
    page,
    page_size: pageSize,
  });

  if (search.trim()) {
    query.set("search", search.trim());
  }

  try {
    const result = await requestDashboardWithAuth<ListUsersPayload>(
      `/dashboard/v1/users?${query.toString()}`,
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
      { error: { message: "failed to fetch users" } },
      { status: 500 },
    );
  }
}

export async function POST(request: Request) {
  let payload: CreateUserRequest;

  try {
    payload = (await request.json()) as CreateUserRequest;
  } catch {
    return NextResponse.json(
      { error: { message: "invalid json body" } },
      { status: 400 },
    );
  }

  try {
    const result = await requestDashboardWithAuth(
      "/dashboard/v1/users",
      {
        method: "POST",
        body: JSON.stringify(payload),
      },
    );

    const response = NextResponse.json({ data: result.data }, { status: 201 });
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
      { error: { message: "failed to create user" } },
      { status: 500 },
    );
  }
}
