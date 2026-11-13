import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useSession } from "../app/session";
import { profileSchema } from "../modules/identity/schemas";
import { identity } from "../modules/identity/api";
import { useCategories } from "../modules/catalog/queries";
import { ErrorBox, Field, PageHeading } from "../shared/ui";
export default function Profile() {
  const { user } = useSession();
  const client = useQueryClient();
  const categories = useCategories();
  const form = useForm<z.infer<typeof profileSchema>>({
    resolver: zodResolver(profileSchema),
    defaultValues: {
      display_name: user!.display_name,
      preferred_categories: user!.preferred_categories,
    },
  });
  const mutation = useMutation({
    mutationFn: identity.update,
    onSuccess: (data) => {
      client.setQueryData(["session"], data);
      form.reset({
        display_name: data.display_name,
        preferred_categories: data.preferred_categories,
      });
    },
  });
  return (
    <>
      <PageHeading eyebrow="АККАУНТ" title="Мой профиль">
        Ваше имя и интересы.
      </PageHeading>
      <form
        className="panel narrow"
        onSubmit={form.handleSubmit((v) => {
          const body: {
            display_name?: string;
            preferred_categories?: string[];
          } = {};
          if (v.display_name !== user?.display_name)
            body.display_name = v.display_name;
          if (
            JSON.stringify(v.preferred_categories) !==
            JSON.stringify(user?.preferred_categories)
          )
            body.preferred_categories = v.preferred_categories;
          if (Object.keys(body).length) mutation.mutate(body);
        })}
      >
        <Field label="Email">
          <input value={user?.email} readOnly />
        </Field>
        <Field label="Имя" error={form.formState.errors.display_name?.message}>
          <input {...form.register("display_name")} />
        </Field>
        <fieldset>
          <legend>Любимые категории</legend>
          <div className="category-checks">
            {categories.data?.map((c) => (
              <label className="check" key={c.code}>
                <input
                  type="checkbox"
                  value={c.code}
                  {...form.register("preferred_categories")}
                />
                {c.name}
              </label>
            ))}
          </div>
          <small>
            Можно снять все отметки. Эти предпочтения не ограничивают поиск.
          </small>
        </fieldset>
        <ErrorBox
          error={categories.error}
          retry={() => {
            void categories.refetch();
          }}
        />
        <ErrorBox error={mutation.error} />
        {mutation.isSuccess && !form.formState.isDirty && (
          <div className="alert success">Профиль сохранён</div>
        )}
        <button
          className="button primary"
          disabled={mutation.isPending || !form.formState.isDirty}
        >
          {mutation.isPending ? "Сохраняем…" : "Сохранить изменения"}
        </button>
      </form>
    </>
  );
}
