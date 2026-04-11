import type { ReactNode } from "react";

import { Sidebar } from "@/components/dashboard/sidebar";
import { UserChip } from "@/components/dashboard/user-chip";

const navItems = [
	{ href: "/dashboard", label: "Overview" },
	{ href: "/dashboard/users", label: "Users" },
	{ href: "/dashboard/profile", label: "Profile" },
];

export default function DashboardLayout({ children }: { children: ReactNode }) {
	return (
		<div className="dashboard-root">
			<Sidebar items={navItems} />

			<section className="dashboard-main">
				<div className="dashboard-main-inner">
					<header className="dashboard-header">
						<div className="dashboard-title">
							<p className="eyebrow">Control Center</p>
							<h1>Dashboard</h1>
						</div>
						<UserChip />
					</header>

					<main className="dashboard-content">{children}</main>
				</div>
			</section>
		</div>
	);
}
