import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { CheckCircle2, EyeOff, Plug, Plus, Search, Settings, SlidersHorizontal } from "lucide-react";
import { ActionMenu, type ActionMenuItem } from "../components/ActionMenu";
import { InfoHint } from "../components/InfoHint";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { isAdminUser } from "../lib/auth";
import { queryKeys } from "../lib/queryKeys";
import { useIntegrationLogo } from "../lib/integrationLogo";
import type { IntegrationSummary } from "../integrations/types";
import { APP_ROUTES } from "../app/routes";

function integrationTestPath(it: IntegrationSummary): string {
  return it.slug === "hubsoft" ? `/api/v1/integrations/${it.id}/hubsoft/test` : `/api/v1/integrations/${it.id}/test`;
}

/** Card de uma integração — logo à esquerda (por slug, ver integrationLogo.ts), nome clicável
 * (vai para a Consulta, tal como o botão), estado, e acções: Consultar / Configuração (admin) /
 * Ativar-Inativar (admin). Sem eliminar — só inativar (nunca perde a configuração). */
function IntegrationCard({
  it,
  admin,
  onToggleEnabled,
  togglingId,
}: {
  it: IntegrationSummary;
  admin: boolean;
  onToggleEnabled: (it: IntegrationSummary) => void;
  togglingId: string | null;
}) {
  const logo = useIntegrationLogo(it.slug);
  return (
    <div
      className="panel"
      style={{
        padding: 18,
        display: "flex",
        gap: 16,
        border: "1px solid var(--border)",
        borderRadius: "var(--radius)",
        opacity: it.enabled ? 1 : 0.6,
      }}
    >
      <div
        style={{
          width: 56,
          height: 56,
          flexShrink: 0,
          borderRadius: 10,
          background: "var(--panel2)",
          border: "1px solid var(--border)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          overflow: "hidden",
        }}
      >
        {logo ? (
          <img src={logo} alt="" style={{ width: "100%", height: "100%", objectFit: "contain" }} />
        ) : (
          <Plug size={26} style={{ color: "var(--muted)" }} />
        )}
      </div>

      <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 10 }}>
        <div>
          <Link
            to={APP_ROUTES.integrationConsulta(it.slug)}
            style={{ fontWeight: 600, fontSize: 17, textDecoration: "none", color: "var(--text)" }}
          >
            {it.name}
          </Link>
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
            to={APP_ROUTES.integrationConsulta(it.slug)}
            className="btn btn--primary"
            style={{ flex: 1, minWidth: 100, textAlign: "center", textDecoration: "none", display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 6 }}
          >
            <Search size={14} /> Consultar
          </Link>
          {admin ? (
            <Link
              to={APP_ROUTES.integrationConfig(it.slug)}
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
              className={it.enabled ? "btn" : "btn btn--primary"}
              title={it.enabled ? "Inativar integração" : "Ativar integração"}
              disabled={togglingId === it.id}
              onClick={() => onToggleEnabled(it)}
            >
              {it.enabled ? <EyeOff size={16} /> : <CheckCircle2 size={16} />}
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

export function IntegrationsHubPage() {
  const qc = useQueryClient();
  const nav = useNavigate();
  const { push: pushToast } = useAppToast();
  const admin = isAdminUser();
  const [showNew, setShowNew] = useState(false);
  const [newName, setNewName] = useState("");
  const [newUrl, setNewUrl] = useState("https://");
  const [newDesc, setNewDesc] = useState("");
  const [search, setSearch] = useState("");
  const [showInactive, setShowInactive] = useState(false);
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const [testingAll, setTestingAll] = useState(false);

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
      toastOk(pushToast, "Integração criada com sucesso.");
      nav(APP_ROUTES.integrationConfig(data.slug));
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao criar integração."),
  });

  const allItems = listQ.data?.integrations ?? [];

  const items = useMemo(() => {
    const q = search.trim().toLowerCase();
    return allItems.filter((it) => {
      if (!showInactive && !it.enabled) return false;
      if (!q) return true;
      return (
        it.name.toLowerCase().includes(q) ||
        (it.description ?? "").toLowerCase().includes(q) ||
        it.base_url.toLowerCase().includes(q)
      );
    });
  }, [allItems, search, showInactive]);

  async function toggleEnabled(it: IntegrationSummary) {
    setTogglingId(it.id);
    try {
      await apiFetch(`/api/v1/integrations/${it.id}`, { method: "PATCH", json: { enabled: !it.enabled } });
      toastOk(pushToast, it.enabled ? "Integração inativada." : "Integração ativada.");
      void qc.invalidateQueries({ queryKey: queryKeys.integrations });
    } catch (e) {
      toastErr(pushToast, e, "Falha ao alterar o estado.");
    } finally {
      setTogglingId(null);
    }
  }

  async function testAll() {
    if (allItems.length === 0) return;
    setTestingAll(true);
    let ok = 0;
    let failed = 0;
    const results = await Promise.allSettled(
      allItems.map((it) => apiFetch<{ ok: boolean }>(integrationTestPath(it), { method: "POST" })),
    );
    for (const r of results) {
      if (r.status === "fulfilled" && r.value?.ok) ok++;
      else failed++;
    }
    setTestingAll(false);
    void qc.invalidateQueries({ queryKey: queryKeys.integrations });
    if (failed === 0) {
      toastOk(pushToast, `Todas as ${ok} integração(ões) testadas com sucesso.`);
    } else {
      toastErr(pushToast, new Error(`${ok} OK, ${failed} falharam.`), "Teste de todas as APIs concluído com falhas");
    }
  }

  const menuItems: ActionMenuItem[] = [
    {
      id: "test-all",
      label: testingAll ? "A testar todas…" : "Testar todas as API",
      onClick: () => void testAll(),
      disabled: testingAll || allItems.length === 0,
    },
    {
      id: "toggle-inactive",
      label: showInactive ? "Ocultar integrações inativas" : "Mostrar integrações inativas",
      onClick: () => setShowInactive((v) => !v),
    },
  ];

  return (
    <div>
      <h1 style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <Plug size={22} strokeWidth={2} />
        Integrações
        <InfoHint label="Sobre integrações">
          <p>
            Ligue sistemas externos por API REST. Cada integração tem a sua própria configuração: URL base, autenticação,
            requisições HTTP (GET com path params, query, variáveis) e testes de coleta. Integrações não podem ser
            eliminadas — só inativadas (a configuração fica guardada e pode ser reativada a qualquer momento).
          </p>
        </InfoHint>
      </h1>
      <p style={{ color: "var(--muted)", fontSize: 13, marginTop: 4 }}>
        Configure N sistemas; cada um abre numa página dedicada com requisições, login e execução de coleta.
      </p>

      <div className="row" style={{ marginTop: 16, gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        {admin ? (
          <button type="button" className="btn btn--primary" onClick={() => setShowNew(true)}>
            <Plus size={16} style={{ marginRight: 6, verticalAlign: "middle" }} />
            Nova integração
          </button>
        ) : null}
        <input
          className="input"
          style={{ flex: 1, minWidth: 200, maxWidth: 420 }}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Pesquisar por nome, descrição ou URL…"
          aria-label="Pesquisar integrações"
        />
        <ActionMenu items={menuItems} title="Opções" icon={<SlidersHorizontal size={16} />} />
      </div>

      {!admin ? (
        <p className="msg" style={{ marginTop: 12, fontSize: 12 }}>
          Apenas administradores podem criar, configurar ou inativar integrações. Pode abrir e consultar as existentes.
        </p>
      ) : null}

      {listQ.isError ? (
        <div className="msg msg--err" style={{ marginTop: 12 }}>
          {(listQ.error as Error).message}
        </div>
      ) : null}

      {listQ.isLoading ? <p style={{ marginTop: 20, color: "var(--muted)" }}>A carregar…</p> : null}

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
          gap: 16,
          marginTop: 20,
        }}
      >
        {items.map((it) => (
          <IntegrationCard key={it.id} it={it} admin={admin} onToggleEnabled={(x) => void toggleEnabled(x)} togglingId={togglingId} />
        ))}
      </div>

      {!listQ.isLoading && items.length === 0 ? (
        <p style={{ marginTop: 24, color: "var(--muted)", textAlign: "center" }}>
          {allItems.length === 0
            ? `Nenhuma integração configurada.${admin ? " Clique em «Nova integração» para começar." : ""}`
            : "Nenhuma integração corresponde à pesquisa/filtro actual."}
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
    </div>
  );
}
