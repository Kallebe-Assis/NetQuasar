import { useEffect, useState } from "react";
import { clearHubsoftLogo, readHubsoftLogo, writeHubsoftLogo } from "./hubsoftLogo";

/**
 * Logo de UMA integração (por slug) — mostrado no card da tela Integrações e no cabeçalho da
 * tela de configuração dela. Guardado no localStorage do navegador — não há endpoint dedicado no
 * backend para upload de ficheiros de marca, e isto é só uma preferência visual local (o mesmo
 * mecanismo já usado só para a HubSoft em hubsoftLogo.ts, generalizado aqui para qualquer slug).
 *
 * A HubSoft mantém o seu próprio campo de logo dedicado (hubsoftLogo.ts, chave própria
 * "netquasar.hubsoft.logo") — sem a ponte abaixo, um logo configurado por lá nunca aparecia no
 * card dela na tela Integrações (que lê por aqui), porque as duas chaves de localStorage nunca
 * se encontravam. Em vez de duplicar o logo em duas chaves, desviamos aqui: para slug="hubsoft"
 * ler/gravar sempre passa pelas funções de hubsoftLogo.ts — uma única fonte da verdade.
 */
const KEY_PREFIX = "netquasar.integration.logo.";
const EVENT = "integration-logo-changed";
const HUBSOFT_SLUG = "hubsoft";

/** Tamanho máximo aceite para o ficheiro de logo (~300KB) — o localStorage tem um teto baixo. */
export const INTEGRATION_LOGO_MAX_BYTES = 300 * 1024;

function key(slug: string): string {
  return KEY_PREFIX + slug;
}

export function readIntegrationLogo(slug: string): string | null {
  if (slug === HUBSOFT_SLUG) return readHubsoftLogo();
  try {
    return localStorage.getItem(key(slug));
  } catch {
    return null;
  }
}

export function writeIntegrationLogo(slug: string, dataUrl: string) {
  if (slug === HUBSOFT_SLUG) {
    writeHubsoftLogo(dataUrl);
    window.dispatchEvent(new Event(EVENT));
    return;
  }
  try {
    localStorage.setItem(key(slug), dataUrl);
    window.dispatchEvent(new Event(EVENT));
  } catch {
    /* localStorage indisponível (modo privado, quota) — ignora silenciosamente */
  }
}

export function clearIntegrationLogo(slug: string) {
  if (slug === HUBSOFT_SLUG) {
    clearHubsoftLogo();
    window.dispatchEvent(new Event(EVENT));
    return;
  }
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
    // "hubsoft-logo-changed" é disparado por HubsoftConfigPage.tsx (via writeHubsoftLogo directo,
    // fora deste módulo) — sem ouvir este evento também, o card na tela Integrações só
    // actualizaria o logo da HubSoft depois de um reload da página.
    window.addEventListener("hubsoft-logo-changed", sync);
    window.addEventListener("storage", sync);
    return () => {
      window.removeEventListener(EVENT, sync);
      window.removeEventListener("hubsoft-logo-changed", sync);
      window.removeEventListener("storage", sync);
    };
  }, [slug]);
  return logo;
}
