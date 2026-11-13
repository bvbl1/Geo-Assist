import { expect, it, vi } from "vitest";
import { activity } from "./api";
import { recordOnce } from "./event-recorder";
import { resetRequests } from "../../shared/api/client";
vi.mock("./api", () => ({ activity: { record: vi.fn() } }));
it("shares an in-flight view and reuses its UUID after a lost response", async () => {
  const mock = vi.mocked(activity.record);
  mock
    .mockRejectedValueOnce(new Error("lost response"))
    .mockResolvedValue({} as never);
  const first = recordOnce("visit-a", "place-a", "view", "user-a");
  expect(recordOnce("visit-a", "place-a", "view", "user-a")).toBe(first);
  await expect(first).rejects.toThrow("lost response");
  await recordOnce("visit-a", "place-a", "view", "user-a");
  expect(mock.mock.calls[0][2]).toBe(mock.mock.calls[1][2]);
  await recordOnce("visit-a", "place-a", "view", "user-a");
  expect(mock).toHaveBeenCalledTimes(2);
  resetRequests();
  await recordOnce("visit-a", "place-a", "view", "user-b");
  expect(mock).toHaveBeenCalledTimes(3);
  expect(mock.mock.calls[2][2]).not.toBe(mock.mock.calls[1][2]);
});
