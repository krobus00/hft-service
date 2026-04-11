import { NextResponse } from "next/server";

import { readAuthCookies, writeAuthCookies } from "@/lib/server/auth-cookies";
import {
  isDashboardGatewayError,
  refreshWithGateway,
} from "@/lib/server/dashboard-gateway";

export async function POST() {
  const { refreshToken } = await readAuthCookies();

  if (!refreshToken) {
    return NextResponse.json(
      { error: { message: "missing refresh token" } },
      { status: 401 },
    );
  }

  try {
    const session = await refreshWithGateway(refreshToken);
    const response = NextResponse.json({ message: "refresh success" }, { status: 200 });

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
      { error: { message: "failed to refresh token" } },
      { status: 500 },
    );
  }
}
