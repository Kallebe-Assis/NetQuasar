import { useMemo, useState } from "react";
import { Download, ExternalLink, Receipt, Search } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { downloadCsv } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { Callout, Pill, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Boletos EM ABERTO por forma de cobrança — SOMENTE LEITURA. A forma de cobrança fica gravada na própria fatura quando o
 * boleto é gerado; trocar a forma do cliente/serviço depois não altera os boletos já emitidos. Esta tela varre as faturas
 * em aberto da HubSoft e lista as que ainda carregam a forma escolhida (ex.: «Sicoob - API (G2)»), para o operador
 * analisar uma a uma na HubSoft (a API não tem rota para trocar a forma de uma fatura).
 */

const BASE = "/api/v1/integrations/hubsoft/hubsoft/report/invoices-by-method";
const DEFAULT_FORMA = "Sicoob - API (G2)";

type Row = {
  id_fatura: string;
  id_cliente?: string;
  codigo_cliente?: string;
  cliente: string;
  servico?: string;
  valor: string;
  vencimento: string;
  status: "pending" | "overdue";
  forma_id?: string;
  forma_nome?: string;
  nosso_numero?: string;
  linha_digitavel?: string;
  link?: string;
};
type Forma = { id?: string; nome: string; count: number; valor: number };
type Report = {
  ok: boolean;
  message?: string;
  from: string;
  to: string;
  match: string;
  scanned: number;
  total_registros: number;
  truncated?: boolean;
  forma_found: number;
  formas: Forma[];
  match_count: number;
  match_value: number;
  rows: Row[];
  campos_fatura?: string[];
};

const brl = (v: number) => v.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
const fmtInt = (n: number) => n.toLocaleString("pt-BR");

export function HubsoftInvoicesByMethod() {
  const [forma, setForma] = useState(DEFAULT_FORMA);
  const [from, setFrom] = useState(() => todayISO(-365 * 3));
  const [to, setTo] = useState(() => todayISO(120));
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  const [rep, setRep] = useState<Report | null>(null);
  const [onlyOverdue, setOnlyOverdue] = useState(false);

  async function run(f = forma) {
    if (from > to) {
      setErr("O período está invertido (a data inicial é maior que a final).");
      return;
    }
    setLoading(true);
    setErr("");
    setRep(null);
    try {
      const p = new URLSearchParams({ data_inicio: from, data_fim: to, forma: f.trim() });
      setRep(await apiFetch<Report>(`${BASE}?${p.toString()}`, { timeoutMs: 12 * 60_000 }));
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  const rows = useMemo(() => (rep?.rows ?? []).filter((r) => !onlyOverdue || r.status === "overdue"), [rep, onlyOverdue]);

  function download() {
    downloadCsv(
      `boletos-em-aberto-${(rep?.match || "forma").replace(/[^A-Za-z0-9]+/g, "-")}-${todayISO()}.csv`,
      ["id_fatura", "id_cliente", "codigo_cliente", "cliente", "login_servico", "vencimento", "valor", "situacao", "forma_cobranca_da_fatura", "nosso_numero", "linha_digitavel", "link_boleto"],
      rows.map((r) => [
        r.id_fatura, r.id_cliente ?? "", r.codigo_cliente ?? "", r.cliente, r.servico ?? "", r.vencimento, r.valor,
        r.status === "overdue" ? "vencido" : "a vencer", r.forma_nome ?? r.forma_id ?? "", r.nosso_numero ?? "", r.linha_digitavel ?? "", r.link ?? "",
      ]),
    );
  }

  return (
    <ToolPanel
      icon={<Receipt size={20} />}
      title="Boletos em aberto por forma de cobrança"
      badge="Somente leitura"
      badgeTone="ok"
      subtitle="Lê a forma de cobrança gravada em cada fatura (não a do cliente/serviço) e lista os boletos pendentes que ainda estão numa forma específica. A correção é feita na HubSoft, boleto a boleto."
    >
      <Step n={1} title="Escolher a forma e o período" done={!!rep && !loading} hint="Filtra pelo vencimento. Boleto em aberto pode ser antigo, por isso o período padrão cobre 3 anos para trás e 120 dias à frente.">
        <div className="hsa-actions">
          <label className="hsa-check">
            Forma de cobrança
            <input
              type="text"
              className="input"
              style={{ marginLeft: 8, width: 260 }}
              value={forma}
              placeholder="Nome (ou parte) ou id da forma"
              onChange={(e) => setForma(e.target.value)}
              disabled={loading}
            />
          </label>
          <label className="hsa-check">
            De
            <input type="date" className="input" style={{ marginLeft: 8 }} value={from} max={to} onChange={(e) => setFrom(e.target.value)} disabled={loading} />
          </label>
          <label className="hsa-check">
            Até
            <input type="date" className="input" style={{ marginLeft: 8 }} value={to} min={from} onChange={(e) => setTo(e.target.value)} disabled={loading} />
          </label>
          <button type="button" className="btn btn--primary" disabled={loading || !from || !to} onClick={() => void run()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {loading ? "Consultando a HubSoft…" : rep ? "Consultar de novo" : "Consultar boletos"}
          </button>
        </div>
        {loading ? <Callout tone="info">Lendo as faturas em aberto, 100 por vez. Pode levar alguns minutos — não feche a tela.</Callout> : null}
        {err ? <Callout tone="err">{err}</Callout> : null}
        {rep?.message ? <Callout tone={rep.forma_found === 0 && rep.scanned > 0 ? "err" : "warn"}>{rep.message}</Callout> : null}
        {rep ? (
          <div className="hsa-stats">
            <Stat label="Faturas em aberto lidas" value={rep.scanned} tone="muted" />
            <Stat label="Com forma de cobrança identificada" value={rep.forma_found} tone={rep.forma_found ? "ok" : "warn"} />
            <Stat label={`Boletos em «${rep.match || "—"}»`} value={rep.match_count} tone={rep.match_count ? "warn" : "ok"} />
            <Stat label="Valor desses boletos" value={brl(rep.match_value)} tone="muted" />
          </div>
        ) : null}
      </Step>

      {rep && rep.formas.length > 0 ? (
        <Step n={2} title="Formas de cobrança encontradas nos boletos em aberto" done={false} hint="Clique numa forma para consultar só ela. O filtro ignora acento, caixa e pontuação.">
          <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
            {rep.formas.map((f) => (
              <button
                key={`${f.id ?? ""}|${f.nome}`}
                type="button"
                className="btn btn--sm"
                disabled={loading}
                title={`${fmtInt(f.count)} boleto(s) · ${brl(f.valor)}`}
                onClick={() => {
                  const next = f.nome || f.id || "";
                  setForma(next);
                  void run(next);
                }}
              >
                {f.nome || `id ${f.id}`} · {fmtInt(f.count)}
              </button>
            ))}
          </div>
        </Step>
      ) : null}

      {rep && rep.forma_found === 0 && rep.campos_fatura?.length ? (
        <Step n={2} title="Campos que a fatura traz" done={false} hint="Sem o campo da forma de cobrança não há como filtrar. Envie esta lista para ajustar a leitura.">
          <div className="mono" style={{ fontSize: 11, wordBreak: "break-word" }}>{rep.campos_fatura.join(", ")}</div>
        </Step>
      ) : null}

      {rep && rep.match ? (
        <Step n={3} title="Boletos para analisar na HubSoft" done={false}>
          <div className="hsa-actions">
            <label className="hsa-check">
              <input type="checkbox" checked={onlyOverdue} onChange={(e) => setOnlyOverdue(e.target.checked)} />
              Só vencidos
            </label>
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={rows.length === 0} onClick={download}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar lista (CSV)
            </button>
          </div>
          {rows.length === 0 ? (
            <Callout tone="info">Nenhum boleto em aberto nessa forma de cobrança{onlyOverdue ? " (vencido)" : ""}.</Callout>
          ) : (
            <div className="hsa-table-wrap" style={{ maxHeight: 560 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <th>Cliente</th>
                    <th>Serviço</th>
                    <th>Fatura</th>
                    <th>Vencimento</th>
                    <th>Valor</th>
                    <th>Situação</th>
                    <th>Boleto</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.id_fatura}>
                      <td>
                        {r.cliente || "—"}
                        <div className="hsa-muted">
                          {r.id_cliente ? `id_cliente ${r.id_cliente}` : ""}
                          {r.codigo_cliente ? ` (código ${r.codigo_cliente})` : ""}
                        </div>
                      </td>
                      <td className="mono">{r.servico || "—"}</td>
                      <td className="mono">
                        {r.id_fatura}
                        {r.nosso_numero ? <div className="hsa-muted">nosso nº {r.nosso_numero}</div> : null}
                      </td>
                      <td>{r.vencimento || "—"}</td>
                      <td>{r.valor || "—"}</td>
                      <td>
                        <Pill tone={r.status === "overdue" ? "err" : "ok"}>{r.status === "overdue" ? "Vencido" : "A vencer"}</Pill>
                      </td>
                      <td>
                        {r.link ? (
                          <a href={r.link} target="_blank" rel="noreferrer noopener" title="Abrir o boleto">
                            <ExternalLink size={14} aria-hidden /> abrir
                          </a>
                        ) : (
                          "—"
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
