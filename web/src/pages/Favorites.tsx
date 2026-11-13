import { useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "../app/session";
import { activity } from "../modules/activity/api";
import { PlaceCard } from "../modules/catalog/components/PlaceCard";
import { date } from "../shared/lib/format";
import {
  Empty,
  ErrorBox,
  Loading,
  PageHeading,
  Pagination,
} from "../shared/ui";
export default function Favorites() {
  const { user } = useSession();
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const raw = Number(params.get("offset") || 0);
  const offset = Number.isInteger(raw) && raw >= 0 && raw <= 10000 ? raw : 0;
  const query = useQuery({
    queryKey: ["user", user!.id, "favorites", offset],
    queryFn: ({ signal }) => activity.favorites(offset, signal),
    staleTime: 10000,
  });
  const remove = useMutation({
    mutationFn: (id: string) => activity.favorite(id, false),
    onSuccess: (s) => {
      client.setQueryData(["user", user!.id, "place-state", s.place_id], s);
      void client.invalidateQueries({ queryKey: ["user", user!.id] });
    },
  });
  useEffect(() => {
    if (query.data?.data.length === 0 && offset > 0 && !query.isFetching)
      setParams(
        { offset: String(Math.max(0, offset - 20)) },
        { replace: true },
      );
  }, [query.data, query.isFetching, offset, setParams]);
  return (
    <>
      <PageHeading eyebrow="ВАША КОЛЛЕКЦИЯ" title="Избранные места">
        Места, к которым хочется вернуться.
      </PageHeading>
      <ErrorBox
        error={query.error}
        retry={() => {
          void query.refetch();
        }}
      />
      <ErrorBox error={remove.error} />
      {query.isPending && <Loading />}
      <div className="favorites-grid">
        {query.data?.data.map((item) => (
          <PlaceCard key={item.place.id} place={item.place}>
            <div className="favorite-meta">
              <small>Добавлено {date(item.favorited_at)}</small>
              <span>Моя оценка: {item.rating ?? "—"}</span>
              <button
                className="text-button"
                disabled={remove.isPending}
                onClick={() => remove.mutate(item.place.id)}
              >
                Убрать из избранного
              </button>
            </div>
          </PlaceCard>
        ))}
      </div>
      {query.data?.data.length === 0 && (
        <Empty title="Ваша коллекция начинается здесь">
          <p>Откройте место и добавьте его в избранное.</p>
          <Link className="button primary" to="/explore">
            Найти места
          </Link>
        </Empty>
      )}
      {query.data && (
        <Pagination
          offset={offset}
          hasMore={query.data.pagination.has_more}
          onChange={(n) => setParams({ offset: String(n) })}
        />
      )}
    </>
  );
}
