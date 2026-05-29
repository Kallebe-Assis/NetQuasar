import { useMutation, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { DropdownMenu } from "./DropdownMenu";
import { apiFetch, downloadBlob } from "../lib/api";
import { displayAlertType, displaySeverity } from "../lib/alertLabels";
import {
  aggregatePingSamples,
  aggregateTelemetryFromSamples,
  buildDeviceCadastroRows,
  buildDeviceReportMainTable,
  buildFullDeviceReportCsv,
  cadastroPlainTextForClipboard,
  deviceShowsInterfaceMonitorSection,
  formatDelta,
  formatNum,
  groupOltInterfaceRows,
  interfaceMonitorRowsFromApi,
  interfaceSnapshotTableRows,
  parseTelemetryKPIs,
  formatCollectedPt,
  reportWindowIso,
  shortFmtIso,
  type PingHistorySample,
  type ReportPeriod,
  type TelemetryHistorySample,
} from "../lib/deviceReportHelpers";
import { formatSnmpMetricCell } from "../lib/formatDisplay";

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

function fmtVsolReportCell(v: unknown): string {
  if (v == null) return "—";
  if (typeof v === "number" && Number.isFinite(v)) return String(v);
  const t = String(v).trim();
  return t || "—";
}

function fmtZteCell(v: unknown): string {
  if (v == null) return "—";
  const s = String(v).trim();
  return s || "—";
}

export type DeviceReportTarget = {
  id: string;
  description: string;
  ip?: string | null;
  category: string;
  brand?: string | null;
};

function TimeSeriesChart({
  points,
  yUnit,
  ariaLabel,
}: {
  points: { iso: string; value: number | null }[];
  yUnit: string;
  ariaLabel: string;
}) {
  const vbW = 420;
  const vbH = 150;
  const padL = 44;
  const padR = 10;
  const padT = 14;
  const padB = 34;
  const plotW = vbW - padL - padR;
  const plotH = vbH - padT - padB;

  const valid = useMemo(
    () => points.filter((p): p is { iso: string; value: number } => p.value != null && Number.isFinite(p.value)),
    [points],
  );

  if (valid.length === 0) {
    return <p style={{ color: "var(--muted)", fontSize: 12 }}>Sem pontos numéricos no período seleccionado.</p>;
  }

  const ts = valid.map((p) => new Date(p.iso).getTime());
  const t0 = Math.min(...ts);
  const t1 = Math.max(...ts);
  const vals = valid.map((p) => p.value);
  const vMin = Math.min(...vals);
  const vMax = Math.max(...vals);
  const vSpan = Math.max(Number.EPSILON * 1000, vMax - vMin);
  const tSpan = Math.max(1, t1 - t0);
  const xAt = (t: number) => padL + ((t - t0) / tSpan) * plotW;
  const yAt = (v: number) => padT + plotH - ((v - vMin) / vSpan) * plotH;
  const yTicks = [vMin, (vMin + vMax) / 2, vMax].map((v) => ({
    v,
    y: yAt(v),
    label: vMax >= 100 || Math.abs(vMax - vMin) > 50 ? v.toFixed(0) : v.toFixed(1),
  }));
  const xLabelStart = shortFmtIso(valid[0].iso);
  const xLabelEnd = shortFmtIso(valid[valid.length - 1].iso);

  let pathD: string;
  if (valid.length === 1) {
    const cx = padL + plotW / 2;
    const cy = yAt(valid[0].value);
    pathD = `M ${cx - 0.01} ${cy} L ${cx + 0.01} ${cy}`;
  } else {
    pathD = valid.map((p, i) => `${i === 0 ? "M" : "L"} ${xAt(new Date(p.iso).getTime())} ${yAt(p.value)}`).join(" ");
  }

  return (
    <svg
      width="100%"
      style={{ maxWidth: "100%", height: "auto", display: "block" }}
      viewBox={`0 0 ${vbW} ${vbH}`}
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label={ariaLabel}
    >
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
      <path d={pathD} fill="none" stroke="currentColor" strokeWidth="1.75" vectorEffect="non-scaling-stroke" />
      {valid.length === 1 && (
        <circle cx={padL + plotW / 2} cy={yAt(valid[0].value)} r="4" fill="currentColor" />
      )}
      <text x={padL} y={vbH - 18} fill="var(--muted)" fontSize="9">
        {xLabelStart}
      </text>
      <text x={padL + plotW} y={vbH - 18} fill="var(--muted)" fontSize="9" textAnchor="end">
        {xLabelEnd}
      </text>
    </svg>
  );
}

type Props = {
  device: DeviceReportTarget | null;
  onClose: () => void;
};

export function DeviceReportModal({ device, onClose }: Props) {
  const [reportPeriod, setReportPeriod] = useState<ReportPeriod>("7d");
  const [reportTab, setReportTab] = useState<"dados" | "interface" | "graficos" | "outros">("dados");
  const [copyCadastroNote, setCopyCadastroNote] = useState<string | null>(null);
  const id = device?.id ?? "";

  const fullReport = useMutation({
    mutationFn: (devId: string) =>
      apiFetch<{ device_id: string; job_id?: string; status?: string; note?: string }>(
        `/api/v1/monitoring/full-report/devices/${devId}`,
        { method: "POST", json: {} },
      ),
    onSuccess: (_, devId) => {
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
  const pingAggCurr = useMemo(
    () => aggregatePingSamples(reportPingHistory.data?.samples ?? []),
    [reportPingHistory.data?.samples],
  );
  const pingAggPrev = useMemo(
    () => aggregatePingSamples(reportPingHistoryPrev.data?.samples ?? []),
    [reportPingHistoryPrev.data?.samples],
  );
  const telAggCurr = useMemo(
    () => aggregateTelemetryFromSamples(reportTelemetryHistory.data?.samples ?? []),
    [reportTelemetryHistory.data?.samples],
  );
  const telAggPrev = useMemo(
    () => aggregateTelemetryFromSamples(reportTelemetryHistoryPrev.data?.samples ?? []),
    [reportTelemetryHistoryPrev.data?.samples],
  );
  const alertCountCurr = reportAlertsHistory.data?.events?.length ?? 0;
  const alertCountPrev = reportAlertsHistoryPrev.data?.events?.length ?? 0;

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
        <h3 style={{ marginBottom: 6 }}>Relatório completo do equipamento</h3>
        <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
          <strong>{device.description}</strong> · <span className="mono">{device.ip ?? "sem IP"}</span> · {device.category}
          {device.brand ? (
            <>
              {" "}
              · marca <span className="mono">{device.brand}</span>
            </>
          ) : null}
        </p>
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
            Relatório actualizado com sucesso para este equipamento.
          </div>
        )}
        <div className="row no-print" style={{ gap: 8, flexWrap: "wrap", marginBottom: 8, alignItems: "center" }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Período dos gráficos e tabelas:</span>
          {(["24h", "7d", "30d"] as const).map((p) => (
            <button
              key={p}
              type="button"
              className={reportPeriod === p ? "btn btn--primary" : "btn"}
              style={{ fontSize: 11, padding: "4px 10px" }}
              onClick={() => setReportPeriod(p)}
            >
              {p === "24h" ? "Últimas 24 h" : p === "7d" ? "7 dias" : "30 dias"}
            </button>
          ))}
        </div>
        <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0, marginBottom: 8 }}>
          Actual: {shortFmtIso(reportWin.fromIso)} — {shortFmtIso(reportWin.toIso)} · Anterior (comparativo):{" "}
          {shortFmtIso(reportWin.prevFromIso)} — {shortFmtIso(reportWin.prevToIso)}
        </p>
        <div className="tabs" style={{ marginBottom: 10, flexWrap: "wrap" }}>
          <button type="button" className={reportTab === "dados" ? "active" : ""} onClick={() => setReportTab("dados")}>
            Dados
          </button>
          <button type="button" className={reportTab === "interface" ? "active" : ""} onClick={() => setReportTab("interface")}>
            Interface
          </button>
          <button type="button" className={reportTab === "graficos" ? "active" : ""} onClick={() => setReportTab("graficos")}>
            Gráficos
          </button>
          <button type="button" className={reportTab === "outros" ? "active" : ""} onClick={() => setReportTab("outros")}>
            Outros
          </button>
        </div>
        {reportTab === "dados" && (
        <>
        <div className="card" style={{ marginBottom: 10 }}>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8, marginBottom: 8 }}>
            <h4 style={{ margin: 0 }}>Dados cadastrais (ficha completa)</h4>
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
                      <td className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
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
          <h4 style={{ marginTop: 0 }}>Comparativo antes / depois (mesma duração)</h4>
          <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
            “Depois” = período seleccionado acima; “Antes” = intervalo imediatamente anterior.
          </p>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Métrica</th>
                  <th>Antes</th>
                  <th>Depois</th>
                  <th>Variação</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>Ping — latência média (ms)</td>
                  <td className="mono">{formatNum(pingAggPrev.avgLatency, 1)}</td>
                  <td className="mono">{formatNum(pingAggCurr.avgLatency, 1)}</td>
                  <td className="mono">{formatDelta(pingAggPrev.avgLatency, pingAggCurr.avgLatency, 1)}</td>
                </tr>
                <tr>
                  <td>Ping — latência máx. (ms)</td>
                  <td className="mono">{formatNum(pingAggPrev.maxLatency, 1)}</td>
                  <td className="mono">{formatNum(pingAggCurr.maxLatency, 1)}</td>
                  <td className="mono">{formatDelta(pingAggPrev.maxLatency, pingAggCurr.maxLatency, 1)}</td>
                </tr>
                <tr>
                  <td>Ping — taxa OK (%)</td>
                  <td className="mono">{pingAggPrev.okRatio != null ? formatNum(pingAggPrev.okRatio * 100, 1) : "—"}</td>
                  <td className="mono">{pingAggCurr.okRatio != null ? formatNum(pingAggCurr.okRatio * 100, 1) : "—"}</td>
                  <td className="mono">
                    {pingAggPrev.okRatio != null && pingAggCurr.okRatio != null
                      ? formatDelta(pingAggPrev.okRatio * 100, pingAggCurr.okRatio * 100, 1, " p.p.")
                      : "—"}
                  </td>
                </tr>
                <tr>
                  <td>Amostras de ping</td>
                  <td className="mono">{pingAggPrev.n}</td>
                  <td className="mono">{pingAggCurr.n}</td>
                  <td className="mono">{formatDelta(pingAggPrev.n, pingAggCurr.n, 0, "")}</td>
                </tr>
                <tr>
                  <td>CPU média (%)</td>
                  <td className="mono">{formatNum(telAggPrev.cpu, 1)}</td>
                  <td className="mono">{formatNum(telAggCurr.cpu, 1)}</td>
                  <td className="mono">{formatDelta(telAggPrev.cpu, telAggCurr.cpu, 1)}</td>
                </tr>
                <tr>
                  <td>Memória média (%)</td>
                  <td className="mono">{formatNum(telAggPrev.memory, 1)}</td>
                  <td className="mono">{formatNum(telAggCurr.memory, 1)}</td>
                  <td className="mono">{formatDelta(telAggPrev.memory, telAggCurr.memory, 1)}</td>
                </tr>
                <tr>
                  <td>Temperatura média (°C)</td>
                  <td className="mono">{formatNum(telAggPrev.temp, 1)}</td>
                  <td className="mono">{formatNum(telAggCurr.temp, 1)}</td>
                  <td className="mono">{formatDelta(telAggPrev.temp, telAggCurr.temp, 1)}</td>
                </tr>
                <tr>
                  <td>Alertas (instâncias no período)</td>
                  <td className="mono">{alertCountPrev}</td>
                  <td className="mono">{alertCountCurr}</td>
                  <td className="mono">{formatDelta(alertCountPrev, alertCountCurr, 0, "")}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div className="card" style={{ marginBottom: 10 }}>
          <h4 style={{ marginTop: 0 }}>Status e leituras colectadas</h4>
          <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
            Valores da última colecta (ping e telemetria) e outras leituras SNMP da mesma amostra, com descrição legível — sem identificadores técnicos de OID.
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

        {reportTab === "interface" && showIfaceMonitor && (
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
                  Última colecta (IF-MIB / ifXTable):{" "}
                  <span className="mono">{formatCollectedPt(String(reportInterfacesLatest.data?.collected_at ?? ""))}</span> · {ifaceTableRows.length}{" "}
                  interface(s).
                  {(device?.category ?? "").trim().toLowerCase() === "olt" && (
                    <span> · Contagens de ONUs por PON vêm das interfaces <code className="mono">GPON…ONU…</code> (oper status).</span>
                  )}
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
                  Sem tabela de interfaces nem snapshots no período. Use «Actualizar interfaces» no equipamento ou «Atualizar relatório agora» para gerar dados.
                </p>
              )
            )}
          </div>
        )}

        {reportTab === "interface" && (device?.category ?? "").trim().toLowerCase() === "olt" && (
          <div className="card" style={{ marginBottom: 10 }}>
            <h4 style={{ marginTop: 0 }}>PONs — total / online / offline por porta</h4>
            <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
              Dados do snapshot OLT (após «Actualizar interfaces» as contagens por PON são derivadas das interfaces <code className="mono">GPON…ONU…</code>).
            </p>
            {reportOltDevice.data?.olt_snapshot_at != null && String(reportOltDevice.data.olt_snapshot_at).length > 0 && (
              <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                Snapshot OLT: <span className="mono">{formatCollectedPt(String(reportOltDevice.data.olt_snapshot_at))}</span>
                {reportOltDevice.data.interface_collected_at != null && String(reportOltDevice.data.interface_collected_at).length > 0 ? (
                  <>
                    {" "}
                    · Interfaces: <span className="mono">{formatCollectedPt(String(reportOltDevice.data.interface_collected_at))}</span>
                  </>
                ) : null}
              </p>
            )}
            {reportOltDevice.isLoading && <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar snapshot OLT…</p>}
            {reportOltDevice.isError && <div className="msg msg--err">{(reportOltDevice.error as Error).message}</div>}
            {reportOltDevice.data && (
              <details className="collapsible-section" style={{ marginTop: 8 }}>
                <summary>
                  Tabela PONs{" "}
                  <span style={{ fontWeight: 400, color: "var(--muted)" }}>
                    ({Array.isArray(reportOltDevice.data.pons_table) ? (reportOltDevice.data.pons_table as unknown[]).length : 0})
                  </span>
                </summary>
                <div className="collapsible-section__body">
                  <div className="table-wrap">
                    <table style={{ fontSize: 11 }}>
                      <thead>
                        <tr>
                          <th>ID</th>
                          <th>Nome</th>
                          <th className="mono">RX PON</th>
                          <th className="mono">TX PON</th>
                          <th className="mono">Voltagem</th>
                          <th className="mono">Corrente</th>
                          <th className="mono">Temp.</th>
                          <th className="mono">Total</th>
                          <th className="mono">Online</th>
                          <th className="mono">Offline</th>
                          <th>Status</th>
                        </tr>
                      </thead>
                      <tbody>
                        {Array.isArray(reportOltDevice.data.pons_table)
                          ? (reportOltDevice.data.pons_table as Array<Record<string, unknown>>).map((p, i) => {
                              const fmtPonMetric = (v: unknown) =>
                                v != null && Number.isFinite(Number(v)) ? Number(v).toFixed(1) : "—";
                              return (
                                <tr key={`${String(p.id ?? i)}`}>
                                  <td className="mono">{String(p.id ?? "—")}</td>
                                  <td>{String(p.name ?? "—")}</td>
                                  <td className="mono">{fmtPonMetric(p.rx_dbm)}</td>
                                  <td className="mono">{fmtPonMetric(p.tx_dbm)}</td>
                                  <td className="mono">{fmtPonMetric(p.voltage)}</td>
                                  <td className="mono">{fmtPonMetric(p.current)}</td>
                                  <td className="mono">{fmtPonMetric(p.temperature)}</td>
                                  <td className="mono">{formatNum(Number.isFinite(Number(p.onu_total)) ? Number(p.onu_total) : null, 0)}</td>
                                  <td className="mono">{formatNum(Number.isFinite(Number(p.onu_online)) ? Number(p.onu_online) : null, 0)}</td>
                                  <td className="mono">{formatNum(Number.isFinite(Number(p.onu_offline)) ? Number(p.onu_offline) : null, 0)}</td>
                                  <td className="mono">{String(p.status ?? "—")}</td>
                                </tr>
                              );
                            })
                          : null}
                      </tbody>
                    </table>
                  </div>
                </div>
              </details>
            )}
            {reportOltDevice.data && Array.isArray(reportOltDevice.data.vsol_onu_table) && (reportOltDevice.data.vsol_onu_table as unknown[]).length > 0 && (
              <details className="collapsible-section" style={{ marginTop: 10 }}>
                <summary>
                  ONUs (MIB OLT){" "}
                  <span style={{ fontWeight: 400, color: "var(--muted)" }}>
                    ({(reportOltDevice.data.vsol_onu_table as unknown[]).length})
                  </span>
                </summary>
                <div className="collapsible-section__body">
                  <div className="table-wrap" style={{ maxHeight: 320, overflow: "auto" }}>
                    <table style={{ fontSize: 10 }}>
                      <thead>
                        <tr>
                          <th>PON</th>
                          <th>ONU</th>
                          <th>Perfil</th>
                          <th>Fase</th>
                          <th className="mono">RX ONU</th>
                          <th className="mono">TX ONU</th>
                          <th>Modelo</th>
                          <th>SN</th>
                        </tr>
                      </thead>
                      <tbody>
                        {(reportOltDevice.data.vsol_onu_table as Array<Record<string, unknown>>).map((u, i) => (
                          <tr key={`vsol-${i}`}>
                            <td className="mono">{fmtVsolReportCell(u.pon)}</td>
                            <td className="mono">{fmtVsolReportCell(u.onu)}</td>
                            <td>{fmtVsolReportCell(u.profile_name)}</td>
                            <td>{fmtVsolReportCell(u.phase_sta)}</td>
                            <td className="mono">{formatSnmpMetricCell(u.rx_pwr)}</td>
                            <td className="mono">{formatSnmpMetricCell(u.tx_pwr)}</td>
                            <td>{fmtVsolReportCell(u.model)}</td>
                            <td className="mono" style={{ wordBreak: "break-all", maxWidth: 120 }}>
                              {fmtVsolReportCell(u.serial)}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              </details>
            )}
            {reportOltDevice.data &&
              Array.isArray(reportOltDevice.data.pons_table) &&
              (reportOltDevice.data.pons_table as unknown[]).length === 0 && (
                <p style={{ fontSize: 12, color: "var(--muted)" }}>Sem linhas em «pons». Actualize interfaces ou snapshot OLT.</p>
              )}
            {reportOltDevice.data &&
              (Array.isArray(reportOltDevice.data.zte_onu_online_table) ||
                Array.isArray(reportOltDevice.data.zte_pon_status_table) ||
                Array.isArray(reportOltDevice.data.zte_transceiver_table)) && (
                <details className="collapsible-section" style={{ marginTop: 10 }}>
                  <summary>
                    ZTE MIB — tabelas coletadas
                    <span style={{ fontWeight: 400, color: "var(--muted)" }}>
                      {" "}
                      (
                      {(Array.isArray(reportOltDevice.data.zte_onu_online_table) ? (reportOltDevice.data.zte_onu_online_table as unknown[]).length : 0) +
                        (Array.isArray(reportOltDevice.data.zte_pon_status_table) ? (reportOltDevice.data.zte_pon_status_table as unknown[]).length : 0) +
                        (Array.isArray(reportOltDevice.data.zte_transceiver_table) ? (reportOltDevice.data.zte_transceiver_table as unknown[]).length : 0)}
                      )
                    </span>
                  </summary>
                  <div className="collapsible-section__body">
                    {String((reportOltDevice.data.summary as Record<string, unknown> | undefined)?.zte_walk_note ?? "").trim() ? (
                      <p className="msg msg--off" style={{ fontSize: 11 }}>
                        Nota walk: {String((reportOltDevice.data.summary as Record<string, unknown>).zte_walk_note)}
                      </p>
                    ) : null}
                    {(
                      [
                        ["ONU online (ZTE)", reportOltDevice.data.zte_onu_online_table],
                        ["Status PON (ZTE)", reportOltDevice.data.zte_pon_status_table],
                        ["Transceiver (ZTE)", reportOltDevice.data.zte_transceiver_table],
                      ] as const
                    ).map(([title, rows]) => {
                      const arr = Array.isArray(rows) ? (rows as Array<Record<string, unknown>>) : [];
                      if (arr.length === 0) return null;
                      return (
                        <div key={title}>
                          <h5 style={{ marginTop: 12, marginBottom: 6 }}>{title}</h5>
                          <div className="table-wrap" style={{ maxHeight: 220, overflow: "auto" }}>
                            <table style={{ fontSize: 10 }}>
                              <thead>
                                <tr>
                                  <th>Suffix</th>
                                  <th>Porta</th>
                                  <th>Valor</th>
                                  <th>Status</th>
                                  <th>Tipo</th>
                                  <th>OID</th>
                                </tr>
                              </thead>
                              <tbody>
                                {arr.map((r, i) => (
                                  <tr key={`${title}-${i}`}>
                                    <td className="mono">{fmtZteCell(r.suffix)}</td>
                                    <td>{fmtZteCell(r.if_name ?? (r.if_index != null ? `ifIndex ${String(r.if_index)}` : ""))}</td>
                                    <td className="mono">{fmtZteCell(r.value)}</td>
                                    <td>{fmtZteCell(r.value_label)}</td>
                                    <td className="mono">{fmtZteCell(r.type)}</td>
                                    <td className="mono" style={{ wordBreak: "break-all" }}>
                                      {fmtZteCell(r.oid)}
                                    </td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </details>
              )}
          </div>
        )}

        {reportTab === "graficos" && (
        <div className="report-chart-grid" style={{ marginTop: 10 }}>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>Histórico de latência (ping)</h4>
            <div style={{ color: "var(--text)" }}>
              <TimeSeriesChart points={pingChartPoints} yUnit="ms" ariaLabel="Latência de ping ao longo do tempo" />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              {reportPingHistory.data?.samples?.length ?? 0} amostras no período · eixo X: tempo da amostra · eixo Y: latência em milissegundos.
            </p>
          </div>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>CPU (histórico)</h4>
            <div style={{ color: "var(--text)" }}>
              <TimeSeriesChart points={cpuChartPoints} yUnit="%" ariaLabel="Percentagem de CPU ao longo do tempo" />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>{telemetrySorted.length} amostras · eixo Y: percentagem.</p>
          </div>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>Memória (histórico)</h4>
            <div style={{ color: "var(--text)" }}>
              <TimeSeriesChart points={memChartPoints} yUnit="%" ariaLabel="Percentagem de memória ao longo do tempo" />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>{telemetrySorted.length} amostras · eixo Y: percentagem.</p>
          </div>
          <div className="card report-chart-cell">
            <h4 style={{ marginTop: 0 }}>Temperatura (histórico)</h4>
            <div style={{ color: "var(--text)" }}>
              <TimeSeriesChart points={tempChartPoints} yUnit="°C" ariaLabel="Temperatura ao longo do tempo" />
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>{telemetrySorted.length} amostras · eixo Y: graus Celsius.</p>
          </div>
        </div>
        )}

        {reportTab === "outros" && (
        <>
        <div className="card" style={{ marginTop: 10 }}>
          <h4 style={{ marginTop: 0 }}>Inventário SNMP e detalhes técnicos</h4>
          {reportInventory.isLoading && <p>A carregar inventário…</p>}
          {reportInventory.isError && <div className="msg msg--err">{(reportInventory.error as Error).message}</div>}
          {reportInventory.data && (
            <pre className="mono" style={{ maxHeight: 220, overflow: "auto", fontSize: 10 }}>
              {JSON.stringify(reportInventory.data, null, 2)}
            </pre>
          )}
        </div>

        <div className="row" style={{ gap: 10, flexWrap: "wrap", marginTop: 10 }}>
          <div className="card" style={{ flex: "1 1 420px", margin: 0 }}>
            <h4 style={{ marginTop: 0 }}>Histórico de alertas</h4>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Quando</th>
                    <th>Sev.</th>
                    <th>Tipo</th>
                    <th>Mensagem</th>
                  </tr>
                </thead>
                <tbody>
                  {(reportAlertsHistory.data?.events ?? []).slice(0, 15).map((ev, i) => (
                    <tr key={`${String(ev.id ?? i)}`}>
                      <td className="mono">{String(ev.active_since ?? ev.created_at ?? "—")}</td>
                          <td>{displaySeverity(String(ev.severity ?? ""))}</td>
                      <td>{displayAlertType(String(ev.type ?? ev.event_type ?? ""))}</td>
                      <td>{String(ev.message ?? "—")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          <div className="card" style={{ flex: "1 1 420px", margin: 0 }}>
            <h4 style={{ marginTop: 0 }}>Resumo de interfaces no período</h4>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Total de snapshots salvos: {reportInterfacesHistory.data?.snapshots?.length ?? 0}
              {!showIfaceMonitor && " · Detalhe por interface aparece no relatório para Mikrotik (categoria ou marca) e Rádio."}
            </p>
          </div>
        </div>
        </>
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
