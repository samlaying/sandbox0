import type {
  DashboardControlPlaneMode,
  DashboardRuntimeConfig,
} from "./types";

function normalizeMode(value: string | undefined): DashboardControlPlaneMode {
  if (value === "global-gateway") {
    return value;
  }
  return "single-cluster";
}

function defaultSiteURL(nodeEnv: string | undefined): string {
  if (nodeEnv === "development") {
    return "http://localhost:4300";
  }
  return "https://sandbox0.ai";
}

function normalizeCookieDomain(value: string): string | undefined {
  const normalized = value.trim().toLowerCase().replace(/^\.+/, "").replace(/\.+$/, "");
  return normalized === "" ? undefined : normalized;
}

function parseCookieDomains(value: string | undefined): string[] | undefined {
  if (!value) {
    return undefined;
  }

  const domains = value
    .split(",")
    .map((entry) => normalizeCookieDomain(entry))
    .filter((entry): entry is string => entry !== undefined);

  if (domains.length === 0) {
    return undefined;
  }

  return [...new Set(domains)];
}

export function resolveDashboardRuntimeConfig(
  env: NodeJS.ProcessEnv = process.env,
): DashboardRuntimeConfig {
  const mode = normalizeMode(env.SANDBOX0_DASHBOARD_MODE);
  const siteURL =
    env.SANDBOX0_DASHBOARD_SITE_URL ?? defaultSiteURL(env.NODE_ENV);
  const cookieDomains = parseCookieDomains(
    env.SANDBOX0_DASHBOARD_COOKIE_DOMAINS,
  );

  if (mode === "global-gateway") {
    return {
      mode,
      siteURL,
      cookieDomains,
      globalGatewayURL:
        env.SANDBOX0_DASHBOARD_GLOBAL_GATEWAY_URL ??
        env.SANDBOX0_BASE_URL ??
        "https://api.sandbox0.ai",
    };
  }

  return {
    mode,
    siteURL,
    cookieDomains,
    singleClusterURL:
      env.SANDBOX0_DASHBOARD_SINGLE_CLUSTER_URL ??
      env.SANDBOX0_BASE_URL ??
      "http://localhost:30080",
  };
}

export function resolveDashboardControlPlaneURL(
  config: DashboardRuntimeConfig,
): string | undefined {
  if (config.mode === "global-gateway") {
    return config.globalGatewayURL;
  }

  return config.singleClusterURL;
}
