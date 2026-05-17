import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type CSSProperties } from "react";
import { Blend, ClockFading, Cpu, Sun, ThermometerSun } from "lucide-react";
import { InfoHint } from "../components/InfoHint";
import { apiFetch, ApiError } from "../lib/api";
import { invalidateAlertListQueries, queryKeys } from "../lib/queryKeys";
import { AppearancePanel } from "./settings/AppearancePanel";
import { MonitoringPingIntervalsCard } from "./settings/MonitoringIntervalsCard";
import { OnuMonthlyReportPanel } from "./settings/OnuMonthlyReportPanel";
import { formatBRPhoneDisplay, normalizeBRPhoneForApi, validateBRPhoneMessage } from "../lib/brPhone";

type SettingsTab =
  | "database"
  | "logs"
  | "users"
  | "alerts"
  | "appearance"
  | "connection"
  | "telegram"
  | "olt"
  | "automation";

export function SettingsPage() {
  const [tab, setTab] = useState<SettingsTab>("database");
  return (
    <>
      <h1>Configurações</h1>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Base de dados, utilizadores, credenciais de rede, Telegram (alertas e relatórios), fabricantes OLT e relatórios automáticos.
      </p>
      <div className="tabs" style={{ flexWrap: "wrap" }}>
        {(
          [
            ["database", "Base de dados"],
            ["logs", "Auditoria"],
            ["users", "Usuários"],
            ["alerts", "Alertas"],
            ["appearance", "Aparência"],
            ["connection", "Rede e SNMP"],
            ["telegram", "Telegram"],
            ["olt", "OLT vendors"],
            ["automation", "Relatório ONU"],
          ] as const
        ).map(([k, lab]) => (
          <button key={k} type="button" className={tab === k ? "active" : ""} onClick={() => setTab(k)}>
            {lab}
          </button>
        ))}
      </div>
      {tab === "database" && <DatabasePanel />}
      {tab === "logs" && <LogsPanel />}
      {tab === "users" && <UsersPanel />}
      {tab === "alerts" && (
        <>
          <MonitoringPingIntervalsCard />
          <AlertThresholdsPanel />
        </>
      )}
      {tab === "appearance" && <AppearancePanel />}
      {tab === "connection" && <ConnectionPanel />}
      {tab === "telegram" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <TelegramPanel id="monitoring" title="Monitorização (alertas)" />
          <TelegramPanel id="reports" title="Relatórios" />
        </div>
      )}
      {tab === "olt" && <OltVendorsPanel />}
      {tab === "automation" && <OnuMonthlyReportPanel />}
    </>
  );
}

type DbMeta = {
  host: string | null;
  port: number | null;
  db_user_masked: unknown;
  db_name: string | null;
  ssl_mode: string | null;
  password_configured: boolean;
  active_dsn_source: string;
  note?: string;
};

function hasMaskedDbUser(meta: DbMeta | undefined): boolean {
  if (!meta) return false;
  const m = meta.db_user_masked;
  if (m == null) return false;
  if (typeof m === "string") return m.trim().length > 0;
  return true;
}

function friendlyDbTestSuccessMessage(serverMessage: string): string {
  const m = serverMessage.toLowerCase();
  if (m.includes("url") && (m.includes("informada") || m.includes("bem-suced"))) {
    return "Ligação bem-sucedida com o endereço completo (URL) que indicou.";
  }
  if (m.includes("parâmetros") || m.includes("parametros")) {
    return "Ligação bem-sucedida: o servidor aceitou os dados de acesso que preencheu.";
  }
  if (m.includes("ping") || m.includes("pool atual")) {
    return "A base de dados que está em uso neste momento respondeu corretamente.";
  }
  return "Ligação à base de dados bem-sucedida.";
}

/** Texto extra devolvido pelo backend em `details.hint` (ex.: Supabase + Docker + IPv6). */
function dbErrorDetailsHint(err: unknown): string | null {
  if (!(err instanceof ApiError) || err.body == null || typeof err.body !== "object") return null;
  const d = (err.body as { details?: unknown }).details;
  if (!d || typeof d !== "object") return null;
  const hint = (d as { hint?: unknown }).hint;
  return typeof hint === "string" && hint.trim() ? hint.trim() : null;
}

function friendlyDbConnectionError(err: unknown): string {
  if (!(err instanceof ApiError)) {
    return "Não foi possível concluir o pedido. Verifique a ligação à internet e tente novamente.";
  }
  const hint = dbErrorDetailsHint(err);
  if (hint) return hint;
  const raw = (err.message || "").toLowerCase();
  const code = (err.code || "").toUpperCase();

  if (code === "VALIDATION" || raw.includes("informe host") || raw.includes("db_password")) {
    return "Falta informação para testar: são necessários o servidor, a porta, o nome da base, o utilizador e a palavra-passe. Se já guardou a palavra-passe antes, pode deixar esse campo vazio e voltar a testar. Pode também usar só o campo “URL completa”.";
  }
  if (code === "NO_DB") {
    return "O serviço de base de dados não está disponível neste momento. Tente reiniciar a aplicação.";
  }
  if (raw.includes("authentication failed") || raw.includes("password authentication")) {
    return "O servidor recusou o utilizador ou a palavra-passe. Confirme as credenciais da base de dados.";
  }
  if (raw.includes("connection refused")) {
    return "O servidor recusou a ligação na porta indicada. Verifique se o PostgreSQL está a correr e se a porta está correta.";
  }
  if (raw.includes("no such host") || raw.includes("name or service not known")) {
    return "Não encontrámos esse endereço de servidor. Confirme o nome ou o IP.";
  }
  if (raw.includes("timeout") || raw.includes("deadline exceeded") || raw.includes("i/o timeout")) {
    return "A ligação demorou demasiado. Verifique rede, firewall e se o servidor está acessível.";
  }
  if (raw.includes("does not exist") && raw.includes("database")) {
    return "Essa base de dados não existe neste servidor. Confirme o nome da base.";
  }
  if (raw.includes("ssl") || raw.includes("tls") || raw.includes("certificate")) {
    return "Há um problema com a ligação segura (SSL). Experimente “require” ou “disable” no modo SSL, conforme o seu fornecedor de base de dados.";
  }
  if (
    (code === "TEST_FAILED" || code === "PING_FAILED" || code === "MIGRATE_FAILED" || code === "CONNECT_FAILED") &&
    (raw.includes("network is unreachable") || raw.includes("no route to host")) &&
    (raw.includes("dial tcp [") || raw.includes("dial tcp6 ["))
  ) {
    return "Falha de rede IPv6 até ao Postgres. Use o Session pooler (….pooler.supabase.com) no painel Supabase ou ative IPv6 no Docker.";
  }
  if (code === "TEST_FAILED" || code === "PING_FAILED" || code === "MIGRATE_FAILED" || code === "CONNECT_FAILED") {
    return "Não foi possível ligar. Confirme servidor, porta, utilizador, palavra-passe e nome da base.";
  }
  return "Não foi possível ligar à base de dados. Revise os dados e tente novamente.";
}

function friendlyDbPatchError(err: unknown): string {
  if (!(err instanceof ApiError)) return "Não foi possível guardar. Tente novamente.";
  const raw = (err.message || "").toLowerCase();
  if (raw.includes("database_url") && raw.includes("apply_connection")) {
    return "Para usar uma URL completa tem de marcar a opção “Aplicar já esta ligação”.";
  }
  return friendlyDbConnectionError(err);
}

function validateDbUrlFormat(url: string): string | null {
  const t = url.trim();
  if (!t) return null;
  if (!/^postgres(ql)?:\/\//i.test(t)) {
    return "O endereço completo (URL) deve começar por postgres:// ou postgresql://.";
  }
  return null;
}

/** db.<ref> sem domínio completo .supabase.co (ex.: …truncado em …s) — a validação antiga não apanha porque falta a palavra "supabase". */
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

/** Host Supabase aceite nos campos: ligação direta ou pooler de sessão. */
function isAllowedSupabasePostgresHost(host: string): boolean {
  const h = host.trim().toLowerCase();
  if (!h.includes("supabase")) return true;
  return h.endsWith(".supabase.co") || h.endsWith(".pooler.supabase.com");
}

/** Campos em falta para um teste por dados (sem URL); considera o que já está gravado no sistema. */
function missingDbFieldsForTest(opts: {
  host: string;
  port: string;
  dbName: string;
  dbUser: string;
  dbPass: string;
  passwordConfigured: boolean;
  userKnownInSettings: boolean;
}): string[] {
  const missing: string[] = [];
  if (!opts.host.trim()) missing.push("servidor (endereço ou IP)");
  const p = opts.port.trim();
  if (!p || Number.isNaN(Number(p)) || Number(p) <= 0) missing.push("porta (em geral 5432)");
  if (!opts.dbName.trim()) missing.push("nome da base de dados");
  if (!opts.dbUser.trim() && !opts.userKnownInSettings) missing.push("utilizador da base de dados");
  if (!opts.dbPass.trim() && !opts.passwordConfigured) missing.push("palavra-passe da base de dados");
  return missing;
}

type DbTestResponse = { ok?: boolean; message?: string };

function DatabasePanel() {
  const qc = useQueryClient();
  const meta = useQuery({ queryKey: ["settings-db-meta"], queryFn: () => apiFetch<DbMeta>("/api/v1/settings/database") });
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [dbUser, setDbUser] = useState("");
  const [dbName, setDbName] = useState("");
  const [sslMode, setSslMode] = useState("");
  const [dbPass, setDbPass] = useState("");
  const [dbUrl, setDbUrl] = useState("");
  const [apply, setApply] = useState(false);
  const [dbToast, setDbToast] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (!meta.data) return;
    setHost(meta.data.host ?? "");
    setPort(meta.data.port != null ? String(meta.data.port) : "");
    setDbName(meta.data.db_name ?? "");
    const sm = (meta.data.ssl_mode ?? "").trim().toLowerCase();
    setSslMode(sm === "disable" ? "disable" : "require");
  }, [meta.data]);

  useEffect(() => {
    if (!dbToast) return;
    const t = window.setTimeout(() => setDbToast(null), 9000);
    return () => window.clearTimeout(t);
  }, [dbToast]);

  const patch = useMutation({
    mutationFn: (body: Record<string, unknown>) => apiFetch("/api/v1/settings/database", { method: "PATCH", json: body }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-db-meta"] });
      setDbToast({ ok: true, text: "Guardado com sucesso (base de dados)." });
    },
    onError: (e) => setDbToast({ ok: false, text: friendlyDbPatchError(e) }),
  });

  const testConn = useMutation({
    mutationFn: (body: Record<string, unknown>) => apiFetch<DbTestResponse>("/api/v1/settings/database/test", { method: "POST", json: body }),
    onSuccess: (data) => {
      const msg = typeof data?.message === "string" ? data.message : "";
      setDbToast({ ok: true, text: friendlyDbTestSuccessMessage(msg) });
    },
    onError: (e) => setDbToast({ ok: false, text: friendlyDbConnectionError(e) }),
  });

  if (meta.isLoading) return <p>A carregar metadados…</p>;
  if (meta.isError) return <div className="msg msg--err">{(meta.error as Error).message}</div>;

  const buildPatchBody = (): Record<string, unknown> => {
    const body: Record<string, unknown> = {};
    if (host.trim()) body.host = host.trim();
    if (port.trim()) body.port = Number(port);
    if (dbUser.trim()) body.db_user = dbUser.trim();
    if (dbName.trim()) body.db_name = dbName.trim();
    if (sslMode.trim()) body.ssl_mode = sslMode.trim();
    if (dbPass) body.db_password = dbPass;
    if (apply) body.apply_connection = true;
    if (dbUrl.trim()) {
      body.database_url = dbUrl.trim();
      body.apply_connection = true;
    }
    return body;
  };

  const runTestConnection = () => {
    const urlErr = validateDbUrlFormat(dbUrl);
    if (urlErr) {
      setDbToast({ ok: false, text: urlErr });
      return;
    }

    const incompleteHost = supabaseDbHostIncompleteMessage(host);
    if (incompleteHost) {
      setDbToast({ ok: false, text: incompleteHost });
      return;
    }

    const hostNorm = host.trim().toLowerCase();
    if (hostNorm.includes("supabase") && !isAllowedSupabasePostgresHost(hostNorm)) {
      setDbToast({
        ok: false,
        text: "Para Supabase use um host completo: db.…supabase.co (direto) ou ….pooler.supabase.com (session pooler). Copie do painel Connect.",
      });
      return;
    }
    const urlTrim = dbUrl.trim().toLowerCase();
    if (
      urlTrim.startsWith("postgres") &&
      urlTrim.includes("supabase") &&
      !urlTrim.includes(".supabase.co") &&
      !urlTrim.includes("pooler.supabase.com")
    ) {
      setDbToast({
        ok: false,
        text: "Na URL, o host Supabase deve incluir db.….supabase.co ou ….pooler.supabase.com (copie do painel). Não use o URL https:// do painel.",
      });
      return;
    }

    const b = buildPatchBody();
    delete b.apply_connection;

    if (dbUrl.trim()) {
      testConn.mutate({ database_url: dbUrl.trim() });
      return;
    }

    const keys = Object.keys(b);
    if (keys.length === 0) {
      testConn.mutate({});
      return;
    }

    const missing = missingDbFieldsForTest({
      host,
      port,
      dbName,
      dbUser,
      dbPass,
      passwordConfigured: !!meta.data?.password_configured,
      userKnownInSettings: hasMaskedDbUser(meta.data),
    });
    if (missing.length > 0) {
      setDbToast({
        ok: false,
        text: `Falta preencher: ${missing.join(", ")}. Depois volte a carregar em “Testar ligação”.`,
      });
      return;
    }

    testConn.mutate(b);
  };

  const fieldStyle: CSSProperties = { maxWidth: 560 };
  const hostFieldStyle: CSSProperties = { width: "100%", maxWidth: "min(100%, 920px)" };
  const sslChoice = sslMode.trim().toLowerCase() === "disable" ? "disable" : "require";

  return (
    <div className="card">
      <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
        Base de dados (PostgreSQL)
        <InfoHint label="Ligação à base de dados">
          <p>
            Estado: ligação em uso é{" "}
            <strong>{meta.data?.active_dsn_source === "env_NETQUASAR_DATABASE_URL" ? "variável de ambiente" : "definições guardadas"}</strong>
            {" · "}
            Palavra-passe na base de dados: <strong>{meta.data?.password_configured ? "já guardada" : "ainda não guardada"}</strong>
          </p>
          <p>
            Preencha os campos abaixo <strong>ou</strong> só o campo “URL completa”. Use “Testar ligação” para confirmar o acesso sem alterar o sistema; use
            “Guardar” para gravar (e “Aplicar já” apenas se souber o que faz — troca a ligação ativa).
          </p>
          <p>
            O host <span className="mono">db.…supabase.co</span> pode resolver só para IPv6; no Docker use o <strong>Session pooler</strong> (ex.:{" "}
            <span className="mono">aws-1-sa-east-1.pooler.supabase.com</span> — o painel indica <span className="mono">aws-0-</span> ou{" "}
            <span className="mono">aws-1-</span>) em “URL completa” ou nos campos. Com <strong>require</strong>, o teste usa o certificado CA incluído para ligações{" "}
            <span className="mono">db.*.supabase.co</span>.
          </p>
          <p>
            Se preencher o campo URL completa, o teste usa só a URL (não precisa dos campos de cima para testar). Para guardar uma nova URL é necessário
            marcar “Aplicar já esta ligação”.
          </p>
          <p>
            <strong>Docker / sem IPv6:</strong> cole a URI do <strong>Session pooler</strong> (Connect → Session): host <span className="mono">aws-0-</span> ou{" "}
            <span className="mono">aws-1-REGIÃO.pooler.supabase.com</span>, utilizador <span className="mono">postgres.SEU_REF</span>.
          </p>
        </InfoHint>
      </h2>

      <h3 style={{ fontSize: 14, marginTop: 16, marginBottom: 8 }}>Dados da ligação</h3>
      <div className="field" style={hostFieldStyle}>
        <label htmlFor="db-host">Servidor (host ou IP)</label>
        <input
          id="db-host"
          className="input mono"
          style={{
            fontSize: 14,
            width: "100%",
            minWidth: 0,
            minHeight: 48,
            padding: "10px 12px",
            boxSizing: "border-box",
          }}
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="db.….supabase.co ou aws-1-….pooler.supabase.com"
          autoComplete="off"
          spellCheck={false}
          title={host ? host : "Host completo"}
        />
        <p style={{ color: "var(--muted)", fontSize: 11, margin: "4px 0 0", lineHeight: 1.45 }}>
          Ligação direta: acaba em <span className="mono">.supabase.co</span>. Session pooler: acaba em{" "}
          <span className="mono">.pooler.supabase.com</span>. Copie o valor completo do painel (Connect).
        </p>
      </div>
      <div className="field" style={fieldStyle}>
        <label htmlFor="db-port">Porta</label>
        <input id="db-port" className="input" style={{ maxWidth: 120 }} value={port} onChange={(e) => setPort(e.target.value)} placeholder="5432" inputMode="numeric" autoComplete="off" />
      </div>
      <div className="field" style={fieldStyle}>
        <label htmlFor="db-user">Utilizador da base de dados</label>
        <input id="db-user" className="input" value={dbUser} onChange={(e) => setDbUser(e.target.value)} placeholder="nome de utilizador PostgreSQL" autoComplete="off" />
        {hasMaskedDbUser(meta.data) && (
          <p style={{ color: "var(--muted)", fontSize: 11, margin: "4px 0 0" }}>Já existe um utilizador guardado; pode deixar em branco para manter o atual.</p>
        )}
      </div>
      <div className="field" style={fieldStyle}>
        <label htmlFor="db-name">Nome da base de dados</label>
        <input id="db-name" className="input" value={dbName} onChange={(e) => setDbName(e.target.value)} placeholder="ex.: netquasar" autoComplete="off" />
      </div>
      <div className="field" style={fieldStyle}>
        <span id="db-ssl-label" style={{ display: "block", marginBottom: 8, fontWeight: 600, fontSize: 13 }}>
          Modo SSL
        </span>
        <div className="row" role="radiogroup" aria-labelledby="db-ssl-label" style={{ flexWrap: "wrap", gap: 16, alignItems: "center" }}>
          <label className="row" style={{ gap: 8, cursor: "pointer", fontSize: 14 }}>
            <input type="radio" name="db-ssl-mode" checked={sslChoice === "require"} onChange={() => setSslMode("require")} />
            <span>
              <strong>require</strong> — encriptado (Supabase, nuvem, Internet)
            </span>
          </label>
          <label className="row" style={{ gap: 8, cursor: "pointer", fontSize: 14 }}>
            <input type="radio" name="db-ssl-mode" checked={sslChoice === "disable"} onChange={() => setSslMode("disable")} />
            <span>
              <strong>disable</strong> — sem TLS (Postgres local / rede de confiança)
            </span>
          </label>
        </div>
      </div>
      <div className="field" style={fieldStyle}>
        <label htmlFor="db-pass">Palavra-passe da base de dados</label>
        <input id="db-pass" className="input" type="password" autoComplete="new-password" value={dbPass} onChange={(e) => setDbPass(e.target.value)} placeholder="não é mostrada depois de guardar" />
        {meta.data?.password_configured && (
          <p style={{ color: "var(--muted)", fontSize: 11, margin: "4px 0 0" }}>Já existe palavra-passe guardada; pode deixar em branco para testar com a guardada.</p>
        )}
      </div>

      <h3 style={{ fontSize: 14, marginTop: 20, marginBottom: 8 }}>URL completa (opcional)</h3>
      <div className="field" style={fieldStyle}>
        <label htmlFor="db-url">Endereço completo (connection string)</label>
        <input id="db-url" className="input mono" value={dbUrl} onChange={(e) => setDbUrl(e.target.value)} placeholder="postgres://utilizador:palavra-passe@servidor:5432/nome_da_base?sslmode=require" spellCheck={false} autoComplete="off" />
      </div>

      <label className="row" style={{ gap: 10, marginTop: 16, alignItems: "flex-start", maxWidth: 560 }}>
        <input type="checkbox" checked={apply} onChange={(e) => setApply(e.target.checked)} style={{ marginTop: 4 }} />
        <span style={{ fontSize: 13, lineHeight: 1.45 }}>
          <strong>Aplicar já esta ligação</strong> — valida, corre migrações no destino e passa a usar esta base em todo o sistema. Só marque se tiver a certeza dos dados.
        </span>
      </label>

      <div className="row" style={{ marginTop: 16, flexWrap: "wrap", gap: 8 }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate(buildPatchBody())}>
          Guardar definições
        </button>
        <button type="button" className="btn" disabled={testConn.isPending} onClick={runTestConnection}>
          Testar ligação
        </button>
      </div>

      {dbToast && (
        <div className={`page-toast ${dbToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 14, maxWidth: 560 }}>
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setDbToast(null)}>
            ×
          </button>
          {dbToast.text}
        </div>
      )}
    </div>
  );
}

function LogsPanel() {
  const [lim, setLim] = useState("100");
  const q = useQuery({
    queryKey: ["settings-db-logs", lim],
    queryFn: () => apiFetch<{ logs: { id: number; created_at: string; ok: boolean; phase: string; message: string }[] }>(
      `/api/v1/settings/database/logs?limit=${encodeURIComponent(lim)}`,
    ),
  });
  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;
  return (
    <div className="card">
      <h2>Auditoria de ligações</h2>
      <div className="row" style={{ marginBottom: 8 }}>
        <input className="input" style={{ width: 80 }} value={lim} onChange={(e) => setLim(e.target.value)} />
        <button type="button" className="btn" onClick={() => q.refetch()}>
          Atualizar
        </button>
      </div>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Quando</th>
              <th>OK</th>
              <th>Fase</th>
              <th>Mensagem</th>
            </tr>
          </thead>
          <tbody>
            {(q.data?.logs ?? []).map((l) => (
              <tr key={l.id}>
                <td>{l.id}</td>
                <td className="mono">{l.created_at}</td>
                <td>{l.ok ? "sim" : "não"}</td>
                <td>{l.phase}</td>
                <td>{l.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

type UserRow = {
  id: string;
  display_name: string;
  email: string;
  phone?: string | null;
  role: string;
};

function UsersPanel() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["settings-users"], queryFn: () => apiFetch<{ users: UserRow[] }>("/api/v1/settings/users") });
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "viewer">("viewer");
  const [editId, setEditId] = useState<string | null>(null);
  const [eName, setEName] = useState("");
  const [eEmail, setEEmail] = useState("");
  const [ePhone, setEPhone] = useState("");
  const [ePass, setEPass] = useState("");
  const [eRole, setERole] = useState<"admin" | "viewer">("viewer");
  const [saveToast, setSaveToast] = useState<{ ok: boolean; text: string } | null>(null);
  const [userCreateErr, setUserCreateErr] = useState("");

  const create = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/users", {
        method: "POST",
        json: {
          display_name: displayName.trim(),
          email: email.trim(),
          phone: normalizeBRPhoneForApi(phone),
          password,
          role,
        },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-users"] });
      setUserCreateErr("");
      setDisplayName("");
      setEmail("");
      setPhone("");
      setPassword("");
    },
  });

  const patch = useMutation({
    mutationFn: () => {
      const body: Record<string, string> = { role: eRole };
      if (eName.trim()) body.display_name = eName.trim();
      if (eEmail.trim()) body.email = eEmail.trim();
      body.phone = normalizeBRPhoneForApi(ePhone);
      if (ePass) body.password = ePass;
      return apiFetch(`/api/v1/settings/users/${editId}`, { method: "PATCH", json: body });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-users"] });
      setEditId(null);
      setSaveToast({ ok: true, text: "Guardado com sucesso (utilizador)." });
    },
    onError: (err) => setSaveToast({ ok: false, text: (err as Error).message || "Falha ao guardar (utilizador)." }),
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/settings/users/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings-users"] }),
  });

  if (list.isLoading) return <p>A carregar…</p>;
  if (list.isError) {
    const ae = list.error as ApiError;
    if (ae?.status === 403) {
      return <p style={{ color: "var(--muted)" }}>Apenas administradores podem gerir utilizadores.</p>;
    }
    return <div className="msg msg--err">{(list.error as Error).message}</div>;
  }

  return (
    <>
      <p style={{ color: "var(--muted)", fontSize: 13, marginTop: 0 }}>
        Novos utilizadores só podem ser criados aqui (não existe registo público). Campos: nome, e-mail, <strong>telefone com DDD</strong> (10 ou 11 dígitos), palavra-passe e nível{" "}
        <strong>administrador</strong> ou <strong>visitante (viewer)</strong>.
      </p>
      <div className="card">
        <h2>Novo utilizador</h2>
        <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "flex-end" }}>
          <div className="field" style={{ minWidth: 140 }}>
            <label style={{ fontSize: 11 }}>Nome</label>
            <input className="input" placeholder="Nome completo" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 180 }}>
            <label style={{ fontSize: 11 }}>E-mail</label>
            <input className="input" type="email" placeholder="email@empresa.com" value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 120 }}>
            <label style={{ fontSize: 11 }}>Telefone</label>
            <input className="input" placeholder="(11) 98765-4321" value={phone} onChange={(e) => setPhone(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 120 }}>
            <label style={{ fontSize: 11 }}>Palavra-passe</label>
            <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </div>
          <select className="input" style={{ maxWidth: 140 }} value={role} onChange={(e) => setRole(e.target.value as "admin" | "viewer")}>
            <option value="viewer">Visitante (viewer)</option>
            <option value="admin">Administrador</option>
          </select>
          <button
            type="button"
            className="btn btn--primary"
            disabled={create.isPending}
            onClick={() => {
              setUserCreateErr("");
              const pe = validateBRPhoneMessage(phone);
              if (pe) {
                setUserCreateErr(pe);
                return;
              }
              if (!displayName.trim() || !email.trim() || !password) {
                setUserCreateErr("Preencha nome, e-mail, telefone e palavra-passe.");
                return;
              }
              create.mutate();
            }}
          >
            Criar utilizador
          </button>
        </div>
        {userCreateErr ? <div className="msg msg--err">{userCreateErr}</div> : null}
        {create.isError && <div className="msg msg--err">{(create.error as Error).message}</div>}
      </div>
      <div className="table-wrap" style={{ marginTop: 12 }}>
        <table>
          <thead>
            <tr>
              <th>Nome</th>
              <th>E-mail</th>
              <th>Telefone</th>
              <th>Nível</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(list.data?.users ?? []).map((u) => (
              <tr key={u.id}>
                <td>{u.display_name}</td>
                <td className="mono">{u.email}</td>
                <td className="mono">{formatBRPhoneDisplay(u.phone)}</td>
                <td>{u.role === "admin" ? "Administrador" : "Visitante"}</td>
                <td>
                  <button
                    type="button"
                    className="btn"
                    onClick={() => {
                      setEditId(u.id);
                      setEName(u.display_name);
                      setEEmail(u.email);
                      setEPhone(u.phone ?? "");
                      setERole(u.role === "admin" ? "admin" : "viewer");
                      setEPass("");
                    }}
                  >
                    Editar
                  </button>{" "}
                  <button
                    type="button"
                    className="btn btn--danger"
                    onClick={() => {
                      if (confirm(`Eliminar ${u.email}?`)) del.mutate(u.id);
                    }}
                  >
                    Apagar
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {editId && (
        <div className="card" style={{ marginTop: 12 }}>
          <h2>Editar utilizador</h2>
          <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "flex-end" }}>
            <div className="field" style={{ minWidth: 140 }}>
              <label style={{ fontSize: 11 }}>Nome</label>
              <input className="input" value={eName} onChange={(e) => setEName(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 180 }}>
              <label style={{ fontSize: 11 }}>E-mail</label>
              <input className="input" type="email" value={eEmail} onChange={(e) => setEEmail(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 120 }}>
              <label style={{ fontSize: 11 }}>Telefone</label>
              <input className="input" value={ePhone} onChange={(e) => setEPhone(e.target.value)} placeholder="(11) 98765-4321" />
            </div>
            <div className="field" style={{ minWidth: 140 }}>
              <label style={{ fontSize: 11 }}>Nova palavra-passe (opcional)</label>
              <input className="input" type="password" value={ePass} onChange={(e) => setEPass(e.target.value)} />
            </div>
            <select className="input" style={{ maxWidth: 160 }} value={eRole} onChange={(e) => setERole(e.target.value as "admin" | "viewer")}>
              <option value="viewer">Visitante (viewer)</option>
              <option value="admin">Administrador</option>
            </select>
            <button
              type="button"
              className="btn btn--primary"
              disabled={patch.isPending}
              onClick={() => {
                const pe = validateBRPhoneMessage(ePhone);
                if (pe) {
                  setSaveToast({ ok: false, text: pe });
                  return;
                }
                patch.mutate();
              }}
            >
              Guardar
            </button>
            <button type="button" className="btn" onClick={() => setEditId(null)}>
              Cancelar
            </button>
          </div>
          {patch.isError && <div className="msg msg--err">{(patch.error as Error).message}</div>}
          {saveToast && (
            <div className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 8 }}>
              <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
                ×
              </button>
              {saveToast.text}
            </div>
          )}
        </div>
      )}
    </>
  );
}

type AlertThresholdMetric = {
  id: string;
  label: string;
  unit: string;
  scope: string;
  enabled?: boolean;
  operator: "gte" | "lte";
  green_min: string;
  warning_min: string;
  critical_min: string;
  /** Categorias de equipamento (base de dados) em minúsculas; vazio = todos. */
  apply_categories?: string[];
};

function equipScopeFromCategories(cats?: string[]): "*" | "olt" | "mikrotik" | "servidor" {
  const c = cats ?? [];
  if (c.length === 0) return "*";
  const low = c.map((x) => String(x).toLowerCase().trim());
  if (low.some((x) => x === "*" || x === "all" || x === "todos")) return "*";
  if (low.includes("olt") && low.length === 1) return "olt";
  if (low.includes("mikrotik") && low.length === 1) return "mikrotik";
  if (low.includes("servidor") || low.includes("outros")) return "servidor";
  return "*";
}

function categoriesFromEquipScope(scope: "*" | "olt" | "mikrotik" | "servidor"): string[] {
  switch (scope) {
    case "olt":
      return ["olt"];
    case "mikrotik":
      return ["mikrotik"];
    case "servidor":
      return ["servidor", "outros"];
    default:
      return [];
  }
}

function defaultAlertMetrics(): AlertThresholdMetric[] {
  return [
    { id: "cpu_usage_pct", label: "CPU utilizada", unit: "%", scope: "equipamento", enabled: true, operator: "gte", green_min: "50", warning_min: "75", critical_min: "90", apply_categories: [] },
    { id: "memory_usage_pct", label: "Memória utilizada", unit: "%", scope: "equipamento", enabled: true, operator: "gte", green_min: "55", warning_min: "75", critical_min: "90", apply_categories: [] },
    { id: "latency_ms", label: "Latência de resposta", unit: "ms", scope: "equipamento", enabled: true, operator: "gte", green_min: "50", warning_min: "120", critical_min: "220", apply_categories: [] },
    { id: "temperature_c", label: "Temperatura do equipamento", unit: "°C", scope: "equipamento", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: [] },
    { id: "uptime_minutes", label: "Uptime (minutos)", unit: "min", scope: "equipamento", enabled: true, operator: "lte", green_min: "120", warning_min: "60", critical_min: "15", apply_categories: [] },
    { id: "olt_pon_tx_dbm", label: "PON TX da OLT", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-8", warning_min: "-14", critical_min: "-20", apply_categories: ["olt"] },
    { id: "olt_pon_rx_dbm", label: "PON RX da OLT", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-10", warning_min: "-16", critical_min: "-22", apply_categories: ["olt"] },
    { id: "olt_pon_temp_c", label: "Temperatura da PON", unit: "°C", scope: "olt_pon", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: ["olt"] },
    { id: "olt_onu_drop_count", label: "Queda de ONUs online (por PON)", unit: "ONUs", scope: "olt_pon", enabled: true, operator: "gte", green_min: "0", warning_min: "2", critical_min: "5", apply_categories: ["olt"] },
    { id: "olt_onu_drop_percent", label: "Queda de ONUs online (%)", unit: "%", scope: "olt_pon", enabled: true, operator: "gte", green_min: "0", warning_min: "10", critical_min: "25", apply_categories: ["olt"] },
    { id: "iface_down_count", label: "Mudança de interface UP→DOWN", unit: "evento", scope: "interface", enabled: true, operator: "gte", green_min: "0", warning_min: "1", critical_min: "1", apply_categories: [] },
    { id: "mikrotik_sfp_tx_dbm", label: "SFP — potência TX", unit: "dBm", scope: "mikrotik_sfp", enabled: true, operator: "lte", green_min: "-8", warning_min: "-13", critical_min: "-18", apply_categories: ["mikrotik"] },
    { id: "mikrotik_sfp_rx_dbm", label: "SFP — potência RX", unit: "dBm", scope: "mikrotik_sfp", enabled: true, operator: "lte", green_min: "-10", warning_min: "-15", critical_min: "-20", apply_categories: ["mikrotik"] },
    { id: "mikrotik_sfp_temp_c", label: "Temperatura do módulo SFP", unit: "°C", scope: "mikrotik_sfp", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: ["mikrotik"] },
  ];
}

function AlertThresholdsPanel() {
  type RuleRow = {
    id: string;
    name: string;
    enabled: boolean;
    condition?: unknown;
  };
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-alert-threshold-rules"],
    queryFn: () => apiFetch<{ rules: RuleRow[] }>("/api/v1/alert-rules"),
  });
  const [rows, setRows] = useState<AlertThresholdMetric[]>(defaultAlertMetrics());
  const [enabled, setEnabled] = useState(true);
  const scopeOptions: { value: string; label: string }[] = [
    { value: "equipamento", label: "Equipamento" },
    { value: "olt_pon", label: "PON da OLT" },
    { value: "mikrotik_sfp", label: "SFP da MikroTik" },
    { value: "interface", label: "Interface de rede" },
    { value: "onu", label: "ONU" },
    { value: "custom", label: "Outro" },
  ];
  const metricKeyFromLabel = (label: string, fallback: string): string => {
    const normalized = String(label)
      .toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_+|_+$/g, "");
    return normalized || fallback;
  };

  const thresholdRule = (q.data?.rules ?? []).find((r) => r.name === "Limiar global de alertas");

  useEffect(() => {
    if (!thresholdRule) return;
    setEnabled(!!thresholdRule.enabled);
    const c = (thresholdRule.condition ?? {}) as { metrics?: AlertThresholdMetric[] };
    if (Array.isArray(c.metrics) && c.metrics.length > 0) {
      const parsed: AlertThresholdMetric[] = c.metrics.map((m, idx) => {
        const ac = (m as AlertThresholdMetric).apply_categories;
        const applyCats = Array.isArray(ac) ? ac.map((x) => String(x).toLowerCase().trim()) : [];
        return {
          id: String(m.id ?? "").trim() || `metrica_${idx + 1}`,
          label: String(m.label ?? "").trim(),
          unit: String(m.unit ?? "").trim(),
          scope: String(m.scope ?? "").trim(),
          enabled: m.enabled !== false,
          operator: (m.operator === "lte" ? "lte" : "gte") as "lte" | "gte",
          green_min: String(m.green_min ?? ""),
          warning_min: String(m.warning_min ?? ""),
          critical_min: String(m.critical_min ?? ""),
          apply_categories: applyCats,
        };
      });
      setRows(parsed);
    }
  }, [thresholdRule]);

  const upsert = useMutation({
    mutationFn: async () => {
      const payload = {
        schema: "netquasar.alert_thresholds.v1",
        metrics: rows
          .map((r, idx) => {
            const label = String(r.label).trim();
            const fallback = `metrica_${idx + 1}`;
            const cats = Array.isArray(r.apply_categories) ? r.apply_categories : [];
            return {
              ...r,
              id: String(r.id).trim() || metricKeyFromLabel(label, fallback),
              label,
              unit: String(r.unit).trim(),
              scope: String(r.scope).trim(),
              enabled: r.enabled !== false,
              green_min: String(r.green_min).trim(),
              warning_min: String(r.warning_min).trim(),
              critical_min: String(r.critical_min).trim(),
              apply_categories: cats,
            };
          })
          .filter((r) => r.label),
      };
      if (thresholdRule?.id) {
        return apiFetch(`/api/v1/alert-rules/${thresholdRule.id}`, {
          method: "PATCH",
          json: { name: "Limiar global de alertas", enabled, condition: payload },
        });
      }
      return apiFetch("/api/v1/alert-rules", {
        method: "POST",
        json: { name: "Limiar global de alertas", enabled, condition: payload, channels: {} },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settingsAlertThresholdRules });
      qc.invalidateQueries({ queryKey: queryKeys.alertRules });
      void invalidateAlertListQueries(qc);
    },
  });

  const updateRow = (idx: number, patch: Partial<AlertThresholdMetric>) =>
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, ...patch } : r)));

  const addRow = () =>
    setRows((prev) => [
      ...prev,
      {
        id: "",
        label: "",
        unit: "",
        scope: "equipamento",
        enabled: true,
        operator: "gte",
        green_min: "",
        warning_min: "",
        critical_min: "",
        apply_categories: [],
      },
    ]);

  const removeRow = (idx: number) => setRows((prev) => prev.filter((_, i) => i !== idx));

  const metricCatalog = defaultAlertMetrics();
  const addFromCatalog = (catalogId: string) => {
    const p = metricCatalog.find((m) => m.id === catalogId);
    if (!p) return;
    setRows((prev) => [
      ...prev,
      {
        ...p,
        id: p.id,
        label: p.label,
        apply_categories: [...(p.apply_categories ?? [])],
      },
    ]);
  };

  const equipScopeOptions: { value: "*" | "olt" | "mikrotik" | "servidor"; label: string }[] = [
    { value: "*", label: "Todos" },
    { value: "olt", label: "Somente OLT" },
    { value: "mikrotik", label: "Somente Mikrotik (Categoria)" },
    { value: "servidor", label: "Servidor e outros" },
  ];
  const unitOptions = ["%", "ms", "°C", "dBm", "min", "ONUs", "evt", "Mbps"];
  const [selectedCatalog, setSelectedCatalog] = useState("");

  const scopeLabel = (scope: string): string => scopeOptions.find((s) => s.value === scope)?.label ?? scope;
  const saveHint = "Salvo em banco na regra «Limiar global de alertas» (tabela alert_rules).";
  const metricIcon = (id: string) => {
    const k = String(id).toLowerCase();
    if (k.includes("mikrotik")) return <img src="/MT_Symbol_Black.svg" alt="" width={14} height={14} />;
    if (k.includes("cpu")) return <Cpu size={14} aria-hidden />;
    if (k.includes("temperature") || k.includes("temp")) return <ThermometerSun size={14} aria-hidden />;
    if (k.includes("uptime")) return <ClockFading size={14} aria-hidden />;
    if (k.includes("sfp") || k.includes("_tx_") || k.includes("_rx_")) return <Blend size={14} aria-hidden />;
    if (k.includes("olt") || k.includes("onu") || k.includes("pon")) return <Sun size={14} aria-hidden />;
    return <Cpu size={14} aria-hidden />;
  };

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card alert-rules-card">
      <div className="alert-rules-head">
        <div>
          <h2 style={{ marginBottom: 6 }}>Configuração de Alertas</h2>
          <p style={{ color: "var(--muted)", fontSize: 13, margin: 0 }}>
            Defina por linha: tipo de equipamento, métrica, operador (maior/menor) e faixas <span style={{ color: "#3fb950" }}>Normal</span>,{" "}
            <span style={{ color: "#d29922" }}>Atenção</span> e <span style={{ color: "#f85149" }}>Crítico</span>.
          </p>
        </div>
        <label className="row" style={{ gap: 8 }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          <span style={{ fontSize: 13 }}>Perfil ativo</span>
        </label>
      </div>

      <div className="alert-rules-toolbar">
        <div className="field" style={{ margin: 0, minWidth: 320 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>Adicionar métrica padrão</label>
          <select
            className="input"
            value={selectedCatalog}
            onChange={(e) => {
              const v = e.target.value;
              setSelectedCatalog(v);
              if (v) {
                addFromCatalog(v);
                setSelectedCatalog("");
              }
            }}
          >
            <option value="">Selecionar…</option>
            {metricCatalog.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label} ({scopeLabel(m.scope)})
              </option>
            ))}
          </select>
        </div>
        <button type="button" className="btn" onClick={addRow}>
          Novo critério
        </button>
      </div>

      <div className="alert-rules-grid-wrap">
        <table className="alert-rules-grid">
          <thead>
            <tr>
              <th>Métrica</th>
              <th>Equipamento</th>
              <th>Tipo de dado</th>
              <th>Condição</th>
              <th>Normal</th>
              <th>Atenção</th>
              <th>Crítico</th>
              <th>Habilitado</th>
              <th>Ações</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, idx) => (
              <tr key={`criterion-${idx}-${r.id || "new"}`}>
                <td>
                  <div className="alert-rules-metric-wrap">
                    <span className="alert-rules-metric-icon">{metricIcon(r.id)}</span>
                    <input className="input alert-rules-input-metric" value={r.label} onChange={(e) => updateRow(idx, { label: e.target.value })} placeholder="Nome da métrica" />
                  </div>
                </td>
                <td>
                  <select
                    className="input alert-rules-input-equip"
                    value={equipScopeFromCategories(r.apply_categories)}
                    onChange={(e) => updateRow(idx, { apply_categories: categoriesFromEquipScope(e.target.value as "*" | "olt" | "mikrotik" | "servidor") })}
                  >
                    {equipScopeOptions.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <select className="input alert-rules-input-unit" value={r.unit} onChange={(e) => updateRow(idx, { unit: e.target.value })}>
                    <option value="">-</option>
                    {unitOptions.map((u) => (
                      <option key={u} value={u}>
                        {u}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <div className="alert-rules-cond">
                    <select className="input alert-rules-input-scope" value={r.scope} onChange={(e) => updateRow(idx, { scope: e.target.value })}>
                      {scopeOptions.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                    <select className="input alert-rules-input-op" value={r.operator} onChange={(e) => updateRow(idx, { operator: e.target.value as "gte" | "lte" })}>
                      <option value="gte">≥</option>
                      <option value="lte">≤</option>
                    </select>
                  </div>
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.green_min} onChange={(e) => updateRow(idx, { green_min: e.target.value })} placeholder="0" />
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.warning_min} onChange={(e) => updateRow(idx, { warning_min: e.target.value })} placeholder="0" />
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.critical_min} onChange={(e) => updateRow(idx, { critical_min: e.target.value })} placeholder="0" />
                </td>
                <td style={{ textAlign: "center" }}>
                  <label className="toggle" htmlFor={`rule-enabled-${idx}`} style={{ justifyContent: "center" }}>
                    <span className="toggle__track">
                      <input
                        id={`rule-enabled-${idx}`}
                        type="checkbox"
                        role="switch"
                        className="toggle__input"
                        checked={r.enabled !== false}
                        onChange={(e) => updateRow(idx, { enabled: e.target.checked })}
                      />
                      <span className="toggle__thumb" aria-hidden />
                    </span>
                  </label>
                </td>
                <td>
                  <button type="button" className="btn btn--danger btn--icon" aria-label="Remover regra" title="Remover" onClick={() => removeRow(idx)}>
                    <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden>
                      <path
                        d="M9 3h6l1 2h4v2H4V5h4l1-2zm1 6h2v9h-2V9zm4 0h2v9h-2V9zM7 9h2v9H7V9z"
                        fill="currentColor"
                      />
                    </svg>
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="row" style={{ marginTop: 16, gap: 10, flexWrap: "wrap", alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={upsert.isPending} onClick={() => upsert.mutate()}>
          Salvar alterações
        </button>
        <span style={{ fontSize: 12, color: "var(--muted)" }}>{saveHint}</span>
      </div>
      {upsert.isError && <div className="msg msg--err">{(upsert.error as Error).message}</div>}
      {upsert.isSuccess && (
        <div className="msg msg--ok">
          Critérios guardados com sucesso. O monitoramento consulta estes valores para decidir se abre, atualiza ou resolve alertas.
        </div>
      )}
    </div>
  );
}

function ConnectionPanel() {
  type CategoryOverrides = {
    cpu_oid?: string;
    cpu_available_oid?: string;
    memory_used_oid?: string;
    memory_size_oid?: string;
    temp_oid?: string;
    uptime_oid?: string;
    interface_oids?: string[];
    optical_oids?: string[];
    pon_oids?: string[];
    onu_oids?: string[];
    bridge_oids?: string[];
    traffic_oids?: string[];
    /** OID normalizado (sem ponto inicial) → descrição mostrada no relatório. */
    oid_labels?: Record<string, string>;
  };
  type OverridesDoc = {
    olt?: CategoryOverrides;
    mikrotik?: CategoryOverrides;
    servidor?: CategoryOverrides;
    bridge?: CategoryOverrides;
  };
  type OidExtraCategory = "olt" | "mikrotik" | "servidor" | "bridge";
  type OidExtraKind = "interface" | "optical" | "pon" | "onu" | "bridge" | "traffic";
  type ExtraOidRow = { id: string; kind: OidExtraKind; oid: string; label: string };

  const OID_KIND_OPTIONS: { value: OidExtraKind; label: string }[] = [
    { value: "interface", label: "Interface" },
    { value: "traffic", label: "Tráfego (banda RX/TX etc.)" },
    { value: "optical", label: "Óptica / SFP" },
    { value: "pon", label: "PON" },
    { value: "onu", label: "ONU" },
    { value: "bridge", label: "Bridge" },
  ];

  const compact = (arr: Array<string | undefined | null>): string[] =>
    arr.map((s) => String(s ?? "").trim()).filter((s) => s.length > 0);

  const newOidRowId = (): string =>
    typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `oid-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;

  const oidLabelMapFromUnknown = (raw: unknown): Record<string, string> => {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
      const kk = String(k ?? "")
        .trim()
        .replace(/^\./, "");
      const vv = String(v ?? "").trim();
      if (kk && vv) out[kk] = vv;
    }
    return out;
  };

  const mergeCategoryOidLabels = (baselineBlk: CategoryOverrides | undefined, rows: ExtraOidRow[]): Record<string, string> | undefined => {
    const m: Record<string, string> = { ...oidLabelMapFromUnknown(baselineBlk?.oid_labels) };
    for (const r of rows) {
      const o = String(r.oid ?? "")
        .trim()
        .replace(/^\./, "");
      const lbl = String(r.label ?? "").trim();
      if (!o) continue;
      if (lbl) m[o] = lbl;
      else delete m[o];
    }
    return Object.keys(m).length ? m : undefined;
  };

  const oidsInCategoryArrays = (blk: CategoryOverrides): Set<string> => {
    const s = new Set<string>();
    const addArr = (arr?: string[]) => {
      for (const x of arr ?? []) {
        const o = String(x).trim().replace(/^\./, "");
        if (o) s.add(o);
      }
    };
    addArr(blk.interface_oids);
    addArr(blk.optical_oids);
    addArr(blk.pon_oids);
    addArr(blk.onu_oids);
    addArr(blk.bridge_oids);
    addArr(blk.traffic_oids);
    return s;
  };

  const pruneOidLabelsToBlock = (blk: CategoryOverrides, labels: Record<string, string> | undefined): Record<string, string> | undefined => {
    if (!labels) return undefined;
    const allowed = oidsInCategoryArrays(blk);
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(labels)) {
      const kk = String(k).trim().replace(/^\./, "");
      if (!kk || !String(v).trim()) continue;
      if (allowed.has(kk)) out[kk] = String(v).trim();
    }
    return Object.keys(out).length ? out : undefined;
  };

  const emptyExtraRows = (): Record<OidExtraCategory, ExtraOidRow[]> => ({
    olt: [],
    mikrotik: [],
    servidor: [],
    bridge: [],
  });

  /** Junta OIDs extra por tipo; mantém ordem e remove duplicados vazios. */
  const mergeOidsByKind = (rows: ExtraOidRow[]): Record<OidExtraKind, string[]> => {
    const acc: Record<OidExtraKind, string[]> = {
      interface: [],
      optical: [],
      pon: [],
      onu: [],
      bridge: [],
      traffic: [],
    };
    const seen: Record<OidExtraKind, Set<string>> = {
      interface: new Set(),
      optical: new Set(),
      pon: new Set(),
      onu: new Set(),
      bridge: new Set(),
      traffic: new Set(),
    };
    for (const r of rows) {
      const o = String(r.oid ?? "").trim();
      if (!o) continue;
      if (seen[r.kind].has(o)) continue;
      seen[r.kind].add(o);
      acc[r.kind].push(o);
    }
    return acc;
  };

  /**
   * Lê o JSON guardado e separa (a) campos reservados dos cartões OLT/Mikrotik
   * e (b) restantes em linhas editáveis por categoria.
   */
  const extraRowsFromOverridesDoc = (doc: OverridesDoc): Record<OidExtraCategory, ExtraOidRow[]> => {
    const out = emptyExtraRows();
    const fromArr = (labels: Record<string, string>, kind: OidExtraKind, list: string[] | undefined): ExtraOidRow[] =>
      (list ?? [])
        .map((oid) => {
          const o = String(oid).trim();
          if (!o) return null;
          const norm = o.replace(/^\./, "");
          const label = String(labels[norm] ?? labels[o] ?? "").trim();
          return { id: newOidRowId(), kind, oid: o, label };
        })
        .filter((r): r is ExtraOidRow => r != null);

    const o = doc.olt ?? {};
    const m = doc.mikrotik ?? {};
    const s = doc.servidor ?? {};
    const b = doc.bridge ?? {};
    const lo = oidLabelMapFromUnknown(o.oid_labels);
    const lm = oidLabelMapFromUnknown(m.oid_labels);
    const ls = oidLabelMapFromUnknown(s.oid_labels);
    const lb = oidLabelMapFromUnknown(b.oid_labels);

    out.olt.push(...fromArr(lo, "onu", (o.onu_oids ?? []).slice(1)));
    out.olt.push(...fromArr(lo, "pon", (o.pon_oids ?? []).slice(2)));
    out.olt.push(...fromArr(lo, "interface", o.interface_oids));
    out.olt.push(...fromArr(lo, "optical", o.optical_oids));
    out.olt.push(...fromArr(lo, "traffic", o.traffic_oids));
    out.olt.push(...fromArr(lo, "bridge", o.bridge_oids));

    out.mikrotik.push(...fromArr(lm, "interface", (m.interface_oids ?? []).slice(1)));
    out.mikrotik.push(...fromArr(lm, "traffic", (m.traffic_oids ?? []).slice(2)));
    out.mikrotik.push(...fromArr(lm, "optical", (m.optical_oids ?? []).slice(2)));
    out.mikrotik.push(...fromArr(lm, "pon", m.pon_oids));
    out.mikrotik.push(...fromArr(lm, "onu", m.onu_oids));
    out.mikrotik.push(...fromArr(lm, "bridge", m.bridge_oids));

    out.servidor.push(...fromArr(ls, "interface", s.interface_oids));
    out.servidor.push(...fromArr(ls, "optical", s.optical_oids));
    out.servidor.push(...fromArr(ls, "traffic", s.traffic_oids));
    out.servidor.push(...fromArr(ls, "pon", s.pon_oids));
    out.servidor.push(...fromArr(ls, "onu", s.onu_oids));
    out.servidor.push(...fromArr(ls, "bridge", s.bridge_oids));

    out.bridge.push(...fromArr(lb, "interface", b.interface_oids));
    out.bridge.push(...fromArr(lb, "optical", b.optical_oids));
    out.bridge.push(...fromArr(lb, "traffic", b.traffic_oids));
    out.bridge.push(...fromArr(lb, "pon", b.pon_oids));
    out.bridge.push(...fromArr(lb, "onu", b.onu_oids));
    out.bridge.push(...fromArr(lb, "bridge", b.bridge_oids));

    return out;
  };

  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-conn-def"],
    queryFn: () =>
      apiFetch<{
        snmp_community: unknown;
        snmp_community_value: string;
        snmp_community_configured: boolean;
        telnet_user: string | null;
        telnet_password: string;
        telnet_password_configured: boolean;
        telnet_enable: string;
        telnet_enable_configured: boolean;
        ssh_user: string | null;
        ssh_password: string;
        ssh_password_configured: boolean;
        oid_defaults: {
          olt: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          mikrotik: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          server: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
        };
        snmp_oid_overrides: unknown;
        updated_at: string;
      }>("/api/v1/settings/connection/defaults"),
  });
  const [snmp, setSnmp] = useState("");
  const [tu, setTu] = useState("");
  const [tp, setTp] = useState("");
  const [te, setTe] = useState("");
  const [su, setSu] = useState("");
  const [sp, setSp] = useState("");
  const [oltCpu, setOltCpu] = useState("");
  const [oltCpuAvail, setOltCpuAvail] = useState("");
  const [oltMemUsed, setOltMemUsed] = useState("");
  const [oltMemSize, setOltMemSize] = useState("");
  const [oltTemp, setOltTemp] = useState("");
  const [oltUptime, setOltUptime] = useState("");
  const [mkCpu, setMkCpu] = useState("");
  const [mkCpuAvail, setMkCpuAvail] = useState("");
  const [mkMemUsed, setMkMemUsed] = useState("");
  const [mkMemSize, setMkMemSize] = useState("");
  const [mkTemp, setMkTemp] = useState("");
  const [mkUptime, setMkUptime] = useState("");
  const [svCpu, setSvCpu] = useState("");
  const [svCpuAvail, setSvCpuAvail] = useState("");
  const [svMemUsed, setSvMemUsed] = useState("");
  const [svMemSize, setSvMemSize] = useState("");
  const [svTemp, setSvTemp] = useState("");
  const [svUptime, setSvUptime] = useState("");
  const [oltOnuTotalOid, setOltOnuTotalOid] = useState("");
  const [oltPonTxOid, setOltPonTxOid] = useState("");
  const [oltPonStatusOid, setOltPonStatusOid] = useState("");
  const [mkInterfacesStatusOid, setMkInterfacesStatusOid] = useState("");
  const [mkBandwidthRxOid, setMkBandwidthRxOid] = useState("");
  const [mkBandwidthTxOid, setMkBandwidthTxOid] = useState("");
  const [mkSfpTxOid, setMkSfpTxOid] = useState("");
  const [mkSfpRxOid, setMkSfpRxOid] = useState("");
  /** Base vinda do servidor (preserva scalars/hand-edits não cobertos pela UI). */
  const [overridesBaseline, setOverridesBaseline] = useState<OverridesDoc>({});
  const [extraOidRows, setExtraOidRows] = useState<Record<OidExtraCategory, ExtraOidRow[]>>(emptyExtraRows);
  const [showGeneratedJson, setShowGeneratedJson] = useState(false);

  useEffect(() => {
    if (!q.data) return;
    setSnmp((v) => (v === "" ? q.data.snmp_community_value ?? "" : v));
    setTu(q.data.telnet_user ?? "");
    setSu(q.data.ssh_user ?? "");
    setOltCpu(q.data.oid_defaults?.olt?.cpu_oid ?? "");
    setOltCpuAvail(q.data.oid_defaults?.olt?.cpu_available_oid ?? "");
    setOltMemUsed(q.data.oid_defaults?.olt?.memory_used_oid ?? "");
    setOltMemSize(q.data.oid_defaults?.olt?.memory_size_oid ?? "");
    setOltTemp(q.data.oid_defaults?.olt?.temp_oid ?? "");
    setOltUptime(q.data.oid_defaults?.olt?.uptime_oid ?? "");
    setMkCpu(q.data.oid_defaults?.mikrotik?.cpu_oid ?? "");
    setMkCpuAvail(q.data.oid_defaults?.mikrotik?.cpu_available_oid ?? "");
    setMkMemUsed(q.data.oid_defaults?.mikrotik?.memory_used_oid ?? "");
    setMkMemSize(q.data.oid_defaults?.mikrotik?.memory_size_oid ?? "");
    setMkTemp(q.data.oid_defaults?.mikrotik?.temp_oid ?? "");
    setMkUptime(q.data.oid_defaults?.mikrotik?.uptime_oid ?? "");
    setSvCpu(q.data.oid_defaults?.server?.cpu_oid ?? "");
    setSvCpuAvail(q.data.oid_defaults?.server?.cpu_available_oid ?? "");
    setSvMemUsed(q.data.oid_defaults?.server?.memory_used_oid ?? "");
    setSvMemSize(q.data.oid_defaults?.server?.memory_size_oid ?? "");
    setSvTemp(q.data.oid_defaults?.server?.temp_oid ?? "");
    setSvUptime(q.data.oid_defaults?.server?.uptime_oid ?? "");
    try {
      const parsed = (q.data.snmp_oid_overrides ?? {}) as OverridesDoc;
      setOverridesBaseline(JSON.parse(JSON.stringify(parsed)) as OverridesDoc);
      const olt = parsed?.olt ?? {};
      const mikrotik = parsed?.mikrotik ?? {};
      setOltOnuTotalOid(olt.onu_oids?.[0] ?? "");
      setOltPonTxOid(olt.pon_oids?.[0] ?? "");
      setOltPonStatusOid(olt.pon_oids?.[1] ?? "");
      setMkInterfacesStatusOid(mikrotik.interface_oids?.[0] ?? "");
      setMkBandwidthRxOid(mikrotik.traffic_oids?.[0] ?? "");
      setMkBandwidthTxOid(mikrotik.traffic_oids?.[1] ?? "");
      setMkSfpTxOid(mikrotik.optical_oids?.[0] ?? "");
      setMkSfpRxOid(mikrotik.optical_oids?.[1] ?? "");
      setExtraOidRows(extraRowsFromOverridesDoc(parsed));
    } catch {
      setOverridesBaseline({});
      setExtraOidRows(emptyExtraRows());
    }
  }, [q.data]);

  const builtOverridesPreview = (): OverridesDoc => {
    const base = JSON.parse(JSON.stringify(overridesBaseline)) as OverridesDoc;
    base.olt = base.olt ?? {};
    base.mikrotik = base.mikrotik ?? {};
    base.servidor = base.servidor ?? {};
    base.bridge = base.bridge ?? {};

    const mergedOlt = mergeOidsByKind(extraOidRows.olt);
    base.olt.onu_oids = compact([oltOnuTotalOid, ...mergedOlt.onu]);
    base.olt.pon_oids = compact([oltPonTxOid, oltPonStatusOid, ...mergedOlt.pon]);
    base.olt.interface_oids = mergedOlt.interface.length ? mergedOlt.interface : undefined;
    base.olt.optical_oids = mergedOlt.optical.length ? mergedOlt.optical : undefined;
    base.olt.traffic_oids = mergedOlt.traffic.length ? mergedOlt.traffic : undefined;
    base.olt.bridge_oids = mergedOlt.bridge.length ? mergedOlt.bridge : undefined;
    Object.keys(base.olt).forEach((k) => {
      const v = (base.olt as Record<string, unknown>)[k];
      if (v === undefined || (Array.isArray(v) && v.length === 0) || v === "") {
        delete (base.olt as Record<string, unknown>)[k];
      }
    });
    delete (base.olt as CategoryOverrides).oid_labels;
    const oltOidLabels = pruneOidLabelsToBlock(
      base.olt as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.olt, extraOidRows.olt),
    );
    if (oltOidLabels) (base.olt as CategoryOverrides).oid_labels = oltOidLabels;

    const mergedMk = mergeOidsByKind(extraOidRows.mikrotik);
    base.mikrotik.interface_oids = compact([mkInterfacesStatusOid, ...mergedMk.interface]);
    base.mikrotik.traffic_oids = compact([mkBandwidthRxOid, mkBandwidthTxOid, ...mergedMk.traffic]);
    base.mikrotik.optical_oids = compact([mkSfpTxOid, mkSfpRxOid, ...mergedMk.optical]);
    if (mergedMk.pon.length) base.mikrotik.pon_oids = mergedMk.pon;
    else delete base.mikrotik.pon_oids;
    if (mergedMk.onu.length) base.mikrotik.onu_oids = mergedMk.onu;
    else delete base.mikrotik.onu_oids;
    if (mergedMk.bridge.length) base.mikrotik.bridge_oids = mergedMk.bridge;
    else delete base.mikrotik.bridge_oids;
    Object.keys(base.mikrotik).forEach((k) => {
      const v = (base.mikrotik as Record<string, unknown>)[k];
      if (v === undefined || (Array.isArray(v) && v.length === 0) || v === "") {
        delete (base.mikrotik as Record<string, unknown>)[k];
      }
    });
    delete (base.mikrotik as CategoryOverrides).oid_labels;
    const mkOidLabels = pruneOidLabelsToBlock(
      base.mikrotik as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.mikrotik, extraOidRows.mikrotik),
    );
    if (mkOidLabels) (base.mikrotik as CategoryOverrides).oid_labels = mkOidLabels;

    const mergedSrv = mergeOidsByKind(extraOidRows.servidor);
    if (mergedSrv.interface.length) base.servidor.interface_oids = mergedSrv.interface;
    else delete base.servidor.interface_oids;
    if (mergedSrv.optical.length) base.servidor.optical_oids = mergedSrv.optical;
    else delete base.servidor.optical_oids;
    if (mergedSrv.traffic.length) base.servidor.traffic_oids = mergedSrv.traffic;
    else delete base.servidor.traffic_oids;
    if (mergedSrv.pon.length) base.servidor.pon_oids = mergedSrv.pon;
    else delete base.servidor.pon_oids;
    if (mergedSrv.onu.length) base.servidor.onu_oids = mergedSrv.onu;
    else delete base.servidor.onu_oids;
    if (mergedSrv.bridge.length) base.servidor.bridge_oids = mergedSrv.bridge;
    else delete base.servidor.bridge_oids;
    Object.keys(base.servidor).forEach((k) => {
      const v = (base.servidor as Record<string, unknown>)[k];
      if (v === undefined || (Array.isArray(v) && v.length === 0) || v === "") {
        delete (base.servidor as Record<string, unknown>)[k];
      }
    });
    delete (base.servidor as CategoryOverrides).oid_labels;
    const srvOidLabels = pruneOidLabelsToBlock(
      base.servidor as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.servidor, extraOidRows.servidor),
    );
    if (srvOidLabels) (base.servidor as CategoryOverrides).oid_labels = srvOidLabels;

    const mergedBr = mergeOidsByKind(extraOidRows.bridge);
    if (mergedBr.interface.length) base.bridge.interface_oids = mergedBr.interface;
    else delete base.bridge.interface_oids;
    if (mergedBr.optical.length) base.bridge.optical_oids = mergedBr.optical;
    else delete base.bridge.optical_oids;
    if (mergedBr.traffic.length) base.bridge.traffic_oids = mergedBr.traffic;
    else delete base.bridge.traffic_oids;
    if (mergedBr.pon.length) base.bridge.pon_oids = mergedBr.pon;
    else delete base.bridge.pon_oids;
    if (mergedBr.onu.length) base.bridge.onu_oids = mergedBr.onu;
    else delete base.bridge.onu_oids;
    if (mergedBr.bridge.length) base.bridge.bridge_oids = mergedBr.bridge;
    else delete base.bridge.bridge_oids;
    Object.keys(base.bridge).forEach((k) => {
      const v = (base.bridge as Record<string, unknown>)[k];
      if (v === undefined || (Array.isArray(v) && v.length === 0) || v === "") {
        delete (base.bridge as Record<string, unknown>)[k];
      }
    });
    delete (base.bridge as CategoryOverrides).oid_labels;
    const brOidLabels = pruneOidLabelsToBlock(
      base.bridge as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.bridge, extraOidRows.bridge),
    );
    if (brOidLabels) (base.bridge as CategoryOverrides).oid_labels = brOidLabels;

    (["olt", "mikrotik", "servidor", "bridge"] as const).forEach((ck) => {
      const blk = base[ck] as Record<string, unknown> | undefined;
      if (blk && Object.keys(blk).length === 0) {
        delete base[ck];
      }
    });
    return base;
  };

  const addExtraRow = (cat: OidExtraCategory, kind?: OidExtraKind) =>
    setExtraOidRows((prev) => ({
      ...prev,
      [cat]: [...prev[cat], { id: newOidRowId(), kind: kind ?? "interface", oid: "", label: "" }],
    }));

  const removeExtraRow = (cat: OidExtraCategory, id: string) =>
    setExtraOidRows((prev) => ({ ...prev, [cat]: prev[cat].filter((r) => r.id !== id) }));

  const updateExtraRow = (cat: OidExtraCategory, id: string, patchRow: Partial<Pick<ExtraOidRow, "kind" | "oid" | "label">>) =>
    setExtraOidRows((prev) => ({
      ...prev,
      [cat]: prev[cat].map((r) => (r.id === id ? { ...r, ...patchRow } : r)),
    }));

  const renderOidExtrasBlock = (cat: OidExtraCategory, title: string) => {
    const rows = extraOidRows[cat];
    return (
      <div className="card" style={{ marginTop: 8 }}>
        <h4 style={{ marginTop: 0 }}>{title}</h4>
        <p style={{ fontSize: 11, color: "var(--muted)", marginTop: -4 }}>
          Um identificador SNMP por linha, com descrição para o relatório (ex.: «CPU 02»). Escolha o tipo de métrica para o sistema organizar os dados ao guardar.
        </p>
        {rows.length === 0 ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>Nenhum extra — use «Adicionar» para incluir mais leituras.</p>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {rows.map((r) => (
              <div key={r.id} className="row" style={{ flexWrap: "wrap", gap: 6, alignItems: "flex-end" }}>
                <select
                  title="Tipo de métrica"
                  aria-label="Tipo de métrica SNMP"
                  className="select"
                  style={{ minWidth: 200, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.kind}
                  onChange={(e) => updateExtraRow(cat, r.id, { kind: e.target.value as OidExtraKind })}
                >
                  {OID_KIND_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
                <input
                  title="Identificador numérico SNMP"
                  aria-label="Identificador SNMP"
                  className="input mono"
                  style={{ flex: "1 1 160px", minWidth: 140, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.oid}
                  onChange={(e) => updateExtraRow(cat, r.id, { oid: e.target.value })}
                />
                <input
                  title="Descrição no relatório"
                  aria-label="Descrição da leitura SNMP extra"
                  className="input"
                  placeholder="Descrição (relatório)"
                  style={{ flex: "1 1 140px", minWidth: 120, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.label}
                  onChange={(e) => updateExtraRow(cat, r.id, { label: e.target.value })}
                />
                <button type="button" className="btn" style={{ padding: "4px 8px", fontSize: 11 }} onClick={() => removeExtraRow(cat, r.id)}>
                  −
                </button>
              </div>
            ))}
          </div>
        )}
        <div className="row" style={{ marginTop: 8, gap: 6 }}>
          <button type="button" className="btn btn--primary" style={{ padding: "4px 10px", fontSize: 11 }} onClick={() => addExtraRow(cat)}>
            Adicionar
          </button>
        </div>
      </div>
    );
  };

  const patch = useMutation({
    mutationFn: () => {
      const parsedOverrides = builtOverridesPreview();
      return apiFetch("/api/v1/settings/connection/defaults", {
        method: "PATCH",
        json: {
          snmp_community: snmp || undefined,
          telnet_user: tu || undefined,
          telnet_password: tp || undefined,
          telnet_enable: te || undefined,
          ssh_user: su || undefined,
          ssh_password: sp || undefined,
          olt_cpu_oid: oltCpu || undefined,
          olt_cpu_available_oid: oltCpuAvail || undefined,
          olt_memory_used_oid: oltMemUsed || undefined,
          olt_memory_size_oid: oltMemSize || undefined,
          olt_temp_oid: oltTemp || undefined,
          olt_uptime_oid: oltUptime || undefined,
          mikrotik_cpu_oid: mkCpu || undefined,
          mikrotik_cpu_available_oid: mkCpuAvail || undefined,
          mikrotik_memory_used_oid: mkMemUsed || undefined,
          mikrotik_memory_size_oid: mkMemSize || undefined,
          mikrotik_temp_oid: mkTemp || undefined,
          mikrotik_uptime_oid: mkUptime || undefined,
          server_cpu_oid: svCpu || undefined,
          server_cpu_available_oid: svCpuAvail || undefined,
          server_memory_used_oid: svMemUsed || undefined,
          server_memory_size_oid: svMemSize || undefined,
          server_temp_oid: svTemp || undefined,
          server_uptime_oid: svUptime || undefined,
          snmp_oid_overrides: parsedOverrides,
        },
      });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings-conn-def"] }),
  });

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Credenciais e leituras SNMP por defeito</h2>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>
        Valores aplicados quando o equipamento não traz credenciais próprias. Palavras-passe não são mostradas ao abrir esta página.
        {q.data?.updated_at ? ` Última alteração: ${q.data.updated_at}` : ""}
      </p>
      <div className="row" style={{ gap: 10, marginBottom: 8 }}>
        <span className={q.data?.snmp_community_configured ? "badge badge--ok" : "badge badge--off"}>
          Comunidade SNMP: {q.data?.snmp_community_configured ? "definida" : "não definida"}
        </span>
        <span className={q.data?.telnet_password_configured ? "badge badge--ok" : "badge badge--off"}>
          Palavra-passe Telnet: {q.data?.telnet_password_configured ? "definida" : "não definida"}
        </span>
        <span className={q.data?.ssh_password_configured ? "badge badge--ok" : "badge badge--off"}>
          Palavra-passe SSH: {q.data?.ssh_password_configured ? "definida" : "não definida"}
        </span>
      </div>
      <div className="field">
        <label>Comunidade SNMP padrão</label>
        <input className="input" value={snmp} onChange={(e) => setSnmp(e.target.value)} />
      </div>
      <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
        <div className="field" style={{ minWidth: 220 }}><label>Utilizador Telnet</label><input className="input" value={tu} onChange={(e) => setTu(e.target.value)} /></div>
        <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe Telnet</label><input className="input" type="password" value={tp} onChange={(e) => setTp(e.target.value)} /></div>
        <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe enable (Telnet)</label><input className="input" type="password" value={te} onChange={(e) => setTe(e.target.value)} /></div>
      </div>
      <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: 8 }}>
        <div className="field" style={{ minWidth: 220 }}><label>Utilizador SSH</label><input className="input" value={su} onChange={(e) => setSu(e.target.value)} /></div>
        <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe SSH</label><input className="input" type="password" value={sp} onChange={(e) => setSp(e.target.value)} /></div>
      </div>
      <h3 style={{ marginTop: 14, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
        Leituras SNMP preferidas por tipo de equipamento
        <InfoHint label="OIDs SNMP preferidos">
          <p>
            Se preencher, estes endereços têm prioridade sobre a descoberta automática. Em «CPU utilizada» indique a carga; em «CPU disponível» use normalmente
            a percentagem em idle (ociosidade). O painel tenta primeiro a utilizada e só depois deriva a partir da disponível (100 − idle).
          </p>
        </InfoHint>
      </h3>
      <div className="field"><label>OLT — CPU, memória, temperatura, tempo ligado</label></div>
      <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
        <div className="field">
          <label>CPU utilizada (uso / carga)</label>
          <input className="input mono" value={oltCpu} onChange={(e) => setOltCpu(e.target.value)} />
        </div>
        <div className="field">
          <label>CPU disponível (% idle)</label>
          <input className="input mono" value={oltCpuAvail} onChange={(e) => setOltCpuAvail(e.target.value)} placeholder="opcional" />
        </div>
        <div className="field"><label>Memória em uso</label><input className="input mono" value={oltMemUsed} onChange={(e) => setOltMemUsed(e.target.value)} /></div>
        <div className="field"><label>Memória total</label><input className="input mono" value={oltMemSize} onChange={(e) => setOltMemSize(e.target.value)} /></div>
        <div className="field"><label>Temperatura</label><input className="input mono" value={oltTemp} onChange={(e) => setOltTemp(e.target.value)} /></div>
        <div className="field"><label>Tempo ligado (uptime)</label><input className="input mono" value={oltUptime} onChange={(e) => setOltUptime(e.target.value)} /></div>
      </div>
      <div className="field"><label>MikroTik — CPU, memória, temperatura, tempo ligado</label></div>
      <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
        <div className="field">
          <label>CPU utilizada (uso / carga)</label>
          <input className="input mono" value={mkCpu} onChange={(e) => setMkCpu(e.target.value)} />
        </div>
        <div className="field">
          <label>CPU disponível (% idle)</label>
          <input className="input mono" value={mkCpuAvail} onChange={(e) => setMkCpuAvail(e.target.value)} placeholder="opcional" />
        </div>
        <div className="field"><label>Memória em uso</label><input className="input mono" value={mkMemUsed} onChange={(e) => setMkMemUsed(e.target.value)} /></div>
        <div className="field"><label>Memória total</label><input className="input mono" value={mkMemSize} onChange={(e) => setMkMemSize(e.target.value)} /></div>
        <div className="field"><label>Temperatura</label><input className="input mono" value={mkTemp} onChange={(e) => setMkTemp(e.target.value)} /></div>
        <div className="field"><label>Tempo ligado (uptime)</label><input className="input mono" value={mkUptime} onChange={(e) => setMkUptime(e.target.value)} /></div>
      </div>
      <div className="field"><label>Servidor — CPU, memória, temperatura, tempo ligado</label></div>
      <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
        <div className="field">
          <label>CPU utilizada (uso / carga)</label>
          <input className="input mono" value={svCpu} onChange={(e) => setSvCpu(e.target.value)} />
        </div>
        <div className="field">
          <label>CPU disponível (% idle)</label>
          <input className="input mono" value={svCpuAvail} onChange={(e) => setSvCpuAvail(e.target.value)} placeholder="opcional" />
        </div>
        <div className="field"><label>Memória em uso</label><input className="input mono" value={svMemUsed} onChange={(e) => setSvMemUsed(e.target.value)} /></div>
        <div className="field"><label>Memória total</label><input className="input mono" value={svMemSize} onChange={(e) => setSvMemSize(e.target.value)} /></div>
        <div className="field"><label>Temperatura</label><input className="input mono" value={svTemp} onChange={(e) => setSvTemp(e.target.value)} /></div>
        <div className="field"><label>Tempo ligado (uptime)</label><input className="input mono" value={svUptime} onChange={(e) => setSvUptime(e.target.value)} /></div>
      </div>
      <h3 style={{ marginTop: 14 }}>Telemetria OLT e MikroTik (PON, interfaces, SFP)</h3>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>Campos rápidos para métricas frequentes; o restante pode ir na secção seguinte.</p>
      <div className="card" style={{ marginTop: 8 }}>
        <h4 style={{ marginTop: 0 }}>OLT (PON / GBIC / ONU)</h4>
        <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Total de ONUs (identificador SNMP)</label>
            <input className="input mono" value={oltOnuTotalOid} onChange={(e) => setOltOnuTotalOid(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Potência TX da PON</label>
            <input className="input mono" value={oltPonTxOid} onChange={(e) => setOltPonTxOid(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Estado da PON</label>
            <input className="input mono" value={oltPonStatusOid} onChange={(e) => setOltPonStatusOid(e.target.value)} />
          </div>
        </div>
      </div>
      <div className="card" style={{ marginTop: 8 }}>
        <h4 style={{ marginTop: 0 }}>MikroTik (Interfaces / SFP)</h4>
        <p style={{ fontSize: 11, color: "var(--muted)", marginTop: -4 }}>
          A página de interfaces faz walk em <span className="mono">mtxrOpticalTable</span> (<span className="mono">1.3.6.1.4.1.14988.1.1.19</span>, MIB MIKROTIK) e em <span className="mono">mtxrInterfaceStatsName</span> (
          <span className="mono">1.3.6.1.4.1.14988.1.1.14.1.1.2</span>) para obter o nome igual ao <span className="mono">ifName</span>. Potências: colunas <strong>9</strong> (TX) e <strong>10</strong> (RX), tipo <strong>IDiv1000</strong> (milésimos de dBm). O índice da linha mtxr não é o ifIndex; o cruzamento usa o nome de <span className="mono">…14.1.1.2</span>, o valor de <span className="mono">mtxrOpticalIndex</span> (col.1) quando coincidir com um ifIndex, e heurísticas sobre <span className="mono">mtxrOpticalName</span> (col.2). Os campos abaixo são OIDs <strong>opcionais</strong> para telemetria SNMP GET.
        </p>
        <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Estado das interfaces (ligado / desligado)</label>
            <input className="input mono" value={mkInterfacesStatusOid} onChange={(e) => setMkInterfacesStatusOid(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Banda recebida (RX) por interface</label>
            <input className="input mono" value={mkBandwidthRxOid} onChange={(e) => setMkBandwidthRxOid(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Banda enviada (TX) por interface</label>
            <input className="input mono" value={mkBandwidthTxOid} onChange={(e) => setMkBandwidthTxOid(e.target.value)} />
          </div>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Potência SFP (TX) — telemetria GET opcional</label>
            <input
              className="input mono"
              value={mkSfpTxOid}
              onChange={(e) => setMkSfpTxOid(e.target.value)}
              placeholder="1.3.6.1.4.1.14988.1.1.19.1.1.9"
            />
          </div>
          <div className="field" style={{ minWidth: 260 }}>
            <label>Potência SFP (RX) — telemetria GET opcional</label>
            <input
              className="input mono"
              value={mkSfpRxOid}
              onChange={(e) => setMkSfpRxOid(e.target.value)}
              placeholder="1.3.6.1.4.1.14988.1.1.19.1.1.10"
            />
          </div>
        </div>
      </div>
      <h3 style={{ marginTop: 14 }}>Outras leituras SNMP por categoria</h3>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>
        Use quando precisar de mais objetos além dos cartões acima. Ao guardar, tudo é enviado para o servidor de forma estruturada (sem editar JSON à mão).
      </p>
      {renderOidExtrasBlock("olt", "OLT — leituras extra")}
      {renderOidExtrasBlock("mikrotik", "MikroTik — leituras extra")}
      {renderOidExtrasBlock("servidor", "Servidor — leituras extra")}
      {renderOidExtrasBlock("bridge", "Pontes — leituras extra")}
      <div className="card" style={{ marginTop: 10 }}>
        <h4 style={{ marginTop: 0 }}>Extras atualmente configurados</h4>
        {(["olt", "mikrotik", "servidor", "bridge"] as const).map((cat) => {
          const block = builtOverridesPreview()[cat];
          const list = [
            ...(block?.interface_oids ?? []),
            ...(block?.traffic_oids ?? []),
            ...(block?.optical_oids ?? []),
            ...(block?.pon_oids ?? []),
            ...(block?.onu_oids ?? []),
            ...(block?.bridge_oids ?? []),
          ].filter((v) => String(v).trim() !== "");
          return (
            <div key={cat} style={{ marginBottom: 8 }}>
              <strong style={{ textTransform: "capitalize" }}>{cat}</strong>
              {list.length === 0 ? (
                <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--muted)" }}>Sem extras configurados.</p>
              ) : (
                <div className="row" style={{ gap: 6, flexWrap: "wrap", marginTop: 4 }}>
                  {list.map((oid) => (
                    <span key={`${cat}-${oid}`} className="badge badge--off mono" title={oid}>
                      {oid}
                    </span>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>
      <div className="field" style={{ marginTop: 12 }}>
        <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer" }}>
          <input type="checkbox" checked={showGeneratedJson} onChange={(e) => setShowGeneratedJson(e.target.checked)} />
          Mostrar pré-visualização técnica (JSON)
        </label>
        {showGeneratedJson && (
          <pre className="mono" style={{ fontSize: 10, marginTop: 8, padding: 8, overflow: "auto", maxHeight: 240, background: "var(--panel2, #161b22)", borderRadius: 6 }}>
            {JSON.stringify(builtOverridesPreview(), null, 2)}
          </pre>
        )}
      </div>
      <button type="button" className="btn btn--primary" style={{ marginTop: 12 }} disabled={patch.isPending} onClick={() => patch.mutate()}>
        Guardar credenciais e SNMP
      </button>
      {patch.isError && <div className="msg msg--err">{(patch.error as Error).message}</div>}
      {patch.isSuccess && <div className="msg msg--ok">Alterações guardadas.</div>}
    </div>
  );
}

function TelegramTestOutcome({ data, error }: { data: unknown; error: Error | null }) {
  if (error) {
    return (
      <div className="msg msg--err" style={{ marginTop: 10 }}>
        {error.message}
      </div>
    );
  }
  if (data === undefined) return null;
  if (data !== null && typeof data === "object") {
    const d = data as Record<string, unknown>;
    if (d.ok === true && d.sent === true) {
      return (
        <div className="msg msg--ok" style={{ marginTop: 10 }}>
          Mensagem de teste enviada com sucesso. Verifique o Telegram.
        </div>
      );
    }
    if (d.ok === false && typeof d.message === "string" && d.message.trim()) {
      return (
        <div className="msg msg--err" style={{ marginTop: 10 }}>
          {d.message}
        </div>
      );
    }
    if (d.ok === true) {
      return (
        <div className="msg msg--ok" style={{ marginTop: 10 }}>
          Pedido de teste concluído.
        </div>
      );
    }
  }
  return (
    <details style={{ marginTop: 10, fontSize: 12 }}>
      <summary style={{ cursor: "pointer", color: "var(--muted)" }}>Detalhe da resposta</summary>
      <pre className="mono" style={{ marginTop: 6, padding: 8, background: "var(--panel2)", borderRadius: 6, fontSize: 11, overflow: "auto" }}>
        {JSON.stringify(data, null, 2)}
      </pre>
    </details>
  );
}

function TelegramPanel({ id, title }: { id: string; title: string }) {
  const qc = useQueryClient();
  const path = id === "monitoring" ? "monitoring" : "reports";
  const q = useQuery({
    queryKey: ["settings-tg", id],
    queryFn: () =>
      apiFetch<{ id: string; bot_token: unknown; chat_id: string | null; topic_id: string | null }>(
        `/api/v1/settings/notifications/telegram/${path}`,
      ),
  });
  const [token, setToken] = useState("");
  const [chat, setChat] = useState("");
  const [topic, setTopic] = useState("");
  const [saveToast, setSaveToast] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (!q.data) return;
    setChat(q.data.chat_id ?? "");
    setTopic(q.data.topic_id ?? "");
  }, [q.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/settings/notifications/telegram/${path}`, {
        method: "PATCH",
        json: { bot_token: token || undefined, chat_id: chat || undefined, topic_id: topic || undefined },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-tg", id] });
      setSaveToast({ ok: true, text: "Guardado com sucesso (Telegram)." });
    },
    onError: (err) => setSaveToast({ ok: false, text: (err as Error).message || "Falha ao guardar (Telegram)." }),
  });

  const test = useMutation({
    mutationFn: () => apiFetch(`/api/v1/settings/notifications/telegram/${path}/test`, { method: "POST", json: {} }),
  });

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Telegram — {title}</h2>
      <p style={{ fontSize: 12, color: "var(--muted)" }}>
        Para alterar o bot, introduza um novo token abaixo. O valor já guardado não é mostrado por segurança.
      </p>
      <div className="field">
        <label>Token do bot (novo)</label>
        <input className="input mono" type="password" value={token} onChange={(e) => setToken(e.target.value)} />
      </div>
      <div className="row" style={{ gap: 8 }}>
        <input className="input" placeholder="ID do chat" value={chat} onChange={(e) => setChat(e.target.value)} />
        <input className="input" placeholder="ID do tópico (opcional)" value={topic} onChange={(e) => setTopic(e.target.value)} />
      </div>
      <div className="row" style={{ marginTop: 12, gap: 8 }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          Guardar
        </button>
        <button type="button" className="btn" disabled={test.isPending} onClick={() => test.mutate()}>
          Enviar mensagem de teste
        </button>
      </div>
      {saveToast && (
        <div className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 10 }}>
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
            ×
          </button>
          {saveToast.text}
        </div>
      )}
      <TelegramTestOutcome data={test.data} error={test.error as Error | null} />
    </div>
  );
}

function OltVendorsPanel() {
  const qc = useQueryClient();
  const brands = useQuery({ queryKey: ["olt-brands"], queryFn: () => apiFetch<{ brands: string[] }>("/api/v1/settings/olt-vendors") });
  const [brand, setBrand] = useState("");
  const [saveToast, setSaveToast] = useState<{ ok: boolean; text: string } | null>(null);
  const vendor = useQuery({
    queryKey: ["olt-vendor", brand],
    enabled: !!brand,
    queryFn: () =>
      apiFetch<{ brand: string; onu_online_oid: string | null; pon_status_oid: string | null; transceiver_oid: string | null; snmp_base_oid: string | null }>(
        `/api/v1/settings/olt-vendors/${encodeURIComponent(brand)}`,
      ),
  });
  const [onu, setOnu] = useState("");
  const [pon, setPon] = useState("");
  const [trx, setTrx] = useState("");
  const [base, setBase] = useState("");

  useEffect(() => {
    if (!vendor.data) return;
    setOnu(vendor.data.onu_online_oid ?? "");
    setPon(vendor.data.pon_status_oid ?? "");
    setTrx(vendor.data.transceiver_oid ?? "");
    setBase(vendor.data.snmp_base_oid ?? "");
  }, [vendor.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/settings/olt-vendors/${encodeURIComponent(brand)}`, {
        method: "PATCH",
        json: {
          onu_online_oid: onu || null,
          pon_status_oid: pon || null,
          transceiver_oid: trx || null,
          snmp_base_oid: base || null,
        },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["olt-vendor", brand] });
      setSaveToast({ ok: true, text: `Guardado com sucesso (perfil OLT: ${brand}).` });
    },
    onError: (err) => {
      setSaveToast({ ok: false, text: (err as Error)?.message || "Falha ao guardar (perfil OLT)." });
    },
  });

  if (brands.isLoading) return <p>A carregar…</p>;
  if (brands.isError) return <div className="msg msg--err">{(brands.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Perfis por fabricante</h2>
      <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
        <select className="input" value={brand} onChange={(e) => setBrand(e.target.value)} style={{ minWidth: 200 }}>
          <option value="">— escolher —</option>
          {(brands.data?.brands ?? []).map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </select>
      </div>
      {brand && vendor.isLoading && <p>A carregar perfil…</p>}
      {brand && vendor.isError && <div className="msg msg--err">{(vendor.error as Error).message}</div>}
      {brand && vendor.data && (
        <>
          <div className="field" style={{ marginTop: 12 }}>
            <label>ONUs em linha</label>
            <input className="input mono" value={onu} onChange={(e) => setOnu(e.target.value)} title="Identificador SNMP" />
          </div>
          <div className="field">
            <label>Estado das portas PON</label>
            <input className="input mono" value={pon} onChange={(e) => setPon(e.target.value)} title="Identificador SNMP" />
          </div>
          <div className="field">
            <label>Dados do transceiver (GBIC/SFP)</label>
            <input className="input mono" value={trx} onChange={(e) => setTrx(e.target.value)} title="Identificador SNMP" />
          </div>
          <div className="field">
            <label>Prefixo SNMP do fabricante</label>
            <input className="input mono" value={base} onChange={(e) => setBase(e.target.value)} title="Árvore SNMP base para este fabricante" />
          </div>
          <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
            Guardar perfil
          </button>
          {saveToast && (
            <div className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 10 }}>
              <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
                ×
              </button>
              {saveToast.text}
            </div>
          )}
        </>
      )}
    </div>
  );
}


