import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type CSSProperties, useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { ApiError, apiFetch } from "../../lib/api";
import { PAGE_TOAST_AUTO_MS } from "../../lib/pageToast";
import { DatabaseCleanupButton } from "./DatabaseCleanupModal";
import { SystemConfigBackupPanel } from "./SystemConfigBackupPanel";

type DbMeta = {
  host: string | null;
  port: number | null;
  db_user_masked: unknown;
  db_name: string | null;
  ssl_mode: string | null;
  provider?: string | null;
  password_configured: boolean;
  active_dsn_source: string;
  note?: string;
};

type DbTestResponse = { ok?: boolean; message?: string };

type B2File = {
  file_id: string;
  file_name: string;
  content_length: number;
  upload_timestamp: number;
  base_name: string;
};

type RestoreJob = {
  job_id: string;
  status: string;
  progress_pct: number;
  current_step: string;
  error?: string;
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

function friendlyDbConnectionError(err: unknown): string {
  if (!(err instanceof ApiError)) {
    return "Não foi possível concluir a requisição. Verifique a ligação à internet e tente novamente.";
  }
  const raw = (err.message || "").toLowerCase();
  const code = (err.code || "").toUpperCase();
  if (code === "VALIDATION" || raw.includes("informe host") || raw.includes("db_password")) {
    return "Falta informação para testar: servidor, porta, nome da base, usuário e palavra-passe.";
  }
  if (code === "NO_DB") return "O serviço de base de dados não está disponível neste momento.";
  if (raw.includes("authentication failed") || raw.includes("password authentication")) {
    return "O servidor recusou o usuário ou a palavra-passe.";
  }
  if (raw.includes("connection refused")) {
    return "Ligação recusada na porta indicada. Verifique se o PostgreSQL está a correr.";
  }
  if (raw.includes("no such host") || raw.includes("name or service not known")) {
    return "Não encontrámos esse endereço de servidor.";
  }
  if (raw.includes("timeout") || raw.includes("deadline exceeded")) {
    return "A ligação demorou demasiado. Verifique rede e firewall.";
  }
  if (raw.includes("does not exist") && raw.includes("database")) {
    return "Essa base de dados não existe neste servidor.";
  }
  if (raw.includes("ssl") || raw.includes("tls") || raw.includes("certificate")) {
    return "Problema SSL/TLS. Experimente require ou disable conforme o ambiente.";
  }
  return "Não foi possível ligar à base de dados. Revise os dados e tente novamente.";
}

function friendlyDbPatchError(err: unknown): string {
  if (!(err instanceof ApiError)) return "Não foi possível salvar. Tente novamente.";
  const raw = (err.message || "").toLowerCase();
  if (raw.includes("database_url") && raw.includes("apply_connection")) {
    return "Para usar uma URL completa tem de marcar “Aplicar já esta ligação”.";
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
  if (!opts.host.trim()) missing.push("servidor");
  const p = opts.port.trim();
  if (!p || Number.isNaN(Number(p)) || Number(p) <= 0) missing.push("porta");
  if (!opts.dbName.trim()) missing.push("nome da base");
  if (!opts.dbUser.trim() && !opts.userKnownInSettings) missing.push("usuário");
  if (!opts.dbPass.trim() && !opts.passwordConfigured) missing.push("palavra-passe");
  return missing;
}

const sectionStyle: CSSProperties = {
  marginTop: 24,
  paddingTop: 16,
  borderTop: "1px solid var(--border)",
};

function RestoreSection() {
  const [open, setOpen] = useState(false);
  const [files, setFiles] = useState<B2File[]>([]);
  const [listErr, setListErr] = useState<string | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const [job, setJob] = useState<RestoreJob | null>(null);
  const [confirmText, setConfirmText] = useState("");
  const [toast, setToast] = useState<string | null>(null);

  const list = useMutation({
    mutationFn: () => apiFetch<{ files: B2File[] }>("/api/v1/settings/database/backups/b2"),
    onSuccess: (d) => {
      setFiles(d.files ?? []);
      setListErr(null);
    },
    onError: (e) => setListErr((e as Error).message),
  });

  const restoreB2 = useMutation({
    mutationFn: (fileName: string) =>
      apiFetch<{ job_id: string }>("/api/v1/settings/database/backups/restore", {
        method: "POST",
        json: { source: "b2", file_name: fileName, confirm: "RESTORE" },
        timeoutMs: 120_000,
      }),
    onSuccess: (d) => {
      setJobId(d.job_id);
      setToast("Restore iniciado a partir do B2…");
    },
    onError: (e) => setToast((e as Error).message),
  });

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const fd = new FormData();
      fd.append("file", file);
      fd.append("confirm", "RESTORE");
      return apiFetch<{ job_id: string }>("/api/v1/settings/database/backups/upload", {
        method: "POST",
        body: fd,
        timeoutMs: 600_000,
      });
    },
    onSuccess: (d) => {
      setJobId(d.job_id);
      setToast("Upload recebido — restore em curso…");
    },
    onError: (e) => setToast((e as Error).message),
  });

  useEffect(() => {
    if (!jobId) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const j = await apiFetch<RestoreJob>(`/api/v1/settings/database/backups/restore/${jobId}`);
        if (cancelled) return;
        setJob(j);
        if (j.status === "running") {
          window.setTimeout(tick, 2000);
        } else if (j.status === "ok") {
          setToast("Restore concluído. Recarregue a página se necessário.");
        } else if (j.status === "error") {
          setToast(j.error || "Restore falhou");
        }
      } catch (e) {
        if (!cancelled) setToast((e as Error).message);
      }
    };
    void tick();
    return () => {
      cancelled = true;
    };
  }, [jobId]);

  const canConfirm = confirmText.trim() === "RESTORE";

  return (
    <div style={sectionStyle}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <h3 style={{ fontSize: 14, margin: 0, display: "flex", alignItems: "center", gap: 6 }}>
          Recuperação de dump
          <InfoHint label="Restore de dump">
            <p>
              Apaga o schema <span className="mono">public</span> e restaura um dump full (.pgdump). Use só em emergência.
              Confirme escrevendo <strong>RESTORE</strong>.
            </p>
          </InfoHint>
        </h3>
        <button type="button" className="btn" onClick={() => setOpen((v) => !v)}>
          {open ? "Ocultar" : "Mostrar"}
        </button>
      </div>
      {!open ? (
        <p style={{ color: "var(--muted)", fontSize: 12, margin: "8px 0 0" }}>
          Restaurar a partir de ficheiro local ou backups no Backblaze B2.
        </p>
      ) : (
        <>
          <div className="field" style={{ maxWidth: 280, marginTop: 12 }}>
            <label htmlFor="restore-confirm">Confirmação</label>
            <input
              id="restore-confirm"
              className="input mono"
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder="RESTORE"
              autoComplete="off"
            />
          </div>
          <div className="row" style={{ gap: 8, flexWrap: "wrap", marginTop: 10 }}>
            <label
              className={`btn ${!canConfirm || upload.isPending ? "btn--disabled" : ""}`}
              style={{ cursor: canConfirm ? "pointer" : "not-allowed" }}
            >
              Carregar .pgdump
              <input
                type="file"
                accept=".pgdump,.dump,application/octet-stream"
                style={{ display: "none" }}
                disabled={!canConfirm || upload.isPending}
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) upload.mutate(f);
                  e.target.value = "";
                }}
              />
            </label>
            <button type="button" className="btn" disabled={list.isPending} onClick={() => list.mutate()}>
              Listar B2
            </button>
          </div>
          {listErr && (
            <div className="msg msg--err" style={{ marginTop: 8 }}>
              {listErr}
            </div>
          )}
          {files.length > 0 && (
            <ul style={{ marginTop: 12, paddingLeft: 18, fontSize: 13 }}>
              {files
                .slice()
                .sort((a, b) => b.upload_timestamp - a.upload_timestamp)
                .map((f) => (
                  <li key={f.file_id} style={{ marginBottom: 8 }}>
                    <span className="mono">{f.base_name}</span>{" "}
                    <span style={{ color: "var(--muted)" }}>({(f.content_length / (1024 * 1024)).toFixed(1)} MB)</span>{" "}
                    <button
                      type="button"
                      className="btn"
                      style={{ fontSize: 12, padding: "2px 8px" }}
                      disabled={!canConfirm || restoreB2.isPending}
                      onClick={() => {
                        if (!window.confirm(`Restaurar ${f.base_name}? Isto apaga os dados actuais.`)) return;
                        restoreB2.mutate(f.file_name);
                      }}
                    >
                      Restaurar
                    </button>
                  </li>
                ))}
            </ul>
          )}
          {job && (
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 10 }}>
              {job.current_step} ({job.progress_pct}%) — {job.status}
            </p>
          )}
          {toast && (
            <div className="msg msg--ok" style={{ marginTop: 8 }}>
              {toast}
            </div>
          )}
        </>
      )}
    </div>
  );
}

export function DatabasePanel() {
  const qc = useQueryClient();
  const meta = useQuery({ queryKey: ["settings-db-meta"], queryFn: () => apiFetch<DbMeta>("/api/v1/settings/database") });
  const [provider, setProvider] = useState<"local" | "external">("local");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("5432");
  const [dbUser, setDbUser] = useState("");
  const [dbName, setDbName] = useState("");
  const [sslMode, setSslMode] = useState("disable");
  const [dbPass, setDbPass] = useState("");
  const [dbUrl, setDbUrl] = useState("");
  const [apply, setApply] = useState(false);
  const [dbToast, setDbToast] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (!meta.data) return;
    const p = (meta.data.provider ?? "local").toLowerCase() === "external" ? "external" : "local";
    setProvider(p);
    setHost(meta.data.host ?? (p === "local" ? "127.0.0.1" : ""));
    setPort(meta.data.port != null ? String(meta.data.port) : "5432");
    setDbName(meta.data.db_name ?? "");
    const sm = (meta.data.ssl_mode ?? "").trim().toLowerCase();
    setSslMode(sm === "require" ? "require" : "disable");
  }, [meta.data]);

  useEffect(() => {
    if (!dbToast) return;
    const t = window.setTimeout(() => setDbToast(null), PAGE_TOAST_AUTO_MS);
    return () => window.clearTimeout(t);
  }, [dbToast]);

  const onProviderChange = (next: "local" | "external") => {
    setProvider(next);
    if (next === "local") {
      if (!host.trim() || host.includes("supabase")) setHost("127.0.0.1");
      if (!port.trim()) setPort("5432");
      setSslMode("disable");
    }
  };

  const patch = useMutation({
    mutationFn: (body: Record<string, unknown>) => apiFetch("/api/v1/settings/database", { method: "PATCH", json: body }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-db-meta"] });
      setDbToast({ ok: true, text: "Guardado com sucesso." });
    },
    onError: (e) => setDbToast({ ok: false, text: friendlyDbPatchError(e) }),
  });

  const testConn = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      apiFetch<DbTestResponse>("/api/v1/settings/database/test", { method: "POST", json: body }),
    onSuccess: (data) => {
      const msg = typeof data?.message === "string" ? data.message : "";
      setDbToast({ ok: true, text: friendlyDbTestSuccessMessage(msg) });
    },
    onError: (e) => setDbToast({ ok: false, text: friendlyDbConnectionError(e) }),
  });

  if (meta.isLoading) return <p>A carregar metadados…</p>;
  if (meta.isError) return <div className="msg msg--err">{(meta.error as Error).message}</div>;

  const buildPatchBody = (): Record<string, unknown> => {
    const body: Record<string, unknown> = { provider };
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
    const b = buildPatchBody();
    delete b.apply_connection;
    if (dbUrl.trim()) {
      testConn.mutate({ database_url: dbUrl.trim() });
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
      setDbToast({ ok: false, text: `Falta preencher: ${missing.join(", ")}.` });
      return;
    }
    testConn.mutate(b);
  };

  const sslChoice = sslMode.trim().toLowerCase() === "disable" ? "disable" : "require";
  const sourceLabel =
    meta.data?.active_dsn_source === "env_NETQUASAR_DATABASE_URL" ? "variável de ambiente" : "definições salvas";

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="card">
        <h2 style={{ marginTop: 0, marginBottom: 6, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Ligação PostgreSQL
          <InfoHint label="Ligação à base de dados">
            <p>
              <strong>Local</strong> — Docker (<span className="mono">postgres</span>) ou Postgres no PC (
              <span className="mono">127.0.0.1</span>). <strong>Externo</strong> — host/IP remoto.
            </p>
            <p>
              “Testar” não altera o sistema. “Aplicar já” troca a ligação activa e corre migrações.
            </p>
          </InfoHint>
        </h2>
        <p style={{ color: "var(--muted)", fontSize: 12, margin: "0 0 14px" }}>
          Em uso: <strong>{sourceLabel}</strong>
          {" · "}
          Palavra-passe: <strong>{meta.data?.password_configured ? "salva" : "não salva"}</strong>
          {meta.data?.host ? (
            <>
              {" · "}
              <span className="mono">
                {meta.data.host}
                {meta.data.port != null ? `:${meta.data.port}` : ""}
                {meta.data.db_name ? `/${meta.data.db_name}` : ""}
              </span>
            </>
          ) : null}
        </p>

        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
            gap: "12px 16px",
            maxWidth: 720,
          }}
        >
          <div className="field" style={{ margin: 0, gridColumn: "1 / -1", maxWidth: 420 }}>
            <label htmlFor="db-provider">Tipo</label>
            <select
              id="db-provider"
              className="input"
              value={provider}
              onChange={(e) => onProviderChange(e.target.value === "external" ? "external" : "local")}
            >
              <option value="local">Local (Docker / este computador)</option>
              <option value="external">Externo (host / IP)</option>
            </select>
          </div>

          <div className="field" style={{ margin: 0, gridColumn: "1 / -1", maxWidth: 520 }}>
            <label htmlFor="db-host">Servidor</label>
            <input
              id="db-host"
              className="input mono"
              value={host}
              onChange={(e) => setHost(e.target.value)}
              placeholder={provider === "local" ? "127.0.0.1 ou postgres" : "db.exemplo.com"}
              autoComplete="off"
              spellCheck={false}
            />
          </div>

          <div className="field" style={{ margin: 0 }}>
            <label htmlFor="db-port">Porta</label>
            <input
              id="db-port"
              className="input"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              placeholder="5432"
              inputMode="numeric"
              autoComplete="off"
            />
          </div>

          <div className="field" style={{ margin: 0 }}>
            <label htmlFor="db-name">Base</label>
            <input
              id="db-name"
              className="input"
              value={dbName}
              onChange={(e) => setDbName(e.target.value)}
              placeholder="netquasar"
              autoComplete="off"
            />
          </div>

          <div className="field" style={{ margin: 0 }}>
            <label htmlFor="db-user">Usuário</label>
            <input
              id="db-user"
              className="input"
              value={dbUser}
              onChange={(e) => setDbUser(e.target.value)}
              placeholder={hasMaskedDbUser(meta.data) ? "(manter atual)" : "postgres"}
              autoComplete="off"
            />
          </div>

          <div className="field" style={{ margin: 0 }}>
            <label htmlFor="db-pass">Palavra-passe</label>
            <input
              id="db-pass"
              className="input"
              type="password"
              autoComplete="new-password"
              value={dbPass}
              onChange={(e) => setDbPass(e.target.value)}
              placeholder={meta.data?.password_configured ? "(manter atual)" : ""}
            />
          </div>

          <div className="field" style={{ margin: 0, gridColumn: "1 / -1" }}>
            <span id="db-ssl-label" style={{ display: "block", marginBottom: 6, fontWeight: 600, fontSize: 13 }}>
              SSL
            </span>
            <div className="row" role="radiogroup" aria-labelledby="db-ssl-label" style={{ flexWrap: "wrap", gap: 16 }}>
              <label className="row" style={{ gap: 8, cursor: "pointer", fontSize: 13 }}>
                <input type="radio" name="db-ssl-mode" checked={sslChoice === "disable"} onChange={() => setSslMode("disable")} />
                <span>disable (local / Docker)</span>
              </label>
              <label className="row" style={{ gap: 8, cursor: "pointer", fontSize: 13 }}>
                <input type="radio" name="db-ssl-mode" checked={sslChoice === "require"} onChange={() => setSslMode("require")} />
                <span>require (remoto / nuvem)</span>
              </label>
            </div>
          </div>

          {provider === "external" && (
            <div className="field" style={{ margin: 0, gridColumn: "1 / -1", maxWidth: 560 }}>
              <label htmlFor="db-url">URL completa (opcional)</label>
              <input
                id="db-url"
                className="input mono"
                value={dbUrl}
                onChange={(e) => setDbUrl(e.target.value)}
                placeholder="postgres://user:pass@host:5432/dbname?sslmode=require"
                spellCheck={false}
                autoComplete="off"
              />
            </div>
          )}
        </div>

        <label className="row" style={{ gap: 10, marginTop: 14, alignItems: "flex-start", maxWidth: 560 }}>
          <input type="checkbox" checked={apply} onChange={(e) => setApply(e.target.checked)} style={{ marginTop: 3 }} />
          <span style={{ fontSize: 13, lineHeight: 1.4 }}>
            Aplicar já esta ligação (valida, migra e passa a usar esta base)
          </span>
        </label>

        <div className="row" style={{ marginTop: 14, flexWrap: "wrap", gap: 8 }}>
          <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate(buildPatchBody())}>
            Salvar
          </button>
          <button type="button" className="btn" disabled={testConn.isPending} onClick={runTestConnection}>
            Testar ligação
          </button>
        </div>

        {dbToast && (
          <div
            className={`page-toast ${dbToast.ok ? "page-toast--ok" : "page-toast--err"}`}
            role="status"
            style={{ marginTop: 12, maxWidth: 560 }}
          >
            <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setDbToast(null)}>
              ×
            </button>
            {dbToast.text}
          </div>
        )}

        <RestoreSection />
      </div>

      <SystemConfigBackupPanel />
      <div className="card" style={{ marginTop: 0 }}>
        <DatabaseCleanupButton embedded />
      </div>
    </div>
  );
}
