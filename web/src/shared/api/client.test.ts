import { afterEach, expect, it, vi } from "vitest";
import { request, resetRequests } from "./client";
afterEach(() => {
  vi.unstubAllGlobals();
  resetRequests();
});
it("sends cookie credentials and CSRF, but no invented body for DELETE /favorite", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetch);
  await expect(request("/me/favorites/abc", "DELETE")).resolves.toBeUndefined();
  expect(fetch).toHaveBeenCalledWith(
    "/api/v1/me/favorites/abc",
    expect.objectContaining({
      method: "DELETE",
      credentials: "same-origin",
      headers: { "X-GeoAssist-CSRF": "1" },
    }),
  );
  expect(fetch.mock.calls[0][1]).not.toHaveProperty("body");
});
it("preserves validation and Retry-After details", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json(
          {
            error: {
              code: "RATE_LIMITED",
              message: "Wait",
              request_id: "req",
              details: [],
            },
          },
          { status: 429, headers: { "Retry-After": "22" } },
        ),
      ),
  );
  await expect(request("/auth/register", "POST", {})).rejects.toMatchObject({
    status: 429,
    retryAfter: 22,
    requestId: "req",
  });
});
it("rejects a late response after the account changes", async () => {
  let resolve!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((r) => {
          resolve = r;
        }),
    ),
  );
  const result = request("/me");
  resetRequests();
  resolve(Response.json({ data: { id: "old-user" } }));
  await expect(result).rejects.toMatchObject({ name: "AbortError" });
});
