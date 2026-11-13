import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "../shared/api/client";
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      retry: (count, error) =>
        count < 1 && !(error instanceof ApiError && error.status < 500),
      refetchOnWindowFocus: true,
    },
    mutations: { retry: false },
  },
});
