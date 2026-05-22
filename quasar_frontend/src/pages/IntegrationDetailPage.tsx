import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { Braces, Play, Plus, Save, Trash2, X, Zap } from "lucide-react";
import { ConfirmModal } from "../components/ConfirmModal";
import { IntegrationNav } from "../components/IntegrationNav";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { queryKeys } from "../lib/queryKeys";
import {
  AUTH_TYPES,
  AUTH_TOKEN_PREFIXES,
  CLIENT_SEARCH_PROVIDERS,
  TERMO_FORMAT_OPTIONS,
  HTTP_METHODS,
  normalizeAuthTokenPrefix,
  type ClientSearchProvider,
  type ConsumerConfig,
  type SearchFieldMapping,
  type IntegrationDetail,
  type IntegrationRequest,
  type OAuth2PasswordAuthConfig,
  type PathParam,
  type QueryParam,
  type IntegrationRunLog,
  type RunResult,
  parseHeaders,
  parsePathParams,
  parseQueryParams,
} from "../integrations/types";
import { PageToastHost, usePageToast } from "../lib/pageToast";

type Tab = "operacao" | "geral" | "auth" | "requests" | "testes";

const emptyRequest = (): Omit<IntegrationRequest, "id" | "integration_id" | "last_run_at" | "last_run_ok" | "last_run_status" | "last_run_message"> => ({
  name: "Nova requisição",
  method: "GET",
  path: "/",
  path_params: [],
  query_params: [],
  headers: {},
  body_template: "",
  body_type: "json",
  extract_json_path: "",
  is_login: false,
  sort_order: 0,
  enabled: true,
});

export function IntegrationDetailPage() {
  const { slug } = useParams<{ slug: string }>();
  const qc = useQueryClient();
  const admin = isAdminUser();
  const [tab, setTab] = useState<Tab>("operacao");
  const { toast, show: showToast, dismiss: dismissToast } = usePageToast();
  const [editingReq, setEditingReq] = useState<IntegrationRequest | null>(null);
  const [deleteReqId, setDeleteReqId] = useState<string | null>(null);
  const [lastRun, setLastRun] = useState<RunResult | null>(null);
  const [runAllResults, setRunAllResults] = useState<RunResult[] | null>(null);
  const [showVarsModal, setShowVarsModal] = useState(false);
  const [varsDraft, setVarsDraft] = useState<Record<string, string>>({});
  const [runPanel, setRunPanel] = useState<{
    result: RunResult;
    requestName: string;
    requestId: string;
  } | null>(null);

  const detailQ = useQuery({
    queryKey: queryKeys.integrationDetail(slug ?? ""),
    queryFn: () => apiFetch<IntegrationDetail>(`/api/v1/integrations/${slug}`),
    enabled: !!slug,
  });

  const d = detailQ.data;
  const [form, setForm] = useState<Partial<IntegrationDetail>>({});

  useEffect(() => {
    if (d) {
      setForm({
        name: d.name,
        description: d.description ?? "",
        base_url: d.base_url,
        enabled: d.enabled,
        auth_type: d.auth_type,
        auth_config: { ...d.auth_config },
        default_headers: parseHeaders(d.default_headers),
        variables: parseVariablesRecord(d.variables),
        consumer_config: parseConsumerConfig(d.consumer_config),
        timeout_ms: d.timeout_ms,
        tls_insecure: d.tls_insecure,
      });
    }
  }, [d]);

  const saveM = useMutation({
    mutationFn: async () => {
      if (!slug || !d) return;
      const headersObj = form.default_headers ?? {};
      const varsObj = form.variables ?? {};
      const authCfg = { ...(form.auth_config as Record<string, unknown>) };
      return apiFetch(`/api/v1/integrations/${slug}`, {
        method: "PATCH",
        json: {
          name: form.name,
          description: (form.description as string) || null,
          base_url: form.base_url,
          enabled: form.enabled,
          auth_type: form.auth_type,
          default_headers: headersObj,
          variables: varsObj,
          consumer_config: form.consumer_config ?? { client_search: { enabled: false } },
          auth_config: authCfg,
          timeout_ms: form.timeout_ms,
          tls_insecure: form.tls_insecure,
        },
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      void qc.invalidateQueries({ queryKey: queryKeys.integrations });
      showToast("ok", "Guardado com sucesso.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const testM = useMutation({
    mutationFn: () => apiFetch<RunResult>(`/api/v1/integrations/${slug}/test`, { method: "POST" }),
    onSuccess: (r) => {
      setLastRun(r);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      showToast(r.ok ? "ok" : "err", r.ok ? "Teste de conexão OK." : r.error || "Teste falhou.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const loginM = useMutation({
    mutationFn: () => apiFetch<RunResult>(`/api/v1/integrations/${slug}/login`, { method: "POST" }),
    onSuccess: (r) => {
      setLastRun(r);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      showToast(
        r.ok ? "ok" : "err",
        r.token_received
          ? "Token obtido e salvo na sessão."
          : r.ok
            ? r.status_code
              ? `Requisição de teste OK (HTTP ${r.status_code}).`
              : "Autenticação configurada."
            : r.error || "Falha na autenticação.",
      );
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const runAllM = useMutation({
    mutationFn: () => apiFetch<{ results: RunResult[] }>(`/api/v1/integrations/${slug}/run-all`, { method: "POST" }),
    onSuccess: (r) => {
      setRunAllResults(r.results ?? []);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      showToast("ok", `Colecta concluída: ${r.results?.length ?? 0} requisição(ões).`);
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const saveReqM = useMutation({
    mutationFn: async (req: IntegrationRequest) => {
      const payload = {
        name: req.name,
        description: req.description,
        method: req.method,
        path: req.path,
        path_params: req.path_params,
        query_params: req.query_params,
        headers: req.headers,
        body_template: req.body_template || null,
        body_type: req.body_type,
        extract_json_path: req.extract_json_path || null,
        is_login: req.is_login,
        sort_order: req.sort_order,
        enabled: req.enabled,
      };
      if (req.id.startsWith("new-")) {
        return apiFetch<{ id: string }>(`/api/v1/integrations/${slug}/requests`, { method: "POST", json: payload });
      }
      return apiFetch(`/api/v1/integrations/${slug}/requests/${req.id}`, { method: "PATCH", json: payload });
    },
    onSuccess: () => {
      setEditingReq(null);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      showToast("ok", "Requisição salva com sucesso.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const deleteReqM = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/integrations/${slug}/requests/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDeleteReqId(null);
      setEditingReq(null);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      showToast("ok", "Requisição eliminada.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const runReqM = useMutation({
    mutationFn: ({ id }: { id: string; name: string }) =>
      apiFetch<RunResult>(`/api/v1/integrations/${slug}/requests/${id}/run`, { method: "POST" }),
    onSuccess: (r, { id, name }) => {
      const result = { ...r, request_id: r.request_id ?? id, request_name: r.request_name ?? name };
      setLastRun(result);
      setRunPanel({ result, requestName: name, requestId: id });
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      void qc.invalidateQueries({ queryKey: queryKeys.integrationLogs(slug ?? "", id) });
      showToast(result.ok ? "ok" : "err", result.ok ? `Requisição «${name}» executada.` : result.error || "Requisição falhou.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const saveVarsM = useMutation({
    mutationFn: async (variables: Record<string, string>) => {
      if (!slug) return;
      return apiFetch(`/api/v1/integrations/${slug}`, {
        method: "PATCH",
        json: { variables },
      });
    },
    onSuccess: (_data, variables) => {
      setForm((f) => ({ ...f, variables }));
      setShowVarsModal(false);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      showToast("ok", "Variáveis globais salvas.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const logsQ = useQuery({
    queryKey: queryKeys.integrationLogs(slug ?? "", runPanel?.requestId),
    queryFn: () => {
      const q = runPanel?.requestId ? `?request_id=${encodeURIComponent(runPanel.requestId)}&limit=15` : "?limit=50";
      return apiFetch<{ logs: IntegrationRunLog[] }>(`/api/v1/integrations/${slug}/logs${q}`);
    },
    enabled: !!slug && (tab === "testes" || !!runPanel),
  });

  const requests = useMemo(() => {
    if (!d?.requests) return [];
    return [...d.requests].sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name));
  }, [d?.requests]);

  const authCfg = (form.auth_config ?? {}) as OAuth2PasswordAuthConfig & Record<string, string>;
  const isOAuth2 = form.auth_type === "oauth2_password";
  const isLoginFlowAuth =
    form.auth_type === "oauth2_password" || form.auth_type === "login";
  const authTestLabel = isLoginFlowAuth ? "Testar login" : "Testar autenticação";
  const oauthKeysInHeaders = Object.keys((form.default_headers as Record<string, string>) ?? {}).filter((k) =>
    /^(client_id|client_secret|username|password|grant_type)$/i.test(k.trim()),
  );

  if (detailQ.isLoading) return <p style={{ padding: 24, color: "var(--muted)" }}>A carregar integração…</p>;
  if (detailQ.isError || !d) {
    return (
      <div style={{ padding: 24 }}>
        <Link to="/integrations" className="btn">
          <ArrowLeft size={14} /> Voltar
        </Link>
        <p className="msg msg--err" style={{ marginTop: 12 }}>
          {(detailQ.error as Error)?.message || "Integração não encontrada."}
        </p>
      </div>
    );
  }

  const consumerCfg = (form.consumer_config ?? { client_search: { enabled: false } }) as ConsumerConfig;

  return (
    <div>
      <IntegrationNav slug={slug!} name={d.name} />
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 8 }}>
        <span className={d.enabled ? "badge badge--ok" : "badge badge--off"}>{d.enabled ? "Ativa" : "Inativa"}</span>
        {d.session_active ? <span className="badge">Sessão ativa</span> : null}
        {consumerCfg.client_search?.enabled ? <span className="badge badge--ok">Consulta ativa</span> : null}
        {consumerCfg.client_attendance?.enabled || consumerCfg.client_work_order?.enabled ? (
          <span className="badge badge--ok">Atend. / O.S. ativos</span>
        ) : null}
      </div>

      <PageToastHost toast={toast} onDismiss={dismissToast} />

      <p style={{ fontSize: 12, color: "var(--muted)", marginBottom: 12 }}>
        Configuração técnica da API (URL, autenticação, requisições HTTP). Usuários do NOC usam o separador{" "}
        <strong>Consulta</strong> para pesquisar clientes.
      </p>

      <div className="tabs" style={{ marginTop: 8, flexWrap: "wrap" }}>
        {(
          [
            ["operacao", "Operação (UI)"],
            ["geral", "Geral"],
            ["auth", "Autenticação"],
            ["requests", "Requisições HTTP"],
            ["testes", "Testes e logs"],
          ] as const
        ).map(([k, lab]) => (
          <button key={k} type="button" className={tab === k ? "active" : ""} onClick={() => setTab(k)}>
            {lab}
          </button>
        ))}
      </div>

      {tab === "operacao" && (
        <section className="panel" style={{ marginTop: 16, padding: 16 }}>
          <h2 style={{ marginTop: 0, fontSize: 16 }}>Operação para usuários</h2>
          <p style={{ fontSize: 12, color: "var(--muted)" }}>
            Define o que aparece no separador <strong>Consulta</strong>. O NetQuasar adapta parâmetros e parser conforme o{" "}
            <strong>ERP</strong>: Hubsoft (GET <code className="mono">/integracao/cliente</code>), IXC (POST{" "}
            <code className="mono">/cliente</code> + header <code className="mono">ixcsoft: listar</code> — GET com query não é suportado pelo IXC), ou genérico (variáveis{" "}
            <code className="mono">{"{{termo_busca}}"}</code> na requisição).
          </p>
          <label className="row" style={{ gap: 8, marginTop: 12 }}>
            <input
              type="checkbox"
              disabled={!admin}
              checked={!!consumerCfg.client_search?.enabled}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_search: {
                      ...consumerCfg.client_search,
                      enabled: e.target.checked,
                    },
                  },
                }))
              }
            />
            Ativar consulta de cliente na UI
          </label>
          <div className="field" style={{ maxWidth: 420, marginTop: 12 }}>
            <label>Requisição HTTP de consulta</label>
            <select
              className="input"
              disabled={!admin}
              value={consumerCfg.client_search?.request_id ?? ""}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_search: {
                      ...consumerCfg.client_search,
                      enabled: consumerCfg.client_search?.enabled ?? false,
                      request_id: e.target.value || undefined,
                    },
                  },
                }))
              }
            >
              <option value="">Detectar automaticamente (Hubsoft ou IXC /cliente)</option>
              {requests.map((req) => (
                <option key={req.id} value={req.id}>
                  {req.name} — {req.method} {req.path}
                </option>
              ))}
            </select>
          </div>
          <div className="field" style={{ maxWidth: 420, marginTop: 12 }}>
            <label>ERP / formato da consulta</label>
            <select
              className="input"
              disabled={!admin}
              value={consumerCfg.client_search?.provider ?? "auto"}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_search: {
                      ...consumerCfg.client_search,
                      enabled: consumerCfg.client_search?.enabled ?? false,
                      request_id: consumerCfg.client_search?.request_id,
                      provider: e.target.value as ClientSearchProvider,
                    },
                  },
                }))
              }
            >
              {CLIENT_SEARCH_PROVIDERS.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label}
                </option>
              ))}
            </select>
          </div>

          <details style={{ marginTop: 16, fontSize: 13 }}>
            <summary style={{ cursor: "pointer", fontWeight: 600 }}>Avançado — IXC / mapeamento de busca</summary>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "8px 0" }}>
              Deixe em branco para usar os padrões do NetQuasar. Preencha só o que a sua API exigir de diferente.
            </p>
            <div className="field" style={{ maxWidth: 280, marginTop: 8 }}>
              <label>Header ixcsoft (listagem)</label>
              <input
                className="input mono"
                disabled={!admin}
                placeholder="listar"
                value={consumerCfg.client_search?.ixc_list_action ?? ""}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    consumer_config: {
                      ...consumerCfg,
                      client_search: { ...consumerCfg.client_search, ixc_list_action: e.target.value || undefined },
                    },
                  }))
                }
              />
            </div>
            <label className="row" style={{ gap: 8, marginTop: 10 }}>
              <input
                type="checkbox"
                disabled={!admin}
                checked={consumerCfg.client_search?.cpf_multi_attempt !== false}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    consumer_config: {
                      ...consumerCfg,
                      client_search: { ...consumerCfg.client_search, cpf_multi_attempt: e.target.checked },
                    },
                  }))
                }
              />
              CPF/CNPJ: tentar várias combinações qtype/oper (recomendado)
            </label>
            <FieldMappingsEditor
              mappings={consumerCfg.client_search?.field_mappings ?? {}}
              disabled={!admin}
              onChange={(field_mappings) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_search: { ...consumerCfg.client_search, field_mappings },
                  },
                }))
              }
            />
          </details>

          <hr style={{ margin: "20px 0", border: "none", borderTop: "1px solid var(--border)" }} />

          <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>Atendimentos e ordens de serviço</h3>
          <p style={{ fontSize: 12, color: "var(--muted)" }}>
            No menu ⋮ dos resultados abre um modal com abas. O sistema preenche <code className="mono">busca</code> e{" "}
            <code className="mono">termo_busca</code> a partir do cartão (código, CPF/CNPJ ou ID do serviço).
          </p>
          <label className="row" style={{ gap: 8, marginTop: 12 }}>
            <input
              type="checkbox"
              disabled={!admin}
              checked={!!consumerCfg.client_attendance?.enabled}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_attendance: {
                      ...consumerCfg.client_attendance,
                      enabled: e.target.checked,
                      request_id: consumerCfg.client_attendance?.request_id,
                    },
                  },
                }))
              }
            />
            Ativar aba Atendimentos (Hubsoft GET ou IXC POST <code className="mono">/su_ticket</code>)
          </label>
          <div className="field" style={{ maxWidth: 420, marginTop: 8 }}>
            <label>Requisição HTTP — atendimentos</label>
            <select
              className="input"
              disabled={!admin}
              value={consumerCfg.client_attendance?.request_id ?? ""}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_attendance: {
                      enabled: consumerCfg.client_attendance?.enabled ?? false,
                      request_id: e.target.value || undefined,
                    },
                  },
                }))
              }
            >
              <option value="">Detectar automaticamente (Hubsoft ou IXC /su_ticket)</option>
              {requests.map((req) => (
                <option key={req.id} value={req.id}>
                  {req.name} — {req.method} {req.path}
                </option>
              ))}
            </select>
          </div>
          <p className="msg" style={{ fontSize: 12, marginTop: 8 }}>
            <strong>IXC su_ticket:</strong> a requisição deve ser <strong>POST</strong> (não GET), path <code className="mono">/su_ticket</code>, header{" "}
            <code className="mono">ixcsoft: listar</code>, body JSON. O filtro usa o campo <code className="mono">id_cliente</code> do ticket (igual ao ID do
            cliente consultado). O mapeamento da consulta de clientes (tabela acima) também vale para atendimentos se não houver mapeamento próprio.
          </p>

          <label className="row" style={{ gap: 8, marginTop: 16 }}>
            <input
              type="checkbox"
              disabled={!admin}
              checked={!!consumerCfg.client_work_order?.enabled}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_work_order: {
                      ...consumerCfg.client_work_order,
                      enabled: e.target.checked,
                      request_id: consumerCfg.client_work_order?.request_id,
                    },
                  },
                }))
              }
            />
            Ativar aba Ordens de serviço (<code className="mono">…/cliente/ordem_servico</code>)
          </label>
          <div className="field" style={{ maxWidth: 420, marginTop: 8 }}>
            <label>Requisição HTTP — ordens de serviço</label>
            <select
              className="input"
              disabled={!admin}
              value={consumerCfg.client_work_order?.request_id ?? ""}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  consumer_config: {
                    ...consumerCfg,
                    client_work_order: {
                      enabled: consumerCfg.client_work_order?.enabled ?? false,
                      request_id: e.target.value || undefined,
                    },
                  },
                }))
              }
            >
              <option value="">Detectar automaticamente (GET …/integracao/cliente/ordem_servico)</option>
              {requests.map((req) => (
                <option key={req.id} value={req.id}>
                  {req.name} — {req.method} {req.path}
                </option>
              ))}
            </select>
          </div>
          {admin ? (
            <button type="button" className="btn btn--primary" style={{ marginTop: 12 }} disabled={saveM.isPending} onClick={() => saveM.mutate()}>
              Salvar operação
            </button>
          ) : null}
        </section>
      )}

      {tab === "geral" && (
        <section className="panel" style={{ marginTop: 16, padding: 16 }}>
          <h2 style={{ marginTop: 0, fontSize: 16 }}>Configuração geral</h2>
          <div className="field">
            <label>Nome</label>
            <input className="input" value={form.name ?? ""} disabled={!admin} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div className="field">
            <label>URL base</label>
            <input className="input mono" value={form.base_url ?? ""} disabled={!admin} onChange={(e) => setForm((f) => ({ ...f, base_url: e.target.value }))} />
          </div>
          <div className="field">
            <label>Descrição</label>
            <textarea className="textarea" rows={2} disabled={!admin} value={(form.description as string) ?? ""} onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))} />
          </div>
          <div className="row" style={{ flexWrap: "wrap", gap: 12 }}>
            <label className="row" style={{ gap: 6 }}>
              <input type="checkbox" disabled={!admin} checked={!!form.enabled} onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))} />
              Integração ativa
            </label>
            <label className="row" style={{ gap: 6 }}>
              <input type="checkbox" disabled={!admin} checked={!!form.tls_insecure} onChange={(e) => setForm((f) => ({ ...f, tls_insecure: e.target.checked }))} />
              TLS inseguro (diagnóstico)
            </label>
          </div>
          <div className="field" style={{ maxWidth: 160 }}>
            <label>Timeout (ms)</label>
            <input className="input mono" type="number" disabled={!admin} value={form.timeout_ms ?? 15000} onChange={(e) => setForm((f) => ({ ...f, timeout_ms: Number(e.target.value) }))} />
          </div>
          {oauthKeysInHeaders.length > 0 ? (
            <div className="msg msg--err" style={{ marginTop: 12, fontSize: 12 }}>
              Remova das <strong>Headers por defeito</strong>: {oauthKeysInHeaders.join(", ")}. Esses dados não são cabeçalhos HTTP —
              configure-os na aba <strong>Autenticação</strong> (tipo OAuth2) ou em <strong>Variáveis globais</strong> se usar login manual.
            </div>
          ) : null}
          {!isOAuth2 ? (
            <>
              <VariablesEditor
                disabled={!admin}
                variables={(form.variables as Record<string, string>) ?? {}}
                onChange={(variables) => setForm((f) => ({ ...f, variables }))}
              />
              <HeadersEditor
                disabled={!admin}
                title="Headers por defeito"
                headers={(form.default_headers as Record<string, string>) ?? {}}
                onChange={(default_headers) => setForm((f) => ({ ...f, default_headers }))}
              />
            </>
          ) : (
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 12 }}>
              Com OAuth2, o token é aplicado automaticamente nos requisições. Headers e variáveis extra só são necessários se a API
              exigir algo além do <code className="mono">Authorization: Bearer …</code> — configure na aba Autenticação ou nos
              requisições HTTP.
            </p>
          )}
          {admin ? (
            <button type="button" className="btn btn--primary" style={{ marginTop: 12 }} disabled={saveM.isPending} onClick={() => saveM.mutate()}>
              <Save size={14} style={{ marginRight: 6 }} />
              {saveM.isPending ? "A salvar…" : "Salvar"}
            </button>
          ) : null}
        </section>
      )}

      {tab === "auth" && (
        <section className="panel" style={{ marginTop: 16, padding: 16 }}>
          <h2 style={{ marginTop: 0, fontSize: 16 }}>Autenticação</h2>
          <div className="field" style={{ maxWidth: 360 }}>
            <label>Tipo</label>
            <select
              className="input"
              disabled={!admin}
              value={form.auth_type ?? "none"}
              onChange={(e) => {
                const auth_type = e.target.value;
                setForm((f) => {
                  const next = { ...f, auth_type };
                  if (auth_type === "oauth2_password") {
                    const ac = { ...(f.auth_config as OAuth2PasswordAuthConfig) };
                    if (!ac.login_path?.trim()) ac.login_path = "/oauth/token";
                    if (!ac.grant_type?.trim()) ac.grant_type = "password";
                    if (!ac.token_json_path?.trim()) ac.token_json_path = "access_token";
                    if (!ac.token_prefix?.trim()) ac.token_prefix = "Bearer";
                    if (!ac.login_method?.trim()) ac.login_method = "POST";
                    if (!ac.login_body_type) ac.login_body_type = "form";
                    next.auth_config = ac;
                  }
                  return next;
                });
              }}
            >
              {AUTH_TYPES.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.label}
                </option>
              ))}
            </select>
          </div>
          {form.auth_type === "oauth2_password" && (
            <>
              <div className="msg" style={{ marginBottom: 12, fontSize: 12 }}>
                <strong>Não coloque client_id, password, etc. em «Headers por defeito».</strong> Esses valores vão no corpo do POST
                (como no Postman). Headers só para coisas como <code className="mono">Accept</code>.
              </div>
              <p style={{ fontSize: 12, color: "var(--muted)", marginBottom: 12 }}>
                Preencha os campos abaixo (iguais ao Postman). Por defeito a requisição usa{" "}
                <code className="mono">application/x-www-form-urlencoded</code>, formato habitual em <code className="mono">/oauth/token</code>.
              </p>
              <div className="field">
                <label>Caminho do token (relativo à URL base)</label>
                <input
                  className="input mono"
                  disabled={!admin}
                  value={authCfg.login_path ?? "/oauth/token"}
                  placeholder="/oauth/token"
                  onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, login_path: e.target.value } }))}
                />
              </div>
              <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
                <div className="field" style={{ flex: 1, minWidth: 200 }}>
                  <label>Client ID</label>
                  <input
                    className="input mono"
                    disabled={!admin}
                    value={authCfg.client_id ?? ""}
                    onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, client_id: e.target.value } }))}
                  />
                </div>
                <div className="field" style={{ flex: 1, minWidth: 200 }}>
                  <label>
                    Client secret{" "}
                    {authCfg.client_secret_configured ? <span style={{ color: "var(--muted)", fontWeight: 400 }}>(já configurado — vazio mantém)</span> : null}
                  </label>
                  <input
                    className="input mono"
                    type="password"
                    disabled={!admin}
                    placeholder={authCfg.client_secret_configured ? "••••••" : ""}
                    onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, client_secret: e.target.value } }))}
                  />
                </div>
              </div>
              <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
                <div className="field" style={{ flex: 1, minWidth: 200 }}>
                  <label>Username</label>
                  <input
                    className="input"
                    disabled={!admin}
                    value={authCfg.username ?? ""}
                    onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, username: e.target.value } }))}
                  />
                </div>
                <div className="field" style={{ flex: 1, minWidth: 200 }}>
                  <label>
                    Password{" "}
                    {authCfg.password_configured ? <span style={{ color: "var(--muted)", fontWeight: 400 }}>(já configurada — vazio mantém)</span> : null}
                  </label>
                  <input
                    className="input"
                    type="password"
                    disabled={!admin}
                    placeholder={authCfg.password_configured ? "••••••" : ""}
                    onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, password: e.target.value } }))}
                  />
                </div>
              </div>
              <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
                <div className="field" style={{ maxWidth: 240 }}>
                  <label>Grant type</label>
                  <input
                    className="input mono"
                    disabled={!admin}
                    value={authCfg.grant_type ?? "password"}
                    onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, grant_type: e.target.value } }))}
                  />
                </div>
                <div className="field" style={{ maxWidth: 280 }}>
                  <label>Formato do body (como no Postman)</label>
                  <select
                    className="input"
                    disabled={!admin}
                    value={authCfg.login_body_type ?? "form"}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        auth_config: { ...authCfg, login_body_type: e.target.value as "json" | "form" },
                      }))
                    }
                  >
                    <option value="form">Form urlencoded (recomendado)</option>
                    <option value="json">JSON</option>
                  </select>
                </div>
              </div>
              <div className="field">
                <label>Campo do token na resposta JSON</label>
                <input
                  className="input mono"
                  disabled={!admin}
                  value={authCfg.token_json_path ?? "access_token"}
                  onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, token_json_path: e.target.value } }))}
                />
              </div>
              <label className="row" style={{ gap: 8 }}>
                <input
                  type="checkbox"
                  disabled={!admin}
                  checked={!!authCfg.token_encode_base64}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      auth_config: { ...authCfg, token_encode_base64: e.target.checked },
                    }))
                  }
                />
                Codificar token em Base64 no header Authorization
              </label>
            </>
          )}
          {form.auth_type === "bearer" && (
            <>
              <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 10px" }}>
                Token fixo (ex. IXC: <code className="mono">id:hash</code> no painel). «Testar autenticação» executa a primeira requisição HTTP activa.
              </p>
              <div className="field">
                <label>Token {d.token_configured ? "(já configurado — deixe vazio para manter)" : ""}</label>
                <input
                  className="input mono"
                  disabled={!admin}
                  placeholder="Ex.: 2:121e78ce… ou Base64 já codificado"
                  onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, token: e.target.value } }))}
                />
              </div>
              <label className="row" style={{ gap: 8, marginBottom: 12 }}>
                <input
                  type="checkbox"
                  disabled={!admin}
                  checked={!!authCfg.token_encode_base64}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      auth_config: { ...authCfg, token_encode_base64: e.target.checked },
                    }))
                  }
                />
                Codificar token em Base64 ao enviar (ex.: IXC com prefixo Basic e token <code className="mono">id:hash</code>)
              </label>
              <div className="field" style={{ maxWidth: 360 }}>
                <label>Prefixo do Authorization</label>
                <select
                  className="input"
                  disabled={!admin}
                  value={normalizeAuthTokenPrefix(authCfg.token_prefix)}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      auth_config: { ...authCfg, token_prefix: e.target.value },
                    }))
                  }
                >
                  {AUTH_TOKEN_PREFIXES.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.label}
                    </option>
                  ))}
                </select>
              </div>
            </>
          )}
          {form.auth_type === "basic" && (
            <>
              <div className="field">
                <label>Usuário</label>
                <input className="input" disabled={!admin} defaultValue={authCfg.username} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, username: e.target.value } }))} />
              </div>
              <div className="field">
                <label>Senha {d.password_configured ? "(deixe vazio para manter)" : ""}</label>
                <input className="input" type="password" disabled={!admin} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, password: e.target.value } }))} />
              </div>
            </>
          )}
          {form.auth_type === "api_key" && (
            <>
              <div className="field">
                <label>Nome do header</label>
                <input className="input mono" disabled={!admin} defaultValue={authCfg.header_name || "X-API-Key"} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, header_name: e.target.value } }))} />
              </div>
              <div className="field">
                <label>Chave API</label>
                <input className="input mono" type="password" disabled={!admin} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, api_key: e.target.value } }))} />
              </div>
            </>
          )}
          {form.auth_type === "login" && (
            <>
              <div className="msg msg--err" style={{ marginBottom: 12, fontSize: 12 }}>
                Credenciais em <strong>Variáveis globais</strong> (aba Geral), não em Headers. Cada{" "}
                <code className="mono">{"{{client_id}}"}</code> precisa de uma variável com o mesmo nome e valor real. Prefira o tipo{" "}
                <strong>OAuth2 — obter token</strong> para evitar este modo manual.
              </div>
              <p style={{ fontSize: 12, color: "var(--muted)" }}>
                Opção A: marque uma requisição HTTP como «Login». Opção B: preencha o caminho de login abaixo.
              </p>
              <div className="field">
                <label>Caminho login (relativo à URL base)</label>
                <input className="input mono" disabled={!admin} defaultValue={authCfg.login_path || "/api/login"} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, login_path: e.target.value } }))} />
              </div>
              <div className="field">
                <label>Método</label>
                <select className="input" disabled={!admin} defaultValue={authCfg.login_method || "POST"} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, login_method: e.target.value } }))}>
                  {HTTP_METHODS.map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field" style={{ maxWidth: 280 }}>
                <label>Formato do body</label>
                <select
                  className="input"
                  disabled={!admin}
                  value={authCfg.login_body_type ?? "json"}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      auth_config: { ...authCfg, login_body_type: e.target.value as "json" | "form" },
                    }))
                  }
                >
                  <option value="form">Form urlencoded</option>
                  <option value="json">JSON</option>
                </select>
              </div>
              <div className="field">
                <label>
                  {authCfg.login_body_type === "form" ? "Body form (uma linha: chave=valor, use {{variavel}})" : "Body JSON (suporta {{variavel}})"}
                </label>
                <textarea
                  className="textarea mono"
                  rows={5}
                  disabled={!admin}
                  value={
                    authCfg.login_body ??
                    (authCfg.login_body_type === "form"
                      ? "client_id={{client_id}}\nclient_secret={{client_secret}}\nusername={{username}}\npassword={{password}}\ngrant_type={{grant_type}}"
                      : '{"client_id":"{{client_id}}","client_secret":"{{client_secret}}","username":"{{username}}","password":"{{password}}","grant_type":"{{grant_type}}"}')
                  }
                  onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, login_body: e.target.value } }))}
                />
              </div>
              <div className="field">
                <label>Caminho JSON do token (ex. access_token)</label>
                <input className="input mono" disabled={!admin} defaultValue={authCfg.token_json_path || "access_token"} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, token_json_path: e.target.value } }))} />
              </div>
            </>
          )}
          {admin ? (
            <div className="row" style={{ gap: 8, marginTop: 12, flexWrap: "wrap" }}>
              <button type="button" className="btn btn--primary" disabled={saveM.isPending} onClick={() => saveM.mutate()}>
                Salvar auth
              </button>
              <button type="button" className="btn" disabled={loginM.isPending} onClick={() => loginM.mutate()}>
                {loginM.isPending ? "A testar…" : authTestLabel}
              </button>
            </div>
          ) : null}
        </section>
      )}

      {tab === "requests" && (
        <section style={{ marginTop: 16 }}>
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 12, flexWrap: "wrap", gap: 8 }}>
            <h2 style={{ margin: 0, fontSize: 16 }}>Requisições de coleta</h2>
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <button
                type="button"
                className="btn"
                title="Variáveis globais ({{nome}})"
                onClick={() => {
                  setVarsDraft({ ...((form.variables as Record<string, string>) ?? {}) });
                  setShowVarsModal(true);
                }}
              >
                <Braces size={14} style={{ marginRight: 4 }} />
                Variáveis
                {Object.keys((form.variables as Record<string, string>) ?? {}).length > 0 ? (
                  <span className="badge" style={{ marginLeft: 6 }}>
                    {Object.keys((form.variables as Record<string, string>) ?? {}).length}
                  </span>
                ) : null}
              </button>
              {admin ? (
                <button
                  type="button"
                  className="btn btn--primary"
                  onClick={() =>
                    setEditingReq({
                      ...emptyRequest(),
                      id: `new-${Date.now()}`,
                      integration_id: d.id,
                      path_params: [],
                      query_params: [],
                      headers: {},
                    } as IntegrationRequest)
                  }
                >
                  <Plus size={14} style={{ marginRight: 4 }} /> Nova requisição
                </button>
              ) : null}
            </div>
          </div>
          <p style={{ fontSize: 12, color: "var(--muted)", marginBottom: 12 }}>
            Use <code className="mono">{"{param}"}</code> no path, query com chave/valor, e variáveis <code className="mono">{"{{nome}}"}</code> no body.
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {requests.map((req) => (
              <div key={req.id} className="panel" style={{ padding: 12, border: "1px solid var(--border)" }}>
                <div className="row" style={{ justifyContent: "space-between", flexWrap: "wrap", gap: 8 }}>
                  <div>
                    <strong>{req.name}</strong>
                    <span className="mono" style={{ marginLeft: 8, fontSize: 11, color: "var(--muted)" }}>
                      {req.method} {req.path}
                    </span>
                    {req.is_login ? <span className="badge" style={{ marginLeft: 6 }}>Login</span> : null}
                  </div>
                  <div className="row" style={{ gap: 6 }}>
                    {admin ? (
                      <>
                        <button type="button" className="btn" onClick={() => setEditingReq(normalizeRequest(req))}>
                          Editar
                        </button>
                        <button
                          type="button"
                          className="btn"
                          disabled={runReqM.isPending}
                          title="Executar e ver resultado"
                          onClick={() => runReqM.mutate({ id: req.id, name: req.name })}
                        >
                          <Play size={12} />
                        </button>
                        <button type="button" className="btn btn--danger" onClick={() => setDeleteReqId(req.id)}>
                          <Trash2 size={12} />
                        </button>
                      </>
                    ) : (
                      <button
                        type="button"
                        className="btn"
                        disabled={runReqM.isPending}
                        onClick={() => runReqM.mutate({ id: req.id, name: req.name })}
                      >
                        Executar
                      </button>
                    )}
                  </div>
                </div>
                {req.last_run_ok != null ? (
                  <p style={{ margin: "6px 0 0", fontSize: 11, color: "var(--muted)" }}>
                    Última execução: {req.last_run_ok ? "OK" : "Falhou"} {req.last_run_status ? `HTTP ${req.last_run_status}` : ""}
                  </p>
                ) : null}
              </div>
            ))}
          </div>
          {requests.length === 0 ? <p style={{ color: "var(--muted)" }}>Nenhuma requisição configurada.</p> : null}

          {editingReq ? (
            <RequestEditorModal
              req={editingReq}
              busy={saveReqM.isPending}
              onClose={() => setEditingReq(null)}
              onSave={(r) => saveReqM.mutate(r)}
            />
          ) : null}
        </section>
      )}

      {tab === "testes" && (
        <section style={{ marginTop: 16 }}>
          <div className="panel" style={{ padding: 16 }}>
            <h2 style={{ marginTop: 0, fontSize: 16 }}>Testes rápidos</h2>
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <button type="button" className="btn btn--primary" disabled={testM.isPending} onClick={() => testM.mutate()}>
                <Zap size={14} style={{ marginRight: 4 }} />
                {testM.isPending ? "A testar…" : "Testar conexão (GET /)"}
              </button>
              <button type="button" className="btn" disabled={loginM.isPending} onClick={() => loginM.mutate()}>
                {loginM.isPending ? "A testar…" : authTestLabel}
              </button>
              <button type="button" className="btn" disabled={runAllM.isPending} onClick={() => runAllM.mutate()}>
                {runAllM.isPending ? "A colectar…" : "Executar todos os requisições"}
              </button>
            </div>
            <RunOutput result={lastRun} />
            {runAllResults ? (
              <div style={{ marginTop: 16 }}>
                <h3 style={{ fontSize: 14 }}>Resultado da colecta em lote</h3>
                {runAllResults.map((r, i) => (
                  <div key={i} style={{ marginTop: 8, padding: 10, border: "1px solid var(--border)", borderRadius: 6 }}>
                    <strong>{r.request_name}</strong>
                    <RunOutput result={r} compact />
                  </div>
                ))}
              </div>
            ) : null}
          </div>
          <div className="panel" style={{ padding: 16, marginTop: 16 }}>
            <h3 style={{ marginTop: 0, fontSize: 14 }}>Histórico (últimas 50)</h3>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Quando</th>
                    <th>Tipo</th>
                    <th>Estado</th>
                    <th>HTTP</th>
                    <th>URL</th>
                  </tr>
                </thead>
                <tbody>
                  {(logsQ.data?.logs ?? []).map((log) => (
                    <tr key={log.id}>
                      <td style={{ fontSize: 11 }}>{new Date(log.created_at).toLocaleString()}</td>
                      <td>{log.run_kind}</td>
                      <td>{log.ok ? <span className="badge badge--ok">OK</span> : <span className="badge badge--err">Erro</span>}</td>
                      <td className="mono">{log.status_code ?? "—"}</td>
                      <td className="mono" style={{ fontSize: 10, maxWidth: 280, wordBreak: "break-all" }}>
                        {log.request_url ?? log.error_message ?? "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </section>
      )}

      {showVarsModal ? (
        <VariablesModal
          variables={varsDraft}
          disabled={!admin}
          busy={saveVarsM.isPending}
          onChange={setVarsDraft}
          onClose={() => setShowVarsModal(false)}
          onSave={() => (admin ? saveVarsM.mutate(varsDraft) : setShowVarsModal(false))}
        />
      ) : null}

      {runPanel ? (
        <RequestRunResultModal
          requestName={runPanel.requestName}
          result={runPanel.result}
          logs={logsQ.data?.logs ?? []}
          logsLoading={logsQ.isLoading}
          onClose={() => setRunPanel(null)}
        />
      ) : null}

      <ConfirmModal
        open={!!deleteReqId}
        title="Eliminar requisição"
        message="Eliminar esta requisição HTTP?"
        danger
        busy={deleteReqM.isPending}
        onCancel={() => setDeleteReqId(null)}
        onConfirm={() => deleteReqId && deleteReqM.mutate(deleteReqId)}
      />
    </div>
  );
}

function parseConsumerConfig(raw: unknown): ConsumerConfig {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return {
      client_search: { enabled: false },
      client_attendance: { enabled: false },
      client_work_order: { enabled: false },
    };
  }
  const o = raw as ConsumerConfig;
  return {
    client_search: {
      enabled: !!o.client_search?.enabled,
      request_id: o.client_search?.request_id,
      provider: (o.client_search?.provider as ClientSearchProvider) || "auto",
      ixc_list_action: o.client_search?.ixc_list_action,
      busca_options: o.client_search?.busca_options,
      field_mappings: o.client_search?.field_mappings,
      cpf_multi_attempt: o.client_search?.cpf_multi_attempt,
    },
    client_attendance: {
      enabled: !!o.client_attendance?.enabled,
      request_id: o.client_attendance?.request_id,
      provider: (o.client_attendance?.provider as ClientSearchProvider) || "auto",
      ixc_list_action: o.client_attendance?.ixc_list_action,
      field_mappings: o.client_attendance?.field_mappings,
    },
    client_work_order: {
      enabled: !!o.client_work_order?.enabled,
      request_id: o.client_work_order?.request_id,
    },
  };
}

function parseVariablesRecord(raw: unknown): Record<string, string> {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    out[k] = String(v ?? "");
  }
  return out;
}

function normalizeRequest(req: IntegrationRequest): IntegrationRequest {
  return {
    ...req,
    path_params: parsePathParams(req.path_params),
    query_params: parseQueryParams(req.query_params),
    headers: parseHeaders(req.headers),
  };
}

function VariablesEditor({
  variables,
  onChange,
  disabled,
  allowRenameKeys,
  compact,
}: {
  variables: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  disabled?: boolean;
  allowRenameKeys?: boolean;
  compact?: boolean;
}) {
  const entries = Object.entries(variables);
  return (
    <div className="field" style={{ marginTop: compact ? 0 : 12 }}>
      {!compact ? (
        <>
          <label>Variáveis globais</label>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 8px" }}>
            Use <code className="mono">{"{{nome}}"}</code> em query, path ou body (ex.: <code className="mono">{"{{termo_busca}}"}</code>
            ).
          </p>
        </>
      ) : null}
      {entries.length === 0 ? (
        <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>Nenhuma variável definida.</p>
      ) : null}
      {entries.map(([k, v]) => (
        <div key={k} className="row" style={{ gap: 6, marginBottom: 6 }}>
          <input
            className="input mono"
            style={{ width: 140 }}
            disabled={disabled || !allowRenameKeys}
            readOnly={!allowRenameKeys}
            value={k}
            placeholder="nome"
            onChange={(e) => {
              const newKey = e.target.value;
              const next: Record<string, string> = {};
              for (const [kk, vv] of Object.entries(variables)) {
                if (kk === k) next[newKey] = vv;
                else next[kk] = vv;
              }
              onChange(next);
            }}
          />
          <input
            className="input mono"
            style={{ flex: 1 }}
            disabled={disabled}
            value={v}
            placeholder="valor"
            onChange={(e) => onChange({ ...variables, [k]: e.target.value })}
          />
          {!disabled ? (
            <button
              type="button"
              className="btn"
              aria-label="Remover"
              onClick={() => {
                const next = { ...variables };
                delete next[k];
                onChange(next);
              }}
            >
              ×
            </button>
          ) : null}
        </div>
      ))}
      {!disabled ? (
        <button
          type="button"
          className="btn"
          style={{ fontSize: 12 }}
          onClick={() => {
            let n = 1;
            let key = "termo_busca";
            while (key in variables) {
              n += 1;
              key = `var${n}`;
            }
            onChange({ ...variables, [key]: "" });
          }}
        >
          + Variável
        </button>
      ) : null}
    </div>
  );
}

function VariablesModal({
  variables,
  onChange,
  onClose,
  onSave,
  disabled,
  busy,
}: {
  variables: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  onClose: () => void;
  onSave: () => void;
  disabled?: boolean;
  busy?: boolean;
}) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        style={{ maxWidth: 560, maxHeight: "85vh", overflow: "auto" }}
        role="dialog"
        aria-labelledby="vars-modal-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8 }}>
          <h3 id="vars-modal-title" style={{ margin: 0 }}>
            Variáveis globais
          </h3>
          <button type="button" className="btn" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 8 }}>
          Valores usados nos requisições com <code className="mono">{"{{chave}}"}</code>. Salve antes de executar requisições que dependem
          destes valores (ex. <code className="mono">busca</code>, <code className="mono">termo_busca</code> na Hubsoft).
        </p>
        <VariablesEditor variables={variables} onChange={onChange} disabled={disabled} allowRenameKeys compact />
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          {disabled ? null : (
            <button type="button" className="btn btn--primary" disabled={busy} onClick={onSave}>
              {busy ? "A salvar…" : "Salvar variáveis"}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function RequestRunResultModal({
  requestName,
  result,
  logs,
  logsLoading,
  onClose,
}: {
  requestName: string;
  result: RunResult;
  logs: IntegrationRunLog[];
  logsLoading: boolean;
  onClose: () => void;
}) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        style={{ maxWidth: 900, width: "min(96vw, 900px)", maxHeight: "90vh", overflow: "auto" }}
        role="dialog"
        aria-labelledby="run-result-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8 }}>
          <div>
            <h3 id="run-result-title" style={{ margin: 0 }}>
              Resultado — {requestName}
            </h3>
            <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--muted)" }}>Execução actual e registos recentes desta requisição</p>
          </div>
          <button type="button" className="btn" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <h4 style={{ fontSize: 13, marginTop: 16, marginBottom: 8 }}>Resposta</h4>
        <RunOutput result={result} />

        <h4 style={{ fontSize: 13, marginTop: 20, marginBottom: 8 }}>Logs da requisição</h4>
        {logsLoading ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar logs…</p>
        ) : logs.length === 0 ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>Sem registos salvos para esta requisição.</p>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            {logs.map((log) => (
              <div
                key={log.id}
                style={{
                  padding: 10,
                  border: "1px solid var(--border)",
                  borderRadius: 6,
                  background: "var(--panel2)",
                }}
              >
                <div className="row" style={{ gap: 8, flexWrap: "wrap", fontSize: 12 }}>
                  <span>{new Date(log.created_at).toLocaleString()}</span>
                  <span className="mono">{log.run_kind}</span>
                  {log.ok ? <span className="badge badge--ok">OK</span> : <span className="badge badge--err">Erro</span>}
                  {log.status_code != null ? <span className="mono">HTTP {log.status_code}</span> : null}
                  {log.latency_ms != null ? <span>{log.latency_ms} ms</span> : null}
                </div>
                {log.request_url ? (
                  <p className="mono" style={{ margin: "6px 0 0", fontSize: 11, wordBreak: "break-all" }}>
                    {log.request_url}
                  </p>
                ) : null}
                {log.error_message ? <p style={{ margin: "6px 0 0", color: "var(--err)", fontSize: 12 }}>{log.error_message}</p> : null}
                {log.response_preview ? (
                  <pre className="mono" style={{ margin: "8px 0 0", maxHeight: 200, overflow: "auto", fontSize: 11, whiteSpace: "pre-wrap" }}>
                    {log.response_preview}
                  </pre>
                ) : null}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function HeadersEditor({
  title,
  headers,
  onChange,
  disabled,
}: {
  title: string;
  headers: Record<string, string>;
  onChange: (h: Record<string, string>) => void;
  disabled?: boolean;
}) {
  const entries = Object.entries(headers);
  return (
    <div className="field" style={{ marginTop: 12 }}>
      <label>{title}</label>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 8px" }}>
        Cabeçalhos HTTP enviados em <strong>todos</strong> os requisições desta integração (ex.: <code className="mono">Accept: application/json</code>
        ). O <code className="mono">Authorization</code> é preenchido automaticamente após obter o token.
      </p>
      {entries.map(([k, v]) => (
        <div key={k} className="row" style={{ gap: 6, marginBottom: 6 }}>
          <input className="input mono" style={{ width: 140 }} disabled={disabled} value={k} onChange={(e) => {
            const next = { ...headers };
            delete next[k];
            next[e.target.value] = v;
            onChange(next);
          }} />
          <input className="input mono" style={{ flex: 1 }} disabled={disabled} value={v} onChange={(e) => onChange({ ...headers, [k]: e.target.value })} />
          {!disabled ? (
            <button type="button" className="btn" onClick={() => {
              const next = { ...headers };
              delete next[k];
              onChange(next);
            }}>
              ×
            </button>
          ) : null}
        </div>
      ))}
      {!disabled ? (
        <button type="button" className="btn" style={{ fontSize: 12 }} onClick={() => onChange({ ...headers, "X-Custom": "" })}>
          + Header
        </button>
      ) : null}
    </div>
  );
}

const FIELD_MAPPING_KEYS = [
  { key: "cpf_cnpj", label: "CPF/CNPJ" },
  { key: "codigo_cliente", label: "ID cliente" },
  { key: "nome_razaosocial", label: "Razão social" },
  { key: "nome_fantasia", label: "Nome fantasia" },
  { key: "telefone", label: "Telefone" },
  { key: "email", label: "E-mail" },
] as const;

function FieldMappingsEditor({
  mappings,
  onChange,
  disabled,
}: {
  mappings: Record<string, SearchFieldMapping>;
  onChange: (m: Record<string, SearchFieldMapping>) => void;
  disabled?: boolean;
}) {
  const patch = (buscaKey: string, field: keyof SearchFieldMapping, value: string) => {
    const next = { ...mappings };
    const cur = { ...(next[buscaKey] ?? {}) };
    if (value === "") delete cur[field];
    else cur[field] = value as SearchFieldMapping[typeof field];
    if (Object.keys(cur).length === 0) delete next[buscaKey];
    else next[buscaKey] = cur;
    onChange(next);
  };
  return (
    <div style={{ marginTop: 12, overflowX: "auto" }}>
      <table className="table-compact" style={{ fontSize: 12, width: "100%", minWidth: 520 }}>
        <thead>
          <tr>
            <th>Tipo</th>
            <th>qtype (API)</th>
            <th>oper</th>
            <th>Formato termo</th>
          </tr>
        </thead>
        <tbody>
          {FIELD_MAPPING_KEYS.map(({ key, label }) => {
            const m = mappings[key] ?? {};
            return (
              <tr key={key}>
                <td>{label}</td>
                <td>
                  <input
                    className="input mono"
                    disabled={disabled}
                    placeholder="padrão"
                    value={m.qtype ?? ""}
                    onChange={(e) => patch(key, "qtype", e.target.value)}
                  />
                </td>
                <td>
                  <input
                    className="input mono"
                    style={{ width: 56 }}
                    disabled={disabled}
                    placeholder="= / L"
                    value={m.oper ?? ""}
                    onChange={(e) => patch(key, "oper", e.target.value)}
                  />
                </td>
                <td>
                  <select
                    className="input"
                    disabled={disabled}
                    value={m.termo_format ?? ""}
                    onChange={(e) => patch(key, "termo_format", e.target.value)}
                  >
                    {TERMO_FORMAT_OPTIONS.map((o) => (
                      <option key={o.id || "default"} value={o.id}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function looksLikeIXCRequest(req: Pick<IntegrationRequest, "path" | "headers" | "body_template">): boolean {
  const path = (req.path || "").toLowerCase();
  if (path.includes("/cliente") && !path.includes("integracao")) return true;
  const hdrs = req.headers || {};
  for (const [k, v] of Object.entries(hdrs)) {
    if (k.toLowerCase() === "ixcsoft" && String(v).trim()) return true;
  }
  const body = (req.body_template || "").toLowerCase();
  return body.includes("qtype") && body.includes("sortname");
}

function RequestEditorModal({
  req,
  onClose,
  onSave,
  busy,
}: {
  req: IntegrationRequest;
  onClose: () => void;
  onSave: (r: IntegrationRequest) => void;
  busy?: boolean;
}) {
  const [local, setLocal] = useState(req);

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div className="modal" style={{ maxWidth: 720, maxHeight: "90vh", overflow: "auto" }} role="dialog" onMouseDown={(e) => e.stopPropagation()}>
        <h3>{req.id.startsWith("new-") ? "Nova requisição" : "Editar requisição"}</h3>
        <div className="field">
          <label>Nome</label>
          <input className="input" value={local.name} onChange={(e) => setLocal((r) => ({ ...r, name: e.target.value }))} />
        </div>
        <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
          <div className="field" style={{ width: 100 }}>
            <label>Método</label>
            <select className="input" value={local.method} onChange={(e) => setLocal((r) => ({ ...r, method: e.target.value }))}>
              {HTTP_METHODS.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </div>
          <div className="field" style={{ flex: 1, minWidth: 200 }}>
            <label>Path (ex. /api/v1/users/{"{id}"})</label>
            <input className="input mono" value={local.path} onChange={(e) => setLocal((r) => ({ ...r, path: e.target.value }))} />
          </div>
        </div>
        <PathParamsEditor params={local.path_params} onChange={(path_params) => setLocal((r) => ({ ...r, path_params }))} />
        <QueryParamsEditor params={local.query_params} onChange={(query_params) => setLocal((r) => ({ ...r, query_params }))} />
        {looksLikeIXCRequest(local) ? (
          <p className="msg" style={{ fontSize: 12, marginTop: 0 }}>
            <strong>IXC:</strong> use POST com header <code className="mono">ixcsoft: listar</code>. Parâmetros{" "}
            <code className="mono">qtype</code>, <code className="mono">query</code>, <code className="mono">oper</code> etc. vão no{" "}
            <strong>body JSON</strong>, não na query string. Exemplo:{" "}
            <code className="mono">{`{"qtype":"cliente.id","query":"0","oper":">","rp":"20","sortname":"cliente.id","sortorder":"desc"}`}</code>
            . Na Consulta, o NetQuasar preenche <code className="mono">qtype</code>/<code className="mono">query</code> automaticamente.
          </p>
        ) : null}
        <HeadersEditor title="Headers desta requisição" headers={local.headers} onChange={(headers) => setLocal((r) => ({ ...r, headers }))} />
        <div className="field">
          <label>Tipo de body</label>
          <select className="input" value={local.body_type} onChange={(e) => setLocal((r) => ({ ...r, body_type: e.target.value }))}>
            <option value="none">Nenhum</option>
            <option value="json">JSON</option>
            <option value="form">Form URL-encoded</option>
            <option value="text">Texto</option>
          </select>
        </div>
        {local.body_type !== "none" ? (
          <div className="field">
            <label>Body</label>
            <textarea className="textarea mono" rows={5} value={local.body_template ?? ""} onChange={(e) => setLocal((r) => ({ ...r, body_template: e.target.value }))} />
          </div>
        ) : null}
        <div className="field">
          <label>Extrair JSON (caminho com pontos, ex. data.items)</label>
          <input className="input mono" value={local.extract_json_path ?? ""} onChange={(e) => setLocal((r) => ({ ...r, extract_json_path: e.target.value }))} />
        </div>
        <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={local.is_login} onChange={(e) => setLocal((r) => ({ ...r, is_login: e.target.checked }))} />
            Esta requisição é o login
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={local.enabled} onChange={(e) => setLocal((r) => ({ ...r, enabled: e.target.checked }))} />
            Ativo
          </label>
          <div className="field" style={{ margin: 0, width: 80 }}>
            <label>Ordem</label>
            <input className="input mono" type="number" value={local.sort_order} onChange={(e) => setLocal((r) => ({ ...r, sort_order: Number(e.target.value) }))} />
          </div>
        </div>
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 12 }}>
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={busy} onClick={() => onSave(local)}>
            {busy ? "…" : "Salvar requisição"}
          </button>
        </div>
      </div>
    </div>
  );
}

function PathParamsEditor({ params, onChange }: { params: PathParam[]; onChange: (p: PathParam[]) => void }) {
  return (
    <div className="field">
      <label>Path params</label>
      {params.map((p, i) => (
        <div key={i} className="row" style={{ gap: 6, marginBottom: 6 }}>
          <input className="input mono" placeholder="nome" value={p.name} onChange={(e) => {
            const next = [...params];
            next[i] = { ...p, name: e.target.value };
            onChange(next);
          }} />
          <select className="input" style={{ width: 100 }} value={p.source} onChange={(e) => {
            const next = [...params];
            next[i] = { ...p, source: e.target.value as "static" | "variable" };
            onChange(next);
          }}>
            <option value="static">Estático</option>
            <option value="variable">Variável</option>
          </select>
          <input className="input mono" style={{ flex: 1 }} placeholder="valor ou chave" value={p.value} onChange={(e) => {
            const next = [...params];
            next[i] = { ...p, value: e.target.value };
            onChange(next);
          }} />
          <button type="button" className="btn" onClick={() => onChange(params.filter((_, j) => j !== i))}>
            ×
          </button>
        </div>
      ))}
      <button type="button" className="btn" style={{ fontSize: 12 }} onClick={() => onChange([...params, { name: "", value: "", source: "static" }])}>
        + Path param
      </button>
    </div>
  );
}

function QueryParamsEditor({ params, onChange }: { params: QueryParam[]; onChange: (p: QueryParam[]) => void }) {
  return (
    <div className="field">
      <label>Query string</label>
      {params.map((p, i) => (
        <div key={i} className="row" style={{ gap: 6, marginBottom: 6 }}>
          <input type="checkbox" checked={p.enabled !== false} onChange={(e) => {
            const next = [...params];
            next[i] = { ...p, enabled: e.target.checked };
            onChange(next);
          }} />
          <input className="input mono" placeholder="chave" value={p.key} onChange={(e) => {
            const next = [...params];
            next[i] = { ...p, key: e.target.value };
            onChange(next);
          }} />
          <input className="input mono" style={{ flex: 1 }} placeholder="valor" value={p.value} onChange={(e) => {
            const next = [...params];
            next[i] = { ...p, value: e.target.value };
            onChange(next);
          }} />
          <button type="button" className="btn" onClick={() => onChange(params.filter((_, j) => j !== i))}>
            ×
          </button>
        </div>
      ))}
      <button type="button" className="btn" style={{ fontSize: 12 }} onClick={() => onChange([...params, { key: "", value: "", enabled: true }])}>
        + Query param
      </button>
    </div>
  );
}

function RunOutput({ result, compact }: { result: RunResult | null; compact?: boolean }) {
  if (!result) return null;
  return (
    <div style={{ marginTop: compact ? 6 : 12, padding: 10, background: "var(--panel2)", borderRadius: 6, fontSize: 12 }}>
      <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
        <span className={result.ok ? "badge badge--ok" : "badge badge--err"}>{result.ok ? "OK" : "Erro"}</span>
        {result.status_code != null ? <span className="mono">HTTP {result.status_code}</span> : null}
        {result.latency_ms != null ? <span>{result.latency_ms} ms</span> : null}
      </div>
      {result.request_url ? <p className="mono" style={{ wordBreak: "break-all", margin: "6px 0" }}>{result.request_method} {result.request_url}</p> : null}
      {result.error ? <p style={{ color: "var(--err)" }}>{result.error}</p> : null}
      {result.extracted != null ? (
        <pre className="mono" style={{ maxHeight: compact ? 120 : 200, overflow: "auto" }}>
          {JSON.stringify(result.extracted, null, 2)}
        </pre>
      ) : null}
      {result.response_preview ? (
        <pre className="mono" style={{ maxHeight: compact ? 160 : 320, overflow: "auto", marginTop: 6, whiteSpace: "pre-wrap" }}>
          {result.response_preview}
        </pre>
      ) : null}
    </div>
  );
}
