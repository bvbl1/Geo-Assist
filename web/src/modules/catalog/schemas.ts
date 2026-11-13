import { z } from "zod";
import { code, text } from "../../shared/lib/validation";
export { categories } from "../../shared/lib/validation";
export const tags = (s: string) =>
  s
    .split(",")
    .map((t) => t.trim().toLowerCase())
    .filter(Boolean);
export function validTags(s: string) {
  const values = tags(s);
  return (
    values.length <= 20 &&
    new Set(values).size === values.length &&
    values.every((t) => /^[a-z0-9][a-z0-9_-]{0,31}$/.test(t))
  );
}
export const coordinate = (min: number, max: number) =>
  z
    .string()
    .refine(
      (v) =>
        v.trim() !== "" &&
        Number.isFinite(Number(v)) &&
        Number(v) >= min &&
        Number(v) <= max,
      `Число от ${min} до ${max}`,
    );
export const placeSchema = z.object({
  name: text(120),
  description: text(2000),
  address: text(300),
  category: code,
  tags: z
    .string()
    .refine(validTags, "До 20 уникальных тегов: латиница, цифры, _ и -"),
  longitude: coordinate(-180, 180),
  latitude: coordinate(-90, 90),
});
export type PlaceForm = z.infer<typeof placeSchema>;
