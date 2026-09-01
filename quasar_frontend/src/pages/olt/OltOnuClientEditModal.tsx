import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useQuery } from "@tanstack/react-query";
import { Blocks } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr } from "../../lib/operationToast";
import type { ClientCard, ClientSearchResponse, IntegrationSummary } from "../../integrations/types";

type Props = {
  open: boolean;
  serial: string;
  currentName: string;
  busy?: boolean;
  onCancel: () => void;
  onSave: (name: string) => void;
  onRemove: () => void;
};

/** Pesquisa leve (só nome + ID) na integração escolhida — usada pelo botão "Integração" abaixo
 * para encontrar o nome exacto do cliente sem digitar à mão. Reaproveita o mesmo endpoint de
 * busca por nome já usado nas telas de Consulta (genérico: consumer/client-search; HubSoft:
 * hubsoft/search), só que descarta tudo menos nome/código do resultado. */
function IntegrationClientSearch({
  initialTerm,
  onPick,
  onClose,
}: {
  initialTerm: string;
  onPick: (name: string) => void;
  onClose: () => void;
}) {
  const { push: pushToast } = useAppToast();
  const integrationsQ = useQuery({
    queryKey: ["integrations-enabled-picker"],
    queryFn: () => apiFetch<{ integrations: IntegrationSummary[] }>("/api/v1/integrations"),
  });
  const enabled = (integrationsQ.data?.integrations ?? []).filter((it) => it.enabled);
  const [integrationId, setIntegrationId] = useState<string>("");
  const [term, setTerm] = useState(initialTerm);
  const [loading, setLoading] = useState(false);
  const [results, setResults] = useState<ClientCard[] | null>(null);

  useEffect(() => {
    if (!integrationId && enabled.length > 0) setIntegrationId(enabled[0].id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled.length]);

  const selectedIntegration = enabled.find((it) => it.id === integrationId) ?? null;

  async function runSearch() {
    const t = term.trim();
    if (!t || !selectedIntegration) return;
    setLoading(true);
    setResults(null);
    try {
      const path =
        selectedIntegration.slug === "hubsoft"
          ? `/api/v1/integrations/${selectedIntegration.id}/hubsoft/search`
          : `/api/v1/integrations/${selectedIntegration.slug}/consumer/client-search`;
      const body =
        selectedIntegration.slug === "hubsoft"
          ? { busca: "nome_razaosocial", termo: t }
          : { busca: "nome_razaosocial", termo_busca: t };
      const res = await apiFetch<ClientSearchResponse>(path, { method: "POST", json: body });
      setResults(res.clients ?? []);
      if (!res.ok) toastErr(pushToast, new Error(res.message || "Falha na busca."));
    } catch (e) {
      toastErr(pushToast, e, "Falha ao consultar a integração.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="modal-backdrop modal-backdrop--stack" role="presentation" onMouseDown={onClose}>
      <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 480 }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>Buscar cliente na integração</h3>
        {integrationsQ.isLoading ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar integrações…</p>
        ) : enabled.length === 0 ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>Nenhuma integração activa configurada.</p>
        ) : (
          <>
            {enabled.length > 1 ? (
              <div className="field">
                <label>Integração</label>
                <select className="input" value={integrationId} onChange={(e) => { setIntegrationId(e.target.value); setResults(null); }}>
                  {enabled.map((it) => (
                    <option key={it.id} value={it.id}>
                      {it.name}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            <div className="field">
              <label>Nome do cliente</label>
              <div className="row" style={{ gap: 8 }}>
                <input
                  className="input"
                  style={{ flex: 1 }}
                  autoFocus
                  value={term}
                  onChange={(e) => setTerm(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void runSearch();
                  }}
                  placeholder="Ex.: João da Silva"
                />
                <button type="button" className="btn btn--primary" disabled={loading || !term.trim()} onClick={() => void runSearch()}>
                  {loading ? "A buscar…" : "Buscar"}
                </button>
              </div>
            </div>

            {results ? (
              results.length === 0 ? (
                <p style={{ fontSize: 12, color: "var(--muted)" }}>Nenhum cliente encontrado.</p>
              ) : (
                <div style={{ display: "flex", flexDirection: "column", gap: 4, maxHeight: 280, overflow: "auto" }}>
                  {results.map((c, i) => (
                    <button
                      key={c.id ?? c.code ?? i}
                      type="button"
                      className="btn"
                      style={{ justifyContent: "flex-start", textAlign: "left" }}
                      onClick={() => c.name && onPick(c.name)}
                    >
                      <div>
                        <div style={{ fontWeight: 600 }}>{c.name || "—"}</div>
                        <div className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
                          {c.code ? `Código: ${c.code}` : c.id ? `ID: ${c.id}` : "—"}
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              )
            ) : null}
          </>
        )}
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * Edição do vínculo cliente ↔ ONU de uma única linha (aba Pesquisa de ONUs) — mesmo padrão
 * visual do ConfirmModal, mas com um campo de texto para o nome do cliente. "Salvar" faz
 * upsert via POST /onu-client-links/import (1 linha); "Remover" chama
 * DELETE /onu-client-links/{serial}. O botão "Integração" busca o nome exacto do cliente numa
 * integração configurada (só nome + ID, sem trazer o cadastro completo) em vez de digitar à mão.
 */
export function OltOnuClientEditModal({ open, serial, currentName, busy, onCancel, onSave, onRemove }: Props) {
  const [name, setName] = useState(currentName);
  const [searchOpen, setSearchOpen] = useState(false);

  useEffect(() => {
    if (open) setName(currentName);
  }, [open, currentName]);

  if (!open) return null;
  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onCancel}>
      <div className="modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <h3>Editar cliente da ONU</h3>
        <p style={{ color: "var(--muted)", fontSize: 12, marginBottom: 10 }}>
          Serial: <span className="mono">{serial}</span>
        </p>
        <div className="field">
          <label>Nome do cliente</label>
          <div className="row" style={{ gap: 8 }}>
            <input
              className="input"
              style={{ flex: 1 }}
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Ex.: João da Silva"
            />
            <button type="button" className="btn btn--sm" title="Buscar na integração" onClick={() => setSearchOpen(true)}>
              <Blocks size={13} style={{ marginRight: 4, verticalAlign: -2 }} />
              Integração
            </button>
          </div>
        </div>
        <div className="row" style={{ justifyContent: "flex-end", marginTop: 12, flexWrap: "wrap", gap: 8 }}>
          <button type="button" className="btn" disabled={busy} onClick={onCancel}>
            Cancelar
          </button>
          {currentName.trim() ? (
            <button type="button" className="btn btn--danger" disabled={busy} onClick={onRemove}>
              {busy ? "…" : "Remover"}
            </button>
          ) : null}
          <button
            type="button"
            className="btn btn--primary"
            disabled={busy || !name.trim()}
            onClick={() => onSave(name.trim())}
          >
            {busy ? "…" : "Salvar"}
          </button>
        </div>
      </div>

      {searchOpen ? (
        <IntegrationClientSearch
          initialTerm={name}
          onClose={() => setSearchOpen(false)}
          onPick={(picked) => {
            setName(picked);
            setSearchOpen(false);
          }}
        />
      ) : null}
    </div>,
    document.body,
  );
}
