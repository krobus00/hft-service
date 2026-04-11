"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

type NavItem = {
  href: string;
  label: string;
};

type SidebarProps = {
  items: NavItem[];
};

export function Sidebar({ items }: SidebarProps) {
  const pathname = usePathname();

  return (
    <aside className="dashboard-sidebar">
      <div className="brand-panel">
        <div className="brand-headline">
          <span className="brand-pill" aria-hidden="true" />
          <p className="brand-mark">Krobot</p>
        </div>
        <p className="brand-subtitle">Trading Dashboard</p>
      </div>

      <nav className="sidebar-nav">
        <p className="nav-section-title">Navigation</p>
        {items.map((item) => {
          const active = pathname === item.href || pathname.startsWith(`${item.href}/`);

          return (
            <Link
              key={item.href}
              href={item.href}
              className={active ? "nav-link active" : "nav-link"}
            >
              {item.label}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
