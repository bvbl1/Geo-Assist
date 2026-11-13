export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details: { field: string; reason: string }[] = [],
    public requestId = "",
    public retryAfter = 0,
  ) {
    super(message);
  }
}
let generation = 0;
const active = new Set<AbortController>();
export function resetRequests() {
  generation++;
  for (const c of active) c.abort();
  active.clear();
}
export function sessionGeneration() {
  return generation;
}
export async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const epoch = generation;
  const controller = new AbortController();
  active.add(controller);
  const signals = [
    controller.signal,
    AbortSignal.timeout(15000),
    ...(signal ? [signal] : []),
  ];
  try {
    const response = await fetch("/api/v1" + path, {
      method,
      credentials: "same-origin",
      signal: AbortSignal.any(signals),
      headers: {
        ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
        ...(method !== "GET" ? { "X-GeoAssist-CSRF": "1" } : {}),
      },
      ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
    });
    if (epoch !== generation)
      throw new DOMException("Session changed", "AbortError");
    if (response.status === 204) return undefined as T;
    const json = await response.json().catch(() => {
      throw new ApiError(
        response.status,
        "INVALID_RESPONSE",
        "Сервер вернул неожиданный ответ",
      );
    });
    if (epoch !== generation)
      throw new DOMException("Session changed", "AbortError");
    if (!response.ok) {
      const e = json.error ?? {};
      const error = new ApiError(
        response.status,
        e.code ?? "UNKNOWN",
        e.message ?? "Ошибка запроса",
        e.details ?? [],
        e.request_id ?? response.headers.get("X-Request-ID") ?? "",
        Number(response.headers.get("Retry-After")) || 0,
      );
      if (error.code === "UNAUTHENTICATED" && path !== "/me")
        window.dispatchEvent(new CustomEvent("session-expired"));
      throw error;
    }
    if (!json || typeof json !== "object" || !("data" in json))
      throw new ApiError(502, "INVALID_RESPONSE", "Некорректный ответ API");
    return json as T;
  } finally {
    active.delete(controller);
  }
}
export function message(error: unknown): string {
  if (error instanceof ApiError) {
    const messages: Record<string, string> = {
      INVALID_CREDENTIALS: "Неверный email или пароль.",
      UNAUTHENTICATED: "Сессия истекла. Войдите снова.",
      EMAIL_ALREADY_EXISTS: "Этот email уже зарегистрирован.",
      FORBIDDEN: "Недостаточно прав для этого действия.",
      CSRF_REJECTED:
        "Не удалось отправить запрос. Проверьте совпадение адреса сайта и APP_ORIGIN.",
      NOT_FOUND: "Место недоступно или удалено.",
      VERSION_CONFLICT:
        "Место изменилось в другой вкладке. Загрузите актуальную версию.",
      RATE_LIMITED: `Слишком много запросов. Повторите через ${error.retryAfter || 60} сек.`,
      IDEMPOTENCY_CONFLICT:
        "Не удалось повторить действие: ключ уже использован.",
      VALIDATION_ERROR: "Проверьте значения полей.",
      BAD_REQUEST: "Неверный формат запроса.",
      SERVICE_UNAVAILABLE: "Сервис временно недоступен. Попробуйте ещё раз.",
    };
    return (
      messages[error.code] ?? "Не удалось выполнить запрос. Попробуйте ещё раз."
    );
  }
  return "Нет ответа от сервера. Проверьте соединение и повторите.";
}
