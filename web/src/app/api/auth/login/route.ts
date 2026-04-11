import { NextResponse } from "next/server";

import { writeAuthCookies } from "@/lib/server/auth-cookies";
import {
  isDashboardGatewayError,
  loginWithGateway,
} from "@/lib/server/dashboard-gateway";
import type { LoginRequest } from "@/lib/types/auth";

export async function POST(request: Request) {
  let payload: LoginRequest;

  try {
    payload = (await request.json()) as LoginRequest;
  } catch {
    return NextResponse.json(
      { error: { message: "invalid json body" } },
      { status: 400 },
    );
  }

  if (!payload.username?.trim() || !payload.password?.trim()) {
    return NextResponse.json(
      { error: { message: "username and password are required" } },
      { status: 400 },
    );
  }

  try {
    const session = await loginWithGateway({
      username: payload.username.trim(),
      password: payload.password,
    });

    const response = NextResponse.json(
      {
        data: {
          user_id: session.user_id,
          username: session.username,
          role: session.role,
        },
        message: "login success",
      },
      { status: 200 },
    );

    writeAuthCookies(response, session);
    return response;
  } catch (error) {
    if (isDashboardGatewayError(error)) {
      return NextResponse.json(
        { error: { code: error.code, message: error.message } },
        { status: error.status || 401 },
      );
    }

    return NextResponse.json(
      { error: { message: "failed to login" } },
      { status: 500 },
    );
  }
}
