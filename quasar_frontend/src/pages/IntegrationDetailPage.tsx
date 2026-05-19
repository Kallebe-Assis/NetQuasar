import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Play, Plus, Save, Trash2, Zap } from "lucide-react";
import { ConfirmModal } from "../components/ConfirmModal";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { queryKeys } from "../lib/queryKeys";
import {
  AUTH_TYPES,
  HTTP_METHODS,
  type IntegrationDetail,
  type IntegrationRequest,
  type PathParam,
  type QueryParam,
  type RunResult,
  parseHeaders,
  parsePathParams,
  parseQueryParams,
} from "../integrations/types";

type Tab = "geral" | "auth" | "requests" | "testes";

const emptyRequest = (): Omit<IntegrationRequest, "id" | "integration_id" | "last_run_at" | "last_run_ok" | "last_run_status" | "last_run_message"> => ({
  name: "Novo pedido",
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
  const [tab, setTab] = useState<Tab>("geral");
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const [editingReq, setEditingReq] = useState<IntegrationRequest | null>(null);
  const [deleteReqId, setDeleteReqId] = useState<string | null>(null);
  const [lastRun, setLastRun] = useState<RunResult | null>(null);
  const [runAllResults, setRunAllResults] = useState<RunResult[] | null>(null);

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
          auth_config: authCfg,
          timeout_ms: form.timeout_ms,
          tls_insecure: form.tls_insecure,
        },
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      void qc.invalidateQueries({ queryKey: queryKeys.integrations });
      setMsg({ kind: "ok", text: "Configuração guardada." });
    },
    onError: (e) => setMsg({ kind: "err", text: e instanceof Error ? e.message : String(e) }),
  });

  const testM = useMutation({
    mutationFn: () => apiFetch<RunResult>(`/api/v1/integrations/${slug}/test`, { method: "POST" }),
    onSuccess: (r) => {
      setLastRun(r);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      setMsg({ kind: r.ok ? "ok" : "err", text: r.ok ? "Teste de conexão OK." : r.error || "Teste falhou." });
    },
    onError: (e) => setMsg({ kind: "err", text: e instanceof Error ? e.message : String(e) }),
  });

  const loginM = useMutation({
    mutationFn: () => apiFetch<RunResult>(`/api/v1/integrations/${slug}/login`, { method: "POST" }),
    onSuccess: (r) => {
      setLastRun(r);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      setMsg({
        kind: r.ok ? "ok" : "err",
        text: r.token_received ? "Login OK — token guardado na sessão." : r.ok ? "Login OK." : r.error || "Login falhou.",
      });
    },
    onError: (e) => setMsg({ kind: "err", text: e instanceof Error ? e.message : String(e) }),
  });

  const runAllM = useMutation({
    mutationFn: () => apiFetch<{ results: RunResult[] }>(`/api/v1/integrations/${slug}/run-all`, { method: "POST" }),
    onSuccess: (r) => {
      setRunAllResults(r.results ?? []);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
      setMsg({ kind: "ok", text: `Colecta concluída: ${r.results?.length ?? 0} pedido(s).` });
    },
    onError: (e) => setMsg({ kind: "err", text: e instanceof Error ? e.message : String(e) }),
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
      setMsg({ kind: "ok", text: "Pedido guardado." });
    },
    onError: (e) => setMsg({ kind: "err", text: e instanceof Error ? e.message : String(e) }),
  });

  const deleteReqM = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/integrations/${slug}/requests/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDeleteReqId(null);
      setEditingReq(null);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
    },
  });

  const runReqM = useMutation({
    mutationFn: (id: string) => apiFetch<RunResult>(`/api/v1/integrations/${slug}/requests/${id}/run`, { method: "POST" }),
    onSuccess: (r) => {
      setLastRun(r);
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug ?? "") });
    },
  });

  const logsQ = useQuery({
    queryKey: queryKeys.integrationLogs(slug ?? ""),
    queryFn: () => apiFetch<{ logs: { id: string; run_kind: string; ok: boolean; status_code?: number; request_url?: string; error_message?: string; created_at: string }[] }>(`/api/v1/integrations/${slug}/logs`),
    enabled: !!slug && tab === "testes",
  });

  const requests = useMemo(() => {
    if (!d?.requests) return [];
    return [...d.requests].sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name));
  }, [d?.requests]);

  const authCfg = (form.auth_config ?? {}) as Record<string, string>;

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

  return (
    <div>
      <div className="row" style={{ alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <Link to="/integrations" className="btn" style={{ textDecoration: "none" }}>
          <ArrowLeft size={14} style={{ marginRight: 4 }} /> Integrações
        </Link>
        <h1 style={{ margin: 0, flex: 1 }}>{d.name}</h1>
        <span className={d.enabled ? "badge badge--ok" : "badge badge--off"}>{d.enabled ? "Activa" : "Inactiva"}</span>
        {d.session_active ? <span className="badge">Sessão activa</span> : null}
      </div>

      {msg ? (
        <div className={`msg msg--${msg.kind === "ok" ? "ok" : "err"}`} style={{ marginTop: 12 }}>
          {msg.text}
        </div>
      ) : null}

      <div className="tabs" style={{ marginTop: 16, flexWrap: "wrap" }}>
        {(
          [
            ["geral", "Geral"],
            ["auth", "Autenticação"],
            ["requests", "Pedidos HTTP"],
            ["testes", "Testes e logs"],
          ] as const
        ).map(([k, lab]) => (
          <button key={k} type="button" className={tab === k ? "active" : ""} onClick={() => setTab(k)}>
            {lab}
          </button>
        ))}
      </div>

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
              Integração activa
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
          {admin ? (
            <button type="button" className="btn btn--primary" style={{ marginTop: 12 }} disabled={saveM.isPending} onClick={() => saveM.mutate()}>
              <Save size={14} style={{ marginRight: 6 }} />
              {saveM.isPending ? "A guardar…" : "Guardar"}
            </button>
          ) : null}
        </section>
      )}

      {tab === "auth" && (
        <section className="panel" style={{ marginTop: 16, padding: 16 }}>
          <h2 style={{ marginTop: 0, fontSize: 16 }}>Autenticação</h2>
          <div className="field" style={{ maxWidth: 280 }}>
            <label>Tipo</label>
            <select className="input" disabled={!admin} value={form.auth_type ?? "none"} onChange={(e) => setForm((f) => ({ ...f, auth_type: e.target.value }))}>
              {AUTH_TYPES.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.label}
                </option>
              ))}
            </select>
          </div>
          {form.auth_type === "bearer" && (
            <>
              <div className="field">
                <label>Token {d.token_configured ? "(já configurado — deixe vazio para manter)" : ""}</label>
                <input className="input mono" disabled={!admin} placeholder="••••••" onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, token: e.target.value } }))} />
              </div>
              <div className="field">
                <label>Prefixo (ex. Bearer)</label>
                <input className="input" disabled={!admin} defaultValue={authCfg.token_prefix || "Bearer"} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, token_prefix: e.target.value } }))} />
              </div>
            </>
          )}
          {form.auth_type === "basic" && (
            <>
              <div className="field">
                <label>Utilizador</label>
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
              <p style={{ fontSize: 12, color: "var(--muted)" }}>
                Opção A: marque um pedido HTTP como «Login». Opção B: preencha o caminho de login abaixo.
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
              <div className="field">
                <label>Body JSON (suporta {"{{variavel}}"})</label>
                <textarea className="textarea mono" rows={4} disabled={!admin} defaultValue={authCfg.login_body || '{"username":"{{user}}","password":"{{pass}}"}'} onChange={(e) => setForm((f) => ({ ...f, auth_config: { ...authCfg, login_body: e.target.value } }))} />
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
                Guardar auth
              </button>
              <button type="button" className="btn" disabled={loginM.isPending} onClick={() => loginM.mutate()}>
                {loginM.isPending ? "A autenticar…" : "Testar login"}
              </button>
            </div>
          ) : null}
        </section>
      )}

      {tab === "requests" && (
        <section style={{ marginTop: 16 }}>
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 12 }}>
            <h2 style={{ margin: 0, fontSize: 16 }}>Pedidos de coleta</h2>
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
                <Plus size={14} style={{ marginRight: 4 }} /> Novo pedido
              </button>
            ) : null}
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
                        <button type="button" className="btn" disabled={runReqM.isPending} onClick={() => runReqM.mutate(req.id)}>
                          <Play size={12} />
                        </button>
                        <button type="button" className="btn btn--danger" onClick={() => setDeleteReqId(req.id)}>
                          <Trash2 size={12} />
                        </button>
                      </>
                    ) : (
                      <button type="button" className="btn" disabled={runReqM.isPending} onClick={() => runReqM.mutate(req.id)}>
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
          {requests.length === 0 ? <p style={{ color: "var(--muted)" }}>Nenhum pedido configurado.</p> : null}

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
                Testar login
              </button>
              <button type="button" className="btn" disabled={runAllM.isPending} onClick={() => runAllM.mutate()}>
                {runAllM.isPending ? "A colectar…" : "Executar todos os pedidos"}
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

      <ConfirmModal
        open={!!deleteReqId}
        title="Eliminar pedido"
        message="Eliminar este pedido HTTP?"
        danger
        busy={deleteReqM.isPending}
        onCancel={() => setDeleteReqId(null)}
        onConfirm={() => deleteReqId && deleteReqM.mutate(deleteReqId)}
      />
    </div>
  );
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
}: {
  variables: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  disabled?: boolean;
}) {
  const entries = Object.entries(variables);
  return (
    <div className="field" style={{ marginTop: 12 }}>
      <label>Variáveis globais (use {"{{chave}}"} no path/body)</label>
      {entries.map(([k, v]) => (
        <div key={k} className="row" style={{ gap: 6, marginBottom: 6 }}>
          <input className="input mono" style={{ width: 120 }} disabled={disabled} value={k} readOnly />
          <input className="input mono" style={{ flex: 1 }} disabled={disabled} value={v} onChange={(e) => onChange({ ...variables, [k]: e.target.value })} />
          {!disabled ? (
            <button type="button" className="btn" onClick={() => {
              const next = { ...variables };
              delete next[k];
              onChange(next);
            }}>
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
          onClick={() => onChange({ ...variables, [`var${entries.length + 1}`]: "" })}
        >
          + Variável
        </button>
      ) : null}
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
        <h3>{req.id.startsWith("new-") ? "Novo pedido" : "Editar pedido"}</h3>
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
        <HeadersEditor title="Headers deste pedido" headers={local.headers} onChange={(headers) => setLocal((r) => ({ ...r, headers }))} />
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
            Este pedido é o login
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={local.enabled} onChange={(e) => setLocal((r) => ({ ...r, enabled: e.target.checked }))} />
            Activo
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
            {busy ? "…" : "Guardar pedido"}
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
