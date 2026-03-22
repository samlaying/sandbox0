import {
  createDashboardTeamSelectRoute,
  resolveDashboardRuntimeConfig,
} from "@sandbox0/dashboard-app";

export const POST = createDashboardTeamSelectRoute(resolveDashboardRuntimeConfig);
