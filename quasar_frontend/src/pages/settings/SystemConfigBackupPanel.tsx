/**
 * SystemConfigBackupPanel — exportação/importação do pacote JSON de configuração do sistema
 * (aba Base de dados em Configurações).
 */
import { useEffect, useRef, useState } from "react";
import { Download, Upload } from "lucide-react";
import { apiFetch, ApiError } from "../../lib/api";
import { apiUrl, getAuthToken, getStoredApiKey } from "../../lib/auth";
import { toastErr, toastOk } from "../../lib/operationToast";
import { useAppToast } from "../../lib/appToast";
import { InfoHint } from "../../components/InfoHint";

/** Estado devolvido pelo job assíncrono de importação no backend. */
type ImportJobStatus = {
  job_id: string;
  status: "running" | "done" | "failed";
  progress_pct: number;
  current_step: string;
  steps_total: number;
  steps_done: number;
  logs: string[];
  errors: string[];
  started_at: string;
  finished_at?: string;
};

/** Opções enviadas no POST de importação. */
type ImportOptions = {
  apply_database_connection: boolean;
  import_users: boolean;
  overwrite_alert_rules: boolean;
};

const POLL_MS = 500;

/** Extrai nome de ficheiro do cabeçalho Content-Disposition. */
function filenameFromDisposition(header: string | null): string {
  if (!header) return `netquasar-config-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.json`;
  const m = /filename="([^"]+)"/i.exec(header);
  return m?.[1] ?? "netquasar-config.json";
}

export function SystemConfigBackupPanel() {
  const { push: pushToast } = useAppToast();
  const fileRef = useRef<HTMLInputElement>(null);

  // —— estado de exportação ——
  const [exporting, setExporting] = useState(false);

  // —— estado de importação ——
  const [importing, setImporting] = useState(false);
  const [importFileName, setImportFileName] = useState("");
  const [job, setJob] = useState<ImportJobStatus | null>(null);
  const [pollJobId, setPollJobId] = useState<string | null>(null);
  const [options, setOptions] = useState<ImportOptions>({
    apply_database_connection: false,
    import_users: false,
    overwrite_alert_rules: false,
  });

  /** Polling estável: depende só do jobId, não de cada update de progresso. */
  useEffect(() => {
    if (!pollJobId) return;
    let cancelled = false;
    const poll = async () => {
      try {
        const next = await apiFetch<ImportJobStatus>(`/api/v1/settings/system-config/import/${pollJobId}`);
        if (cancelled) return;
        setJob(next);
        if (next.status === "done") {
          setPollJobId(null);
          setImporting(false);
          toastOk(pushToast, "Configurações importadas com sucesso.");
        } else if (next.status === "failed") {
          setPollJobId(null);
          setImporting(false);
          toastErr(pushToast, next.errors[0] ?? "Importação falhou. Consulte os logs abaixo.");
        }
      } catch (e) {
        if (cancelled) return;
        setPollJobId(null);
        setImporting(false);
        toastErr(pushToast, e instanceof Error ? e.message : "Erro ao consultar progresso.");
      }
    };
    const id = window.setInterval(() => void poll(), POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [pollJobId, pushToast]);

  /** GET /settings/system-config/export — descarrega JSON organizado. */
  const handleExport = async () => {
    setExporting(true);
    try {
      const headers = new Headers();
      const token = getAuthToken();
      if (token) headers.set("Authorization", `Bearer ${token}`);
      const key = getStoredApiKey();
      if (key) headers.set("X-API-Key", key);

      const res = await fetch(apiUrl("/api/v1/settings/system-config/export"), { headers, credentials: "include" });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        const msg = (body as { error?: string }).error ?? res.statusText;
        throw new ApiError(msg, res.status);
      }
      const blob = await res.blob();
      const filename = filenameFromDisposition(res.headers.get("content-disposition"));
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(url);
      toastOk(pushToast, "Backup exportado. Guarde o ficheiro em local seguro (contém segredos).");
    } catch (e) {
      toastErr(pushToast, e instanceof Error ? e.message : "Falha na exportação.");
    } finally {
      setExporting(false);
    }
  };

  /** Lê ficheiro JSON local e inicia POST de importação. */
  const handleFileChosen = async (file: File) => {
    setImportFileName(file.name);
    setImporting(true);
    setJob(null);
    try {
      const text = await file.text();
      const bundle = JSON.parse(text) as Record<string, unknown>;
      const resp = await apiFetch<{ job_id: string }>("/api/v1/settings/system-config/import", {
        method: "POST",
        json: { bundle, options },
      });
      const initial = await apiFetch<ImportJobStatus>(`/api/v1/settings/system-config/import/${resp.job_id}`);
      setJob(initial);
      if (initial.status === "running") {
        setPollJobId(resp.job_id);
      } else {
        setImporting(false);
      }
    } catch (e) {
      setImporting(false);
      if (e instanceof SyntaxError) {
        toastErr(pushToast, "Ficheiro JSON inválido.");
      } else {
        toastErr(pushToast, e instanceof Error ? e.message : "Não foi possível iniciar a importação.");
      }
    }
  };

  const progressPct = job?.progress_pct ?? 0;
  const stepLabel = job?.current_step ?? (importing ? "A iniciar…" : "");

  return (
    <div className="card" style={{ marginTop: 20 }}>
      <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6, marginTop: 0 }}>
        Backup de configuração
        <InfoHint label="Exportar / importar definições">
          <p>
            Exporta todas as definições do sistema em JSON: ligação PostgreSQL, credenciais SNMP/SSH, monitoramento, SMTP, Telegram, regras de alerta, perfis OLT,
            integrações, automações e variáveis <span className="mono">NETQUASAR_*</span> do ambiente.
          </p>
          <p>
            <strong>Atenção:</strong> o ficheiro contém segredos (palavras-passe, tokens). Não partilhe publicamente.
          </p>
          <p>A importação aplica secções por ordem, com progresso e logs. Não inclui inventário (equipamentos, ONUs, etc.).</p>
        </InfoHint>
      </h2>

      <p style={{ color: "var(--muted)", fontSize: 13, marginTop: 0 }}>
        Exporte um snapshot completo ou restaure definições a partir de um backup JSON anterior.
      </p>

      <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: 12 }}>
        <button type="button" className="btn btn--primary" disabled={exporting || importing} onClick={() => void handleExport()}>
          <Download size={16} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
          {exporting ? "A exportar…" : "Exportar configurações (JSON)"}
        </button>
        <button
          type="button"
          className="btn"
          disabled={exporting || importing}
          onClick={() => fileRef.current?.click()}
        >
          <Upload size={16} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
          {importing ? "A importar…" : "Importar backup…"}
        </button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          style={{ display: "none" }}
          onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (f) void handleFileChosen(f);
          }}
        />
      </div>

      <fieldset style={{ border: "none", padding: 0, marginTop: 16 }}>
        <legend style={{ fontWeight: 600, fontSize: 13, marginBottom: 8 }}>Opções de importação</legend>
        <label className="row" style={{ gap: 8, alignItems: "flex-start", marginBottom: 8, cursor: "pointer", maxWidth: 640 }}>
          <input
            type="checkbox"
            checked={options.overwrite_alert_rules}
            disabled={importing}
            onChange={(e) => setOptions((o) => ({ ...o, overwrite_alert_rules: e.target.checked }))}
            style={{ marginTop: 3 }}
          />
          <span style={{ fontSize: 13, lineHeight: 1.45 }}>
            <strong>Substituir regras de alerta</strong> — apaga regras existentes antes de importar.
          </span>
        </label>
        <label className="row" style={{ gap: 8, alignItems: "flex-start", marginBottom: 8, cursor: "pointer", maxWidth: 640 }}>
          <input
            type="checkbox"
            checked={options.import_users}
            disabled={importing}
            onChange={(e) => setOptions((o) => ({ ...o, import_users: e.target.checked }))}
            style={{ marginTop: 3 }}
          />
          <span style={{ fontSize: 13, lineHeight: 1.45 }}>
            <strong>Importar utilizadores</strong> — cria/atualiza por e-mail (palavra-passe temporária; redefinir depois).
          </span>
        </label>
        <label className="row" style={{ gap: 8, alignItems: "flex-start", cursor: "pointer", maxWidth: 640 }}>
          <input
            type="checkbox"
            checked={options.apply_database_connection}
            disabled={importing}
            onChange={(e) => setOptions((o) => ({ ...o, apply_database_connection: e.target.checked }))}
            style={{ marginTop: 3 }}
          />
          <span style={{ fontSize: 13, lineHeight: 1.45 }}>
            <strong>Aplicar ligação PostgreSQL</strong> — grava metadados e solicita refresh do pool (não troca DSN automaticamente como «Aplicar já»).
          </span>
        </label>
      </fieldset>

      {importing && (
        <div className="conn-import-modal__loading" role="status" style={{ marginTop: 16, padding: 14, borderRadius: 8, background: "var(--surface-2, rgba(0,0,0,.04))" }}>
          <span className="page-toast__spinner" aria-hidden />
          <div style={{ flex: 1, minWidth: 0 }}>
            <strong>A importar configurações…</strong>
            {importFileName ? (
              <div style={{ fontSize: 12, color: "var(--muted)", marginTop: 4 }}>{importFileName}</div>
            ) : null}
            {stepLabel ? (
              <div style={{ fontSize: 12, marginTop: 6 }}>{stepLabel}</div>
            ) : null}
            <div className="conn-import-progress" style={{ marginTop: 10 }}>
              <div className="conn-import-progress__bar" aria-hidden>
                <div className="conn-import-progress__fill" style={{ width: `${Math.min(100, progressPct)}%` }} />
              </div>
              <div className="conn-import-progress__meta">
                <span>
                  {job ? `${job.steps_done} / ${job.steps_total} passos` : "A preparar…"}
                </span>
                <span>{progressPct}%</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {job && job.logs.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h3 style={{ fontSize: 14, marginBottom: 8 }}>Log {job.status === "failed" ? "(com erros)" : job.status === "done" ? "(sucesso)" : ""}</h3>
          <pre
            className="mono"
            style={{
              fontSize: 11,
              maxHeight: 220,
              overflow: "auto",
              padding: 12,
              margin: 0,
              borderRadius: 6,
              background: "var(--surface-2, rgba(0,0,0,.04))",
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
            }}
          >
            {job.logs.join("\n")}
          </pre>
          {job.errors.length > 0 && (
            <ul style={{ color: "var(--danger, #c44)", fontSize: 12, marginTop: 8 }}>
              {job.errors.map((err, i) => (
                <li key={i}>{err}</li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
