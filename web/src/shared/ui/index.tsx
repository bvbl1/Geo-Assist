import {
  Children,
  cloneElement,
  isValidElement,
  useEffect,
  useId,
  useRef,
  type ReactNode,
} from "react";
import { ApiError, message } from "../api/client";
import { ChevronLeft, ChevronRight, LoaderCircle, MapPin } from "lucide-react";
export function Loading() {
  return (
    <div className="loading" role="status">
      <LoaderCircle className="spin" size={22} /> Загружаем…
    </div>
  );
}
export function ErrorBox({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  if (!error) return null;
  return (
    <div className="alert error" role="alert">
      <strong>{message(error)}</strong>
      {error instanceof ApiError && error.details.length > 0 && (
        <ul>
          {error.details.map((d) => (
            <li key={d.field}>
              {d.field}: {d.reason}
            </li>
          ))}
        </ul>
      )}
      {retry && (
        <button type="button" className="text-button" onClick={retry}>
          Повторить
        </button>
      )}
      {error instanceof ApiError && error.requestId && (
        <details>
          <summary>Подробности</summary>
          <small>Код запроса: {error.requestId}</small>
        </details>
      )}
    </div>
  );
}
export function Empty({
  title = "Пока ничего нет",
  children,
}: {
  title?: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <MapPin size={32} />
      <h2>{title}</h2>
      {children}
    </div>
  );
}
export function Pagination({
  offset,
  hasMore,
  onChange,
}: {
  offset: number;
  hasMore: boolean;
  onChange: (n: number) => void;
}) {
  return (
    <nav className="pagination" aria-label="Страницы">
      <button
        className="button secondary"
        disabled={!offset}
        onClick={() => onChange(Math.max(0, offset - 20))}
      >
        <ChevronLeft size={16} /> Назад
      </button>
      <span>Страница {Math.floor(offset / 20) + 1}</span>
      <button
        className="button secondary"
        disabled={!hasMore || offset + 20 > 10000}
        onClick={() => onChange(offset + 20)}
      >
        Далее <ChevronRight size={16} />
      </button>
    </nav>
  );
}
export function PageHeading({
  eyebrow,
  title,
  children,
}: {
  eyebrow?: string;
  title: string;
  children?: ReactNode;
}) {
  return (
    <header className="page-heading">
      {eyebrow && <div className="eyebrow">{eyebrow}</div>}
      <h1>{title}</h1>
      {children && <p>{children}</p>}
    </header>
  );
}
export function Field({
  label,
  error,
  children,
  hint,
}: {
  label: string;
  error?: string;
  children: ReactNode;
  hint?: string;
}) {
  const id = useId();
  const description =
    [hint ? id + "-hint" : "", error ? id + "-error" : ""]
      .filter(Boolean)
      .join(" ") || undefined;
  function bindControl(nodes: ReactNode): ReactNode {
    return Children.map(nodes, (node) => {
      if (
        !isValidElement<{
          id?: string;
          children?: ReactNode;
          "aria-describedby"?: string;
          "aria-invalid"?: boolean;
        }>(node)
      )
        return node;
      if (
        typeof node.type === "string" &&
        ["input", "select", "textarea"].includes(node.type)
      )
        return cloneElement(node, {
          id,
          "aria-describedby": description,
          "aria-invalid": !!error,
        });
      return node.props.children
        ? cloneElement(node, { children: bindControl(node.props.children) })
        : node;
    });
  }
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {bindControl(children)}
      {hint && <small id={id + "-hint"}>{hint}</small>}
      {error && (
        <small id={id + "-error"} className="field-error" role="alert">
          {error}
        </small>
      )}
    </div>
  );
}
export function Confirm({
  open,
  title,
  children,
  busy,
  onClose,
  onConfirm,
}: {
  open: boolean;
  title: string;
  children: ReactNode;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (open) ref.current?.showModal();
    else ref.current?.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onClose();
      }}
      aria-labelledby="confirm-title"
    >
      <h2 id="confirm-title">{title}</h2>
      {children}
      <div className="actions">
        <button className="button secondary" disabled={busy} onClick={onClose}>
          Отмена
        </button>
        <button className="button danger" disabled={busy} onClick={onConfirm}>
          {busy ? "Удаляем…" : "Удалить"}
        </button>
      </div>
    </dialog>
  );
}
