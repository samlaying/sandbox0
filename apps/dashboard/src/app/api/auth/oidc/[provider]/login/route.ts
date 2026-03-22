import {
  createDashboardOIDCLoginRoute,
  resolveDashboardRuntimeConfig,
} from "@sandbox0/dashboard-app";

export const GET = createDashboardOIDCLoginRoute(resolveDashboardRuntimeConfig);
