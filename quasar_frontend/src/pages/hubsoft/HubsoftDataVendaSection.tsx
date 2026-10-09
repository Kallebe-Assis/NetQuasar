import { useMemo, useRef, useState } from "react";
import { apiFetch } from "../../lib/api";
import { CalendarClock, Download, Eye } from "lucide-react";
import { downloadCsv, parseCsv } from "./hubsoftCsv";
import { Callout, CsvDropzone, Pill, ProgressBar, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Correção em lote da "data da venda" dos serviços HubSoft a partir de
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

export const HEADER_ALIASES: Record<string, string[]> = {
  login: ["login_pppoe", "login", "pppoe"],
  service_id: ["id_cliente_servico", "id_servico", "id"],
  client_code: ["codigo_cliente", "codigo"],
  client_name: ["cliente", "nome", "nome_razaosocial"],
  new_date: ["data_venda_nova", "nova_data", "data_nova"],
};

export type { InputRow, PreviewResp, PreviewRow };

/** Lê o CSV de datas. `extraDateAliases` aceita outros nomes para a coluna da data (ex.: «data_venda» na conferência). */
export function mapCsv(rows: string[][], extraDateAliases: string[] = []): { items: InputRow[]; error?: string; skipped: number } {
  if (rows.length < 2) return { items: [], error: "CSV vazio.", skipped: 0 };
  const head = rows[0].map((h) => h.trim().toLowerCase());
  const idx: Record<string, number> = {};
  for (const [k, names] of Object.entries(HEADER_ALIASES)) {
    const all = k === "new_date" ? [...names, ...extraDateAliases] : names;
    idx[k] = head.findIndex((h) => all.includes(h));
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

  function clearFile() {
    setItems([]);
    setFileName("");
    setSkipped(0);
    setPreview(null);
    setResults([]);
    setStopMsg("");
    setAck(false);
    setTyped("");
    setErr("");
  }

  return (
    <ToolPanel
      icon={<CalendarClock size={20} />}
      title="Correção em lote da data de venda"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Altera dados de produção na HubSoft. Cada serviço é conferido por login PPPoE, id, código e nome do cliente antes e depois da alteração."
    >
      <Step n={1} title="Exportar a base" done={false} hint="Gera um CSV com todos os serviços; preencha a coluna data_venda_nova (AAAA-MM-DD ou DD/MM/AAAA) e não altere login, id, código nem cliente.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={exporting} onClick={() => void doExport()}>
            <Download size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {exporting ? "Lendo a HubSoft…" : "Exportar CSV de todos os serviços"}
          </button>
          <label className="hsa-check">
            <input type="checkbox" checked={includeCancelled} onChange={(e) => setIncludeCancelled(e.target.checked)} />
            Incluir serviços cancelados
          </label>
        </div>
      </Step>

      <Step n={2} title="Enviar o CSV preenchido e pré-visualizar" done={!!preview} hint="Nada é alterado nesta etapa.">
        <CsvDropzone
          fileName={fileName}
          info={fileName ? `${items.length} linha(s) com nova data${skipped ? `, ${skipped} sem data (ignoradas)` : ""}` : undefined}
          disabled={previewing || applying}
          onFile={(f) => void onFile(f)}
          onClear={clearFile}
        />
        <div className="hsa-actions">
          <span className="hsa-spacer" />
          <button type="button" className="btn btn--primary" disabled={previewing || items.length === 0} onClick={() => void doPreview()}>
            <Eye size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {previewing ? "Conferindo com a HubSoft…" : "Pré-visualizar alterações"}
          </button>
        </div>
        {err ? <Callout tone="err">{err}</Callout> : null}
      </Step>

      {preview ? (
        <Step n={3} title="Pré-visualização" done={approved.length > 0 && preview.blocked === 0}>
          <div className="hsa-stats">
            <Stat label="Aprovadas" value={approved.length} tone="ok" />
            <Stat label="Bloqueadas" value={preview.blocked} tone={preview.blocked ? "err" : "muted"} />
            <Stat label="Serviços na HubSoft" value={preview.base_size} tone="muted" />
          </div>
          <div className="hsa-actions">
            <Segmented
              label="Filtrar linhas"
              value={filter}
              onChange={setFilter}
              options={[
                { value: "all", label: "Todas" },
                { value: "approved", label: "Aprovadas" },
                { value: "blocked", label: "Bloqueadas" },
              ]}
            />
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={downloadPlan}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar plano (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Login PPPoE</th>
                  <th>ID</th>
                  <th>Cliente</th>
                  <th>Status</th>
                  <th>Data atual</th>
                  <th>Nova data</th>
                  <th>Decisão</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((r) => (
                  <tr key={r.line}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td className="mono">{r.login}</td>
                    <td className="mono">{r.service_id || "—"}</td>
                    <td>{r.client_name || "—"}</td>
                    <td>{r.status || "—"}</td>
                    <td className="mono">{r.current_date || "—"}</td>
                    <td className="mono">{r.new_date || "—"}</td>
                    <td>
                      {r.approved ? <Pill tone="ok">Aprovada</Pill> : <span style={{ color: "var(--err)" }}>Bloqueada: {r.reason}</span>}
                      {(r.warnings ?? []).map((w) => (
                        <div key={w} className="hsa-muted" style={{ color: "var(--warn)" }}>
                          ⚠ {w}
                        </div>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Step>
      ) : null}

      {preview && approved.length > 0 ? (
        <Step n={4} title="Aplicar" done={results.length > 0 && !applying}>
          {overLimit ? (
            <Callout tone="err">
              São {approved.length} aprovadas; o máximo por execução é {APPLY_MAX}. Divida o CSV.
            </Callout>
          ) : (
            <>
              <label className="hsa-check">
                <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
                Revisei a pré-visualização e entendo que isto altera a data de venda de {approved.length} serviço(s) na HubSoft.
              </label>
              <div className="hsa-actions">
                <input className="input" style={{ width: 190 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
                <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void doApply()}>
                  {applying ? "Aplicando…" : `Aplicar ${approved.length} alteração(ões)`}
                </button>
                {applying ? (
                  <button
                    type="button"
                    className="btn"
                    onClick={() => {
                      stopRef.current = true;
                    }}
                  >
                    Parar
                  </button>
                ) : null}
              </div>
              <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão.</span>
              {applying ? <ProgressBar done={results.length} total={Math.min(approved.length, APPLY_MAX)} label={`Aplicando… ${results.length} de ${Math.min(approved.length, APPLY_MAX)}`} /> : null}
            </>
          )}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step n={5} title="Resultado" done={!applying && okCount === results.length}>
          <div className="hsa-stats">
            <Stat label="OK" value={okCount} tone="ok" />
            <Stat label="Com problema" value={results.length - okCount} tone={results.length - okCount ? "err" : "muted"} />
          </div>
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={downloadResults}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar resultado (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Login</th>
                  <th>ID</th>
                  <th>Resultado</th>
                  <th>Antes → Depois</th>
                </tr>
              </thead>
              <tbody>
                {results.map((r) => (
                  <tr key={`${r.line}-${r.service_id}`}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td className="mono">{r.login}</td>
                    <td className="mono">{r.service_id}</td>
                    <td style={{ color: r.ok ? "var(--ok)" : "var(--err)" }}>
                      {r.ok ? "OK" : r.halt ? "PAROU" : "Erro"} — {r.message}
                    </td>
                    <td className="mono">
                      {r.date_before || "?"} → {r.date_after || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Step>
      ) : stopMsg ? (
        <div className="hsa-panel__body--pad">
          <Callout tone="err">{stopMsg}</Callout>
        </div>
      ) : null}
    </ToolPanel>
  );
}
