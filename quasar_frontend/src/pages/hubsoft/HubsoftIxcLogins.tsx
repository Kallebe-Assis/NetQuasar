import { useMemo, useRef, useState } from "react";
import { Download, PowerOff, Search, Undo2 } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { downloadCsv, parseCsv } from "./hubsoftCsv";
import { useConsultaToast } from "./hubsoftConsulta";
import { Callout, CsvDropzone, Pill, ProgressBar, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Inativar logins no IXC em massa (migração para a HubSoft). Fluxo: CSV com os logins → conferir no IXC (somente leitura:
 * cliente, contrato, ativo/online) → inativar em lotes, com leitura de conferência depois de cada um → desfazer (reativar).
 * Inativar um login no IXC derruba a conexão PPPoE dele na hora.
 */

const BASE = "/api/v1/integrations/ixc/ixc/logins";
const PREVIEW_BATCH = 50;
const APPLY_BATCH = 10;
const MAX_LOGINS = 1000;

type PreviewRow = {
  login: string;
  status: "pronto" | "ja_inativo" | "nao_encontrado" | "ambiguo" | "erro";
  message?: string;
  id_login?: string;
  login_ixc?: string;
  ativo: boolean;
  online: boolean;
  id_cliente?: string;
  id_contrato?: string;
  cliente?: string;
};

type ApplyResult = {
  id_login: string;
  login: string;
  ok: boolean;
  changed: boolean;
  message: string;
  online_before: boolean;
  online_after: boolean;
};

const STATUS_LABEL: Record<PreviewRow["status"], { text: string; tone?: "ok" | "err" | "warn" }> = {
  pronto: { text: "Ativo — pode inativar", tone: "ok" },
  ja_inativo: { text: "Já inativo", tone: "warn" },
  nao_encontrado: { text: "Não encontrado", tone: "err" },
  ambiguo: { text: "Duplicado no IXC", tone: "err" },
  erro: { text: "Erro", tone: "err" },
};

/** Lê os logins do CSV: coluna "login" (ou "logins"/"usuario") se houver cabeçalho; senão a 1ª coluna. Sem repetidos. */
function loginsFromCsv(text: string): string[] {
  const rows = parseCsv(text);
  if (rows.length === 0) return [];
  const head = rows[0].map((h) => h.trim().toLowerCase());
  let col = head.findIndex((h) => h === "login" || h === "logins" || h === "usuario" || h === "usuário");
  let start = 0;
  if (col >= 0) start = 1;
  else col = 0;
  const seen = new Set<string>();
  const out: string[] = [];
  for (const r of rows.slice(start)) {
    const v = (r[col] ?? "").trim().replace(/^="?|"$/g, "");
    if (!v || seen.has(v.toLowerCase())) continue;
    seen.add(v.toLowerCase());
    out.push(v);
  }
  return out;
}

export function HubsoftIxcLogins() {
  const { notify } = useConsultaToast();
  const stopRef = useRef(false);
  const [fileName, setFileName] = useState("");
  const [logins, setLogins] = useState<string[]>([]);
  const [rows, setRows] = useState<PreviewRow[]>([]);
  const [checking, setChecking] = useState(false);
  const [checkProgress, setCheckProgress] = useState({ done: 0, total: 0 });
  const [ack, setAck] = useState(false);
  const [applying, setApplying] = useState(false);
  const [applyProgress, setApplyProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<ApplyResult[]>([]);
  const [undoing, setUndoing] = useState(false);
  const [err, setErr] = useState("");

  const ready = useMemo(() => rows.filter((r) => r.status === "pronto"), [rows]);
  const counts = useMemo(() => {
    const c = { pronto: 0, ja_inativo: 0, nao_encontrado: 0, ambiguo: 0, erro: 0, online: 0 };
    for (const r of rows) {
      c[r.status]++;
      if (r.status === "pronto" && r.online) c.online++;
    }
    return c;
  }, [rows]);
  const resultByLogin = useMemo(() => new Map(results.map((r) => [r.login.toLowerCase(), r])), [results]);
  const changed = results.filter((r) => r.ok && r.changed);

  function reset() {
    setFileName("");
    setLogins([]);
    setRows([]);
    setResults([]);
    setAck(false);
    setErr("");
  }

  async function onFile(f: File | undefined) {
    reset();
    if (!f) return;
    setFileName(f.name);
    const list = loginsFromCsv(await f.text());
    if (list.length === 0) {
      setErr("O CSV não tem logins. Use uma coluna chamada \"login\" (ou os logins na primeira coluna).");
      return;
    }
    if (list.length > MAX_LOGINS) {
      setErr(`No máximo ${MAX_LOGINS} logins por vez (o arquivo tem ${list.length}).`);
      return;
    }
    setLogins(list);
  }

  async function runCheck() {
    setChecking(true);
    setErr("");
    setRows([]);
    setResults([]);
    setAck(false);
    stopRef.current = false;
    setCheckProgress({ done: 0, total: logins.length });
    const acc: PreviewRow[] = [];
    try {
      for (let i = 0; i < logins.length && !stopRef.current; i += PREVIEW_BATCH) {
        const r = await apiFetch<{ rows: PreviewRow[] }>(`${BASE}/preview`, {
          method: "POST",
          json: { logins: logins.slice(i, i + PREVIEW_BATCH) },
          timeoutMs: 4 * 60_000,
        });
        acc.push(...r.rows);
        setRows([...acc]);
        setCheckProgress({ done: Math.min(i + PREVIEW_BATCH, logins.length), total: logins.length });
      }
    } catch (e) {
      setErr(`Consulta interrompida: ${(e as Error).message}`);
    } finally {
      setChecking(false);
    }
  }

  async function runBatches(action: "inativar" | "reativar", items: { id_login: string; login: string }[], onDone: (r: ApplyResult[]) => void) {
    const acc: ApplyResult[] = [];
    setApplyProgress({ done: 0, total: items.length });
    try {
      for (let i = 0; i < items.length; i += APPLY_BATCH) {
        const r = await apiFetch<{ results: ApplyResult[] }>(`${BASE}/apply`, {
          method: "POST",
          json: { action, items: items.slice(i, i + APPLY_BATCH) },
          timeoutMs: 4 * 60_000,
        });
        acc.push(...r.results);
        onDone([...acc]);
        setApplyProgress({ done: Math.min(i + APPLY_BATCH, items.length), total: items.length });
        if (r.results.length > 0 && r.results.every((x) => !x.ok) && i === 0) break; // falha sistêmica logo no 1º lote: para
      }
    } catch (e) {
      setErr(`Interrompido por erro de comunicação: ${(e as Error).message}. Rode a conferência de novo para ver o que já mudou.`);
    }
    return acc;
  }

  async function runApply() {
    setApplying(true);
    setErr("");
    const acc = await runBatches(
      "inativar",
      ready.filter((r) => r.id_login).map((r) => ({ id_login: r.id_login!, login: r.login_ixc || r.login })),
      setResults,
    );
    setApplying(false);
    notify({ ok: true }, null, `${acc.filter((r) => r.ok && r.changed).length} de ${acc.length} login(s) inativado(s) no IXC.`);
  }

  async function runUndo() {
    setUndoing(true);
    setErr("");
    const acc = await runBatches(
      "reativar",
      changed.map((r) => ({ id_login: r.id_login, login: r.login })),
      () => undefined,
    );
    setUndoing(false);
    const okLogins = new Set(acc.filter((r) => r.ok).map((r) => r.login.toLowerCase()));
    setResults((cur) => cur.map((r) => (okLogins.has(r.login.toLowerCase()) ? { ...r, changed: false, message: "reativado no IXC (desfeito)" } : r)));
    notify({ ok: true }, null, `${okLogins.size} login(s) reativado(s) no IXC.`);
  }

  function download() {
    downloadCsv(
      `inativacao-logins-ixc-${new Date().toISOString().slice(0, 10)}.csv`,
      ["login", "id_login_ixc", "cliente", "id_contrato", "situacao_antes", "online_antes", "resultado"],
      rows.map((r) => [
        r.login,
        r.id_login ?? "",
        r.cliente ?? "",
        r.id_contrato ?? "",
        STATUS_LABEL[r.status].text,
        r.online ? "sim" : "não",
        resultByLogin.get(r.login.toLowerCase())?.message ?? r.message ?? "",
      ]),
    );
  }

  return (
    <ToolPanel
      icon={<PowerOff size={20} />}
      title="Inativar logins no IXC"
      badge="Altera o IXC"
      badgeTone="warn"
      subtitle="Inativa em massa os logins (radusuarios) de uma lista, para o cliente passar a conectar pela HubSoft. O IXC derruba a conexão PPPoE do login inativado na hora."
    >
      <Step n={1} title="Enviar o CSV" done={logins.length > 0} hint='Uma coluna chamada "login" (ou os logins na primeira coluna). Nada é enviado ao IXC nesta etapa.'>
        <CsvDropzone
          fileName={fileName}
          info={logins.length > 0 ? `${logins.length} login(s) único(s)` : undefined}
          disabled={checking || applying}
          onFile={(f) => void onFile(f)}
          onClear={reset}
        />
        {err ? <Callout tone="err">{err}</Callout> : null}
      </Step>

      <Step n={2} title="Conferir no IXC" done={rows.length > 0 && !checking} hint="Somente leitura: mostra cliente, contrato e se o login está ativo e online agora.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={logins.length === 0 || checking || applying} onClick={() => void runCheck()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {checking ? "Consultando o IXC…" : rows.length > 0 ? "Conferir de novo" : `Conferir ${logins.length || ""} login(s)`}
          </button>
          {checking ? (
            <button type="button" className="btn btn--sm" onClick={() => (stopRef.current = true)}>
              Parar
            </button>
          ) : null}
        </div>
        {checking ? <ProgressBar done={checkProgress.done} total={checkProgress.total} label={`Consultando… ${checkProgress.done} de ${checkProgress.total}`} /> : null}
        {rows.length > 0 ? (
          <>
            <div className="hsa-stats">
              <Stat label="Podem ser inativados" value={counts.pronto} tone={counts.pronto ? "ok" : "muted"} />
              <Stat label="Online agora" value={counts.online} tone={counts.online ? "warn" : "muted"} />
              <Stat label="Já inativos" value={counts.ja_inativo} tone="muted" />
              <Stat label="Não encontrados" value={counts.nao_encontrado} tone={counts.nao_encontrado ? "err" : "muted"} />
              <Stat label="Duplicados / erro" value={counts.ambiguo + counts.erro} tone={counts.ambiguo + counts.erro ? "err" : "muted"} />
            </div>
            <div className="hsa-table-wrap" style={{ maxHeight: 420 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <th>Login</th>
                    <th>Cliente</th>
                    <th>Contrato</th>
                    <th>Online</th>
                    <th>Situação</th>
                    <th>Resultado</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => {
                    const st = STATUS_LABEL[r.status];
                    const res = resultByLogin.get(r.login.toLowerCase());
                    return (
                      <tr key={r.login}>
                        <td className="mono">{r.login}</td>
                        <td>{r.cliente || <span className="hsa-muted">—</span>}</td>
                        <td className="mono">{r.id_contrato || "—"}</td>
                        <td>{r.status === "pronto" || r.status === "ja_inativo" ? (r.online ? "sim" : "não") : "—"}</td>
                        <td>
                          <Pill tone={st.tone}>{st.text}</Pill>
                          {r.message ? <div className="hsa-muted">{r.message}</div> : null}
                        </td>
                        <td>{res ? <span style={{ color: res.ok ? "var(--ok)" : "var(--err)" }}>{res.message}</span> : <span className="hsa-muted">—</span>}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </>
        ) : null}
      </Step>

      {rows.length > 0 && !checking ? (
        <Step n={3} title="Inativar" done={results.length > 0 && !applying} hint="Cada login é conferido de novo antes e depois; o estado anterior fica no histórico.">
          {counts.online > 0 ? (
            <Callout tone="warn">
              {counts.online} desses login(s) estão online agora e cairão assim que forem inativados — eles precisam estar prontos para conectar pela HubSoft.
            </Callout>
          ) : null}
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} disabled={applying || undoing} />
            Entendo que isto inativa {ready.length} login(s) no IXC e derruba a conexão PPPoE deles.
          </label>
          <div className="hsa-actions">
            <button type="button" className="btn btn--primary" disabled={!ack || applying || undoing || ready.length === 0 || results.length > 0} onClick={() => void runApply()}>
              <PowerOff size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              {applying ? "Inativando…" : `Inativar ${ready.length}`}
            </button>
            {changed.length > 0 ? (
              <button type="button" className="btn btn--sm" disabled={applying || undoing} onClick={() => void runUndo()}>
                <Undo2 size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                {undoing ? "Reativando…" : `Desfazer: reativar ${changed.length}`}
              </button>
            ) : null}
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={download}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar resultado (CSV)
            </button>
          </div>
          {applying || undoing ? <ProgressBar done={applyProgress.done} total={applyProgress.total} label={`${undoing ? "Reativando" : "Inativando"}… ${applyProgress.done} de ${applyProgress.total}`} /> : null}
          {results.length > 0 && !applying ? (
            <Callout tone={results.every((r) => r.ok) ? "info" : "warn"}>
              {changed.length} inativado(s) de {results.length} processado(s)
              {results.some((r) => !r.ok) ? `, ${results.filter((r) => !r.ok).length} com erro (veja a coluna Resultado)` : ""}. Rode “Conferir de novo” para confirmar o estado no IXC.
            </Callout>
          ) : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
