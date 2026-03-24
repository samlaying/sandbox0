import assert from "node:assert/strict";
import test from "node:test";

import { handleDashboardOIDCCallbackRequest } from "./browser-auth-routes";
import type { DashboardRuntimeConfig } from "./types";

const config: DashboardRuntimeConfig = {
  mode: "global-gateway",
  siteURL: "https://cloud.sandbox0.ai",
  globalGatewayURL: "https://api.sandbox0.ai",
};

test("oidc callback redirects to configured site url instead of internal request host", async () => {
  const response = await handleDashboardOIDCCallbackRequest(
    config,
    new Request(
      "http://0.0.0.0:4401/api/auth/oidc/supabase/callback?code=code-123&state=state-456",
    ),
    "supabase",
    {
      fetchImpl: async (input, init) => {
        assert.equal(
          String(input),
          "https://api.sandbox0.ai/auth/oidc/supabase/callback?code=code-123&state=state-456",
        );
        assert.equal(init?.method, "GET");
        return new Response(
          JSON.stringify({
            data: {
              access_token: "oidc-access-token",
              refresh_token: "oidc-refresh-token",
              expires_at: Math.floor(Date.now() / 1000) + 3600,
            },
          }),
        );
      },
    },
  );

  assert.equal(response.status, 303);
  assert.equal(response.headers.get("location"), "https://cloud.sandbox0.ai/");
});
