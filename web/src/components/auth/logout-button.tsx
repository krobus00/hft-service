"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

export function LogoutButton() {
	const router = useRouter();
	const [loading, setLoading] = useState(false);

	const handleLogout = async () => {
		setLoading(true);

		try {
			await fetch("/api/auth/logout", {
				method: "POST",
				credentials: "include",
			});
		} finally {
			router.replace("/login");
			router.refresh();
			setLoading(false);
		}
	};

	return (
		<button type="button" className="logout-button" onClick={handleLogout} disabled={loading}>
			{loading ? "Logging out..." : "Logout"}
		</button>
	);
}
