import { z } from "zod";
export const categories = [
  "cafe",
  "park",
  "library",
  "coworking",
  "attraction",
] as const;
export const code = z.enum(categories, { error: "Выберите категорию" });
export const text = (max: number) =>
  z
    .string()
    .refine(
      (s) =>
        Array.from(s.trim()).length >= 1 && Array.from(s.trim()).length <= max,
      `От 1 до ${max} символов`,
    );
export function mapCenter(longitude: string, latitude: string) {
  const lng = Number(longitude),
    lat = Number(latitude);
  return {
    longitude:
      longitude.trim() && Number.isFinite(lng) && Math.abs(lng) <= 180
        ? lng
        : 71.4304,
    latitude:
      latitude.trim() && Number.isFinite(lat) && Math.abs(lat) <= 90
        ? lat
        : 51.1282,
  };
}
