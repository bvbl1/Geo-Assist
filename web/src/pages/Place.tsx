import { lazy, Suspense, useEffect, useState } from "react";
import { Link, useLocation, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, MapPin, Check, ArrowRight } from "lucide-react";
import { usePlace } from "../modules/catalog/queries";
import { PlaceActions } from "../modules/activity/components/PlaceActions";
import { recordOnce } from "../modules/activity/event-recorder";
import { useSession } from "../app/session";
import { categoryNames } from "../shared/lib/format";
import { Empty, ErrorBox, Loading } from "../shared/ui";
const MapCanvas = lazy(() => import("../shared/map/MapCanvas"));
export default function Place() {
  const location = useLocation();
  const back =
    typeof location.state?.from === "string" &&
    /^\/(explore|favorites|admin\/places)(\?|$)/.test(location.state.from)
      ? location.state.from
      : "/explore";
  const { id = "" } = useParams();
  const place = usePlace(id);
  const { user } = useSession();
  const client = useQueryClient();
  const [visit] = useState(() => crypto.randomUUID());
  const [viewError, setViewError] = useState<unknown>(null);
  const [selectKey, setSelectKey] = useState(() => crypto.randomUUID());
  const [selectError, setSelectError] = useState<unknown>(null);
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState(false);
  async function view() {
    if (!user || !place.data) return;
    try {
      await recordOnce(visit, id, "view", user.id);
      setViewError(null);
      void client.invalidateQueries({ queryKey: ["user", user.id, "history"] });
    } catch (e) {
      setViewError(e);
    }
  }
  useEffect(() => {
    if (user && place.data) {
      void view();
    }
  }, [id, user?.id, !!place.data]);
  async function select() {
    if (!user) return;
    setSelecting(true);
    setSelected(false);
    try {
      await recordOnce(selectKey, id, "select", user.id);
      setSelectError(null);
      setSelected(true);
      setSelectKey(crypto.randomUUID());
      void client.invalidateQueries({ queryKey: ["user", user.id, "history"] });
    } catch (e) {
      setSelectError(e);
    } finally {
      setSelecting(false);
    }
  }
  if (!/^[a-f\d]{24}$/i.test(id))
    return (
      <Empty title="Некорректный адрес места">
        <Link to="/explore">Вернуться к поиску</Link>
      </Empty>
    );
  if (place.isPending) return <Loading />;
  if (place.error)
    return (
      <>
        <Link to="/explore">← К поиску</Link>
        <ErrorBox
          error={place.error}
          retry={() => {
            void place.refetch();
          }}
        />
      </>
    );
  const p = place.data!;
  return (
    <>
      <Link className="back-link" to={back}>
        <ArrowLeft size={16} /> Все места
      </Link>
      <div className="detail-grid">
        <section className="panel detail">
          <div className="eyebrow">{categoryNames[p.category]}</div>
          <h1>{p.name}</h1>
          <p className="address">
            <MapPin size={17} />
            {p.address}
          </p>
          <div className="tags">
            {p.tags.map((t) => (
              <span key={t}>{t}</span>
            ))}
          </div>
          <p className="description">{p.description}</p>
          {user ? (
            <>
              <PlaceActions id={id} userId={user.id} />
              <button
                className="button primary"
                disabled={selecting}
                onClick={select}
              >
                {selecting ? "Сохраняем…" : "Выбрать это место"}
                <ArrowRight size={17} />
              </button>
              {selected && (
                <p className="success-text">
                  <Check size={16} /> Выбор сохранён в истории
                </p>
              )}
              <ErrorBox error={selectError} retry={select} />
              {viewError && (
                <div className="alert">
                  Просмотр не сохранён.{" "}
                  <button className="text-button" onClick={view}>
                    Повторить
                  </button>
                </div>
              )}
              <Link className="subtle-link" to={"/history?place_id=" + id}>
                История этого места →
              </Link>
            </>
          ) : (
            <div className="alert">
              <Link
                to={"/login?returnTo=" + encodeURIComponent("/places/" + id)}
              >
                Войдите
              </Link>
              , чтобы сохранять места и ставить оценки.
            </div>
          )}
          {user?.role === "admin" && (
            <Link className="subtle-link" to={`/admin/places/${id}/edit`}>
              Редактировать место →
            </Link>
          )}
          <details className="coordinates">
            <summary>Координаты</summary>
            <p>
              Широта: {p.location.latitude} · Долгота: {p.location.longitude}
            </p>
          </details>
        </section>
        <Suspense fallback={<Loading />}>
          <MapCanvas items={[p]} center={p.location} />
        </Suspense>
      </div>
    </>
  );
}
