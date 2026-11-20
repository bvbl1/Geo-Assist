import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "../app/session";
import { recommendations } from "../modules/recommendation/api";
import { date } from "../shared/lib/format";
import { ErrorBox, Loading, PageHeading } from "../shared/ui";
const labels = {
  building: "Выполняется",
  ready: "Готово",
  skipped: "Данные не изменились",
  failed: "Ошибка расчёта",
};
export default function AdminRecommendations() {
  const { user } = useSession();
  const client = useQueryClient();
  const [buildId, setBuildId] = useState<string>();
  const status = useQuery({
    queryKey: ["user", user!.id, "recommendation-status", buildId],
    queryFn: ({ signal }) => recommendations.status(buildId, signal),
    refetchInterval: (q) =>
      q.state.data?.requested_attempt?.status === "building" ||
      q.state.data?.last_attempt?.status === "building" ||
      (buildId && !q.state.data)
        ? 2000
        : 30000,
  });
  const rebuild = useMutation({
    mutationFn: recommendations.rebuild,
    onSuccess: (result) => {
      setBuildId(result.build_id);
      void client.invalidateQueries({
        queryKey: ["user", user!.id, "recommendation-status"],
      });
    },
  });
  const model = status.data?.active;
  const attempt = status.data?.requested_attempt ?? status.data?.last_attempt;
  const busy =
    rebuild.isPending || status.data?.last_attempt?.status === "building";
  return (
    <>
      <PageHeading eyebrow="АДМИНИСТРАТОР" title="Модель рекомендаций">
        Модель проверяется каждые пять минут. Пересчёт запускается, если
        изменились места или избранное.
      </PageHeading>
      <p>
        <Link to="/recommendations">← К рекомендациям</Link>
      </p>
      <ErrorBox
        error={status.error}
        retry={() => {
          void status.refetch();
        }}
      />
      <ErrorBox error={rebuild.error} />
      {status.isPending && <Loading />}
      {status.data && (
        <section className="panel recommendation-model">
          <h2>{model ? "Активная модель" : "Модель ещё не готова"}</h2>
          {model && (
            <dl className="recommendation-stats">
              <div>
                <dt>Версия</dt>
                <dd className="model-id">{model.id}</dd>
              </div>
              <div>
                <dt>Построена</dt>
                <dd>{date(model.built_at!)}</dd>
              </div>
              <div>
                <dt>Мест</dt>
                <dd>{model.item_count}</dd>
              </div>
              <div>
                <dt>Связей избранного</dt>
                <dd>{model.edge_count}</dd>
              </div>
              <div>
                <dt>Пар похожих мест</dt>
                <dd>{model.pair_count}</dd>
              </div>
              <div>
                <dt>Длительность расчёта</dt>
                <dd>{model.duration_ms} мс</dd>
              </div>
            </dl>
          )}
          <p>
            Последняя проверка:{" "}
            {status.data.last_checked_at
              ? date(status.data.last_checked_at)
              : "ещё не выполнялась"}
          </p>
          {attempt && (
            <div role="status" className="alert">
              <strong>Последний запрос: {labels[attempt.status]}</strong>
              {attempt.status === "failed" && (
                <p>
                  Не удалось обновить модель. Предыдущая версия продолжает
                  работать; подробности — в журнале сервера.
                </p>
              )}
            </div>
          )}
          <p>
            Проверка выполняется в фоне. Если данные не изменились, новая версия
            не создаётся.
          </p>
          <button
            className="button primary"
            disabled={busy}
            onClick={() => rebuild.mutate()}
          >
            {busy ? "Идёт проверка…" : "Проверить и пересчитать"}
          </button>
          <p className="muted">Ручной запуск доступен не чаще раза в минуту.</p>
        </section>
      )}
    </>
  );
}
