import { useMutation } from "@tanstack/react-query";
import { useState, type CSSProperties } from "react";
import { apiFetch, ApiError } from "../lib/api";

type DbTestResponse = { ok?: boolean; message?: string };

function validateDbUrlFormat(url: string): string | null {
  const t = url.trim();
  if (!t) return null;
  if (!/^postgres(ql)?:\/\//i.test(t)) {
    return "O endereço completo (URL) deve começar por postgres:// ou postgresql://.";
  }
  return null;
}

function supabaseDbHostIncompleteMessage(host: string): string | null {
  const t = host.trim().toLowerCase();
  if (!t.startsWith("db.")) return null;
  if (t.endsWith(".supabase.co")) return null;
  const withoutDb = t.slice(3);
  const parts = withoutDb.split(".");
  if (parts.length < 2) {
    return "O servidor está incompleto: o host da Supabase tem de ser db.SEU_REF.supabase.co (com .supabase.co no fim). Copie o valor completo do painel.";
  }
  if (parts.length === 2 && parts[1].length <= 3 && parts[1] !== "supabase") {
    return "O servidor parece truncado (falta supabase.co). Confirme db.SEU_REF.supabase.co inteiro. A partir do Docker, use a URI do Session pooler em “URL completa” (Connect → Session).";
  }
  return null;
}

function isAllowedSupabasePostgresHost(host: string): boolean {
  const h = host.trim().toLowerCase();
  if (!h.includes("supabase")) return true;
  return h.endsWith(".supabase.co") || h.endsWith(".pooler.supabase.com");
}

function setupErrMessage(err: unknown): string {
  if (!(err instanceof ApiError)) {
    return "Não foi possível concluir a requisição. Verifique a ligação e tente novamente.";
  }
  const body = err.body;
  if (body != null && typeof body === "object") {
    const d = (body as { details?: unknown }).details;
    if (d != null && typeof d === "object") {
      const hint = (d as { hint?: unknown }).hint;
      if (typeof hint === "string" && hint.trim()) return hint.trim();
    }
  }
  return err.message || "Erro desconhecido.";
}

type Props = { onConfigured: () => void };

export function FirstRunDatabaseSetup({ onConfigured }: Props) {
  const [host, setHost] = useState("");
  const [port, setPort] = useState("5432");
  const [dbUser, setDbUser] = useState("");
  const [dbName, setDbName] = useState("");
  const [sslMode, setSslMode] = useState("require");
  const [dbPass, setDbPass] = useState("");
  const [dbUrl, setDbUrl] = useState("");
  const [toast, setToast] = useState<{ ok: boolean; text: string } | null>(null);

  const testConn = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      apiFetch<DbTestResponse>("/api/v1/setup/database/test", { method: "POST", json: body }),
    onSuccess: (data) => {
      const msg = typeof data?.message === "string" ? data.message : "";
      setToast({
        ok: true,
        text: msg.toLowerCase().includes("url") ? "Ligação bem-sucedida com a URL indicada." : "Ligação bem-sucedida com os dados preenchidos.",
      });
    },
    onError: (e) => setToast({ ok: false, text: setupErrMessage(e) }),
  });

  const apply = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      apiFetch<{ ok?: boolean }>("/api/v1/setup/database/apply", { method: "POST", json: body }),
    onSuccess: () => {
      setToast({ ok: true, text: "Base de dados configurada. A carregar a aplicação…" });
      onConfigured();
    },
    onError: (e) => setToast({ ok: false, text: setupErrMessage(e) }),
  });

  const buildBody = (): Record<string, unknown> => {
    if (dbUrl.trim()) {
      return { database_url: dbUrl.trim() };
    }
    const body: Record<string, unknown> = {
      host: host.trim(),
      port: Number(port),
      db_user: dbUser.trim(),
      db_name: dbName.trim(),
      ssl_mode: sslMode.trim() || "require",
      db_password: dbPass,
    };
    return body;
  };

  const runValidate = (): boolean => {
    const urlErr = validateDbUrlFormat(dbUrl);
    if (urlErr) {
      setToast({ ok: false, text: urlErr });
      return false;
    }
    const incompleteHost = supabaseDbHostIncompleteMessage(host);
    if (incompleteHost) {
      setToast({ ok: false, text: incompleteHost });
      return false;
    }
    const hostNorm = host.trim().toLowerCase();
    if (hostNorm.includes("supabase") && !isAllowedSupabasePostgresHost(hostNorm)) {
      setToast({
        ok: false,
        text: "Para Supabase use um host completo: db.…supabase.co ou ….pooler.supabase.com (Session pooler). Copie do painel Connect.",
      });
      return false;
    }
    const urlTrim = dbUrl.trim().toLowerCase();
    if (
      urlTrim.startsWith("postgres") &&
      urlTrim.includes("supabase") &&
      !urlTrim.includes(".supabase.co") &&
      !urlTrim.includes("pooler.supabase.com")
    ) {
      setToast({
        ok: false,
        text: "Na URL, o host Supabase deve incluir db.….supabase.co ou ….pooler.supabase.com.",
      });
      return false;
    }
    if (!dbUrl.trim()) {
      const missing: string[] = [];
      if (!host.trim()) missing.push("servidor");
      const p = port.trim();
      if (!p || Number.isNaN(Number(p)) || Number(p) <= 0) missing.push("porta");
      if (!dbName.trim()) missing.push("nome da base");
      if (!dbUser.trim()) missing.push("utilizador");
      if (!dbPass.trim()) missing.push("palavra-passe");
      if (missing.length) {
        setToast({ ok: false, text: `Preencha: ${missing.join(", ")} (ou use só a URL completa).` });
        return false;
      }
    }
    return true;
  };

  const onTest = () => {
    if (!runValidate()) return;
    testConn.mutate(buildBody());
  };

  const onApply = () => {
    if (!runValidate()) return;
    apply.mutate(buildBody());
  };

  const fieldStyle: CSSProperties = { maxWidth: 560 };
  const sslChoice = sslMode.trim().toLowerCase() === "disable" ? "disable" : "require";

  return (
    <div className="modal-backdrop" style={{ zIndex: 80 }}>
      <div className="modal modal--wide" role="dialog" aria-modal="true" aria-labelledby="first-run-db-title">
        <h2 id="first-run-db-title" style={{ marginTop: 0 }}>
          Configuração inicial da base de dados
        </h2>
        <p style={{ color: "var(--muted)", fontSize: 13, lineHeight: 1.45, marginTop: 0 }}>
          Este servidor ainda não tem PostgreSQL configurado. Indique a ligação da sua empresa, teste e aplique. As credenciais são gravadas em{" "}
          <code className="mono">data/database-credentials.json</code> (ou em <code className="mono">NETQUASAR_DATA_DIR</code>) para reinícios futuros.
        </p>
        {toast ? <div className={toast.ok ? "msg msg--ok" : "msg msg--err"}>{toast.text}</div> : null}

        <h3 style={{ fontSize: 14, marginTop: 16, marginBottom: 8 }}>URL completa (opcional)</h3>
        <div className="field" style={{ width: "100%", maxWidth: "min(100%, 920px)" }}>
          <label htmlFor="first-db-url">postgres://…</label>
          <input
            id="first-db-url"
            className="input mono"
            style={{ width: "100%", minHeight: 44, boxSizing: "border-box" }}
            value={dbUrl}
            onChange={(e) => setDbUrl(e.target.value)}
            placeholder="postgresql://utilizador:senha@host:5432/nome_bd?sslmode=require"
            autoComplete="off"
            spellCheck={false}
          />
        </div>

        <h3 style={{ fontSize: 14, marginTop: 20, marginBottom: 8 }}>Ou parâmetros separados</h3>
        <div className="field" style={{ width: "100%", maxWidth: "min(100%, 920px)" }}>
          <label htmlFor="first-db-host">Servidor</label>
          <input
            id="first-db-host"
            className="input mono"
            style={{ width: "100%", minHeight: 44, boxSizing: "border-box" }}
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="host ou IP"
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div className="field" style={fieldStyle}>
          <label htmlFor="first-db-port">Porta</label>
          <input id="first-db-port" className="input" style={{ maxWidth: 120 }} value={port} onChange={(e) => setPort(e.target.value)} inputMode="numeric" />
        </div>
        <div className="field" style={fieldStyle}>
          <label htmlFor="first-db-user">Utilizador</label>
          <input id="first-db-user" className="input" value={dbUser} onChange={(e) => setDbUser(e.target.value)} autoComplete="off" />
        </div>
        <div className="field" style={fieldStyle}>
          <label htmlFor="first-db-name">Nome da base</label>
          <input id="first-db-name" className="input" value={dbName} onChange={(e) => setDbName(e.target.value)} autoComplete="off" />
        </div>
        <div className="field" style={fieldStyle}>
          <label htmlFor="first-db-ssl">SSL</label>
          <select id="first-db-ssl" className="input" value={sslChoice} onChange={(e) => setSslMode(e.target.value)}>
            <option value="require">require</option>
            <option value="disable">disable</option>
          </select>
        </div>
        <div className="field" style={fieldStyle}>
          <label htmlFor="first-db-pass">Palavra-passe</label>
          <input id="first-db-pass" className="input" type="password" value={dbPass} onChange={(e) => setDbPass(e.target.value)} autoComplete="new-password" />
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" onClick={onTest} disabled={testConn.isPending || apply.isPending}>
            {testConn.isPending ? "A testar…" : "Testar ligação"}
          </button>
          <button type="button" className="btn btn--primary" onClick={onApply} disabled={testConn.isPending || apply.isPending}>
            {apply.isPending ? "A aplicar…" : "Aplicar e continuar"}
          </button>
        </div>
      </div>
    </div>
  );
}
