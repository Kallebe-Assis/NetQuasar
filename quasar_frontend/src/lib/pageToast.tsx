import { useCallback, useEffect, useRef, useState } from "react";

export type PageToastTone = "ok" | "err" | "info";

export type PageToastState = { tone: PageToastTone; text: string } | null;

export function usePageToast() {
  const [toast, setToast] = useState<PageToastState>(null);
  const timerRef = useRef<number | null>(null);

  const dismiss = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setToast(null);
  }, []);

  const show = useCallback((tone: PageToastTone, text: string, autoMs?: number) => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setToast({ tone, text });
    const ms = autoMs ?? (tone === "err" ? 10_000 : 7000);
    if (ms > 0) {
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null;
        setToast(null);
      }, ms);
    }
  }, []);

  useEffect(() => {
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
    };
  }, []);

  return { toast, show, dismiss };
}

export function PageToastHost({ toast, onDismiss }: { toast: PageToastState; onDismiss: () => void }) {
  if (!toast) return null;
  const mod = toast.tone === "ok" ? "page-toast--ok" : toast.tone === "err" ? "page-toast--err" : "page-toast--info";
  return (
    <div className={`page-toast ${mod}`} role="status" aria-live="polite" style={{ marginTop: 12, maxWidth: 560 }}>
      <button type="button" className="page-toast__close" aria-label="Fechar" onClick={onDismiss}>
        ×
      </button>
      {toast.text}
    </div>
  );
}
