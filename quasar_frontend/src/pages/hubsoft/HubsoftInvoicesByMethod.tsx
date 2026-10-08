import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowUp, ArrowUpDown, Download, ExternalLink, Receipt, Search } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { downloadCsv } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { Callout, Pill, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Boletos EM ABERTO por forma de cobrança — SOMENTE LEITURA. A forma de cobrança fica gravada na própria fatura quando o
 * boleto é gerado; trocar a forma do cliente/serviço depois não altera os boletos já emitidos. Esta tela varre as faturas
 * em aberto da HubSoft e lista as que ainda carregam a forma escolhida (ex.: «Sicoob - API (G2)»), para o operador
 * analisar uma a uma na HubSoft (a API não tem rota para trocar a forma de uma fatura).
 *
 * A consulta à HubSoft é pesada (lê todas as faturas do período); depois dela, filtros, ordenação e agrupamento por
 * cliente são feitos aqui no navegador, sem nova chamada.
 */

const BASE = "/api/v1/integrations/hubsoft/hubsoft/report/invoices-by-method";
const DEFAULT_FORMA_HINT = "sicoobapig2"; // pré-seleciona «Sicoob - API (G2)» quando ela existir na conta

type Row = {
  id_fatura: string;
  id_cliente?: string;
  codigo_cliente?: string;
  cliente: string;
  servico?: string;
  plano?: string;
  id_cliente_servico?: string;
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
type FormaOption = { id: string; nome: string; tipo?: string };
type ServiceRow = {
  id_cliente: string;
  cliente?: string;
  id_cliente_servico: string;
  login?: string;
  plano?: string;
  status?: string;
  forma_id?: string;
  forma_nome?: string;
  resultado: "ok" | "diferente" | "sem_dado";
};
type ServiceCheck = {
  ok: boolean;
  message?: string;
  esperada: string;
  clientes: number;
  servicos: number;
  servicos_ok: number;
  servicos_diferentes: number;
  servicos_sem_dado: number;
  clientes_todos_ok: number;
  clientes_com_diferenca?: string[];
  nao_encontrados?: string[];
  erros?: string[];
  rows: ServiceRow[];
  campos_servico?: Record<string, string>;
};
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
  amostra_fatura?: Record<string, string>;
};

// ---- helpers de formatação / conversão -----------------------------------------------------------------------

const brl = (v: number) => v.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
const fmtInt = (n: number) => n.toLocaleString("pt-BR");
const alnum = (s: string) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/[^a-z0-9]/g, "");

/** "2026-12-15" ou "15/12/2026" → "2026-12-15" (chave comparável); vazio se não reconhecer. */
function toIso(d: string): string {
  const iso = /^(\d{4})-(\d{2})-(\d{2})/.exec(d);
  if (iso) return `${iso[1]}-${iso[2]}-${iso[3]}`;
  const br = /^(\d{2})\/(\d{2})\/(\d{4})/.exec(d);
  return br ? `${br[3]}-${br[2]}-${br[1]}` : "";
}
/** Data para exibição: sempre DD/MM/AAAA (a API já manda assim; isto só protege contra ISO). */
function fmtDate(d: string): string {
  const iso = toIso(d);
  return iso ? `${iso.slice(8, 10)}/${iso.slice(5, 7)}/${iso.slice(0, 4)}` : d || "—";
}
/** "1.234,56" ou "89.90" → número. */
function toNum(v: string): number {
  const s = v.trim();
  if (s.includes(",")) return Number(s.replace(/\./g, "").replace(",", ".")) || 0;
  return Number(s) || 0;
}
const fmtMoney = (v: string) => brl(toNum(v));

// ---- agrupamento por cliente ---------------------------------------------------------------------------------

type ClientGroup = {
  key: string;
  id_cliente?: string;
  codigo_cliente?: string;
  cliente: string;
  logins: string[];
  count: number;
  overdue: number;
  total: number;
  oldest: string; // ISO do vencimento mais antigo
};

function groupByClient(rows: Row[]): ClientGroup[] {
  const map = new Map<string, ClientGroup>();
  for (const r of rows) {
    const key = r.id_cliente || r.codigo_cliente || r.cliente;
    let g = map.get(key);
    if (!g) {
      g = { key, id_cliente: r.id_cliente, codigo_cliente: r.codigo_cliente, cliente: r.cliente, logins: [], count: 0, overdue: 0, total: 0, oldest: "" };
      map.set(key, g);
    }
    g.count++;
    if (r.status === "overdue") g.overdue++;
    g.total += toNum(r.valor);
    const login = r.servico || r.plano;
    if (login && !g.logins.includes(login)) g.logins.push(login);
    const iso = toIso(r.vencimento);
    if (iso && (!g.oldest || iso < g.oldest)) g.oldest = iso;
  }
  return [...map.values()];
}

// ---- ordenação -----------------------------------------------------------------------------------------------

type SortKey = "cliente" | "servico" | "fatura" | "vencimento" | "valor" | "situacao" | "boletos";
type Sort = { key: SortKey; dir: "asc" | "desc" };
const cmpText = (a: string, b: string) => a.localeCompare(b, "pt-BR", { sensitivity: "base", numeric: true });

function SortTh({ label, k, sort, onSort }: { label: string; k: SortKey; sort: Sort; onSort: (k: SortKey) => void }) {
  const active = sort.key === k;
  const Icon = !active ? ArrowUpDown : sort.dir === "asc" ? ArrowUp : ArrowDown;
  return (
    <th aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}>
      <button type="button" style={{ border: 0, background: "transparent", padding: 0, font: "inherit", fontWeight: 600, color: "inherit", cursor: "pointer" }} onClick={() => onSort(k)}>
        {label} <Icon size={12} aria-hidden style={{ verticalAlign: -1, opacity: active ? 1 : 0.45 }} />
      </button>
    </th>
  );
}

export function HubsoftInvoicesByMethod() {
  // consulta à HubSoft
  const [formas, setFormas] = useState<FormaOption[]>([]);
  const [formasErr, setFormasErr] = useState("");
  const [forma, setForma] = useState(""); // id da forma (ou nome, se a conta não trouxer id)
  const [from, setFrom] = useState(() => todayISO(-365 * 3));
  const [to, setTo] = useState(() => todayISO(120));
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  const [rep, setRep] = useState<Report | null>(null);
  const [repForma, setRepForma] = useState("");

  // conferência da forma de cobrança ATUAL dos serviços desses clientes
  const [svcForma, setSvcForma] = useState("");
  const [svcLoading, setSvcLoading] = useState(false);
  const [svcErr, setSvcErr] = useState("");
  const [svc, setSvc] = useState<ServiceCheck | null>(null);

  // filtros e visão (no navegador)
  const [situacao, setSituacao] = useState<"" | "overdue" | "pending">("");
  const [vFrom, setVFrom] = useState("");
  const [vTo, setVTo] = useState("");
  const [valMin, setValMin] = useState("");
  const [valMax, setValMax] = useState("");
  const [busca, setBusca] = useState("");
  const [byClient, setByClient] = useState(true);
  const [sort, setSort] = useState<Sort>({ key: "cliente", dir: "asc" });

  // lista de formas de cobrança cadastradas na HubSoft
  useEffect(() => {
    let alive = true;
    apiFetch<{ formas: FormaOption[] }>(`${BASE}/formas`, { timeoutMs: 60_000 })
      .then((d) => {
        if (!alive) return;
        const list = d.formas ?? [];
        setFormas(list);
        const pre = list.find((f) => alnum(f.nome).includes(DEFAULT_FORMA_HINT));
        if (pre) setForma(pre.id || pre.nome);
        const bb = list.find((f) => /^(bancodobrasil|bb)/.test(alnum(f.nome)) && alnum(f.nome).includes("g2"));
        if (bb) setSvcForma(bb.id || bb.nome);
      })
      .catch((e) => alive && setFormasErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, []);

  const formaNome = (v: string) => formas.find((f) => (f.id || f.nome) === v)?.nome ?? v;

  async function run(f = forma) {
    if (!f) {
      setErr("Escolha a forma de cobrança.");
      return;
    }
    if (from > to) {
      setErr("O período está invertido (a data inicial é maior que a final).");
      return;
    }
    setLoading(true);
    setErr("");
    setRep(null);
    try {
      const p = new URLSearchParams({ data_inicio: from, data_fim: to, forma: f });
      setRep(await apiFetch<Report>(`${BASE}?${p.toString()}`, { timeoutMs: 12 * 60_000 }));
      setRepForma(f);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function checkServices() {
    const ids = [...new Set((rep?.rows ?? []).map((r) => r.id_cliente).filter((x): x is string => !!x))];
    if (!svcForma || ids.length === 0) return;
    setSvcLoading(true);
    setSvcErr("");
    setSvc(null);
    try {
      setSvc(await apiFetch<ServiceCheck>(`${BASE}/services-check`, { method: "POST", body: JSON.stringify({ ids, forma: svcForma }), timeoutMs: 5 * 60_000 }));
    } catch (e) {
      setSvcErr((e as Error).message);
    } finally {
      setSvcLoading(false);
    }
  }

  function downloadServices() {
    if (!svc) return;
    downloadCsv(
      `forma-atual-dos-servicos-${todayISO()}.csv`,
      ["id_cliente", "cliente", "id_cliente_servico", "login", "plano", "status_servico", "forma_cobranca_atual", "resultado"],
      svc.rows.map((r) => [r.id_cliente, r.cliente ?? "", r.id_cliente_servico, r.login ?? "", r.plano ?? "", r.status ?? "", r.forma_nome ?? r.forma_id ?? "", r.resultado]),
    );
  }

  function toggleSort(k: SortKey) {
    setSort((s) => (s.key === k ? { key: k, dir: s.dir === "asc" ? "desc" : "asc" } : { key: k, dir: k === "valor" || k === "boletos" ? "desc" : "asc" }));
  }

  // filtros
  const rows = useMemo(() => {
    const q = alnum(busca);
    const mn = valMin.trim() ? toNum(valMin) : null;
    const mx = valMax.trim() ? toNum(valMax) : null;
    return (rep?.rows ?? []).filter((r) => {
      if (situacao && r.status !== situacao) return false;
      const iso = toIso(r.vencimento);
      if (vFrom && (!iso || iso < vFrom)) return false;
      if (vTo && (!iso || iso > vTo)) return false;
      const v = toNum(r.valor);
      if (mn !== null && v < mn) return false;
      if (mx !== null && v > mx) return false;
      if (q && !alnum(`${r.cliente} ${r.servico ?? ""} ${r.codigo_cliente ?? ""} ${r.id_cliente ?? ""}`).includes(q)) return false;
      return true;
    });
  }, [rep, situacao, vFrom, vTo, valMin, valMax, busca]);

  const sortedRows = useMemo(() => {
    const k = sort.key;
    const val = (r: Row): string | number => {
      switch (k) {
        case "servico": return r.servico || r.plano || "";
        case "fatura": return Number(r.id_fatura) || 0;
        case "vencimento": return toIso(r.vencimento);
        case "valor": return toNum(r.valor);
        case "situacao": return r.status;
        default: return r.cliente;
      }
    };
    const out = [...rows].sort((a, b) => {
      const x = val(a);
      const y = val(b);
      return typeof x === "number" && typeof y === "number" ? x - y : cmpText(String(x), String(y));
    });
    return sort.dir === "desc" ? out.reverse() : out;
  }, [rows, sort]);

  const groups = useMemo(() => {
    const k = sort.key;
    const out = groupByClient(rows).sort((a, b) => {
      switch (k) {
        case "boletos": return a.count - b.count;
        case "valor": return a.total - b.total;
        case "vencimento": return cmpText(a.oldest, b.oldest);
        case "servico": return cmpText(a.logins.join(","), b.logins.join(","));
        default: return cmpText(a.cliente, b.cliente);
      }
    });
    return sort.dir === "desc" ? out.reverse() : out;
  }, [rows, sort]);

  const filtered = !!(situacao || vFrom || vTo || valMin.trim() || valMax.trim() || busca.trim());
  const rowsTotal = rows.reduce((s, r) => s + toNum(r.valor), 0);
  const slug = alnum(formaNome(repForma)) || "forma";

  function downloadClients() {
    downloadCsv(
      `clientes-com-boleto-${slug}-${todayISO()}.csv`,
      ["id_cliente", "codigo_cliente", "cliente", "logins", "boletos_em_aberto", "boletos_vencidos", "valor_total", "vencimento_mais_antigo"],
      groups.map((g) => [g.id_cliente ?? "", g.codigo_cliente ?? "", g.cliente, g.logins.join(" | "), String(g.count), String(g.overdue), g.total.toFixed(2).replace(".", ","), fmtDate(g.oldest)]),
    );
  }
  function downloadBoletos() {
    downloadCsv(
      `boletos-em-aberto-${slug}-${todayISO()}.csv`,
      ["id_fatura", "id_cliente", "codigo_cliente", "cliente", "login_servico", "plano", "id_cliente_servico", "vencimento", "valor", "situacao", "forma_cobranca_da_fatura", "nosso_numero", "linha_digitavel", "link_boleto"],
      sortedRows.map((r) => [
        r.id_fatura, r.id_cliente ?? "", r.codigo_cliente ?? "", r.cliente, r.servico ?? "", r.plano ?? "", r.id_cliente_servico ?? "", fmtDate(r.vencimento), toNum(r.valor).toFixed(2).replace(".", ","),
        r.status === "overdue" ? "vencido" : "a vencer", r.forma_nome ?? r.forma_id ?? "", r.nosso_numero ?? "", r.linha_digitavel ?? "", r.link ?? "",
      ]),
    );
  }
  function clearFilters() {
    setSituacao("");
    setVFrom("");
    setVTo("");
    setValMin("");
    setValMax("");
    setBusca("");
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
            <select className="input" style={{ marginLeft: 8, minWidth: 260 }} value={forma} onChange={(e) => setForma(e.target.value)} disabled={loading || formas.length === 0}>
              <option value="">{formas.length === 0 && !formasErr ? "Carregando formas da HubSoft…" : "Selecione…"}</option>
              {formas.map((f) => (
                <option key={f.id || f.nome} value={f.id || f.nome}>
                  {f.nome}
                </option>
              ))}
            </select>
          </label>
          <label className="hsa-check">
            Vencimento de
            <input type="date" className="input" style={{ marginLeft: 8 }} value={from} max={to} onChange={(e) => setFrom(e.target.value)} disabled={loading} />
          </label>
          <label className="hsa-check">
            até
            <input type="date" className="input" style={{ marginLeft: 8 }} value={to} min={from} onChange={(e) => setTo(e.target.value)} disabled={loading} />
          </label>
          <button type="button" className="btn btn--primary" disabled={loading || !forma || !from || !to} onClick={() => void run()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {loading ? "Consultando a HubSoft…" : rep ? "Consultar de novo" : "Consultar boletos"}
          </button>
        </div>
        {formasErr ? <Callout tone="err">Não foi possível carregar as formas de cobrança da HubSoft: {formasErr}</Callout> : null}
        {loading ? <Callout tone="info">Lendo as faturas em aberto, 100 por vez. Pode levar alguns minutos — não feche a tela.</Callout> : null}
        {err ? <Callout tone="err">{err}</Callout> : null}
        {rep?.message ? <Callout tone={rep.forma_found === 0 && rep.scanned > 0 ? "err" : "warn"}>{rep.message}</Callout> : null}
        {rep ? (
          <div className="hsa-stats">
            <Stat label="Faturas em aberto lidas" value={fmtInt(rep.scanned)} tone="muted" />
            <Stat label="Com forma de cobrança identificada" value={fmtInt(rep.forma_found)} tone={rep.forma_found ? "ok" : "warn"} />
            <Stat label={`Boletos em «${formaNome(repForma)}»`} value={fmtInt(rep.match_count)} tone={rep.match_count ? "warn" : "ok"} />
            <Stat label="Valor desses boletos" value={brl(rep.match_value)} tone="muted" />
          </div>
        ) : null}
      </Step>

      {rep && rep.formas.length > 0 ? (
        <Step n={2} title="Formas de cobrança encontradas nos boletos em aberto" done={false} hint="Quantos boletos em aberto existem em cada forma. Clique numa para consultar só ela.">
          <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
            {rep.formas.map((f) => (
              <button
                key={`${f.id ?? ""}|${f.nome}`}
                type="button"
                className="btn btn--sm"
                disabled={loading}
                title={`${fmtInt(f.count)} boleto(s) · ${brl(f.valor)}`}
                onClick={() => {
                  const next = f.id || f.nome;
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

      {rep && rep.amostra_fatura ? (
        <details style={{ margin: "4px 0 10px" }}>
          <summary style={{ cursor: "pointer", fontSize: 12 }}>Campos que a HubSoft devolve em cada fatura (diagnóstico)</summary>
          <div className="hsa-table-wrap" style={{ maxHeight: 320, marginTop: 6 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Campo</th>
                  <th>Valor na 1ª fatura lida</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(rep.amostra_fatura)
                  .sort(([a], [b]) => a.localeCompare(b))
                  .map(([k, v]) => (
                    <tr key={k}>
                      <td className="mono">{k}</td>
                      <td className="mono" style={{ wordBreak: "break-all" }}>{v || "—"}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </details>
      ) : null}

      {rep && rep.match ? (
        <Step n={3} title={`Boletos para analisar na HubSoft — ${fmtInt(groups.length)} cliente(s), ${fmtInt(rows.length)} boleto(s), ${brl(rowsTotal)}`} done={false}>
          <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
            <label className="hsa-check">
              Situação
              <select className="input" style={{ marginLeft: 8 }} value={situacao} onChange={(e) => setSituacao(e.target.value as typeof situacao)}>
                <option value="">Todas</option>
                <option value="overdue">Vencido</option>
                <option value="pending">A vencer</option>
              </select>
            </label>
            <label className="hsa-check">
              Vencimento de
              <input type="date" className="input" style={{ marginLeft: 8 }} value={vFrom} max={vTo || undefined} onChange={(e) => setVFrom(e.target.value)} />
            </label>
            <label className="hsa-check">
              até
              <input type="date" className="input" style={{ marginLeft: 8 }} value={vTo} min={vFrom || undefined} onChange={(e) => setVTo(e.target.value)} />
            </label>
            <label className="hsa-check">
              Valor de
              <input type="text" inputMode="decimal" className="input" style={{ marginLeft: 8, width: 80 }} placeholder="0,00" value={valMin} onChange={(e) => setValMin(e.target.value)} />
            </label>
            <label className="hsa-check">
              até
              <input type="text" inputMode="decimal" className="input" style={{ marginLeft: 8, width: 80 }} placeholder="999,00" value={valMax} onChange={(e) => setValMax(e.target.value)} />
            </label>
            <label className="hsa-check">
              Cliente / login
              <input type="search" className="input" style={{ marginLeft: 8, width: 180 }} placeholder="nome, login ou código" value={busca} onChange={(e) => setBusca(e.target.value)} />
            </label>
            {filtered ? (
              <button type="button" className="btn btn--sm" onClick={clearFilters}>
                Limpar filtros
              </button>
            ) : null}
          </div>
          <div className="hsa-actions">
            <label className="hsa-check">
              <input type="checkbox" checked={byClient} onChange={(e) => setByClient(e.target.checked)} />
              Agrupar por cliente
            </label>
            <span className="hsa-muted">Clique no título de uma coluna para ordenar.</span>
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={rows.length === 0} onClick={byClient ? downloadClients : downloadBoletos}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              {byClient ? "Baixar clientes (CSV)" : "Baixar boletos (CSV)"}
            </button>
          </div>

          {rows.length === 0 ? (
            <Callout tone="info">{filtered ? "Nenhum boleto com esses filtros." : "Nenhum boleto em aberto nessa forma de cobrança."}</Callout>
          ) : byClient ? (
            <div className="hsa-table-wrap" style={{ maxHeight: 560 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <SortTh label="Cliente" k="cliente" sort={sort} onSort={toggleSort} />
                    <SortTh label="Login(s)" k="servico" sort={sort} onSort={toggleSort} />
                    <SortTh label="Boletos em aberto" k="boletos" sort={sort} onSort={toggleSort} />
                    <SortTh label="Valor" k="valor" sort={sort} onSort={toggleSort} />
                    <SortTh label="Vencimento mais antigo" k="vencimento" sort={sort} onSort={toggleSort} />
                  </tr>
                </thead>
                <tbody>
                  {groups.map((g) => (
                    <tr key={g.key}>
                      <td>
                        {g.cliente || "—"}
                        <div className="hsa-muted">
                          {g.id_cliente ? `id_cliente ${g.id_cliente}` : ""}
                          {g.codigo_cliente ? ` (código ${g.codigo_cliente})` : ""}
                        </div>
                      </td>
                      <td className="mono">{g.logins.join(", ") || "—"}</td>
                      <td>
                        {fmtInt(g.count)}
                        {g.overdue ? <div className="hsa-muted">{fmtInt(g.overdue)} vencido(s)</div> : null}
                      </td>
                      <td>{brl(g.total)}</td>
                      <td>{fmtDate(g.oldest)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="hsa-table-wrap" style={{ maxHeight: 560 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <SortTh label="Cliente" k="cliente" sort={sort} onSort={toggleSort} />
                    <SortTh label="Serviço" k="servico" sort={sort} onSort={toggleSort} />
                    <SortTh label="Fatura" k="fatura" sort={sort} onSort={toggleSort} />
                    <SortTh label="Vencimento" k="vencimento" sort={sort} onSort={toggleSort} />
                    <SortTh label="Valor" k="valor" sort={sort} onSort={toggleSort} />
                    <SortTh label="Situação" k="situacao" sort={sort} onSort={toggleSort} />
                    <th>Boleto</th>
                  </tr>
                </thead>
                <tbody>
                  {sortedRows.map((r) => (
                    <tr key={r.id_fatura}>
                      <td>
                        {r.cliente || "—"}
                        <div className="hsa-muted">
                          {r.id_cliente ? `id_cliente ${r.id_cliente}` : ""}
                          {r.codigo_cliente ? ` (código ${r.codigo_cliente})` : ""}
                        </div>
                      </td>
                      <td>
                        <span className="mono">{r.servico || "—"}</span>
                        {r.plano ? <div className="hsa-muted">{r.plano}</div> : null}
                      </td>
                      <td className="mono">
                        {r.id_fatura}
                        {r.nosso_numero ? <div className="hsa-muted">nosso nº {r.nosso_numero}</div> : null}
                      </td>
                      <td>{fmtDate(r.vencimento)}</td>
                      <td>{fmtMoney(r.valor)}</td>
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
      {rep && rep.match && rep.rows.length > 0 ? (
        <Step n={4} title="Forma de cobrança atual dos serviços desses clientes" done={!!svc && !svcLoading} hint="Lê os serviços de cada cliente da lista na HubSoft (somente leitura) e compara com a forma escolhida. Serviços cancelados não entram na conta.">
          <div className="hsa-actions">
            <label className="hsa-check">
              Forma esperada
              <select className="input" style={{ marginLeft: 8, minWidth: 260 }} value={svcForma} onChange={(e) => setSvcForma(e.target.value)} disabled={svcLoading || formas.length === 0}>
                <option value="">Selecione…</option>
                {formas.map((f) => (
                  <option key={f.id || f.nome} value={f.id || f.nome}>
                    {f.nome}
                  </option>
                ))}
              </select>
            </label>
            <button type="button" className="btn btn--primary" disabled={svcLoading || !svcForma} onClick={() => void checkServices()}>
              {svcLoading ? "Conferindo os serviços…" : svc ? "Conferir de novo" : "Conferir serviços"}
            </button>
            {svc ? (
              <button type="button" className="btn btn--sm" disabled={svc.rows.length === 0} onClick={downloadServices}>
                <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Baixar (CSV)
              </button>
            ) : null}
          </div>
          {svcErr ? <Callout tone="err">{svcErr}</Callout> : null}
          {svc?.message ? <Callout tone="err">{svc.message}</Callout> : null}
          {svc?.erros?.length ? <Callout tone="warn">Falha ao consultar {svc.erros.length} cliente(s): {svc.erros.slice(0, 3).join(" · ")}</Callout> : null}
          {svc?.nao_encontrados?.length ? <Callout tone="warn">Cliente(s) não encontrado(s) na HubSoft: {svc.nao_encontrados.join(", ")}</Callout> : null}
          {svc ? (
            <>
              <div className="hsa-stats">
                <Stat label="Clientes conferidos" value={fmtInt(svc.clientes)} tone="muted" />
                <Stat label="Clientes com TODOS os serviços na forma esperada" value={fmtInt(svc.clientes_todos_ok)} tone="ok" />
                <Stat label="Clientes com algum serviço fora" value={fmtInt(svc.clientes_com_diferenca?.length ?? 0)} tone={svc.clientes_com_diferenca?.length ? "err" : "ok"} />
                <Stat label="Serviços na forma esperada" value={`${fmtInt(svc.servicos_ok)} de ${fmtInt(svc.servicos)}`} tone="muted" />
                <Stat label="Serviços em outra forma" value={fmtInt(svc.servicos_diferentes)} tone={svc.servicos_diferentes ? "err" : "ok"} />
                <Stat label="Serviços sem dado de forma" value={fmtInt(svc.servicos_sem_dado)} tone={svc.servicos_sem_dado ? "warn" : "ok"} />
              </div>
              {svc.servicos_sem_dado > 0 && svc.campos_servico ? (
                <details style={{ margin: "4px 0 10px" }}>
                  <summary style={{ cursor: "pointer", fontSize: 12 }}>Campos que a HubSoft devolve em cada serviço (diagnóstico)</summary>
                  <div className="hsa-table-wrap" style={{ maxHeight: 280, marginTop: 6 }}>
                    <table className="hsa-table">
                      <tbody>
                        {Object.entries(svc.campos_servico)
                          .sort(([a], [b]) => a.localeCompare(b))
                          .map(([k, v]) => (
                            <tr key={k}>
                              <td className="mono">{k}</td>
                              <td className="mono" style={{ wordBreak: "break-all" }}>{v || "—"}</td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                </details>
              ) : null}
              <div className="hsa-table-wrap" style={{ maxHeight: 420 }}>
                <table className="hsa-table">
                  <thead>
                    <tr>
                      <th>Cliente</th>
                      <th>Serviço</th>
                      <th>Status</th>
                      <th>Forma de cobrança atual</th>
                      <th>Resultado</th>
                    </tr>
                  </thead>
                  <tbody>
                    {svc.rows.map((r) => (
                      <tr key={`${r.id_cliente}-${r.id_cliente_servico}`}>
                        <td>
                          {r.cliente || "—"}
                          <div className="hsa-muted">id_cliente {r.id_cliente}</div>
                        </td>
                        <td>
                          <span className="mono">{r.login || "—"}</span>
                          {r.plano ? <div className="hsa-muted">{r.plano}</div> : null}
                        </td>
                        <td>{r.status || "—"}</td>
                        <td>{r.forma_nome || (r.forma_id ? `id ${r.forma_id}` : "—")}</td>
                        <td>
                          <Pill tone={r.resultado === "ok" ? "ok" : r.resultado === "diferente" ? "err" : "warn"}>
                            {r.resultado === "ok" ? "Na forma esperada" : r.resultado === "diferente" ? "Outra forma" : "Sem dado"}
                          </Pill>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          ) : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
