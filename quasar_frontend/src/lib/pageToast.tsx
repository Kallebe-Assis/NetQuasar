import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { useAppToast, type AppToastTone } from "./appToast";

export type PageToastTone = AppToastTone;

export type PageToastState = { tone: PageToastTone; text: string } | null;

/** Tempo padrão para toasts inline e globais desaparecerem sozinhos. */
export const PAGE_TOAST_AUTO_MS = 7_000;

/** Duração da animação de saída (deve coincidir com CSS `offlineToastOut`). */
export const PAGE_TOAST_LEAVE_MS = 350;

export type InlinePageToast = { ok: boolean; text: string } | null;

/** Estado de toast local com auto-dismiss e animação de saída. */
export function useInlinePageToast() {
  const [toast, setToast] = useState<InlinePageToast>(null);
  const [leaving, setLeaving] = useState(false);
  const timerRef = useRef<number | null>(null);
  const leaveTimerRef = useRef<number | null>(null);

  const clearTimers = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (leaveTimerRef.current != null) {
      window.clearTimeout(leaveTimerRef.current);
      leaveTimerRef.current = null;
    }
  }, []);

  const dismissToast = useCallback(() => {
    clearTimers();
    setLeaving(true);
    leaveTimerRef.current = window.setTimeout(() => {
      setToast(null);
      setLeaving(false);
      leaveTimerRef.current = null;
    }, PAGE_TOAST_LEAVE_MS);
  }, [clearTimers]);

  const setToastWithAuto = useCallback(
    (next: InlinePageToast) => {
      clearTimers();
      setLeaving(false);
      setToast(next);
    },
    [clearTimers],
  );

  useEffect(() => {
    if (!toast) return;
    timerRef.current = window.setTimeout(() => dismissToast(), PAGE_TOAST_AUTO_MS);
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
    };
  }, [toast, dismissToast]);

  useEffect(() => () => clearTimers(), [clearTimers]);

  return [toast, setToastWithAuto, leaving, dismissToast] as const;
}

export function InlinePageToastBanner({
  toast,
  leaving,
  onDismiss,
  style,
}: {
  toast: InlinePageToast;
  leaving?: boolean;
  onDismiss: () => void;
  style?: CSSProperties;
}) {
  if (!toast) return null;
  return (
    <div
      className={`page-toast ${toast.ok ? "page-toast--ok" : "page-toast--err"}${leaving ? " page-toast--leave" : ""}`}
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
  const leaveTimerRef = useRef<number | null>(null);
  const [, setLeaving] = useState(false);

  useEffect(() => {
    if (value == null) return;
    const t = window.setTimeout(() => {
      setLeaving(true);
      leaveTimerRef.current = window.setTimeout(() => {
        setValue(null);
        setLeaving(false);
      }, PAGE_TOAST_LEAVE_MS);
    }, ms);
    return () => {
      window.clearTimeout(t);
      if (leaveTimerRef.current != null) window.clearTimeout(leaveTimerRef.current);
    };
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
      lastId.current = push({ tone, text, autoMs: autoMs ?? PAGE_TOAST_AUTO_MS });
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
