import { NextResponse } from "next/server";

import { clearAuthCookies } from "@/lib/server/auth-cookies";

export async function POST() {
  const response = NextResponse.json({ message: "logout success" }, { status: 200 });
  clearAuthCookies(response);
  return response;
}
