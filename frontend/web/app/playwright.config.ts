import { defineConfig, devices } from "@playwright/test";

// E2E runs against an already deployed instance (production in CI, locally e.g. compose.local.yml),
// so it starts no dev server of its own. Target URL via E2E_BASE_URL.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  // The API (Traefik) rate-limits per IP (heavy GETs: 4 concurrent, 20/min); in CI all tests run from one IP
  // against production, so run them one at a time.
  workers: process.env.CI ? 1 : undefined,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5174",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
