import {
  createDashboardLogoutRoute,
  resolveDashboardRuntimeConfig,
} from "@sandbox0/dashboard-app";

export const POST = createDashboardLogoutRoute(resolveDashboardRuntimeConfig);
