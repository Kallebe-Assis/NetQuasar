import { useMutation, useQuery } from "@tanstack/react-query";
import { createPortal } from "react-dom";
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { FileDown, FileText, Eye, Send, X } from "lucide-react";
import { PageCountPill } from "../components/PageCountPill";
import { InfoHint } from "../components/InfoHint";
import { formatBngDateTime } from "../lib/bngDisplay";
import { isAdminUser } from "../lib/auth";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import {
  downloadSystemReportCsv,
  fetchSystemReport,
  fetchSystemReportCatalog,
  sendSystemReportTelegram,
  summaryEntries,
  type ConnectionsReportOptions,
  type EquipmentByPopReportOptions,
  type OltOverviewReportOptions,
  type SystemReportId,
  type SystemReportPayload,
} from "../lib/systemReports";
import { apiFetch } from "../lib/api";
import { SystemReportInfoTooltip } from "../lib/systemReportInfo";

const DEFAULT_EQUIP_POP_OPTS: EquipmentByPopReportOptions = {
  include_without_pop: false,
  include_pop_coordinates: false,
};

const DEFAULT_CONNECTIONS_OPTS: ConnectionsReportOptions = {
  mode: "detailed",
  source: "connections",
};

const DEFAULT_OLT_OPTS: OltOverviewReportOptions = {
  period: "7d",
};

type ReportVariant = "screen" | "print";

function ChartDataTable({
  title,
  columns,
  rows,
}: {
  title: string;
  columns: string[];
  rows: (string | number | null)[][];
}) {
  if (rows.length === 0) return null;
  return (
    <div className="card system-report-chart-table" style={{ padding: 12, marginTop: 12 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>{title}</h3>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={i}>
                {row.map((cell, j) => (
                  <td key={j}>{cell ?? "—"}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ConnectionsOptionsPanel({
  value,
  onChange,
  bngDevices,
}: {
  value: ConnectionsReportOptions;
  onChange: (next: ConnectionsReportOptions) => void;
  bngDevices: Array<{ id: string; description: string }>;
}) {
  const source = value.source ?? "connections";
  const mode = value.mode ?? "detailed";
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, fontSize: 13 }}>
      <fieldset style={{ border: "none", margin: 0, padding: 0 }}>
        <legend style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Formato</legend>
        <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", marginBottom: 6 }}>
          <input
            type="radio"
            name="conn-mode"
            checked={mode === "summary"}
            onChange={() => onChange({ ...value, mode: "summary" })}
          />
          Resumido (totais por tipo, meio, plano…)
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
          <input
            type="radio"
            name="conn-mode"
            checked={mode === "detailed"}
            onChange={() => onChange({ ...value, mode: "detailed" })}
          />
          Detalhado (cada login)
        </label>
      </fieldset>
      <fieldset style={{ border: "none", margin: 0, padding: 0 }}>
        <legend style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Fonte de dados</legend>
        <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", marginBottom: 6 }}>
          <input
            type="radio"
            name="conn-source"
            checked={source === "connections"}
            onChange={() => onChange({ ...value, source: "connections", bng_device_id: undefined })}
          />
          Cadastro de conexões
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
          <input
            type="radio"
            name="conn-source"
            checked={source === "bng_cache"}
            onChange={() =>
              onChange({
                ...value,
                source: "bng_cache",
                bng_device_id: value.bng_device_id || bngDevices[0]?.id,
              })
            }
          />
          Cache PPPoE do BNG (última consulta SNMP)
        </label>
      </fieldset>
      {source === "bng_cache" && (
        <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>BNG</span>
          <select
            className="input"
            value={value.bng_device_id ?? ""}
            onChange={(e) => onChange({ ...value, bng_device_id: e.target.value })}
          >
            <option value="">Selecione o BNG…</option>
            {bngDevices.map((d) => (
              <option key={d.id} value={d.id}>
                {d.description}
              </option>
            ))}
          </select>
        </label>
      )}
    </div>
  );
}

function OltPeriodPanel({
  value,
  onChange,
}: {
  value: OltOverviewReportOptions;
  onChange: (next: OltOverviewReportOptions) => void;
}) {
  const period = value.period ?? "7d";
  const options: Array<{ id: OltOverviewReportOptions["period"]; label: string }> = [
    { id: "today", label: "Hoje (gráfico por hora)" },
    { id: "3d", label: "Últimos 3 dias" },
    { id: "7d", label: "Últimos 7 dias" },
    { id: "30d", label: "Últimos 30 dias" },
  ];
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8, fontSize: 13 }}>
      {options.map((opt) => (
        <label key={opt.id} style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
          <input
            type="radio"
            name="olt-period"
            checked={period === opt.id}
            onChange={() => onChange({ period: opt.id })}
          />
          {opt.label}
        </label>
      ))}
    </div>
  );
}

function EquipmentByPopOptionsPanel({
  value,
  onChange,
  compact,
}: {
  value: EquipmentByPopReportOptions;
  onChange: (next: EquipmentByPopReportOptions) => void;
  compact?: boolean;
}) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: compact ? "column" : "row",
        flexWrap: "wrap",
        gap: compact ? 10 : 16,
        fontSize: 12,
      }}
    >
      <label style={{ display: "flex", alignItems: "center", gap: 6, cursor: "pointer", margin: 0 }}>
        <input
          type="checkbox"
          checked={value.include_without_pop === true}
          onChange={(e) => onChange({ ...value, include_without_pop: e.target.checked })}
        />
        Incluir equipamentos sem POP
      </label>
      <label style={{ display: "flex", alignItems: "center", gap: 6, cursor: "pointer", margin: 0 }}>
        <input
          type="checkbox"
          checked={value.include_pop_coordinates === true}
          onChange={(e) => onChange({ ...value, include_pop_coordinates: e.target.checked })}
        />
        Incluir coordenadas do POP
      </label>
    </div>
  );
}

function bngChartLabel(iso: string) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function BngDeviceChart({
  device,
  points,
  title,
  variant = "screen",
}: {
  device?: string;
  points: NonNullable<SystemReportPayload["chart"]>["points"];
  title?: string;
  variant?: ReportVariant;
}) {
  const data = useMemo(
    () =>
      (points ?? []).map((p) => {
        const iso = String(p.collected_at ?? p.t ?? "");
        return {
          iso,
          label: bngChartLabel(iso),
          device: p.device,
          Total: p.total ?? null,
          PPPoE: p.pppoe ?? null,
          IPv4: p.ipv4 ?? null,
          IPv6: p.ipv6 ?? null,
          "Dual-stack": p.dual_stack ?? null,
        };
      }),
    [points],
  );

  const yDomain = useMemo(() => {
    const numeric: number[] = [];
    for (const row of data) {
      for (const v of [row.Total, row.PPPoE, row.IPv4, row.IPv6, row["Dual-stack"]]) {
        if (v != null && Number.isFinite(v)) numeric.push(v);
      }
    }
    if (numeric.length === 0) return [0, 1] as [number, number];
    const min = Math.min(...numeric);
    const max = Math.max(...numeric);
    const span = Math.max(1, max - min);
    const pad = Math.max(1, Math.round(span * 0.08));
    return [min - pad, max + pad] as [number, number];
  }, [data]);

  if (data.length === 0) return null;

  const chartTitle = `${title ?? "Gráfico BNG"}${device ? ` — ${device}` : ""}`;

  if (variant === "print") {
    return (
      <ChartDataTable
        title={chartTitle}
        columns={["Data", "Total", "PPPoE", "IPv4", "IPv6", "Dual-stack"]}
        rows={data.map((row) => [
          row.iso ? new Date(String(row.iso)).toLocaleString("pt-BR") : row.label,
          row.Total,
          row.PPPoE,
          row.IPv4,
          row.IPv6,
          row["Dual-stack"],
        ])}
      />
    );
  }

  return (
    <div className="card" style={{ padding: 12, marginTop: 12 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>
        {title ?? "Gráfico BNG"}
        {device ? <span style={{ fontWeight: 400, color: "var(--muted)" }}> — {device}</span> : null}
      </h3>
      <ResponsiveContainer width="100%" height={260}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 9 }} interval="preserveStartEnd" minTickGap={28} />
          <YAxis tick={{ fontSize: 10 }} width={48} allowDecimals={false} domain={yDomain} />
          <Tooltip
            labelFormatter={(_, items) => {
              const iso = items?.[0]?.payload?.iso;
              return iso ? new Date(String(iso)).toLocaleString("pt-BR") : "";
            }}
          />
          <Legend />
          <Line type="monotone" dataKey="Total" stroke="#64748b" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="PPPoE" stroke="#3b82f6" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="IPv4" stroke="#22c55e" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="IPv6" stroke="#a855f7" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="Dual-stack" stroke="#f59e0b" strokeWidth={2} dot={false} connectNulls={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function BngReportAverages({ payload }: { payload: SystemReportPayload }) {
  const windows = payload.averages?.windows ?? [];
  if (windows.length === 0) return null;
  return (
    <section style={{ marginBottom: 16 }}>
      <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>Médias de logins (BNG)</h3>
      <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>
        Média aritmética das coletas SNMP por janela — incluída no envio Telegram quando há dados suficientes.
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Período</th>
              <th>Amostras</th>
              <th>Total</th>
              <th>PPPoE</th>
              <th>IPv4</th>
              <th>IPv6</th>
              <th>Dual-stack</th>
            </tr>
          </thead>
          <tbody>
            {windows.map((w) => (
              <tr key={w.days}>
                <td>{w.label}</td>
                <td>{w.samples.toLocaleString("pt-PT")}</td>
                <td>{w.total != null ? w.total.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.pppoe != null ? w.pppoe.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.ipv4 != null ? w.ipv4.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.ipv6 != null ? w.ipv6.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.dual_stack != null ? w.dual_stack.toLocaleString("pt-PT") : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function BngReportChart({ payload, variant = "screen" }: { payload: SystemReportPayload; variant?: ReportVariant }) {
  const pts = payload.chart?.points ?? [];
  const byDevice = useMemo(() => {
    const map = new Map<string, typeof pts>();
    for (const p of pts) {
      const key = p.device?.trim() || "BNG";
      const list = map.get(key) ?? [];
      list.push(p);
      map.set(key, list);
    }
    return map;
  }, [pts]);

  if (pts.length === 0) return null;

  if (byDevice.size <= 1) {
    const only = [...byDevice.values()][0] ?? pts;
    return <BngDeviceChart points={only} title={payload.chart?.label} variant={variant} />;
  }

  return (
    <>
      {[...byDevice.entries()].map(([device, devicePts]) => (
        <BngDeviceChart key={device} device={device} points={devicePts} title={payload.chart?.label} variant={variant} />
      ))}
    </>
  );
}

function ReportChart({ payload, variant = "screen" }: { payload: SystemReportPayload; variant?: ReportVariant }) {
  const pts = payload.chart?.points ?? [];
  if (pts.length === 0) return null;
  const data = pts.map((p) => ({
    label: String(p.t ?? ""),
    Total: Number(p.total ?? 0),
    Online: Number(p.online ?? 0),
    Offline: Number(p.offline ?? 0),
  }));

  if (variant === "print") {
    return (
      <ChartDataTable
        title={payload.chart?.label ?? "Gráfico"}
        columns={["Período", "Total", "Online", "Offline"]}
        rows={data.map((row) => [row.label, row.Total, row.Online, row.Offline])}
      />
    );
  }

  return (
    <div className="card" style={{ padding: 12, marginTop: 12 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>{payload.chart?.label ?? "Gráfico"}</h3>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 10 }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 10 }} width={44} allowDecimals={false} />
          <Tooltip />
          <Legend />
          <Line type="monotone" dataKey="Total" stroke="#58a6ff" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="Online" stroke="#3fb950" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="Offline" stroke="#f85149" strokeWidth={2} dot={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function EquipmentByPopReport({ payload }: { payload: SystemReportPayload }) {
  const groups = payload.groups ?? [];
  if (groups.length === 0) return null;
  return (
    <section>
      <h3 style={{ fontSize: 14, margin: "0 0 10px" }}>Equipamentos por POP</h3>
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        {groups.map((g) => (
          <div key={g.pop ?? "sem-pop"} className="card" style={{ padding: "12px 14px", margin: 0 }}>
            <h4 style={{ margin: "0 0 8px", fontSize: 14, fontWeight: 600 }}>{g.pop || "(sem POP)"}</h4>
            {g.coordinates ? (
              <p style={{ margin: "0 0 8px", fontSize: 11, color: "var(--muted)" }} className="mono">
                {g.coordinates}
              </p>
            ) : null}
            {(g.devices ?? []).length === 0 ? (
              <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>Sem equipamentos.</p>
            ) : (
              <ul style={{ margin: 0, paddingLeft: 18, fontSize: 13, lineHeight: 1.55 }}>
                {(g.devices ?? []).map((d) => (
                  <li key={`${g.pop}-${d.label ?? d.name}`}>
                    {d.label || (d.name && d.category ? `${d.name} [${d.category}]` : d.name || "—")}
                  </li>
                ))}
              </ul>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}

function ReportPreviewBody({ payload, variant = "screen" }: { payload: SystemReportPayload; variant?: ReportVariant }) {
  const summary = summaryEntries(payload.summary);
  const cols = payload.columns ?? [];
  const rows = payload.rows ?? [];
  const isPrint = variant === "print";
  const visibleRows = isPrint ? rows : rows.slice(0, 500);
  return (
    <div className="system-report-print">
      <header style={{ marginBottom: 16 }}>
        <h2 style={{ margin: "0 0 4px" }}>{payload.title}</h2>
        {payload.description && (
          <p style={{ margin: "0 0 8px", color: "var(--muted)", fontSize: 13 }}>{payload.description}</p>
        )}
        <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>
          Gerado: {new Date(payload.generated_at).toLocaleString("pt-PT")}
        </p>
      </header>

      {summary.length > 0 && (
        <section style={{ marginBottom: 16 }}>
          <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>Resumo</h3>
          <div className="table-wrap">
            <table>
              <tbody>
                {summary.map(([k, v]) => (
                  <tr key={k}>
                    <th style={{ textAlign: "left", width: "40%" }}>{k}</th>
                    <td>{v}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {payload.report_id === "olt-overview" && <ReportChart payload={payload} variant={variant} />}
      {payload.report_id === "bng-subscribers" && <BngReportAverages payload={payload} />}
      {payload.report_id === "bng-subscribers" && <BngReportChart payload={payload} variant={variant} />}

      {payload.report_id === "equipment-by-pop" && (payload.groups?.length ?? 0) > 0 ? (
        <EquipmentByPopReport payload={payload} />
      ) : null}

      {payload.report_id !== "equipment-by-pop" && rows.length > 0 && (
        <section>
          <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>
            Detalhes <span style={{ color: "var(--muted)", fontWeight: 400 }}>({rows.length} linhas)</span>
          </h3>
          <div className={`table-wrap${isPrint ? " system-report-print-table" : ""}`} style={isPrint ? undefined : { maxHeight: 420, overflow: "auto" }}>
            <table>
              <thead>
                <tr>
                  {cols.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {visibleRows.map((row, i) => (
                  <tr key={i}>
                    {row.map((cell, j) => {
                      const col = cols[j] ?? "";
                      const display =
                        col === "Última coleta" && cell ? formatBngDateTime(cell) : cell || "—";
                      return (
                        <td
                          key={j}
                          style={isPrint ? undefined : { maxWidth: 280, overflow: "hidden", textOverflow: "ellipsis" }}
                          title={isPrint ? undefined : display}
                        >
                          {display}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!isPrint && rows.length > 500 && (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>Pré-visualização limitada a 500 linhas. Exporte CSV para o conjunto completo.</p>
          )}
        </section>
      )}
    </div>
  );
}

type EquipPopAction = "preview" | "csv" | "pdf" | "telegram";

function ReportIconButton({
  title,
  onClick,
  disabled,
  children,
}: {
  title: string;
  onClick: () => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      className="btn btn--icon-menu"
      title={title}
      aria-label={title}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function ReportCard({
  id,
  title,
  description,
  onOpen,
  onPrint,
  onCsv,
  onTelegram,
  printPending,
  csvPending,
  tgPending,
  onEquipPopRequest,
  onConnectionsRequest,
  onOltRequest,
}: {
  id: SystemReportId;
  title: string;
  description: string;
  onOpen: (id: SystemReportId) => void;
  onPrint: (id: SystemReportId) => void;
  onCsv: (id: SystemReportId) => void;
  onTelegram: (id: SystemReportId) => void;
  printPending: boolean;
  csvPending: boolean;
  tgPending: boolean;
  onEquipPopRequest: (action: EquipPopAction) => void;
  onConnectionsRequest: (action: EquipPopAction) => void;
  onOltRequest: (action: EquipPopAction) => void;
}) {
  const admin = isAdminUser();

  const runOrModal = (action: EquipPopAction, fn: () => void) => {
    if (id === "equipment-by-pop") {
      onEquipPopRequest(action);
      return;
    }
    if (id === "connections") {
      onConnectionsRequest(action);
      return;
    }
    if (id === "olt-overview") {
      onOltRequest(action);
      return;
    }
    fn();
  };

  return (
    <div
      className="card report-list-card"
      style={{
        padding: "10px 12px",
        margin: 0,
        display: "flex",
        alignItems: "center",
        gap: 10,
        minHeight: 0,
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <h3
          style={{
            margin: 0,
            fontSize: 14,
            fontWeight: 600,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {title}
        </h3>
        <p
          style={{
            margin: "2px 0 0",
            fontSize: 12,
            color: "var(--muted)",
            lineHeight: 1.35,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {description}
        </p>
      </div>

      <div className="row" style={{ gap: 4, flexShrink: 0 }}>
        <ReportIconButton title="Ver relatório" onClick={() => runOrModal("preview", () => onOpen(id))}>
          <Eye size={16} />
        </ReportIconButton>
        <ReportIconButton title="Exportar CSV" disabled={csvPending} onClick={() => runOrModal("csv", () => onCsv(id))}>
          <FileDown size={16} />
        </ReportIconButton>
        <ReportIconButton title="PDF / Imprimir" disabled={printPending} onClick={() => runOrModal("pdf", () => onPrint(id))}>
          <FileText size={16} />
        </ReportIconButton>
        {admin ? (
          <ReportIconButton title="Enviar Telegram" disabled={tgPending} onClick={() => runOrModal("telegram", () => onTelegram(id))}>
            <Send size={16} />
          </ReportIconButton>
        ) : null}
      </div>

      <InfoHint label={`Informação: ${title}`} className="info-hint--align-end">
        <SystemReportInfoTooltip id={id} />
      </InfoHint>
    </div>
  );
}

export function ReportsPage() {
  const [previewId, setPreviewId] = useState<SystemReportId | null>(null);
  const [printPayload, setPrintPayload] = useState<SystemReportPayload | null>(null);
  const [search, setSearch] = useState("");
  const [equipPopOpts, setEquipPopOpts] = useState<EquipmentByPopReportOptions>(DEFAULT_EQUIP_POP_OPTS);
  const [connectionsOpts, setConnectionsOpts] = useState<ConnectionsReportOptions>(DEFAULT_CONNECTIONS_OPTS);
  const [oltOpts, setOltOpts] = useState<OltOverviewReportOptions>(DEFAULT_OLT_OPTS);
  const [previewEquipPopOpts, setPreviewEquipPopOpts] = useState<EquipmentByPopReportOptions | null>(null);
  const [previewConnectionsOpts, setPreviewConnectionsOpts] = useState<ConnectionsReportOptions | null>(null);
  const [previewOltOpts, setPreviewOltOpts] = useState<OltOverviewReportOptions | null>(null);
  const [equipPopModal, setEquipPopModal] = useState<{
    draft: EquipmentByPopReportOptions;
    action: EquipPopAction;
  } | null>(null);
  const [connectionsModal, setConnectionsModal] = useState<{
    draft: ConnectionsReportOptions;
    action: EquipPopAction;
  } | null>(null);
  const [oltModal, setOltModal] = useState<{
    draft: OltOverviewReportOptions;
    action: EquipPopAction;
  } | null>(null);
  const admin = isAdminUser();
  const { push: pushToast } = useAppToast();

  const triggerPrint = useCallback((payload: SystemReportPayload) => {
    setPrintPayload(payload);
  }, []);

  useEffect(() => {
    if (!printPayload) return;

    const cleanup = () => {
      document.body.classList.remove("print-system-report");
      setPrintPayload(null);
    };

    const onAfterPrint = () => cleanup();
    window.addEventListener("afterprint", onAfterPrint);

    const frame = requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        document.body.classList.add("print-system-report");
        window.print();
      });
    });

    const fallback = window.setTimeout(cleanup, 3000);

    return () => {
      cancelAnimationFrame(frame);
      window.clearTimeout(fallback);
      window.removeEventListener("afterprint", onAfterPrint);
    };
  }, [printPayload]);

  const catalog = useQuery({
    queryKey: ["system-reports-catalog"],
    queryFn: fetchSystemReportCatalog,
  });

  const bngDevices = useQuery({
    queryKey: ["bng-devices-report-modal"],
    queryFn: () => apiFetch<{ devices: Array<{ id: string; description: string }> }>("/api/v1/bng/devices"),
    enabled: connectionsModal != null,
  });

  const previewReportOpts = useMemo(() => {
    if (previewId === "equipment-by-pop") return previewEquipPopOpts ?? equipPopOpts;
    if (previewId === "connections") return previewConnectionsOpts ?? connectionsOpts;
    if (previewId === "olt-overview") return previewOltOpts ?? oltOpts;
    return undefined;
  }, [previewId, previewEquipPopOpts, equipPopOpts, previewConnectionsOpts, connectionsOpts, previewOltOpts, oltOpts]);

  const preview = useQuery({
    queryKey: ["system-report", previewId, previewReportOpts],
    queryFn: () => fetchSystemReport(previewId!, previewReportOpts as never),
    enabled: previewId != null,
  });

  const openReport = useCallback((id: SystemReportId) => {
    setPreviewEquipPopOpts(null);
    setPreviewConnectionsOpts(null);
    setPreviewOltOpts(null);
    setPreviewId(id);
  }, []);

  const runEquipPopAction = useCallback(
    (opts: EquipmentByPopReportOptions, action: EquipPopAction) => {
      setEquipPopOpts(opts);
      setPreviewEquipPopOpts(opts);
      if (action === "preview") {
        setPreviewId("equipment-by-pop");
        return;
      }
      if (action === "csv") {
        void downloadSystemReportCsv("equipment-by-pop", opts)
          .then(() => pushToast({ tone: "ok", text: "CSV descarregado." }))
          .catch((e) => toastErr(pushToast, e, "Falha ao exportar CSV"));
        return;
      }
      if (action === "pdf") {
        void fetchSystemReport("equipment-by-pop", opts)
          .then((data) => triggerPrint(data))
          .catch((e) => toastErr(pushToast, e, "Falha ao preparar PDF"));
        return;
      }
      if (action === "telegram") {
        void sendSystemReportTelegram("equipment-by-pop", opts)
          .then(() => toastOk(pushToast, "Relatório enviado ao Telegram."))
          .catch((e) => toastErr(pushToast, e, "Falha ao enviar Telegram"));
      }
    },
    [pushToast, triggerPrint],
  );

  const runConnectionsAction = useCallback(
    (opts: ConnectionsReportOptions, action: EquipPopAction) => {
      if (opts.source === "bng_cache" && !opts.bng_device_id) {
        pushToast({ tone: "err", text: "Selecione o BNG para o cache PPPoE." });
        return;
      }
      setConnectionsOpts(opts);
      setPreviewConnectionsOpts(opts);
      if (action === "preview") {
        setPreviewId("connections");
        return;
      }
      if (action === "csv") {
        void downloadSystemReportCsv("connections", opts)
          .then(() => pushToast({ tone: "ok", text: "CSV descarregado." }))
          .catch((e) => toastErr(pushToast, e, "Falha ao exportar CSV"));
        return;
      }
      if (action === "pdf") {
        void fetchSystemReport("connections", opts)
          .then((data) => triggerPrint(data))
          .catch((e) => toastErr(pushToast, e, "Falha ao preparar PDF"));
        return;
      }
      if (action === "telegram") {
        void sendSystemReportTelegram("connections", opts)
          .then(() => toastOk(pushToast, "Relatório enviado ao Telegram."))
          .catch((e) => toastErr(pushToast, e, "Falha ao enviar Telegram"));
      }
    },
    [pushToast, triggerPrint],
  );

  const runOltAction = useCallback(
    (opts: OltOverviewReportOptions, action: EquipPopAction) => {
      setOltOpts(opts);
      setPreviewOltOpts(opts);
      if (action === "preview") {
        setPreviewId("olt-overview");
        return;
      }
      if (action === "csv") {
        void downloadSystemReportCsv("olt-overview", opts)
          .then(() => pushToast({ tone: "ok", text: "CSV descarregado." }))
          .catch((e) => toastErr(pushToast, e, "Falha ao exportar CSV"));
        return;
      }
      if (action === "pdf") {
        void fetchSystemReport("olt-overview", opts)
          .then((data) => triggerPrint(data))
          .catch((e) => toastErr(pushToast, e, "Falha ao preparar PDF"));
        return;
      }
      if (action === "telegram") {
        void sendSystemReportTelegram("olt-overview", opts)
          .then(() => toastOk(pushToast, "Relatório enviado ao Telegram."))
          .catch((e) => toastErr(pushToast, e, "Falha ao enviar Telegram"));
      }
    },
    [pushToast, triggerPrint],
  );

  const printMut = useMutation({
    mutationFn: (id: SystemReportId) => fetchSystemReport(id),
    onSuccess: (data) => triggerPrint(data),
    onError: (e) => toastErr(pushToast, e, "Falha ao preparar PDF"),
  });

  const csvMut = useMutation({
    mutationFn: (id: SystemReportId) => downloadSystemReportCsv(id),
    onSuccess: () => pushToast({ tone: "ok", text: "CSV descarregado." }),
    onError: (e) => toastErr(pushToast, e, "Falha ao exportar CSV"),
  });

  const tgMut = useMutation({
    mutationFn: (id: SystemReportId) => sendSystemReportTelegram(id),
    onSuccess: () => toastOk(pushToast, "Relatório enviado ao Telegram."),
    onError: (e) => toastErr(pushToast, e, "Falha ao enviar Telegram"),
  });

  const items = useMemo(() => {
    const all = catalog.data?.reports ?? [];
    const q = search.trim().toLowerCase();
    if (!q) return all;
    return all.filter(
      (r) => r.title.toLowerCase().includes(q) || r.description.toLowerCase().includes(q) || r.id.toLowerCase().includes(q),
    );
  }, [catalog.data?.reports, search]);

  if (catalog.isLoading) return <p>Carregando relatórios…</p>;
  if (catalog.isError) return <div className="msg msg--err">{(catalog.error as Error).message}</div>;

  return (
    <>
      <div className="page-heading">
        <h1>Relatórios</h1>
        <PageCountPill label="Relatórios" count={items.length} />
      </div>

      <div style={{ marginBottom: 12, maxWidth: 360 }}>
        <input
          className="input"
          type="search"
          placeholder="Pesquisar relatório…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: "100%" }}
        />
      </div>

      {items.length === 0 ? (
        <p style={{ color: "var(--muted)" }}>Nenhum relatório corresponde à pesquisa.</p>
      ) : (
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {items.map((r) => (
          <ReportCard
            key={r.id}
            id={r.id as SystemReportId}
            title={r.title}
            description={r.description}
            onOpen={openReport}
            onPrint={(id) => printMut.mutate(id)}
            onCsv={(id) => csvMut.mutate(id)}
            onTelegram={(id) => tgMut.mutate(id)}
            printPending={printMut.isPending}
            csvPending={csvMut.isPending}
            tgPending={tgMut.isPending}
            onEquipPopRequest={(action) =>
              setEquipPopModal({ draft: { ...equipPopOpts }, action })
            }
            onConnectionsRequest={(action) =>
              setConnectionsModal({ draft: { ...connectionsOpts }, action })
            }
            onOltRequest={(action) => setOltModal({ draft: { ...oltOpts }, action })}
          />
        ))}
      </div>
      )}

      {equipPopModal &&
        createPortal(
          <div className="modal-backdrop" role="presentation" onClick={() => setEquipPopModal(null)}>
            <div
              className="modal card"
              role="dialog"
              aria-modal="true"
              aria-labelledby="equip-pop-modal-title"
              style={{ width: "min(420px, 94vw)", padding: 20 }}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 14 }}>
                <h2 id="equip-pop-modal-title" style={{ margin: 0, fontSize: 17 }}>
                  Equipamentos por POP
                </h2>
                <button type="button" className="btn btn--icon-menu" aria-label="Fechar" onClick={() => setEquipPopModal(null)}>
                  <X size={16} />
                </button>
              </div>
              <EquipmentByPopOptionsPanel
                compact
                value={equipPopModal.draft}
                onChange={(next) => setEquipPopModal((m) => (m ? { ...m, draft: next } : m))}
              />
              <div style={{ marginTop: 16, display: "flex", justifyContent: "flex-end" }}>
                <button
                  type="button"
                  className="btn btn--primary"
                  onClick={() => {
                    const { draft, action } = equipPopModal;
                    setEquipPopModal(null);
                    runEquipPopAction(draft, action);
                  }}
                >
                  Gerar relatório
                </button>
              </div>
            </div>
          </div>,
          document.body,
        )}

      {connectionsModal &&
        createPortal(
          <div className="modal-backdrop" role="presentation" onClick={() => setConnectionsModal(null)}>
            <div
              className="modal card"
              role="dialog"
              aria-modal="true"
              aria-labelledby="connections-modal-title"
              style={{ width: "min(460px, 94vw)", padding: 20 }}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 14 }}>
                <h2 id="connections-modal-title" style={{ margin: 0, fontSize: 17 }}>
                  Conexões de clientes
                </h2>
                <button type="button" className="btn btn--icon-menu" aria-label="Fechar" onClick={() => setConnectionsModal(null)}>
                  <X size={16} />
                </button>
              </div>
              <ConnectionsOptionsPanel
                value={connectionsModal.draft}
                onChange={(next) => setConnectionsModal((m) => (m ? { ...m, draft: next } : m))}
                bngDevices={bngDevices.data?.devices ?? []}
              />
              <div style={{ marginTop: 16, display: "flex", justifyContent: "flex-end" }}>
                <button
                  type="button"
                  className="btn btn--primary"
                  onClick={() => {
                    const { draft, action } = connectionsModal;
                    setConnectionsModal(null);
                    runConnectionsAction(draft, action);
                  }}
                >
                  Gerar relatório
                </button>
              </div>
            </div>
          </div>,
          document.body,
        )}

      {oltModal &&
        createPortal(
          <div className="modal-backdrop" role="presentation" onClick={() => setOltModal(null)}>
            <div
              className="modal card"
              role="dialog"
              aria-modal="true"
              aria-labelledby="olt-modal-title"
              style={{ width: "min(420px, 94vw)", padding: 20 }}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 14 }}>
                <h2 id="olt-modal-title" style={{ margin: 0, fontSize: 17 }}>
                  OLTs — período
                </h2>
                <button type="button" className="btn btn--icon-menu" aria-label="Fechar" onClick={() => setOltModal(null)}>
                  <X size={16} />
                </button>
              </div>
              <OltPeriodPanel
                value={oltModal.draft}
                onChange={(next) => setOltModal((m) => (m ? { ...m, draft: next } : m))}
              />
              <div style={{ marginTop: 16, display: "flex", justifyContent: "flex-end" }}>
                <button
                  type="button"
                  className="btn btn--primary"
                  onClick={() => {
                    const { draft, action } = oltModal;
                    setOltModal(null);
                    runOltAction(draft, action);
                  }}
                >
                  Gerar relatório
                </button>
              </div>
            </div>
          </div>,
          document.body,
        )}

      {previewId != null &&
        createPortal(
          <div className="modal-backdrop" role="presentation" onClick={() => setPreviewId(null)}>
            <div
              className="modal card system-report-modal"
              role="dialog"
              aria-modal="true"
              style={{ width: "min(960px, 96vw)", maxHeight: "92vh", overflow: "auto", padding: 20 }}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 12 }}>
                <h2 style={{ margin: 0, fontSize: 18 }}>Pré-visualização</h2>
                <button type="button" className="btn no-print" aria-label="Fechar" onClick={() => setPreviewId(null)}>
                  <X size={16} />
                </button>
              </div>

              {preview.isLoading && <p>A carregar dados…</p>}
              {preview.isError && <div className="msg msg--err">{(preview.error as Error).message}</div>}
              {preview.data && (
                <>
                  <ReportPreviewBody payload={preview.data} />
                  <div className="row no-print" style={{ gap: 8, marginTop: 16, flexWrap: "wrap" }}>
                    <button
                      type="button"
                      className="btn btn--primary"
                      onClick={() => triggerPrint(preview.data!)}
                    >
                      <FileText size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                      PDF / Imprimir
                    </button>
                    <button
                      type="button"
                      className="btn"
                      disabled={csvMut.isPending}
                      onClick={() => {
                        if (previewId === "equipment-by-pop") {
                          runEquipPopAction(previewEquipPopOpts ?? equipPopOpts, "csv");
                        } else if (previewId === "connections") {
                          runConnectionsAction(previewConnectionsOpts ?? connectionsOpts, "csv");
                        } else if (previewId === "olt-overview") {
                          runOltAction(previewOltOpts ?? oltOpts, "csv");
                        } else {
                          csvMut.mutate(previewId);
                        }
                      }}
                    >
                      <FileDown size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                      CSV
                    </button>
                    {admin && (
                      <button
                        type="button"
                        className="btn"
                        disabled={tgMut.isPending}
                        onClick={() => {
                          if (previewId === "equipment-by-pop") {
                            runEquipPopAction(previewEquipPopOpts ?? equipPopOpts, "telegram");
                          } else if (previewId === "connections") {
                            runConnectionsAction(previewConnectionsOpts ?? connectionsOpts, "telegram");
                          } else if (previewId === "olt-overview") {
                            runOltAction(previewOltOpts ?? oltOpts, "telegram");
                          } else {
                            tgMut.mutate(previewId);
                          }
                        }}
                      >
                        <Send size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                        Telegram
                      </button>
                    )}
                  </div>
                </>
              )}
            </div>
          </div>,
          document.body,
        )}

      {printPayload &&
        createPortal(
          <div id="system-report-print-root" className="system-report-print-document" aria-hidden="true">
            <ReportPreviewBody payload={printPayload} variant="print" />
          </div>,
          document.body,
        )}
    </>
  );
}
