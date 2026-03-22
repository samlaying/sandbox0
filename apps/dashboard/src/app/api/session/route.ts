import {
  createDashboardSessionRoute,
  resolveDashboardRuntimeConfig,
} from "@sandbox0/dashboard-app";

export const dynamic = "force-dynamic";
export const GET = createDashboardSessionRoute(resolveDashboardRuntimeConfig);
