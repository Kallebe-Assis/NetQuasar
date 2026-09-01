import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { CheckCircle2, ImageUp, Link as LinkIcon, PlugZap, Trash2, XCircle } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import type { IntegrationDetail } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { PageToastHost, usePageToast } from "../../lib/pageToast";
import { queryKeys } from "../../lib/queryKeys";
import { HUBSOFT_LOGO_MAX_BYTES, clearHubsoftLogo, useHubsoftLogo, writeHubsoftLogo } from "../../lib/hubsoftLogo";

type TestOutcome = { ok: boolean; message: string; latency_ms?: number };

/**
 * Configuração simplificada da HubSoft: só os campos comuns a qualquer integração
 * (URL, credenciais) + um botão único que salva e testa a ligação de verdade (login
 * OAuth2 + uma chamada real de API — não só um GET de conectividade).
 */
export function HubsoftConfigPage() {
  const slug = "hubsoft";
  const qc = useQueryClient();
  const { toast, show: showToast, dismiss: dismissToast } = usePageToast();

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
  const logo = useHubsoftLogo();
  const logoInputRef = useRef<HTMLInputElement>(null);
  const [logoUrl, setLogoUrl] = useState("");

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

  function handleLogoFile(file: File | null) {
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      showToast("err", "Selecione um ficheiro de imagem.");
      return;
    }
    if (file.size > HUBSOFT_LOGO_MAX_BYTES) {
      showToast("err", `Imagem muito grande (máx. ${Math.round(HUBSOFT_LOGO_MAX_BYTES / 1024)}KB).`);
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") writeHubsoftLogo(reader.result);
    };
    reader.onerror = () => showToast("err", "Falha ao ler a imagem.");
    reader.readAsDataURL(file);
  }

  function applyLogoUrl() {
    const url = logoUrl.trim();
    if (!/^https?:\/\/.+/i.test(url)) {
      showToast("err", "Cole um link http(s) válido de uma imagem.");
      return;
    }
    writeHubsoftLogo(url);
    setLogoUrl("");
  }

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

  return (
    <div>
      <HubsoftHeader />
      <PageToastHost toast={toast} onDismiss={dismissToast} />

      <div className="card" style={{ maxWidth: 560 }}>
        <h2 style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
          <PlugZap size={18} aria-hidden /> Conexão com a HubSoft
        </h2>
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: -4 }}>
          Credenciais criadas por um administrador no HubSoft (client_id/client_secret/usuário/senha da API). Ver{" "}
          <a href="https://docs.hubsoft.com.br" target="_blank" rel="noreferrer">
            docs.hubsoft.com.br
          </a>
          .
        </p>

        <div className="field">
          <label>Logo</label>
          <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
            {logo ? <img src={logo} alt="" style={{ height: 32, maxWidth: 120, objectFit: "contain" }} /> : null}
            <input
              ref={logoInputRef}
              type="file"
              accept="image/*"
              style={{ display: "none" }}
              onChange={(e) => handleLogoFile(e.target.files?.[0] ?? null)}
            />
            <button type="button" className="btn btn--sm" onClick={() => logoInputRef.current?.click()}>
              <ImageUp size={13} /> {logo ? "Trocar logo" : "Enviar logo"}
            </button>
            {logo ? (
              <button type="button" className="btn btn--sm" onClick={clearHubsoftLogo}>
                <Trash2 size={13} /> Remover
              </button>
            ) : null}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 8 }}>
            <input
              className="input mono"
              style={{ flex: 1 }}
              value={logoUrl}
              onChange={(e) => setLogoUrl(e.target.value)}
              placeholder="…ou cole o link de uma imagem (https://…)"
              onKeyDown={(e) => {
                if (e.key === "Enter") applyLogoUrl();
              }}
            />
            <button type="button" className="btn btn--sm" disabled={!logoUrl.trim()} onClick={applyLogoUrl}>
              <LinkIcon size={13} /> Usar link
            </button>
          </div>
        </div>

        <div className="field">
          <label>URL da API</label>
          <input className="input mono" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.seudominio.hubsoft.com.br" />
        </div>
        <div className="field">
          <label>Client ID</label>
          <input className="input mono" value={clientId} onChange={(e) => setClientId(e.target.value)} />
        </div>
        <div className="field">
          <label>Client Secret {secretConfigured ? <span style={{ color: "var(--muted)", fontWeight: 400 }}>(já configurado — deixe em branco para manter)</span> : null}</label>
          <input className="input mono" type="password" value={clientSecret} onChange={(e) => setClientSecret(e.target.value)} placeholder={secretConfigured ? "••••••••" : ""} />
        </div>
        <div className="field">
          <label>Username</label>
          <input className="input mono" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="api@provedor.com.br" />
        </div>
        <div className="field">
          <label>Password {passwordConfigured ? <span style={{ color: "var(--muted)", fontWeight: 400 }}>(já configurado — deixe em branco para manter)</span> : null}</label>
          <input className="input mono" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={passwordConfigured ? "••••••••" : ""} />
        </div>

        <div style={{ marginTop: 14, marginBottom: "0.65rem" }}>
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
            <span className="toggle__label" style={{ display: "inline" }}>Carregar dados ao iniciar o sistema</span>
          </label>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0 52px" }}>
            Ligado: o servidor coleta atendimentos/O.S./financeiro recentes assim que arranca, em segundo plano — quem
            abrir a tela de Integrações já encontra os dados prontos. Desligado (padrão): só carrega quando alguém
            entra na tela.
          </p>
        </div>

        <div className="row" style={{ gap: 8, marginTop: 14, alignItems: "center", flexWrap: "wrap" }}>
          <button
            type="button"
            className="btn btn--primary"
            disabled={busy || !baseUrl.trim() || !clientId.trim() || !username.trim()}
            onClick={() => saveM.mutate()}
          >
            {saveM.isPending ? "A salvar…" : "Salvar"}
          </button>
          <button type="button" className="btn" disabled={busy} onClick={() => { setTestResult(null); testM.mutate(); }}>
            {testM.isPending ? "A testar…" : "Testar API"}
          </button>
          {d.last_test_at ? (
            <span style={{ fontSize: 11, color: "var(--muted)" }}>
              Último teste: {new Date(d.last_test_at).toLocaleString("pt-PT")}
            </span>
          ) : null}
        </div>

        {testResult ? (
          <div
            className={`msg ${testResult.ok ? "msg--ok" : "msg--err"}`}
            style={{ marginTop: 12, display: "flex", alignItems: "flex-start", gap: 8 }}
          >
            {testResult.ok ? <CheckCircle2 size={16} style={{ flexShrink: 0, marginTop: 2 }} /> : <XCircle size={16} style={{ flexShrink: 0, marginTop: 2 }} />}
            <span>
              {testResult.message}
              {testResult.latency_ms != null ? ` (${testResult.latency_ms} ms)` : ""}
            </span>
          </div>
        ) : null}
      </div>
    </div>
  );
}
