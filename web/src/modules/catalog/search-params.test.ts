import { describe, expect, it } from "vitest";
import { initialSearch, buildSearch, readSearch } from "./search-params";
import { mapCenter } from "../../shared/lib/validation";
describe("search URL contract", () => {
  it("serializes repeated AND tags, all geo fields, and restores the same page", () => {
    const query = buildSearch(
      {
        ...initialSearch,
        q: " Астана ",
        category: "cafe",
        tags: "wifi, quiet",
        nearby: true,
      },
      20,
    );
    const params = new URLSearchParams(query);
    expect(params.getAll("tag")).toEqual(["quiet", "wifi"]);
    expect(params.get("longitude")).toBe("71.4304");
    expect(params.get("latitude")).toBe("51.1282");
    expect(readSearch(params)).toMatchObject({ query, offset: 20, error: "" });
  });
  it.each([
    "longitude=71",
    "longitude=&latitude=51&radius_m=10",
    "longitude=181&latitude=51&radius_m=10",
    "longitude=71&latitude=91&radius_m=10",
    "longitude=71&latitude=51&radius_m=0",
    "longitude=71&latitude=51&radius_m=50001",
    "offset=NaN",
    "offset=1.5",
    "offset=-1",
    "category=unknown",
    "q=a&q=b",
    "limit=100",
    "tag=wifi&tag=wifi",
  ])("rejects malformed URL: %s", (q) =>
    expect(readSearch(new URLSearchParams(q)).error).not.toBe(""),
  );
  it("omits geography when disabled and does not swap zero coordinates", () => {
    expect(
      new URLSearchParams(buildSearch(initialSearch)).has("longitude"),
    ).toBe(false);
    expect(mapCenter("0", "0")).toEqual({ longitude: 0, latitude: 0 });
    expect(mapCenter("", "100")).toEqual({
      longitude: 71.4304,
      latitude: 51.1282,
    });
  });
});
