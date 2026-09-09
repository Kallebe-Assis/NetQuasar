import { useMutation, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { DropdownMenu } from "./DropdownMenu";
import { apiFetch, downloadBlob } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastLoading, toastOk } from "../lib/operationToast";
import {
  buildDeviceCadastroRows,
  buildDeviceReportMainTable,
  buildFullDeviceReportCsv,
  cadastroPlainTextForClipboard,
  chartPointStats,
  deviceShowsInterfaceMonitorSection,
  extractSystemInfo,
  formatNum,
  groupOltInterfaceRows,
  interfaceMonitorRowsFromApi,
  interfaceSnapshotTableRows,
  parseTelemetryKPIs,
  formatCollectedPt,
  isOltCategory,
  reportWindowIso,
  shortFmtIso,
  type PingHistorySample,
  type ReportPeriod,
  type TelemetryHistorySample,
} from "../lib/deviceReportHelpers";

function fmtIfaceOctCell(v: number | null | undefined, saturated?: boolean): string {
  if (saturated && v == null) return "— (32b)";
  if (v == null || !Number.isFinite(Number(v))) return "—";
  // Conversão solicitada: exibir octetos em Kb/s, Mb/s, Gb/s...
  return fmtIfaceBps(Number(v) * 8);
}

function fmtIfaceBps(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(Number(v)) || Number(v) < 0) return "—";
  const units = ["b/s", "Kb/s", "Mb/s", "Gb/s", "Tb/s"];
  let n = Number(v);
  let i = 0;
  while (n >= 1000 && i < units.length - 1) {
    n /= 1000;
    i++;
  }
  const digits = n >= 100 ? 0 : n >= 10 ? 1 : 2;
  return `${n.toFixed(digits)} ${units[i]}`;
}

function operStatusSpan(s: string | undefined | null) {
  const raw = String(s ?? "—").trim();
  if (raw === "" || raw === "0") return <span className="badge badge--off">—</span>;
  const x = raw.toLowerCase();
  const cls =
    x === "up" || x === "online" || x === "ok"
      ? "badge badge--ok"
      : x === "down" || x === "offline"
        ? "badge badge--err"
        : "badge badge--off";
  return <span className={cls}>{raw || "—"}</span>;
}

function adminStatusSpan(s: string | undefined | null) {
  const raw = String(s ?? "—").trim();
  if (raw === "" || raw === "0") return <span className="badge badge--off">—</span>;
  const x = raw.toLowerCase();
  const cls =
    x === "up" || x === "online" || x === "ok"
      ? "badge badge--ok"
      : x === "down" || x === "offline"
        ? "badge badge--err"
        : "badge badge--off";
  return <span className={cls}>{raw || "—"}</span>;
}

export type DeviceReportTarget = {
  id: string;
  description: string;
  ip?: string | null;
  category: string;
  brand?: string | null;
};

/** Uma série do gráfico (linha + preenchimento). `thresholdValue` desenha uma linha tracejada de
 * referência (ex.: limiar de alerta), rotulada com `thresholdLabel`. */
type ChartSeries = {
  points: { iso: string; value: number | null }[];
  color: string;
  label: string;
};

function seriesPath(valid: { t: number; value: number }[], xAt: (t: number) => number, yAt: (v: number) => number): string {
  if (valid.length === 1) {
    const cx = xAt(valid[0].t);
    const cy = yAt(valid[0].value);
    return `M ${cx - 0.01} ${cy} L ${cx + 0.01} ${cy}`;
  }
  return valid.map((p, i) => `${i === 0 ? "M" : "L"} ${xAt(p.t)} ${yAt(p.value)}`).join(" ");
}

/** Gráfico de série temporal com área preenchida (gradiente) — suporta 1-2 séries sobrepostas
 * (ex.: CPU + RAM no mesmo eixo, como no relatório de referência) e uma linha tracejada opcional
 * de limiar. */
function TimeSeriesChart({
  series,
  yUnit,
  ariaLabel,
  thresholdValue,
  thresholdLabel,
  gid,
}: {
  series: ChartSeries[];
  yUnit: string;
  ariaLabel: string;
  thresholdValue?: number | null;
  thresholdLabel?: string;
  gid: string;
}) {
  const vbW = 420;
  const vbH = 160;
  const padL = 44;
  const padR = 10;
  const padT = 14;
  const padB = 34;
  const plotW = vbW - padL - padR;
  const plotH = vbH - padT - padB;

  const validBySeries = useMemo(
    () =>
      series.map((s) =>
        s.points
          .filter((p): p is { iso: string; value: number } => p.value != null && Number.isFinite(p.value))
          .map((p) => ({ t: new Date(p.iso).getTime(), value: p.value })),
      ),
    [series],
  );
  const allValid = validBySeries.flat();

  if (allValid.length === 0) {
    return <p style={{ color: "var(--muted)", fontSize: 12 }}>Sem pontos numéricos no período seleccionado.</p>;
  }

  const t0 = Math.min(...allValid.map((p) => p.t));
  const t1 = Math.max(...allValid.map((p) => p.t));
  let vMin = Math.min(...allValid.map((p) => p.value), 0);
  let vMax = Math.max(...allValid.map((p) => p.value));
  if (thresholdValue != null && Number.isFinite(thresholdValue)) vMax = Math.max(vMax, thresholdValue);
  const vSpan = Math.max(Number.EPSILON * 1000, vMax - vMin) * 1.08;
  vMax = vMin + vSpan;
  const tSpan = Math.max(1, t1 - t0);
  const xAt = (t: number) => padL + ((t - t0) / tSpan) * plotW;
  const yAt = (v: number) => padT + plotH - ((v - vMin) / vSpan) * plotH;
  const yTicks = [vMin, (vMin + vMax) / 2, vMax].map((v) => ({
    v,
    y: yAt(v),
    label: vMax >= 100 || Math.abs(vMax - vMin) > 50 ? v.toFixed(0) : v.toFixed(1),
  }));
  const firstIso = series.find((s) => s.points.length > 0)?.points[0]?.iso;
  const lastPoints = [...series].reverse().find((s) => s.points.length > 0)?.points;
  const xLabelStart = firstIso ? shortFmtIso(firstIso) : "";
  const xLabelEnd = lastPoints?.length ? shortFmtIso(lastPoints[lastPoints.length - 1].iso) : "";

  return (
    <svg
      width="100%"
      style={{ maxWidth: "100%", height: "auto", display: "block" }}
      viewBox={`0 0 ${vbW} ${vbH}`}
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label={ariaLabel}
    >
      <defs>
        {series.map((s, i) => (
          <linearGradient key={s.label} id={`${gid}-grad-${i}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={s.color} stopOpacity="0.38" />
            <stop offset="100%" stopColor={s.color} stopOpacity="0.02" />
          </linearGradient>
        ))}
      </defs>
      <text x={padL} y={11} fill="var(--muted)" fontSize="10">
        {yUnit ? `Eixo Y: ${yUnit}` : "Eixo Y"}
      </text>
      <text x={padL} y={vbH - 6} fill="var(--muted)" fontSize="9">
        Tempo →
      </text>
      {yTicks.map((tk) => (
        <g key={tk.label + tk.y}>
          <line x1={padL} x2={padL + plotW} y1={tk.y} y2={tk.y} stroke="var(--border)" strokeOpacity={0.45} strokeDasharray="3 3" />
          <text x={4} y={tk.y + 3} fill="var(--muted)" fontSize="9">
            {tk.label}
          </text>
        </g>
      ))}
      <rect x={padL} y={padT} width={plotW} height={plotH} fill="transparent" stroke="var(--border)" strokeWidth="1" />
      {thresholdValue != null && Number.isFinite(thresholdValue) && thresholdValue >= vMin && thresholdValue <= vMax && (
        <g>
          <line
            x1={padL}
            x2={padL + plotW}
            y1={yAt(thresholdValue)}
            y2={yAt(thresholdValue)}
            stroke="var(--err, #dc2626)"
            strokeWidth="1"
            strokeDasharray="4 3"
          />
          <text x={padL + plotW} y={yAt(thresholdValue) - 3} fill="var(--err, #dc2626)" fontSize="8" textAnchor="end">
            {thresholdLabel ?? `Limiar (${thresholdValue})`}
          </text>
        </g>
      )}
      {series.map((s, i) => {
        const valid = validBySeries[i];
        if (valid.length === 0) return null;
        const linePath = seriesPath(valid, xAt, yAt);
        const baseY = padT + plotH;
        const areaPath =
          valid.length > 1
            ? `${linePath} L ${xAt(valid[valid.length - 1].t)} ${baseY} L ${xAt(valid[0].t)} ${baseY} Z`
            : "";
        return (
          <g key={s.label}>
            {areaPath && <path d={areaPath} fill={`url(#${gid}-grad-${i})`} stroke="none" />}
            <path d={linePath} fill="none" stroke={s.color} strokeWidth="1.75" vectorEffect="non-scaling-stroke" />
            {valid.length === 1 && <circle cx={xAt(valid[0].t)} cy={yAt(valid[0].value)} r="4" fill={s.color} />}
          </g>
        );
      })}
      <text x={padL} y={vbH - 18} fill="var(--muted)" fontSize="9">
        {xLabelStart}
      </text>
      <text x={padL + plotW} y={vbH - 18} fill="var(--muted)" fontSize="9" textAnchor="end">
        {xLabelEnd}
      </text>
      {series.length > 1 && (
        <g transform={`translate(${padL}, ${padT})`}>
          {series.map((s, i) => (
            <g key={s.label} transform={`translate(${i * 90}, 0)`}>
              <rect width="8" height="8" fill={s.color} rx="2" />
              <text x={12} y={7} fontSize="9" fill="var(--muted)">
                {s.label}
              </text>
            </g>
          ))}
        </g>
      )}
    </svg>
  );
}

function ChartStatsLegend({ stats, unit }: { stats: { min: number | null; avg: number | null; max: number | null }; unit: string }) {
  const fmt = (v: number | null) => (v == null ? "—" : `${v.toFixed(v >= 100 ? 0 : 1)}${unit}`);
  return (
    <div style={{ display: "flex", gap: 14, fontSize: 11, color: "var(--muted)", marginBottom: 4 }}>
      <span>Mín: <strong style={{ color: "var(--text)" }}>{fmt(stats.min)}</strong></span>
      <span>Média: <strong style={{ color: "var(--text)" }}>{fmt(stats.avg)}</strong></span>
      <span>Máx: <strong style={{ color: "var(--text)" }}>{fmt(stats.max)}</strong></span>
    </div>
  );
}

type Props = {
  device: DeviceReportTarget | null;
  onClose: () => void;
};

export function DeviceReportModal({ device, onClose }: Props) {
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const [reportPeriod, setReportPeriod] = useState<ReportPeriod>("7d");
  const [reportTab, setReportTab] = useState<"resumo" | "interfaces" | "graficos">("resumo");
  const [copyCadastroNote, setCopyCadastroNote] = useState<string | null>(null);
  const id = device?.id ?? "";

  const fullReport = useMutation({
    mutationFn: (devId: string) =>
      apiFetch<{ device_id: string; job_id?: string; status?: string; note?: string }>(
        `/api/v1/monitoring/full-report/devices/${devId}`,
        { method: "POST", json: {} },
      ),
    onMutate: () => {
      const loadingId = toastLoading(pushToast, "A executar relatório completo…");
      return { loadingId };
    },
    onSuccess: (data, devId, ctx) => {
      if (ctx?.loadingId) dismissToast(ctx.loadingId);
      const status = String(data?.status ?? "done").toLowerCase();
      const note = data?.note?.trim();
      if (status === "partial") {
        pushToast({ tone: "info", text: note || "Relatório completo concluído com avisos parciais." });
      } else {
        toastOk(pushToast, note || "Relatório completo concluído.");
      }
      void deviceCadastro.refetch();
      void reportPingLatest.refetch();
      void reportPingHistory.refetch();
      void reportPingHistoryPrev.refetch();
      void reportTelemetryLatest.refetch();
      void reportTelemetryHistory.refetch();
      void reportTelemetryHistoryPrev.refetch();
      void reportInterfacesHistory.refetch();
      void reportInterfacesLatest.refetch();
      void reportOltDevice.refetch();
      void reportInventory.refetch();
      void reportAlertsHistory.refetch();
      void reportAlertsHistoryPrev.refetch();
      void devId;
    },
    onError: (e, _devId, ctx) => {
      if (ctx?.loadingId) dismissToast(ctx.loadingId);
      toastErr(pushToast, e, "Falha ao executar relatório completo.");
    },
  });

  const reportPingLatest = useQuery({
    queryKey: ["device-report-ping-latest", id],
    enabled: !!id,
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/ping/devices/${id}/latest`),
  });
  const reportPingHistory = useQuery({
    queryKey: ["device-report-ping-history", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ device_id: id, limit: "500", from: w.fromIso, to: w.toIso });
      return apiFetch<{ samples: PingHistorySample[] }>(`/api/v1/ping/history?${q}`);
    },
  });
  const reportPingHistoryPrev = useQuery({
    queryKey: ["device-report-ping-history-prev", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ device_id: id, limit: "500", from: w.prevFromIso, to: w.prevToIso });
      return apiFetch<{ samples: PingHistorySample[] }>(`/api/v1/ping/history?${q}`);
    },
  });
  const reportTelemetryLatest = useQuery({
    queryKey: ["device-report-telemetry-latest", id],
    enabled: !!id,
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/telemetry/devices/${id}/latest`),
  });
  const reportTelemetryHistory = useQuery({
    queryKey: ["device-report-telemetry-history", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ device_id: id, limit: "500", from: w.fromIso, to: w.toIso });
      return apiFetch<{ samples: TelemetryHistorySample[] }>(`/api/v1/telemetry/history?${q}`);
    },
  });
  const reportTelemetryHistoryPrev = useQuery({
    queryKey: ["device-report-telemetry-history-prev", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ device_id: id, limit: "500", from: w.prevFromIso, to: w.prevToIso });
      return apiFetch<{ samples: TelemetryHistorySample[] }>(`/api/v1/telemetry/history?${q}`);
    },
  });
  const reportInterfacesHistory = useQuery({
    queryKey: ["device-report-interfaces-history", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ device_id: id, limit: "120", from: w.fromIso, to: w.toIso });
      return apiFetch<{ snapshots: Array<{ id: number; collected_at: string; interfaces?: unknown[] }> }>(
        `/api/v1/interfaces/history?${q}`,
      );
    },
  });
  const showIfaceMonitor = deviceShowsInterfaceMonitorSection(device?.category ?? "", device?.brand);
  const reportInterfacesLatest = useQuery({
    queryKey: ["device-report-interfaces-latest", id],
    enabled: !!id && showIfaceMonitor,
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/interfaces/devices/${id}`),
  });
  const reportOltDevice = useQuery({
    queryKey: ["device-report-olt-device", id],
    enabled: !!id && (device?.category ?? "").trim().toLowerCase() === "olt",
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/olt/devices/${id}`),
  });
  const reportInventory = useQuery({
    queryKey: ["device-report-inventory", id],
    enabled: !!id,
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/devices/${id}/snmp-inventory`),
  });
  const reportAlertsHistory = useQuery({
    queryKey: ["device-report-alerts-history", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ limit: "200", device_id: id, from: w.fromIso, to: w.toIso });
      return apiFetch<{ events: Array<Record<string, unknown>> }>(`/api/v1/alerts/history?${q}`);
    },
  });
  const reportAlertsHistoryPrev = useQuery({
    queryKey: ["device-report-alerts-history-prev", id, reportPeriod],
    enabled: !!id,
    queryFn: () => {
      const w = reportWindowIso(reportPeriod);
      const q = new URLSearchParams({ limit: "200", device_id: id, from: w.prevFromIso, to: w.prevToIso });
      return apiFetch<{ events: Array<Record<string, unknown>> }>(`/api/v1/alerts/history?${q}`);
    },
  });

  // Alerta activo de "possível reinício" (uptime baixo) para este equipamento — mostra a
  // bolinha colorida + "ALERTA UPTIME" na barra de resumo, ao lado do valor de uptime. Não dá
  // pra filtrar por device_id no /alerts/active (só severity/type/limit), então filtra-se aqui.
  const reportUptimeAlert = useQuery({
    queryKey: ["device-report-uptime-alert", id],
    enabled: !!id,
    staleTime: 30_000,
    queryFn: () => apiFetch<{ alerts: Array<{ device_id?: string; severity?: string }> }>("/api/v1/alerts/active?type=uptime_restart_low&limit=500"),
  });
  const uptimeAlertSeverity = reportUptimeAlert.data?.alerts?.find((a) => a.device_id === id)?.severity ?? null;

  const deviceCadastro = useQuery({
    queryKey: ["device-report-cadastro", id],
    enabled: !!id,
    queryFn: () => apiFetch<Record<string, unknown>>(`/api/v1/devices/${id}`),
  });
  const cadastroRows = useMemo(() => {
    const d = deviceCadastro.data;
    if (!d || typeof d !== "object") return [];
    return buildDeviceCadastroRows(d);
  }, [deviceCadastro.data]);

  const reportWin = useMemo(() => reportWindowIso(reportPeriod), [reportPeriod]);

  const telemetrySorted = useMemo(
    () =>
      [...(reportTelemetryHistory.data?.samples ?? [])].sort(
        (a, b) => new Date(a.collected_at).getTime() - new Date(b.collected_at).getTime(),
      ),
    [reportTelemetryHistory.data?.samples],
  );
  const telemetryKPIs = useMemo(() => telemetrySorted.map((s) => parseTelemetryKPIs(s)), [telemetrySorted]);
  const latestTelemetryKPI = useMemo(() => {
    if (!telemetryKPIs.length) return { cpu: null, memory: null, temp: null };
    return telemetryKPIs[telemetryKPIs.length - 1];
  }, [telemetryKPIs]);

  const pingChartPoints = useMemo(
    () =>
      [...(reportPingHistory.data?.samples ?? [])]
        .sort((a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime())
        .map((s) => ({ iso: s.checked_at, value: typeof s.latency_ms === "number" ? s.latency_ms : null })),
    [reportPingHistory.data?.samples],
  );
  const cpuChartPoints = useMemo(
    () => telemetrySorted.map((s) => ({ iso: s.collected_at, value: parseTelemetryKPIs(s).cpu })),
    [telemetrySorted],
  );
  const memChartPoints = useMemo(
    () => telemetrySorted.map((s) => ({ iso: s.collected_at, value: parseTelemetryKPIs(s).memory })),
    [telemetrySorted],
  );
  const tempChartPoints = useMemo(
    () => telemetrySorted.map((s) => ({ iso: s.collected_at, value: parseTelemetryKPIs(s).temp })),
    [telemetrySorted],
  );

  const mainReportRows = useMemo(() => {
    const tel = reportTelemetryLatest.data as Record<string, unknown> | undefined;
    const telShape =
      tel && tel.metrics != null && typeof tel.metrics === "object"
        ? { collected_at: String(tel.collected_at ?? ""), metrics: tel.metrics as Record<string, unknown> }
        : null;
    return buildDeviceReportMainTable({
      pingLatest: reportPingLatest.data ?? null,
      telemetryLatest: telShape,
      fallbackKpis: latestTelemetryKPI,
    });
  }, [reportPingLatest.data, reportTelemetryLatest.data, latestTelemetryKPI]);

  // Cabeçalho do relatório (status/IP/modelo/POP/uptime) — sysName/uptime vêm da telemetria mais
  // recente (extractSystemInfo), o resto do cadastro (GET /devices/:id).
  const sysInfo = useMemo(() => {
    const tel = reportTelemetryLatest.data as Record<string, unknown> | undefined;
    const m = tel?.metrics;
    return extractSystemInfo(m && typeof m === "object" ? (m as Record<string, unknown>) : undefined);
  }, [reportTelemetryLatest.data]);
  const pingOk = reportPingLatest.data?.ok;
  const cadastro = deviceCadastro.data;
  const popName = cadastro && typeof cadastro === "object" ? String((cadastro as Record<string, unknown>).pop_name ?? "").trim() : "";
  const cpuStats = useMemo(() => chartPointStats(cpuChartPoints), [cpuChartPoints]);
  const memStats = useMemo(() => chartPointStats(memChartPoints), [memChartPoints]);
  const tempStats = useMemo(() => chartPointStats(tempChartPoints), [tempChartPoints]);
  const pingStats = useMemo(() => chartPointStats(pingChartPoints), [pingChartPoints]);

  const ifaceTableRows = useMemo(
    () => interfaceMonitorRowsFromApi(reportInterfacesLatest.data),
    [reportInterfacesLatest.data],
  );
  const oltIfaceGroups = useMemo(() => {
    const cat = (device?.category ?? "").trim().toLowerCase();
    if (cat !== "olt" || !ifaceTableRows.some((r) => String(r.olt_iface_kind ?? "").length > 0)) return null;
    return groupOltInterfaceRows(ifaceTableRows);
  }, [device?.category, ifaceTableRows]);
  const latestIfaceSnapshot = useMemo(() => {
    const snaps = [...(reportInterfacesHistory.data?.snapshots ?? [])].sort(
      (a, b) => new Date(b.collected_at).getTime() - new Date(a.collected_at).getTime(),
    );
    for (const s of snaps) {
      if (Array.isArray(s.interfaces) && s.interfaces.length > 0) {
        return { collected_at: s.collected_at, rows: interfaceSnapshotTableRows(s.interfaces) };
      }
    }
    return null;
  }, [reportInterfacesHistory.data?.snapshots]);
  const ifaceRichRows = useMemo(
    () =>
      ifaceTableRows.filter(
        (r) =>
          r.in_bps != null ||
          r.out_bps != null ||
          r.in_octets != null ||
          r.out_octets != null ||
          String(r.admin_status ?? "").trim() !== "" ||
          String(r.oper_status ?? "").trim() !== "",
      ).length,
    [ifaceTableRows],
  );
  const ifaceLooksIncomplete = ifaceTableRows.length > 0 && ifaceRichRows === 0;

  if (!device) return null;

  const refetchAll = () => {
    deviceCadastro.refetch();
    reportPingLatest.refetch();
    reportPingHistory.refetch();
    reportPingHistoryPrev.refetch();
    reportTelemetryLatest.refetch();
    reportTelemetryHistory.refetch();
    reportTelemetryHistoryPrev.refetch();
    reportInterfacesHistory.refetch();
    reportInterfacesLatest.refetch();
    reportOltDevice.refetch();
    reportInventory.refetch();
    reportAlertsHistory.refetch();
    reportAlertsHistoryPrev.refetch();
  };

  return (
    <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal modal--wide device-report-print" onClick={(e) => e.stopPropagation()}>
        <h3 style={{ marginBottom: 6 }}>
          Relatório do equipamento: <span className="mono">{device.description}</span>
        </h3>
        <div className="device-report-summary-bar">
          {typeof pingOk === "boolean" ? (
            <span className={`badge ${pingOk ? "badge--ok" : "badge--err"}`}>● {pingOk ? "ONLINE" : "OFFLINE"}</span>
          ) : (
            <span className="badge badge--off">● —</span>
          )}
          {device.ip ? (
            <span>
              IP: <strong>{device.ip}</strong>
            </span>
          ) : null}
          {cadastro?.model || device.brand ? (
            <span>
              Modelo: <strong>{String(cadastro?.model ?? "—")}</strong>
              {device.brand ? ` (${device.brand})` : ""}
            </span>
          ) : null}
          {popName ? (
            <span>
              POP: <strong>{popName}</strong>
            </span>
          ) : null}
          {sysInfo.uptime ? (
            <span>
              Uptime: <strong className="mono">{sysInfo.uptime}</strong>
            </span>
          ) : null}
          {uptimeAlertSeverity ? (
            <span className={`badge ${uptimeAlertSeverity === "critical" ? "badge--err" : uptimeAlertSeverity === "warning" ? "badge--warn" : "badge--off"}`}>
              ● ALERTA UPTIME
            </span>
          ) : null}
        </div>
        <div className="row no-print" style={{ gap: 8, flexWrap: "wrap", marginBottom: 10, alignItems: "center" }}>
          <button type="button" className="btn btn--primary" disabled={fullReport.isPending} onClick={() => fullReport.mutate(device.id)}>
            Atualizar relatório agora
          </button>
          <button type="button" className="btn btn--icon-menu" title="Recarregar dados da tela" aria-label="Recarregar dados da tela" onClick={() => refetchAll()}>
            ⟳
          </button>
          <DropdownMenu
            key={id || "export"}
            align="start"
            className="action-menu"
            trigger={({ toggle, open }) => (
              <button
                type="button"
                className="btn"
                aria-haspopup="menu"
                aria-expanded={open}
                onClick={toggle}
              >
                Exportar ▾
              </button>
            )}
          >
            {({ close }) => (
              <>
                <button
                  type="button"
                  className="action-menu__item"
                  role="menuitem"
                  onClick={() => {
                    const csv = buildFullDeviceReportCsv({
                      device,
                      period: reportPeriod,
                      win: reportWin,
                      pingLatest: reportPingLatest.data,
                      telemetryLatest: reportTelemetryLatest.data,
                      pingCurr: reportPingHistory.data?.samples ?? [],
                      pingPrev: reportPingHistoryPrev.data?.samples ?? [],
                      telCurr: reportTelemetryHistory.data?.samples ?? [],
                      telPrev: reportTelemetryHistoryPrev.data?.samples ?? [],
                      alertsCurr: reportAlertsHistory.data?.events ?? [],
                      alertsPrev: reportAlertsHistoryPrev.data?.events ?? [],
                      snapshots: reportInterfacesHistory.data?.snapshots ?? [],
                      inventory: reportInventory.data,
                      interfacesLatest: showIfaceMonitor ? (reportInterfacesLatest.data ?? null) : null,
                      oltDeviceLatest:
                        (device?.category ?? "").trim().toLowerCase() === "olt" ? (reportOltDevice.data ?? null) : null,
                    });
                    downloadBlob(
                      `relatorio_equipamento_${device.id.slice(0, 8)}_${reportPeriod}.csv`,
                      new Blob(["\uFEFF", csv], { type: "text/csv;charset=utf-8" }),
                    );
                    close();
                  }}
                >
                  CSV
                </button>
                <button
                  type="button"
                  className="action-menu__item"
                  role="menuitem"
                  onClick={() => {
                    close();
                    document.body.classList.add("print-device-report");
                    requestAnimationFrame(() => {
                      window.print();
                      window.setTimeout(() => document.body.classList.remove("print-device-report"), 800);
                    });
                  }}
                >
                  PDF / Imprimir
                </button>
              </>
            )}
          </DropdownMenu>
        </div>
        {fullReport.data && (
          <div className="msg msg--ok" style={{ marginBottom: 8 }}>
            Relatório atualizado com sucesso para este equipamento.
          </div>
        )}
        <div className="row no-print" style={{ gap: 8, flexWrap: "wrap", marginBottom: 8, alignItems: "center" }}>
          {reportTab === "graficos" && (
            <>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Período dos gráficos:</span>
              {(["24h", "7d", "30d"] as const).map((p) => (
                <button
                  key={p}
                  type="button"
                  className={reportPeriod === p ? "btn btn--primary" : "btn"}
                  style={{ fontSize: 11, padding: "4px 10px" }}
                  onClick={() => setReportPeriod(p)}
                >
                  {p === "24h" ? "24 h" : p === "7d" ? "7 dias" : "30 dias"}
                </button>
              ))}
            </>
          )}
        </div>
        <div className="tabs" style={{ marginBottom: 10, flexWrap: "wrap" }}>
          <button type="button" className={reportTab === "resumo" ? "active" : ""} onClick={() => setReportTab("resumo")}>
            Resumo
          </button>
          <button type="button" className={reportTab === "interfaces" ? "active" : ""} onClick={() => setReportTab("interfaces")}>
            Interfaces
          </button>
          <button type="button" className={reportTab === "graficos" ? "active" : ""} onClick={() => setReportTab("graficos")}>
            Gráficos
          </button>
        </div>
        {reportTab === "resumo" && (
        <>
        <div className="device-report-kpi-grid" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))", gap: 10, marginBottom: 12 }}>
          {mainReportRows.slice(0, 5).map((r) => (
            <div key={r.description} className="card" style={{ padding: "10px 12px", margin: 0 }}>
              <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 4 }}>{r.description}</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{r.value}</div>
            </div>
          ))}
        </div>
        {isOltCategory(device.category) && reportOltDevice.data && (
          <div className="card" style={{ marginBottom: 10 }}>
            <h4 style={{ marginTop: 0 }}>ONUs por porta PON</h4>
            {reportOltDevice.data?.olt_snapshot_at != null && String(reportOltDevice.data.olt_snapshot_at).length > 0 && (
              <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                Última coleta: {formatCollectedPt(String(reportOltDevice.data.olt_snapshot_at))}
              </p>
            )}
            {reportOltDevice.isLoading && <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p>}
            {reportOltDevice.isError && <div className="msg msg--err">{(reportOltDevice.error as Error).message}</div>}
            <div className="table-wrap">
              <table style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>PON</th>
                    <th>Nome</th>
                    <th className="mono">Total</th>
                    <th className="mono">Online</th>
                    <th className="mono">Offline</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {Array.isArray(reportOltDevice.data.pons_table)
                    ? (reportOltDevice.data.pons_table as Array<Record<string, unknown>>).map((p, i) => (
                        <tr key={`${String(p.id ?? i)}`}>
                          <td>{String(p.id ?? "—")}</td>
                          <td>{String(p.name ?? "—")}</td>
                          <td className="mono">{formatNum(Number.isFinite(Number(p.onu_total)) ? Number(p.onu_total) : null, 0)}</td>
                          <td className="mono">{formatNum(Number.isFinite(Number(p.onu_online)) ? Number(p.onu_online) : null, 0)}</td>
                          <td className="mono">{formatNum(Number.isFinite(Number(p.onu_offline)) ? Number(p.onu_offline) : null, 0)}</td>
                          <td>{String(p.status ?? "—")}</td>
                        </tr>
                      ))
                    : null}
                </tbody>
              </table>
            </div>
            {Array.isArray(reportOltDevice.data.pons_table) && (reportOltDevice.data.pons_table as unknown[]).length === 0 && (
              <p style={{ fontSize: 12, color: "var(--muted)", marginBottom: 0 }}>Sem dados de PON. Atualize o snapshot da OLT.</p>
            )}
          </div>
        )}
        <div className="card" style={{ marginBottom: 10 }}>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8, marginBottom: 8 }}>
            <h4 style={{ margin: 0 }}>Ficha do equipamento</h4>
            <span className="no-print">
            <button
              type="button"
              className="btn"
              disabled={!cadastroRows.length}
              onClick={() => {
                const text = cadastroPlainTextForClipboard(cadastroRows);
                void navigator.clipboard.writeText(text).then(
                  () => {
                    setCopyCadastroNote("Copiado para a área de transferência.");
                    window.setTimeout(() => setCopyCadastroNote(null), 10_000);
                  },
                  () => setCopyCadastroNote("Não foi possível copiar (permissão do navegador)."),
                );
              }}
            >
              Copiar tudo
            </button>
            </span>
          </div>
          {copyCadastroNote && (
            <p className="msg msg--ok" style={{ marginTop: 0, marginBottom: 8, fontSize: 12 }}>
              {copyCadastroNote}
            </p>
          )}
          {deviceCadastro.isLoading && <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar ficha do equipamento…</p>}
          {deviceCadastro.isError && <div className="msg msg--err">{(deviceCadastro.error as Error).message}</div>}
          {deviceCadastro.data && (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th style={{ textAlign: "left", width: "38%" }}>Campo</th>
                    <th style={{ textAlign: "left" }}>Valor</th>
                  </tr>
                </thead>
                <tbody>
                  {cadastroRows.map((r, idx) => (
                    <tr key={`${idx}-${r.label}`}>
                      <td>{r.label}</td>
                      <td style={{ fontSize: 13, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
                        {r.value}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
        <div className="card" style={{ marginBottom: 10 }}>
          <h4 style={{ marginTop: 0 }}>Leituras actuais</h4>
          <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
            Última coleta de ping e telemetria — valores em tempo real do equipamento.
          </p>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Descrição</th>
                  <th>Valor</th>
                </tr>
              </thead>
              <tbody>
                {mainReportRows.map((r, i) => (
                  <tr key={`${i}-${r.description}`}>
                    <td>{r.description}</td>
                    <td className="mono">{r.value}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        </>
        )}

        {reportTab === "interfaces" && showIfaceMonitor && (
          <div className="card" style={{ marginBottom: 10 }}>
            <h4 style={{ marginTop: 0 }}>Interfaces (rede)</h4>
            {reportInterfacesLatest.isLoading && <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar última colecta de interfaces…</p>}
            {reportInterfacesLatest.isError && (
              <div className="msg msg--err">{(reportInterfacesLatest.error as Error).message}</div>
            )}
            {ifaceTableRows.length > 0 ? (
              <>
                {ifaceLooksIncomplete && (
                  <p className="msg msg--off" style={{ fontSize: 11 }}>
                    A coleta mais recente trouxe poucas métricas de consumo/status. Abaixo também mostramos a última amostra do período para comparação.
                  </p>
                )}
                <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                  Última coleta:{" "}
                  <span>{formatCollectedPt(String(reportInterfacesLatest.data?.collected_at ?? ""))}</span> · {ifaceTableRows.length}{" "}
                  interface(s)
                </p>
                {oltIfaceGroups ? (
                  <>
                    {(
                      [
                        ["Gestão e VLANs (GE, VLAN)", oltIfaceGroups.geVlan],
                        ["Portas PON (GPONx/y)", oltIfaceGroups.pon],
                        ["ONUs (uma linha por ONU)", oltIfaceGroups.onu],
                        ["Outras interfaces", oltIfaceGroups.other],
                      ] as const
                    ).map(([title, rows]) => {
                      if (rows.length === 0) return null;
                      const tableBlock = (
                        <div className="table-wrap" style={{ maxWidth: "100%", overflowX: "hidden" }}>
                          <table style={{ fontSize: 10, width: "100%", tableLayout: "fixed" }}>
                            <thead>
                              <tr>
                                <th>Idx</th>
                                <th style={{ width: "26%" }}>Nome</th>
                                <th>Admin</th>
                                <th>Oper</th>
                                <th className="mono">Entrada</th>
                                <th className="mono">Saída</th>
                                <th className="mono">RX tráfego</th>
                                <th className="mono">TX tráfego</th>
                                <th className="mono">RX dBm</th>
                                <th className="mono">TX dBm</th>
                              </tr>
                            </thead>
                            <tbody>
                              {rows.map((r) => (
                                <tr key={r.if_index}>
                                  <td className="mono">{r.if_index ?? "—"}</td>
                                  <td style={{ wordBreak: "break-word", overflowWrap: "anywhere" }}>{String(r.display_name ?? r.if_name ?? r.descr ?? "—").trim() || "—"}</td>
                                  <td>{adminStatusSpan(r.admin_status)}</td>
                                  <td>{operStatusSpan(r.oper_status)}</td>
                                  <td className="mono">{fmtIfaceOctCell(r.in_octets, r.octets_saturated_32bit)}</td>
                                  <td className="mono">{fmtIfaceOctCell(r.out_octets, r.octets_saturated_32bit)}</td>
                                  <td className="mono">{fmtIfaceBps(r.in_bps)}</td>
                                  <td className="mono">{fmtIfaceBps(r.out_bps)}</td>
                                  <td className="mono">
                                    {r.rx_dbm != null && Number.isFinite(Number(r.rx_dbm)) ? Number(r.rx_dbm).toFixed(1) : "—"}
                                  </td>
                                  <td className="mono">
                                    {r.tx_dbm != null && Number.isFinite(Number(r.tx_dbm)) ? Number(r.tx_dbm).toFixed(1) : "—"}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      );
                      if (title.startsWith("ONUs (")) {
                        return (
                          <details key={title} className="collapsible-section" style={{ marginTop: 10 }}>
                            <summary>
                              {title}{" "}
                              <span style={{ fontWeight: 400, color: "var(--muted)" }}>({rows.length})</span>
                            </summary>
                            <div className="collapsible-section__body">{tableBlock}</div>
                          </details>
                        );
                      }
                      return (
                        <div key={title}>
                          <h5 style={{ marginTop: 14, marginBottom: 6 }}>{title}</h5>
                          {tableBlock}
                        </div>
                      );
                    })}
                  </>
                ) : (
                  <div className="table-wrap" style={{ maxWidth: "100%", overflowX: "hidden" }}>
                    <table style={{ fontSize: 10, width: "100%", tableLayout: "fixed" }}>
                      <thead>
                        <tr>
                          <th>Idx</th>
                          <th style={{ width: "26%" }}>Nome</th>
                          <th>Admin</th>
                          <th>Oper</th>
                          <th className="mono">Entrada</th>
                          <th className="mono">Saída</th>
                          <th className="mono">RX tráfego</th>
                          <th className="mono">TX tráfego</th>
                          <th className="mono">RX dBm</th>
                          <th className="mono">TX dBm</th>
                        </tr>
                      </thead>
                      <tbody>
                        {ifaceTableRows.map((r) => (
                          <tr key={r.if_index}>
                            <td className="mono">{r.if_index ?? "—"}</td>
                            <td style={{ wordBreak: "break-word", overflowWrap: "anywhere" }}>{String(r.display_name ?? r.if_name ?? r.descr ?? "—").trim() || "—"}</td>
                            <td>{adminStatusSpan(r.admin_status)}</td>
                            <td>{operStatusSpan(r.oper_status)}</td>
                            <td className="mono">{fmtIfaceOctCell(r.in_octets, r.octets_saturated_32bit)}</td>
                            <td className="mono">{fmtIfaceOctCell(r.out_octets, r.octets_saturated_32bit)}</td>
                            <td className="mono">{fmtIfaceBps(r.in_bps)}</td>
                            <td className="mono">{fmtIfaceBps(r.out_bps)}</td>
                            <td className="mono">{r.rx_dbm != null && Number.isFinite(Number(r.rx_dbm)) ? Number(r.rx_dbm).toFixed(1) : "—"}</td>
                            <td className="mono">{r.tx_dbm != null && Number.isFinite(Number(r.tx_dbm)) ? Number(r.tx_dbm).toFixed(1) : "—"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                {ifaceLooksIncomplete && latestIfaceSnapshot && (
                  <>
                    <h5 style={{ marginTop: 14, marginBottom: 6 }}>Fallback do último snapshot do período</h5>
                    <div className="table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>Descrição</th>
                            <th>Valor colectado</th>
                          </tr>
                        </thead>
                        <tbody>
                          {latestIfaceSnapshot.rows.map((r, i) => (
                            <tr key={`if-fallback-${i}-${r.description}`}>
                              <td>{r.description}</td>
                              <td className="mono">{r.value}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                )}
              </>
            ) : latestIfaceSnapshot ? (
              <>
                <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                  Última amostra no período (formato bruto): {shortFmtIso(latestIfaceSnapshot.collected_at)} · {latestIfaceSnapshot.rows.length} leitura(s).
                </p>
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Descrição</th>
                        <th>Valor colectado</th>
                      </tr>
                    </thead>
                    <tbody>
                      {latestIfaceSnapshot.rows.map((r, i) => (
                        <tr key={`if-${i}-${r.description}`}>
                          <td>{r.description}</td>
                          <td className="mono">{r.value}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              !reportInterfacesLatest.isLoading && (
                <p style={{ color: "var(--muted)", fontSize: 12 }}>
                  Sem tabela de interfaces nem snapshots no período. Use «Atualizar interfaces» no equipamento ou «Atualizar relatório agora» para gerar dados.
                </p>
              )
            )}
          </div>
        )}

        {reportTab === "graficos" && (
        <div className="report-chart-grid" style={{ marginTop: 10 }}>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>Latência de ping (histórico)</h4>
            <ChartStatsLegend stats={pingStats} unit=" ms" />
            <div style={{ color: "var(--accent, #58a6ff)" }}>
              <TimeSeriesChart
                gid="ping"
                series={[{ points: pingChartPoints, color: "#58a6ff", label: "Latência" }]}
                yUnit="ms"
                ariaLabel="Latência de ping ao longo do tempo"
              />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              {reportPingHistory.data?.samples?.length ?? 0} amostras no período.
            </p>
          </div>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>Telemetria (CPU / RAM)</h4>
            <div style={{ display: "flex", gap: 16, marginBottom: 4 }}>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>
                CPU actual: <strong style={{ color: "var(--text)" }}>{formatNum(latestTelemetryKPI.cpu, 0, "%")}</strong>
              </div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>
                RAM actual: <strong style={{ color: "var(--text)" }}>{formatNum(latestTelemetryKPI.memory, 0, "%")}</strong>
              </div>
            </div>
            <div>
              <TimeSeriesChart
                gid="cpumem"
                series={[
                  { points: cpuChartPoints, color: "#22c55e", label: "CPU" },
                  { points: memChartPoints, color: "#eab308", label: "RAM" },
                ]}
                yUnit="%"
                ariaLabel="Percentagem de CPU e memória ao longo do tempo"
              />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              CPU — mín {formatNum(cpuStats.min, 0, "%")} · média {formatNum(cpuStats.avg, 0, "%")} · máx {formatNum(cpuStats.max, 0, "%")}
              {" · "}RAM — mín {formatNum(memStats.min, 0, "%")} · média {formatNum(memStats.avg, 0, "%")} · máx {formatNum(memStats.max, 0, "%")}
            </p>
          </div>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>Temperatura do sistema (histórico)</h4>
            <ChartStatsLegend stats={tempStats} unit="°C" />
            <div style={{ color: "#f97316" }}>
              <TimeSeriesChart
                gid="temp"
                series={[{ points: tempChartPoints, color: "#f97316", label: "Temperatura" }]}
                yUnit="°C"
                ariaLabel="Temperatura ao longo do tempo"
              />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>{telemetrySorted.length} amostras no período.</p>
          </div>
        </div>
        )}
        <div className="row no-print" style={{ marginTop: 12 }}>
          <button type="button" className="btn" onClick={() => onClose()}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}
