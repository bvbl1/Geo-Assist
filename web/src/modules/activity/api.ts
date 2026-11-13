import { request } from "../../shared/api/client";
import type {
  State,
  Data,
  Favorite,
  Page,
  Interaction,
} from "../../shared/api/types";
export const activity = {
  state: async (id: string, signal?: AbortSignal) =>
    (
      await request<Data<State>>(
        `/me/places/${id}/state`,
        "GET",
        undefined,
        signal,
      )
    ).data,
  favorite: async (id: string, value: boolean) =>
    (
      await request<Data<State>>(
        `/me/favorites/${id}`,
        value ? "PUT" : "DELETE",
      )
    ).data,
  rate: async (id: string, value: number | null) =>
    (
      await request<Data<State>>(
        `/me/ratings/${id}`,
        value === null ? "DELETE" : "PUT",
        value === null ? undefined : { rating: value },
      )
    ).data,
  favorites: (offset: number, signal?: AbortSignal) =>
    request<Page<Favorite>>(
      `/me/favorites?limit=20&offset=${offset}`,
      "GET",
      undefined,
      signal,
    ),
  history: (params: string, signal?: AbortSignal) =>
    request<Page<Interaction>>(
      `/me/interactions?${params}`,
      "GET",
      undefined,
      signal,
    ),
  record: async (
    place_id: string,
    type: "view" | "select",
    event_key: string,
  ) =>
    (
      await request<Data<Interaction>>("/interactions", "POST", {
        place_id,
        type,
        event_key,
      })
    ).data,
};
