import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "../app/session";
import { activity } from "../modules/activity/api";
import { PlaceActions } from "../modules/activity/components/PlaceActions";
import { catalog } from "../modules/catalog/api";
import { ApiError } from "../shared/api/client";
import { date, eventNames } from "../shared/lib/format";
import {
  Empty,
  ErrorBox,
  Field,
  Loading,
  PageHeading,
  Pagination,
} from "../shared/ui";
import type { Place } from "../shared/api/types";
export default function History() {
  const { user } = useSession();
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const [searchTerm, setSearchTerm] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const offset = Number(params.get("offset") || 0);
  const type = params.get("type") || "";
  const placeId = params.get("place_id") || "";
  const valid =
    (!type || type in eventNames) &&
    (!placeId || /^[a-f\d]{24}$/i.test(placeId)) &&
    Number.isInteger(offset) &&
    offset >= 0 &&
    offset <= 10000;
  const request = new URLSearchParams({ limit: "20", offset: String(offset) });
  if (type) request.set("type", type);
  if (placeId) request.set("place_id", placeId);
  const history = useQuery({
    queryKey: ["user", user!.id, "history", request.toString()],
    queryFn: ({ signal }) => activity.history(request.toString(), signal),
    enabled: valid,
    staleTime: 10000,
  });
  const ids = [
    ...new Set([
      ...(history.data?.data.map((e) => e.place_id) ?? []),
      ...(placeId && valid ? [placeId] : []),
    ]),
  ];
  const names = useQuery({
    queryKey: ["place-names", ids],
    enabled: ids.length > 0,
    queryFn: async () => {
      const result: Record<
        string,
        { place?: Place; missing?: boolean; failed?: boolean }
      > = {};
      for (let i = 0; i < ids.length; i += 4)
        await Promise.all(
          ids.slice(i, i + 4).map(async (id) => {
            try {
              result[id] = {
                place: await client.fetchQuery({
                  queryKey: ["place", id],
                  queryFn: ({ signal }) => catalog.get(id, signal),
                  retry: false,
                }),
              };
            } catch (e) {
              result[id] =
                e instanceof ApiError && e.status === 404
                  ? { missing: true }
                  : { failed: true };
            }
          }),
        );
      return result;
    },
  });
  const choices = useQuery({
    queryKey: ["places", "history-picker", searchTerm],
    queryFn: ({ signal }) =>
      catalog.search(
        new URLSearchParams({ q: searchTerm, limit: "20" }).toString(),
        signal,
      ),
    enabled: searchTerm.length > 0,
  });
  function filter(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("offset");
    setParams(next);
  }
  return (
    <>
      <PageHeading eyebrow="ВАШИ ДЕЙСТВИЯ" title="История мест">
        Просмотры, выбор, избранное и оценки — в хронологическом порядке.
      </PageHeading>
      <div className="filter-panel history-filter">
        <Field label="Тип события">
          <select value={type} onChange={(e) => filter("type", e.target.value)}>
            <option value="">Все действия</option>
            {Object.entries(eventNames).map(([code, name]) => (
              <option key={code} value={code}>
                {name}
              </option>
            ))}
          </select>
        </Field>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setSearchTerm(search.trim());
          }}
        >
          <Field label="Найти место для фильтра">
            <input
              placeholder="Начало названия"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              maxLength={120}
            />
          </Field>
          <button className="text-button">Найти место</button>
        </form>
        {placeId && (
          <div className="alert">
            Фильтр: {names.data?.[placeId]?.place?.name ?? "Выбранное место"}{" "}
            <button
              className="text-button"
              onClick={() => filter("place_id", "")}
            >
              Убрать
            </button>
          </div>
        )}
        {searchTerm && (
          <div className="picker-results">
            {choices.isPending && <Loading />}
            <ErrorBox error={choices.error} />
            {choices.data?.data.map((p) => (
              <button
                key={p.id}
                className="text-button"
                onClick={() => {
                  filter("place_id", p.id);
                  setSearchTerm("");
                }}
              >
                {p.name}
              </button>
            ))}
            {choices.data?.data.length === 0 && <p>Места не найдены</p>}
          </div>
        )}
      </div>
      {!valid && (
        <div className="alert error">
          Некорректные фильтры.{" "}
          <button className="text-button" onClick={() => setParams({})}>
            Сбросить
          </button>
        </div>
      )}
      {history.isPending && valid && <Loading />}
      <ErrorBox
        error={history.error}
        retry={() => {
          void history.refetch();
        }}
      />
      <div className="timeline">
        {history.data?.data.map((e) => {
          const name = names.data?.[e.place_id];
          return (
            <article className="history-item" key={e.id}>
              <div className="timeline-dot" />
              <div>
                <div className="card-meta">
                  <strong>{eventNames[e.type]}</strong>
                  <time>{date(e.occurred_at)}</time>
                </div>
                {name?.place ? (
                  <Link className="place-title" to={"/places/" + e.place_id}>
                    {name.place.name}
                  </Link>
                ) : (
                  <p>
                    {name?.missing
                      ? "Место недоступно"
                      : name?.failed
                        ? "Не удалось загрузить название"
                        : "Загружаем место…"}
                  </p>
                )}
                {name?.failed && (
                  <button
                    className="text-button"
                    onClick={() => {
                      void names.refetch();
                    }}
                  >
                    Повторить загрузку названия
                  </button>
                )}
                {e.type === "rating_set" && (
                  <p>
                    Оценка: {e.previous_rating ?? "не было"} → {e.rating}
                  </p>
                )}
                {e.type === "rating_removed" && (
                  <p>Удалена оценка {e.previous_rating ?? ""}</p>
                )}
                {name?.missing && (
                  <>
                    <button
                      className="text-button"
                      onClick={() =>
                        setExpanded(expanded === e.id ? null : e.id)
                      }
                    >
                      Управление сохранённым состоянием
                    </button>
                    {expanded === e.id && (
                      <PlaceActions
                        id={e.place_id}
                        userId={user!.id}
                        inactive
                      />
                    )}
                  </>
                )}
              </div>
            </article>
          );
        })}
      </div>
      {history.data?.data.length === 0 && (
        <Empty title="История пока пуста">
          <p>Откройте место или сохраните его в избранное.</p>
        </Empty>
      )}
      {history.data && (
        <Pagination
          offset={offset}
          hasMore={history.data.pagination.has_more}
          onChange={(n) => {
            const next = new URLSearchParams(params);
            next.set("offset", String(n));
            setParams(next);
          }}
        />
      )}
    </>
  );
}
