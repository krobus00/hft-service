"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type LoginApiResponse = {
	error?: {
		message: string;
	};
};

export function LoginForm() {
	const router = useRouter();
	const [username, setUsername] = useState("");
	const [password, setPassword] = useState("");
	const [error, setError] = useState("");
	const [loading, setLoading] = useState(false);

	const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		setError("");
		setLoading(true);

		try {
			const response = await fetch("/api/auth/login", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				credentials: "include",
				body: JSON.stringify({
					username,
					password,
				}),
			});

			const payload = (await response.json()) as LoginApiResponse;

			if (!response.ok) {
				setError(payload.error?.message ?? "Login failed");
				return;
			}

			router.replace("/dashboard");
			router.refresh();
		} catch {
			setError("Network error. Please try again.");
		} finally {
			setLoading(false);
		}
	};

	return (
		<div className="auth-shell">
			<div className="auth-card">
				<div className="auth-brand-row">
					<div className="auth-brand-dot" aria-hidden="true" />
					<p className="eyebrow">Krobot Dashboard</p>
				</div>
				<h1>Welcome back</h1>
				<p className="auth-copy">Sign in using your dashboard username and password.</p>

				<form onSubmit={handleSubmit} className="auth-form">
					<label htmlFor="username">Username</label>
					<input
						id="username"
						name="username"
						autoComplete="username"
						value={username}
						onChange={(event) => setUsername(event.target.value)}
						placeholder="username"
						required
					/>

					<label htmlFor="password">Password</label>
					<input
						id="password"
						name="password"
						type="password"
						autoComplete="current-password"
						value={password}
						onChange={(event) => setPassword(event.target.value)}
						placeholder="••••••••"
						required
					/>

					{error ? <p className="auth-error">{error}</p> : null}

					<button type="submit" className="primary-button" disabled={loading}>
						{loading ? "Signing in..." : "Sign in"}
					</button>
				</form>
			</div>
		</div>
	);
}
