import { expect, it } from "vitest";
import { registerSchema, profileSchema, passwordBytes } from "./schemas";
import { safeReturn } from "../../shared/lib/format";
const user = {
  email: "new@example.org",
  display_name: "Студент",
  preferred_categories: [],
};
it("validates passwords in UTF-8 bytes and preserves whitespace", () => {
  expect(registerSchema.safeParse({ ...user, password: "short" }).success).toBe(
    false,
  );
  expect(passwordBytes("я".repeat(36))).toBe(72);
  expect(
    registerSchema.safeParse({ ...user, password: "я".repeat(36) }).success,
  ).toBe(true);
  expect(
    registerSchema.safeParse({ ...user, password: "я".repeat(37) }).success,
  ).toBe(false);
  expect(
    registerSchema.parse({ ...user, password: " 12345678901 " }).password,
  ).toBe(" 12345678901 ");
});
it("accepts clearing preferred categories", () =>
  expect(
    profileSchema.parse({ display_name: "Тест", preferred_categories: [] })
      .preferred_categories,
  ).toEqual([]));
it.each([
  "https://evil.example",
  "//evil.example",
  "/\\evil.example",
  "/login",
  "/register",
])("rejects unsafe or cyclic return URL %s", (value) =>
  expect(safeReturn(value)).toBe("/explore"),
);
