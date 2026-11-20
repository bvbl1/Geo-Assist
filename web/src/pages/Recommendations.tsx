import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { useSession } from "../app/session";
import { recommendations } from "../modules/recommendation/api";
import { PlaceCard } from "../modules/catalog/components/PlaceCard";
import { useCategories } from "../modules/catalog/queries";
import { ApiError } from "../shared/api/client";
import { date } from "../shared/lib/format";
import { Empty, ErrorBox, Field, Loading, PageHeading } from "../shared/ui";

export default function Recommendations() {
  const { user } = useSession();
  const categories = useCategories();
  const [nearby, setNearby] = useState(false);
  const [params, setParams] = useState("limit=10");
  const query = useQuery({
    queryKey: ["user", user!.id, "recommendations", params],
    queryFn: ({ signal }) => recommendations.get(params, signal),
    staleTime: 30000,
    refetchInterval: (q) =>
      q.state.error instanceof ApiError &&
      q.state.error.code === "MODEL_NOT_READY"
        ? 5000
        : 60000,
  });
  function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const p = new URLSearchParams({ limit: "10" });
    const category = String(form.get("category") || "");
    if (category) p.set("category", category);
    const tags = [
      ...new Set(
        String(form.get("tags") || "")
          .split(",")
          .map((s) => s.trim().toLowerCase())
          .filter(Boolean),
      ),
    ].sort();
    for (const tag of tags) p.append("tag", tag);
    if (nearby)
      for (const key of ["longitude", "latitude", "radius_m"])
        p.set(key, String(form.get(key)));
    setParams(p.toString());
  }
  const preparing =
    query.error instanceof ApiError && query.error.code === "MODEL_NOT_READY";
  return (
    <>
      <div className="heading-actions">
        <PageHeading eyebrow="ОТКРЫВАЙТЕ АСТАНУ" title="Для вас">
          Места, которые могут понравиться, на основе вашего избранного.
        </PageHeading>
        {user?.role === "admin" && (
          <Link className="button secondary" to="/admin/recommendations">
            Модель рекомендаций
          </Link>
        )}
      </div>
      <form className="panel recommendation-filters" onSubmit={apply}>
        <div className="form-grid">
          <Field label="Категория">
            <select name="category" defaultValue="">
              <option value="">Все категории</option>
              {categories.data?.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Теги через запятую">
            <input name="tags" placeholder="wifi, quiet" maxLength={659} />
          </Field>
        </div>
        <label className="recommendation-checkbox">
          <input
            type="checkbox"
            checked={nearby}
            onChange={(e) => setNearby(e.target.checked)}
          />{" "}
          Только рядом с точкой
        </label>
        {nearby && (
          <div className="form-grid">
            <Field label="Долгота">
              <input
                name="longitude"
                type="number"
                step="any"
                min={-180}
                max={180}
                defaultValue="71.4304"
                required
              />
            </Field>
            <Field label="Широта">
              <input
                name="latitude"
                type="number"
                step="any"
                min={-90}
                max={90}
                defaultValue="51.1282"
                required
              />
            </Field>
            <Field label="Радиус, метры">
              <input
                name="radius_m"
                type="number"
                min={1}
                max={50000}
                defaultValue={5000}
                required
              />
            </Field>
          </div>
        )}
        <button className="button primary" type="submit">
          Подобрать места
        </button>
      </form>
      <ErrorBox error={categories.error} />
      {preparing ? (
        <Empty title="Готовим рекомендации">
          <p>Первый расчёт ещё идёт. Страница обновится автоматически.</p>
        </Empty>
      ) : (
        <ErrorBox
          error={query.error}
          retry={() => {
            void query.refetch();
          }}
        />
      )}
      {query.isPending && <Loading />}
      {query.data && (
        <>
          <div className="recommendation-summary" role="status">
            <Sparkles size={22} />
            <div>
              <strong>
                {query.data.meta.fallback_used
                  ? "Популярное среди пользователей"
                  : "Похожие на ваше избранное"}
              </strong>
              <p>
                {query.data.meta.fallback_used
                  ? "Пока недостаточно подходящих совпадений. Начните сохранять любимые места — подборка станет персональной."
                  : "Эти места сохраняют пользователи, которым нравятся те же места, что и вам."}
              </p>
              <small>
                Модель от {date(query.data.meta.model_built_at)}
                {query.data.meta.refresh_in_progress ? " · Обновляется" : ""}
              </small>
            </div>
          </div>
          {query.data.data.length === 0 ? (
            <Empty title="Подходящих рекомендаций пока нет">
              <p>
                Попробуйте другую категорию, уберите теги или увеличьте радиус.
              </p>
              <Link to="/explore">Посмотреть весь каталог</Link>
            </Empty>
          ) : (
            <div className="favorites-grid">
              {query.data.data.map((item) => (
                <PlaceCard key={item.place.id} place={item.place}>
                  <div className="favorite-meta">
                    <span>
                      {item.reason === "similar_to_favorites"
                        ? "На основе избранного"
                        : item.reason === "popular_nearby"
                          ? "Популярное рядом"
                          : "Популярное место"}
                    </span>
                  </div>
                </PlaceCard>
              ))}
            </div>
          )}
        </>
      )}
    </>
  );
}
