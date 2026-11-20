import { request } from "../../shared/api/client";
import type { Data } from "../../shared/api/types";

import type { components } from "../../shared/api/generated/schema";
export type Recommendations = components["schemas"]["RecommendationResponse"];
export type Attempt = components["schemas"]["RecommendationModel"];
export type ModelStatus = components["schemas"]["RecommendationStatus"];
export const recommendations = {
  get: (params: string, signal?: AbortSignal) =>
    request<Recommendations>(
      "/me/recommendations?" + params,
      "GET",
      undefined,
      signal,
    ),
  status: async (id?: string, signal?: AbortSignal) =>
    (
      await request<Data<ModelStatus>>(
        "/admin/recommendations/status" +
          (id ? "?build_id=" + encodeURIComponent(id) : ""),
        "GET",
        undefined,
        signal,
      )
    ).data,
  rebuild: async () =>
    (
      await request<Data<{ build_id: string; status: string }>>(
        "/admin/recommendations/rebuild",
        "POST",
      )
    ).data,
};
