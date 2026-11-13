import { test, expect, type Page } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  // Real API and MongoDB; external map/font availability is tested separately.
  await page.route("https://tile.openstreetmap.org/**", (route) =>
    route.abort(),
  );
  await page.route("https://fonts.googleapis.com/**", (route) => route.abort());
});
async function login(
  page: Page,
  email = "student02@geoassist.local",
  password = "Student-demo-2026!",
) {
  await page.goto("/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Выйти", exact: true }),
  ).toBeVisible();
}
async function api(page: Page, path: string, method = "GET", body?: unknown) {
  return page.evaluate(
    async ({ path, method, body }) => {
      const response = await fetch("/api/v1" + path, {
        method,
        headers: {
          ...(method !== "GET" ? { "X-GeoAssist-CSRF": "1" } : {}),
          ...(body === undefined ? {} : { "Content-Type": "application/json" }),
        },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      return {
        status: response.status,
        json: response.status === 204 ? null : await response.json(),
      };
    },
    { path, method, body },
  );
}
test("guest catalog, filters, nearby search, pagination and mobile fallback", async ({
  page,
}) => {
  await page.goto("/explore");
  await expect(
    page.getByRole("heading", { name: "Найдите своё место" }),
  ).toBeVisible();
  await expect(page.locator(".place-card")).toHaveCount(20);
  await page.getByRole("button", { name: "Далее", exact: true }).click();
  await expect(page.getByText("Страница 2", { exact: true })).toBeVisible();
  const filtered = page.url();
  await page.locator(".place-card .place-title").first().click();
  await expect(
    page.getByText("чтобы сохранять места и ставить оценки.", { exact: false }),
  ).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(filtered);
  await page.getByLabel("Категория", { exact: true }).selectOption("cafe");
  await page.getByLabel("Теги через запятую", { exact: true }).fill("wifi");
  await page.getByLabel("Искать рядом", { exact: false }).check();
  await page.getByLabel("Радиус, метры").fill("50000");
  await page.getByRole("button", { name: "Найти", exact: true }).click();
  await expect(page.locator(".place-card").first()).toBeVisible();
  const url = new URL(page.url());
  expect(url.searchParams.get("longitude")).toBe("71.4304");
  const results = await api(page, "/places?" + url.searchParams.toString());
  expect(results.json.data.length).toBeGreaterThan(0);
  for (const p of results.json.data) {
    expect(p.category).toBe("cafe");
    expect(p.tags).toContain("wifi");
    expect(p.distance_m).toBeLessThanOrEqual(50000);
  }
  await expect(
    page.getByText("Подложка недоступна.", { exact: false }),
  ).toBeVisible();
  await page.evaluate(() => {
    Object.defineProperty(navigator, "geolocation", {
      configurable: true,
      value: {
        getCurrentPosition: (
          _success: unknown,
          failure: (error: object) => void,
        ) => failure({ code: 1 }),
      },
    });
  });
  await page.getByRole("button", { name: "Моё местоположение" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Местоположение недоступно" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".place-card").first()).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/mobile-explore.png",
    fullPage: true,
  });
  await page.goto("/explore?longitude=71");
  await expect(page.getByRole("alert")).toContainText(
    "Укажите центр и радиус полностью",
  );
  await page.goto("/favorites");
  await expect(page).toHaveURL(/\/login\?returnTo=/);
  const missing = await page.request.get("/api/v1/no-such-endpoint");
  expect(missing.status()).toBe(404);
  expect(missing.headers()["content-type"]).toContain("application/json");
  expect((await page.request.get("/assets/no-such-file.js")).status()).toBe(
    404,
  );
});

test("registration, cookie login, profile, personal state and account isolation", async ({
  page,
  context,
}) => {
  const email = `web-${Date.now()}@example.org`;
  await page.goto("/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Имя", { exact: true }).fill("Тест сайта");
  await page.locator('input[name="password"]').fill("12345");
  await page
    .getByRole("button", { name: "Создать аккаунт", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("12 до 72 байт");
  await page.locator('input[name="password"]').fill("Student-test-2026!");
  await page.locator('input[type="checkbox"]').first().check();
  await page
    .getByRole("button", { name: "Создать аккаунт", exact: true })
    .click();
  await expect(page).toHaveURL(/\/login\?created=1/);
  expect(
    (await context.cookies()).some((c) => c.name === "geoassist_session"),
  ).toBe(false);
  await page.getByLabel("Пароль", { exact: true }).fill("Student-test-2026!");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page.getByRole("button", { name: "Выйти" })).toBeVisible();
  const cookie = (await context.cookies()).find(
    (c) => c.name === "geoassist_session",
  )!;
  expect(cookie.httpOnly).toBe(true);
  expect(cookie.sameSite).toBe("Lax");
  await page.reload();
  await page.getByRole("link", { name: "Тест сайта", exact: true }).click();
  await page.getByLabel("Имя", { exact: true }).fill("Тест обновлён");
  for (const checkbox of await page.locator('input[type="checkbox"]').all())
    await checkbox.uncheck();
  await page.getByRole("button", { name: "Сохранить изменения" }).click();
  await expect(page.getByText("Профиль сохранён")).toBeVisible();
  expect((await api(page, "/me")).json.data.preferred_categories).toEqual([]);
  await page.goto("/explore");
  await page.locator(".place-card .place-title").first().click();
  const id = new URL(page.url()).pathname.split("/").pop()!;
  await expect(
    page.getByRole("button", { name: "В избранное", exact: true }),
  ).toBeVisible();
  // A response is deliberately lost AFTER the server commits select. Retry must reuse UUID.
  let lost = false;
  await page.route("**/api/v1/interactions", async (route) => {
    if (route.request().postDataJSON().type === "select" && !lost) {
      lost = true;
      await route.fetch();
      await route.abort();
      return;
    }
    await route.continue();
  });
  await page.getByRole("button", { name: "В избранное", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Убрать из избранного" }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Оценить на 5", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Оценить на 5", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "Оценить на 4", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Оценить на 4", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "Оценить на 4", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Оценить на 4", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Выбрать это место" }).click();
  await expect(page.getByRole("alert")).toContainText("Нет ответа от сервера");
  await page.getByRole("button", { name: "Повторить", exact: true }).click();
  await expect(page.getByText("Выбор сохранён в истории")).toBeVisible();
  const events = (await api(page, "/me/interactions?place_id=" + id)).json.data;
  expect(
    events.filter((e: { type: string }) => e.type === "view"),
  ).toHaveLength(1);
  expect(
    events.filter((e: { type: string }) => e.type === "select"),
  ).toHaveLength(1);
  expect(
    events.filter((e: { type: string }) => e.type === "rating_set"),
  ).toHaveLength(2);
  await page
    .getByRole("button", { name: "Убрать оценку", exact: true })
    .click();
  await expect(page.getByText("Нет оценки", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Избранное", exact: true }).click();
  await expect(page.locator(".place-card")).toHaveCount(1);
  await page.getByRole("button", { name: "Убрать из избранного" }).click();
  await expect(
    page.getByRole("heading", { name: "Ваша коллекция начинается здесь" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "История", exact: true }).click();
  await expect(page.locator(".history-item")).toHaveCount(7);
  await page.getByLabel("Тип события").selectOption("select");
  await expect(page.locator(".history-item")).toHaveCount(1);
  await page.goto("/admin/places");
  await expect(
    page.getByRole("heading", { name: "Доступ только для администратора" }),
  ).toBeVisible();
  expect(
    (
      await api(page, "/places", "POST", {
        name: "Forbidden test",
        description: "A valid body to reach the role check",
        address: "Astana",
        category: "cafe",
        tags: [],
        location: { longitude: 71.43, latitude: 51.12 },
      })
    ).status,
  ).toBe(403);
  const otherTab = await context.newPage();
  await otherTab.goto("/profile");
  await expect(otherTab.getByLabel("Имя", { exact: true })).toHaveValue(
    "Тест обновлён",
  );
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(otherTab).toHaveURL(/\/login\?returnTo=/);
  await login(page);
  await page.goto("/profile");
  await expect(page.getByLabel("Email", { exact: true })).toHaveValue(
    "student02@geoassist.local",
  );
  await context.clearCookies();
  await page.goto("/favorites");
  await expect(page).toHaveURL(/\/login\?returnTo=/);
  await otherTab.close();
});

test("admin CRUD, stale version conflict, and cleanup after soft delete", async ({
  page,
  context,
}) => {
  await login(page, "admin@geoassist.local", "Admin-demo-2026!");
  await page.getByRole("link", { name: "Управление", exact: true }).click();
  await page.getByRole("link", { name: "Добавить место" }).click();
  const name = "E2E Астана " + Date.now();
  await page.getByLabel("Название", { exact: true }).fill(name);
  await page
    .getByLabel("Описание", { exact: true })
    .fill("Место для проверки настоящего интерфейса и MongoDB.");
  await page.getByLabel("Категория", { exact: true }).selectOption("library");
  await page
    .getByLabel("Теги через запятую", { exact: false })
    .fill("wifi, quiet");
  await page
    .getByLabel("Адрес", { exact: true })
    .fill("Астана, тестовая улица, 42");
  await page.getByLabel("Долгота", { exact: true }).fill("71.4304");
  await page.getByLabel("Широта", { exact: true }).fill("51.1282");
  await page
    .getByRole("button", { name: "Создать место", exact: true })
    .click();
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  const id = new URL(page.url()).pathname.split("/").pop()!;
  await page.getByRole("button", { name: "В избранное", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Убрать из избранного" }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Оценить на 5", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Оценить на 5", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await page
    .getByRole("link", { name: "Редактировать место", exact: false })
    .click();
  const second = await context.newPage();
  await second.route("https://tile.openstreetmap.org/**", (route) =>
    route.abort(),
  );
  await second.goto("/admin/places/" + id + "/edit");
  await expect(second.getByLabel("Название", { exact: true })).toHaveValue(
    name,
  );
  await second
    .getByLabel("Описание", { exact: true })
    .fill("Черновик второй вкладки");
  await page.getByLabel("Название", { exact: true }).fill(name + " новое");
  await page.getByRole("button", { name: "Сохранить изменения" }).click();
  await expect(
    page.getByRole("heading", { name: name + " новое", exact: true }),
  ).toBeVisible();
  await second.getByRole("button", { name: "Сохранить изменения" }).click();
  await expect(second.getByText("Есть новая версия: 2")).toBeVisible();
  await expect(second.getByLabel("Описание", { exact: true })).toHaveValue(
    "Черновик второй вкладки",
  );
  await second
    .getByRole("button", { name: "Загрузить актуальные данные" })
    .click();
  await expect(second.getByLabel("Название", { exact: true })).toHaveValue(
    name + " новое",
  );
  await second
    .getByLabel("Описание", { exact: true })
    .fill("Обновлено после разрешения конфликта");
  await second.getByRole("button", { name: "Сохранить изменения" }).click();
  await expect(
    second.getByText("Обновлено после разрешения конфликта", { exact: true }),
  ).toBeVisible();
  expect((await api(second, "/places/" + id)).json.data.version).toBe(3);
  await second.close();
  await page.goto("/admin/places?q=" + encodeURIComponent(name));
  await expect(page.locator(".place-card")).toHaveCount(1);
  await page.getByRole("button", { name: "Удалить", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Удалить", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(page.locator(".place-card")).toHaveCount(0);
  expect((await api(page, "/places/" + id)).status).toBe(404);
  const favorites = (await api(page, "/me/favorites?limit=100")).json.data;
  expect(
    favorites.some((f: { place: { id: string } }) => f.place.id === id),
  ).toBe(false);
  await page.goto("/history?place_id=" + id);
  await expect(
    page.getByText("Место недоступно", { exact: true }).first(),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Управление сохранённым состоянием" })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Убрать из избранного", exact: true })
    .click();
  // The newly inserted event changes the first row, but the expanded row keeps its event ID.
  await expect(
    page.getByRole("button", { name: "В избранное", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Убрать оценку", exact: true })
    .click();
  await expect(page.getByText("Нет оценки", { exact: true })).toBeVisible();
  expect(
    (await api(page, "/me/places/" + id + "/state")).json.data,
  ).toMatchObject({ favorite: false, rating: null });
});

test("network, CSRF and rate-limit errors stay actionable", async ({
  page,
}) => {
  await page.route("**/api/v1/places?**", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: {
          code: "SERVICE_UNAVAILABLE",
          details: [],
          request_id: "e2e-503",
        },
      }),
    }),
  );
  await page.goto("/explore");
  await expect(page.getByRole("alert")).toContainText(
    "Сервис временно недоступен",
  );
  await page.unroute("**/api/v1/places?**");
  await page.getByRole("button", { name: "Повторить", exact: true }).click();
  await expect(page.locator(".place-card")).toHaveCount(20);
  await page.goto("/login");
  await page
    .getByLabel("Email", { exact: true })
    .fill("student03@geoassist.local");
  await page.getByLabel("Пароль", { exact: true }).fill("Student-demo-2026!");
  await page.route("**/api/v1/auth/login", (route) =>
    route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "CSRF_REJECTED", details: [] } }),
    }),
  );
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Не удалось отправить запрос",
  );
  await page.unroute("**/api/v1/auth/login");
  await page.route("**/api/v1/auth/login", (route) =>
    route.fulfill({
      status: 429,
      headers: { "Retry-After": "2" },
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "RATE_LIMITED", details: [] } }),
    }),
  );
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page.getByRole("button", { name: /Подождите/ })).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Войти", exact: true }),
  ).toBeEnabled();
});
