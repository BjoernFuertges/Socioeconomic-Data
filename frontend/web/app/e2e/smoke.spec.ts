import { expect, test } from "@playwright/test";

// Read-only smoke tests against the deployed instance: no mocks, no assumptions about
// concrete data values. E2E_EXPECT_DATA=false skips the data check (fresh server
// before the initial data load).
const expectData = process.env.E2E_EXPECT_DATA !== "false";

test("startet auf /regionen und zeigt die Navigation", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/regionen$/);
  await expect(page.getByRole("heading", { name: "Sozioökonomische Daten" })).toBeVisible();

  for (const name of ["Regionen", "Karte", "Eigene Region", "Datenkatalog"]) {
    await expect(page.getByRole("tab", { name })).toBeVisible();
  }
  await expect(page.getByRole("tab", { name: "Regionen" })).toHaveAttribute("aria-selected", "true");
});

test("API ist erreichbar (kein Fehlerbanner)", async ({ page }) => {
  const health = page.waitForResponse((res) => res.url().includes("/health"));
  await page.goto("/");
  expect((await health).ok()).toBe(true);
  await expect(page.locator(".api-status.error-text")).toHaveCount(0);
});

test("Tab-Navigation wechselt die Route", async ({ page }) => {
  await page.goto("/");

  await page.getByRole("tab", { name: "Datenkatalog" }).click();
  await expect(page).toHaveURL(/\/katalog/);
  await expect(page.getByRole("tab", { name: "Datenkatalog" })).toHaveAttribute("aria-selected", "true");

  await page.getByRole("tab", { name: "Eigene Region" }).click();
  await expect(page).toHaveURL(/\/eigene-region$/);
});

test("unbekannte URL leitet auf /regionen um", async ({ page }) => {
  await page.goto("/gibt-es-nicht");
  await expect(page).toHaveURL(/\/regionen$/);
});

test("Regionenliste enthält Einträge", async ({ page }) => {
  test.skip(!expectData, "E2E_EXPECT_DATA=false");
  await page.goto("/regionen");
  await expect(page.locator(".region-item").first()).toBeVisible({ timeout: 30_000 });
});
