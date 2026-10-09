import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { BookOpen, CalendarClock, CheckCircle2, ClipboardCheck, History, KeyRound, MapPinned, Package, PlugZap, PowerOff, UserPlus, XCircle } from "lucide-react";
import { HubsoftPasswordFix } from "./HubsoftPasswordFix";
import { ToolPanel } from "./hubsoftAdminKit";
import { HubsoftHeader } from "./HubsoftHeader";
import { HubsoftDataVendaSection } from "./HubsoftDataVendaSection";
import { HubsoftCatalogExplorer } from "./HubsoftCatalogExplorer";
import { HubsoftBulkImportSection } from "./HubsoftBulkImportSection";
import { HubsoftBulkImportHistory } from "./HubsoftBulkImportHistory";
import { HubsoftStock } from "./HubsoftStock";
import { HubsoftConference } from "./HubsoftConference";
import { HubsoftIxcLogins } from "./HubsoftIxcLogins";
import { HubsoftAddressCheck } from "./HubsoftAddressCheck";
import { can, isAdminUser } from "../../lib/auth";
import { IntegrationLogoField } from "../../components/IntegrationLogoField";
import type { IntegrationDetail } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { PageToastHost, usePageToast } from "../../lib/pageToast";
import { queryKeys } from "../../lib/queryKeys";

type TestOutcome = { ok: boolean; message: string; latency_ms?: number };

type ConfigTab = "conexao" | "importar" | "conferir" | "senhas" | "data-venda" | "catalogos" | "historico" | "ixc-logins" | "enderecos" | "estoque";

// Seções da configuração. "Conexão" é de quem gere integrações; as de edição em massa (bulk) só aparecem para
// administradores ou perfis com a permissão "integrations.hubsoft_bulk" (Configurações → Perfis de permissão).
const CONFIG_TABS: { id: ConfigTab; label: string; icon: ReactNode; bulk?: boolean; ixc?: boolean }[] = [
  { id: "conexao", label: "Conexão", icon: <PlugZap size={15} aria-hidden /> },
  { id: "importar", label: "Importar clientes", icon: <UserPlus size={15} aria-hidden />, bulk: true },
  { id: "conferir", label: "Conferência", icon: <ClipboardCheck size={15} aria-hidden />, bulk: true },
  { id: "senhas", label: "Corrigir senhas", icon: <KeyRound size={15} aria-hidden />, bulk: true },
  { id: "enderecos", label: "Conferir endereços", icon: <MapPinned size={15} aria-hidden />, bulk: true },
  { id: "estoque", label: "Estoque", icon: <Package size={15} aria-hidden />, bulk: true },
  { id: "data-venda", label: "Data de venda", icon: <CalendarClock size={15} aria-hidden />, bulk: true },
  { id: "catalogos", label: "Catálogos", icon: <BookOpen size={15} aria-hidden />, bulk: true },
  { id: "historico", label: "Histórico", icon: <History size={15} aria-hidden />, bulk: true },
  { id: "ixc-logins", label: "Inativar no IXC", icon: <PowerOff size={15} aria-hidden />, ixc: true },
];

/**
 * Configuração simplificada da HubSoft: só os campos comuns a qualquer integração
 * (URL, credenciais) + um botão único que salva e testa a ligação de verdade (login
 * OAuth2 + uma chamada real de API — não só um GET de conectividade).
 */
export function HubsoftConfigPage() {
  const slug = "hubsoft";
  const qc = useQueryClient();
  const { toast, show: showToast, dismiss: dismissToast } = usePageToast();
  const [searchParams, setSearchParams] = useSearchParams();

  const detailQ = useQuery({
    queryKey: queryKeys.integrationDetail(slug),
    queryFn: () => apiFetch<IntegrationDetail>(`/api/v1/integrations/${slug}`),
  });

  const [baseUrl, setBaseUrl] = useState("");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [testResult, setTestResult] = useState<TestOutcome | null>(null);

  useEffect(() => {
    const d = detailQ.data;
    if (!d) return;
    setBaseUrl(d.base_url ?? "");
    setClientId(String(d.auth_config?.client_id ?? ""));
    setUsername(String(d.auth_config?.username ?? ""));
    setClientSecret("");
    setPassword("");
  }, [detailQ.data]);

  const secretConfigured = !!detailQ.data?.auth_config?.client_secret_configured;
  const passwordConfigured = !!detailQ.data?.password_configured;

  const saveM = useMutation({
    mutationFn: () => {
      const authConfig: Record<string, unknown> = {
        client_id: clientId.trim(),
        username: username.trim(),
        grant_type: "password",
      };
      if (clientSecret.trim()) authConfig.client_secret = clientSecret.trim();
      if (password.trim()) authConfig.password = password.trim();
      return apiFetch(`/api/v1/integrations/${slug}`, {
        method: "PATCH",
        json: { base_url: baseUrl.trim(), auth_type: "oauth2_password", auth_config: authConfig },
      });
    },
    onSuccess: () => {
      setClientSecret("");
      setPassword("");
      showToast("ok", "Configuração salva.");
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug) });
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : "Falha ao salvar."),
  });

  const testM = useMutation({
    mutationFn: () => apiFetch<TestOutcome>(`/api/v1/integrations/${slug}/hubsoft/test`, { method: "POST" }),
    onSuccess: (r) => {
      setTestResult(r);
      showToast(r.ok ? "ok" : "err", r.ok ? "Conexão OK." : r.message || "Teste falhou.");
      void qc.invalidateQueries({ queryKey: queryKeys.integrationDetail(slug) });
    },
    onError: (e) => {
      const message = e instanceof Error ? e.message : String(e);
      setTestResult({ ok: false, message });
      showToast("err", message);
    },
  });

  const preloadQ = useQuery({
    queryKey: ["hubsoft-preload", slug],
    queryFn: () => apiFetch<{ preload_on_startup: boolean }>(`/api/v1/integrations/${slug}/hubsoft/preload`),
  });
  const [preloadOnStartup, setPreloadOnStartup] = useState(false);
  useEffect(() => {
    if (preloadQ.data) setPreloadOnStartup(preloadQ.data.preload_on_startup);
  }, [preloadQ.data]);

  const preloadM = useMutation({
    mutationFn: (v: boolean) =>
      apiFetch(`/api/v1/integrations/${slug}/hubsoft/preload`, { method: "PUT", json: { preload_on_startup: v } }),
    onSuccess: () => showToast("ok", "Preferência gravada."),
    onError: (e) => {
      showToast("err", e instanceof Error ? e.message : "Falha ao gravar.");
      if (preloadQ.data) setPreloadOnStartup(preloadQ.data.preload_on_startup);
    },
  });

  const busy = saveM.isPending || testM.isPending;
  const d = detailQ.data;

  if (detailQ.isLoading) return <p style={{ padding: 24, color: "var(--muted)" }}>A carregar…</p>;
  if (detailQ.isError || !d) {
    return (
      <div style={{ padding: 24 }}>
        <p className="msg msg--err">{(detailQ.error as Error)?.message || "Integração não encontrada."}</p>
      </div>
    );
  }

  const canConnection = isAdminUser() || can("integrations.manage");
  const canBulk = can("integrations.hubsoft_bulk");
  const canIxc = can("integrations.ixc_logins");
  const visibleTabs = CONFIG_TABS.filter((t) => (t.bulk ? canBulk : t.ixc ? canIxc : canConnection));
  // links antigos (?aba=produtos / ?aba=patrimonios) abrem a aba «Estoque» já na sub-aba certa
  const rawAba = searchParams.get("aba");
  const tabParam = (rawAba === "produtos" || rawAba === "patrimonios" ? "estoque" : rawAba) as ConfigTab | null;
  const tab: ConfigTab = visibleTabs.some((t) => t.id === tabParam) ? (tabParam as ConfigTab) : (visibleTabs[0]?.id ?? "conexao");
  const selectTab = (id: ConfigTab) => {
    const next = new URLSearchParams(searchParams);
    if (id === "conexao") next.delete("aba");
    else next.set("aba", id);
    setSearchParams(next, { replace: true });
  };

  return (
    <div className="hsa-page">
      <HubsoftHeader />
      <PageToastHost toast={toast} onDismiss={dismissToast} />

      <nav className="hsa-tabs" aria-label="Seções da configuração da HubSoft">
        {visibleTabs.map((t) => (
          <button key={t.id} type="button" className={`hsa-tabs__btn${tab === t.id ? " is-active" : ""}`} aria-current={tab === t.id ? "page" : undefined} onClick={() => selectTab(t.id)}>
            {t.icon}
            {t.label}
          </button>
        ))}
      </nav>

      {tab === "conexao" ? (
        <ToolPanel
          icon={<PlugZap size={20} />}
          title="Conexão com a HubSoft"
          subtitle={
            <>
              Credenciais criadas por um administrador na HubSoft (client_id, client_secret, usuário e senha da API). Documentação:{" "}
              <a href="https://docs.hubsoft.com.br" target="_blank" rel="noreferrer">
                docs.hubsoft.com.br
              </a>
              .
            </>
          }
        >
          <div className="hsa-panel__body hsa-panel__body--pad">
            <IntegrationLogoField slug={slug} logoUrl={d.logo_url} />

            <div className="hsa-form-grid">
              <div className="field field--wide">
                <label>URL da API</label>
                <input className="input mono" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.seudominio.hubsoft.com.br" />
              </div>
              <div className="field">
                <label>Client ID</label>
                <input className="input mono" value={clientId} onChange={(e) => setClientId(e.target.value)} />
              </div>
              <div className="field">
                <label>
                  Client Secret {secretConfigured ? <span style={{ color: "var(--muted)", fontWeight: 400 }}>(já configurado — deixe em branco para manter)</span> : null}
                </label>
                <input className="input mono" type="password" value={clientSecret} onChange={(e) => setClientSecret(e.target.value)} placeholder={secretConfigured ? "••••••••" : ""} />
              </div>
              <div className="field">
                <label>Username</label>
                <input className="input mono" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="api@provedor.com.br" />
              </div>
              <div className="field">
                <label>
                  Password {passwordConfigured ? <span style={{ color: "var(--muted)", fontWeight: 400 }}>(já configurado — deixe em branco para manter)</span> : null}
                </label>
                <input className="input mono" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={passwordConfigured ? "••••••••" : ""} />
              </div>
            </div>

            <div>
              <label className="toggle" style={{ display: "inline-flex" }}>
                <span className="toggle__track">
                  <input
                    type="checkbox"
                    role="switch"
                    className="toggle__input"
                    checked={preloadOnStartup}
                    onChange={(e) => {
                      setPreloadOnStartup(e.target.checked);
                      preloadM.mutate(e.target.checked);
                    }}
                  />
                  <span className="toggle__thumb" aria-hidden />
                </span>
                <span className="toggle__label" style={{ display: "inline" }}>
                  Carregar dados ao iniciar o sistema
                </span>
              </label>
              <p className="hsa-muted" style={{ margin: "4px 0 0 52px", maxWidth: "72ch" }}>
                Ligado: o servidor coleta atendimentos, O.S. e financeiro recentes assim que arranca, em segundo plano — quem abrir a tela de
                Integrações já encontra os dados prontos. Desligado (padrão): só carrega quando alguém entra na tela.
              </p>
            </div>

            <div className="hsa-actions">
              <button type="button" className="btn btn--primary" disabled={busy || !baseUrl.trim() || !clientId.trim() || !username.trim()} onClick={() => saveM.mutate()}>
                {saveM.isPending ? "Salvando…" : "Salvar"}
              </button>
              <button
                type="button"
                className="btn"
                disabled={busy}
                onClick={() => {
                  setTestResult(null);
                  testM.mutate();
                }}
              >
                {testM.isPending ? "Testando…" : "Testar API"}
              </button>
              {d.last_test_at ? <span className="hsa-muted">Último teste: {new Date(d.last_test_at).toLocaleString("pt-BR")}</span> : null}
            </div>

            {testResult ? (
              <div className={`msg ${testResult.ok ? "msg--ok" : "msg--err"}`} style={{ display: "flex", alignItems: "flex-start", gap: 8 }}>
                {testResult.ok ? <CheckCircle2 size={16} style={{ flexShrink: 0, marginTop: 2 }} /> : <XCircle size={16} style={{ flexShrink: 0, marginTop: 2 }} />}
                <span>
                  {testResult.message}
                  {testResult.latency_ms != null ? ` (${testResult.latency_ms} ms)` : ""}
                </span>
              </div>
            ) : null}
          </div>
        </ToolPanel>
      ) : null}

      {tab === "importar" ? <HubsoftBulkImportSection /> : null}
      {tab === "conferir" ? <HubsoftConference /> : null}
      {tab === "senhas" ? <HubsoftPasswordFix /> : null}
      {tab === "enderecos" ? <HubsoftAddressCheck /> : null}
      {tab === "estoque" ? <HubsoftStock /> : null}
      {tab === "data-venda" ? <HubsoftDataVendaSection /> : null}
      {tab === "catalogos" ? <HubsoftCatalogExplorer /> : null}
      {tab === "historico" ? <HubsoftBulkImportHistory /> : null}
      {tab === "ixc-logins" ? <HubsoftIxcLogins /> : null}
    </div>
  );
}
