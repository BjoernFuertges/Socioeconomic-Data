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

// SPA fallback: a hard reload on a deep link must be answered by index.html (nginx try_files),
// not by a 404, and the router must keep the route.
test("Deep-Link-Reload bleibt auf der Route (SPA-Fallback)", async ({ page }) => {
  for (const [path, tab] of [
    ["/katalog", "Datenkatalog"],
    ["/eigene-region", "Eigene Region"],
  ] as const) {
    const response = await page.goto(path);
    expect(response?.status()).toBe(200);
    await page.reload();
    await expect(page).toHaveURL(new RegExp(`${path}$`));
    await expect(page.getByRole("heading", { name: "Sozioökonomische Daten" })).toBeVisible();
    await expect(page.getByRole("tab", { name: tab })).toHaveAttribute("aria-selected", "true");
  }
});

test("Region mit Datensatz zeigt Chart, auch nach Reload des Deep-Links", async ({ page }) => {
  test.skip(!expectData, "E2E_EXPECT_DATA=false");
  await page.goto("/regionen");
  await page.locator(".region-item").first().click({ timeout: 30_000 });
  await expect(page).toHaveURL(/\/regionen\/[^/?]+/);

  const firstDataset = page.locator(".dataset-checkbox input[type=checkbox]").first();
  await firstDataset.check({ timeout: 30_000 });
  const chart = page.locator(".chart-wrap svg").first();
  const noValues = page.getByText("Keine Werte für diesen Datensatz");
  await expect(chart.or(noValues).first()).toBeVisible({ timeout: 45_000 });
  // The first region/dataset of the list may legitimately have no values; that is a data
  // property, not a regression, so don't fail the deployment gate on it.
  test.skip(await noValues.isVisible(), "erste Region/erster Datensatz ohne Werte");
  await expect(chart).toBeVisible();

  // Deep link incl. ?dataset=: the reload must restore region and chart.
  const deepLink = page.url();
  expect(deepLink).toContain("dataset=");
  await page.reload();
  await expect(page).toHaveURL(deepLink);
  await expect(page.locator(".chart-wrap svg").first()).toBeVisible({ timeout: 45_000 });
});

test("Regionenliste enthält Einträge", async ({ page }) => {
  test.skip(!expectData, "E2E_EXPECT_DATA=false");
  await page.goto("/regionen");
  await expect(page.locator(".region-item").first()).toBeVisible({ timeout: 30_000 });
});
