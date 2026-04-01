export {
  createDashboardHomePage,
  type DashboardHomePageOptions,
} from "./home-page";
export {
  createDashboardOnboardingPage,
  type DashboardConfigResolver,
  type DashboardOnboardingPageSearchParams,
  type DashboardOnboardingViewOptions,
} from "./onboarding-page";
export { createDashboardOnboardingRoute } from "./onboarding-route";
export {
  DashboardRootLayout,
  createDashboardMetadata,
  type DashboardMetadataOptions,
} from "./layout";
export {
  defaultDashboardNavigation,
  extendDashboardNavigation,
  type DashboardNavItem,
} from "./navigation";
export { DashboardShell, DashboardRoutePanel } from "./route-shell";
export {
  createDashboardSessionRoute,
  createDashboardTeamSelectRoute,
} from "./session-routes";
export {
  createDashboardVolumesPage,
  type DashboardVolumesPageOptions,
} from "./volumes-page";
export {
  createDashboardVolumeForkRoute,
  createDashboardVolumeRoute,
  createDashboardVolumesRoute,
} from "./volumes-routes";

export {
  createDashboardLoginPage,
  requireDashboardAuth,
  requireDashboardHomeRender,
  type DashboardLoginViewOptions,
  type DashboardPageSearchParams,
} from "./internal/auth-pages";
export {
  createDashboardAuthProvidersRoute,
  createDashboardBuiltinLoginRoute,
  createDashboardLogoutRoute,
  createDashboardOIDCCallbackRoute,
  createDashboardOIDCLoginRoute,
  createDashboardRefreshRoute,
} from "./internal/auth-routes";
export {
  resolveDashboardControlPlaneURL,
  resolveDashboardRuntimeConfig,
} from "./internal/config";
export type {
  DashboardActiveTeam,
  DashboardAuthProvider,
  DashboardAuthProviderType,
  DashboardControlPlaneMode,
  DashboardRegion,
  DashboardRuntimeConfig,
  DashboardSandboxSummary,
  DashboardSession,
  DashboardTeam,
  DashboardTemplateSummary,
  DashboardUser,
  DashboardVolumeAccessMode,
  DashboardVolumeSummary,
} from "./internal/types";
