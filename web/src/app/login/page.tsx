import { redirect } from "next/navigation";

import { LoginForm } from "@/components/auth/login-form";
import { readAuthCookies } from "@/lib/server/auth-cookies";

export default async function LoginPage() {
	const { refreshToken } = await readAuthCookies();

	if (refreshToken) {
		redirect("/dashboard");
	}

	return <LoginForm />;
}
