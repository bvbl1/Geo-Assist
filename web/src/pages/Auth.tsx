import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation } from "@tanstack/react-query";
import { Eye, EyeOff, ArrowRight, Compass } from "lucide-react";
import { identity } from "../modules/identity/api";
import { registerSchema } from "../modules/identity/schemas";
import { useCategories } from "../modules/catalog/queries";
import { useSession } from "../app/session";
import { ApiError } from "../shared/api/client";
import { safeReturn } from "../shared/lib/format";
import { ErrorBox, Field } from "../shared/ui";
const loginFormSchema = registerSchema.extend({
  display_name: z.string(),
  password: z.string().min(1, "Введите пароль"),
});
export default function Auth({ register = false }: { register?: boolean }) {
  const session = useSession();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [visible, setVisible] = useState(false);
  const [wait, setWait] = useState(0);
  const category = useCategories();
  const form = useForm<z.infer<typeof registerSchema>>({
    resolver: zodResolver(register ? registerSchema : loginFormSchema),
    defaultValues: {
      email: params.get("email") ?? "",
      password: "",
      display_name: "",
      preferred_categories: [],
    },
  });
  const mutation = useMutation({
    mutationFn: async (v: z.infer<typeof registerSchema>) =>
      register
        ? identity.register(v)
        : identity.login({ email: v.email, password: v.password }),
    onSuccess: (user) => {
      form.resetField("password");
      if (register)
        navigate(
          "/login?" +
            new URLSearchParams({
              created: "1",
              email: user.email,
              returnTo: safeReturn(params.get("returnTo")),
            }),
        );
      else {
        session.accept(user);
        navigate(safeReturn(params.get("returnTo")), { replace: true });
      }
    },
    onError: (e) => {
      if (e instanceof ApiError) {
        if (e.status === 429) setWait(e.retryAfter || 60);
        if (e.code === "EMAIL_ALREADY_EXISTS")
          form.setError("email", { message: "Email уже зарегистрирован" });
        for (const d of e.details)
          if (["email", "password", "display_name"].includes(d.field))
            form.setError(d.field as "email" | "password" | "display_name", {
              message: "Проверьте поле: " + d.reason,
            });
      }
    },
  });
  useEffect(() => {
    if (wait <= 0) return;
    const timer = setTimeout(() => setWait((n) => n - 1), 1000);
    return () => clearTimeout(timer);
  }, [wait]);
  if (session.user)
    return (
      <div className="panel">
        <h1>Вы уже вошли</h1>
        <Link className="button primary" to="/explore">
          Перейти к местам
        </Link>
      </div>
    );
  return (
    <div className="auth-layout">
      <div className="auth-story">
        <Compass size={38} />
        <div className="eyebrow">GEOASSIST · АСТАНА</div>
        <h1>
          Город становится
          <br />
          ближе.
        </h1>
        <p>
          Сохраняйте любимые места, делитесь оценкой и находите пространство для
          своего дня.
        </p>
        <div className="story-line" />
        <span>Ваши места. Ваш ритм.</span>
      </div>
      <form
        className="panel auth-form"
        onSubmit={form.handleSubmit((v) => mutation.mutate(v))}
        noValidate
      >
        <div className="eyebrow">ЛИЧНОЕ ПРОСТРАНСТВО</div>
        <h2>{register ? "Создать аккаунт" : "С возвращением"}</h2>
        <p className="muted">
          {register
            ? "Соберите свои места в одном месте."
            : "Войдите, чтобы продолжить исследовать город."}
        </p>
        {params.get("created") && (
          <div className="alert success">Аккаунт создан. Теперь войдите.</div>
        )}
        <Field label="Email" error={form.formState.errors.email?.message}>
          <input
            type="email"
            autoComplete="email"
            {...form.register("email")}
            aria-invalid={!!form.formState.errors.email}
          />
        </Field>
        {register && (
          <Field
            label="Имя"
            error={form.formState.errors.display_name?.message}
          >
            <input autoComplete="name" {...form.register("display_name")} />
          </Field>
        )}
        <Field
          label="Пароль"
          error={form.formState.errors.password?.message}
          hint={
            register
              ? "12–72 байта. Для латинских букв и цифр — 12–72 символа."
              : undefined
          }
        >
          <div className="password-input">
            <input
              type={visible ? "text" : "password"}
              autoComplete={register ? "new-password" : "current-password"}
              {...form.register("password")}
            />
            <button
              type="button"
              aria-label={visible ? "Скрыть пароль" : "Показать пароль"}
              onClick={() => setVisible(!visible)}
            >
              {visible ? <EyeOff size={18} /> : <Eye size={18} />}
            </button>
          </div>
        </Field>
        {register && (
          <fieldset>
            <legend>
              Что вам интересно? <small>Необязательно</small>
            </legend>
            <div className="category-checks">
              {category.data?.map((c) => (
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
            <ErrorBox
              error={category.error}
              retry={() => {
                void category.refetch();
              }}
            />
          </fieldset>
        )}
        <ErrorBox error={mutation.error} />
        <button
          className="button primary full"
          disabled={mutation.isPending || wait > 0}
        >
          {wait > 0
            ? `Подождите ${wait} сек.`
            : mutation.isPending
              ? "Отправляем…"
              : register
                ? "Создать аккаунт"
                : "Войти"}
          <ArrowRight size={17} />
        </button>
        <p className="auth-switch">
          {register ? "Уже есть аккаунт?" : "Впервые здесь?"}{" "}
          <Link
            to={
              (register ? "/login" : "/register") +
              "?returnTo=" +
              encodeURIComponent(safeReturn(params.get("returnTo")))
            }
          >
            {register ? "Войти" : "Зарегистрироваться"}
          </Link>
        </p>
      </form>
    </div>
  );
}
