import { useState } from "react";
import { Download, RefreshCw, Share, X } from "lucide-react";
import { promptInstall, usePwaInstall } from "./install";
import { usePwaUpdate } from "./registerServiceWorker";

const DISMISS_KEY = "netquasar-pwa-install-dismissed";
const DISMISS_DAYS = 14;

function dismissedRecently(): boolean {
  try {
    const t = Number(localStorage.getItem(DISMISS_KEY) || 0);
    return t > 0 && Date.now() - t < DISMISS_DAYS * 86_400_000;
  } catch {
    return false;
  }
}

/** Aviso «Nova versão disponível» — aparece quando o service worker novo já foi baixado. */
export function PwaUpdatePrompt() {
  const { updateReady, applyUpdate } = usePwaUpdate();
  if (!updateReady) return null;
  return (
    <div className="pwa-banner pwa-banner--update" role="status">
      <RefreshCw size={18} aria-hidden />
      <span className="pwa-banner__txt">Nova versão do NetQuasar disponível.</span>
      <button type="button" className="btn btn--primary btn--sm" onClick={applyUpdate}>
        Atualizar
      </button>
    </div>
  );
}

/**
 * Convite para instalar o app. Só aparece no celular/tablet (tela estreita), quando o app AINDA não está instalado,
 * e some por 14 dias se o usuário dispensar.
 */
export function PwaInstallBanner() {
  const { canPrompt, installed, standalone, ios } = usePwaInstall();
  const { updateReady } = usePwaUpdate();
  const [hidden, setHidden] = useState(dismissedRecently);
  const narrow = typeof window !== "undefined" && window.matchMedia("(max-width: 900px)").matches;

  // com «Nova versão» na tela, o convite de instalação espera (os dois ocupam o mesmo lugar)
  if (hidden || installed || standalone || !narrow || updateReady) return null;
  if (!canPrompt && !ios) return null;

  const dismiss = () => {
    try {
      localStorage.setItem(DISMISS_KEY, String(Date.now()));
    } catch {
      /* sem armazenamento: apenas some nesta sessão */
    }
    setHidden(true);
  };

  return (
    <div className="pwa-banner" role="region" aria-label="Instalar o NetQuasar">
      <Download size={18} aria-hidden />
      {canPrompt ? (
        <>
          <span className="pwa-banner__txt">Instale o NetQuasar no celular: abre em tela cheia e recebe notificações.</span>
          <button
            type="button"
            className="btn btn--primary btn--sm"
            onClick={() => {
              void promptInstall().then((r) => r === "accepted" && setHidden(true));
            }}
          >
            Instalar
          </button>
        </>
      ) : (
        <span className="pwa-banner__txt">
          Para instalar no iPhone: toque em <Share size={14} aria-hidden style={{ verticalAlign: -2 }} /> <b>Compartilhar</b> e depois em <b>Adicionar à Tela de Início</b>.
        </span>
      )}
      <button type="button" className="pwa-banner__close" aria-label="Dispensar" onClick={dismiss}>
        <X size={16} aria-hidden />
      </button>
    </div>
  );
}
