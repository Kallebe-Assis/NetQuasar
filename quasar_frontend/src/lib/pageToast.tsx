import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { useAppToast, type AppToastTone } from "./appToast";

export type PageToastTone = AppToastTone;

export type PageToastState = { tone: PageToastTone; text: string } | null;

/** Tempo padrão para toasts inline e globais desaparecerem sozinhos. */
export const PAGE_TOAST_AUTO_MS = 10_000;

export type InlinePageToast = { ok: boolean; text: string } | null;

/** Estado de toast local com auto-dismiss em 10s (padrão do produto). */
export function useInlinePageToast() {
  const [toast, setToast] = useState<InlinePageToast>(null);
  useEffect(() => {
    if (!toast) return;
    const t = window.setTimeout(() => setToast(null), PAGE_TOAST_AUTO_MS);
    return () => window.clearTimeout(t);
  }, [toast]);
  return [toast, setToast] as const;
}

export function InlinePageToastBanner({
  toast,
  onDismiss,
  style,
}: {
  toast: InlinePageToast;
  onDismiss: () => void;
  style?: CSSProperties;
}) {
  if (!toast) return null;
  return (
    <div
      className={`page-toast ${toast.ok ? "page-toast--ok" : "page-toast--err"}`}
      role="status"
      style={style}
    >
      <button type="button" className="page-toast__close" aria-label="Fechar" onClick={onDismiss}>
        ×
      </button>
      {toast.text}
    </div>
  );
}

/** Auto-dismiss para estados de toast já existentes (migração gradual). */
export function useAutoDismissToast<T>(value: T | null, setValue: (v: T | null) => void, ms = PAGE_TOAST_AUTO_MS) {
  useEffect(() => {
    if (value == null) return;
    const t = window.setTimeout(() => setValue(null), ms);
    return () => window.clearTimeout(t);
  }, [value, setValue, ms]);
}

/** Compatível com páginas antigas: envia para a pilha global (canto superior direito). */
export function usePageToast() {
  const { push, dismiss } = useAppToast();
  const lastId = useRef<string | null>(null);

  const dismissLast = useCallback(() => {
    if (lastId.current) {
      dismiss(lastId.current);
      lastId.current = null;
    }
  }, [dismiss]);

  const show = useCallback(
    (tone: PageToastTone, text: string, autoMs?: number) => {
      dismissLast();
      lastId.current = push({ tone, text, autoMs });
    },
    [push, dismissLast],
  );

  useEffect(() => () => dismissLast(), [dismissLast]);

  return { toast: null as PageToastState, show, dismiss: dismissLast };
}

/** Host legado — toasts vão para AppToastProvider (nada a renderizar aqui). */
export function PageToastHost(_props: { toast: PageToastState; onDismiss: () => void }) {
  return null;
}
