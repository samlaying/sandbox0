export interface DashboardNavItem {
  href: string;
  label: string;
  scope: "shared" | "extension";
  description: string;
}

export const defaultDashboardNavigation: DashboardNavItem[] = [
  {
    href: "/",
    label: "Overview",
    scope: "shared",
    description: "Workspace entrypoint provided by the shared dashboard app.",
  },
];

export function extendDashboardNavigation(
  extensionItems: DashboardNavItem[],
): DashboardNavItem[] {
  return [...defaultDashboardNavigation, ...extensionItems];
}
