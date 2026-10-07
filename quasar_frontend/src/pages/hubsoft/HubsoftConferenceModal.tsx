import { useEffect, useMemo, useRef, useState } from "react";
import { X } from "lucide-react";
import { PeriodPicker } from "./HubsoftReportPage";
import { useConsultaToast } from "./hubsoftConsulta";
import { Switch } from "../../components/Switch";
import { TableCellExpandableText } from "../../integrations/TableCellExpandableText";
import { apiFetch } from "../../lib/api";
import type { HubsoftConferenceItem, HubsoftConferenceJobStatus, HubsoftConferenceResponse } from "../../integrations/types";
import { todayISO } from "./hubsoftDates";

const SLUG = "hubsoft";
const POLL_MS = 1500;

function fmtInt(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR");
}

function fmtPct(part: number, total: number): string {
  if (total <= 0) return "—";
  return `${((part / total) * 100).toLocaleString("pt-BR", { maximumFractionDigits: 1 })}%`;
}

/** "2026-10-06 09:46:39" → "06/10/2026 09:46". */
function fmtDateTime(v?: string): string {
  const m = (v ?? "").match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
  if (!m) return v?.trim() || "—";
  return `${m[3]}/${m[2]}/${m[1]} ${m[4]}:${m[5]}`;
}

type CheckKey = "connection" | "remote_access" | "ipv6";
type FilterState = { check: CheckKey; value: "ok" | "fail" } | null;

const CHECK_LABELS: Record<CheckKey, { title: string; okLabel: string; failLabel: string }> = {
  connection: { title: "Status da conexão", okLabel: "Conectados", failLabel: "Desconectados" },
  remote_access: { title: "Acesso remoto (HTTP/HTTPS)", okLabel: "Com acesso", failLabel: "Sem acesso" },
  ipv6: { title: "IPv6", okLabel: "Com IPv6", failLabel: "Sem IPv6" },
};

function itemPassesFilter(item: HubsoftConferenceItem, filter: FilterState, statusFilter: Set<string>): boolean {
  if (statusFilter.size > 0 && !statusFilter.has(item.status || "—")) return false;
  if (!filter) return true;
  if (filter.check === "connection") {
    if (!item.connection_checked) return false;
    return filter.value === "ok" ? item.connection_online : !item.connection_online;
  }
  if (filter.check === "remote_access") {
    if (!item.remote_access_checked) return false;
    return filter.value === "ok" ? item.remote_access_ok : !item.remote_access_ok;
  }
  if (!item.ipv6_checked) return false;
  return filter.value === "ok" ? item.ipv6_present : !item.ipv6_present;
}

/** Barra de progresso (o servidor informa o percentual de 5 em 5). */
function ProgressBar({ percent, label }: { percent: number; label: string }) {
  return (
    <div role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent} style={{ margin: "4px 0 14px" }}>
      <div className="row" style={{ justifyContent: "space-between", fontSize: 12, marginBottom: 4 }}>
        <span>{label || "Consultando…"}</span>
        <b className="mono">{percent}%</b>
      </div>
      <div style={{ height: 10, borderRadius: 6, background: "var(--panel2)", border: "1px solid var(--border)", overflow: "hidden" }}>
        <div style={{ width: `${percent}%`, height: "100%", background: "var(--accent)", transition: "width .6s ease" }} />
      </div>
    </div>
  );
}

export function HubsoftConferenceModal({ onClose }: { onClose: () => void }) {
  const [from, setFrom] = useState(todayISO());
  const [to, setTo] = useState(todayISO());
  const [onlyFinished, setOnlyFinished] = useState(true);
  const [checkConnection, setCheckConnection] = useState(true);
  const [checkRemoteAccess, setCheckRemoteAccess] = useState(true);
  const [checkIPv6, setCheckIPv6] = useState(true);
  const [filter, setFilter] = useState<FilterState>(null);
  const [statusFilter, setStatusFilter] = useState<Set<string>>(new Set());

  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState({ percent: 0, label: "" });
  const [d, setD] = useState<HubsoftConferenceResponse | null>(null);
  const [error, setError] = useState("");
  const aliveRef = useRef(true);
  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false; // fechar o modal interrompe o acompanhamento
    };
  }, []);

  const { notify, missing } = useConsultaToast();

  async function start() {
    setRunning(true);
    setError("");
    setD(null);
    setFilter(null);
    setStatusFilter(new Set());
    setProgress({ percent: 0, label: "Iniciando" });
    try {
      const started = await apiFetch<{ job_id: string }>(`/api/v1/integrations/${SLUG}/hubsoft/conference`, {
        method: "POST",
        json: {
          data_inicio: from,
          data_fim: to,
          only_finished: onlyFinished,
          check_connection: checkConnection,
          check_remote_access: checkRemoteAccess,
          check_ipv6: checkIPv6,
        },
      });
      for (;;) {
        await new Promise((r) => setTimeout(r, POLL_MS));
        if (!aliveRef.current) return;
        const st = await apiFetch<HubsoftConferenceJobStatus>(`/api/v1/integrations/${SLUG}/hubsoft/conference/${started.job_id}`);
        setProgress({ percent: st.percent, label: st.label });
        if (st.status === "running") continue;
        const result = st.result ?? null;
        if (result) setD(result);
        if (st.status === "error") setError(st.message || result?.message || "Falha ao executar a conferência.");
        else notify(result, null);
        break;
      }
    } catch (e) {
      if (aliveRef.current) {
        setError(e instanceof Error ? e.message : String(e));
        notify(null, e);
      }
    } finally {
      if (aliveRef.current) setRunning(false);
    }
  }

  function consult() {
    if (!from || !to) return missing("informe o período (De / Até).");
    if (from > to) return missing("o período está invertido (a data inicial é maior que a final).");
    if (!checkConnection && !checkRemoteAccess && !checkIPv6) return missing("ligue pelo menos uma verificação (conexão, acesso remoto ou IPv6).");
    void start();
  }

  const availableStatuses = useMemo(() => {
    if (!d?.items) return [];
    const set = new Set<string>();
    for (const it of d.items) set.add(it.status || "—");
    return Array.from(set).sort((a, b) => a.localeCompare(b, "pt-BR"));
  }, [d?.items]);

  const filteredItems = useMemo(() => {
    if (!d?.items) return [];
    return d.items.filter((it) => itemPassesFilter(it, filter, statusFilter));
  }, [d?.items, filter, statusFilter]);

  function toggleFilter(check: CheckKey, value: "ok" | "fail") {
    setFilter((cur) => (cur && cur.check === check && cur.value === value ? null : { check, value }));
  }

  function toggleStatus(status: string) {
    setStatusFilter((cur) => {
      const next = new Set(cur);
      if (next.has(status)) next.delete(status);
      else next.add(status);
      return next;
    });
  }

  function StatTile({ check }: { check: CheckKey }) {
    if (!d) return null;
    const bucket = d.stats[check];
    const labels = CHECK_LABELS[check];
    const okActive = filter?.check === check && filter.value === "ok";
    const failActive = filter?.check === check && filter.value === "fail";
    return (
      <div className="stat" style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <div className="stat__k">{labels.title}</div>
        <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
          <button
            type="button"
            className={`btn btn--sm${okActive ? " btn--primary" : ""}`}
            disabled={bucket.checked === 0}
            onClick={() => toggleFilter(check, "ok")}
            title={`Ver só ${labels.okLabel.toLowerCase()}`}
          >
            {labels.okLabel}: {fmtInt(bucket.ok)} ({fmtPct(bucket.ok, bucket.checked)})
          </button>
          <button
            type="button"
            className={`btn btn--sm${failActive ? " btn--danger" : ""}`}
            disabled={bucket.checked === 0}
            onClick={() => toggleFilter(check, "fail")}
            title={`Ver só ${labels.failLabel.toLowerCase()}`}
          >
            {labels.failLabel}: {fmtInt(bucket.fail)} ({fmtPct(bucket.fail, bucket.checked)})
          </button>
        </div>
        {bucket.checked < (d.resolved ?? 0) ? (
          <span style={{ fontSize: 11, color: "var(--muted)" }}>
            {fmtInt(bucket.checked)} de {fmtInt(d.resolved)} O.S. resolvidas tinham dado para esta conferência.
          </span>
        ) : null}
      </div>
    );
  }

  const checkCols = Number(checkConnection) + Number(checkRemoteAccess) + Number(checkIPv6);

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        style={{ width: "min(1760px, 98vw)", maxWidth: "min(1760px, 98vw)", maxHeight: "94vh", overflowY: "auto" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 4 }}>
          <div>
            <h3 style={{ margin: 0 }}>Conferência de ordens de serviço</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "4px 0 0" }}>
              Cruza cada O.S. do período com status de conexão, acesso remoto e IPv6 do cliente e mostra quem fechou, quando e como
              (técnico, tipo, motivo e descrição do fechamento).
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <PeriodPicker from={from} to={to} onChange={(f, t) => { setFrom(f); setTo(t); }} />

        <div className="row" style={{ gap: 18, flexWrap: "wrap", marginBottom: 12 }}>
          <Switch
            checked={onlyFinished}
            onChange={setOnlyFinished}
            label="Somente O.S. finalizadas"
            hint="A HubSoft já devolve só as finalizadas (pela data de término) — mais rápido."
          />
          <Switch checked={checkConnection} onChange={setCheckConnection} label="Status da conexão" />
          <Switch checked={checkRemoteAccess} onChange={setCheckRemoteAccess} label="Acesso remoto (HTTP/HTTPS)" />
          <Switch checked={checkIPv6} onChange={setCheckIPv6} label="IPv6" />
        </div>

        <div className="row" style={{ gap: 8, alignItems: "center", marginBottom: 14 }}>
          <button type="button" className="btn btn--primary" disabled={running} onClick={consult}>
            {running ? "Consultando…" : "Consultar"}
          </button>
        </div>

        {running ? <ProgressBar percent={progress.percent} label={progress.label} /> : null}

        {error ? <div className="msg msg--err">{error}</div> : null}
        {d && !d.ok && !error ? <div className="msg msg--err">{d.message || "Falha ao executar a conferência."}</div> : null}

        {d?.ok ? (
          <>
            <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))" }}>
              <div className="stat">
                <div className="stat__k">{onlyFinished ? "O.S. finalizadas no período" : "Total de O.S. no período"}</div>
                <div className="stat__v">{fmtInt(d.total)}</div>
              </div>
              <div className="stat">
                <div className="stat__k">Clientes resolvidos</div>
                <div className="stat__v">{fmtInt(d.resolved)}</div>
                {d.resolved < d.total ? (
                  <span style={{ fontSize: 11, color: "var(--warn)" }}>
                    {fmtInt(d.total - d.resolved)} O.S. sem cliente/serviço correspondente encontrado.
                  </span>
                ) : null}
              </div>
            </div>

            {d.truncated ? (
              <p style={{ fontSize: 11, color: "var(--warn)" }}>Período muito grande — resultado truncado, refine as datas.</p>
            ) : null}

            <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", marginTop: 10 }}>
              {checkConnection ? <StatTile check="connection" /> : null}
              {checkRemoteAccess ? <StatTile check="remote_access" /> : null}
              {checkIPv6 ? <StatTile check="ipv6" /> : null}
            </div>

            {availableStatuses.length > 1 ? (
              <div className="row" style={{ gap: 6, flexWrap: "wrap", margin: "14px 0 0", alignItems: "center" }}>
                <span style={{ fontSize: 12, color: "var(--muted)" }}>Status da O.S.:</span>
                {availableStatuses.map((st) => (
                  <button
                    key={st}
                    type="button"
                    className={`btn btn--sm${statusFilter.has(st) ? " btn--primary" : ""}`}
                    onClick={() => toggleStatus(st)}
                  >
                    {st}
                  </button>
                ))}
                {statusFilter.size > 0 ? (
                  <button type="button" className="btn btn--sm" onClick={() => setStatusFilter(new Set())}>
                    Limpar status
                  </button>
                ) : null}
              </div>
            ) : null}

            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", margin: "16px 0 6px" }}>
              <h4 style={{ margin: 0, fontSize: 13 }}>
                {filter
                  ? `${CHECK_LABELS[filter.check].title} — ${filter.value === "ok" ? CHECK_LABELS[filter.check].okLabel : CHECK_LABELS[filter.check].failLabel}`
                  : onlyFinished
                    ? "O.S. finalizadas do período"
                    : "Todas as O.S. do período"}
                {statusFilter.size > 0 ? ` · status: ${Array.from(statusFilter).join(", ")}` : ""}{" "}
                ({fmtInt(filteredItems.length)})
              </h4>
              {filter ? (
                <button type="button" className="btn btn--sm" onClick={() => setFilter(null)}>
                  Limpar filtro
                </button>
              ) : null}
            </div>

            <div className="table-wrap" style={{ maxHeight: "56vh", overflow: "auto" }}>
              <table style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>O.S.</th>
                    <th>Tipo</th>
                    <th>Status</th>
                    <th>Cliente</th>
                    <th>Login</th>
                    <th>Fechada em</th>
                    <th>Fechada por / técnico</th>
                    <th>Motivo do fechamento</th>
                    <th style={{ minWidth: 220 }}>Descrição do fechamento</th>
                    <th>IPv4</th>
                    {checkConnection ? <th>Conexão</th> : null}
                    {checkRemoteAccess ? <th>Acesso remoto</th> : null}
                    {checkIPv6 ? <th>IPv6</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {filteredItems.length === 0 ? (
                    <tr>
                      <td colSpan={10 + checkCols} style={{ color: "var(--muted)" }}>
                        Nenhuma O.S. neste filtro.
                      </td>
                    </tr>
                  ) : (
                    filteredItems.map((it) => {
                      const who = it.closed_by || (it.technicians ?? []).join(", ");
                      const otherTechs = (it.technicians ?? []).filter((t) => t !== it.closed_by);
                      return (
                        <tr key={`${it.id ?? ""}-${it.number ?? ""}`}>
                          <td className="mono">
                            {it.number || "—"}
                            {it.protocol ? <div style={{ fontSize: 10, color: "var(--muted)" }}>atend. {it.protocol}</div> : null}
                          </td>
                          <td>{it.type || "—"}</td>
                          <td>{it.status || "—"}</td>
                          <td>
                            {it.client_name || "—"}
                            {it.service_name ? <div style={{ fontSize: 10, color: "var(--muted)" }}>{it.service_name}</div> : null}
                          </td>
                          <td className="mono">{it.login || "—"}</td>
                          <td className="mono" style={{ whiteSpace: "nowrap" }}>
                            {fmtDateTime(it.closed_at)}
                            {it.started_at ? <div style={{ fontSize: 10, color: "var(--muted)" }}>início {fmtDateTime(it.started_at)}</div> : null}
                          </td>
                          <td>
                            {who || "—"}
                            {it.closed_by && otherTechs.length > 0 ? (
                              <div style={{ fontSize: 10, color: "var(--muted)" }}>+ {otherTechs.join(", ")}</div>
                            ) : null}
                          </td>
                          <td>{(it.closing_reasons ?? []).join(", ") || "—"}</td>
                          <td style={{ maxWidth: 360 }}>
                            <TableCellExpandableText text={it.closing_description} maxLength={90} />
                          </td>
                          <td className="mono">{it.ipv4 || "—"}</td>
                          {checkConnection ? (
                            <td>
                              {!it.connection_checked ? (
                                <span style={{ color: "var(--muted)" }}>—</span>
                              ) : it.connection_online ? (
                                <span className="badge badge--ok">Conectado</span>
                              ) : (
                                <span className="badge badge--off">Desconectado</span>
                              )}
                            </td>
                          ) : null}
                          {checkRemoteAccess ? (
                            <td>
                              {!it.remote_access_checked ? (
                                <span style={{ color: "var(--muted)" }}>—</span>
                              ) : it.remote_access_ok ? (
                                <span className="badge badge--ok">OK</span>
                              ) : (
                                <span className="badge badge--off">Sem acesso</span>
                              )}
                            </td>
                          ) : null}
                          {checkIPv6 ? (
                            <td>
                              {!it.ipv6_checked ? (
                                <span style={{ color: "var(--muted)" }}>—</span>
                              ) : it.ipv6_present ? (
                                <span className="badge badge--ok">Sim</span>
                              ) : (
                                <span className="badge badge--off">Não</span>
                              )}
                            </td>
                          ) : null}
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
