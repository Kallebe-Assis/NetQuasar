import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Plug, Plus, Search, Settings, Trash2 } from "lucide-react";
import { ConfirmModal } from "../components/ConfirmModal";
import { InfoHint } from "../components/InfoHint";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { queryKeys } from "../lib/queryKeys";
import type { IntegrationSummary } from "../integrations/types";

export function IntegrationsHubPage() {
  const qc = useQueryClient();
  const nav = useNavigate();
  const admin = isAdminUser();
  const [showNew, setShowNew] = useState(false);
  const [newName, setNewName] = useState("");
  const [newUrl, setNewUrl] = useState("https://");
  const [newDesc, setNewDesc] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<IntegrationSummary | null>(null);

  const listQ = useQuery({
    queryKey: queryKeys.integrations,
    queryFn: () => apiFetch<{ integrations: IntegrationSummary[] }>("/api/v1/integrations"),
  });

  const createM = useMutation({
    mutationFn: () =>
      apiFetch<{ id: string; slug: string }>("/api/v1/integrations", {
        method: "POST",
        json: { name: newName.trim(), base_url: newUrl.trim(), description: newDesc.trim() || undefined, auth_type: "none" },
      }),
    onSuccess: (data) => {
      void qc.invalidateQueries({ queryKey: queryKeys.integrations });
      setShowNew(false);
      setNewName("");
      setNewUrl("https://");
      setNewDesc("");
      nav(`/integrations/${data.slug}/config`);
    },
  });

  const deleteM = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/integrations/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.integrations });
      setDeleteTarget(null);
    },
  });

  const items = listQ.data?.integrations ?? [];

  return (
    <div>
      <h1 style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <Plug size={22} strokeWidth={2} />
        Integrações
        <InfoHint label="Sobre integrações">
          <p>
            Ligue sistemas externos por API REST. Cada integração tem a sua própria configuração: URL base, autenticação,
            requisições HTTP (GET com path params, query, variáveis) e testes de coleta.
          </p>
        </InfoHint>
      </h1>
      <p style={{ color: "var(--muted)", fontSize: 13, marginTop: 4 }}>
        Configure N sistemas; cada um abre numa página dedicada com requisições, login e execução de coleta.
      </p>

      {admin ? (
        <div className="row" style={{ marginTop: 16, gap: 8, flexWrap: "wrap" }}>
          <button type="button" className="btn btn--primary" onClick={() => setShowNew(true)}>
            <Plus size={16} style={{ marginRight: 6, verticalAlign: "middle" }} />
            Nova integração
          </button>
        </div>
      ) : (
        <p className="msg" style={{ marginTop: 12, fontSize: 12 }}>
          Apenas administradores podem criar ou eliminar integrações. Pode abrir e consultar as existentes.
        </p>
      )}

      {listQ.isError ? (
        <div className="msg msg--err" style={{ marginTop: 12 }}>
          {(listQ.error as Error).message}
        </div>
      ) : null}

      {listQ.isLoading ? <p style={{ marginTop: 20, color: "var(--muted)" }}>A carregar…</p> : null}

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
          gap: 12,
          marginTop: 20,
        }}
      >
        {items.map((it) => (
          <div
            key={it.id}
            className="panel"
            style={{
              padding: 14,
              display: "flex",
              flexDirection: "column",
              gap: 10,
              border: "1px solid var(--border)",
              borderRadius: "var(--radius)",
            }}
          >
            <div>
              <div style={{ fontWeight: 600, fontSize: 15 }}>{it.name}</div>
              {it.description ? (
                <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--muted)" }}>{it.description}</p>
              ) : null}
              <p className="mono" style={{ margin: "6px 0 0", fontSize: 11, color: "var(--muted)", wordBreak: "break-all" }}>
                {it.base_url}
              </p>
            </div>
            <div className="row" style={{ gap: 6, flexWrap: "wrap", alignItems: "center" }}>
              <span className={it.enabled ? "badge badge--ok" : "badge badge--off"}>{it.enabled ? "Ativa" : "Inativa"}</span>
              <span className="badge">{it.request_count} requisição(ões)</span>
              {it.last_test_ok === true ? <span className="badge badge--ok">Teste OK</span> : null}
              {it.last_test_ok === false ? <span className="badge badge--err">Teste falhou</span> : null}
            </div>
            <div className="row" style={{ gap: 8, marginTop: "auto", flexWrap: "wrap" }}>
              <Link
                to={`/integrations/${it.slug}/consulta`}
                className="btn btn--primary"
                style={{ flex: 1, minWidth: 100, textAlign: "center", textDecoration: "none", display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 6 }}
              >
                <Search size={14} /> Consultar
              </Link>
              {admin ? (
                <Link
                  to={`/integrations/${it.slug}/config`}
                  className="btn"
                  style={{ textDecoration: "none", display: "inline-flex", alignItems: "center", gap: 4 }}
                  title="Configuração API"
                >
                  <Settings size={14} />
                </Link>
              ) : null}
              {admin ? (
                <button
                  type="button"
                  className="btn btn--danger"
                  title="Eliminar integração"
                  onClick={() => setDeleteTarget(it)}
                >
                  <Trash2 size={16} />
                </button>
              ) : null}
            </div>
          </div>
        ))}
      </div>

      {!listQ.isLoading && items.length === 0 ? (
        <p style={{ marginTop: 24, color: "var(--muted)", textAlign: "center" }}>
          Nenhuma integração configurada. {admin ? "Clique em «Nova integração» para começar." : ""}
        </p>
      ) : null}

      {showNew ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !createM.isPending && setShowNew(false)}>
          <div className="modal" style={{ maxWidth: 480 }} role="dialog" onMouseDown={(e) => e.stopPropagation()}>
            <h3>Nova integração</h3>
            <div className="field">
              <label>Nome do sistema</label>
              <input className="input" value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="Ex.: ERP, CRM, Radius" />
            </div>
            <div className="field">
              <label>URL base da API</label>
              <input className="input mono" value={newUrl} onChange={(e) => setNewUrl(e.target.value)} placeholder="https://api.exemplo.com" />
            </div>
            <div className="field">
              <label>Descrição (opcional)</label>
              <textarea className="textarea" rows={2} value={newDesc} onChange={(e) => setNewDesc(e.target.value)} />
            </div>
            {createM.isError ? <div className="msg msg--err">{(createM.error as Error).message}</div> : null}
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 12 }}>
              <button type="button" className="btn" disabled={createM.isPending} onClick={() => setShowNew(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={createM.isPending || !newName.trim() || !newUrl.trim()}
                onClick={() => createM.mutate()}
              >
                {createM.isPending ? "A criar…" : "Criar e configurar"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Eliminar integração"
        message={
          deleteTarget
            ? `Eliminar «${deleteTarget.name}» e todos os requisições configurados? Esta acção não pode ser desfeita.`
            : ""
        }
        danger
        busy={deleteM.isPending}
        confirmLabel="Eliminar"
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteM.mutate(deleteTarget.id)}
      />
    </div>
  );
}
