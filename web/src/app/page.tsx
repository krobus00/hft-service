import { redirect } from "next/navigation";

import { readAuthCookies } from "@/lib/server/auth-cookies";

export default async function Home() {
  const { refreshToken } = await readAuthCookies();

  if (refreshToken) {
    redirect("/dashboard");
  }

  redirect("/login");
}
