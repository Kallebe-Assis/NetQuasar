import { useEffect, useState } from "react";

/**
 * Logo de UMA integração (por slug) — mostrado no card da tela Integrações e no cabeçalho da
 * tela de configuração dela. Guardado no localStorage do navegador — não há endpoint dedicado no
 * backend para upload de ficheiros de marca, e isto é só uma preferência visual local (o mesmo
 * mecanismo já usado só para a HubSoft em hubsoftLogo.ts, generalizado aqui para qualquer slug).
 */
const KEY_PREFIX = "netquasar.integration.logo.";
const EVENT = "integration-logo-changed";

/** Tamanho máximo aceite para o ficheiro de logo (~300KB) — o localStorage tem um teto baixo. */
export const INTEGRATION_LOGO_MAX_BYTES = 300 * 1024;

function key(slug: string): string {
  return KEY_PREFIX + slug;
}

export function readIntegrationLogo(slug: string): string | null {
  try {
    return localStorage.getItem(key(slug));
  } catch {
    return null;
  }
}

export function writeIntegrationLogo(slug: string, dataUrl: string) {
  try {
    localStorage.setItem(key(slug), dataUrl);
    window.dispatchEvent(new Event(EVENT));
  } catch {
    /* localStorage indisponível (modo privado, quota) — ignora silenciosamente */
  }
}

export function clearIntegrationLogo(slug: string) {
  try {
    localStorage.removeItem(key(slug));
    window.dispatchEvent(new Event(EVENT));
  } catch {
    /* ignore */
  }
}

/** Mantém o card/cabeçalho sincronizado quando o logo é alterado na aba de Configuração. */
export function useIntegrationLogo(slug: string): string | null {
  const [logo, setLogo] = useState<string | null>(() => readIntegrationLogo(slug));
  useEffect(() => {
    setLogo(readIntegrationLogo(slug));
    const sync = () => setLogo(readIntegrationLogo(slug));
    window.addEventListener(EVENT, sync);
    window.addEventListener("storage", sync);
    return () => {
      window.removeEventListener(EVENT, sync);
      window.removeEventListener("storage", sync);
    };
  }, [slug]);
  return logo;
}
