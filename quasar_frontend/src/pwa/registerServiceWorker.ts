/**
 * Registro do service worker (/sw.js) e controle de atualização.
 *
 * Fluxo: o SW novo é instalado e FICA ESPERANDO; `usePwaUpdate` avisa a tela («Nova versão disponível»); ao tocar em
 * «Atualizar» mandamos SKIP_WAITING e, quando o SW novo assume (`controllerchange`), recarregamos a página uma vez.
 * Só em produção (em `npm run dev` o SW atrapalharia o hot reload).
 */
import { useSyncExternalStore } from "react";

let registration: ServiceWorkerRegistration | null = null;
let updateReady = false;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

function watch(reg: ServiceWorkerRegistration) {
  const mark = () => {
    // só é "atualização" se já havia um SW controlando a página (a 1ª instalação não conta)
    if (reg.waiting && navigator.serviceWorker.controller) {
      updateReady = true;
      emit();
    }
  };
  mark();
  reg.addEventListener("updatefound", () => {
    const sw = reg.installing;
    sw?.addEventListener("statechange", () => {
      if (sw.state === "installed") mark();
    });
  });
}

export function registerServiceWorker(): void {
  if (!("serviceWorker" in navigator) || !import.meta.env.PROD) return;

  const hadController = !!navigator.serviceWorker.controller;
  let reloading = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    // 1ª instalação (clients.claim) não precisa recarregar; troca de versão, sim — uma vez só.
    if (!hadController || reloading) return;
    reloading = true;
    window.location.reload();
  });

  const start = () => {
    navigator.serviceWorker
      .register("/sw.js")
      .then((reg) => {
        registration = reg;
        watch(reg);
        // procura versão nova ao voltar para o app e a cada 30 min (PWA instalado fica aberto por dias)
        const check = () => void reg.update().catch(() => undefined);
        document.addEventListener("visibilitychange", () => {
          if (document.visibilityState === "visible") check();
        });
        window.setInterval(check, 30 * 60 * 1000);
      })
      .catch(() => undefined);
  };
  // `load` pode já ter acontecido quando este código roda (ex.: carregamento adiado)
  if (document.readyState === "complete") start();
  else window.addEventListener("load", start, { once: true });
}

export function applyServiceWorkerUpdate(): void {
  registration?.waiting?.postMessage({ type: "SKIP_WAITING" });
}

/** `true` quando há uma versão nova baixada esperando para ser aplicada. */
export function usePwaUpdate(): { updateReady: boolean; applyUpdate: () => void } {
  const ready = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => updateReady,
    () => false,
  );
  return { updateReady: ready, applyUpdate: applyServiceWorkerUpdate };
}
