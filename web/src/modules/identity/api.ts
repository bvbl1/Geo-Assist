import { request } from "../../shared/api/client";
import type { User, Data } from "../../shared/api/types";
export const identity = {
  me: async (signal?: AbortSignal) =>
    (await request<Data<User>>("/me", "GET", undefined, signal)).data,
  login: async (value: { email: string; password: string }) =>
    (await request<Data<User>>("/auth/login", "POST", value)).data,
  register: async (value: {
    email: string;
    password: string;
    display_name: string;
    preferred_categories: string[];
  }) => (await request<Data<User>>("/auth/register", "POST", value)).data,
  logout: () => request<void>("/auth/logout", "POST"),
  update: async (value: {
    display_name?: string;
    preferred_categories?: string[];
  }) => (await request<Data<User>>("/me", "PATCH", value)).data,
};
