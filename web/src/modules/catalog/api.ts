import { request } from "../../shared/api/client";
import type {
  Category,
  Data,
  Page,
  Place,
  PlaceCreate,
  PlacePatch,
} from "../../shared/api/types";
export const catalog = {
  categories: async (signal?: AbortSignal) =>
    (await request<Data<Category[]>>("/categories", "GET", undefined, signal))
      .data,
  search: (params: string, signal?: AbortSignal) =>
    request<Page<Place>>(
      "/places" + (params ? "?" + params : ""),
      "GET",
      undefined,
      signal,
    ),
  get: async (id: string, signal?: AbortSignal) =>
    (
      await request<Data<Place>>(
        "/places/" + encodeURIComponent(id),
        "GET",
        undefined,
        signal,
      )
    ).data,
  create: async (body: PlaceCreate) =>
    (await request<Data<Place>>("/places", "POST", body)).data,
  update: async (id: string, body: PlacePatch) =>
    (await request<Data<Place>>("/places/" + id, "PATCH", body)).data,
  delete: (id: string, version: number) =>
    request<void>(`/places/${id}?expected_version=${version}`, "DELETE"),
};
