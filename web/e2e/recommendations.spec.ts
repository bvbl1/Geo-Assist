import { test, expect, type Page } from "@playwright/test";
async function login(page: Page, admin = false) {
  await page.goto("/login");
  await page
    .getByLabel("Email", { exact: true })
    .fill(admin ? "admin@geoassist.local" : "student44@geoassist.local");
  await page
    .getByLabel("Пароль", { exact: true })
    .fill(admin ? "Admin-demo-2026!" : "Student-demo-2026!");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Выйти", exact: true }),
  ).toBeVisible();
}
test.beforeEach(async ({ page }) => {
  await page.route("https://tile.openstreetmap.org/**", (route) =>
    route.abort(),
  );
  await page.route("https://fonts.googleapis.com/**", (route) => route.abort());
});
test("recommendations: personal results, favorite exclusion, geo filters, empty result and mobile", async ({
  page,
}) => {
  await login(page);
  await page.getByRole("link", { name: "Для вас", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Для вас", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".place-card").first()).toBeVisible();
  const card = page.locator(".place-card .place-title").first();
  const href = await card.getAttribute("href");
  await card.click();
  await page.getByRole("button", { name: "В избранное", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Убрать из избранного", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Для вас", exact: true }).click();
  await expect(page.locator(".place-card").first()).toBeVisible();
  await expect(page.locator(`.place-card a[href="${href}"]`)).toHaveCount(0);
  // Restore the demo user's original favorite state.
  await page.goto(href!);
  await page
    .getByRole("button", { name: "Убрать из избранного", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "В избранное", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Для вас", exact: true }).click();
  await page.getByLabel("Категория", { exact: true }).selectOption("cafe");
  await page.getByLabel("Только рядом с точкой").check();
  await page.getByLabel("Радиус, метры").fill("50000");
  const response = page.waitForResponse(
    (r) =>
      r.url().includes("/me/recommendations?") &&
      r.url().includes("category=cafe"),
  );
  await page.getByRole("button", { name: "Подобрать места" }).click();
  const data = await (await response).json();
  expect(data.data.length).toBeGreaterThan(0);
  for (const item of data.data) {
    expect(item.place.category).toBe("cafe");
    expect(item.place.distance_m).toBeLessThanOrEqual(50000);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".place-card").first()).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/recommendations-mobile.png",
    fullPage: true,
  });
  await page.getByLabel("Теги через запятую").fill("nonexistent-tag");
  await page.getByRole("button", { name: "Подобрать места" }).click();
  await expect(
    page.getByRole("heading", { name: "Подходящих рекомендаций пока нет" }),
  ).toBeVisible();
  const forbidden = await page.request.get(
    "/api/v1/admin/recommendations/status",
  );
  expect(forbidden.status()).toBe(403);
});
test("admin model status and background rebuild", async ({ page }) => {
  await login(page, true);
  await page.goto("/recommendations");
  await expect(
    page.getByText("Популярное среди пользователей", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("link", { name: "Модель рекомендаций", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Активная модель" }),
  ).toBeVisible();
  const response = page.waitForResponse(
    (r) =>
      r.url().endsWith("/admin/recommendations/rebuild") &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Проверить и пересчитать" }).click();
  expect((await response).status()).toBe(202);
  await expect(
    page.getByText(/Последний запрос: (Данные не изменились|Готово)/),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/recommendations-admin.png",
    fullPage: true,
  });
});
