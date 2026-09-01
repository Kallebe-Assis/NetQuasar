import { useRef, useState } from "react";
import { ImageUp, Link as LinkIcon, Trash2 } from "lucide-react";
import { INTEGRATION_LOGO_MAX_BYTES, clearIntegrationLogo, useIntegrationLogo, writeIntegrationLogo } from "../lib/integrationLogo";
import { useAppToast } from "../lib/appToast";
import { toastErr } from "../lib/operationToast";

/** Campo de logo (por integração/slug) — upload de ficheiro ou link de imagem, guardado no
 * localStorage do navegador (ver lib/integrationLogo.ts). Usado na tela de configuração de
 * qualquer integração genérica; a HubSoft mantém o seu próprio campo dedicado (hubsoftLogo.ts). */
export function IntegrationLogoField({ slug }: { slug: string }) {
  const logo = useIntegrationLogo(slug);
  const inputRef = useRef<HTMLInputElement>(null);
  const [url, setUrl] = useState("");
  const { push: pushToast } = useAppToast();

  function handleFile(file: File | null) {
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toastErr(pushToast, new Error("Selecione um ficheiro de imagem."));
      return;
    }
    if (file.size > INTEGRATION_LOGO_MAX_BYTES) {
      toastErr(pushToast, new Error(`Imagem muito grande (máx. ${Math.round(INTEGRATION_LOGO_MAX_BYTES / 1024)}KB).`));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") writeIntegrationLogo(slug, reader.result);
    };
    reader.onerror = () => toastErr(pushToast, new Error("Falha ao ler a imagem."));
    reader.readAsDataURL(file);
  }

  function applyUrl() {
    const u = url.trim();
    if (!/^https?:\/\/.+/i.test(u)) {
      toastErr(pushToast, new Error("Cole um link http(s) válido de uma imagem."));
      return;
    }
    writeIntegrationLogo(slug, u);
    setUrl("");
  }

  return (
    <div className="field">
      <label>Logo</label>
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        {logo ? <img src={logo} alt="" style={{ height: 32, maxWidth: 120, objectFit: "contain" }} /> : null}
        <input ref={inputRef} type="file" accept="image/*" style={{ display: "none" }} onChange={(e) => handleFile(e.target.files?.[0] ?? null)} />
        <button type="button" className="btn btn--sm" onClick={() => inputRef.current?.click()}>
          <ImageUp size={13} /> {logo ? "Trocar logo" : "Enviar logo"}
        </button>
        {logo ? (
          <button type="button" className="btn btn--sm" onClick={() => clearIntegrationLogo(slug)}>
            <Trash2 size={13} /> Remover
          </button>
        ) : null}
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 8 }}>
        <input
          className="input mono"
          style={{ flex: 1 }}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="…ou cole o link de uma imagem (https://…)"
          onKeyDown={(e) => {
            if (e.key === "Enter") applyUrl();
          }}
        />
        <button type="button" className="btn btn--sm" disabled={!url.trim()} onClick={applyUrl}>
          <LinkIcon size={13} /> Usar link
        </button>
      </div>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0" }}>
        Mostrado ao lado do nome no card da tela Integrações. Guardado neste navegador (não é partilhado entre dispositivos).
      </p>
    </div>
  );
}
