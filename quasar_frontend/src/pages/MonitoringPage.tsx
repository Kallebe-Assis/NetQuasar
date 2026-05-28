import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { PageCountPill } from "../components/PageCountPill";
import { DeviceReportModal, type DeviceReportTarget } from "../components/DeviceReportModal";
import { InfoHint } from "../components/InfoHint";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { EM_DASH } from "../lib/formatDisplay";
import { prettyAuditDiff } from "../lib/auditDisplay";
import { useAppToast } from "../lib/appToast";
import { queryKeys } from "../lib/queryKeys";

type ActiveEquipRow = {
  id: string;
  description: string;
  category: string;
  brand?: string;
  ip: string;
  checked_at?: string | null;
  latency_ms?: number | null;
  probe_ok?: boolean | null;
  ping_reachable?: boolean | null;
  cpu_percent?: number | null;
  memory_percent?: number | null;
  uptime?: string | null;
  temperature_c?: number | null;
  metrics_note?: string;
};

type OfflineAlertRow = {
  id: string;
  severity: string;
  type: string;
  message: string;
  ip: string;
  device_name: string;
  active_since: string;
};

type NetCheck = {
  ok: boolean;
  checked_at: string;
  targets_tried: { url: string; ok: boolean; latency_ms: number; error?: string }[];
  latency_ms?: number;
  error_code?: string;
  error_detail?: string;
};

type MonState = {
  is_running: boolean;
  monitoring_mode: string;
  last_started_at?: string | null;
  last_stopped_at?: string | null;
  last_internet_check_at?: string | null;
  last_internet_check_ok?: boolean | null;
  /** Última escrita em monitoring_runtime (ligar/desligar, ciclo, atividade) — útil para vários clientes. */
  runtime_updated_at?: string | null;
};
type NightlyCfg = {
  enabled: boolean;
  run_time_hhmm: string;
  timezone: string;
  last_run_at?: string | null;
  last_status?: string | null;
};
type MaintWindow = { id: string; title: string; scope_type: string; starts_at: string; ends_at: string; status: string };
type AuditRow = {
  id: number;
  entity_type: string;
  entity_id: string;
  action: string;
  actor?: string | null;
  created_at: string;
  before_data?: Record<string, unknown> | null;
  after_data?: Record<string, unknown> | null;
};

type PingRunResponse = {
  ok?: boolean;
  device_id?: string;
  host?: string;
  method?: string;
  latency_ms?: number | null;
  icmp_only?: boolean;
  icmp?: { ok?: boolean; rtt_ms?: number; error?: string; note?: string };
  tcp_fallback?: { ok?: boolean; latency_ms?: number; error?: string };
  cache_update_error?: string;
};

type InventoryProfile = {
  uptime_oid?: string;
  cpu_primary_oid?: string;
  memory_used_oid?: string;
  memory_size_oid?: string;
  temp_primary_oid?: string;
};

function statusFromReachability(row: ActiveEquipRow): "Online" | "Offline" | "Desconhecido" {
  const reachable = row.ping_reachable;
  if (reachable === true) return "Online";
  if (reachable === false) return "Offline";
  return "Desconhecido";
}

function badgeClassFromStatus(status: "Online" | "Offline" | "Desconhecido"): string {
  if (status === "Online") return "badge badge--ok";
  if (status === "Offline") return "badge badge--err";
  return "badge badge--off";
}

/** Atualiza textos “Xs atrás” na aba Equipamentos. `_tick` força re-render periódico. */
function formatCheckedAtAgo(iso: string | null | undefined, _tick: number): string {
  void _tick;
  if (iso == null || String(iso).trim() === "") return EM_DASH;
  const t = Date.parse(String(iso));
  if (Number.isNaN(t)) return EM_DASH;
  const sec = Math.floor((Date.now() - t) / 1000);
  if (sec < 5) return "agora";
  if (sec < 60) return `${sec}s atrás`;
  const min = Math.floor(sec / 60);
  if (min < 60) return min === 1 ? "1 minuto atrás" : `${min} minutos atrás`;
  const h = Math.floor(min / 60);
  if (h < 48) return h === 1 ? "1 hora atrás" : `${h} horas atrás`;
  const d = Math.floor(h / 24);
  const remH = h % 24;
  if (remH === 0) return d === 1 ? "1 dia atrás" : `${d} dias atrás`;
  if (d === 1) return remH === 1 ? "1 dia e 1 hora atrás" : `1 dia e ${remH} horas atrás`;
  return `${d} dias e ${remH} horas atrás`;
}

function normalizeInternetTargets(raw: unknown): string[] {
  if (Array.isArray(raw)) return raw.map(String);
  if (typeof raw === "string") {
    try {
      const j = JSON.parse(raw) as unknown;
      if (Array.isArray(j)) return j.map(String);
    } catch {
      return raw.trim() ? [raw] : [];
    }
  }
  return [];
}

type ActiveEquipSortKey =
  | "description"
  | "ip"
  | "status"
  | "checked_at"
  | "latency_ms"
  | "cpu_percent"
  | "memory_percent"
  | "uptime"
  | "temperature_c";

function statusRankForSort(row: ActiveEquipRow): number {
  const s = statusFromReachability(row);
  if (s === "Offline") return 0;
  if (s === "Desconhecido") return 1;
  return 2;
}

function compareActiveEquipRows(a: ActiveEquipRow, b: ActiveEquipRow, key: ActiveEquipSortKey, dir: "asc" | "desc"): number {
  const m = dir === "asc" ? 1 : -1;
  const nOr = (x: number | null | undefined) => (x == null || !Number.isFinite(x) ? Number.NEGATIVE_INFINITY : x);
  switch (key) {
    case "description":
      return m * String(a.description).localeCompare(String(b.description), "pt", { sensitivity: "base" });
    case "ip":
      return m * String(a.ip).localeCompare(String(b.ip), "pt", { numeric: true });
    case "status":
      return m * (statusRankForSort(a) - statusRankForSort(b));
    case "checked_at": {
      const ta = Date.parse(String(a.checked_at ?? "")) || 0;
      const tb = Date.parse(String(b.checked_at ?? "")) || 0;
      return m * (ta - tb);
    }
    case "latency_ms":
      return m * (nOr(a.latency_ms) - nOr(b.latency_ms));
    case "cpu_percent":
      return m * (nOr(a.cpu_percent) - nOr(b.cpu_percent));
    case "memory_percent":
      return m * (nOr(a.memory_percent) - nOr(b.memory_percent));
    case "uptime":
      return m * String(a.uptime ?? "").localeCompare(String(b.uptime ?? ""), "pt", { numeric: true });
    case "temperature_c":
      return m * (nOr(a.temperature_c) - nOr(b.temperature_c));
    default:
      return 0;
  }
}

function readInvProfile(data: Record<string, unknown> | undefined): InventoryProfile {
  if (!data) return {};
  const cp = (data.collect_profile as Record<string, unknown> | undefined) ?? {};
  return {
    uptime_oid: typeof cp.uptime_oid === "string" ? cp.uptime_oid : undefined,
    cpu_primary_oid: typeof cp.cpu_primary_oid === "string" ? cp.cpu_primary_oid : undefined,
    memory_used_oid: typeof cp.memory_used_oid === "string" ? cp.memory_used_oid : undefined,
    memory_size_oid: typeof cp.memory_size_oid === "string" ? cp.memory_size_oid : undefined,
    temp_primary_oid: typeof cp.temp_primary_oid === "string" ? cp.temp_primary_oid : undefined,
  };
}

function IconMonPlay({ size = 20 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M5 5a2 2 0 0 1 3.008-1.728l11.997 6.998a2 2 0 0 1 .003 3.458l-12 7A2 2 0 0 1 5 19z" />
    </svg>
  );
}

function IconMonOctagonPause({ size = 20 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M10 15V9" />
      <path d="M14 15V9" />
      <path d="M2.586 16.726A2 2 0 0 1 2 15.312V8.688a2 2 0 0 1 .586-1.414l4.688-4.688A2 2 0 0 1 8.688 2h6.624a2 2 0 0 1 1.414.586l4.688 4.688A2 2 0 0 1 22 8.688v6.624a2 2 0 0 1-.586 1.414l-4.688 4.688a2 2 0 0 1-1.414.586H8.688a2 2 0 0 1-1.414-.586z" />
    </svg>
  );
}

function IconSearch({ size = 20 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.3-4.3" />
    </svg>
  );
}

function IconMonRefreshCcw({ size = 20 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
      <path d="M3 3v5h5" />
      <path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16" />
      <path d="M16 16h5v5" />
    </svg>
  );
}

function IconMonGlobe({ size = 20 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
      <path d="M2 12h20" />
    </svg>
  );
}

function formatMonitoringModeLabel(mode: string | undefined): string {
  const m = String(mode ?? "")
    .trim()
    .toLowerCase();
  if (m === "simple_ping") return "Só ping";
  if (m === "full") return "Ping + telemetria";
  if (m === "off" || m === "") return "Parado";
  return String(mode ?? "—").trim() || "—";
}

function formatInternetCheckLine(ok: boolean | null | undefined, isoAt: string | null | undefined): { status: string; when: string } | null {
  if (isoAt == null || String(isoAt).trim() === "") return null;
  const d = new Date(String(isoAt));
  const when = Number.isNaN(d.getTime())
    ? String(isoAt)
    : d.toLocaleString("pt-PT", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
  if (ok === true) return { status: "Internet acessível", when };
  if (ok === false) return { status: "Internet inacessível (na última verificação)", when };
  return { status: "Última verificação de internet", when };
}

/** Evita mensagens técnicas (gRPC, Go, etc.) cruas nos toasts. */
function friendlyApiMessage(raw: string): string {
  const t = String(raw ?? "").trim();
  if (!t) return "Ocorreu um erro inesperado.";
  const lower = t.toLowerCase();
  if (lower.includes("context deadline exceeded") || lower.includes("deadline exceeded")) {
    return "A requisição demorou demasiado ou o servidor não respondeu a tempo. Tente novamente.";
  }
  if (lower.includes("connection reset") || lower.includes("econnreset")) {
    return "A ligação foi interrompida. Verifique a rede e tente novamente.";
  }
  if (lower.includes("etimedout") || lower.includes("timeout") || lower.includes("i/o timeout")) {
    return "Tempo de espera esgotado. Tente novamente.";
  }
  if (lower.includes("failed to fetch") || lower.includes("networkerror when attempting to fetch")) {
    return "Sem ligação ao servidor. Verifique a API ou a rede.";
  }
  if (lower.includes("no_internet") || lower.includes("no internet") || (lower.includes("424") && lower.length < 80)) {
    return "Sem acesso à internet conforme a verificação no servidor.";
  }
  if (t.length > 220) return `${t.slice(0, 217)}…`;
  return t;
}

function formatNetCheckToast(data: NetCheck): string {
  const head = data.ok ? "Internet acessível" : "Internet inacessível";
  const at = new Date(data.checked_at);
  const when = Number.isNaN(at.getTime()) ? data.checked_at : at.toLocaleString("pt-PT", { dateStyle: "short", timeStyle: "medium" });
  const bits = (data.targets_tried ?? []).map((t) =>
    t.ok ? `${t.url} (${t.latency_ms} ms)` : `${t.url} (${friendlyApiMessage(t.error?.trim() || "falhou")})`,
  );
  return [head + " · " + when, ...bits].join(" — ").slice(0, 720);
}

/** Lucide «external-link» — indica navegação para outro ecrã. */
function IconExternalLinkSubtle({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden style={{ opacity: 0.55 }}>
      <path d="M15 3h6v6" />
      <path d="M10 14 21 3" />
      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
    </svg>
  );
}

export function MonitoringPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const showPageToastRef = useRef<(ok: boolean, text: string) => void>(() => {});
  const offlineToastIdsRef = useRef(new Map<string, string>());

  const showPageToast = useCallback(
    (ok: boolean, text: string) => {
      pushToast({ tone: ok ? "ok" : "err", text: ok ? text : friendlyApiMessage(text), autoMs: 4800 });
    },
    [pushToast],
  );
  showPageToastRef.current = showPageToast;

  const [tab, setTab] = useState<"overview" | "settings" | "ops">("overview");
  useEffect(() => {
    if (!canMutate && (tab === "settings" || tab === "ops")) setTab("overview");
  }, [canMutate, tab]);
  const handledOfflineAlertsRef = useRef(new Set<string>());
  const [snmpModalDeviceId, setSnmpModalDeviceId] = useState<string | null>(null);
  const [actionMenuRow, setActionMenuRow] = useState<ActiveEquipRow | null>(null);
  const [reportModalDevice, setReportModalDevice] = useState<DeviceReportTarget | null>(null);
  const [startMonModalOpen, setStartMonModalOpen] = useState(false);
  const [agoTick, setAgoTick] = useState(0);

  const state = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<MonState>("/api/v1/monitoring/state"),
    refetchInterval: 1000,
    refetchOnWindowFocus: true,
    staleTime: 0,
  });
  const intervals = useQuery({
    queryKey: queryKeys.monIntervals,
    queryFn: () =>
      apiFetch<{
        ping_seconds: number;
        telemetry_seconds?: number;
        telemetry_minutes: number;
        interface_snapshot_seconds?: number;
        olt_if_derived_pon_seconds?: number;
        ping_timeout_ms: number;
      }>("/api/v1/settings/monitoring-intervals"),
  });

  const pingListPollMs = useMemo(() => {
    const d = intervals.data;
    if (!d) return 5000;
    const secs = Math.min(
      d.ping_seconds,
      d.telemetry_seconds ?? d.telemetry_minutes * 60,
      d.interface_snapshot_seconds ?? 300,
      d.olt_if_derived_pon_seconds ?? 240,
    );
    return Math.max(secs * 1000, 5000);
  }, [
    intervals.data?.ping_seconds,
    intervals.data?.telemetry_seconds,
    intervals.data?.telemetry_minutes,
    intervals.data?.interface_snapshot_seconds,
    intervals.data?.olt_if_derived_pon_seconds,
  ]);
  const msetRaw = useQuery({
    queryKey: ["mon-settings"],
    queryFn: () =>
      apiFetch<{ vps_latency_offset_ms: number; internet_check_targets: unknown; internet_check_timeout_ms: number }>(
        "/api/v1/settings/monitoring",
      ),
  });
  const mset = {
    ...msetRaw,
    data: msetRaw.data
      ? {
          vps_latency_offset_ms: msetRaw.data.vps_latency_offset_ms,
          internet_check_timeout_ms: msetRaw.data.internet_check_timeout_ms,
          internet_check_targets: normalizeInternetTargets(msetRaw.data.internet_check_targets),
        }
      : undefined,
  };
  const nightly = useQuery({
    queryKey: ["nightly-collection"],
    queryFn: () => apiFetch<NightlyCfg>("/api/v1/monitoring/nightly-collection"),
    enabled: tab === "ops",
  });
  const maint = useQuery({
    queryKey: ["maintenance-windows"],
    queryFn: () => apiFetch<{ items: MaintWindow[] }>("/api/v1/maintenance/windows"),
    enabled: tab === "ops",
  });
  const audit = useQuery({
    queryKey: ["ops-audit"],
    queryFn: () => apiFetch<{ items: AuditRow[] }>("/api/v1/ops/audit?limit=120"),
    enabled: tab === "ops",
  });
  const popsLite = useQuery({
    queryKey: ["pops-lite-ops"],
    queryFn: () => apiFetch<{ pops: Array<{ id: string; description: string }> }>("/api/v1/pops"),
    enabled: tab === "ops",
  });
  const devicesLite = useQuery({
    queryKey: ["devices-lite-ops"],
    queryFn: () => apiFetch<{ devices: Array<{ id: string; description: string }> }>("/api/v1/devices"),
    enabled: tab === "ops",
  });

  const activeEquipList = useQuery({
    queryKey: ["monitoring-active-equipment"],
    queryFn: () => apiFetch<{ devices: ActiveEquipRow[] }>("/api/v1/monitoring/active-equipment"),
    enabled: tab === "overview",
    refetchInterval: tab === "overview" ? pingListPollMs : false,
  });

  const [equipSort, setEquipSort] = useState<{ key: ActiveEquipSortKey; dir: "asc" | "desc" }>({ key: "description", dir: "asc" });
  const [equipQuickSearchOpen, setEquipQuickSearchOpen] = useState(false);
  const [equipQuickSearch, setEquipQuickSearch] = useState("");
  const equipQuickSearchInputRef = useRef<HTMLInputElement | null>(null);

  const sortedActiveEquipment = useMemo(() => {
    const raw = activeEquipList.data?.devices ?? [];
    const list = [...raw];
    list.sort((a, b) => {
      const ra = statusRankForSort(a);
      const rb = statusRankForSort(b);
      if (ra !== rb) return ra - rb;
      const c = compareActiveEquipRows(a, b, equipSort.key, equipSort.dir);
      if (c !== 0) return c;
      return String(a.id).localeCompare(String(b.id));
    });
    return list;
  }, [activeEquipList.data?.devices, equipSort.key, equipSort.dir]);

  const filteredActiveEquipment = useMemo(() => {
    const q = equipQuickSearch.trim().toLowerCase();
    if (!q) return sortedActiveEquipment;
    return sortedActiveEquipment.filter((row) => {
      const d = String(row.description ?? "").toLowerCase();
      const ip = String(row.ip ?? "").toLowerCase();
      const c = String(row.category ?? "").toLowerCase();
      return d.includes(q) || ip.includes(q) || c.includes(q);
    });
  }, [sortedActiveEquipment, equipQuickSearch]);

  useEffect(() => {
    if (!equipQuickSearchOpen) return;
    const id = requestAnimationFrame(() => {
      equipQuickSearchInputRef.current?.focus();
      equipQuickSearchInputRef.current?.select();
    });
    return () => cancelAnimationFrame(id);
  }, [equipQuickSearchOpen]);

  const onEquipSortClick = (key: ActiveEquipSortKey) => {
    setEquipSort((prev) => (prev.key === key ? { key, dir: prev.dir === "asc" ? "desc" : "asc" } : { key, dir: "asc" }));
  };
  const sortMark = (key: ActiveEquipSortKey) => (equipSort.key === key ? (equipSort.dir === "asc" ? " ▲" : " ▼") : "");

  const snmpInventoryQuery = useQuery({
    queryKey: ["snmp-inventory", snmpModalDeviceId],
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/devices/${snmpModalDeviceId}/snmp-inventory`),
    enabled: !!snmpModalDeviceId,
  });
  const offlineAlerts = useQuery({
    queryKey: queryKeys.alertsPingUnreachable,
    queryFn: () => apiFetch<{ alerts: OfflineAlertRow[] }>("/api/v1/alerts/active?type=ping_unreachable&limit=50"),
    enabled: tab === "overview",
    refetchInterval: tab === "overview" ? 3000 : false,
  });

  const invalidateActiveList = () => {
    qc.invalidateQueries({ queryKey: ["monitoring-active-equipment"] });
    qc.invalidateQueries({ queryKey: queryKeys.alertsPingUnreachable });
  };

  useEffect(() => {
    if (tab !== "overview") {
      for (const tid of offlineToastIdsRef.current.values()) dismissToast(tid);
      offlineToastIdsRef.current.clear();
      return;
    }
    const list = (offlineAlerts.data?.alerts as OfflineAlertRow[] | undefined) ?? [];
    const active = new Set<string>();
    for (const a of list) {
      if (handledOfflineAlertsRef.current.has(a.id)) continue;
      active.add(a.id);
      if (!offlineToastIdsRef.current.has(a.id)) {
        const alertId = a.id;
        const tid = pushToast({
          tone: "err",
          text: "",
          kind: "offline",
          offlineTitle: a.device_name || "Equipamento offline",
          offlineIp: a.ip || "—",
          autoMs: 0,
          onDismiss: () => {
            handledOfflineAlertsRef.current.add(alertId);
            offlineToastIdsRef.current.delete(alertId);
          },
        });
        offlineToastIdsRef.current.set(a.id, tid);
      }
    }
    for (const [alertId, tid] of [...offlineToastIdsRef.current.entries()]) {
      if (!active.has(alertId)) {
        dismissToast(tid);
        offlineToastIdsRef.current.delete(alertId);
      }
    }
  }, [tab, offlineAlerts.data, pushToast, dismissToast]);

  useEffect(() => {
    if (tab !== "overview") return;
    const id = window.setInterval(() => setAgoTick((n) => n + 1), 15000);
    return () => window.clearInterval(id);
  }, [tab]);

  const pingOne = useMutation({
    mutationFn: (id: string) => apiFetch<PingRunResponse>(`/api/v1/ping/devices/${id}/run?port=443&icmp_only=true`, { method: "POST" }),
    onSuccess: (data) => {
      const ms = data.latency_ms != null ? `${data.latency_ms} ms` : "—";
      const icmpErr = data.icmp?.error ? ` · ${data.icmp.error}` : "";
      const cacheNote = data.cache_update_error ? ` · cache: ${data.cache_update_error}` : "";
      const line = `${data.host ?? "?"}: ${data.ok ? "OK" : "FALHOU"} (${data.method ?? "?"}, ${ms})${icmpErr}${cacheNote}`;
      showPageToastRef.current(!!data.ok, `Ping: ${line}`);
      invalidateActiveList();
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });
  const telOne = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/telemetry/devices/${id}/collect`, { method: "POST", json: {} }),
    onSuccess: () => {
      invalidateActiveList();
      showPageToastRef.current(true, "Telemetria solicitada.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });
  const reportOne = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/monitoring/full-report/devices/${id}`, { method: "POST", json: {} }),
    onSuccess: () => {
      invalidateActiveList();
      showPageToastRef.current(true, "Relatório completo solicitado ao servidor.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });
  const discoverSNMP = useMutation({
    mutationFn: (id: string) =>
      apiFetch<{ job_id?: string; status?: string; note?: string }>(`/api/v1/devices/${id}/telemetry/discover`, { method: "POST" }),
    onSuccess: (data, id) => {
      qc.invalidateQueries({ queryKey: ["devices"] });
      qc.invalidateQueries({ queryKey: ["snmp-inventory", id] });
      invalidateActiveList();
      showPageToastRef.current(
        true,
        `Descoberta SNMP em fila — job ${String(data.job_id ?? "?")} (${String(data.status ?? "?")}). ${String(data.note ?? "")}`,
      );
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });

  const checkNet = useMutation({
    mutationFn: () => apiFetch<NetCheck>("/api/v1/monitoring/internet-check"),
    onSuccess: (data) => {
      void qc.invalidateQueries({ queryKey: queryKeys.monState });
      void qc.invalidateQueries({ queryKey: ["mon-state-global-indicator"] });
      showPageToastRef.current(data.ok, formatNetCheckToast(data));
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });

  const startMon = useMutation({
    mutationFn: (args: { mode: string }) =>
      apiFetch("/api/v1/monitoring/start", { method: "POST", json: args }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.monState });
      void qc.invalidateQueries({ queryKey: ["mon-state-global-indicator"] });
      showPageToastRef.current(true, "Monitoramento iniciado.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });

  const stopMon = useMutation({
    mutationFn: () => apiFetch("/api/v1/monitoring/stop", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.monState });
      void qc.invalidateQueries({ queryKey: ["mon-state-global-indicator"] });
      showPageToastRef.current(true, "Monitoramento parado.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });

  const reloadDev = useMutation({
    mutationFn: () => apiFetch<{ reloaded: boolean; device_count: number }>("/api/v1/monitoring/reload-devices", { method: "POST", json: {} }),
    onSuccess: (d) => {
      showPageToastRef.current(true, `Lista recarregada (${d.device_count} equipamento(s) na memória do serviço).`);
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });

  const [vps, setVps] = useState("");
  const [ict, setIct] = useState("");
  const [targetsRaw, setTargetsRaw] = useState("");

  useEffect(() => {
    if (tab !== "settings" || !mset.data) return;
    setVps((v) => (v === "" ? String(mset.data!.vps_latency_offset_ms) : v));
    setIct((v) => (v === "" ? String(mset.data!.internet_check_timeout_ms) : v));
    setTargetsRaw((t) => (t.trim() === "" ? JSON.stringify(mset.data!.internet_check_targets, null, 2) : t));
  }, [tab, mset.data]);

  const patchMset = useMutation({
    mutationFn: (body: Record<string, unknown>) => apiFetch("/api/v1/settings/monitoring", { method: "PATCH", json: body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mon-settings"] });
      patchMset.reset();
      showPageToastRef.current(true, "Definições de internet salvas.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });
  const [mwTitle, setMwTitle] = useState("");
  const [mwScope, setMwScope] = useState<"global" | "pop" | "device">("global");
  const [mwPopID, setMwPopID] = useState("");
  const [mwDevID, setMwDevID] = useState("");
  const [mwStart, setMwStart] = useState("");
  const [mwEnd, setMwEnd] = useState("");
  const createMw = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/maintenance/windows", {
        method: "POST",
        json: {
          title: mwTitle,
          scope_type: mwScope,
          pop_id: mwScope === "pop" ? mwPopID : null,
          device_id: mwScope === "device" ? mwDevID : null,
          starts_at: new Date(mwStart).toISOString(),
          ends_at: new Date(mwEnd).toISOString(),
          checklist: [],
        },
      }),
    onSuccess: () => {
      void maint.refetch();
      showPageToastRef.current(true, "Janela de manutenção criada.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });
  const runNightlyNow = useMutation({
    mutationFn: () => apiFetch("/api/v1/monitoring/nightly-collection/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void nightly.refetch();
      showPageToastRef.current(true, "Coleta noturna pedida ao servidor.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });
  const patchNightly = useMutation({
    mutationFn: (body: Record<string, unknown>) => apiFetch("/api/v1/monitoring/nightly-collection", { method: "PATCH", json: body }),
    onSuccess: () => {
      void nightly.refetch();
      showPageToastRef.current(true, "Agenda noturna actualizada.");
    },
    onError: (e: Error) => showPageToastRef.current(false, e.message),
  });

  useEffect(() => {
    if (!activeEquipList.isError || !activeEquipList.error) return;
    showPageToastRef.current(false, (activeEquipList.error as Error).message);
  }, [activeEquipList.isError, activeEquipList.error]);

  useEffect(() => {
    if (tab !== "settings" || !msetRaw.isError || !msetRaw.error) return;
    showPageToastRef.current(false, (msetRaw.error as Error).message);
  }, [tab, msetRaw.isError, msetRaw.error]);

  useEffect(() => {
    if (!snmpModalDeviceId || !snmpInventoryQuery.isError || !snmpInventoryQuery.error) return;
    showPageToastRef.current(false, (snmpInventoryQuery.error as Error).message);
  }, [snmpModalDeviceId, snmpInventoryQuery.isError, snmpInventoryQuery.error]);

  if (state.isLoading) return <p>Carregando estado…</p>;

  return (
    <>
      <div className="page-heading">
        <h1>
          Monitoramento
          {canMutate ? (
            <InfoHint label="Dicas de monitoramento">
              <p>
                Inicie o monitoramento só depois de validar a rede, se precisar. Os ícones à direita permitem verificar internet e recarregar a
                lista em memória no serviço.
              </p>
            </InfoHint>
          ) : null}
        </h1>
        <PageCountPill
          label={tab === "overview" ? "Equipamentos monitorados" : tab === "ops" ? "Registros de auditoria" : "Itens"}
          count={tab === "overview" ? filteredActiveEquipment.length : tab === "ops" ? (audit.data?.items ?? []).length : 0}
        />
      </div>
      {!canMutate ? (
        <p style={{ color: "var(--muted)", marginTop: 0 }}>
          Modo só leitura: pode acompanhar o estado do motor e da lista de equipamentos; iniciar/parar monitoramento e outras acções ficam reservadas a
          administradores.
        </p>
      ) : null}

      <div className="tabs" style={{ flexWrap: "wrap" }}>
        <button type="button" className={tab === "overview" ? "active" : ""} onClick={() => setTab("overview")}>
          Visão geral
        </button>
        {canMutate ? (
          <>
            <button type="button" className={tab === "settings" ? "active" : ""} onClick={() => setTab("settings")}>
              Internet / VPS
            </button>
            <button type="button" className={tab === "ops" ? "active" : ""} onClick={() => setTab("ops")}>
              Operação
            </button>
          </>
        ) : null}
      </div>

      {tab === "overview" && (
        <div className="card">
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "flex-start",
              gap: 16,
              flexWrap: "wrap",
              marginBottom: 10,
            }}
          >
            <h2 style={{ margin: 0 }}>Equipamentos monitorados</h2>
            <div style={{ fontSize: 13, lineHeight: 1.55, textAlign: "right", color: "var(--text)", minWidth: 200 }}>
              <div>
                <strong>Monitoramento:</strong> {state.data?.is_running ? "ligado" : "desligado"}
              </div>
              <div>
                <strong>Modo:</strong> {formatMonitoringModeLabel(state.data?.monitoring_mode)}
              </div>
              <div className="mono" style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }} title="Reflete qualquer alteração gravada no servidor (outros browsers incluídos)">
                Estado no servidor:{" "}
                {state.data?.runtime_updated_at
                  ? new Date(state.data.runtime_updated_at).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "medium" })
                  : "—"}{" "}
                · atualização ~1s
              </div>
            </div>
          </div>
          {(() => {
            const line = formatInternetCheckLine(state.data?.last_internet_check_ok ?? null, state.data?.last_internet_check_at ?? null);
            if (!line) return null;
            return (
              <p style={{ margin: "0 0 10px", fontSize: 12, color: "var(--muted)" }}>
                <span>{line.status}</span>
                <span style={{ marginLeft: 6 }}>· {line.when}</span>
              </p>
            );
          })()}
          <div className="row" style={{ margin: "12px 0", flexWrap: "wrap", gap: 10, alignItems: "center" }}>
            {canMutate ? (
              <>
                <button
                  type="button"
                  className="btn btn--primary"
                  style={{ display: "inline-flex", alignItems: "center", gap: 8 }}
                  disabled={startMon.isPending}
                  onClick={() => {
                    startMon.reset();
                    setStartMonModalOpen(true);
                  }}
                >
                  <IconMonPlay size={20} />
                  Iniciar monitoramento
                </button>
                <button
                  type="button"
                  className="btn btn--danger"
                  style={{ display: "inline-flex", alignItems: "center", gap: 8 }}
                  disabled={stopMon.isPending}
                  onClick={() => stopMon.mutate()}
                >
                  <IconMonOctagonPause size={20} />
                  Parar monitoramento
                </button>
                <button
                  type="button"
                  className="btn btn--icon btn--icon-menu"
                  title="Verificar internet"
                  aria-label="Verificar internet"
                  disabled={checkNet.isPending}
                  onClick={() => checkNet.mutate()}
                >
                  {checkNet.isPending ? <span style={{ fontSize: 12 }}>…</span> : <IconMonGlobe size={22} />}
                </button>
                <button
                  type="button"
                  className="btn btn--icon btn--icon-menu"
                  title="Recarregar equipamentos"
                  aria-label="Recarregar equipamentos"
                  disabled={reloadDev.isPending}
                  onClick={() => reloadDev.mutate()}
                >
                  <span className={reloadDev.isPending ? "map-refresh-spin" : undefined} style={{ display: "inline-flex" }}>
                    <IconMonRefreshCcw size={22} />
                  </span>
                </button>
              </>
            ) : null}
            <div className={`monitoring-toolbar-search${equipQuickSearchOpen ? " monitoring-toolbar-search--open" : ""}`}>
              {!equipQuickSearchOpen ? (
                <button
                  type="button"
                  className="btn btn--icon btn--icon-menu"
                  title="Pesquisar na lista"
                  aria-label="Pesquisar na lista de equipamentos"
                  aria-expanded={false}
                  onClick={() => setEquipQuickSearchOpen(true)}
                >
                  <IconSearch size={22} />
                </button>
              ) : (
                <div className="monitoring-toolbar-search__field">
                  <input
                    ref={equipQuickSearchInputRef}
                    type="search"
                    className="input"
                    placeholder="Descrição, IP ou categoria…"
                    aria-label="Filtrar equipamentos na lista"
                    value={equipQuickSearch}
                    onChange={(e) => setEquipQuickSearch(e.target.value)}
                  />
                  <button
                    type="button"
                    className="btn btn--icon btn--icon-menu"
                    title="Fechar pesquisa"
                    aria-label="Fechar pesquisa"
                    onClick={() => {
                      setEquipQuickSearchOpen(false);
                      setEquipQuickSearch("");
                    }}
                  >
                    ×
                  </button>
                </div>
              )}
            </div>
          </div>
          {activeEquipList.isLoading && <p>A carregar…</p>}
          {activeEquipList.data && (
            <>
              <div className="table-wrap" style={{ overflowX: "auto" }}>
                <table style={{ fontSize: 11, minWidth: 880 }}>
                  <thead>
                    <tr>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("description")} title="Ordenar">
                        Descrição{sortMark("description")}
                      </th>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("ip")} title="Ordenar">
                        IP{sortMark("ip")}
                      </th>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("status")} title="Ordenar">
                        STATUS{sortMark("status")}
                      </th>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("checked_at")} title="Ordenar">
                        Última atualização{sortMark("checked_at")}
                      </th>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("latency_ms")} title="Ordenar">
                        Latência{sortMark("latency_ms")}
                      </th>
                      <th
                        title="Percentagem de utilização de CPU na última amostra — clique para ordenar"
                        style={{ cursor: "pointer", userSelect: "none" }}
                        onClick={() => onEquipSortClick("cpu_percent")}
                      >
                        CPU %{sortMark("cpu_percent")}
                      </th>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("memory_percent")} title="Ordenar">
                        Mem %{sortMark("memory_percent")}
                      </th>
                      <th
                        title="sysUpTime (SNMP) — clique para ordenar"
                        style={{ cursor: "pointer", userSelect: "none" }}
                        onClick={() => onEquipSortClick("uptime")}
                      >
                        Uptime{sortMark("uptime")}
                      </th>
                      <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onEquipSortClick("temperature_c")} title="Ordenar">
                        Temp °C{sortMark("temperature_c")}
                      </th>
                      {canMutate ? <th style={{ width: 48 }} /> : null}
                    </tr>
                  </thead>
                  <tbody>
                    {filteredActiveEquipment.map((row) => {
                      const status = statusFromReachability(row);
                      const reachable = row.ping_reachable;
                      return (
                        <tr key={row.id}>
                          <td>
                            {row.description}
                            <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                              {row.category}
                            </div>
                          </td>
                          <td className="mono">{row.ip}</td>
                          <td>
                            <span className={badgeClassFromStatus(status)}>{status}</span>
                          </td>
                          <td className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                            {formatCheckedAtAgo(row.checked_at as string | null | undefined, agoTick)}
                          </td>
                          <td className="mono" style={{ color: reachable === false ? "var(--err)" : undefined }}>
                            {row.latency_ms != null ? `${row.latency_ms} ms` : "—"}
                          </td>
                          <td className="mono">{row.cpu_percent != null ? `${row.cpu_percent.toFixed(1)}%` : EM_DASH}</td>
                          <td className="mono">{row.memory_percent != null ? `${row.memory_percent.toFixed(0)}%` : EM_DASH}</td>
                          <td className="mono">{row.uptime ?? EM_DASH}</td>
                          <td className="mono">{row.temperature_c != null ? `${row.temperature_c.toFixed(0)} °C` : EM_DASH}</td>
                          {canMutate ? (
                            <td>
                              <button type="button" className="btn btn--icon" aria-label="Opções do equipamento" title="Opções" onClick={() => setActionMenuRow(row)}>
                                ⋮
                              </button>
                            </td>
                          ) : (
                            <td>
                              <Link className="btn" style={{ fontSize: 11, padding: "4px 8px" }} to={`/devices?focus=${encodeURIComponent(row.id)}`}>
                                Ver equipamento
                              </Link>
                            </td>
                          )}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              {filteredActiveEquipment.length === 0 && (
                <p style={{ color: "var(--muted)" }}>
                  {equipQuickSearch.trim()
                    ? `Nenhum equipamento corresponde a «${equipQuickSearch.trim()}».`
                    : "Nenhum equipamento cumpre os critérios (Normal + ping + Ativo)."}
                </p>
              )}
            </>
          )}
        </div>
      )}

      {tab === "settings" && canMutate && mset.data && (
        <div className="card">
          <h2>Alvos HTTPS + offset VPS</h2>
          <p style={{ fontSize: 12, color: "var(--muted)" }}>
            Ajuste o atraso medido no VPS e a lista de URLs usadas para testar a internet. Os valores são salvos no servidor.
            Intervalos globais do worker, pacote ICMP e limiar para alertas passaram para <strong>Configurações · Alertas</strong>.
          </p>
          <div className="field">
            <label>Compensação de latência do VPS (ms)</label>
            <input className="input mono" title="vps_latency_offset_ms" value={vps} onChange={(e) => setVps(e.target.value)} />
          </div>
          <div className="field">
            <label>Tempo limite do teste HTTPS (ms)</label>
            <input className="input mono" title="internet_check_timeout_ms" value={ict} onChange={(e) => setIct(e.target.value)} />
          </div>
          <div className="field">
            <label>URLs de teste (JSON: array de endereços)</label>
            <textarea className="textarea mono" rows={5} title="internet_check_targets" value={targetsRaw} onChange={(e) => setTargetsRaw(e.target.value)} />
          </div>
          <button
            type="button"
            className="btn btn--primary"
            disabled={patchMset.isPending}
            onClick={() => {
              const body: Record<string, unknown> = {};
              if (vps !== "") body.vps_latency_offset_ms = Number(vps);
              if (ict !== "") body.internet_check_timeout_ms = Number(ict);
              if (targetsRaw.trim()) body.internet_check_targets = JSON.parse(targetsRaw);
              patchMset.mutate(body);
            }}
          >
            Salvar definições de internet
          </button>
        </div>
      )}

      {tab === "ops" && canMutate && (
        <>
          <div className="card">
            <h2>Coleta completa noturna</h2>
            <div className="row" style={{ gap: 8 }}>
              <input
                className="input mono"
                style={{ width: 90 }}
                defaultValue={nightly.data?.run_time_hhmm ?? "02:30"}
                onBlur={(e) => patchNightly.mutate({ run_time_hhmm: e.target.value })}
              />
              <input
                className="input mono"
                style={{ width: 190 }}
                defaultValue={nightly.data?.timezone ?? "America/Sao_Paulo"}
                onBlur={(e) => patchNightly.mutate({ timezone: e.target.value })}
              />
              <button type="button" className="btn" onClick={() => patchNightly.mutate({ enabled: !(nightly.data?.enabled ?? false) })}>
                {nightly.data?.enabled ? "Desativar agenda" : "Ativar agenda"}
              </button>
              <button type="button" className="btn btn--primary" disabled={runNightlyNow.isPending} onClick={() => runNightlyNow.mutate()}>
                Executar agora
              </button>
            </div>
            <p className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
              Última execução: {nightly.data?.last_run_at ?? "—"} · status: {nightly.data?.last_status ?? "—"}
            </p>
          </div>

          <div className="card">
            <h2>Checklist de manutenção com janela</h2>
            <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
              <input className="input" placeholder="Título" value={mwTitle} onChange={(e) => setMwTitle(e.target.value)} />
              <select className="select" value={mwScope} onChange={(e) => setMwScope(e.target.value as "global" | "pop" | "device")}>
                <option value="global">Global</option>
                <option value="pop">POP</option>
                <option value="device">Dispositivo</option>
              </select>
              {mwScope === "pop" ? (
                <select className="select" value={mwPopID} onChange={(e) => setMwPopID(e.target.value)}>
                  <option value="">POP</option>
                  {(popsLite.data?.pops ?? []).map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.description}
                    </option>
                  ))}
                </select>
              ) : null}
              {mwScope === "device" ? (
                <select className="select" value={mwDevID} onChange={(e) => setMwDevID(e.target.value)}>
                  <option value="">Dispositivo</option>
                  {(devicesLite.data?.devices ?? []).map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.description}
                    </option>
                  ))}
                </select>
              ) : null}
              <input className="input mono" type="datetime-local" value={mwStart} onChange={(e) => setMwStart(e.target.value)} />
              <input className="input mono" type="datetime-local" value={mwEnd} onChange={(e) => setMwEnd(e.target.value)} />
              <button type="button" className="btn btn--primary" disabled={!mwTitle || !mwStart || !mwEnd || createMw.isPending} onClick={() => createMw.mutate()}>
                Criar
              </button>
            </div>
            <div className="table-wrap" style={{ marginTop: 8 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Título</th>
                    <th>Escopo</th>
                    <th>Início</th>
                    <th>Fim</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {(maint.data?.items ?? []).map((m) => (
                    <tr key={m.id}>
                      <td>{m.title}</td>
                      <td className="mono">{m.scope_type}</td>
                      <td className="mono">{new Date(m.starts_at).toLocaleString("pt-PT")}</td>
                      <td className="mono">{new Date(m.ends_at).toLocaleString("pt-PT")}</td>
                      <td>{m.status}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="card">
            <h2>Auditoria operacional</h2>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Quando</th>
                    <th>Entidade</th>
                    <th>ID</th>
                    <th>Ação</th>
                    <th>Ator</th>
                    <th>Diff</th>
                  </tr>
                </thead>
                <tbody>
                  {(audit.data?.items ?? []).map((a) => (
                    <tr key={a.id}>
                      <td className="mono">{new Date(a.created_at).toLocaleString("pt-PT")}</td>
                      <td>{a.entity_type}</td>
                      <td className="mono">{a.entity_id}</td>
                      <td>{a.action}</td>
                      <td>{a.actor ?? "—"}</td>
                      <td>
                        <details>
                          <summary style={{ cursor: "pointer", color: "var(--muted)" }}>ver</summary>
                          <pre className="mono" style={{ margin: 0, fontSize: 10, whiteSpace: "pre-wrap" }}>
                            {prettyAuditDiff(a.before_data, a.after_data)}
                          </pre>
                        </details>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}

      {snmpModalDeviceId && (
        <div
          className="modal-backdrop"
          role="dialog"
          aria-modal="true"
          aria-labelledby="snmp-inv-title"
          onClick={(e) => e.target === e.currentTarget && setSnmpModalDeviceId(null)}
        >
          <div className="modal modal--wide" onClick={(e) => e.stopPropagation()}>
            <h3 id="snmp-inv-title">Inventário SNMP</h3>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Equipamento <span className="mono">{snmpModalDeviceId}</span> — resultado da última descoberta de objetos SNMP (dados técnicos abaixo).
            </p>
            {snmpInventoryQuery.isLoading && <p>A carregar…</p>}
            {snmpInventoryQuery.data && (() => {
              const profile = readInvProfile(snmpInventoryQuery.data);
              return (
                <>
                  <div className="card" style={{ marginBottom: 8 }}>
                    <h4 style={{ marginTop: 0 }}>Objetos escolhidos automaticamente</h4>
                    <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
                      <span className="badge badge--off" title={profile.uptime_oid ?? ""}>
                        Tempo ligado: {profile.uptime_oid ? "definido" : "não identificado"}
                      </span>
                      <span className="badge badge--off" title={profile.cpu_primary_oid ?? ""}>
                        CPU: {profile.cpu_primary_oid ? "definido" : "não identificado"}
                      </span>
                      <span className="badge badge--off" title={`${profile.memory_used_oid ?? ""} / ${profile.memory_size_oid ?? ""}`}>
                        Memória: {profile.memory_used_oid && profile.memory_size_oid ? "definida" : "incompleta"}
                      </span>
                      <span className="badge badge--off" title={profile.temp_primary_oid ?? ""}>
                        Temperatura: {profile.temp_primary_oid ? "definida" : "não identificado"}
                      </span>
                    </div>
                  </div>
                  <pre style={{ maxHeight: 440, overflow: "auto", fontSize: 10, marginTop: 8 }}>{JSON.stringify(snmpInventoryQuery.data, null, 2)}</pre>
                </>
              );
            })()}
            <div className="row" style={{ marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setSnmpModalDeviceId(null)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      )}

      {startMonModalOpen && (
        <div
          className="modal-backdrop"
          role="dialog"
          aria-modal="true"
          aria-labelledby="start-mon-title"
          onClick={(e) => e.target === e.currentTarget && setStartMonModalOpen(false)}
        >
          <div className="modal" style={{ maxWidth: 420 }} onClick={(e) => e.stopPropagation()}>
            <h3 id="start-mon-title">Iniciar monitoramento</h3>
            <p style={{ fontSize: 13, color: "var(--muted)", marginTop: 0 }}>
              Escolha o modo de execução do motor no servidor. No modo completo, o walk SNMP de inventário corre apenas para equipamentos sem inventário
              gravado (com telemetria ativa e host acessível); para actualizar um equipamento já inventariado, use a página Equipamentos ou a acção de walk SNMP.
            </p>
            <div className="row" style={{ flexDirection: "column", alignItems: "stretch", gap: 8, marginTop: 12 }}>
              <button
                type="button"
                className="btn btn--primary"
                disabled={startMon.isPending}
                onClick={() => {
                  startMon.mutate(
                    { mode: "simple_ping" },
                    {
                      onSuccess: () => setStartMonModalOpen(false),
                    },
                  );
                }}
              >
                Só ping
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={startMon.isPending}
                onClick={() => {
                  startMon.mutate(
                    { mode: "full" },
                    {
                      onSuccess: () => setStartMonModalOpen(false),
                    },
                  );
                }}
              >
                Completo (ping + telemetria)
              </button>
              <button type="button" className="btn" disabled={startMon.isPending} onClick={() => setStartMonModalOpen(false)}>
                Cancelar
              </button>
            </div>
          </div>
        </div>
      )}

      {actionMenuRow && (
        <div
          className="modal-backdrop"
          role="dialog"
          aria-modal="true"
          aria-labelledby="equip-actions-title"
          onClick={(e) => e.target === e.currentTarget && setActionMenuRow(null)}
        >
          <div className="modal" style={{ maxWidth: 420 }} onClick={(e) => e.stopPropagation()}>
            <h3 id="equip-actions-title">Ações do equipamento</h3>
            <p className="mono" style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
              {actionMenuRow.description} · {actionMenuRow.ip}
            </p>
            <div className="row" style={{ flexDirection: "column", alignItems: "stretch", gap: 8 }}>
              <Link
                to={`/devices?focus=${encodeURIComponent(actionMenuRow.id)}`}
                className="btn"
                onClick={() => setActionMenuRow(null)}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  position: "relative",
                  width: "100%",
                  textAlign: "center",
                  paddingLeft: 14,
                  paddingRight: 36,
                }}
              >
                <span style={{ flex: "1 1 auto", textAlign: "center" }}>Abrir equipamento</span>
                <span style={{ position: "absolute", right: 10, top: "50%", transform: "translateY(-50%)", display: "flex", pointerEvents: "none" }}>
                  <IconExternalLinkSubtle size={15} />
                </span>
              </Link>
              {canMutate ? (
                <>
                  <button type="button" className="btn" disabled={pingOne.isPending} onClick={() => { pingOne.mutate(actionMenuRow.id); setActionMenuRow(null); }}>
                    Ping ICMP
                  </button>
                  <button type="button" className="btn" disabled={telOne.isPending} onClick={() => { telOne.mutate(actionMenuRow.id); setActionMenuRow(null); }}>
                    Colectar telemetria (SNMP)
                  </button>
                  <button type="button" className="btn" disabled={discoverSNMP.isPending} onClick={() => { discoverSNMP.mutate(actionMenuRow.id); setActionMenuRow(null); }}>
                    Redescobrir SNMP (walk)
                  </button>
                  <button
                    type="button"
                    className="btn"
                    disabled={reportOne.isPending}
                    onClick={() => {
                      reportOne.mutate(actionMenuRow.id);
                      setReportModalDevice({
                        id: actionMenuRow.id,
                        description: actionMenuRow.description,
                        ip: actionMenuRow.ip,
                        category: actionMenuRow.category,
                        brand: actionMenuRow.brand,
                      });
                      setActionMenuRow(null);
                    }}
                  >
                    Relatório completo
                  </button>
                  <button type="button" className="btn" onClick={() => { setSnmpModalDeviceId(actionMenuRow.id); setActionMenuRow(null); }}>
                    Ver inventário SNMP (walk)
                  </button>
                </>
              ) : null}
            </div>
          </div>
        </div>
      )}

      {reportModalDevice && <DeviceReportModal device={reportModalDevice} onClose={() => setReportModalDevice(null)} />}

    </>
  );
}
