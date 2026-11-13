import { mapCenter } from "../shared/lib/validation";
import { lazy, Suspense, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Compass, LocateFixed, Search, SlidersHorizontal } from "lucide-react";
import { catalog } from "../modules/catalog/api";
import { useCategories } from "../modules/catalog/queries";
import {
  readSearch,
  buildSearch,
  initialSearch,
  type SearchForm,
} from "../modules/catalog/search-params";
import { PlaceCard } from "../modules/catalog/components/PlaceCard";
import {
  Empty,
  ErrorBox,
  Field,
  Loading,
  PageHeading,
  Pagination,
} from "../shared/ui";
const MapCanvas = lazy(() => import("../shared/map/MapCanvas"));
export default function Explore() {
  const [params, setParams] = useSearchParams();
  const parsed = readSearch(params);
  const places = useQuery({
    queryKey: ["places", parsed.query],
    queryFn: ({ signal }) => catalog.search(parsed.query, signal),
    enabled: !parsed.error,
  });
  const [mode, setMode] = useState("both");
  return (
    <>
      <PageHeading
        eyebrow="АСТАНА · ВАШ ГОРОД РЯДОМ"
        title="Найдите своё место"
      >
        Тихая кофейня, парк для прогулки или пространство для работы.
      </PageHeading>
      <div className="demo-note">
        <Compass size={16} /> Демонстрационные места Астаны · названия и адреса
        вымышлены
      </div>
      <Filters
        key={params.toString()}
        initial={parsed.value}
        initialError={parsed.error}
        onApply={(v) => setParams(buildSearch(v))}
        onReset={() => setParams({})}
      />
      <div className="results-toolbar">
        <div>
          <strong>{places.data?.data.length ?? 0} мест</strong>
          <span> на этой странице</span>
        </div>
        <div className="segmented">
          <button
            className={mode === "list" ? "active" : ""}
            onClick={() => setMode("list")}
          >
            Список
          </button>
          <button
            className={mode !== "list" ? "active" : ""}
            onClick={() => setMode("both")}
          >
            С картой
          </button>
        </div>
      </div>
      {places.isPending && !parsed.error && <Loading />}
      <ErrorBox
        error={places.error}
        retry={() => {
          void places.refetch();
        }}
      />
      {!parsed.error && (
        <div className={"explore-grid " + (mode === "list" ? "list-only" : "")}>
          <section aria-label="Результаты поиска" className="results-list">
            {places.data?.data.map((p) => (
              <PlaceCard key={p.id} place={p} />
            ))}
            {places.data?.data.length === 0 && (
              <Empty title="Места не найдены">
                <p>Попробуйте другую категорию или увеличьте радиус.</p>
              </Empty>
            )}
            {places.data && (
              <Pagination
                offset={parsed.offset}
                hasMore={places.data.pagination.has_more}
                onChange={(offset) =>
                  setParams(buildSearch(parsed.value, offset))
                }
              />
            )}
          </section>
          {mode !== "list" && (
            <aside className="map-panel">
              <Suspense fallback={<Loading />}>
                <MapCanvas
                  items={places.data?.data ?? []}
                  center={
                    parsed.value.nearby
                      ? {
                          longitude: Number(parsed.value.longitude),
                          latitude: Number(parsed.value.latitude),
                        }
                      : undefined
                  }
                  radius={
                    parsed.value.nearby
                      ? Number(parsed.value.radius)
                      : undefined
                  }
                />
              </Suspense>
              <p>На карте — места с этой страницы. Расстояние по прямой.</p>
            </aside>
          )}
        </div>
      )}
    </>
  );
}
function Filters({
  initial,
  initialError,
  onApply,
  onReset,
}: {
  initial: SearchForm;
  initialError: string;
  onApply: (v: SearchForm) => void;
  onReset: () => void;
}) {
  const [value, setValue] = useState(initial);
  const [error, setError] = useState(initialError);
  const [locating, setLocating] = useState(false);
  const [pick, setPick] = useState(false);
  const category = useCategories();
  const update = (key: keyof SearchForm, v: string | boolean) =>
    setValue((old) => ({ ...old, [key]: v }));
  function locate() {
    if (!navigator.geolocation) {
      setError("Браузер не поддерживает геолокацию");
      return;
    }
    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      (p) => {
        setValue((v) => ({
          ...v,
          nearby: true,
          longitude: String(p.coords.longitude),
          latitude: String(p.coords.latitude),
        }));
        setLocating(false);
        setError("");
      },
      () => {
        setLocating(false);
        setError(
          "Местоположение недоступно. Выберите точку вручную или центр Астаны.",
        );
      },
      { timeout: 10000, maximumAge: 60000 },
    );
  }
  return (
    <form
      className="filter-panel"
      onSubmit={(e) => {
        e.preventDefault();
        try {
          buildSearch(value);
          setError("");
          onApply(value);
        } catch (err) {
          setError((err as Error).message);
        }
      }}
    >
      <div className="filter-row">
        <Field label="Начало названия">
          <div className="input-icon">
            <Search size={17} />
            <input
              placeholder="Например, Астана"
              value={value.q}
              onChange={(e) => update("q", e.target.value)}
            />
          </div>
        </Field>
        <Field label="Категория">
          <select
            value={value.category}
            onChange={(e) => update("category", e.target.value)}
          >
            <option value="">Все категории</option>
            {category.data?.map((c) => (
              <option key={c.code} value={c.code}>
                {c.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Теги через запятую">
          <input
            placeholder="wifi, quiet"
            value={value.tags}
            onChange={(e) => update("tags", e.target.value)}
          />
        </Field>
        <button className="button primary" type="submit">
          <Search size={17} />
          Найти
        </button>
      </div>
      <div className="filter-footer">
        <label className="check">
          <input
            type="checkbox"
            checked={value.nearby}
            onChange={(e) => update("nearby", e.target.checked)}
          />
          <SlidersHorizontal size={16} /> Искать рядом
        </label>
        <button
          className="text-button"
          type="button"
          onClick={locate}
          disabled={locating}
        >
          <LocateFixed size={16} />
          {locating ? "Определяем…" : "Моё местоположение"}
        </button>
        <button className="text-button muted" type="button" onClick={onReset}>
          Сбросить
        </button>
      </div>
      {value.nearby && (
        <>
          <div className="form-grid">
            <Field label="Долгота">
              <input
                inputMode="decimal"
                value={value.longitude}
                onChange={(e) => update("longitude", e.target.value)}
              />
            </Field>
            <Field label="Широта">
              <input
                inputMode="decimal"
                value={value.latitude}
                onChange={(e) => update("latitude", e.target.value)}
              />
            </Field>
            <Field label="Радиус, метры">
              <input
                inputMode="numeric"
                value={value.radius}
                onChange={(e) => update("radius", e.target.value)}
              />
            </Field>
          </div>
          <div className="actions">
            <button
              className="text-button"
              type="button"
              onClick={() => setPick(!pick)}
            >
              Выбрать точку на карте
            </button>
            <button
              className="text-button"
              type="button"
              onClick={() =>
                setValue((v) => ({
                  ...v,
                  longitude: initialSearch.longitude,
                  latitude: initialSearch.latitude,
                }))
              }
            >
              Центр Астаны
            </button>
          </div>
          {pick && (
            <Suspense fallback={<Loading />}>
              <MapCanvas
                center={mapCenter(value.longitude, value.latitude)}
                onPick={(p) =>
                  setValue((v) => ({
                    ...v,
                    longitude: p.longitude.toFixed(6),
                    latitude: p.latitude.toFixed(6),
                  }))
                }
              />
            </Suspense>
          )}
        </>
      )}
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
      <ErrorBox
        error={category.error}
        retry={() => {
          void category.refetch();
        }}
      />
    </form>
  );
}
