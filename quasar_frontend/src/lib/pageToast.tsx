import { useCallback, useEffect, useRef } from "react";
import { useAppToast, type AppToastTone } from "./appToast";

export type PageToastTone = AppToastTone;

export type PageToastState = { tone: PageToastTone; text: string } | null;

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
