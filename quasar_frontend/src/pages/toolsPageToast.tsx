import { useCallback, useEffect, useRef, useState } from "react";

export type ToolsToastTone = "ok" | "err" | "info";

export type ToolsToastState = { tone: ToolsToastTone; text: string } | null;

export function useToolsPageToast() {
  const [toast, setToast] = useState<ToolsToastState>(null);
  const timerRef = useRef<number | null>(null);

  const dismiss = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setToast(null);
  }, []);

  const show = useCallback(
    (tone: ToolsToastTone, text: string, autoMs?: number) => {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      setToast({ tone, text });
      const ms = autoMs ?? 10_000;
      if (ms > 0) {
        timerRef.current = window.setTimeout(() => {
          timerRef.current = null;
          setToast(null);
        }, ms);
      }
    },
    [],
  );

  useEffect(() => {
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
    };
  }, []);

  return { toast, show, dismiss };
}

export function ToolsPageToastHost({ toast, onDismiss }: { toast: ToolsToastState; onDismiss: () => void }) {
  if (!toast) return null;
  const mod = toast.tone === "ok" ? "page-toast--ok" : toast.tone === "err" ? "page-toast--err" : "page-toast--info";
  return (
    <div className={`page-toast ${mod}`} role="status" aria-live="polite">
      <button type="button" className="page-toast__close" aria-label="Fechar" onClick={onDismiss}>
        ×
      </button>
      {toast.text}
    </div>
  );
}
