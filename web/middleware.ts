import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import { REFRESH_TOKEN_COOKIE } from "@/lib/server/auth-config";

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const refreshToken = request.cookies.get(REFRESH_TOKEN_COOKIE)?.value;

  if (pathname.startsWith("/dashboard") && !refreshToken) {
    const loginUrl = new URL("/login", request.url);
    return NextResponse.redirect(loginUrl);
  }

  if (pathname === "/login" && refreshToken) {
    const dashboardUrl = new URL("/dashboard", request.url);
    return NextResponse.redirect(dashboardUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*", "/login"],
};
