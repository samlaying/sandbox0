import type { ReactNode } from "react";

import type { DashboardNavItem } from "./navigation";

interface DashboardShellProps {
  title: string;
  eyebrow: string;
  description: string;
  navItems: DashboardNavItem[];
  children: ReactNode;
  productLabel?: string;
  productTitle?: string;
  productDescription?: string;
}

export function DashboardShell({
  title,
  eyebrow,
  description,
  navItems,
  children,
  productLabel = "Sandbox0",
  productTitle = "Dashboard",
  productDescription = "Shared dashboard product assembly with extension surfaces.",
}: DashboardShellProps) {
  return (
    <main className="page">
      <div className="shell shell-grid">
        <aside className="sidebar">
          <div className="sidebar-card">
            <p className="hero-kicker">{productLabel}</p>
            <h1 className="sidebar-title">{productTitle}</h1>
            <p className="sidebar-copy">{productDescription}</p>
          </div>

          <nav className="sidebar-card">
            <p className="nav-label">Product areas</p>
            <ul className="nav-list">
              {navItems.map((item) => (
                <li key={item.href}>
                  <a className="nav-link" href={item.href}>
                    <span>{item.label}</span>
                    <small>
                      {item.scope === "shared" ? "shared" : "extension"}
                    </small>
                  </a>
                </li>
              ))}
            </ul>
          </nav>
        </aside>

        <section className="content">
          <header className="hero-card">
            <p className="hero-kicker">{eyebrow}</p>
            <h1 className="hero-title">{title}</h1>
            <p className="hero-copy">{description}</p>
          </header>

          {children}
        </section>
      </div>
    </main>
  );
}

export function DashboardRoutePanel({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <article className="panel">
      <h2>{title}</h2>
      {children}
    </article>
  );
}
