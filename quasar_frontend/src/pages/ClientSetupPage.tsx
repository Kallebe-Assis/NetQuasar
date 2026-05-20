import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { LoginCircuitBackdrop } from "../components/LoginCircuitBackdrop";
import { getApiBase, markClientConfigured, saveSession } from "../lib/auth";

/** Valida /api/v1/health com a URL e chave salvas em memória (antes de persistir). */
async function pingHealth(apiBase: string, apiKey: string): Promise<void> {
  const base = apiBase.replace(/\/$/, "");
  const path = "/api/v1/health";
  const url = base ? `${base}${path}` : path;
  const headers: Record<string, string> = {};
  if (apiKey.trim()) headers["X-API-Key"] = apiKey.trim();
  const res = await fetch(url, { headers });
  if (!res.ok) {
    const t = await res.text().catch(() => "");
    throw new Error(t || `HTTP ${res.status}`);
  }
}

export function ClientSetupPage() {
  const nav = useNavigate();
  const [apiBase, setApiBase] = useState(() => localStorage.getItem("netquasar_api_base") ?? getApiBase());
  const [apiKey, setApiKey] = useState(() => localStorage.getItem("netquasar_api_key") ?? "");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setLoading(true);
    try {
      await pingHealth(apiBase.trim(), apiKey.trim());
      saveSession(apiBase.trim(), apiKey.trim());
      markClientConfigured();
      nav("/config-setup", { replace: true });
    } catch (x) {
      setErr(x instanceof Error ? x.message : String(x));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="login-page">
      <LoginCircuitBackdrop />
      <div className="login-box">
        <h1 style={{ marginBottom: "0.25rem" }}>NetQuasar</h1>
        <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
          Primeira configuração neste dispositivo: indique onde o <strong>backend</strong> está a correr e, se o servidor exigir, a{" "}
          <strong>X-API-Key</strong> (<code className="mono">NETQUASAR_API_KEYS</code>). Só aparece uma vez por browser até limpar dados do site.
        </p>
        {err ? <div className="msg msg--err">{err}</div> : null}
        <form onSubmit={onSubmit}>
          <div className="field">
            <label>URL base da API (vazio = mesma origem / proxy Vite em dev)</label>
            <input className="input" style={{ width: "100%" }} value={apiBase} onChange={(e) => setApiBase(e.target.value)} placeholder="http://127.0.0.1:8080" />
          </div>
          <div className="field">
            <label>API Key (opcional)</label>
            <input className="input" style={{ width: "100%" }} type="password" value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="NETQUASAR_API_KEYS" />
          </div>
          <button className="btn btn--primary" type="submit" disabled={loading} style={{ width: "100%", marginTop: 8 }}>
            {loading ? "A validar…" : "Continuar"}
          </button>
        </form>
      </div>
    </div>
  );
}
