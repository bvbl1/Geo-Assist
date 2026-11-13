import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Heart, Star } from "lucide-react";
import { activity } from "../api";
import { ErrorBox, Loading } from "../../../shared/ui";
export function PlaceActions({
  id,
  userId,
  inactive = false,
}: {
  id: string;
  userId: string;
  inactive?: boolean;
}) {
  const client = useQueryClient();
  const key = ["user", userId, "place-state", id];
  const state = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => activity.state(id, signal),
    staleTime: 10000,
  });
  const mutation = useMutation({
    mutationFn: (v: { favorite?: boolean; rating?: number | null }) =>
      v.favorite !== undefined
        ? activity.favorite(id, v.favorite)
        : activity.rate(id, v.rating ?? null),
    onSuccess: (data) => {
      client.setQueryData(key, data);
      void client.invalidateQueries({
        queryKey: ["user", userId, "favorites"],
      });
      void client.invalidateQueries({ queryKey: ["user", userId, "history"] });
    },
  });
  if (state.isPending) return <Loading />;
  if (state.error)
    return (
      <ErrorBox
        error={state.error}
        retry={() => {
          void state.refetch();
        }}
      />
    );
  const data = state.data!;
  return (
    <div className="place-actions">
      <button
        className={"button " + (data.favorite ? "favorite-on" : "secondary")}
        disabled={mutation.isPending || (inactive && !data.favorite)}
        onClick={() => mutation.mutate({ favorite: !data.favorite })}
      >
        <Heart size={18} fill={data.favorite ? "currentColor" : "none"} />
        {data.favorite ? "Убрать из избранного" : "В избранное"}
      </button>
      <div>
        <span className="label">Моя оценка</span>
        <div className="rating" role="group" aria-label="Моя оценка">
          {[1, 2, 3, 4, 5].map((n) => (
            <button
              key={n}
              aria-label={`Оценить на ${n}`}
              aria-pressed={data.rating === n}
              disabled={mutation.isPending || inactive}
              onClick={() => mutation.mutate({ rating: n })}
            >
              <Star
                size={25}
                fill={n <= (data.rating ?? 0) ? "#c98a27" : "none"}
                color="#c98a27"
              />
            </button>
          ))}
          <span>{data.rating ?? "Нет оценки"}</span>
        </div>
        {data.rating != null && (
          <button
            className="text-button"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate({ rating: null })}
          >
            Убрать оценку
          </button>
        )}
      </div>
      <ErrorBox error={mutation.error} />
    </div>
  );
}
