import type { Metadata } from "next";
import type { ReactNode } from "react";

export interface DashboardMetadataOptions {
  title?: string;
  description?: string;
}

export function createDashboardMetadata(
  options?: DashboardMetadataOptions,
): Metadata {
  return {
    title: options?.title ?? "Sandbox0 Dashboard",
    description:
      options?.description ??
      "Topology-aware control plane for Sandbox0 deployments",
  };
}

export function DashboardRootLayout({
  children,
}: Readonly<{
  children: ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body>{children}</body>
    </html>
  );
}
