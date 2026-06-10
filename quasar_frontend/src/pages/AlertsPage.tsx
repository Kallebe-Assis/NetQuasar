import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { ActionMenu } from "../components/ActionMenu";
import { InfoHint } from "../components/InfoHint";
import { useEffect, useMemo, useState } from "react";
import { flushSync } from "react-dom";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { useMonitoringLiveSync } from "../lib/monitoringLiveSync";
import { invalidateAlertListQueries, queryKeys } from "../lib/queryKeys";
import {
  activeRowSeverityPillClass,
  displayActiveRowSeverity,
  displayAlertMessage,
  displaySeverity,
  formatAlertDateTimePt,
  severityPillClass,
} from "../lib/alertLabels";
import {
  ALERT_SEVERITY_FILTER_OPTIONS,
  ALERT_TYPE_FILTER_OPTIONS,
  alertCategoryFromType,
  alertCategoryLabel,
  alertEquipmentPrimary,
  alertProblemTitle,
  alertValueText,
  formatRelativeCompactPt,
} from "../lib/alertsPresentation";

type ActiveAlert = {
  id: string;
  device_id: string;
  severity: string;
  type: string;
  message: string;
  ip: string;
  device_name: string;
  active_since: string;
  /** Preenchido ~1 min após fecho: linha mostrada como «Resolvido» na lista Ativos. */
  closed_at?: string | null;
  incident_id?: string | null;
  meta?: unknown;
};

type OpenIncident = {
  id: string;
  root_cause: string;
  title: string;
  summary?: string | null;
  alert_count: number;
  open_alert_count: number;
  opened_at: string;
};

function incidentCauseLabel(cause: string): string {
  switch (cause) {
    case "pop_offline":
      return "POP offline";
    case "olt_offline":
      return "OLT offline";
    default:
      return cause;
  }
}

type IgnoredAlert = {
  id: string;
  device_id: string;
  type: string;
  meta_key?: string;
  device_name?: string;
  ip?: string;
  severity?: string;
  message?: string;
  meta?: unknown;
  reason?: string;
  ignored_at: string;
  last_verified_at?: string | null;
  last_verify_result?: Record<string, unknown>;
};

type VerifyResult = {
  alert_id: string;
  still_active: boolean;
  resolved: boolean;
  summary: string;
  collected?: Record<string, unknown>;
};

type HistoryEvent = {
  id: string;
  device_id?: string | null;
  device_name?: string | null;
  ip?: string | null;
  severity: string;
  type: string;
  message: string;
  active_since: string;
  closed_at?: string | null;
  meta?: unknown;
};

type MonStateLite = {
  runtime_updated_at?: string | null;
  last_alerts_change_at?: string | null;
  last_telemetry_cycle_at?: string | null;
  last_latency_cycle_at?: string | null;
  last_interface_snapshot_cycle_at?: string | null;
  last_olt_if_derived_cycle_at?: string | null;
  activity_updated_at?: string | null;
};

/** Recarrega alertas periodicamente — mesma instância pode ter message/meta novos (ex.: latência 243→210). */
const ALERTS_ACTIVE_REFRESH_MS = 2_500;
const ALERTS_HISTORY_REFRESH_MS = 45_000;

export function AlertsPage() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [tab, setTab] = useState<"active" | "hist">("active");
  const [ignoredOpen, setIgnoredOpen] = useState(false);
  const [verifyingId, setVerifyingId] = useState<string | null>(null);
  const [agoTick, setAgoTick] = useState(0);
  const [refreshTick, setRefreshTick] = useState(0);
  const [sev, setSev] = useState("");
  const [typ, setTyp] = useState("");
  const [limitActive] = useState("5000");
  const [limitHist] = useState("5000");
  const [histSearch, setHistSearch] = useState("");
  const [histFrom, setHistFrom] = useState("");
  const [histTo, setHistTo] = useState("");
  const [searchActive, setSearchActive] = useState("");

  useEffect(() => {
    if (tab !== "active") return;
    const id = window.setInterval(() => setAgoTick((n) => n + 1), Math.max(ALERTS_ACTIVE_REFRESH_MS, 4000));
    return () => window.clearInterval(id);
  }, [tab]);

  useEffect(() => {
    const id = window.setInterval(() => setRefreshTick((n) => n + 1), Math.max(ALERTS_ACTIVE_REFRESH_MS, 15000));
    return () => window.clearInterval(id);
  }, []);

  const monState = useQuery({
    queryKey: queryKeys.monState,
    staleTime: 1000,
  });

  useMonitoringLiveSync(monState.data, { monitoring: false, alerts: true, olt: false });

  const incidents = useQuery({
    queryKey: queryKeys.alertsIncidents,
    queryFn: () => apiFetch<{ incidents: OpenIncident[] }>("/api/v1/alerts/incidents/active"),
    refetchInterval: ALERTS_ACTIVE_REFRESH_MS,
    enabled: tab === "active",
  });

  const active = useQuery({
    queryKey: [...queryKeys.alertsActive, sev, typ, limitActive],
    queryFn: () => {
      const p = new URLSearchParams();
      if (sev.trim()) p.set("severity", sev.trim());
      if (typ.trim()) p.set("type", typ.trim());
      const lim = Math.min(5000, Math.max(1, Number(limitActive) || 5000));
      p.set("limit", String(lim));
      return apiFetch<{ alerts: ActiveAlert[] }>(`/api/v1/alerts/active?${p.toString()}`);
    },
    staleTime: 0,
    refetchOnMount: "always",
    /** Reverter o default global (main.tsx desativa refetch ao foco). */
    refetchOnWindowFocus: true,
    refetchInterval: tab === "active" ? ALERTS_ACTIVE_REFRESH_MS : false,
    refetchIntervalInBackground: tab === "active",
  });

  const histRange = useMemo(() => {
    const to = Date.now();
    const from = to - 24 * 3600 * 1000;
    return { from: new Date(from).toISOString(), to: new Date(to).toISOString() };
  }, [refreshTick]);

  const resolved24h = useQuery({
    queryKey: ["alerts-resolved-window", histRange.from, histRange.to],
    queryFn: () => {
      const p = new URLSearchParams({
        limit: "400",
        from: histRange.from,
        to: histRange.to,
      });
      return apiFetch<{ events: HistoryEvent[] }>(`/api/v1/alerts/history?${p}`);
    },
    staleTime: Math.min(ALERTS_ACTIVE_REFRESH_MS / 2, 5_000),
    enabled: tab === "active",
    refetchInterval: tab === "active" ? ALERTS_ACTIVE_REFRESH_MS : false,
  });

  const hist = useQuery({
    queryKey: ["alerts-hist", limitHist, histFrom, histTo],
    queryFn: () => {
      const lim = Math.min(5000, Math.max(1, Number(limitHist) || 5000));
      const p = new URLSearchParams({ limit: String(lim) });
      const from = histFrom.trim();
      const to = histTo.trim();
      if (from && to) {
        p.set("from", new Date(from).toISOString());
        p.set("to", new Date(to).toISOString());
      }
      return apiFetch<{ events: HistoryEvent[] }>(`/api/v1/alerts/history?${p}`);
    },
    enabled: tab === "hist",
    refetchOnMount: "always",
    refetchInterval: tab === "hist" ? ALERTS_HISTORY_REFRESH_MS : false,
    refetchIntervalInBackground: tab === "hist",
  });

  const ignoredQ = useQuery({
    queryKey: queryKeys.alertsIgnored,
    queryFn: () => apiFetch<{ ignored: IgnoredAlert[] }>("/api/v1/alerts/ignored"),
    enabled: ignoredOpen,
  });

  const verifyAll = useMutation({
    mutationFn: async () => {
      await apiFetch<{ ok: boolean; closed_ping_count?: number; verified_count?: number; resolved_count?: number }>(
        "/api/v1/alerts/verify-all",
        { method: "POST", json: {} },
      );
      await active.refetch();
      await incidents.refetch();
      await resolved24h.refetch();
    },
    onSuccess: () => {
      void invalidateAlertListQueries(qc);
      pushToast({ tone: "ok", text: "Alertas verificados e lista actualizada." });
    },
    onError: (e: unknown) => {
      pushToast({ tone: "err", text: e instanceof Error ? e.message : "Falha ao verificar alertas." });
    },
  });

  const ignoreMut = useMutation({
    mutationFn: (alertId: string) => apiFetch(`/api/v1/alerts/${alertId}/ignore`, { method: "POST", json: {} }),
    onSuccess: () => {
      void invalidateAlertListQueries(qc);
      pushToast({ tone: "info", text: "Alerta ignorado — não voltará a alarmar nem no Telegram." });
    },
    onError: (e: unknown) => {
      pushToast({ tone: "err", text: e instanceof Error ? e.message : "Não foi possível ignorar." });
    },
  });

  const verifyOneMut = useMutation({
    mutationFn: (alertId: string) => apiFetch<VerifyResult>(`/api/v1/alerts/${alertId}/verify`, { method: "POST", json: {} }),
    onSuccess: (res) => {
      void invalidateAlertListQueries(qc);
      pushToast({
        tone: res.resolved ? "ok" : "info",
        text: res.summary || (res.resolved ? "Problema normalizado." : "Verificação concluída."),
      });
    },
    onError: (e: unknown) => {
      pushToast({ tone: "err", text: e instanceof Error ? e.message : "Falha na verificação." });
    },
    onSettled: () => setVerifyingId(null),
  });

  const reactivateMut = useMutation({
    mutationFn: (ignoreId: string) => apiFetch(`/api/v1/alerts/ignored/${ignoreId}/reactivate`, { method: "POST", json: {} }),
    onSuccess: () => {
      void invalidateAlertListQueries(qc);
      void ignoredQ.refetch();
      pushToast({ tone: "ok", text: "Alerta reactivado — voltará a ser monitorizado." });
    },
    onError: (e: unknown) => {
      pushToast({ tone: "err", text: e instanceof Error ? e.message : "Falha ao reactivar." });
    },
  });

  async function runVerifyOne(alertId: string) {
    setVerifyingId(alertId);
    verifyOneMut.mutate(alertId);
  }

  const rawAlerts = active.data?.alerts ?? [];
  const filteredActive = useMemo(() => {
    const q = searchActive.trim().toLowerCase();
    if (!q) return rawAlerts;
    return rawAlerts.filter((a) => {
      const hay = [
        a.device_name,
        a.ip,
        a.message,
        a.type,
        displaySeverity(a.severity),
        displayActiveRowSeverity(a.severity, a.closed_at ?? null),
        alertProblemTitle(a.type),
      ]
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    });
  }, [rawAlerts, searchActive]);


  const stats = useMemo(() => {
    const openOnly = rawAlerts.filter((a) => !a.closed_at);
    const crit = openOnly.filter((a) => a.severity === "critical").length;
    const warn = openOnly.filter((a) => a.severity === "warning").length;
    const info = openOnly.filter((a) => a.severity === "info").length;
    const events = resolved24h.data?.events ?? [];
    const cutoff = Date.now() - 24 * 3600 * 1000;
    const resolvedN = events.filter((e) => {
      if (!e.closed_at) return false;
      const t = new Date(e.closed_at).getTime();
      return !Number.isNaN(t) && t >= cutoff;
    }).length;
    return {
      active: openOnly.length,
      critical: crit,
      warning: warn,
      info,
      resolved24h: resolvedN,
    };
  }, [rawAlerts, resolved24h.data?.events]);


  const filteredHistory = useMemo(() => {
    const list = hist.data?.events ?? [];
    const q = histSearch.trim().toLowerCase();
    if (!q) return list;
    return list.filter((e) => {
      const haystack = [
        e.message,
        e.type,
        e.severity,
        e.device_name ?? "",
        e.ip ?? "",
        alertProblemTitle(e.type),
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [hist.data?.events, histSearch]);


  return (
    <div className="alerts-page">
      <div className="page-heading">
        <h1>
          Alertas
          <InfoHint label="Sobre a lista de alertas">
            <p>
              Valores (latência, dBm, etc.) actualizam quando o worker grava na BD; renovação automática a cada ~
              {Math.round(ALERTS_ACTIVE_REFRESH_MS / 1000)} s com esta página aberta.
            </p>
          </InfoHint>
        </h1>
      </div>

      <div className="alerts-stat-grid">
        <div className="alerts-stat-card alerts-stat-card--active">
          <span className="alerts-stat-card__lab">Alertas ativos</span>
          <span className="alerts-stat-card__val">{stats.active}</span>
        </div>
        <div className="alerts-stat-card alerts-stat-card--critical">
          <span className="alerts-stat-card__lab">Crítico</span>
          <span className="alerts-stat-card__val">{stats.critical}</span>
        </div>
        <div className="alerts-stat-card alerts-stat-card--warning">
          <span className="alerts-stat-card__lab">Atenção</span>
          <span className="alerts-stat-card__val">{stats.warning}</span>
        </div>
        <div className="alerts-stat-card alerts-stat-card--info">
          <span className="alerts-stat-card__lab">Informação</span>
          <span className="alerts-stat-card__val">{stats.info}</span>
        </div>
        <div className="alerts-stat-card alerts-stat-card--resolved">
          <span className="alerts-stat-card__lab">Resolvidos (24 h)</span>
          <span className="alerts-stat-card__val">{resolved24h.isLoading ? "…" : stats.resolved24h}</span>
        </div>
      </div>

      <div className="tabs" style={{ marginBottom: "0.65rem", flexWrap: "wrap" }}>
        <button type="button" className={tab === "active" ? "active" : ""} onClick={() => setTab("active")}>
          Ativos
        </button>
        <button type="button" className={tab === "hist" ? "active" : ""} onClick={() => setTab("hist")}>
          Histórico
        </button>
      </div>

      {tab === "active" && (incidents.data?.incidents?.length ?? 0) > 0 && (
        <div className="card" style={{ marginBottom: 12 }}>
          <h2 style={{ fontSize: 15, marginTop: 0 }}>Incidentes correlacionados</h2>
          <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
            Alertas agrupados por causa provável (POP com vários equipamentos offline, OLT offline com efeito em cascata).
          </p>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Incidente</th>
                  <th>Causa</th>
                  <th>Alertas</th>
                  <th>Abertos</th>
                  <th>Desde</th>
                </tr>
              </thead>
              <tbody>
                {incidents.data!.incidents.map((inc) => (
                  <tr key={inc.id}>
                    <td>{inc.title}</td>
                    <td>
                      <span className="badge badge--off">{incidentCauseLabel(inc.root_cause)}</span>
                    </td>
                    <td className="mono">{inc.alert_count}</td>
                    <td className="mono">{inc.open_alert_count}</td>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {formatAlertDateTimePt(inc.opened_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab === "active" && (
        <>
          <div className="alerts-toolbar">
            <div className="field alerts-toolbar__search" style={{ margin: 0 }}>
              <label style={{ fontSize: 11, color: "var(--muted)" }}>Buscar</label>
              <input
                className="input"
                placeholder="Equipamento, IP, tipo de problema…"
                value={searchActive}
                onChange={(e) => setSearchActive(e.target.value)}
              />
            </div>
            <div className="field" style={{ margin: 0, minWidth: 160 }}>
              <label style={{ fontSize: 11, color: "var(--muted)" }}>Tipo</label>
              <select className="input" value={typ} onChange={(e) => setTyp(e.target.value)}>
                {ALERT_TYPE_FILTER_OPTIONS.map((o) => (
                  <option key={o.value || "all"} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="field" style={{ margin: 0, minWidth: 140 }}>
              <label style={{ fontSize: 11, color: "var(--muted)" }}>Severidade</label>
              <select className="input" value={sev} onChange={(e) => setSev(e.target.value)}>
                {ALERT_SEVERITY_FILTER_OPTIONS.map((o) => (
                  <option key={o.value || "all-sev"} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
            <button
              type="button"
              className="btn btn--primary"
              disabled={verifyAll.isPending}
              title="Recalcula estado no servidor (ping, limiares, OLT…) e actualiza a lista"
              onClick={() => verifyAll.mutate()}
            >
              {verifyAll.isPending ? (
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                  <Loader2 size={16} className="map-refresh-spin" aria-hidden />
                  A verificar…
                </span>
              ) : (
                "Verificar alertas"
              )}
            </button>
            <button type="button" className="btn" onClick={() => setIgnoredOpen(true)}>
              Alertas ignorados
            </button>
          </div>
          {verifyAll.isError && <div className="msg msg--err margin-bottom mb-12">{(verifyAll.error as Error).message}</div>}

          <div className="alerts-panel">
            <div className="alerts-panel__head">
              <strong style={{ fontSize: 14 }}>Lista de alertas</strong>
            </div>
            {active.isLoading && <p style={{ padding: 16 }}>A carregar…</p>}
            {active.isError && <div className="msg msg--err margin m-14">{(active.error as Error).message}</div>}
            {!active.isLoading && active.data && (
              <>
                <div className="table-wrap" style={{ border: "none", borderRadius: 0 }}>
                  <table>
                    <thead>
                      <tr>
                        <th>Quando</th>
                        <th>Severidade</th>
                        <th>Categoria</th>
                        <th>Problema</th>
                        <th>Valor</th>
                        <th>Equipamento</th>
                        <th>Estado</th>
                        <th style={{ width: 48 }} />
                      </tr>
                    </thead>
                    <tbody>
                      {filteredActive.map((a) => {
                        const cat = alertCategoryFromType(a.type);
                        const resolved = Boolean(a.closed_at);
                        const timeRef = resolved ? (a.closed_at as string) : a.active_since;
                        const busy = verifyingId === a.id && verifyOneMut.isPending;
                        return (
                          <tr key={a.id}>
                            <td style={{ whiteSpace: "nowrap", fontSize: 12 }} title={formatAlertDateTimePt(timeRef)}>
                              {resolved ? (
                                <>
                                  <span title="Quando voltou ao normal">{formatRelativeCompactPt(timeRef, agoTick)}</span>
                                  <span style={{ display: "block", fontSize: 10, color: "var(--muted)" }}>
                                    normalizado
                                  </span>
                                </>
                              ) : (
                                formatRelativeCompactPt(timeRef, agoTick)
                              )}
                            </td>
                            <td>
                              <span className={activeRowSeverityPillClass(a.severity, a.closed_at ?? null)}>
                                {displayActiveRowSeverity(a.severity, a.closed_at ?? null)}
                              </span>
                            </td>
                            <td>
                              <span className="alerts-cat-badge">{alertCategoryLabel(cat)}</span>
                            </td>
                            <td className="alerts-problem">{alertProblemTitle(a.type)}</td>
                            <td className="alerts-msg">{alertValueText(a.type, a.message, a.meta)}</td>
                            <td>
                              <div className="alerts-dev">
                                {alertEquipmentPrimary(a.type, a.device_name, a.message, a.meta)}
                                {a.ip ? <div className="alerts-dev__ip">{a.ip}</div> : null}
                              </div>
                            </td>
                            <td>
                              {resolved ? (
                                <span className="alerts-status-pill alerts-status-pill--resolved">✓ Resolvido</span>
                              ) : (
                                <span className="alerts-status-pill alerts-status-pill--open">● Ativo</span>
                              )}
                              {a.incident_id ? (
                                <span className="badge badge--off" style={{ marginLeft: 6, fontSize: 10 }} title="Incidente correlacionado">
                                  incidente
                                </span>
                              ) : null}
                            </td>
                            <td>
                              {!resolved ? (
                                <ActionMenu
                                  align="end"
                                  title="Opções do alerta"
                                  items={[
                                    {
                                      id: "verify",
                                      label: busy ? "A verificar…" : "Verificar",
                                      disabled: busy || ignoreMut.isPending,
                                      onClick: () => void runVerifyOne(a.id),
                                    },
                                    {
                                      id: "ignore",
                                      label: "Ignorar alerta",
                                      disabled: ignoreMut.isPending || busy,
                                      onClick: () => ignoreMut.mutate(a.id),
                                    },
                                  ]}
                                />
                              ) : null}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
                {filteredActive.length === 0 && <p style={{ padding: 16, color: "var(--muted)", margin: 0 }}>Nenhum alerta neste filtro.</p>}
                <div className="alerts-panel__foot">
                  <span>{filteredActive.length} alerta(s) nesta lista.</span>
                </div>
              </>
            )}
          </div>
        </>
      )}

      {tab === "hist" && (
        <>
          <div className="alerts-toolbar">
            <div className="field alerts-toolbar__search" style={{ margin: 0 }}>
              <label style={{ fontSize: 11, color: "var(--muted)" }}>Buscar histórico</label>
              <input className="input" placeholder="Texto livre…" value={histSearch} onChange={(e) => setHistSearch(e.target.value)} />
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label style={{ fontSize: 11, color: "var(--muted)" }}>Desde</label>
              <input className="input" type="datetime-local" value={histFrom} onChange={(e) => setHistFrom(e.target.value)} />
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label style={{ fontSize: 11, color: "var(--muted)" }}>Até</label>
              <input className="input" type="datetime-local" value={histTo} onChange={(e) => setHistTo(e.target.value)} />
            </div>
            <button type="button" className="btn btn--primary" disabled={!histFrom.trim() || !histTo.trim()} onClick={() => void hist.refetch()}>
              Aplicar datas
            </button>
            <button
              type="button"
              className="btn"
              onClick={() => {
                flushSync(() => {
                  setHistFrom("");
                  setHistTo("");
                });
                void hist.refetch();
              }}
            >
              Limpar datas
            </button>
            <button type="button" className="btn" onClick={() => void hist.refetch()}>
              Actualizar
            </button>
          </div>
          <p style={{ color: "var(--muted)", fontSize: 12, marginTop: -4 }}>
            Sem datas: mostra os eventos mais recentes (limite no servidor). Com datas, o filtro aplica-se no intervalo escolhido.
          </p>

          <div className="alerts-panel">
            <div className="alerts-panel__head">
              <strong style={{ fontSize: 14 }}>Histórico</strong>
            </div>
            {hist.isLoading && <p style={{ padding: 16 }}>A carregar…</p>}
            {hist.isError && <div className="msg msg--err margin m-14">{(hist.error as Error).message}</div>}
            {hist.data && (
              <>
                <div className="table-wrap" style={{ border: "none", borderRadius: 0 }}>
                  <table>
                    <thead>
                      <tr>
                        <th>Aberto</th>
                        <th>Fechado</th>
                        <th>Severidade</th>
                        <th>Problema</th>
                        <th>Mensagem</th>
                        <th>Equipamento</th>
                        <th>Estado</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredHistory.map((e) => (
                        <tr key={e.id}>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{formatAlertDateTimePt(e.active_since)}</td>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{e.closed_at ? formatAlertDateTimePt(e.closed_at) : "—"}</td>
                          <td>
                            <span className={severityPillClass(e.severity)}>{displaySeverity(e.severity)}</span>
                          </td>
                          <td className="alerts-problem">{alertProblemTitle(e.type)}</td>
                          <td className="alerts-msg">{displayAlertMessage(e.message, e.type)}</td>
                          <td>
                            <div className="alerts-dev">
                              {alertEquipmentPrimary(e.type, e.device_name ?? null, e.message, e.meta)}
                              {e.ip ? <div className="alerts-dev__ip">{e.ip}</div> : null}
                            </div>
                          </td>
                          <td>
                            {e.closed_at ? (
                              <span className="alerts-status-pill alerts-status-pill--resolved">✓ Resolvido</span>
                            ) : (
                              <span className="alerts-status-pill alerts-status-pill--open">● Em aberto</span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {filteredHistory.length === 0 && <p style={{ padding: 16, color: "var(--muted)", margin: 0 }}>Nenhum evento com este filtro.</p>}
                <div className="alerts-panel__foot">
                  <span>{filteredHistory.length} evento(s) nesta lista.</span>
                </div>
              </>
            )}
          </div>
        </>
      )}

      {ignoredOpen ? (
        <div className="modal-overlay" role="presentation" onClick={() => setIgnoredOpen(false)}>
          <div
            className="modal conn-form-modal"
            style={{ width: "min(920px, 92vw)", maxHeight: "85vh", overflow: "auto" }}
            role="dialog"
            aria-labelledby="ignored-alerts-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
              <h2 id="ignored-alerts-title" style={{ margin: 0 }}>
                Alertas ignorados
              </h2>
              <button type="button" className="btn" onClick={() => setIgnoredOpen(false)}>
                Fechar
              </button>
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
              Estes padrões de alerta estão silenciados na UI e no Telegram até serem reactivados.
            </p>
            {ignoredQ.isLoading ? <p>A carregar…</p> : null}
            {ignoredQ.isError ? <div className="msg msg--err">{(ignoredQ.error as Error).message}</div> : null}
            {ignoredQ.data ? (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Ignorado em</th>
                      <th>Severidade</th>
                      <th>Problema</th>
                      <th>Valor</th>
                      <th>Equipamento</th>
                      <th>Última verificação</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {(ignoredQ.data.ignored ?? []).map((row) => (
                      <tr key={row.id}>
                        <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{formatAlertDateTimePt(row.ignored_at)}</td>
                        <td>
                          <span className={severityPillClass(row.severity)}>{displaySeverity(row.severity)}</span>
                        </td>
                        <td className="alerts-problem">{alertProblemTitle(row.type)}</td>
                        <td className="alerts-msg">{alertValueText(row.type, row.message ?? "", row.meta)}</td>
                        <td>
                          <div className="alerts-dev">
                            {alertEquipmentPrimary(row.type, row.device_name ?? null, row.message ?? "", row.meta)}
                            {row.ip ? <div className="alerts-dev__ip">{row.ip}</div> : null}
                          </div>
                        </td>
                        <td style={{ fontSize: 11 }}>
                          {row.last_verified_at ? formatAlertDateTimePt(row.last_verified_at) : "—"}
                          {row.last_verify_result?.summary ? (
                            <div style={{ color: "var(--muted)", marginTop: 4 }}>{String(row.last_verify_result.summary)}</div>
                          ) : null}
                        </td>
                        <td>
                          <button
                            type="button"
                            className="btn"
                            disabled={reactivateMut.isPending}
                            onClick={() => reactivateMut.mutate(row.id)}
                          >
                            Reactivar
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {(ignoredQ.data.ignored ?? []).length === 0 ? (
                  <p style={{ padding: 16, color: "var(--muted)" }}>Nenhum alerta ignorado.</p>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
