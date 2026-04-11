import { NextResponse } from "next/server";

import { readAuthCookies, writeAuthCookies } from "@/lib/server/auth-cookies";
import {
  getMeFromGateway,
  isDashboardGatewayError,
  refreshWithGateway,
} from "@/lib/server/dashboard-gateway";

export async function GET() {
  const { accessToken, refreshToken, username } = await readAuthCookies();

  if (!accessToken && !refreshToken) {
    return NextResponse.json({ error: { message: "unauthorized" } }, { status: 401 });
  }

  const loadProfile = async (token: string) => {
    const profile = await getMeFromGateway(token);
    return {
      userId: profile.user_id,
      role: profile.role,
      username,
    };
  };

  try {
    if (accessToken) {
      const profile = await loadProfile(accessToken);
      return NextResponse.json({ data: profile }, { status: 200 });
    }
  } catch (error) {
    if (!isDashboardGatewayError(error) || error.status !== 401) {
      return NextResponse.json(
        { error: { message: "failed to fetch current user" } },
        { status: 500 },
      );
    }
  }

  if (!refreshToken) {
    return NextResponse.json({ error: { message: "unauthorized" } }, { status: 401 });
  }

  try {
    const session = await refreshWithGateway(refreshToken);
    const profile = await loadProfile(session.access_token);

    const response = NextResponse.json({ data: profile }, { status: 200 });
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
      { error: { message: "failed to fetch current user" } },
      { status: 500 },
    );
  }
}
