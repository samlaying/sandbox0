import {
  createDashboardBuiltinLoginRoute,
  resolveDashboardRuntimeConfig,
} from "@sandbox0/dashboard-app";

export const POST = createDashboardBuiltinLoginRoute(resolveDashboardRuntimeConfig);
