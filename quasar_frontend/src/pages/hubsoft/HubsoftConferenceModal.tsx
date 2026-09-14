import { useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { X } from "lucide-react";
import { PeriodPicker } from "./HubsoftReportPage";
import { Switch } from "../../components/Switch";
import { apiFetch } from "../../lib/api";
import type { HubsoftConferenceItem, HubsoftConferenceResponse } from "../../integrations/types";

const SLUG = "hubsoft";

function todayISO(offsetDays = 0): string {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return d.toISOString().slice(0, 10);
}

function fmtInt(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR");
}

function fmtPct(part: number, total: number): string {
  if (total <= 0) return "—";
  return `${((part / total) * 100).toLocaleString("pt-BR", { maximumFractionDigits: 1 })}%`;
}

type CheckKey = "connection" | "remote_access" | "ipv6";
type FilterState = { check: CheckKey; value: "ok" | "fail" } | null;

const CHECK_LABELS: Record<CheckKey, { title: string; okLabel: string; failLabel: string }> = {
  connection: { title: "Status da conexão", okLabel: "Conectados", failLabel: "Desconectados" },
  remote_access: { title: "Acesso remoto (HTTP/HTTPS)", okLabel: "Com acesso", failLabel: "Sem acesso" },
  ipv6: { title: "IPv6", okLabel: "Com IPv6", failLabel: "Sem IPv6" },
};

function itemPassesFilter(item: HubsoftConferenceItem, filter: FilterState): boolean {
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

export function HubsoftConferenceModal({ onClose }: { onClose: () => void }) {
  const [from, setFrom] = useState(todayISO(-30));
  const [to, setTo] = useState(todayISO());
  const [checkConnection, setCheckConnection] = useState(true);
  const [checkRemoteAccess, setCheckRemoteAccess] = useState(true);
  const [checkIPv6, setCheckIPv6] = useState(true);
  const [filter, setFilter] = useState<FilterState>(null);

  const run = useMutation({
    mutationFn: () =>
      apiFetch<HubsoftConferenceResponse>(`/api/v1/integrations/${SLUG}/hubsoft/conference`, {
        method: "POST",
        json: {
          data_inicio: from,
          data_fim: to,
          check_connection: checkConnection,
          check_remote_access: checkRemoteAccess,
          check_ipv6: checkIPv6,
        },
        timeoutMs: 3 * 60_000,
      }),
    onSuccess: () => setFilter(null),
  });

  const d = run.data;

  const filteredItems = useMemo(() => {
    if (!d?.items) return [];
    return d.items.filter((it) => itemPassesFilter(it, filter));
  }, [d?.items, filter]);

  function toggleFilter(check: CheckKey, value: "ok" | "fail") {
    setFilter((cur) => (cur && cur.check === check && cur.value === value ? null : { check, value }));
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

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        style={{ width: "min(1100px, 96vw)", maxHeight: "92vh", overflowY: "auto" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 4 }}>
          <div>
            <h3 style={{ margin: 0 }}>Conferência de ordens de serviço</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "4px 0 0" }}>
              Cruza cada O.S. do período com status de conexão, acesso remoto e IPv6 do cliente — os mesmos dados já
              usados nas abas Relatório → Clientes e Ferramentas → HTTP/HTTPS.
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <PeriodPicker from={from} to={to} onChange={(f, t) => { setFrom(f); setTo(t); }} />

        <div className="row" style={{ gap: 18, flexWrap: "wrap", marginBottom: 12 }}>
          <Switch checked={checkConnection} onChange={setCheckConnection} label="Status da conexão" />
          <Switch checked={checkRemoteAccess} onChange={setCheckRemoteAccess} label="Acesso remoto (HTTP/HTTPS)" />
          <Switch checked={checkIPv6} onChange={setCheckIPv6} label="IPv6" />
        </div>

        <div className="row" style={{ gap: 8, alignItems: "center", marginBottom: 14 }}>
          <button
            type="button"
            className="btn btn--primary"
            disabled={run.isPending || (!checkConnection && !checkRemoteAccess && !checkIPv6)}
            onClick={() => run.mutate()}
          >
            {run.isPending ? "A conferir…" : "Executar conferência"}
          </button>
          {run.isPending ? (
            <span style={{ fontSize: 12, color: "var(--muted)" }}>
              Pode demorar — depende de quantas O.S. o período tem (testa acesso remoto de cada cliente, se activado).
            </span>
          ) : null}
        </div>

        {run.isError ? <div className="msg msg--err">{(run.error as Error).message}</div> : null}
        {d && !d.ok ? <div className="msg msg--err">{d.message || "Falha ao executar a conferência."}</div> : null}

        {d?.ok ? (
          <>
            <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))" }}>
              <div className="stat">
                <div className="stat__k">Total de O.S. no período</div>
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

            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", margin: "16px 0 6px" }}>
              <h4 style={{ margin: 0, fontSize: 13 }}>
                {filter
                  ? `${CHECK_LABELS[filter.check].title} — ${filter.value === "ok" ? CHECK_LABELS[filter.check].okLabel : CHECK_LABELS[filter.check].failLabel}`
                  : "Todas as O.S. do período"}{" "}
                ({fmtInt(filteredItems.length)})
              </h4>
              {filter ? (
                <button type="button" className="btn btn--sm" onClick={() => setFilter(null)}>
                  Limpar filtro
                </button>
              ) : null}
            </div>

            <div className="table-wrap" style={{ maxHeight: 420, overflowY: "auto" }}>
              <table style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>O.S.</th>
                    <th>Status</th>
                    <th>Cliente</th>
                    <th>Login</th>
                    <th>IPv4</th>
                    {checkConnection ? <th>Conexão</th> : null}
                    {checkRemoteAccess ? <th>Acesso remoto</th> : null}
                    {checkIPv6 ? <th>IPv6</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {filteredItems.length === 0 ? (
                    <tr>
                      <td colSpan={5 + Number(checkConnection) + Number(checkRemoteAccess) + Number(checkIPv6)} style={{ color: "var(--muted)" }}>
                        Nenhuma O.S. neste filtro.
                      </td>
                    </tr>
                  ) : (
                    filteredItems.map((it) => (
                      <tr key={`${it.id ?? ""}-${it.number ?? ""}`}>
                        <td className="mono">{it.number || "—"}</td>
                        <td>{it.status || "—"}</td>
                        <td>{it.client_name || "—"}</td>
                        <td className="mono">{it.login || "—"}</td>
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
                    ))
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
