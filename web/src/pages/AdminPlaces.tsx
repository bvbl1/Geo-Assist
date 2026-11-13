import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Pencil, Trash2 } from "lucide-react";
import { catalog } from "../modules/catalog/api";
import { PlaceCard } from "../modules/catalog/components/PlaceCard";
import type { Place } from "../shared/api/types";
import {
  Confirm,
  Empty,
  ErrorBox,
  Loading,
  PageHeading,
  Pagination,
} from "../shared/ui";
export default function AdminPlaces() {
  const [params, setParams] = useSearchParams();
  const offset = Math.max(
    0,
    Math.min(10000, Number(params.get("offset")) || 0),
  );
  const q = params.get("q") || "";
  const [draft, setDraft] = useState(q);
  const [target, setTarget] = useState<Place | null>(null);
  const client = useQueryClient();
  const p = new URLSearchParams({ limit: "20", offset: String(offset) });
  if (q) p.set("q", q);
  const places = useQuery({
    queryKey: ["places", p.toString()],
    queryFn: ({ signal }) => catalog.search(p.toString(), signal),
  });
  const remove = useMutation({
    mutationFn: (place: Place) => catalog.delete(place.id, place.version),
    onSuccess: () => {
      client.removeQueries({ queryKey: ["place", target!.id] });
      void client.invalidateQueries();
      setTarget(null);
    },
  });
  async function refreshTarget() {
    if (!target) return;
    try {
      setTarget(await catalog.get(target.id));
      remove.reset();
    } catch {
      setTarget(null);
      void client.invalidateQueries();
    }
  }
  return (
    <>
      <div className="heading-actions">
        <PageHeading eyebrow="АДМИНИСТРАТОР" title="Управление местами">
          Каталог Астаны: создание, редактирование и удаление.
        </PageHeading>
        <Link className="button primary" to="/admin/places/new">
          <Plus size={18} />
          Добавить место
        </Link>
      </div>
      <form
        className="admin-search"
        onSubmit={(e) => {
          e.preventDefault();
          setParams(draft.trim() ? { q: draft.trim() } : {});
        }}
      >
        <input
          aria-label="Поиск мест"
          placeholder="Начало названия"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          maxLength={120}
        />
        <button className="button secondary">Найти</button>
      </form>
      {places.isPending && <Loading />}
      <ErrorBox
        error={places.error}
        retry={() => {
          void places.refetch();
        }}
      />
      <div className="favorites-grid">
        {places.data?.data.map((place) => (
          <PlaceCard key={place.id} place={place}>
            <div className="actions">
              <Link
                className="text-button"
                to={`/admin/places/${place.id}/edit`}
              >
                <Pencil size={15} />
                Редактировать
              </Link>
              <button
                className="text-button danger-text"
                onClick={() => {
                  remove.reset();
                  setTarget(place);
                }}
              >
                <Trash2 size={15} />
                Удалить
              </button>
            </div>
          </PlaceCard>
        ))}
      </div>
      {places.data?.data.length === 0 && <Empty title="Места не найдены" />}
      {places.data && (
        <Pagination
          offset={offset}
          hasMore={places.data.pagination.has_more}
          onChange={(n) =>
            setParams({ ...(q ? { q } : {}), offset: String(n) })
          }
        />
      )}
      <Confirm
        open={!!target}
        title="Удалить место?"
        busy={remove.isPending}
        onClose={() => setTarget(null)}
        onConfirm={() => target && remove.mutate(target)}
      >
        <p>
          «{target?.name}» исчезнет из каталога и избранного. История действий
          сохранится.
        </p>
        <ErrorBox error={remove.error} />
        {remove.error && (
          <button className="text-button" onClick={refreshTarget}>
            Перечитать актуальные данные перед подтверждением
          </button>
        )}
      </Confirm>
    </>
  );
}
