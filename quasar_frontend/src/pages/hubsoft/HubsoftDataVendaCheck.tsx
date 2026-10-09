import { useMemo, useState } from "react";
import { CalendarCheck, Download, Play } from "lucide-react";
import { Callout, CsvDropzone, Pill, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { mapCsv, type InputRow, type PreviewResp, type PreviewRow } from "./HubsoftDataVendaSection";

const BASE = "/api/v1/integrations/hubsoft/hubsoft/data-venda";

/**
 * Conferência (SOMENTE LEITURA) da data da venda dos serviços: compara a data do CSV com a que está na HubSoft. Usa a mesma
 * consulta da pré-visualização da correção em lote, mas nunca aplica nada — a correção continua na aba «Data de venda».
 * Cada serviço é localizado por login PPPoE + id + código + nome do cliente (os quatro precisam bater).
 */

type Verdict = "igual" | "diferente" | "nao_conferido";
type Row = PreviewRow & { verdict: Verdict };
type Filter = "all" | Verdict;
const SAME_DATE = "data de venda já está correta";

function verdictOf(r: PreviewRow): Verdict {
  if (r.approved) return "diferente";
  return (r.reason ?? "").toLowerCase().includes(SAME_DATE) ? "igual" : "nao_conferido";
}
const LABEL: Record<Verdict, string> = { igual: "Data igual à HubSoft", diferente: "Data DIFERENTE", nao_conferido: "Não conferido" };
const TONE: Record<Verdict, "ok" | "warn" | "err"> = { igual: "ok", diferente: "warn", nao_conferido: "err" };

export function HubsoftDataVendaCheck() {
  const { notify, missing } = useConsultaToast();
  const [fileName, setFileName] = useState("");
  const [items, setItems] = useState<InputRow[]>([]);
  const [skipped, setSkipped] = useState(0);
  const [includeCancelled, setIncludeCancelled] = useState(false);
  const [running, setRunning] = useState(false);
  const [err, setErr] = useState("");
  const [rows, setRows] = useState<Row[]>([]);
  const [baseSize, setBaseSize] = useState(0);
  const [filter, setFilter] = useState<Filter>("all");

  function reset() {
    setFileName("");
    setItems([]);
    setSkipped(0);
    setRows([]);
    setErr("");
  }

  async function onFile(f: File | undefined) {
    reset();
    if (!f) return;
    setFileName(f.name);
    // aceita «data_venda» (e a coluna do CSV exportado pela correção em lote) como a data a conferir
    const m = mapCsv(parseCsv(await f.text()), ["data_venda"]);
    if (m.error) return missing(m.error);
    if (m.items.length === 0) return missing("nenhuma linha com a data da venda preenchida.");
    setItems(m.items);
    setSkipped(m.skipped);
  }

  async function run() {
    setRunning(true);
    setErr("");
    setRows([]);
    try {
      const r = await apiFetch<PreviewResp>(`${BASE}/preview`, { method: "POST", json: { rows: items, include_cancelled: includeCancelled }, timeoutMs: 8 * 60_000 });
      if (!r.ok) throw new Error(r.message || "Falha na conferência.");
      setRows(r.rows.map((x) => ({ ...x, verdict: verdictOf(x) })));
      setBaseSize(r.base_size);
      notify({ ok: true }, null, `Conferência concluída — ${r.rows.length} serviço(s).`);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  const counts = useMemo(() => {
    const c = { igual: 0, diferente: 0, nao_conferido: 0 };
    for (const r of rows) c[r.verdict]++;
    return c;
  }, [rows]);
  const shown = useMemo(() => (filter === "all" ? rows : rows.filter((r) => r.verdict === filter)), [rows, filter]);

  function download() {
    downloadCsv(
      `conferencia-data-venda-${todayISO()}.csv`,
      ["linha", "login", "id_cliente_servico", "cliente", "status", "data_na_hubsoft", "data_no_arquivo", "situacao", "motivo"],
      shown.map((r) => [String(r.line), r.login, r.service_id ?? "", r.client_name ?? "", r.status ?? "", r.current_date ?? "", r.new_date ?? "", LABEL[r.verdict], r.verdict === "igual" ? "" : (r.reason ?? "")]),
    );
  }

  return (
    <ToolPanel
      icon={<CalendarCheck size={20} />}
      title="Conferência da data de venda"
      badge="Somente leitura"
      badgeTone="ok"
      subtitle="Compara a data da venda do CSV com a que está na HubSoft, serviço a serviço. Para corrigir as diferentes use a aba «Data de venda»."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <Callout tone="info">
          O CSV precisa ter as colunas <b>login_pppoe</b>, <b>id_cliente_servico</b>, <b>cliente</b> e a data em <b>data_venda</b> (ou <b>data_venda_nova</b>), em AAAA-MM-DD ou DD/MM/AAAA.
          Cada serviço só é conferido se login, id e nome do cliente baterem com a HubSoft. Nada é alterado.
        </Callout>
      </div>
      <Step n={1} title="Enviar o arquivo" done={items.length > 0} hint="A conferência lê a base de serviços da HubSoft (pode levar alguns minutos).">
        <CsvDropzone fileName={fileName} info={items.length > 0 ? `${items.length} linha(s) com data${skipped ? ` — ${skipped} sem data ignorada(s)` : ""}` : undefined} disabled={running} onFile={(f) => void onFile(f)} onClear={reset} />
        <div className="hsa-actions">
          <label className="hsa-check">
            <input type="checkbox" checked={includeCancelled} onChange={(e) => setIncludeCancelled(e.target.checked)} disabled={running} />
            Incluir serviços cancelados
          </label>
          <span className="hsa-spacer" />
          <button type="button" className="btn btn--primary" disabled={running || items.length === 0} onClick={() => void run()}>
            <Play size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {running ? "Conferindo…" : "Conferir"}
          </button>
        </div>
        {running ? <ConsultaLoading text="Conferindo com a HubSoft…" /> : null}
        {err ? <Callout tone="err">{err}</Callout> : null}
      </Step>
      {rows.length > 0 ? (
        <Step n={2} title="Resultado da conferência" done={counts.diferente + counts.nao_conferido === 0}>
          <div className="hsa-stats">
            <Stat label="Datas iguais" value={counts.igual} tone="ok" />
            <Stat label="Datas diferentes" value={counts.diferente} tone={counts.diferente ? "warn" : "muted"} />
            <Stat label="Não conferidos" value={counts.nao_conferido} tone={counts.nao_conferido ? "err" : "muted"} />
            <Stat label="Serviços na HubSoft" value={baseSize.toLocaleString("pt-BR")} tone="muted" />
          </div>
          <div className="hsa-actions">
            <Segmented
              label="Filtrar"
              value={filter}
              onChange={setFilter}
              options={[
                { value: "all", label: "Todos" },
                { value: "igual", label: "Iguais" },
                { value: "diferente", label: "Diferentes" },
                { value: "nao_conferido", label: "Não conferidos" },
              ]}
            />
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={shown.length === 0} onClick={download}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap" style={{ maxHeight: 520 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Serviço</th>
                  <th>Data na HubSoft</th>
                  <th>Data no arquivo</th>
                  <th>Situação</th>
                </tr>
              </thead>
              <tbody>
                {shown.slice(0, 1000).map((r) => (
                  <tr key={r.line}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td>
                      <span className="mono">{r.login}</span>
                      <div className="hsa-muted">{r.client_name}{r.client_code ? ` (código ${r.client_code})` : ""}{r.status ? ` · ${r.status}` : ""}</div>
                    </td>
                    <td>{r.current_date || "—"}</td>
                    <td>{r.new_date || "—"}</td>
                    <td>
                      <Pill tone={TONE[r.verdict]}>{LABEL[r.verdict]}</Pill>
                      {r.verdict === "nao_conferido" && r.reason ? <div className="hsa-muted" style={{ marginTop: 4 }}>{r.reason}</div> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {shown.length > 1000 ? <span className="hsa-muted">Mostrando 1.000 de {shown.length}. Baixe o CSV para ver todos.</span> : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
