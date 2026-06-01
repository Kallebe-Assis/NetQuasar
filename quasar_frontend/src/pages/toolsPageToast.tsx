import { useCallback, useEffect, useRef, useState } from "react";
import { PAGE_TOAST_AUTO_MS, PAGE_TOAST_LEAVE_MS } from "../lib/pageToast";

export type ToolsToastTone = "ok" | "err" | "info";

export type ToolsToastState = { tone: ToolsToastTone; text: string } | null;

export function useToolsPageToast() {
  const [toast, setToast] = useState<ToolsToastState>(null);
  const [leaving, setLeaving] = useState(false);
  const timerRef = useRef<number | null>(null);
  const leaveTimerRef = useRef<number | null>(null);

  const dismiss = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (leaveTimerRef.current != null) return;
    setLeaving(true);
    leaveTimerRef.current = window.setTimeout(() => {
      setToast(null);
      setLeaving(false);
      leaveTimerRef.current = null;
    }, PAGE_TOAST_LEAVE_MS);
  }, []);

  const show = useCallback(
    (tone: ToolsToastTone, text: string, autoMs?: number) => {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      if (leaveTimerRef.current != null) {
        window.clearTimeout(leaveTimerRef.current);
        leaveTimerRef.current = null;
      }
      setLeaving(false);
      setToast({ tone, text });
      const ms = autoMs ?? PAGE_TOAST_AUTO_MS;
      if (ms > 0) {
        timerRef.current = window.setTimeout(() => dismiss(), ms);
      }
    },
    [dismiss],
  );

  useEffect(() => {
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
      if (leaveTimerRef.current != null) window.clearTimeout(leaveTimerRef.current);
    };
  }, []);

  return { toast, leaving, show, dismiss };
}

export function ToolsPageToastHost({
  toast,
  leaving,
  onDismiss,
}: {
  toast: ToolsToastState;
  leaving?: boolean;
  onDismiss: () => void;
}) {
  if (!toast) return null;
  const mod = toast.tone === "ok" ? "page-toast--ok" : toast.tone === "err" ? "page-toast--err" : "page-toast--info";
  return (
    <div className={`page-toast ${mod}${leaving ? " page-toast--leave" : ""}`} role="status" aria-live="polite">
      <button type="button" className="page-toast__close" aria-label="Fechar" onClick={onDismiss}>
        ×
      </button>
      {toast.text}
    </div>
  );
}
