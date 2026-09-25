import { useMemo, useRef, useState } from "react";
import { Download, Square } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { fmtConsultaAt, useConsultaToast } from "./hubsoftConsulta";

const SLUG = "hubsoft";
const BASE = `/api/v1/integrations/${SLUG}/hubsoft/report/preventive`;
const CHUNK = 40;

type SvcRef = { id: string; login?: string; plan?: string; status?: string; city?: string };
type ClientRef = { id: string; code?: string; name?: string; services: SvcRef[] };
type BaseResp = { ok: boolean; message?: string; clients: ClientRef[]; services: number; truncated?: boolean };
type Count = { service_id: string; preventive: number; last_at?: string; dates?: string[] };
type ChunkResp = { ok: boolean; message?: string; items: Count[]; failed?: string[] };

type Result = {
  base: BaseResp;
  counts: Map<string, Count>;
  failed: number;
  stopped: boolean;
  at: number;
};

// Mantém o último resultado ao trocar de aba (só o botão Consultar chama a API).
let LAST: Result | null = null;

const fmtInt = (n: number) => n.toLocaleString("pt-BR");
function fmtBR(iso?: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso ?? "");
  return m ? `${m[3]}/${m[2]}/${m[1]}` : "—";
}
function csvEsc(v: string): string {
  return /[";,\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

type Row = { client: string; code: string; svc: SvcRef; count: number; last: string };

const selectStyle = { fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3, minWidth: 0 } as const;

/**
 * Aba Relatório → Desbloqueio preventivo. O log de status do painel da HubSoft não existe na API; o que
 * existe é o "desbloqueio em confiança" de cada serviço (observação "DESBLOQUEIO PREVENTIVO - ...").
 * Como só vem por cliente, a consulta percorre os clientes em lotes, com progresso e botão de parar.
 * Os filtros (período, status, plano, cidade) são aplicados na tela sobre o resultado já coletado.
 */
export function HubsoftPreventiveSection() {
  const { notify } = useConsultaToast();
  const [res, setRes] = useState<Result | null>(LAST);
  const [phase, setPhase] = useState<"idle" | "base" | "counting">("idle");
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [search, setSearch] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [fStatus, setFStatus] = useState("");
  const [fPlan, setFPlan] = useState("");
  const [fCity, setFCity] = useState("");
  const stopRef = useRef(false);

  async function run() {
    stopRef.current = false;
    setPhase("base");
    setProgress({ done: 0, total: 0 });
    let base: BaseResp;
    try {
      base = await apiFetch<BaseResp>(`${BASE}/base`, { timeoutMs: 6 * 60_000 });
    } catch (e) {
      notify(null, e);
      setPhase("idle");
      return;
    }
    if (!notify(base, null, "Lista de clientes carregada — contando os desbloqueios…")) {
      setPhase("idle");
      return;
    }
    const counts = new Map<string, Count>();
    let failed = 0;
    let stopped = false;
    setPhase("counting");
    setProgress({ done: 0, total: base.clients.length });
    try {
      for (let i = 0; i < base.clients.length; i += CHUNK) {
        if (stopRef.current) {
          stopped = true;
          break;
        }
        const ids = base.clients.slice(i, i + CHUNK).map((c) => c.id);
        const r = await apiFetch<ChunkResp>(`${BASE}/chunk`, { method: "POST", json: { client_ids: ids }, timeoutMs: 3 * 60_000 });
        if (!r.ok) throw new Error(r.message || "Falha ao consultar um lote de clientes.");
        for (const it of r.items) counts.set(it.service_id, it);
        failed += r.failed?.length ?? 0;
        setProgress({ done: Math.min(i + CHUNK, base.clients.length), total: base.clients.length });
      }
    } catch (e) {
      notify(null, e);
      const partial: Result = { base, counts, failed, stopped: true, at: Date.now() };
      LAST = partial;
      setRes(partial);
      setPhase("idle");
      return;
    }
    const done: Result = { base, counts, failed, stopped, at: Date.now() };
    LAST = done;
    setRes(done);
    setPhase("idle");
    if (stopped) notify(null, new Error("Consulta interrompida — mostrando o que foi coletado até aqui."));
    else notify({ ok: true }, null);
  }

  // Serviços com pelo menos 1 desbloqueio preventivo dentro do período (se informado). Ainda sem os
  // filtros de status/plano/cidade — eles é que alimentam as opções dos seletores.
  const dated = useMemo<Row[]>(() => {
    if (!res) return [];
    const rows: Row[] = [];
    for (const cl of res.base.clients) {
      for (const sv of cl.services) {
        const c = res.counts.get(sv.id);
        if (!c || c.preventive <= 0) continue;
        let dates = c.dates ?? [];
        // Sem data por ocorrência (resposta antiga) só dá para filtrar por período pelo "último".
        if (dates.length === 0 && c.last_at) dates = [c.last_at];
        const inRange = dates.filter((d) => (!from || d >= from) && (!to || d <= to));
        const count = from || to ? inRange.length : c.preventive;
        if (count <= 0) continue;
        const last = inRange.length > 0 ? [...inRange].sort().at(-1)! : (c.last_at ?? "");
        rows.push({ client: cl.name ?? "", code: cl.code ?? "", svc: sv, count, last });
      }
    }
    return rows;
  }, [res, from, to]);

  const options = useMemo(() => {
    const uniq = (f: (r: Row) => string | undefined) => Array.from(new Set(dated.map(f).filter((v): v is string => !!v))).sort((a, b) => a.localeCompare(b, "pt-BR"));
    return { status: uniq((r) => r.svc.status), plan: uniq((r) => r.svc.plan), city: uniq((r) => r.svc.city) };
  }, [dated]);

  const rows = useMemo(() => {
    const s = search.trim().toLowerCase();
    return dated
      .filter((r) => (!fStatus || r.svc.status === fStatus) && (!fPlan || r.svc.plan === fPlan) && (!fCity || r.svc.city === fCity))
      .filter((r) => !s || [r.client, r.code, r.svc.login, r.svc.plan, r.svc.city].some((v) => (v ?? "").toLowerCase().includes(s)))
      .sort((a, b) => b.count - a.count || a.client.localeCompare(b.client, "pt-BR"));
  }, [dated, fStatus, fPlan, fCity, search]);

  const stats = useMemo(() => {
    const dist = [
      { label: "1 vez", n: 0 },
      { label: "2 vezes", n: 0 },
      { label: "3 vezes", n: 0 },
      { label: "4 vezes", n: 0 },
      { label: "5 vezes ou mais", n: 0 },
    ];
    for (const r of rows) dist[Math.min(r.count, 5) - 1].n++;
    return { dist, occurrences: rows.reduce((a, r) => a + r.count, 0) };
  }, [rows]);

  const filtered = !!(from || to || fStatus || fPlan || fCity || search.trim());

  function clearFilters() {
    setFrom("");
    setTo("");
    setFStatus("");
    setFPlan("");
    setFCity("");
    setSearch("");
  }

  function exportCsv() {
    const head = ["Cliente", "Código", "Login", "Plano", "Cidade", "Status", "Desbloqueios preventivos", "Último desbloqueio"];
    const lines = [head, ...rows.map((r) => [r.client, r.code, r.svc.login ?? "", r.svc.plan ?? "", r.svc.city ?? "", r.svc.status ?? "", String(r.count), fmtBR(r.last)])];
    const text = lines.map((l) => l.map(csvEsc).join(";")).join("\r\n");
    const url = URL.createObjectURL(new Blob([`﻿${text}`], { type: "text/csv;charset=utf-8;" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `desbloqueio-preventivo-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const busy = phase !== "idle";
  const pct = progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div className="card" style={{ padding: 14 }}>
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 12 }}>
          <div style={{ minWidth: 0, flex: "1 1 320px" }}>
            <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Desbloqueio preventivo</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>
              Quantos serviços têm “desbloqueio preventivo” registrado e quantas vezes ocorreu em cada um. Considera serviços não cancelados;
              a HubSoft só informa isso cliente a cliente, então a consulta percorre a base em lotes e pode levar alguns minutos.
            </p>
            {res ? <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>Atualizado em {fmtConsultaAt(res.at)}</p> : null}
          </div>
          <div className="row" style={{ gap: 8 }}>
            {busy ? (
              <button type="button" className="btn btn--sm" onClick={() => { stopRef.current = true; }}>
                <Square size={12} style={{ marginRight: 4, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
            <button type="button" className="btn btn--sm btn--primary" disabled={busy} onClick={() => void run()}>
              {busy ? "A consultar…" : res ? "Consultar de novo" : "Consultar"}
            </button>
          </div>
        </div>
      </div>

      {phase === "base" ? (
        <ConsultaLoading text="Carregando a lista de clientes e serviços da HubSoft…" />
      ) : phase === "counting" ? (
        <ConsultaLoading text={`Contando desbloqueios… ${fmtInt(progress.done)} de ${fmtInt(progress.total)} clientes (${pct}%)`} />
      ) : null}

      {!res && !busy ? (
        <div className="hubsoft-empty">Nenhuma consulta feita ainda.</div>
      ) : res ? (
        <>
          <div className="card" style={{ padding: 14 }}>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))", gap: 10, alignItems: "end" }}>
              <label style={selectStyle}>
                Desbloqueio de
                <input type="date" className="input" value={from} onChange={(e) => setFrom(e.target.value)} />
              </label>
              <label style={selectStyle}>
                até
                <input type="date" className="input" value={to} onChange={(e) => setTo(e.target.value)} />
              </label>
              <label style={selectStyle}>
                Status
                <select className="input" value={fStatus} onChange={(e) => setFStatus(e.target.value)}>
                  <option value="">Todos</option>
                  {options.status.map((o) => (
                    <option key={o} value={o}>{o}</option>
                  ))}
                </select>
              </label>
              <label style={selectStyle}>
                Plano
                <select className="input" value={fPlan} onChange={(e) => setFPlan(e.target.value)}>
                  <option value="">Todos</option>
                  {options.plan.map((o) => (
                    <option key={o} value={o}>{o}</option>
                  ))}
                </select>
              </label>
              <label style={selectStyle}>
                Cidade
                <select className="input" value={fCity} onChange={(e) => setFCity(e.target.value)}>
                  <option value="">Todas</option>
                  {options.city.map((o) => (
                    <option key={o} value={o}>{o}</option>
                  ))}
                </select>
              </label>
              <div>
                <button type="button" className="btn btn--sm" disabled={!filtered} onClick={clearFilters}>
                  Limpar filtros
                </button>
              </div>
            </div>
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
              Os filtros são aplicados sobre o resultado já coletado (não consultam a HubSoft). O período considera a data de cada desbloqueio preventivo.
            </p>
          </div>

          <div className="card" style={{ padding: 14 }}>
            <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))" }}>
              <div className="stat">
                <div className="stat__k">Serviços com desbloqueio preventivo</div>
                <div className="stat__v">{fmtInt(rows.length)}</div>
              </div>
              <div className="stat">
                <div className="stat__k">Ocorrências (soma)</div>
                <div className="stat__v">{fmtInt(stats.occurrences)}</div>
              </div>
              <div className="stat">
                <div className="stat__k">Serviços consultados</div>
                <div className="stat__v">{fmtInt(res.base.services)}</div>
              </div>
            </div>
            {res.stopped ? <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>Consulta incompleta — os números cobrem só os clientes já percorridos.</p> : null}
            {res.failed > 0 ? <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>{fmtInt(res.failed)} cliente(s) não puderam ser consultados; consulte de novo para completar.</p> : null}
            {res.base.truncated ? <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>Base maior que o teto de páginas — parte dos clientes ficou de fora.</p> : null}
          </div>

          <div className="card" style={{ padding: 14 }}>
            <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>Quantas vezes ocorreu</h4>
            <div className="table-wrap">
              <table style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>Desbloqueios preventivos no serviço</th>
                    <th>Serviços</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.dist.map((x) => (
                    <tr key={x.label}>
                      <td>{x.label}</td>
                      <td className="mono">{fmtInt(x.n)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="card" style={{ padding: 14 }}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8, marginBottom: 8 }}>
              <h4 style={{ margin: 0, fontSize: 13 }}>Serviços ({fmtInt(rows.length)})</h4>
              <div className="row" style={{ gap: 6 }}>
                <input className="input" style={{ fontSize: 12 }} placeholder="Buscar por nome, login, plano, cidade…" value={search} onChange={(e) => setSearch(e.target.value)} />
                <button type="button" className="btn btn--sm" disabled={rows.length === 0} onClick={exportCsv}>
                  <Download size={12} style={{ marginRight: 4, verticalAlign: -2 }} aria-hidden />
                  Exportar CSV
                </button>
              </div>
            </div>
            {rows.length === 0 ? (
              <div className="hubsoft-empty">Nenhum serviço com desbloqueio preventivo{filtered ? " para esses filtros" : ""}.</div>
            ) : (
              <div className="table-wrap integration-support-table" style={{ maxHeight: 520, overflow: "auto" }}>
                <table className="integration-support-table__grid">
                  <thead>
                    <tr>
                      <th>Cliente</th>
                      <th>Login</th>
                      <th>Plano</th>
                      <th>Cidade</th>
                      <th>Status</th>
                      <th>Preventivos</th>
                      <th>Último</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.svc.id}>
                        <td className="integration-support-table__cell">
                          {r.client || "—"}
                          {r.code ? <span className="mono integration-support-table__meta"> · {r.code}</span> : null}
                        </td>
                        <td className="mono integration-support-table__cell">{r.svc.login || "—"}</td>
                        <td className="integration-support-table__cell">{r.svc.plan || "—"}</td>
                        <td className="integration-support-table__cell">{r.svc.city || "—"}</td>
                        <td className="integration-support-table__cell">{r.svc.status || "—"}</td>
                        <td className="mono integration-support-table__cell">{r.count}</td>
                        <td className="mono integration-support-table__cell">{fmtBR(r.last)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      ) : null}
    </div>
  );
}
