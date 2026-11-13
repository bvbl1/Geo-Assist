export const categoryNames: Record<string, string> = {
  cafe: "Кафе",
  park: "Парк",
  library: "Библиотека",
  coworking: "Коворкинг",
  attraction: "Достопримечательность",
};
export const eventNames: Record<string, string> = {
  view: "Просмотр места",
  select: "Выбор места",
  favorite_added: "Добавлено в избранное",
  favorite_removed: "Убрано из избранного",
  rating_set: "Установлена оценка",
  rating_removed: "Оценка удалена",
};
export function distance(n?: number | null) {
  return n == null
    ? ""
    : n < 1000
      ? `${Math.round(n)} м`
      : `${(n / 1000).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} км`;
}
export function date(s?: string | null) {
  return s
    ? new Date(s).toLocaleString("ru-RU", {
        dateStyle: "medium",
        timeStyle: "short",
      })
    : "—";
}
export function safeReturn(value: string | null) {
  return value &&
    value.startsWith("/") &&
    !value.startsWith("//") &&
    !value.includes("\\") &&
    !value.startsWith("/login") &&
    !value.startsWith("/register")
    ? value
    : "/explore";
}
