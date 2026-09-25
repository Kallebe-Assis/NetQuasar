import { useMemo, useRef, useState } from "react";
import { apiFetch } from "../../lib/api";

/**
 * PROVISÓRIO — só administradores. Correção em lote da "data da venda" dos serviços HubSoft a partir de
 * um CSV. Fluxo: exportar base → preencher data_venda_nova → pré-visualizar (conferência por login + id +
 * código + nome, no servidor) → aplicar em lotes pequenos, parando ao primeiro sinal de inconsistência.
 */

const SLUG = "hubsoft";
const BASE = `/api/v1/integrations/${SLUG}/hubsoft/data-venda`;
const APPLY_BATCH = 10;
const APPLY_MAX = 500;

type InputRow = { line: number; login: string; service_id: string; client_code: string; client_name: string; new_date: string };
type PreviewRow = {
  line: number;
  login: string;
  service_id?: string;
  client_id?: string;
  client_code?: string;
  client_name?: string;
  status?: string;
  current_date?: string;
  new_date?: string;
  approved: boolean;
  reason?: string;
  warnings?: string[];
};
type PreviewResp = { ok: boolean; message?: string; rows: PreviewRow[]; approved: number; blocked: number; base_size: number };
type ApplyResult = { line: number; service_id: string; login: string; ok: boolean; halt?: boolean; message: string; date_before?: string; date_after?: string };
type ExportResp = {
  ok: boolean;
  message?: string;
  rows: { login: string; service_id: string; client_code: string; client_name: string; status: string; current_date?: string }[];
};

function csvEsc(v: string): string {
  return /[";,\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

function downloadCsv(name: string, head: string[], rows: string[][]) {
  const text = [head, ...rows].map((r) => r.map((c) => csvEsc(c ?? "")).join(";")).join("\r\n");
  const url = URL.createObjectURL(new Blob([`﻿${text}`], { type: "text/csv;charset=utf-8;" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

/** Parser CSV mínimo: separador ; ou , (detectado pelo cabeçalho), aspas duplas, BOM. */
function parseCsv(text: string): string[][] {
  const t = text.replace(/^﻿/, "");
  const firstLine = t.split(/\r?\n/, 1)[0] ?? "";
  const sep = (firstLine.match(/;/g)?.length ?? 0) >= (firstLine.match(/,/g)?.length ?? 0) ? ";" : ",";
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let q = false;
  for (let i = 0; i < t.length; i++) {
    const c = t[i];
    if (q) {
      if (c === '"' && t[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (c === '"') q = false;
      else cell += c;
    } else if (c === '"') q = true;
    else if (c === sep) {
      row.push(cell);
      cell = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && t[i + 1] === "\n") i++;
      row.push(cell);
      cell = "";
      if (row.some((x) => x.trim() !== "")) rows.push(row);
      row = [];
    } else cell += c;
  }
  row.push(cell);
  if (row.some((x) => x.trim() !== "")) rows.push(row);
  return rows;
}

const HEADER_ALIASES: Record<string, string[]> = {
  login: ["login_pppoe", "login", "pppoe"],
  service_id: ["id_cliente_servico", "id_servico", "id"],
  client_code: ["codigo_cliente", "codigo"],
  client_name: ["cliente", "nome", "nome_razaosocial"],
  new_date: ["data_venda_nova", "nova_data", "data_nova"],
};

function mapCsv(rows: string[][]): { items: InputRow[]; error?: string; skipped: number } {
  if (rows.length < 2) return { items: [], error: "CSV vazio.", skipped: 0 };
  const head = rows[0].map((h) => h.trim().toLowerCase());
  const idx: Record<string, number> = {};
  for (const [k, names] of Object.entries(HEADER_ALIASES)) {
    idx[k] = head.findIndex((h) => names.includes(h));
  }
  const missing = ["login", "service_id", "client_name", "new_date"].filter((k) => idx[k] < 0);
  if (missing.length) {
    return {
      items: [],
      error: `Colunas obrigatórias ausentes: ${missing.map((k) => HEADER_ALIASES[k][0]).join(", ")}. Use o CSV exportado por aqui.`,
      skipped: 0,
    };
  }
  const get = (r: string[], k: string) => (idx[k] >= 0 ? (r[idx[k]] ?? "").trim() : "");
  const items: InputRow[] = [];
  let skipped = 0;
  rows.slice(1).forEach((r, i) => {
    const nd = get(r, "new_date");
    if (!nd) {
      skipped++;
      return;
    }
    items.push({ line: i + 2, login: get(r, "login"), service_id: get(r, "service_id"), client_code: get(r, "client_code"), client_name: get(r, "client_name"), new_date: nd });
  });
  return { items, skipped };
}

type Filter = "all" | "approved" | "blocked";

export function HubsoftDataVendaSection() {
  const fileRef = useRef<HTMLInputElement>(null);
  const [includeCancelled, setIncludeCancelled] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [items, setItems] = useState<InputRow[]>([]);
  const [fileName, setFileName] = useState("");
  const [skipped, setSkipped] = useState(0);
  const [err, setErr] = useState("");
  const [previewing, setPreviewing] = useState(false);
  const [preview, setPreview] = useState<PreviewResp | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [results, setResults] = useState<ApplyResult[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  const approved = useMemo(() => (preview?.rows ?? []).filter((r) => r.approved), [preview]);
  const shown = useMemo(() => {
    const all = preview?.rows ?? [];
    return filter === "all" ? all : all.filter((r) => (filter === "approved" ? r.approved : !r.approved));
  }, [preview, filter]);
  const overLimit = approved.length > APPLY_MAX;
  const confirmText = `APLICAR ${approved.length}`;

  async function doExport() {
    setExporting(true);
    setErr("");
    try {
      const r = await apiFetch<ExportResp>(`${BASE}/export?include_cancelled=${includeCancelled ? 1 : 0}`, { timeoutMs: 6 * 60_000 });
      if (!r.ok) throw new Error(r.message || "Falha ao exportar.");
      const rows = [...r.rows].sort((a, b) => a.login.toLowerCase().localeCompare(b.login.toLowerCase()));
      downloadCsv(
        `servicos-hubsoft-${new Date().toISOString().slice(0, 10)}.csv`,
        ["login_pppoe", "id_cliente_servico", "codigo_cliente", "cliente", "status", "data_venda_atual", "data_venda_nova"],
        rows.map((x) => [x.login, x.service_id, x.client_code, x.client_name, x.status, x.current_date ?? "", ""]),
      );
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setExporting(false);
    }
  }

  async function onFile(f: File | undefined) {
    setPreview(null);
    setResults([]);
    setStopMsg("");
    setAck(false);
    setTyped("");
    setErr("");
    if (!f) return;
    setFileName(f.name);
    const m = mapCsv(parseCsv(await f.text()));
    setItems(m.items);
    setSkipped(m.skipped);
    if (m.error) setErr(m.error);
    else if (m.items.length === 0) setErr("Nenhuma linha com data_venda_nova preenchida.");
  }

  async function doPreview() {
    setPreviewing(true);
    setErr("");
    setPreview(null);
    setResults([]);
    setAck(false);
    setTyped("");
    try {
      const r = await apiFetch<PreviewResp>(`${BASE}/preview`, {
        method: "POST",
        json: { rows: items, include_cancelled: includeCancelled },
        timeoutMs: 6 * 60_000,
      });
      if (!r.ok) throw new Error(r.message || "Falha na pré-visualização.");
      setPreview(r);
      setFilter(r.blocked > 0 ? "blocked" : "all");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setPreviewing(false);
    }
  }

  function downloadPlan() {
    if (!preview) return;
    downloadCsv(
      `plano-data-venda-${new Date().toISOString().slice(0, 10)}.csv`,
      ["linha", "login", "id_servico", "cliente", "codigo", "status", "data_atual", "data_nova", "decisao", "motivo", "avisos"],
      preview.rows.map((r) => [String(r.line), r.login, r.service_id ?? "", r.client_name ?? "", r.client_code ?? "", r.status ?? "", r.current_date ?? "", r.new_date ?? "", r.approved ? "aprovada" : "bloqueada", r.reason ?? "", (r.warnings ?? []).join(" | ")]),
    );
  }

  function downloadResults() {
    downloadCsv(
      `resultado-data-venda-${new Date().toISOString().slice(0, 10)}.csv`,
      ["linha", "login", "id_servico", "resultado", "mensagem", "data_antes", "data_depois"],
      results.map((r) => [String(r.line), r.login, r.service_id, r.ok ? "ok" : r.halt ? "PAROU" : "erro", r.message, r.date_before ?? "", r.date_after ?? ""]),
    );
  }

  async function doApply() {
    stopRef.current = false;
    setApplying(true);
    setStopMsg("");
    setResults([]);
    const queue = approved.slice(0, APPLY_MAX);
    const acc: ApplyResult[] = [];
    let consecutiveErr = 0;
    try {
      for (let i = 0; i < queue.length && !stopRef.current; i += APPLY_BATCH) {
        const batch = queue.slice(i, i + APPLY_BATCH);
        const r = await apiFetch<{ results: ApplyResult[]; halted: boolean }>(`${BASE}/apply`, {
          method: "POST",
          json: { rows: batch.map((b) => ({ line: b.line, login: b.login, service_id: b.service_id, client_id: b.client_id, client_name: b.client_name, new_date: b.new_date })) },
          timeoutMs: 3 * 60_000,
        });
        acc.push(...r.results);
        setResults([...acc]);
        if (r.halted) {
          setStopMsg("EXECUÇÃO INTERROMPIDA por inconsistência grave. Confira o último item antes de continuar.");
          break;
        }
        for (const x of r.results) consecutiveErr = x.ok ? 0 : consecutiveErr + 1;
        if (consecutiveErr >= 3) {
          setStopMsg("Interrompido: 3 falhas seguidas.");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador.");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Confira o resultado antes de repetir.`);
    } finally {
      setApplying(false);
    }
  }

  const okCount = results.filter((r) => r.ok).length;
  const canApply = !!preview && approved.length > 0 && !overLimit && ack && typed.trim() === confirmText && !applying && results.length === 0;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div className="card" style={{ padding: 14 }}>
        <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Correção em lote da data de venda</h3>
        <p style={{ fontSize: 12, color: "var(--warn)", margin: "0 0 10px" }}>
          Ferramenta provisória, só para administradores. Altera dados de produção na HubSoft — cada serviço é conferido por login PPPoE, id, código e nome do cliente antes e depois da alteração.
        </p>
        <ol style={{ margin: 0, paddingLeft: 18, fontSize: 12, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 4 }}>
          <li>Exporte o CSV com todos os serviços e preencha a coluna <b>data_venda_nova</b> (AAAA-MM-DD ou DD/MM/AAAA). Não altere login, id, código nem cliente.</li>
          <li>Envie o CSV preenchido e gere a pré-visualização — nada é alterado nesta etapa.</li>
          <li>Revise o que foi aprovado/bloqueado, confirme e aplique (máx. {APPLY_MAX} por execução).</li>
        </ol>
      </div>

      <div className="card" style={{ padding: 14 }}>
        <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>1 · Exportar base</h4>
        <div className="row" style={{ gap: 10, alignItems: "center", flexWrap: "wrap" }}>
          <button type="button" className="btn btn--sm btn--primary" disabled={exporting} onClick={() => void doExport()}>
            {exporting ? "Lendo a HubSoft…" : "Exportar CSV de todos os serviços"}
          </button>
          <label className="row" style={{ gap: 6, fontSize: 12 }}>
            <input type="checkbox" checked={includeCancelled} onChange={(e) => setIncludeCancelled(e.target.checked)} />
            Incluir serviços cancelados
          </label>
        </div>
      </div>

      <div className="card" style={{ padding: 14 }}>
        <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>2 · Enviar CSV preenchido e pré-visualizar</h4>
        <div className="row" style={{ gap: 10, alignItems: "center", flexWrap: "wrap" }}>
          <input ref={fileRef} type="file" accept=".csv,text/csv" onChange={(e) => void onFile(e.target.files?.[0])} />
          <button type="button" className="btn btn--sm btn--primary" disabled={previewing || items.length === 0} onClick={() => void doPreview()}>
            {previewing ? "Conferindo com a HubSoft…" : "Pré-visualizar alterações"}
          </button>
        </div>
        {fileName ? (
          <p style={{ fontSize: 12, color: "var(--muted)", margin: "8px 0 0" }}>
            {fileName}: {items.length} linha(s) com nova data{skipped ? `, ${skipped} sem data (ignoradas)` : ""}.
          </p>
        ) : null}
        {err ? <div className="msg msg--err" style={{ marginTop: 8 }}>{err}</div> : null}
      </div>

      {preview ? (
        <div className="card" style={{ padding: 14 }}>
          <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>3 · Pré-visualização</h4>
          <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(3, minmax(0, 180px))", marginBottom: 10 }}>
            <div className="stat"><div className="stat__k">Aprovadas</div><div className="stat__v" style={{ color: "var(--ok)" }}>{approved.length}</div></div>
            <div className="stat"><div className="stat__k">Bloqueadas</div><div className="stat__v" style={{ color: preview.blocked ? "var(--err)" : undefined }}>{preview.blocked}</div></div>
            <div className="stat"><div className="stat__k">Serviços na HubSoft</div><div className="stat__v">{preview.base_size}</div></div>
          </div>
          <div className="row" style={{ gap: 6, marginBottom: 8, flexWrap: "wrap" }}>
            {(["all", "approved", "blocked"] as Filter[]).map((f) => (
              <button key={f} type="button" className={`btn btn--sm${filter === f ? " btn--primary" : ""}`} onClick={() => setFilter(f)}>
                {f === "all" ? "Todas" : f === "approved" ? "Aprovadas" : "Bloqueadas"}
              </button>
            ))}
            <button type="button" className="btn btn--sm" style={{ marginLeft: "auto" }} onClick={downloadPlan}>Baixar plano (CSV)</button>
          </div>
          <div className="table-wrap integration-support-table" style={{ maxHeight: 460, overflow: "auto" }}>
            <table className="integration-support-table__grid">
              <thead>
                <tr><th>Linha</th><th>Login PPPoE</th><th>ID</th><th>Cliente</th><th>Status</th><th>Data atual</th><th>Nova data</th><th>Decisão</th></tr>
              </thead>
              <tbody>
                {shown.map((r) => (
                  <tr key={r.line}>
                    <td className="mono integration-support-table__cell">{r.line}</td>
                    <td className="mono integration-support-table__cell">{r.login}</td>
                    <td className="mono integration-support-table__cell">{r.service_id || "—"}</td>
                    <td className="integration-support-table__cell">{r.client_name || "—"}</td>
                    <td className="integration-support-table__cell">{r.status || "—"}</td>
                    <td className="mono integration-support-table__cell">{r.current_date || "—"}</td>
                    <td className="mono integration-support-table__cell">{r.new_date || "—"}</td>
                    <td className="integration-support-table__cell" style={{ color: r.approved ? "var(--ok)" : "var(--err)" }}>
                      {r.approved ? "Aprovada" : `Bloqueada: ${r.reason}`}
                      {(r.warnings ?? []).map((w) => (
                        <div key={w} style={{ color: "var(--warn)", fontSize: 11 }}>⚠ {w}</div>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {preview && approved.length > 0 ? (
        <div className="card" style={{ padding: 14 }}>
          <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>4 · Aplicar</h4>
          {overLimit ? (
            <div className="msg msg--err">São {approved.length} aprovadas; o máximo por execução é {APPLY_MAX}. Divida o CSV.</div>
          ) : (
            <>
              <label className="row" style={{ gap: 6, fontSize: 12, marginBottom: 8 }}>
                <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
                Revisei a pré-visualização e entendo que isto altera a data de venda de {approved.length} serviço(s) na HubSoft.
              </label>
              <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
                <input className="input" style={{ width: 170 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
                <button type="button" className="btn btn--sm btn--primary" disabled={!canApply} onClick={() => void doApply()}>
                  {applying ? "Aplicando…" : `Aplicar ${approved.length} alteração(ões)`}
                </button>
                {applying ? (
                  <button type="button" className="btn btn--sm" onClick={() => { stopRef.current = true; }}>Parar</button>
                ) : null}
              </div>
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0" }}>Digite exatamente “{confirmText}” para liberar o botão.</p>
            </>
          )}
        </div>
      ) : null}

      {results.length > 0 ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, flexWrap: "wrap", gap: 8 }}>
            <h4 style={{ margin: 0, fontSize: 13 }}>Resultado — {okCount} ok, {results.length - okCount} com problema {applying ? `(processando… ${results.length}/${Math.min(approved.length, APPLY_MAX)})` : ""}</h4>
            <button type="button" className="btn btn--sm" onClick={downloadResults}>Baixar resultado (CSV)</button>
          </div>
          {stopMsg ? <div className="msg msg--err" style={{ marginBottom: 8 }}>{stopMsg}</div> : null}
          <div className="table-wrap integration-support-table" style={{ maxHeight: 360, overflow: "auto" }}>
            <table className="integration-support-table__grid">
              <thead><tr><th>Linha</th><th>Login</th><th>ID</th><th>Resultado</th><th>Antes → Depois</th></tr></thead>
              <tbody>
                {results.map((r) => (
                  <tr key={`${r.line}-${r.service_id}`}>
                    <td className="mono integration-support-table__cell">{r.line}</td>
                    <td className="mono integration-support-table__cell">{r.login}</td>
                    <td className="mono integration-support-table__cell">{r.service_id}</td>
                    <td className="integration-support-table__cell" style={{ color: r.ok ? "var(--ok)" : "var(--err)" }}>{r.ok ? "OK" : r.halt ? "PAROU" : "Erro"} — {r.message}</td>
                    <td className="mono integration-support-table__cell">{r.date_before || "?"} → {r.date_after || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : stopMsg ? (
        <div className="msg msg--err">{stopMsg}</div>
      ) : null}
    </div>
  );
}
