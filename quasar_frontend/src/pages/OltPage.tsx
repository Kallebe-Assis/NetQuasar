import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { EM_DASH, format1f, formatNullable, formatNum } from "../lib/formatDisplay";
import { formatBitrate } from "../lib/formatBitrate";
import { invalidateAlertListQueries } from "../lib/queryKeys";
import { formatCollectedPt, groupOltInterfaceRows, type InterfaceMonitorTableRow } from "../lib/deviceReportHelpers";
import { formatYearMonthPt, monthSelectChoicesWithFallback, recentYearMonthChoices } from "../lib/yearMonthPt";
import { DropdownMenu } from "../components/DropdownMenu";
import { OltReportsTab } from "./olt/OltReportsTab";
import { OltMetricsCollectLogModal, type MetricsWalkRow } from "../components/OltMetricsCollectLogModal";
import { OltSnmpDebugPanel } from "../components/OltSnmpDebugPanel";
import { OltVsolOnuTable, type VsOnuRow } from "../components/OltVsolOnuTable";

type OltRow = {
  id: string;
  description?: string | null;
  ip?: string | null;
  brand?: string | null;
  model?: string | null;
  locality_id?: string | null;
  locality_name?: string | null;
  /** ISO — última gravação do snapshot OLT (SNMP / PONs). */
  olt_snapshot_at?: string | null;
  summary: unknown;
  pons: unknown;
  computed?: {
    pon_count?: number;
    onu_total_sum?: number;
    onu_online_sum?: number;
    onu_offline_sum?: number;
  };
};

type PonTableRow = {
  id?: string;
  name?: string;
  rx_dbm?: number | null;
  tx_dbm?: number | null;
  onu_total?: number;
  onu_online?: number;
  onu_offline?: number;
  status?: string;
};

type IfRow = Omit<InterfaceMonitorTableRow, "if_index"> & { if_index: number };

type ZteMibRow = {
  oid?: string;
  suffix?: string;
  type?: string;
  value?: string;
  value_int?: number;
  value_label?: string;
  if_index?: number;
  if_name?: string;
};

type OltSortKey = "description" | "ip" | "pons" | "onus" | "online" | "offline" | "updated";

function IconExternalLinkSubtle({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      style={{ opacity: 0.55 }}
    >
      <path d="M15 3h6v6" />
      <path d="M10 14 21 3" />
      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
    </svg>
  );
}

function ifDisplayLabel(r: IfRow): string {
  const s = String(r.display_name ?? r.if_name ?? r.descr ?? "").trim();
  return s || EM_DASH;
}

function badgeOper(s: string | undefined): string {
  const x = String(s ?? "").toLowerCase();
  if (x === "up" || x === "online" || x === "ok") return "badge badge--ok";
  if (x === "down" || x === "offline") return "badge badge--err";
  return "badge badge--off";
}

function fmtOctCell(v: number | null | undefined, saturated?: boolean): string {
  if (saturated && v == null) return `${EM_DASH} (32b)`;
  if (v == null || !Number.isFinite(Number(v))) return EM_DASH;
  return Number(v).toLocaleString("pt-PT");
}

const fmtBps = (v: number | null | undefined) => formatBitrate(v, "perSecond");

function isPonRow(r: IfRow): boolean {
  const kind = String(r.olt_iface_kind ?? "").toLowerCase();
  if (kind === "pon") return true;
  const name = String(r.display_name ?? r.if_name ?? r.descr ?? "").toLowerCase();
  return name.includes("gpon_olt-") || name.startsWith("pon-");
}

function asIfRows(list: InterfaceMonitorTableRow[]): IfRow[] {
  return list.filter((r): r is IfRow => typeof r.if_index === "number" && Number.isFinite(r.if_index));
}

function OltIfaceSection({ rows }: { rows: IfRow[] }) {
  const g = groupOltInterfaceRows(rows);
  const blocks: [string, IfRow[]][] = [
    ["Gestão e VLANs (GE, VLAN)", asIfRows(g.geVlan)],
    ["Portas PON (GPON slot/porta)", asIfRows(g.pon)],
    ["ONUs (interface por assinante)", asIfRows(g.onu)],
    ["Outras", asIfRows(g.other)],
  ];
  if (!rows.some((r) => String(r.olt_iface_kind ?? "").length > 0)) {
    return (
      <div className="table-wrap" style={{ maxHeight: 280, overflow: "auto", maxWidth: "100%" }}>
        <table style={{ fontSize: 10, width: "100%", minWidth: 1100, tableLayout: "fixed" }}>
          <thead>
            <tr>
              <th>Idx</th>
              <th style={{ width: "23%" }}>Nome / descrição</th>
              <th>Admin</th>
              <th>Oper</th>
              <th className="mono">Entrada (octets)</th>
              <th className="mono">Saída (octets)</th>
              <th className="mono">RX tráfego</th>
              <th className="mono">TX tráfego</th>
              <th className="mono">RX dBm</th>
              <th className="mono">TX dBm</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.if_index}>
                <td className="mono">{r.if_index}</td>
                <td style={{ wordBreak: "break-word", overflowWrap: "anywhere" }}>
                  {ifDisplayLabel(r)}
                  {r.descr && String(r.descr).trim() !== ifDisplayLabel(r) && (
                    <div className="mono" style={{ fontSize: 8, color: "var(--muted)", wordBreak: "break-word", overflowWrap: "anywhere" }} title="ifDescr">
                      {r.descr}
                    </div>
                  )}
                </td>
                <td>
                  <span className={badgeOper(r.admin_status)}>{r.admin_status ?? "—"}</span>
                </td>
                <td>
                  <span className={badgeOper(r.oper_status)}>{r.oper_status ?? "—"}</span>
                </td>
                <td className="mono">{isPonRow(r) ? "—" : fmtOctCell(r.in_octets, r.octets_saturated_32bit)}</td>
                <td className="mono">{isPonRow(r) ? "—" : fmtOctCell(r.out_octets, r.octets_saturated_32bit)}</td>
                <td className="mono">{isPonRow(r) ? "—" : fmtBps(r.in_bps)}</td>
                <td className="mono">{isPonRow(r) ? "—" : fmtBps(r.out_bps)}</td>
                <td className="mono">{r.rx_dbm != null && Number.isFinite(Number(r.rx_dbm)) ? Number(r.rx_dbm).toFixed(1) : "—"}</td>
                <td className="mono">{r.tx_dbm != null && Number.isFinite(Number(r.tx_dbm)) ? Number(r.tx_dbm).toFixed(1) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }
  const ifaceTableBlock = (title: string, br: IfRow[], withHeading: boolean) => (
    <>
      {withHeading ? (
        <h3 style={{ marginTop: 16, marginBottom: 6, fontSize: 14 }}>{title}</h3>
      ) : null}
      <div className="table-wrap" style={{ maxHeight: 260, overflow: "auto", maxWidth: "100%" }}>
        <table style={{ fontSize: 10, width: "100%", minWidth: 1100, tableLayout: "fixed" }}>
          <thead>
            <tr>
              <th>Idx</th>
              <th style={{ width: "23%" }}>Nome / descrição</th>
              <th>Admin</th>
              <th>Oper</th>
              <th className="mono">Entrada (octets)</th>
              <th className="mono">Saída (octets)</th>
              <th className="mono">RX tráfego</th>
              <th className="mono">TX tráfego</th>
              <th className="mono">RX dBm</th>
              <th className="mono">TX dBm</th>
            </tr>
          </thead>
          <tbody>
            {br.map((r) => (
              <tr key={r.if_index}>
                <td className="mono">{r.if_index}</td>
                <td style={{ wordBreak: "break-word", overflowWrap: "anywhere" }}>
                  {ifDisplayLabel(r)}
                  {r.descr && String(r.descr).trim() !== ifDisplayLabel(r) && (
                    <div className="mono" style={{ fontSize: 8, color: "var(--muted)", wordBreak: "break-word", overflowWrap: "anywhere" }} title="ifDescr">
                      {r.descr}
                    </div>
                  )}
                </td>
                <td>
                  <span className={badgeOper(r.admin_status)}>{r.admin_status ?? "—"}</span>
                </td>
                <td>
                  <span className={badgeOper(r.oper_status)}>{r.oper_status ?? "—"}</span>
                </td>
                <td className="mono">{isPonRow(r) ? "—" : fmtOctCell(r.in_octets, r.octets_saturated_32bit)}</td>
                <td className="mono">{isPonRow(r) ? "—" : fmtOctCell(r.out_octets, r.octets_saturated_32bit)}</td>
                <td className="mono">{isPonRow(r) ? "—" : fmtBps(r.in_bps)}</td>
                <td className="mono">{isPonRow(r) ? "—" : fmtBps(r.out_bps)}</td>
                <td className="mono">{r.rx_dbm != null && Number.isFinite(Number(r.rx_dbm)) ? Number(r.rx_dbm).toFixed(1) : "—"}</td>
                <td className="mono">{r.tx_dbm != null && Number.isFinite(Number(r.tx_dbm)) ? Number(r.tx_dbm).toFixed(1) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );

  return (
    <>
      {blocks.map(([title, br]) => {
        if (br.length === 0) return null;
        const collapsible = title.startsWith("ONUs (interface por assinante)");
        if (!collapsible) return <div key={title}>{ifaceTableBlock(title, br, true)}</div>;
        return (
          <details key={title} className="collapsible-section">
            <summary>
              {title} <span style={{ fontWeight: 400, color: "var(--muted)" }}>({br.length})</span>
            </summary>
            <div className="collapsible-section__body">{ifaceTableBlock(title, br, false)}</div>
          </details>
        );
      })}
    </>
  );
}

type OltPageTab = "equipamentos" | "relatorios";

export function OltPage() {
  const [pageTab, setPageTab] = useState<OltPageTab>("equipamentos");
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const bulkMonthChoices = useMemo(() => recentYearMonthChoices(72), []);
  const [sel, setSel] = useState<string | null>(null);
  const [snmpDebugOpen, setSnmpDebugOpen] = useState(true);
  const [refreshScope, setRefreshScope] = useState<"onu" | "full" | "telemetry">("onu");
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkPhase, setBulkPhase] = useState<"select" | "running" | "results">("select");
  const [bulkRunning, setBulkRunning] = useState(false);
  const [bulkLog, setBulkLog] = useState<string[]>([]);
  const [bulkProgress, setBulkProgress] = useState({ done: 0, total: 0 });
  const [bulkCollectedRows, setBulkCollectedRows] = useState<
    Array<{ olt_id: string; olt_description: string; locality_id: string; locality_name: string; online: number; offline: number; total: number }>
  >([]);
  const [saveToast, setSaveToast] = useState<{ ok: boolean; text: string } | null>(null);
  const [bulkMonth, setBulkMonth] = useState(() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
  });
  const [bulkSelectedIds, setBulkSelectedIds] = useState<string[]>([]);
  const [bulkOltFilter, setBulkOltFilter] = useState("");
  const [sortKey, setSortKey] = useState<OltSortKey>("updated");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [collectLogOpen, setCollectLogOpen] = useState(false);
  const [metricsWalkRows, setMetricsWalkRows] = useState<MetricsWalkRow[] | null>(null);

  useEffect(() => {
    setMetricsWalkRows(null);
    setCollectLogOpen(false);
  }, [sel]);

  const list = useQuery({
    queryKey: ["olt-devices"],
    queryFn: () => apiFetch<{ olts: OltRow[] }>("/api/v1/olt/devices"),
  });

  const detail = useQuery({
    queryKey: ["olt-device", sel],
    enabled: !!sel,
    queryFn: () =>
      apiFetch<{
        id: string;
        description?: string | null;
        ip?: string | null;
        olt_snapshot_at?: string | null;
        interface_collected_at?: string | null;
        summary: unknown;
        pons: unknown;
        computed?: OltRow["computed"];
        pons_table?: PonTableRow[];
        interface_table?: IfRow[];
        optical_sensors?: Array<{ oid?: string; value?: string }>;
        vsol_onu_table?: VsOnuRow[];
        zte_onu_online_table?: ZteMibRow[];
        zte_pon_status_table?: ZteMibRow[];
        zte_transceiver_table?: ZteMibRow[];
        collection_log?: Record<string, unknown>;
        snmp_debug?: Record<string, unknown>;
      }>(`/api/v1/olt/devices/${sel}`),
  });

  const OLT_REFRESH_ONU_MS = 110 * 1000;
  const OLT_REFRESH_FULL_MS = 15 * 60 * 1000;
  const OLT_TELEMETRY_MS = 15 * 60 * 1000;

  const refresh = useMutation({
    mutationFn: ({ id, scope }: { id: string; scope: "onu" | "full" | "telemetry" }) => {
      const q =
        scope === "telemetry"
          ? "?telemetry=1&scope=onu"
          : scope === "onu"
            ? "?scope=onu"
            : "?scope=full";
      const timeoutMs =
        scope === "telemetry" ? OLT_TELEMETRY_MS : scope === "onu" ? OLT_REFRESH_ONU_MS : OLT_REFRESH_FULL_MS;
      return apiFetch<{
        computed?: OltRow["computed"];
        collection_log?: Record<string, unknown>;
        vsol_onu_table?: VsOnuRow[];
      }>(`/api/v1/olt/devices/${id}/refresh${q}`, {
        method: "POST",
        json: {},
        timeoutMs,
      });
    },
    onSuccess: (data, { id, scope }) => {
      setRefreshScope(scope);
      const comp = data.computed;
      const sum = (data as { summary?: unknown }).summary;
      let sumObj: Record<string, unknown> | undefined;
      if (sum && typeof sum === "object" && !Array.isArray(sum)) {
        sumObj = sum as Record<string, unknown>;
      } else if (typeof sum === "string" && sum.trim()) {
        try {
          sumObj = JSON.parse(sum) as Record<string, unknown>;
        } catch {
          sumObj = undefined;
        }
      }
      if (sumObj && Array.isArray(sumObj.onu_metrics_walks)) {
        setMetricsWalkRows(sumObj.onu_metrics_walks as MetricsWalkRow[]);
      }
      const on = comp?.onu_online_sum ?? 0;
      const tot = comp?.onu_total_sum ?? 0;
      setSaveToast({
        ok: true,
        text: `Coleta concluída (${scope === "onu" ? "rápida" : scope === "telemetry" ? "telemetria" : "completa"}): ${on} online / ${tot} ONUs`,
      });
      qc.invalidateQueries({ queryKey: ["olt-devices"] });
      qc.invalidateQueries({ queryKey: ["olt-device", id] });
      void invalidateAlertListQueries(qc);
    },
    onError: (err) => {
      setSaveToast({ ok: false, text: (err as Error).message || "Falha na coleta OLT" });
    },
  });

  const refreshIf = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ["olt-device", id] });
      void invalidateAlertListQueries(qc);
    },
  });

  async function runBulkSnapshotAndInterfaces() {
    setBulkPhase("running");
    setBulkRunning(true);
    setBulkLog([]);
    setBulkCollectedRows([]);
    try {
      const data = await apiFetch<{ olts: OltRow[] }>("/api/v1/olt/devices");
      const sel = new Set(bulkSelectedIds);
      const olts = (data.olts ?? []).filter((o) => sel.has(o.id));
      if (olts.length === 0) {
        setBulkLog(["Seleccione pelo menos uma OLT na lista abaixo (ou abra novamente o diálogo para repor a selecção)."]);
        return;
      }
      setBulkProgress({ done: 0, total: olts.length });
      const collected: Array<{ olt_id: string; olt_description: string; locality_id: string; locality_name: string; online: number; offline: number; total: number }> = [];
      const toInt = (v: unknown) => (Number.isFinite(Number(v)) ? Number(v) : 0);
      for (const o of olts) {
        const label = (o.description && String(o.description).trim()) || o.id;
        setBulkLog((m) => [...m, `${label}: snapshot OLT…`]);
        const snap = await apiFetch<{ computed?: { onu_online_sum?: number; onu_offline_sum?: number; onu_total_sum?: number } }>(`/api/v1/olt/devices/${o.id}/refresh`, { method: "POST", json: {} });
        setBulkLog((m) => [...m, `${label}: interfaces (SNMP)…`]);
        await apiFetch(`/api/v1/interfaces/devices/${o.id}/refresh`, { method: "POST", json: {} });
        const online = toInt(snap.computed?.onu_online_sum);
        const offline = toInt(snap.computed?.onu_offline_sum);
        const total = toInt(snap.computed?.onu_total_sum);
        if (o.locality_id) {
          collected.push({
            olt_id: o.id,
            olt_description: label,
            locality_id: o.locality_id,
            locality_name: o.locality_name || o.locality_id,
            online,
            offline,
            total,
          });
        }
        setBulkProgress((p) => ({ ...p, done: Math.min(p.total, p.done + 1) }));
      }
      setBulkCollectedRows(collected);
      setBulkPhase("results");
      setBulkLog((m) => [...m.slice(-12), "Concluído."]);
      await qc.invalidateQueries({ queryKey: ["olt-devices"] });
      if (sel) await qc.invalidateQueries({ queryKey: ["olt-device", sel] });
      await invalidateAlertListQueries(qc);
    } catch (e) {
      setBulkLog((m) => [...m, (e as Error).message || String(e)]);
    } finally {
      setBulkRunning(false);
    }
  }

  const saveBulkToCommercial = useMutation({
    mutationFn: async () => {
      const byLoc = new Map<string, { locality_id: string; client_count: number }>();
      for (const r of bulkCollectedRows) {
        const ex = byLoc.get(r.locality_id);
        if (ex) ex.client_count += r.total;
        else byLoc.set(r.locality_id, { locality_id: r.locality_id, client_count: r.total });
      }
      return apiFetch<{ upserted: number }>("/api/v1/commercial/monthly-records/bulk", {
        method: "POST",
        json: { records: [...byLoc.values()].map((r) => ({ ...r, year_month: bulkMonth })) },
      });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      await qc.invalidateQueries({ queryKey: ["commercial-agg"] });
      await qc.invalidateQueries({ queryKey: ["commercial-cmp"] });
      setBulkLog((m) => [...m, `Base comercial actualizada para ${formatYearMonthPt(bulkMonth)}.`]);
      setSaveToast({ ok: true, text: `Guardado com sucesso (Base comercial: ${formatYearMonthPt(bulkMonth)}).` });
    },
    onError: (err: Error) => setSaveToast({ ok: false, text: err.message || "Falha ao salvar (Base comercial)." }),
  });

  const rows = list.data?.olts ?? [];
  const sortedRows = useMemo(() => {
    const out = [...rows];
    const num = (v: unknown) => (Number.isFinite(Number(v)) ? Number(v) : -1);
    const txt = (v: unknown) => String(v ?? "").trim().toLowerCase();
    const ts = (v: unknown) => {
      const t = Date.parse(String(v ?? ""));
      return Number.isFinite(t) ? t : 0;
    };
    out.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "description":
          cmp = txt(a.description).localeCompare(txt(b.description), "pt");
          break;
        case "ip":
          cmp = txt(a.ip).localeCompare(txt(b.ip), "pt");
          break;
        case "pons":
          cmp = num(a.computed?.pon_count) - num(b.computed?.pon_count);
          break;
        case "onus":
          cmp = num(a.computed?.onu_total_sum) - num(b.computed?.onu_total_sum);
          break;
        case "online":
          cmp = num(a.computed?.onu_online_sum) - num(b.computed?.onu_online_sum);
          break;
        case "offline":
          cmp = num(a.computed?.onu_offline_sum) - num(b.computed?.onu_offline_sum);
          break;
        case "updated":
          cmp = ts(a.olt_snapshot_at) - ts(b.olt_snapshot_at);
          break;
      }
      if (cmp == 0) cmp = txt(a.description).localeCompare(txt(b.description), "pt");
      return sortDir === "asc" ? cmp : -cmp;
    });
    return out;
  }, [rows, sortDir, sortKey]);
  const sortArrow = (k: OltSortKey) => (sortKey !== k ? "↕" : sortDir === "asc" ? "↑" : "↓");
  const toggleSort = (k: OltSortKey) => {
    setSortKey((cur) => {
      if (cur === k) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
        return cur;
      }
      setSortDir(k === "updated" ? "desc" : "asc");
      return k;
    });
  };

  if (list.isLoading) return <p>A carregar OLTs…</p>;
  if (list.isError) return <div className="msg msg--err">{(list.error as Error).message}</div>;

  const bulkOltFilterNorm = bulkOltFilter.trim().toLowerCase();
  const bulkListFiltered = !bulkOltFilterNorm
    ? rows
    : rows.filter((o) => {
        const d = String(o.description ?? "").toLowerCase();
        const ip = String(o.ip ?? "").toLowerCase();
        return d.includes(bulkOltFilterNorm) || ip.includes(bulkOltFilterNorm) || o.id.toLowerCase().includes(bulkOltFilterNorm);
      });
  const ponsRows = detail.data?.pons_table ?? [];
  const ifRows = detail.data?.interface_table ?? [];
  const sensors = detail.data?.optical_sensors ?? [];
  const comp = detail.data?.computed;
  const vsolOnuRows = detail.data?.vsol_onu_table ?? [];
  const summaryObj = detail.data?.summary as Record<string, unknown> | undefined;
  const walkRowsFromSnapshot = Array.isArray(summaryObj?.onu_metrics_walks)
    ? (summaryObj.onu_metrics_walks as MetricsWalkRow[])
    : [];
  const metricsWalkLogRows = metricsWalkRows ?? walkRowsFromSnapshot;
  const metricsLogNote =
    typeof summaryObj?.onu_metrics_note === "string" && summaryObj.onu_metrics_note.trim()
      ? summaryObj.onu_metrics_note.trim()
      : null;
  const metricsLogElapsed =
    typeof summaryObj?.onu_metrics_elapsed_ms === "number" ? summaryObj.onu_metrics_elapsed_ms : null;
  const selectedOlt = rows.find((x) => x.id === sel);
  const isZte =
    String(selectedOlt?.brand ?? "")
      .toLowerCase()
      .includes("zte") ||
    String(selectedOlt?.model ?? "")
      .toLowerCase()
      .includes("zte");
  const mibHint = String(summaryObj?.vsol_mib ?? "").toLowerCase();
  const vsolHint =
    vsolOnuRows.length > 0 ||
    mibHint.includes("gonuauthlist") ||
    mibHint.includes("vsol") ||
    String(summaryObj?.source ?? "")
      .toLowerCase()
      .includes("vsol") ||
    String(selectedOlt?.brand ?? "")
      .toLowerCase()
      .includes("vsol") ||
    ((m) => m.includes("vsol") || m.includes("v1600") || m.includes("1600g"))(String(selectedOlt?.model ?? "").toLowerCase());
  const zteOnuRows = (detail.data?.zte_onu_online_table ?? []) as ZteMibRow[];
  const ztePonRows = (detail.data?.zte_pon_status_table ?? []) as ZteMibRow[];
  const zteTrxRows = (detail.data?.zte_transceiver_table ?? []) as ZteMibRow[];

  if (pageTab === "relatorios") {
    return (
      <>
        <div className="page-heading" style={{ marginBottom: 8 }}>
          <h1>OLT</h1>
        </div>
        <div className="tabs" style={{ marginBottom: 16 }}>
          <button type="button" className="" onClick={() => setPageTab("equipamentos")}>
            Equipamentos
          </button>
          <button type="button" className="active" onClick={() => setPageTab("relatorios")}>
            Relatórios
          </button>
        </div>
        <OltReportsTab />
      </>
    );
  }

  return (
    <>
      <div className="tabs" style={{ marginBottom: 12 }}>
        <button type="button" className="active" onClick={() => setPageTab("equipamentos")}>
          Equipamentos
        </button>
        <button type="button" onClick={() => setPageTab("relatorios")}>
          Relatórios
        </button>
      </div>
      <div className="row" style={{ flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12, marginBottom: 12 }}>
        <div className="page-heading" style={{ marginBottom: 0, flex: "1 1 280px" }}>
          <h1>
            OLT — PONs, ONUs e interfaces
            <InfoHint label="Sobre dados OLT e PON">
              <p>
                Equipamentos OLT. Os dados de PON e ONU vêm do perfil SNMP configurado em{" "}
                <strong>Definições → Perfis OLT</strong> (serial, estado, RX, TX, temperatura, modelo). Clique numa linha para ver detalhes.
              </p>
            </InfoHint>
          </h1>
          <PageCountPill label="OLTs" count={rows.length} />
        </div>
        {canMutate ? (
          <button
            type="button"
            className="btn btn--primary"
            disabled={bulkRunning}
            onClick={() => {
              setBulkSelectedIds((list.data?.olts ?? []).map((o) => o.id));
              setBulkOltFilter("");
              setBulkPhase("select");
              setBulkOpen(true);
            }}
          >
            {bulkRunning ? "Actualização em massa…" : "Actualizar dados e interfaces (OLTs seleccionadas)"}
          </button>
        ) : null}
      </div>
      {bulkOpen && canMutate && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !bulkRunning && setBulkOpen(false)}>
          <div
            className="modal modal--wide olt-bulk-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="bulk-olt-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="olt-bulk-modal__head">
              <h2 id="bulk-olt-title" style={{ marginTop: 0 }}>
                Actualizar OLTs em massa
              </h2>
              <p style={{ fontSize: 13, color: "var(--muted)", marginBottom: 0 }}>
                {bulkPhase === "results" ? "Colecta concluída. Revise os totais abaixo." : "Snapshot OLT e interfaces por OLT seleccionada."}
              </p>
            </div>
            <div className="olt-bulk-modal__body">
            {bulkPhase !== "results" && (
            <>
            <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap", marginBottom: 8 }}>
              <input
                className="input"
                style={{ flex: "1 1 180px", minWidth: 140 }}
                placeholder="Filtrar por nome, IP ou ID…"
                value={bulkOltFilter}
                onChange={(e) => setBulkOltFilter(e.target.value)}
                disabled={bulkRunning}
              />
              <button type="button" className="btn" disabled={bulkRunning} onClick={() => setBulkSelectedIds(rows.map((o) => o.id))}>
                Marcar todas
              </button>
              <button type="button" className="btn" disabled={bulkRunning} onClick={() => setBulkSelectedIds([])}>
                Limpar
              </button>
            </div>
            <div
              className="table-wrap"
              style={{ maxHeight: 200, overflow: "auto", marginBottom: 10, border: "1px solid var(--border)", borderRadius: "var(--radius)" }}
            >
              <table style={{ fontSize: 12, width: "100%" }}>
                <tbody>
                  {bulkListFiltered.length === 0 && (
                    <tr>
                      <td colSpan={2} style={{ padding: 12, color: "var(--muted)" }}>
                        {rows.length === 0 ? "Nenhuma OLT na lista." : "Nenhum resultado com este filtro."}
                      </td>
                    </tr>
                  )}
                  {bulkListFiltered.map((o) => {
                    const label = (o.description && String(o.description).trim()) || o.id;
                    const checked = bulkSelectedIds.includes(o.id);
                    return (
                      <tr key={o.id}>
                        <td style={{ width: 36, verticalAlign: "middle" }}>
                          <input
                            type="checkbox"
                            checked={checked}
                            disabled={bulkRunning}
                            onChange={() =>
                              setBulkSelectedIds((prev) => (prev.includes(o.id) ? prev.filter((x) => x !== o.id) : [...prev, o.id]))
                            }
                          />
                        </td>
                        <td>
                          {label}
                          <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                            {o.ip ?? ""} · {o.id}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <p className="mono" style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
              {bulkSelectedIds.length} de {rows.length} OLTs seleccionadas
              {bulkOltFilterNorm ? ` · ${bulkListFiltered.length} visíveis com o filtro` : ""}
            </p>
            </>
            )}
            {(bulkRunning || bulkPhase === "results") && (
            <p className="mono" style={{ fontSize: 12, marginTop: 8 }}>
              Progresso: {bulkProgress.done}/{bulkProgress.total} concluídas
            </p>
            )}
            {bulkLog.length > 0 && (
              <pre className="mono" style={{ fontSize: 11, maxHeight: 100, overflow: "auto", background: "var(--panel2)", padding: 8, borderRadius: "var(--radius)" }}>
                {bulkLog.slice(-15).join("\n")}
              </pre>
            )}
            {bulkCollectedRows.length > 0 && (
              <>
                <h3 style={{ marginBottom: 6, fontSize: 14 }}>Conferência para Base comercial</h3>
                <div className="table-wrap" style={{ maxHeight: 180, overflow: "auto" }}>
                  <table style={{ fontSize: 11 }}>
                    <thead>
                      <tr>
                        <th>OLT</th>
                        <th>Localidade</th>
                        <th className="mono">Online</th>
                        <th className="mono">Offline</th>
                        <th className="mono">Total</th>
                      </tr>
                    </thead>
                    <tbody>
                      {bulkCollectedRows.map((r) => (
                        <tr key={r.olt_id}>
                          <td>{r.olt_description}</td>
                          <td>{r.locality_name}</td>
                          <td className="mono">{r.online}</td>
                          <td className="mono">{r.offline}</td>
                          <td className="mono">{r.total}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="row" style={{ gap: 8, marginTop: 8, alignItems: "flex-end", flexWrap: "wrap" }}>
                  <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 200 }}>
                    <label htmlFor="olt-commercial-bulk-month" style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
                      Mês na base comercial
                    </label>
                    <select id="olt-commercial-bulk-month" className="select" value={bulkMonth} onChange={(e) => setBulkMonth(e.target.value)}>
                      {monthSelectChoicesWithFallback(bulkMonthChoices, bulkMonth).map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <button type="button" className="btn btn--primary" disabled={saveBulkToCommercial.isPending} onClick={() => saveBulkToCommercial.mutate()}>
                    {saveBulkToCommercial.isPending ? "Salvando…" : "Confirmar e salvar na Base comercial"}
                  </button>
                </div>
                {saveToast && (
                  <div className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 10 }}>
                    <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
                      ×
                    </button>
                    {saveToast.text}
                  </div>
                )}
              </>
            )}
            </div>
            <div className="olt-bulk-modal__foot">
            <div className="row" style={{ gap: 8, justifyContent: "flex-end", flexWrap: "wrap" }}>
              <button
                type="button"
                className="btn"
                disabled={bulkRunning}
                onClick={() => {
                  setBulkOpen(false);
                  setBulkPhase("select");
                }}
              >
                {bulkRunning ? "…" : "Fechar"}
              </button>
              {bulkPhase !== "results" && (
              <button
                type="button"
                className="btn btn--primary"
                disabled={bulkRunning || bulkSelectedIds.length === 0}
                onClick={() => void runBulkSnapshotAndInterfaces()}
              >
                Confirmar e iniciar
              </button>
              )}
            </div>
            </div>
          </div>
        </div>
      )}
      <style>
        {`
          @keyframes olt-fade-slide-in {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
          }
          .olt-update-menu__item {
            width: 100%;
            border: 0;
            background: transparent;
            text-align: left;
            padding: 8px 10px;
            font-size: 12px;
            color: inherit;
            cursor: pointer;
          }
          .olt-update-menu__item:hover { background: var(--hover-bg-menu); }
          .olt-update-menu__item:disabled { opacity: 0.5; cursor: not-allowed; }
          .olt-anim-enter { animation: olt-fade-slide-in 220ms ease; }
        `}
      </style>
      {!sel ? (
        <div className="table-wrap olt-anim-enter" style={{ maxWidth: "100%", overflowX: "auto" }}>
          <table style={{ fontSize: 12 }}>
            <thead>
              <tr>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("description")}>Descrição {sortArrow("description")}</th>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("ip")}>IP {sortArrow("ip")}</th>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("pons")}>PONs {sortArrow("pons")}</th>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("onus")}>ONUs {sortArrow("onus")}</th>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("online")}>Online {sortArrow("online")}</th>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("offline")}>Offline {sortArrow("offline")}</th>
                <th style={{ cursor: "pointer" }} onClick={() => toggleSort("updated")}>Última atualização {sortArrow("updated")}</th>
              </tr>
            </thead>
            <tbody>
              {sortedRows.map((r) => {
                const c = r.computed;
                return (
                  <tr key={r.id} onClick={() => setSel(r.id)} style={{ cursor: "pointer" }}>
                    <td>{r.description ?? "—"}</td>
                    <td className="mono">{r.ip ?? "—"}</td>
                    <td className="mono">{formatNum(c?.pon_count)}</td>
                    <td className="mono">{formatNum(c?.onu_total_sum)}</td>
                    <td className="mono">{formatNum(c?.onu_online_sum)}</td>
                    <td className="mono">{formatNum(c?.onu_offline_sum)}</td>
                    <td className="mono">{formatCollectedPt(r.olt_snapshot_at)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {rows.length === 0 && <p style={{ padding: 12, color: "var(--muted)" }}>Nenhuma OLT cadastrada.</p>}
        </div>
      ) : (
        <div className="olt-anim-enter" style={{ maxWidth: "100%" }}>
          <button
            type="button"
            className="btn"
            style={{ marginBottom: 12 }}
            onClick={() => {
              setSel(null);
              setSnmpDebugOpen(false);
              setCollectLogOpen(false);
              setMetricsWalkRows(null);
            }}
          >
            ← Todas as OLTs
          </button>
          <div className="card" style={{ position: "relative", paddingTop: 12 }}>
            {detail.isLoading && <p>A carregar…</p>}
            {detail.isError && <div className="msg msg--err">{(detail.error as Error).message}</div>}
            {detail.data && (
              <>
                <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "center" }}>
                  <h2 style={{ margin: 0 }}>{detail.data.description}</h2>
                  <span className="mono" style={{ color: "var(--muted)" }}>{detail.data.ip}</span>
                  <Link
                    to={`/devices?report=${encodeURIComponent(sel)}`}
                    className="btn"
                    title="Abrir relatório da OLT em Equipamentos"
                    style={{ display: "flex", alignItems: "center", justifyContent: "center", position: "relative", paddingLeft: 14, paddingRight: 36 }}
                  >
                    <span style={{ flex: "1 1 auto", textAlign: "center" }}>Equipamentos</span>
                    <span style={{ position: "absolute", right: 10, top: "50%", transform: "translateY(-50%)", display: "flex", pointerEvents: "none" }}>
                      <IconExternalLinkSubtle size={15} />
                    </span>
                  </Link>
                  {canMutate ? (
                    <>
                      <button
                        type="button"
                        className="btn btn--primary"
                        disabled={refresh.isPending || refreshIf.isPending}
                        onClick={() => sel && refresh.mutate({ id: sel, scope: "onu" })}
                      >
                        {refresh.isPending && refreshScope === "onu" ? "A coletar ONUs…" : "Atualizar ONUs"}
                      </button>
                      <DropdownMenu
                        key={sel ?? "olt-update"}
                        align="end"
                        className="dropdown"
                        trigger={({ toggle, open }) => (
                          <button
                            type="button"
                            className="btn"
                            disabled={refresh.isPending || refreshIf.isPending}
                            aria-haspopup="menu"
                            aria-expanded={open}
                            onClick={toggle}
                            title="Mais opções de coleta"
                          >
                            Mais ▾
                          </button>
                        )}
                      >
                        {({ close }) => (
                          <>
                            <button
                              type="button"
                              className="olt-update-menu__item"
                              disabled={refresh.isPending}
                              onClick={() => {
                                close();
                                if (sel) refresh.mutate({ id: sel, scope: "full" });
                              }}
                            >
                              Coleta completa (interfaces + ONUs)
                            </button>
                            {vsolHint ? (
                              <button
                                type="button"
                                className="olt-update-menu__item"
                                disabled={refresh.isPending}
                                onClick={() => {
                                  close();
                                  if (sel) refresh.mutate({ id: sel, scope: "telemetry" });
                                }}
                              >
                                Só métricas das ONUs (RX, modelo…)
                              </button>
                            ) : null}
                            <button
                              type="button"
                              className="olt-update-menu__item"
                              disabled={refreshIf.isPending}
                              onClick={() => {
                                close();
                                refreshIf.mutate(sel!);
                              }}
                            >
                              Só interfaces de rede
                            </button>
                          </>
                        )}
                      </DropdownMenu>
                    </>
                  ) : null}
                  {vsolHint && canMutate ? (
                    <button type="button" className="btn" onClick={() => setSnmpDebugOpen((v) => !v)}>
                      {snmpDebugOpen ? "Ocultar tabela SNMP" : "Tabela SNMP"}
                    </button>
                  ) : null}
                  <button type="button" className="btn" onClick={() => setCollectLogOpen(true)} title="Detalhe de cada snmpwalk por métrica">
                    Logs da coleta
                  </button>
                </div>
                <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 6, marginBottom: 0 }}>
                  Última coleta OLT: {formatCollectedPt(detail.data.olt_snapshot_at)} · Interfaces:{" "}
                  {formatCollectedPt(detail.data.interface_collected_at)}
                  {refresh.isPending ? " · coleta em curso…" : ""}
                </p>

                {saveToast && (
                  <div
                    className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`}
                    role="status"
                    style={{ marginTop: 8 }}
                  >
                    <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
                      ×
                    </button>
                    {saveToast.text}
                  </div>
                )}

                {collectLogOpen ? (
                  <OltMetricsCollectLogModal
                    open={collectLogOpen}
                    onClose={() => setCollectLogOpen(false)}
                    walkRows={metricsWalkLogRows}
                    elapsedMs={metricsLogElapsed}
                    note={metricsLogNote}
                  />
                ) : null}
                {vsolHint && snmpDebugOpen && sel ? (
                  <OltSnmpDebugPanel
                    deviceId={sel}
                    open={snmpDebugOpen}
                    onClose={() => setSnmpDebugOpen(false)}
                    initialDebug={(detail.data.snmp_debug ?? summaryObj?.snmp_debug) as Record<string, unknown> | undefined}
                  />
                ) : null}
                {refreshIf.isError && <div className="msg msg--err">{(refreshIf.error as Error).message}</div>}

                <div className="grid-cards" style={{ marginTop: 12 }}>
                  <div className="stat"><div className="stat__k">PONs (linhas / resumo)</div><div className="stat__v">{formatNum(comp?.pon_count)}</div></div>
                  <div className="stat"><div className="stat__k">ONUs total (soma PONs)</div><div className="stat__v">{formatNum(comp?.onu_total_sum)}</div></div>
                  <div className="stat"><div className="stat__k">Online</div><div className="stat__v">{formatNum(comp?.onu_online_sum)}</div></div>
                  <div className="stat"><div className="stat__k">Offline</div><div className="stat__v">{formatNum(comp?.onu_offline_sum)}</div></div>
                </div>

                <h2 style={{ marginTop: 18 }}>Portas PON</h2>
                <div className="table-wrap" style={{ maxHeight: 280, overflow: "auto" }}>
                  <table className="table table--compact" style={{ width: "100%", fontSize: 12 }}>
                    <thead>
                      <tr>
                        <th>Porta / ID</th>
                        <th>Nome</th>
                        <th className="mono">RX PON</th>
                        <th className="mono">TX PON</th>
                        <th className="mono">Total</th>
                        <th className="mono">On</th>
                        <th className="mono">Off</th>
                        <th>Estado</th>
                      </tr>
                    </thead>
                    <tbody>
                      {ponsRows.map((p, i) => (
                        <tr key={`${p.id}-${i}`}>
                          <td className="mono">{p.id || `#${i}`}</td>
                          <td>{p.name ?? "—"}</td>
                          <td className="mono">{p.rx_dbm != null ? format1f(p.rx_dbm) : "—"}</td>
                          <td className="mono">{p.tx_dbm != null ? format1f(p.tx_dbm) : "—"}</td>
                          <td className="mono">{formatNum(p.onu_total)}</td>
                          <td className="mono">{formatNum(p.onu_online)}</td>
                          <td className="mono">{formatNum(p.onu_offline)}</td>
                          <td><span className={badgeOper(p.status)}>{p.status ?? "—"}</span></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {ponsRows.length === 0 && (
                  <p style={{ fontSize: 12, color: "var(--muted)" }}>
                    Sem portas PON registadas. Confira IP e community SNMP e use «Atualizar ONUs» ou coleta completa.
                  </p>
                )}

              {vsolHint && (
                <>
                  <h2 style={{ marginTop: 20 }}>
                    ONUs — estado e telemetria{" "}
                    <span style={{ fontWeight: 400, fontSize: 12, color: "var(--muted)" }}>({vsolOnuRows.length})</span>
                  </h2>
                  <OltVsolOnuTable
                    rows={vsolOnuRows}
                    onuRefs={typeof summaryObj?.vsol_onu_refs_count === "number" ? summaryObj.vsol_onu_refs_count : undefined}
                    note={
                      summaryObj?.onu_metrics_note
                        ? String(summaryObj.onu_metrics_note)
                        : summaryObj?.vsol_get_note
                          ? String(summaryObj.vsol_get_note)
                          : summaryObj?.vsol_walk_note
                            ? String(summaryObj.vsol_walk_note)
                            : summaryObj?.onu_metrics_missing
                              ? "Nenhuma MIB SNMP configurada para monitoramento deste modelo. Configure em Definições → Perfis OLT."
                              : undefined
                    }
                  />
                </>
              )}

              {isZte && (
                <details className="collapsible-section" style={{ marginTop: 18 }}>
                  <summary>
                    ZTE MIB (PON/ONU/Transceiver){" "}
                    <span style={{ fontWeight: 400, color: "var(--muted)" }}>
                      ({zteOnuRows.length + ztePonRows.length + zteTrxRows.length})
                    </span>
                  </summary>
                  <div className="collapsible-section__body">
                    <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                      Dados coletados conforme OIDs configurados para marca ZTE em <code className="mono">Configurações &gt; Perfil OLT por marca</code>.
                    </p>
                    {String(summaryObj?.zte_walk_note ?? "").trim() ? (
                      <p className="msg msg--off" style={{ fontSize: 11 }}>
                        Nota walk: {String(summaryObj?.zte_walk_note)}
                      </p>
                    ) : null}

                    <h3 style={{ marginTop: 10, marginBottom: 6, fontSize: 14 }}>ONU online (ZTE)</h3>
                    <div className="table-wrap" style={{ maxHeight: 220, overflow: "auto", maxWidth: "100%" }}>
                      <table style={{ fontSize: 10, width: "100%", tableLayout: "fixed" }}>
                        <thead>
                          <tr>
                            <th>Suffix</th>
                            <th>Porta</th>
                            <th>Valor</th>
                            <th>Tipo</th>
                            <th style={{ width: "32%" }}>OID</th>
                          </tr>
                        </thead>
                        <tbody>
                          {zteOnuRows.map((r, i) => (
                            <tr key={`zte-onu-${i}`}>
                              <td className="mono">{r.suffix ?? "—"}</td>
                              <td>{r.if_name ?? (r.if_index != null ? `ifIndex ${r.if_index}` : "—")}</td>
                              <td className="mono">{r.value ?? "—"}</td>
                              <td className="mono">{r.type ?? "—"}</td>
                              <td className="mono" style={{ wordBreak: "break-all", overflowWrap: "anywhere" }}>
                                {r.oid ?? "—"}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>

                    <h3 style={{ marginTop: 10, marginBottom: 6, fontSize: 14 }}>Status PON (ZTE)</h3>
                    <div className="table-wrap" style={{ maxHeight: 220, overflow: "auto", maxWidth: "100%" }}>
                      <table style={{ fontSize: 10, width: "100%", tableLayout: "fixed" }}>
                        <thead>
                          <tr>
                            <th>Suffix</th>
                            <th>Porta</th>
                            <th>Valor</th>
                            <th>Estado</th>
                            <th>Tipo</th>
                            <th style={{ width: "28%" }}>OID</th>
                          </tr>
                        </thead>
                        <tbody>
                          {ztePonRows.map((r, i) => (
                            <tr key={`zte-pon-${i}`}>
                              <td className="mono">{r.suffix ?? "—"}</td>
                              <td>{r.if_name ?? (r.if_index != null ? `ifIndex ${r.if_index}` : "—")}</td>
                              <td className="mono">{r.value ?? "—"}</td>
                              <td>
                                <span className={badgeOper(r.value_label)}>{r.value_label ?? "—"}</span>
                              </td>
                              <td className="mono">{r.type ?? "—"}</td>
                              <td className="mono" style={{ wordBreak: "break-all", overflowWrap: "anywhere" }}>
                                {r.oid ?? "—"}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>

                    <h3 style={{ marginTop: 10, marginBottom: 6, fontSize: 14 }}>Transceiver (ZTE)</h3>
                    {zteTrxRows.length === 0 ? (
                      <p style={{ fontSize: 11, color: "var(--muted)" }}>
                        Sem retorno no OID de transceiver configurado para ZTE.
                      </p>
                    ) : null}
                    <div className="table-wrap" style={{ maxHeight: 220, overflow: "auto", maxWidth: "100%" }}>
                      <table style={{ fontSize: 10, width: "100%", tableLayout: "fixed" }}>
                        <thead>
                          <tr>
                            <th>Suffix</th>
                            <th>Porta</th>
                            <th>Valor</th>
                            <th>Tipo</th>
                            <th style={{ width: "32%" }}>OID</th>
                          </tr>
                        </thead>
                        <tbody>
                          {zteTrxRows.map((r, i) => (
                            <tr key={`zte-trx-${i}`}>
                              <td className="mono">{r.suffix ?? "—"}</td>
                              <td>{r.if_name ?? (r.if_index != null ? `ifIndex ${r.if_index}` : "—")}</td>
                              <td className="mono">{r.value ?? "—"}</td>
                              <td className="mono">{r.type ?? "—"}</td>
                              <td className="mono" style={{ wordBreak: "break-all", overflowWrap: "anywhere" }}>
                                {r.oid ?? "—"}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </details>
              )}

              <h2 style={{ marginTop: 18 }}>Interfaces (IF-MIB / ifXTable)</h2>
              <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                Colunas <strong>Entrada/Saída</strong> são contadores de tráfego (octets). <strong>RX/TX dBm</strong> são potências ópticas quando o equipamento expõe dados compatíveis. Valores{" "}
                <code className="mono">2147483647</code> (máx. 32&nbsp;bit) são omitidos — use 64&nbsp;bit (ifHC*) na OLT se disponível.
              </p>
              <p style={{ fontSize: 11, color: "var(--muted)", marginTop: -6 }}>
                Em ZTE esses contadores podem ficar muito altos, pois são acumulados desde o uptime da porta (não é taxa instantânea).
              </p>
              {ifRows.length > 0 ? <OltIfaceSection rows={ifRows as IfRow[]} /> : null}
              {ifRows.length === 0 && <p style={{ fontSize: 12, color: "var(--muted)" }}>Use «Actualizar interfaces» para colher IF-MIB desta OLT.</p>}

              {sensors.length > 0 && (
                <details className="collapsible-section" style={{ marginTop: 18 }}>
                  <summary>
                    Sensores SFP (ENTITY) <span style={{ fontWeight: 400, color: "var(--muted)" }}>({sensors.length})</span>
                  </summary>
                  <div className="collapsible-section__body">
                    <div className="table-wrap" style={{ maxHeight: 200, overflow: "auto" }}>
                      <table className="table table--compact" style={{ width: "100%", fontSize: 12 }}>
                        <thead>
                          <tr>
                            <th>OID</th>
                            <th>Valor</th>
                          </tr>
                        </thead>
                        <tbody>
                          {sensors.slice(0, 80).map((s, i) => (
                            <tr key={`${s.oid}-${i}`}>
                              <td className="mono" style={{ wordBreak: "break-all" }}>
                                {s.oid}
                              </td>
                              <td className="mono">{s.value}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </details>
              )}
              </>
            )}
          </div>
        </div>
      )}
    </>
  );
}
