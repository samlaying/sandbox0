import {
  createDashboardRefreshRoute,
  resolveDashboardRuntimeConfig,
} from "@sandbox0/dashboard-app";

export const GET = createDashboardRefreshRoute(resolveDashboardRuntimeConfig);
