import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ImageUp, Link as LinkIcon, Trash2 } from "lucide-react";
import { apiFetch } from "../lib/api";
import { queryKeys } from "../lib/queryKeys";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";

/** Tamanho máximo aceite para upload de ficheiro (~300KB) — vira um data: URI guardado como
 * texto no banco (coluna integrations.logo_url); um link (https://…) não tem este limite, é só
 * texto curto. */
export const INTEGRATION_LOGO_MAX_BYTES = 300 * 1024;

/** Campo de logo (por integração) — upload de ficheiro ou link de imagem, guardado no banco
 * (integrations.logo_url, via PATCH /api/v1/integrations/{slug}) — antes ficava só no
 * localStorage do navegador, por isso não persistia entre navegadores/dispositivos ("sumia" ao
 * trocar de máquina ou limpar dados). */
export function IntegrationLogoField({ slug, logoUrl }: { slug: string; logoUrl?: string | null }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [url, setUrl] = useState("");
  const { push: pushToast } = useAppToast();
  const qc = useQueryClient();

  const save = useMutation({
    mutationFn: (next: string | null) =>
      apiFetch(`/api/v1/integrations/${slug}`, { method: "PATCH", json: { logo_url: next } }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.integrations });
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug) });
      toastOk(pushToast, "Logo guardada.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar a logo."),
  });

  function handleFile(file: File | null) {
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toastErr(pushToast, new Error("Selecione um ficheiro de imagem."));
      return;
    }
    if (file.size > INTEGRATION_LOGO_MAX_BYTES) {
      toastErr(pushToast, new Error(`Imagem muito grande (máx. ${Math.round(INTEGRATION_LOGO_MAX_BYTES / 1024)}KB) — prefira colar um link.`));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") save.mutate(reader.result);
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
    save.mutate(u);
    setUrl("");
  }

  return (
    <div className="field">
      <label>Logo</label>
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        {logoUrl ? <img src={logoUrl} alt="" style={{ height: 32, maxWidth: 120, objectFit: "contain" }} /> : null}
        <input ref={inputRef} type="file" accept="image/*" style={{ display: "none" }} onChange={(e) => handleFile(e.target.files?.[0] ?? null)} />
        <button type="button" className="btn btn--sm" disabled={save.isPending} onClick={() => inputRef.current?.click()}>
          <ImageUp size={13} /> {logoUrl ? "Trocar logo" : "Enviar logo"}
        </button>
        {logoUrl ? (
          <button type="button" className="btn btn--sm" disabled={save.isPending} onClick={() => save.mutate(null)}>
            <Trash2 size={13} /> Remover
          </button>
        ) : null}
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 8 }}>
        <input
          className="input mono"
          style={{ flex: 1 }}
          value={url}
          disabled={save.isPending}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="…ou cole o link de uma imagem (https://…)"
          onKeyDown={(e) => {
            if (e.key === "Enter") applyUrl();
          }}
        />
        <button type="button" className="btn btn--sm" disabled={save.isPending || !url.trim()} onClick={applyUrl}>
          <LinkIcon size={13} /> {save.isPending ? "A guardar…" : "Usar link"}
        </button>
      </div>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0" }}>
        Mostrado ao lado do nome no card da tela Integrações — guardado no servidor, igual para toda a equipa.
      </p>
    </div>
  );
}
