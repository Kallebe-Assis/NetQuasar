import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, CircleAlert, Flame, Info, LayoutGrid, Loader2, MessageCircleX, Search, ShieldAlert, SquareStack, TriangleAlert } from "lucide-react";
import { ActionMenu } from "../components/ActionMenu";
import { AffectedClientsModal } from "../components/AffectedClientsModal";
import { ConfirmModal } from "../components/ConfirmModal";
import { InfoHint } from "../components/InfoHint";
import { EmptyState } from "../components/EmptyState";
import { ExportMenu } from "../components/ExportMenu";
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
import { toastErr, toastOk } from "../lib/operationToast";
import {
  classifyOnuRx,
  ONU_RX_QUALITY_COLOR,
  ONU_RX_QUALITY_LABEL,
  useOnuRxThresholds,
} from "../lib/onuRxQuality";

type OnuMonitor = {
  id: string;
  serial: string;
  olt_description: string;
  pon?: number | null;
  onu?: number | null;
  client_name: string;
  watch_status: boolean;
  watch_rx: boolean;
  watch_login: boolean;
  bng_login: string;
  notify_telegram: boolean;
  expires_at?: string | null;
  created_by: string;
  created_at: string;
  current_online?: boolean | null;
  current_rx_dbm?: number | null;
  current_rx_class?: string | null;
  login_online?: boolean | null;
  login_bng?: string | null;
};

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
  /** Nome que o usuário deu à interface no cadastro do equipamento (aba Interfaces). */
  interface_custom_name?: string | null;
  /** Categoria do equipamento (ex.: "OLT") — só usado para decidir se mostra «Clientes afetados». */
  device_category?: string | null;
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
  interface_custom_name?: string | null;
};

type HistoryResponse = { events: HistoryEvent[]; total: number; page: number; page_size: number };

/** Itens por página do histórico (paginação feita no servidor). */
const HISTORY_PAGE_SIZE = 50;

const SEVERITY_RANK: Record<string, number> = { critical: 3, warning: 2, info: 1 };
type SevKey = "critical" | "warning" | "info";
const SEV_CHIPS: { value: SevKey; label: string; Icon: typeof Flame }[] = [
  { value: "critical", label: "Crítico", Icon: Flame },
  { value: "warning", label: "Atenção", Icon: TriangleAlert },
  { value: "info", label: "Informativo", Icon: Info },
];
const GROUP_ICON: Record<SevKey, typeof Flame> = { critical: TriangleAlert, warning: CircleAlert, info: TriangleAlert };

function sevKeyOf(raw: string | null | undefined): SevKey {
  const k = String(raw ?? "").trim().toLowerCase();
  return k === "critical" || k === "warning" ? k : "info";
}

/** Recarrega alertas periodicamente — mesma instância pode ter message/meta novos (ex.: latência 243→210). */
const ALERTS_ACTIVE_REFRESH_MS = 2_500;
const ALERTS_HISTORY_REFRESH_MS = 45_000;

export function AlertsPage() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [tab, setTab] = useState<"active" | "hist" | "onus">("active");
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
  const [histPage, setHistPage] = useState(1);
  const [histSearch, setHistSearch] = useState("");
  /** Texto do histórico já «assentado» (evita uma consulta a cada tecla). */
  const [histQ, setHistQ] = useState("");
  const [histFrom, setHistFrom] = useState("");
  const [histTo, setHistTo] = useState("");
  const [searchActive, setSearchActive] = useState("");
  /** Agrupa a lista por tipo de alerta (offline, latência, SFP…). Ligado por defeito. */
  const [groupByType, setGroupByType] = useState(true);
  const [typMenuOpen, setTypMenuOpen] = useState(false);
  const toolbarFiltersRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (tab !== "active") return;
    const id = window.setInterval(() => setAgoTick((n) => n + 1), Math.max(ALERTS_ACTIVE_REFRESH_MS, 4000));
    return () => window.clearInterval(id);
  }, [tab]);

  useEffect(() => {
    if (!typMenuOpen) return;
    const onDoc = (ev: MouseEvent) => {
      const el = toolbarFiltersRef.current;
      if (el && !el.contains(ev.target as Node)) setTypMenuOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [typMenuOpen]);

  useEffect(() => {
    const id = window.setTimeout(() => {
      setHistQ(histSearch.trim());
      setHistPage(1);
    }, 350);
    return () => window.clearTimeout(id);
  }, [histSearch]);

  const monState = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<MonitoringStateSync>("/api/v1/monitoring/state"),
    staleTime: 1000,
  });

  const { thresholds: rxThresholds } = useOnuRxThresholds();
  const onuMonitors = useQuery({
    queryKey: ["onu-monitors"],
    queryFn: () => apiFetch<{ monitors: OnuMonitor[] }>("/api/v1/olt/onu-monitors"),
    enabled: tab === "onus",
    refetchInterval: tab === "onus" ? 30_000 : false,
  });
  const removeMonitor = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/olt/onu-monitors/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["onu-monitors"] });
      toastOk(pushToast, "Monitoramento da ONU removido.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao remover monitoramento."),
  });

  useMonitoringLiveSync(monState.data, { monitoring: false, alerts: true, olt: false });

  const incidents = useQuery({
    queryKey: queryKeys.alertsIncidents,
    queryFn: () => apiFetch<{ incidents: OpenIncident[] }>("/api/v1/alerts/incidents/active"),
    refetchInterval: ALERTS_ACTIVE_REFRESH_MS,
    enabled: tab === "active",
  });

  const active = useQuery({
    queryKey: [...queryKeys.alertsActive, typ, limitActive],
    queryFn: () => {
      const p = new URLSearchParams();
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
    queryKey: ["alerts-hist", HISTORY_PAGE_SIZE, histPage, histQ, histFrom, histTo],
    queryFn: () => {
      const p = new URLSearchParams({ page: String(histPage), page_size: String(HISTORY_PAGE_SIZE) });
      const from = histFrom.trim();
      const to = histTo.trim();
      if (from && to) {
        p.set("from", new Date(from).toISOString());
        p.set("to", new Date(to).toISOString());
      }
      if (histQ) p.set("q", histQ);
      return apiFetch<HistoryResponse>(`/api/v1/alerts/history?${p}`);
    },
    enabled: tab === "hist",
    placeholderData: keepPreviousData,
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

  // "Clientes afetados" (3 pontinhos, só para OLT offline/PON DOWN) — abre um modal com a lista
  // de ONUs afetadas que têm cliente vinculado (ver AffectedClientsModal), com opção de exportar
  // CSV e/ou enviar por Telegram a partir de lá.
  const [affectedClientsAlertId, setAffectedClientsAlertId] = useState<string | null>(null);

  // "Clientes afetados": OLT offline (ping_unreachable numa OLT) ou PON DOWN — nos dois casos há
  // um conjunto de ONUs claramente identificável (a OLT inteira, ou só a PON) para procurar
  // clientes vinculados (ver alertAffectedClientsTelegram no backend).
  function showAffectedClientsButton(a: ActiveAlert): boolean {
    if (a.type === "pon_down") return true;
    return a.type === "ping_unreachable" && (a.device_category ?? "").trim().toLowerCase() === "olt";
  }

  const rawAlerts = active.data?.alerts ?? [];
  const sevCounts = useMemo(() => {
    const c: Record<SevKey, number> = { critical: 0, warning: 0, info: 0 };
    for (const a of rawAlerts) if (!a.closed_at) c[sevKeyOf(a.severity)] += 1;
    return c;
  }, [rawAlerts]);
  const openCount = sevCounts.critical + sevCounts.warning + sevCounts.info;

  const filteredActive = useMemo(() => {
    const q = searchActive.trim().toLowerCase();
    return rawAlerts.filter((a) => {
      if (sev && sevKeyOf(a.severity) !== sev) return false;
      if (!q) return true;
      const hay = [
        a.device_name,
        a.ip,
        a.interface_custom_name,
        a.message,
        a.type,
        formatAlertPopName(a.pop_name, a.meta),
        displaySeverity(a.severity),
        displayActiveRowSeverity(a.severity, a.closed_at ?? null),
        alertProblemTitle(a.type, a.meta),
      ]
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    });
  }, [rawAlerts, searchActive, sev]);

  const groupedActive = useMemo(() => {
    const map = new Map<string, ActiveAlert[]>();
    for (const a of filteredActive) {
      const key = String(a.type || "").trim() || "_other";
      const list = map.get(key);
      if (list) list.push(a);
      else map.set(key, [a]);
    }
    return [...map.entries()]
      .map(([type, items]) => {
        // a cor do cabeçalho do grupo é a da pior severidade entre os alertas dele
        let worst: SevKey = "info";
        for (const a of items) if ((SEVERITY_RANK[sevKeyOf(a.severity)] ?? 0) > (SEVERITY_RANK[worst] ?? 0)) worst = sevKeyOf(a.severity);
        return { type, title: alertProblemTitle(type === "_other" ? "" : type), items, sev: worst };
      })
      .sort((a, b) => (SEVERITY_RANK[b.sev] ?? 0) - (SEVERITY_RANK[a.sev] ?? 0) || a.title.localeCompare(b.title, "pt"));
  }, [filteredActive]);

  // histórico: a página já vem filtrada e paginada do servidor
  /** Todos os eventos do filtro atual (a tela pagina de 50 em 50; a exportação traz até 5000, os mais recentes). */
  async function loadHistoryForExport(): Promise<HistoryEvent[]> {
    const p = new URLSearchParams({ limit: "5000" });
    const from = histFrom.trim();
    const to = histTo.trim();
    if (from && to) {
      p.set("from", new Date(from).toISOString());
      p.set("to", new Date(to).toISOString());
    }
    if (histQ) p.set("q", histQ);
    const r = await apiFetch<{ events: HistoryEvent[] }>(`/api/v1/alerts/history?${p}`);
    return r.events ?? [];
  }
  const historyEvents = hist.data?.events ?? [];
  const histTotal = hist.data?.total ?? 0;
  const histPages = Math.max(1, Math.ceil(histTotal / HISTORY_PAGE_SIZE));
  const histFirst = histTotal === 0 ? 0 : (histPage - 1) * HISTORY_PAGE_SIZE + 1;
  const histLast = Math.min(histTotal, histPage * HISTORY_PAGE_SIZE);

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
        alertProblemTitle(row.type, row.meta),
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
              Valores (latência, dBm, etc.) atualizam quando o worker grava na BD; renovação automática a cada ~
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
        <button type="button" className={tab === "onus" ? "active" : ""} onClick={() => setTab("onus")}>
          ONUs
          {(onuMonitors.data?.monitors.length ?? 0) > 0 ? ` (${onuMonitors.data!.monitors.length})` : ""}
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
          {verifyAll.isError && <div className="msg msg--err margin-bottom mb-12">{(verifyAll.error as Error).message}</div>}
          {verifyAll.isPending ? (
            <div className="msg" style={{ marginBottom: 12, display: "flex", alignItems: "center", gap: 8 }}>
              <Loader2 size={18} className="map-refresh-spin" aria-hidden />
              A recolectar dados de cada alerta activo (ping, interfaces, OLT, BNG…). Isto pode demorar alguns minutos.
            </div>
          ) : null}

          <div className="alerts-panel alerts-panel--cards">
            <div className="alerts-panel__head">
              <div className="alerts-panel__title">
                <strong>Lista de alertas</strong>
                <span className={`alerts-count-pill${openCount > 0 ? " alerts-count-pill--on" : ""}`}>
                  {openCount} {openCount === 1 ? "alerta ativo" : "alertas ativos"}
                </span>
              </div>
              <div className="alerts-toolbar__actions" ref={toolbarFiltersRef}>
                <ExportMenu
                  filename="alertas-ativos"
                  title="Alertas ativos"
                  rows={filteredActive}
                  columns={[
                    { header: "Início", value: (a) => formatAlertDateTimePt(a.active_since) },
                    { header: "Normalizado em", value: (a) => (a.closed_at ? formatAlertDateTimePt(a.closed_at) : "") },
                    { header: "Severidade", value: (a) => displayActiveRowSeverity(a.severity, a.closed_at ?? null) },
                    { header: "Categoria", value: (a) => alertCategoryLabel(alertCategoryFromType(a.type)) },
                    { header: "Problema", value: (a) => alertProblemTitle(a.type, a.meta) },
                    { header: "Interface", value: (a) => a.interface_custom_name },
                    { header: "Valor", value: (a) => alertValueText(a.type, a.message, a.meta) },
                    { header: "Equipamento", value: (a) => alertEquipmentPrimary(a.type, a.device_name, a.message, a.meta) },
                    { header: "IP", value: (a) => a.ip },
                    { header: "POP", value: (a) => formatAlertPopName(a.pop_name, a.meta) },
                    { header: "Mensagem", value: (a) => a.message },
                  ]}
                />
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
                    className={`btn btn--icon btn--icon-menu${typ ? " btn--filter-active" : ""}${typMenuOpen ? " btn--primary" : ""}`}
                    title="Tipo"
                    aria-label="Filtrar por tipo"
                    aria-expanded={typMenuOpen}
                    onClick={() => setTypMenuOpen((o) => !o)}
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

            <div className="alerts-filterbar">
              <span className="alerts-filterbar__label">Filtrar por</span>
              <div className="alerts-sev-chips" role="group" aria-label="Severidade">
                {SEV_CHIPS.map(({ value, label, Icon }) => (
                  <button
                    key={value}
                    type="button"
                    className={`alerts-sev-chip alerts-sev-chip--${value}${sev === value ? " is-active" : ""}`}
                    aria-pressed={sev === value}
                    title={sev === value ? "Clique para remover o filtro" : `Mostrar só ${label.toLowerCase()}`}
                    onClick={() => setSev(sev === value ? "" : value)}
                  >
                    <Icon size={13} aria-hidden />
                    {label}
                    <span className="alerts-sev-chip__n">{sevCounts[value]}</span>
                  </button>
                ))}
              </div>
              <label className="alerts-filterbar__search">
                <Search size={14} aria-hidden />
                <input
                  className="input"
                  aria-label="Pesquisar alertas"
                  placeholder="Filtrar por equipamento, POP ou problema…"
                  value={searchActive}
                  onChange={(e) => setSearchActive(e.target.value)}
                />
              </label>
            </div>

            {active.isLoading && <p style={{ padding: 16 }}>A carregar…</p>}
            {active.isError && <div className="msg msg--err margin m-14">{(active.error as Error).message}</div>}
            {!active.isLoading && active.data && (
              <>
                <div className="alerts-table-wrap">
                  <table className="alerts-table">
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
                              sev: g.sev,
                            },
                            ...g.items.map((a) => ({ kind: "row" as const, key: a.id, alert: a })),
                          ])
                        : filteredActive.map((a) => ({ kind: "row" as const, key: a.id, alert: a }))
                      ).map((entry) => {
                        if (entry.kind === "group") {
                          const GroupIcon = GROUP_ICON[entry.sev];
                          return (
                            <tr key={entry.key} className={`alerts-group-row alerts-group-row--${entry.sev}`}>
                              <td colSpan={9}>
                                <GroupIcon size={15} className="alerts-group-row__icon" aria-hidden />
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
                          <tr key={a.id} className="alerts-row">
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
                            <td className="alerts-problem">
                              {alertProblemTitle(a.type, a.meta)}
                              {a.interface_custom_name ? <div className="alerts-problem__iface">Interface: {a.interface_custom_name}</div> : null}
                            </td>
                            <td className="alerts-msg">{alertValueText(a.type, a.message, a.meta)}</td>
                            <td>
                              <div className="alerts-dev">
                                <span className="alerts-dev__name">{alertEquipmentPrimary(a.type, a.device_name, a.message, a.meta)}</span>
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
                                <span className="alerts-status-pill alerts-status-pill--active">● Ativo</span>
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
                                    ...(showAffectedClientsButton(a)
                                      ? [
                                          {
                                            id: "affected-clients",
                                            label: "Clientes afetados",
                                            onClick: () => setAffectedClientsAlertId(a.id),
                                          },
                                        ]
                                      : []),
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
          <div className="alerts-toolbar alerts-toolbar--hist">
            <div className="field alerts-toolbar__search alerts-toolbar__search--wide" style={{ margin: 0 }}>
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
            <button
              type="button"
              className="btn btn--primary"
              disabled={!histFrom.trim() || !histTo.trim()}
              onClick={() => {
                setHistPage(1);
                void hist.refetch();
              }}
            >
              Aplicar datas
            </button>
            <button
              type="button"
              className="btn"
              onClick={() => {
                flushSync(() => {
                  setHistFrom("");
                  setHistTo("");
                  setHistPage(1);
                });
                void hist.refetch();
              }}
            >
              Limpar datas
            </button>
            <button type="button" className="btn" onClick={() => void hist.refetch()}>
              Atualizar
            </button>
          </div>
          <p style={{ color: "var(--muted)", fontSize: 12, marginTop: -4 }}>
            Sem datas: mostra os eventos mais recentes, {HISTORY_PAGE_SIZE} por página. Com datas, lista alertas que
            <strong> abriram ou fecharam</strong> dentro do intervalo (hora local convertida para UTC).
          </p>

          <div className="alerts-panel alerts-panel--cards">
            <div className="alerts-panel__head">
              <div className="alerts-panel__title">
                <strong>Histórico</strong>
                {hist.data ? <span className="alerts-count-pill">{histTotal.toLocaleString("pt-BR")} evento(s)</span> : null}
                <ExportMenu
                  filename="alertas-historico"
                  title="Histórico de alertas"
                  rows={loadHistoryForExport}
                  columns={[
                    { header: "Início", value: (e) => formatAlertDateTimePt(e.active_since) },
                    { header: "Fim", value: (e) => (e.closed_at ? formatAlertDateTimePt(e.closed_at) : "") },
                    { header: "Duração", value: (e) => formatAlertDuration(e.active_since, e.closed_at) },
                    { header: "POP", value: (e) => formatAlertPopName(e.pop_name, e.meta) },
                    { header: "Severidade", value: (e) => displaySeverity(e.severity) },
                    { header: "Problema", value: (e) => alertProblemTitle(e.type, e.meta) },
                    { header: "Interface", value: (e) => e.interface_custom_name },
                    { header: "Valor normalizado", value: (e) => (e.closed_at ? formatAlertResolvedValue(e.type, e.meta, e.message) : "") },
                    { header: "Equipamento", value: (e) => alertEquipmentPrimary(e.type, e.device_name ?? null, e.message, e.meta) },
                    { header: "IP", value: (e) => e.ip },
                    { header: "Estado", value: (e) => (e.closed_at ? "Resolvido" : "Em aberto") },
                    { header: "Mensagem", value: (e) => e.message },
                  ]}
                />
              </div>
              {hist.data ? (
              <nav className="alerts-pager" aria-label="Paginação do histórico">
                <button type="button" className="btn btn--sm" disabled={histPage <= 1} onClick={() => setHistPage(1)} aria-label="Primeira página">
                  «
                </button>
                <button type="button" className="btn btn--sm" disabled={histPage <= 1} onClick={() => setHistPage((p) => Math.max(1, p - 1))}>
                  <ChevronLeft size={14} aria-hidden /> Anterior
                </button>
                <span className="alerts-pager__info">
                  Página {histPage} de {histPages}
                </span>
                <button type="button" className="btn btn--sm" disabled={histPage >= histPages} onClick={() => setHistPage((p) => Math.min(histPages, p + 1))}>
                  Próxima <ChevronRight size={14} aria-hidden />
                </button>
                <button type="button" className="btn btn--sm" disabled={histPage >= histPages} onClick={() => setHistPage(histPages)} aria-label="Última página">
                  »
                </button>
              </nav>
              ) : null}
            </div>
            {hist.isLoading && <p style={{ padding: 16 }}>A carregar…</p>}
            {hist.isError && <div className="msg msg--err margin m-14">{(hist.error as Error).message}</div>}
            {hist.data && (
              <>
                <div className="alerts-table-wrap" style={{ opacity: hist.isPlaceholderData ? 0.6 : 1 }}>
                  <table className="alerts-table">
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
                      {historyEvents.map((e) => (
                        <tr key={e.id} className="alerts-row">
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{formatAlertDateTimePt(e.active_since)}</td>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{e.closed_at ? formatAlertDateTimePt(e.closed_at) : "—"}</td>
                          <td style={{ fontSize: 12, whiteSpace: "nowrap" }}>{formatAlertDuration(e.active_since, e.closed_at)}</td>
                          <td style={{ fontSize: 12 }}>{formatAlertPopName(e.pop_name, e.meta)}</td>
                          <td>
                            <span className={severityPillClass(e.severity)}>{displaySeverity(e.severity)}</span>
                          </td>
                          <td className="alerts-problem">
                            {alertProblemTitle(e.type, e.meta)}
                            {e.interface_custom_name ? <div className="alerts-problem__iface">Interface: {e.interface_custom_name}</div> : null}
                          </td>
                          <td className="alerts-msg mono" style={{ fontSize: 12 }}>
                            {e.closed_at ? formatAlertResolvedValue(e.type, e.meta, e.message) : "—"}
                          </td>
                          <td>
                            <div className="alerts-dev">
                              <span className="alerts-dev__name">{alertEquipmentPrimary(e.type, e.device_name ?? null, e.message, e.meta)}</span>
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
                {historyEvents.length === 0 && <p style={{ padding: 16, color: "var(--muted)", margin: 0 }}>Nenhum evento com este filtro.</p>}
                <div className="alerts-panel__foot">
                  <span>
                    {histTotal === 0 ? "0 eventos" : `Mostrando ${histFirst.toLocaleString("pt-BR")}–${histLast.toLocaleString("pt-BR")} de ${histTotal.toLocaleString("pt-BR")}`}
                  </span>
                </div>
              </>
            )}
          </div>
        </>
      )}

      {tab === "onus" && (
        <div className="card">
          <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
            ONUs colocadas em monitoramento manual/temporário (tela OLT → ONUs → menu ⋮ → <strong>Monitorar</strong>). Cada
            uma alarma quando o item degrada e quando normaliza — os alertas de degradação e as normalizações também
            aparecem em <strong>Ativos</strong> / <strong>Histórico</strong> e no Telegram.
          </p>
          {onuMonitors.isLoading ? (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p>
          ) : (onuMonitors.data?.monitors.length ?? 0) === 0 ? (
            <EmptyState
              title="Nenhuma ONU em monitoramento."
              hint='Na tela OLT → aba ONUs, abra o menu de opções (⋮) de uma ONU e escolha "Monitorar".'
            />
          ) : (
            <div className="table-wrap">
              <table style={{ fontSize: 12, width: "100%" }}>
                <thead>
                  <tr>
                    <th>Cliente</th>
                    <th>Serial</th>
                    <th>OLT · PON/ONU</th>
                    <th>Status</th>
                    <th>RX</th>
                    <th>Login BNG</th>
                    <th>Monitorando</th>
                    <th>Expira</th>
                    <th style={{ width: 44 }} />
                  </tr>
                </thead>
                <tbody>
                  {onuMonitors.data!.monitors.map((m) => {
                    const rxCls =
                      typeof m.current_rx_dbm === "number" ? classifyOnuRx(m.current_rx_dbm, rxThresholds) : null;
                    return (
                      <tr key={m.id}>
                        <td>{m.client_name || <span style={{ color: "var(--muted)" }}>—</span>}</td>
                        <td className="mono">{m.serial}</td>
                        <td>
                          {m.olt_description || "—"}
                          <span style={{ color: "var(--muted)" }}> · {m.pon ?? "?"}/{m.onu ?? "?"}</span>
                        </td>
                        <td>
                          {m.current_online === true ? (
                            <span className="badge badge--ok">Online</span>
                          ) : m.current_online === false ? (
                            <span className="badge badge--err">Offline</span>
                          ) : (
                            <span style={{ color: "var(--muted)" }}>—</span>
                          )}
                        </td>
                        <td className="mono">
                          {typeof m.current_rx_dbm === "number" ? (
                            <span style={{ color: rxCls ? ONU_RX_QUALITY_COLOR[rxCls] : undefined }}>
                              {m.current_rx_dbm.toFixed(2)}
                              {rxCls ? ` · ${ONU_RX_QUALITY_LABEL[rxCls]}` : ""}
                            </span>
                          ) : (
                            <span style={{ color: "var(--muted)" }}>—</span>
                          )}
                        </td>
                        <td>
                          {m.watch_login && m.bng_login ? (
                            <span>
                              <span className="mono">{m.bng_login}</span>{" "}
                              {m.login_online === true ? (
                                <span className="badge badge--ok">online</span>
                              ) : m.login_online === false ? (
                                <span className="badge badge--err">offline</span>
                              ) : (
                                <span style={{ color: "var(--muted)" }}>—</span>
                              )}
                            </span>
                          ) : (
                            <span style={{ color: "var(--muted)" }}>—</span>
                          )}
                        </td>
                        <td style={{ fontSize: 11 }}>
                          {[
                            m.watch_status ? "Status" : null,
                            m.watch_rx ? "RX" : null,
                            m.watch_login ? "Login" : null,
                            m.notify_telegram ? "Telegram" : null,
                          ]
                            .filter(Boolean)
                            .join(" · ")}
                        </td>
                        <td style={{ fontSize: 11, whiteSpace: "nowrap" }}>
                          {m.expires_at ? formatAlertDateTimePt(m.expires_at) : "manual"}
                        </td>
                        <td>
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Remover monitoramento"
                            aria-label="Remover monitoramento"
                            disabled={removeMonitor.isPending}
                            onClick={() => removeMonitor.mutate(m.id)}
                          >
                            ✕
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
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
                        <td className="alerts-problem">{alertProblemTitle(row.type, row.meta)}</td>
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

      <AffectedClientsModal
        open={affectedClientsAlertId != null}
        alertId={affectedClientsAlertId}
        onClose={() => setAffectedClientsAlertId(null)}
      />
    </div>
  );
}
