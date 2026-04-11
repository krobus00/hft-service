export default function ProfilePage() {
	return (
		<div className="stack-gap">
			<section className="panel hero-panel">
				<p className="eyebrow">Profile</p>
				<h2>Account Settings</h2>
				<p>
					Manage user preferences, role visibility, and security settings from a single place.
				</p>
			</section>

			<section className="dashboard-grid">
				<article className="panel">
					<h2>Identity</h2>
					<ul className="status-list">
						<li>
							<span>Display Name</span>
							<strong>Loaded from session</strong>
						</li>
						<li>
							<span>Role</span>
							<strong>Visible in header</strong>
						</li>
						<li>
							<span>Status</span>
							<strong>Authenticated</strong>
						</li>
					</ul>
				</article>

				<article className="panel">
					<h2>Security</h2>
					<ul className="activity-list">
						<li>
							<strong>Refresh token enabled</strong>
							<span>Automatic session continuation is active.</span>
						</li>
						<li>
							<strong>Logout available</strong>
							<span>Secure sign-out is accessible in the header.</span>
						</li>
					</ul>
				</article>
			</section>
		</div>
	);
}
