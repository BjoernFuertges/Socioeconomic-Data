import { defineConfig, devices } from "@playwright/test";

// E2E läuft gegen eine bereits deployte Instanz (Produktion in CI, lokal z. B. compose.local.yml),
// startet also keinen eigenen Dev-Server. Ziel-URL per E2E_BASE_URL.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5174",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
