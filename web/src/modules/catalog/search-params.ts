import { categories, validTags, tags } from "./schemas";
export type SearchForm = {
  q: string;
  category: string;
  tags: string;
  nearby: boolean;
  longitude: string;
  latitude: string;
  radius: string;
};
export const initialSearch: SearchForm = {
  q: "",
  category: "",
  tags: "",
  nearby: false,
  longitude: "71.4304",
  latitude: "51.1282",
  radius: "5000",
};
export function buildSearch(v: SearchForm, offset = 0): string {
  const p = new URLSearchParams();
  if (Array.from(v.q.trim()).length > 120)
    throw new Error("Название: не больше 120 символов");
  if (
    v.category &&
    !categories.includes(v.category as (typeof categories)[number])
  )
    throw new Error("Неизвестная категория");
  if (!validTags(v.tags))
    throw new Error(
      "Проверьте теги: уникальные, до 20, латиница, цифры, _ и -",
    );
  if (v.q.trim()) p.set("q", v.q.trim());
  if (v.category) p.set("category", v.category);
  tags(v.tags)
    .sort()
    .forEach((t) => p.append("tag", t));
  if (v.nearby) {
    for (const [key, raw, min, max] of [
      ["longitude", v.longitude, -180, 180],
      ["latitude", v.latitude, -90, 90],
      ["radius_m", v.radius, 1, 50000],
    ] as const) {
      const n = Number(raw);
      if (
        !raw.trim() ||
        !Number.isFinite(n) ||
        n < min ||
        n > max ||
        (key === "radius_m" && !Number.isInteger(n))
      )
        throw new Error(`${key}: число от ${min} до ${max}`);
      p.set(key, String(n));
    }
  }
  if (!Number.isInteger(offset) || offset < 0 || offset > 10000)
    throw new Error("Некорректная страница");
  p.set("limit", "20");
  p.set("offset", String(offset));
  return p.toString();
}
export function readSearch(p: URLSearchParams) {
  const geo = ["longitude", "latitude", "radius_m"].filter((k) => p.has(k));
  const value: SearchForm = {
    ...initialSearch,
    q: p.get("q") ?? "",
    category: p.get("category") ?? "",
    tags: p.getAll("tag").join(", "),
    nearby: geo.length > 0,
    longitude: p.get("longitude") ?? initialSearch.longitude,
    latitude: p.get("latitude") ?? initialSearch.latitude,
    radius: p.get("radius_m") ?? initialSearch.radius,
  };
  let error = "";
  let query = "";
  const offset = Number(p.get("offset") ?? "0");
  try {
    const allowed = [
      "q",
      "category",
      "tag",
      "longitude",
      "latitude",
      "radius_m",
      "limit",
      "offset",
    ];
    for (const key of p.keys())
      if (!allowed.includes(key) || (key !== "tag" && p.getAll(key).length > 1))
        throw new Error("Некорректные параметры ссылки");
    if (geo.length && geo.length !== 3)
      throw new Error("Укажите центр и радиус полностью");
    if (p.has("limit") && p.get("limit") !== "20")
      throw new Error("Сайт использует страницы по 20 мест. Сбросьте фильтры.");
    query = buildSearch(value, offset);
  } catch (e) {
    error = (e as Error).message;
  }
  return { value, offset, query, error };
}
