import { useQuery } from "@tanstack/react-query";
import { catalog } from "./api";
export function useCategories() {
  return useQuery({
    queryKey: ["categories"],
    queryFn: ({ signal }) => catalog.categories(signal),
    staleTime: 1800000,
  });
}
export function usePlace(id: string) {
  return useQuery({
    queryKey: ["place", id],
    queryFn: ({ signal }) => catalog.get(id, signal),
    enabled: /^[a-f\d]{24}$/i.test(id),
  });
}
