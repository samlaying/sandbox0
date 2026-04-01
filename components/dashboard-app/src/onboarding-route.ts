import { cookies } from "next/headers";

import { handleDashboardOnboardingRequest } from "./internal/browser-auth-routes";
import type { DashboardRuntimeConfig } from "./internal/types";

type RouteDashboardConfigResolver = () => DashboardRuntimeConfig;

export function createDashboardOnboardingRoute(
  resolveConfig: RouteDashboardConfigResolver,
) {
  return async function POST(request: Request) {
    return handleDashboardOnboardingRequest(
      resolveConfig(),
      request,
      await cookies(),
    );
  };
}
