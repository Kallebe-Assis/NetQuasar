import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

export type AppToastTone = "ok" | "err" | "info";

export type AppToastItem = {
  id: string;
  tone: AppToastTone;
  text: string;
  at: number;
  kind?: "default" | "offline";
  offlineTitle?: string;
  offlineIp?: string;
  onDismiss?: () => void;
};

type PushInput = {
  tone: AppToastTone;
  text: string;
  autoMs?: number;
  kind?: "default" | "offline";
  offlineTitle?: string;
  offlineIp?: string;
  onDismiss?: () => void;
};

type AppToastContextValue = {
  push: (input: PushInput) => string;
  dismiss: (id: string) => void;
};

const AppToastContext = createContext<AppToastContextValue | null>(null);

let toastSeq = 0;

export function AppToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<AppToastItem[]>([]);
  const timersRef = useRef<Map<string, number>>(new Map());

  const dismiss = useCallback((id: string) => {
    const t = timersRef.current.get(id);
    if (t != null) {
      window.clearTimeout(t);
      timersRef.current.delete(id);
    }
    setItems((prev) => {
      const row = prev.find((x) => x.id === id);
      if (row?.onDismiss) row.onDismiss();
      return prev.filter((x) => x.id !== id);
    });
  }, []);

  const push = useCallback(
    (input: PushInput) => {
      const id = `toast-${++toastSeq}-${Date.now()}`;
      const at = Date.now();
      const row: AppToastItem = {
        id,
        tone: input.tone,
        text: input.text,
        at,
        kind: input.kind ?? "default",
        offlineTitle: input.offlineTitle,
        offlineIp: input.offlineIp,
        onDismiss: input.onDismiss,
      };
      setItems((prev) => [row, ...prev].slice(0, 12));
      const ms = input.autoMs ?? 10_000;
      if (ms > 0) {
        const tid = window.setTimeout(() => dismiss(id), ms);
        timersRef.current.set(id, tid);
      }
      return id;
    },
    [dismiss],
  );

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      timers.forEach((tid) => window.clearTimeout(tid));
      timers.clear();
    };
  }, []);

  const value = useMemo(() => ({ push, dismiss }), [push, dismiss]);

  return (
    <AppToastContext.Provider value={value}>
      {children}
      <AppToastStack items={items} onDismiss={dismiss} />
    </AppToastContext.Provider>
  );
}

export function useAppToast() {
  const ctx = useContext(AppToastContext);
  if (!ctx) {
    throw new Error("useAppToast deve ser usado dentro de AppToastProvider");
  }
  return ctx;
}

function toneClass(tone: AppToastTone): string {
  if (tone === "ok") return "page-toast--ok";
  if (tone === "err") return "page-toast--err";
  return "page-toast--info";
}

export function AppToastStack({ items, onDismiss }: { items: AppToastItem[]; onDismiss: (id: string) => void }) {
  if (items.length === 0) return null;
  const sorted = [...items].sort((a, b) => b.at - a.at);
  return (
    <div className="app-toast-stack" aria-live="polite">
      {sorted.map((t) =>
        t.kind === "offline" ? (
          <div key={t.id} className="page-toast page-toast--err page-toast--offline app-toast-stack__item" role="status">
            <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => onDismiss(t.id)}>
              ×
            </button>
            <div className="app-toast-stack__offline-title">{t.offlineTitle || "Equipamento offline"}</div>
            {t.offlineIp ? <div className="app-toast-stack__offline-ip mono">{t.offlineIp}</div> : null}
          </div>
        ) : (
          <div key={t.id} className={`page-toast ${toneClass(t.tone)} app-toast-stack__item`} role="status">
            <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => onDismiss(t.id)}>
              ×
            </button>
            <div className="app-toast-stack__text">{t.text}</div>
          </div>
        ),
      )}
    </div>
  );
}
