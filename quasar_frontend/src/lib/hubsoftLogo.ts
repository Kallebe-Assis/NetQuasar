import { useEffect, useState } from "react";

/**
 * Logo da HubSoft mostrado ao lado do título no cabeçalho das telas de Consulta/Configuração
 * (src/pages/hubsoft). Guardado no localStorage do navegador — não há endpoint dedicado no
 * backend para upload de ficheiros de marca, e isto é só uma preferência visual local.
 */
const KEY = "netquasar.hubsoft.logo";
const EVENT = "hubsoft-logo-changed";

/** Tamanho máximo aceite para o ficheiro de logo (~300KB) — o localStorage tem um teto baixo. */
export const HUBSOFT_LOGO_MAX_BYTES = 300 * 1024;

export function readHubsoftLogo(): string | null {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function writeHubsoftLogo(dataUrl: string) {
  try {
    localStorage.setItem(KEY, dataUrl);
    window.dispatchEvent(new Event(EVENT));
  } catch {
    /* localStorage indisponível (modo privado, quota) — ignora silenciosamente */
  }
}

export function clearHubsoftLogo() {
  try {
    localStorage.removeItem(KEY);
    window.dispatchEvent(new Event(EVENT));
  } catch {
    /* ignore */
  }
}

/** Mantém o cabeçalho sincronizado quando o logo é alterado na aba de Configuração. */
export function useHubsoftLogo(): string | null {
  const [logo, setLogo] = useState<string | null>(() => readHubsoftLogo());
  useEffect(() => {
    const sync = () => setLogo(readHubsoftLogo());
    window.addEventListener(EVENT, sync);
    window.addEventListener("storage", sync);
    return () => {
      window.removeEventListener(EVENT, sync);
      window.removeEventListener("storage", sync);
    };
  }, []);
  return logo;
}
