import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileExclamationPoint, LayoutGrid, Loader2, MessageCircleX, ShieldAlert, SquareStack } from "lucide-react";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { InfoHint } from "../components/InfoHint";
import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { flushSync } from "react-dom";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { type MonitoringStateSync, useMonitoringLiveSync } from "../lib/monitoringLiveSync";
import { invalidateAlertListQueries, queryKeys } from "../lib/queryKeys";
import {
  activeRowSeverityPillClass,
  displayActiveRowSeverity,
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
import {
  formatAlertDuration,
  formatAlertPopName,
  formatAlertResolvedValue,
} from "../lib/alertResolution";

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
  pop_name?: string | null;
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
  pop_name?: string | null;
};

/** Recarrega alertas periodicamente — mesma instância pode ter message/meta novos (ex.: latência 243→210). */
const ALERTS_ACTIVE_REFRESH_MS = 2_500;
const ALERTS_HISTORY_REFRESH_MS = 45_000;

export function AlertsPage() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [tab, setTab] = useState<"active" | "hist">("active");
  const [ignoredOpen, setIgnoredOpen] = useState(false);
  const [ignoredSearch, setIgnoredSearch] = useState("");
  const [ignoredDevice, setIgnoredDevice] = useState("");
  const [confirmVerifyAll, setConfirmVerifyAll] = useState(false);
  const [reactivateId, setReactivateId] = useState<string | null>(null);
  const [verifyingId, setVerifyingId] = useState<string | null>(null);
  const [agoTick, setAgoTick] = useState(0);
  const [sev, setSev] = useState("");
  const [typ, setTyp] = useState("");
  const [limitActive] = useState("5000");
  const [limitHist] = useState("5000");
  const [histSearch, setHistSearch] = useState("");
  const [histFrom, setHistFrom] = useState("");
  const [histTo, setHistTo] = useState("");
  const [searchActive, setSearchActive] = useState("");
  /** Agrupa a lista por tipo de alerta (offline, latência, SFP…). Ligado por defeito. */
  const [groupByType, setGroupByType] = useState(true);
  const [sevMenuOpen, setSevMenuOpen] = useState(false);
  const [typMenuOpen, setTypMenuOpen] = useState(false);
  const toolbarFiltersRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (tab !== "active") return;
    const id = window.setInterval(() => setAgoTick((n) => n + 1), Math.max(ALERTS_ACTIVE_REFRESH_MS, 4000));
    return () => window.clearInterval(id);
  }, [tab]);

  useEffect(() => {
    if (!sevMenuOpen && !typMenuOpen) return;
    const onDoc = (ev: MouseEvent) => {
      const el = toolbarFiltersRef.current;
      if (el && !el.contains(ev.target as Node)) {
        setSevMenuOpen(false);
        setTypMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [sevMenuOpen, typMenuOpen]);

  const monState = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<MonitoringStateSync>("/api/v1/monitoring/state"),
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
      const res = await apiFetch<{
        ok: boolean;
        closed_ping_count?: number;
        verified_count?: number;
        resolved_count?: number;
      }>("/api/v1/alerts/verify-all", { method: "POST", json: {}, timeoutMs: 15 * 60_000 });
      await active.refetch();
      await incidents.refetch();
      return res;
    },
    onSuccess: (res) => {
      void invalidateAlertListQueries(qc);
      const verified = res?.verified_count ?? 0;
      const resolved = res?.resolved_count ?? 0;
      pushToast({
        tone: "ok",
        text: `Verificação concluída: ${verified} alerta(s) reavaliado(s), ${resolved} normalizado(s).`,
      });
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
    mutationFn: (alertId: string) =>
      apiFetch<VerifyResult>(`/api/v1/alerts/${alertId}/verify`, { method: "POST", json: {}, timeoutMs: 5 * 60_000 }),
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

  const groupedActive = useMemo(() => {
    const map = new Map<string, ActiveAlert[]>();
    for (const a of filteredActive) {
      const key = String(a.type || "").trim() || "_other";
      const list = map.get(key);
      if (list) list.push(a);
      else map.set(key, [a]);
    }
    return [...map.entries()]
      .map(([type, items]) => ({
        type,
        title: alertProblemTitle(type === "_other" ? "" : type),
        items,
      }))
      .sort((a, b) => a.title.localeCompare(b.title, "pt"));
  }, [filteredActive]);


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

  const ignoredRows = ignoredQ.data?.ignored ?? [];
  const ignoredDeviceOptions = useMemo(() => {
    const names = new Set<string>();
    for (const row of ignoredRows) {
      const label = alertEquipmentPrimary(row.type, row.device_name ?? null, row.message ?? "", row.meta).trim();
      if (label && label !== "-") names.add(label);
      else if (row.device_name?.trim()) names.add(row.device_name.trim());
    }
    return [...names].sort((a, b) => a.localeCompare(b, "pt"));
  }, [ignoredRows]);

  const filteredIgnored = useMemo(() => {
    const q = ignoredSearch.trim().toLowerCase();
    return ignoredRows.filter((row) => {
      const equip = alertEquipmentPrimary(row.type, row.device_name ?? null, row.message ?? "", row.meta).trim();
      if (ignoredDevice && equip !== ignoredDevice && (row.device_name ?? "").trim() !== ignoredDevice) {
        return false;
      }
      if (!q) return true;
      const hay = [
        row.message ?? "",
        row.type,
        row.severity ?? "",
        row.device_name ?? "",
        row.ip ?? "",
        equip,
        alertProblemTitle(row.type),
      ]
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    });
  }, [ignoredRows, ignoredSearch, ignoredDevice]);

  function openIgnoredModal() {
    setIgnoredSearch("");
    setIgnoredDevice("");
    setIgnoredOpen(true);
  }


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
          <div className="alerts-toolbar" ref={toolbarFiltersRef}>
            <input
              className="input alerts-toolbar__search"
              aria-label="Pesquisar alertas"
              placeholder="Pesquisar equipamento, IP, tipo de problema…"
              value={searchActive}
              onChange={(e) => setSearchActive(e.target.value)}
            />
            <div className="alerts-toolbar__actions">
              <button
                type="button"
                className="btn btn--icon btn--icon-menu"
                disabled={verifyAll.isPending}
                title="Verificar alertas"
                aria-label="Verificar alertas"
                onClick={() => setConfirmVerifyAll(true)}
              >
                {verifyAll.isPending ? <Loader2 size={18} className="map-refresh-spin" aria-hidden /> : <ShieldAlert size={18} aria-hidden />}
              </button>

              <div className="alerts-toolbar__menu-wrap">
                <button
                  type="button"
                  className={`btn btn--icon btn--icon-menu${sev ? " btn--filter-active" : ""}${sevMenuOpen ? " btn--primary" : ""}`}
                  title="Severidade"
                  aria-label="Filtrar por severidade"
                  aria-expanded={sevMenuOpen}
                  onClick={() => {
                    setSevMenuOpen((o) => !o);
                    setTypMenuOpen(false);
                  }}
                >
                  <FileExclamationPoint size={18} aria-hidden />
                </button>
                {sevMenuOpen ? (
                  <div className="alerts-toolbar__menu" role="listbox" aria-label="Severidade">
                    {ALERT_SEVERITY_FILTER_OPTIONS.map((o) => (
                      <button
                        key={o.value || "all-sev"}
                        type="button"
                        className={`alerts-toolbar__menu-item${sev === o.value ? " is-active" : ""}`}
                        onClick={() => {
                          setSev(o.value);
                          setSevMenuOpen(false);
                        }}
                      >
                        {o.label}
                      </button>
                    ))}
                  </div>
                ) : null}
              </div>

              <div className="alerts-toolbar__menu-wrap">
                <button
                  type="button"
                  className={`btn btn--icon btn--icon-menu${typ ? " btn--filter-active" : ""}${typMenuOpen ? " btn--primary" : ""}`}
                  title="Tipo"
                  aria-label="Filtrar por tipo"
                  aria-expanded={typMenuOpen}
                  onClick={() => {
                    setTypMenuOpen((o) => !o);
                    setSevMenuOpen(false);
                  }}
                >
                  <LayoutGrid size={18} aria-hidden />
                </button>
                {typMenuOpen ? (
                  <div className="alerts-toolbar__menu alerts-toolbar__menu--wide alerts-toolbar__menu--end" role="listbox" aria-label="Tipo">
                    {ALERT_TYPE_FILTER_OPTIONS.map((o) => (
                      <button
                        key={o.value || "all"}
                        type="button"
                        className={`alerts-toolbar__menu-item${typ === o.value ? " is-active" : ""}`}
                        onClick={() => {
                          setTyp(o.value);
                          setTypMenuOpen(false);
                        }}
                      >
                        {o.label}
                      </button>
                    ))}
                  </div>
                ) : null}
              </div>

              <button
                type="button"
                className={`btn btn--icon btn--icon-menu${groupByType ? " btn--primary" : ""}`}
                title={groupByType ? "Categorizar: ligado (lista separada por tipo)" : "Categorizar: desligado (lista unificada)"}
                aria-label="Categorizar alertas por tipo"
                aria-pressed={groupByType}
                onClick={() => setGroupByType((v) => !v)}
              >
                <SquareStack size={18} aria-hidden />
              </button>

              <button
                type="button"
                className="btn btn--icon btn--icon-menu"
                title="Alertas ignorados"
                aria-label="Alertas ignorados"
                onClick={openIgnoredModal}
              >
                <MessageCircleX size={18} aria-hidden />
              </button>
            </div>
          </div>
          {verifyAll.isError && <div className="msg msg--err margin-bottom mb-12">{(verifyAll.error as Error).message}</div>}
          {verifyAll.isPending ? (
            <div className="msg" style={{ marginBottom: 12, display: "flex", alignItems: "center", gap: 8 }}>
              <Loader2 size={18} className="map-refresh-spin" aria-hidden />
              A recolectar dados de cada alerta activo (ping, interfaces, OLT, BNG…). Isto pode demorar alguns minutos.
            </div>
          ) : null}

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
                        <th>POP</th>
                        <th>Estado</th>
                        <th style={{ width: 48 }} />
                      </tr>
                    </thead>
                    <tbody>
                      {(groupByType
                        ? groupedActive.flatMap((g) => [
                            {
                              kind: "group" as const,
                              key: `g-${g.type}`,
                              title: g.title,
                              count: g.items.length,
                            },
                            ...g.items.map((a) => ({ kind: "row" as const, key: a.id, alert: a })),
                          ])
                        : filteredActive.map((a) => ({ kind: "row" as const, key: a.id, alert: a }))
                      ).map((entry) => {
                        if (entry.kind === "group") {
                          return (
                            <tr key={entry.key} className="alerts-group-row">
                              <td colSpan={9}>
                                <span className="alerts-group-row__title">{entry.title}</span>
                                <span className="alerts-group-row__count">{entry.count}</span>
                              </td>
                            </tr>
                          );
                        }
                        const a = entry.alert;
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
                            <td style={{ fontSize: 12 }}>{formatAlertPopName(a.pop_name, a.meta)}</td>
                            <td>
                              {resolved ? (
                                <div>
                                  <span className="alerts-status-pill alerts-status-pill--resolved">✓ Resolvido</span>
                                  <div style={{ fontSize: 10, color: "var(--muted)", marginTop: 4, lineHeight: 1.45 }}>
                                    <div>Início: {formatAlertDateTimePt(a.active_since)}</div>
                                    <div>Fim: {formatAlertDateTimePt(a.closed_at!)}</div>
                                    <div>Duração: {formatAlertDuration(a.active_since, a.closed_at)}</div>
                                    <div>Normalizado: {formatAlertResolvedValue(a.type, a.meta, a.message)}</div>
                                  </div>
                                </div>
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
            Sem datas: mostra os eventos mais recentes (limite no servidor). Com datas, lista alertas que
            <strong> abriram ou fecharam</strong> dentro do intervalo (hora local convertida para UTC).
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
                        <th>Início</th>
                        <th>Fim</th>
                        <th>Duração</th>
                        <th>POP</th>
                        <th>Severidade</th>
                        <th>Problema</th>
                        <th>Valor normalizado</th>
                        <th>Equipamento</th>
                        <th>Estado</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredHistory.map((e) => (
                        <tr key={e.id}>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{formatAlertDateTimePt(e.active_since)}</td>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{e.closed_at ? formatAlertDateTimePt(e.closed_at) : "—"}</td>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{formatAlertDuration(e.active_since, e.closed_at)}</td>
                          <td style={{ fontSize: 12 }}>{formatAlertPopName(e.pop_name, e.meta)}</td>
                          <td>
                            <span className={severityPillClass(e.severity)}>{displaySeverity(e.severity)}</span>
                          </td>
                          <td className="alerts-problem">{alertProblemTitle(e.type)}</td>
                          <td className="alerts-msg mono" style={{ fontSize: 12 }}>
                            {e.closed_at ? formatAlertResolvedValue(e.type, e.meta, e.message) : "—"}
                          </td>
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

      {ignoredOpen
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={() => setIgnoredOpen(false)}>
              <div
                className="modal modal--wide ignored-alerts-modal"
                role="dialog"
                aria-modal="true"
                aria-labelledby="ignored-alerts-title"
                onMouseDown={(e) => e.stopPropagation()}
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
            <div className="ignored-alerts-toolbar">
              <input
                className="input ignored-alerts-toolbar__search"
                aria-label="Pesquisar alertas ignorados"
                placeholder="Pesquisar problema, equipamento, IP…"
                value={ignoredSearch}
                onChange={(e) => setIgnoredSearch(e.target.value)}
              />
              <select
                className="input ignored-alerts-toolbar__device"
                aria-label="Filtrar por equipamento"
                value={ignoredDevice}
                onChange={(e) => setIgnoredDevice(e.target.value)}
              >
                <option value="">Todos os equipamentos</option>
                {ignoredDeviceOptions.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            </div>
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
                    {filteredIgnored.map((row) => (
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
                            onClick={() => setReactivateId(row.id)}
                          >
                            Reactivar
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {filteredIgnored.length === 0 ? (
                  <p style={{ padding: 16, color: "var(--muted)" }}>
                    {(ignoredQ.data.ignored ?? []).length === 0 ? "Nenhum alerta ignorado." : "Nenhum resultado com este filtro."}
                  </p>
                ) : null}
              </div>
            ) : null}
              </div>
            </div>,
            document.body,
          )
        : null}

      <ConfirmModal
        open={confirmVerifyAll}
        title="Verificar todos os alertas?"
        message="O sistema vai recolectar dados ao vivo (ping, interfaces, OLT, BNG…) e reavaliar cada alerta activo. Isto pode demorar alguns minutos."
        confirmLabel="Verificar"
        cancelLabel="Cancelar"
        busy={verifyAll.isPending}
        onCancel={() => setConfirmVerifyAll(false)}
        onConfirm={() => {
          setConfirmVerifyAll(false);
          verifyAll.mutate();
        }}
      />

      <ConfirmModal
        open={reactivateId != null}
        title="Reactivar alerta ignorado?"
        message="Este padrão voltará a gerar alertas na lista e notificações Telegram quando a condição ocorrer."
        confirmLabel="Reactivar"
        cancelLabel="Cancelar"
        busy={reactivateMut.isPending}
        onCancel={() => setReactivateId(null)}
        onConfirm={() => {
          if (!reactivateId) return;
          const id = reactivateId;
          setReactivateId(null);
          reactivateMut.mutate(id);
        }}
      />
    </div>
  );
}
