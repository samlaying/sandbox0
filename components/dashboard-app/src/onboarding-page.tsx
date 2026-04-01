import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { resolveDashboardOnboardingEntry } from "./internal/browser-auth";
import { DashboardOnboardingView } from "./internal/onboarding-view";
import type { DashboardRuntimeConfig } from "./internal/types";

export interface DashboardOnboardingPageSearchParams {
  searchParams: Promise<{ onboarding_error?: string }>;
}

export interface DashboardOnboardingViewOptions {
  logoSrc?: string;
  brandName?: string;
  onboardingPath?: string;
}

export type DashboardConfigResolver = () => DashboardRuntimeConfig;

export function createDashboardOnboardingPage(
  resolveConfig: DashboardConfigResolver,
  options?: DashboardOnboardingViewOptions,
) {
  return async function DashboardOnboardingPage({
    searchParams,
  }: DashboardOnboardingPageSearchParams) {
    const { onboarding_error: onboardingError } = await searchParams;
    const result = await resolveDashboardOnboardingEntry(
      resolveConfig(),
      await cookies(),
    );

    if (result.kind === "redirect") {
      redirect(result.location);
    }

    return (
      <DashboardOnboardingView
        onboardingError={onboardingError ?? result.session.errors[0]}
        userEmail={result.session.user?.email}
        regions={result.regions}
        {...options}
      />
    );
  };
}
