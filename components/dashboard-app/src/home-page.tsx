import Image from "next/image";
import {
  PixelButton,
  PixelCard,
  PixelInput,
  PixelLayout,
} from "@sandbox0/ui";

import {
  requireDashboardHomeRender,
  type DashboardConfigResolver,
  type DashboardPageSearchParams,
} from "./internal/auth-pages";

export interface DashboardHomePageOptions {
  brandName?: string;
  footerText?: string;
  welcomeTitle?: string;
  welcomeDescription?: string;
}

function DashboardHomeView({
  brandName,
  footerText,
  welcomeTitle,
  welcomeDescription,
}: Required<DashboardHomePageOptions>) {
  return (
    <PixelLayout>
      <header className="flex items-center justify-between border-b border-foreground/10 p-4">
        <div className="flex items-center gap-3">
          <Image
            src="/sandbox0.png"
            alt={brandName}
            width={32}
            height={32}
            className="pixel-art"
            data-pixel
          />
          <h1 className="font-pixel text-sm">{brandName}</h1>
        </div>
        <nav className="flex gap-4">
          <PixelButton variant="secondary" scale="sm">
            Sandboxes
          </PixelButton>
          <PixelButton variant="secondary" scale="sm">
            Templates
          </PixelButton>
          <PixelButton variant="secondary" scale="sm">
            Settings
          </PixelButton>
        </nav>
      </header>

      <main className="flex-1 p-6">
        <div className="mb-8">
          <h2 className="mb-2 font-pixel text-lg">{welcomeTitle}</h2>
          <p className="text-muted">{welcomeDescription}</p>
        </div>

        <div className="mb-8 grid grid-cols-1 gap-6 md:grid-cols-3">
          <PixelCard header="New Sandbox" interactive accent>
            <p className="mb-4 text-sm text-muted">
              Create a new sandbox instance from a template
            </p>
            <PixelButton variant="primary" scale="sm">
              + Create
            </PixelButton>
          </PixelCard>

          <PixelCard header="Running" interactive>
            <p className="mb-2 text-4xl font-pixel text-accent">3</p>
            <p className="text-sm text-muted">Active sandboxes</p>
          </PixelCard>

          <PixelCard header="Storage" interactive>
            <p className="mb-2 text-4xl font-pixel">12.4 GB</p>
            <p className="text-sm text-muted">Total volume usage</p>
          </PixelCard>
        </div>

        <div className="mb-8 max-w-md">
          <PixelInput
            label="Search Sandboxes"
            placeholder="Enter sandbox name or ID..."
          />
        </div>

        <div className="space-y-4">
          <h3 className="mb-4 font-pixel text-sm">Recent Sandboxes</h3>
          {["sandbox-dev-001", "sandbox-test-002", "sandbox-prod-003"].map(
            (name) => (
              <PixelCard key={name} scale="sm" interactive>
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-mono text-sm">{name}</p>
                    <p className="text-xs text-muted">Running · 2h 34m</p>
                  </div>
                  <div className="flex gap-2">
                    <PixelButton variant="secondary" scale="sm">
                      Terminal
                    </PixelButton>
                    <PixelButton variant="secondary" scale="sm">
                      Stop
                    </PixelButton>
                  </div>
                </div>
              </PixelCard>
            ),
          )}
        </div>
      </main>

      <footer className="border-t border-foreground/10 p-4 text-center text-xs text-muted">
        {footerText}
      </footer>
    </PixelLayout>
  );
}

export function createDashboardHomePage(
  resolveConfig: DashboardConfigResolver,
  options?: DashboardHomePageOptions,
) {
  const resolvedOptions: Required<DashboardHomePageOptions> = {
    brandName: options?.brandName ?? "SANDBOX0",
    footerText: options?.footerText ?? "Sandbox0 Dashboard v0.0.1",
    welcomeTitle: options?.welcomeTitle ?? "Welcome back",
    welcomeDescription:
      options?.welcomeDescription ?? "Manage your AI sandboxes",
  };

  return async function DashboardHomePage({
    searchParams,
  }: DashboardPageSearchParams) {
    await requireDashboardHomeRender(resolveConfig, searchParams);
    return <DashboardHomeView {...resolvedOptions} />;
  };
}
