import { mapCenter } from "../shared/lib/validation";
import { lazy, Suspense, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCategories, usePlace } from "../modules/catalog/queries";
import { placeSchema, tags, type PlaceForm } from "../modules/catalog/schemas";
import { catalog } from "../modules/catalog/api";
import type { Place, PlaceCreate, PlacePatch } from "../shared/api/types";
import { ApiError } from "../shared/api/client";
import { ErrorBox, Field, Loading, PageHeading } from "../shared/ui";
const MapCanvas = lazy(() => import("../shared/map/MapCanvas"));
const toForm = (p?: Place): PlaceForm =>
  p
    ? {
        name: p.name,
        description: p.description,
        address: p.address,
        category: p.category,
        tags: p.tags.join(", "),
        longitude: String(p.location.longitude),
        latitude: String(p.location.latitude),
      }
    : {
        name: "",
        description: "",
        address: "",
        category: "cafe",
        tags: "",
        longitude: "71.4304",
        latitude: "51.1282",
      };
export default function PlaceEditor() {
  const { id = "" } = useParams();
  const place = usePlace(id);
  if (id && !/^[a-f\d]{24}$/i.test(id)) return <p>Некорректный адрес места.</p>;
  if (id && place.isPending) return <Loading />;
  if (id && place.error)
    return (
      <ErrorBox
        error={place.error}
        retry={() => {
          void place.refetch();
        }}
      />
    );
  return <Editor key={id || "new"} initial={place.data} />;
}
function Editor({ initial }: { initial?: Place }) {
  const navigate = useNavigate();
  const client = useQueryClient();
  const categories = useCategories();
  const [baseline, setBaseline] = useState(initial);
  const [fresh, setFresh] = useState<Place | null>(null);
  const [conflictError, setConflictError] = useState<unknown>(null);
  const form = useForm<PlaceForm>({
    resolver: zodResolver(placeSchema),
    defaultValues: toForm(initial),
  });
  const mutation = useMutation({
    mutationFn: async (v: PlaceForm) => {
      const body: PlaceCreate = {
        name: v.name.trim(),
        description: v.description.trim(),
        category: v.category,
        address: v.address.trim(),
        tags: tags(v.tags),
        location: {
          longitude: Number(v.longitude),
          latitude: Number(v.latitude),
        },
      };
      if (!baseline) return catalog.create(body);
      const patch: Record<string, unknown> = {
        expected_version: baseline.version,
      };
      for (const [key, value] of Object.entries(body))
        if (
          JSON.stringify(value) !== JSON.stringify(baseline[key as keyof Place])
        )
          patch[key] = value;
      if (Object.keys(patch).length === 1) return baseline;
      return catalog.update(baseline.id, patch as PlacePatch);
    },
    onSuccess: (data) => {
      client.setQueryData(["place", data.id], data);
      void client.invalidateQueries({ queryKey: ["places"] });
      void client.invalidateQueries({ queryKey: ["user"] });
      void client.invalidateQueries({ queryKey: ["place-names"] });
      navigate("/places/" + data.id);
    },
    onError: async (e) => {
      if (e instanceof ApiError && e.code === "VERSION_CONFLICT" && baseline) {
        try {
          setFresh(await catalog.get(baseline.id));
          setConflictError(null);
        } catch (err) {
          setConflictError(err);
        }
      }
      if (e instanceof ApiError)
        for (const d of e.details) {
          const field = d.field.replace("location.", "") as keyof PlaceForm;
          if (field in form.getValues())
            form.setError(field, {
              message: "Проверьте значение: " + d.reason,
            });
        }
    },
  });
  const center = mapCenter(form.watch("longitude"), form.watch("latitude"));
  return (
    <>
      <Link className="back-link" to="/admin/places">
        ← К управлению местами
      </Link>
      <PageHeading
        eyebrow="КАТАЛОГ"
        title={initial ? "Редактировать место" : "Новое место"}
      >
        Заполните информацию и отметьте точку на карте.
      </PageHeading>
      <form
        className="editor-grid"
        onSubmit={form.handleSubmit((v) => mutation.mutate(v))}
        noValidate
      >
        <section className="panel">
          <Field label="Название" error={form.formState.errors.name?.message}>
            <input {...form.register("name")} />
          </Field>
          <Field
            label="Описание"
            error={form.formState.errors.description?.message}
          >
            <textarea rows={4} {...form.register("description")} />
          </Field>
          <Field
            label="Категория"
            error={form.formState.errors.category?.message}
          >
            <select {...form.register("category")}>
              {categories.data?.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.name}
                </option>
              ))}
            </select>
          </Field>
          <Field
            label="Теги через запятую"
            hint="Например: wifi, quiet, outdoor"
            error={form.formState.errors.tags?.message}
          >
            <input {...form.register("tags")} />
          </Field>
          <Field label="Адрес" error={form.formState.errors.address?.message}>
            <input {...form.register("address")} />
          </Field>
          <ErrorBox error={categories.error} />
          <ErrorBox error={mutation.error} />
          <ErrorBox error={conflictError} />
          {mutation.error && !initial && (
            <p>
              Если ответ был потерян, сначала проверьте каталог — место могло
              сохраниться.
            </p>
          )}
          {fresh && (
            <div className="alert">
              <strong>Есть новая версия: {fresh.version}</strong>
              <p>
                {fresh.name} · {fresh.address}
              </p>
              <p>
                Ваш черновик сохранён в форме. Выберите, какие данные
                использовать.
              </p>
              <details>
                <summary>Актуальное описание</summary>
                <p>{fresh.description}</p>
                <p>{fresh.tags.join(", ")}</p>
              </details>
              <div className="actions">
                <button
                  type="button"
                  className="text-button"
                  onClick={() => {
                    setBaseline(fresh);
                    form.reset(toForm(fresh));
                    setFresh(null);
                    mutation.reset();
                  }}
                >
                  Загрузить актуальные данные
                </button>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => {
                    setBaseline(fresh);
                    setFresh(null);
                    mutation.reset();
                  }}
                >
                  Сохранить мой черновик для новой версии
                </button>
              </div>
              <small>После выбора снова нажмите «Сохранить».</small>
            </div>
          )}
          <button
            className="button primary"
            disabled={mutation.isPending || !!fresh || categories.isPending}
          >
            {mutation.isPending
              ? "Сохраняем…"
              : initial
                ? "Сохранить изменения"
                : "Создать место"}
          </button>
        </section>
        <section className="panel">
          <h2>Расположение</h2>
          <p className="muted">
            Нажмите на карту или введите координаты. Адрес заполняется вручную.
          </p>
          <div className="form-grid">
            <Field
              label="Долгота"
              error={form.formState.errors.longitude?.message}
            >
              <input inputMode="decimal" {...form.register("longitude")} />
            </Field>
            <Field
              label="Широта"
              error={form.formState.errors.latitude?.message}
            >
              <input inputMode="decimal" {...form.register("latitude")} />
            </Field>
          </div>
          <Suspense fallback={<Loading />}>
            <MapCanvas
              center={center}
              onPick={(p) => {
                form.setValue("longitude", p.longitude.toFixed(6), {
                  shouldDirty: true,
                  shouldValidate: true,
                });
                form.setValue("latitude", p.latitude.toFixed(6), {
                  shouldDirty: true,
                  shouldValidate: true,
                });
              }}
            />
          </Suspense>
        </section>
      </form>
    </>
  );
}
