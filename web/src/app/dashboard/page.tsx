export default function DashboardPage() {
	return (
		<div className="stack-gap">
			<section className="panel hero-panel">
				<div className="hero-row">
					<div>
						<p className="eyebrow">Overview</p>
						<h2>Welcome to Krobot</h2>
						<p>
							You are logged in. This workspace is ready for market data, order engine, and
							monitoring widgets.
						</p>
					</div>
					<div className="hero-actions">
						<span className="tag-chip">Minimal mode</span>
						<span className="tag-chip">Dashboard v1</span>
					</div>
				</div>
			</section>

			<section className="stats-grid">
				<article className="panel stat-card">
					<p className="stat-label">Gateway Health</p>
					<p className="stat-value">Connected</p>
					<p className="stat-meta">dashboard-gateway reachable</p>
				</article>

				<article className="panel stat-card">
					<p className="stat-label">Environment</p>
					<p className="stat-value">Development</p>
					<p className="stat-meta">local workspace mode</p>
				</article>

				<article className="panel stat-card">
					<p className="stat-label">Ready Modules</p>
					<p className="stat-value">Auth + Layout</p>
					<p className="stat-meta">minimal shell active</p>
				</article>
			</section>

			<section className="dashboard-grid">
				<article className="panel">
					<h2>System Status</h2>
					<ul className="status-list">
						<li>
							<span>Authentication</span>
							<strong>Healthy</strong>
						</li>
						<li>
							<span>Token Refresh</span>
							<strong>Ready</strong>
						</li>
						<li>
							<span>Dashboard Shell</span>
							<strong>Minimal</strong>
						</li>
					</ul>
				</article>

				<article className="panel">
					<h2>Recent Activity</h2>
					<ul className="activity-list">
						<li>
							<strong>Login successful</strong>
							<span>Session established with dashboard-gateway.</span>
						</li>
						<li>
							<strong>Profile loaded</strong>
							<span>Current user details shown in top-right chip.</span>
						</li>
						<li>
							<strong>Dashboard initialized</strong>
							<span>Sidebar and overview layout are active.</span>
						</li>
					</ul>
				</article>
			</section>
		</div>
	);
}
