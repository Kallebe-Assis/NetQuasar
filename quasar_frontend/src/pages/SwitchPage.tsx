import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { PageCountPill } from "../components/PageCountPill";
import { MikrotikNocDashboard, type MikrotikNocSection } from "../components/MikrotikNocDashboard";
import { apiFetch } from "../lib/api";
import { EM_DASH, formatDbm } from "../lib/formatDisplay";
import { formatBitrate } from "../lib/formatBitrate";
import { isAdminUser } from "../lib/auth";
import { DropdownMenu } from "../components/DropdownMenu";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { collectDeviceTelemetry } from "../lib/telemetryCollectToast";
import { formatCollectedPt } from "../lib/deviceReportHelpers";
import { parseSwitchCollectionStatus, buildSwitchNocKpis, type MikrotikIfRow } from "../lib/switchNocData";
import { ifDisplayName } from "../lib/mikrotikNocData";

type DeviceRow = {
  id: string;
  description?: string | null;
  category?: string | null;
  brand?: string | null;
  model?: string | null;
  ip?: string | null;
  telemetry_enabled?: boolean;
  mikrotik_telnet_profile_id?: string | null;
  switch_telnet_profile_id?: string | null;
};

type SensorRow = { oid?: string; value?: string; type?: string };

function isSwitch(d: DeviceRow): boolean {
  return String(d.category ?? "").trim().toLowerCase() === "switch";
}

function inferIfaceType(r: MikrotikIfRow): string {
  const n = String(r.if_name ?? r.display_name ?? r.descr ?? "").toLowerCase();
  if (n.includes("port-channel") || n.includes("portchannel")) return "Port-Channel";
  if (n.includes("mgmt")) return "Mgmt";
  if (n.includes("vlan")) return "VLAN";
  if (n.includes("ethernet") || n.startsWith("eth")) return "Ethernet";
  if (n.includes("sfp")) return "SFP";
  return "Outros";
}

function ifaceStatus(r: MikrotikIfRow): "up" | "down" | "other" {
  const s = String(r.oper_status ?? "").toLowerCase();
  if (s === "up") return "up";
  if (s === "down") return "down";
  return "other";
}

function MiniTrafficChart({ points }: { points: Array<{ ts: number; tx: number; rx: number }> }) {
  if (points.length < 2) {
    const p = points[0];
    return (
      <div>
        <p className="mk-noc-muted" style={{ margin: "0 0 6px 0" }}>Aguardando histórico do gráfico em tempo real.</p>
        <div className="row" style={{ gap: 12 }}>
          <span className="mono">TX atual: {p ? formatBitrate(p.tx) : "—"}</span>
          <span className="mono">RX atual: {p ? formatBitrate(p.rx) : "—"}</span>
        </div>
      </div>
    );
  }
  const w = 520;
  const h = 170;
  const pad = 16;
  const maxV = Math.max(1, ...points.map((p) => (Number.isFinite(p.tx) ? p.tx : 0)), ...points.map((p) => (Number.isFinite(p.rx) ? p.rx : 0)));
  const xFor = (i: number) => pad + (i * (w - pad * 2)) / Math.max(1, points.length - 1);
  const yFor = (v: number) => h - pad - (Math.max(0, v) / maxV) * (h - pad * 2);
  const txPath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(p.tx)}`).join(" ");
  const rxPath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(p.rx)}`).join(" ");
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label="Gráfico de tráfego da interface">
      <rect x={0} y={0} width={w} height={h} fill="transparent" />
      <line x1={pad} y1={h - pad} x2={w - pad} y2={h - pad} stroke="var(--border)" strokeWidth={1} />
      <path d={txPath} fill="none" stroke="var(--ok)" strokeWidth={2} />
      <path d={rxPath} fill="none" stroke="var(--accent)" strokeWidth={2} />
    </svg>
  );
}

export function SwitchPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const [telCollecting, setTelCollecting] = useState(false);
  const [sel, setSel] = useState<string | null>(null);
  const [section, setSection] = useState<MikrotikNocSection>("overview");
  const [realtimeOn, setRealtimeOn] = useState(false);
  const [realtimeMs, setRealtimeMs] = useState(3000);
  const [realtimeModalOpen, setRealtimeModalOpen] = useState(false);
  const [realtimeDraft, setRealtimeDraft] = useState("3000");
  const [liveTable, setLiveTable] = useState<MikrotikIfRow[]>([]);
  const [selectedChartIfs, setSelectedChartIfs] = useState<number[]>([]);
  const [trafficHistory, setTrafficHistory] = useState<Record<number, Array<{ ts: number; tx: number; rx: number }>>>({});
  const [cpuHistory, setCpuHistory] = useState<Array<{ ts: number; v: number }>>([]);
  const [memHistory, setMemHistory] = useState<Array<{ ts: number; v: number }>>([]);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "up" | "down">("all");
  const [typeFilter, setTypeFilter] = useState<"all" | "Ethernet" | "Port-Channel" | "VLAN" | "Mgmt" | "SFP" | "Outros">("all");
  const [trafficFilter] = useState<"all" | "with" | "without">("all");
  const chartsRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!canMutate && realtimeOn) setRealtimeOn(false);
  }, [canMutate, realtimeOn]);

  const devices = useQuery({
    queryKey: ["devices-switch-list"],
    queryFn: () => apiFetch<{ devices: DeviceRow[] }>("/api/v1/devices"),
  });

  const rows = useMemo(() => (devices.data?.devices ?? []).filter(isSwitch), [devices.data?.devices]);
  const selectedId = sel ?? rows[0]?.id ?? null;
  const selectedDevice = useMemo(() => rows.find((r) => r.id === selectedId) ?? null, [rows, selectedId]);

  const iface = useQuery({
    queryKey: ["switch-if", selectedId],
    enabled: !!selectedId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      apiFetch<{
        device_id: string;
        collected_at?: string;
        interface_table?: MikrotikIfRow[];
        interface_count?: number;
        walk_truncated?: boolean;
        walk_note?: string;
        optical_sensors?: SensorRow[];
        note?: string;
      }>(`/api/v1/interfaces/devices/${selectedId}`),
  });

  const telemetry = useQuery({
    queryKey: ["switch-tel", selectedId],
    enabled: !!selectedId,
    placeholderData: keepPreviousData,
    refetchInterval: selectedId ? 30_000 : false,
    queryFn: () => apiFetch<{ collected_at?: string; metrics?: Record<string, unknown> }>(`/api/v1/telemetry/devices/${selectedId}/latest`),
  });

  const telnetProfiles = useQuery({
    queryKey: ["switch-telnet-profiles"],
    queryFn: () => apiFetch<{ profiles: Array<{ id: string; name: string; is_default?: boolean }> }>("/api/v1/settings/switch-telnet-profiles"),
    staleTime: 60_000,
  });

  const patchTelnetProfile = useMutation({
    mutationFn: ({ deviceId, profileId }: { deviceId: string; profileId: string | null }) =>
      apiFetch(`/api/v1/devices/${deviceId}`, { method: "PATCH", json: { switch_telnet_profile_id: profileId } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["devices-switch-list"] });
      toastOk(pushToast, "Perfil telnet actualizado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao atribuir perfil."),
  });

  const refreshIf = useMutation({
    mutationFn: (id: string) =>
      apiFetch<{
        device_id: string;
        collected_at?: string;
        interface_table?: MikrotikIfRow[];
        interface_count?: number;
        walk_truncated?: boolean;
        walk_note?: string;
      }>(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} }),
    onSuccess: (data, id) => {
      if (data?.device_id) qc.setQueryData(["switch-if", id], data);
      qc.invalidateQueries({ queryKey: ["switch-if", id] });
      const n = data.interface_count ?? data.interface_table?.length;
      toastOk(pushToast, typeof n === "number" ? `Interfaces actualizadas (${n}).` : "Interfaces actualizadas.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao actualizar interfaces."),
  });

  const realtimeTick = useMutation({
    mutationFn: async (id: string) => {
      try {
        return await apiFetch<{ updates?: Array<{ if_index: number; tx_dbm?: number; rx_dbm?: number; in_bps?: number; out_bps?: number }> }>(
          `/api/v1/interfaces/devices/${id}/realtime`,
          { method: "POST", json: {} },
        );
      } catch (e) {
        const msg = String((e as Error)?.message ?? e);
        if (!msg.includes("404") && !msg.toLowerCase().includes("not found")) throw e;
        const full = await apiFetch<{ interface_table?: MikrotikIfRow[] }>(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} });
        return {
          updates: (full.interface_table ?? []).map((r) => ({
            if_index: r.if_index,
            tx_dbm: r.tx_dbm,
            rx_dbm: r.rx_dbm,
            in_bps: r.in_bps,
            out_bps: r.out_bps,
          })),
        };
      }
    },
    onSuccess: (data) => {
      const ups = data?.updates ?? [];
      if (ups.length === 0) return;
      setLiveTable((prev) =>
        prev.map((row) => {
          const up = ups.find((u) => u.if_index === row.if_index);
          if (!up) return row;
          return { ...row, tx_dbm: up.tx_dbm ?? row.tx_dbm, rx_dbm: up.rx_dbm ?? row.rx_dbm, in_bps: up.in_bps ?? row.in_bps, out_bps: up.out_bps ?? row.out_bps };
        }),
      );
    },
    onError: (e) => toastErr(pushToast, e, "Falha na actualização em tempo real."),
  });

  const runTelCollect = async () => {
    if (!selectedId || !selectedDevice) return;
    setTelCollecting(true);
    try {
      await collectDeviceTelemetry(selectedId, selectedDevice.description ?? "", { push: pushToast, dismiss: dismissToast }, qc);
      await qc.invalidateQueries({ queryKey: ["switch-tel", selectedId] });
    } finally {
      setTelCollecting(false);
    }
  };

  const cachedTable = useMemo(() => iface.data?.interface_table ?? [], [iface.data?.interface_table]);
  const table = realtimeOn && liveTable.length > 0 ? liveTable : cachedTable.length > 0 ? cachedTable : liveTable;
  const walkTruncated = Boolean(iface.data?.walk_truncated) || /truncad/i.test(String(iface.data?.walk_note ?? iface.data?.note ?? ""));
  const collectionStatus = useMemo(
    () => parseSwitchCollectionStatus(telemetry.data?.metrics as Record<string, unknown> | undefined),
    [telemetry.data?.metrics],
  );

  const interfaceRowsFiltered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return table.filter((r) => {
      const type = inferIfaceType(r);
      const status = ifaceStatus(r);
      const tx = Number(r.out_bps ?? 0);
      const rx = Number(r.in_bps ?? 0);
      const hasTraffic = Number.isFinite(tx) && Number.isFinite(rx) && (tx > 0 || rx > 0);
      if (statusFilter !== "all" && status !== statusFilter) return false;
      if (typeFilter !== "all" && type !== typeFilter) return false;
      if (trafficFilter === "with" && !hasTraffic) return false;
      if (trafficFilter === "without" && hasTraffic) return false;
      if (!q) return true;
      const hay = `${r.if_index} ${r.display_name ?? ""} ${r.if_name ?? ""} ${r.descr ?? ""} ${type}`.toLowerCase();
      return hay.includes(q);
    });
  }, [table, search, statusFilter, typeFilter, trafficFilter]);

  const selectedChartRows = useMemo(() => table.filter((r) => selectedChartIfs.includes(r.if_index)), [table, selectedChartIfs]);

  useEffect(() => {
    if (cachedTable.length > 0) setLiveTable(cachedTable);
  }, [cachedTable, selectedId]);

  useEffect(() => {
    const now = Date.now();
    setTrafficHistory((prev) => {
      const next = { ...prev };
      for (const r of table) {
        const tx = Number(r.out_bps ?? NaN);
        const rx = Number(r.in_bps ?? NaN);
        if (!Number.isFinite(tx) || !Number.isFinite(rx)) continue;
        const arr = [...(next[r.if_index] ?? [])];
        arr.push({ ts: now, tx, rx });
        next[r.if_index] = arr.slice(-50);
      }
      return next;
    });
  }, [table]);

  useEffect(() => {
    if (!telemetry.data?.metrics) return;
    const kpis = buildSwitchNocKpis(telemetry.data.metrics, selectedDevice?.description ?? "");
    const ts = telemetry.data.collected_at ? new Date(telemetry.data.collected_at).getTime() : Date.now();
    if (kpis.cpuPct != null) {
      setCpuHistory((h) => {
        if (h.some((p) => p.ts === ts)) return h;
        return [...h, { ts, v: kpis.cpuPct! }].slice(-40);
      });
    }
    if (kpis.memPct != null) {
      setMemHistory((h) => {
        if (h.some((p) => p.ts === ts)) return h;
        return [...h, { ts, v: kpis.memPct! }].slice(-40);
      });
    }
  }, [telemetry.data?.metrics, telemetry.data?.collected_at, selectedDevice?.description]);

  useEffect(() => {
    if (!realtimeOn || !selectedId) return;
    const intervalMs = Math.max(1500, Number(realtimeMs) || 3000);
    const timer = window.setInterval(() => {
      if (!realtimeTick.isPending) realtimeTick.mutate(selectedId);
    }, intervalMs);
    return () => window.clearInterval(timer);
  }, [realtimeOn, realtimeMs, selectedId, realtimeTick]);

  useEffect(() => {
    setRealtimeOn(false);
    setSelectedChartIfs([]);
  }, [selectedId]);

  useEffect(() => {
    if (!sel && rows.length > 0) setSel(rows[0].id);
  }, [rows, sel]);

  const initialDataLoading =
    !!selectedId && !iface.data && !telemetry.data && (iface.isLoading || telemetry.isLoading);

  const devicePicker = (
    <select
      className="input"
      style={{ maxWidth: 280, fontSize: 12 }}
      value={selectedId ?? ""}
      onChange={(e) => setSel(e.target.value || null)}
    >
      {rows.map((d) => (
        <option key={d.id} value={d.id}>
          {d.description ?? "—"}
        </option>
      ))}
    </select>
  );

  const telnetProfileSelect =
    canMutate && selectedId ? (
      <select
        className="input"
        style={{ maxWidth: 180, fontSize: 12 }}
        value={selectedDevice?.switch_telnet_profile_id ?? ""}
        disabled={patchTelnetProfile.isPending}
        onChange={(e) => patchTelnetProfile.mutate({ deviceId: selectedId, profileId: e.target.value === "" ? null : e.target.value })}
      >
        <option value="">Telnet: Padrão</option>
        {(telnetProfiles.data?.profiles ?? []).map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
    ) : null;

  const collectionWarning =
    collectionStatus && (collectionStatus.missingOid.length > 0 || collectionStatus.message) ? (
      <div className="msg msg--warn" style={{ fontSize: 12, marginBottom: 12 }}>
        {collectionStatus.message || "Algumas métricas activas não têm OID configurado."}
        {collectionStatus.missingOid.length > 0 ? (
          <span>
            {" "}
            Campos: <span className="mono">{collectionStatus.missingOid.join(", ")}</span>
          </span>
        ) : null}
      </div>
    ) : null;

  const interfacesPanel = (
    <div className="mk-noc-panel" style={{ background: "transparent", border: "none", padding: 0 }}>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
        {devicePicker}
        <input className="input" style={{ minWidth: 200, fontSize: 12 }} placeholder="Buscar interface…" value={search} onChange={(e) => setSearch(e.target.value)} />
        <select className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as "all" | "up" | "down")}>
          <option value="all">Todos status</option>
          <option value="up">UP</option>
          <option value="down">DOWN</option>
        </select>
        <select className="input" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as typeof typeFilter)}>
          <option value="all">Todos tipos</option>
          <option value="Ethernet">Ethernet</option>
          <option value="Port-Channel">Port-Channel</option>
          <option value="VLAN">VLAN</option>
          <option value="Mgmt">Mgmt</option>
          <option value="SFP">SFP</option>
          <option value="Outros">Outros</option>
        </select>
        {canMutate ? (
          <>
            <button type="button" className="mk-noc-btn" disabled={refreshIf.isPending || !selectedId} onClick={() => selectedId && refreshIf.mutate(selectedId)}>
              {refreshIf.isPending ? "A actualizar…" : "Actualizar interfaces"}
            </button>
            <button
              type="button"
              className="mk-noc-btn"
              onClick={() => {
                if (realtimeOn) {
                  setRealtimeOn(false);
                  return;
                }
                setRealtimeDraft(String(Math.max(1500, realtimeMs || 3000)));
                setRealtimeModalOpen(true);
              }}
              disabled={!selectedId}
            >
              {realtimeOn ? "Parar tempo real" : "Tempo real"}
            </button>
          </>
        ) : null}
        <DropdownMenu
          align="end"
          minWidth={210}
          trigger={({ toggle }) => (
            <button type="button" className="mk-noc-btn" onClick={toggle}>
              ⚙
            </button>
          )}
        >
          {({ close }) => (
            <Link to="/devices" className="mk-options-item" onClick={() => close()} style={{ color: "inherit" }}>
              Ir para equipamentos ↗
            </Link>
          )}
        </DropdownMenu>
      </div>
      {walkTruncated && (
        <div className="msg msg--warn" style={{ fontSize: 12, marginBottom: 8 }}>
          Coleta SNMP truncada — aumente o timeout em Configurações → Alertas.
        </div>
      )}
      {realtimeOn && <p style={{ fontSize: 11, color: "var(--ok)" }}>Tempo real activo ({realtimeMs} ms)</p>}
      <div className="table-wrap" style={{ maxHeight: "min(70vh, 640px)", overflow: "auto" }}>
        <table className="mk-noc-table">
          <thead>
            <tr>
              <th>Idx</th>
              <th>Nome</th>
              <th>Tipo</th>
              <th>Status</th>
              <th>VLAN(s)</th>
              <th>TX</th>
              <th>RX</th>
              <th>TX dBm</th>
              <th>RX dBm</th>
              <th>Gráfico</th>
            </tr>
          </thead>
          <tbody>
            {interfaceRowsFiltered.map((r) => (
              <tr key={r.if_index}>
                <td className="mono">{r.if_index}</td>
                <td>{ifDisplayName(r)}</td>
                <td>{inferIfaceType(r)}</td>
                <td>
                  <span className={`mk-noc-dot ${ifaceStatus(r) === "up" ? "mk-noc-dot--up" : "mk-noc-dot--down"}`} /> {ifaceStatus(r)}
                </td>
                <td className="mono" style={{ fontSize: 11, maxWidth: 220, whiteSpace: "normal" }}>
                  {r.vlan_label ?? (r.vlans?.length ? r.vlans.join(", ") : EM_DASH)}
                </td>
                <td className="mono">{formatBitrate(r.out_bps)}</td>
                <td className="mono">{formatBitrate(r.in_bps)}</td>
                <td className="mono">{formatDbm(r.tx_dbm)}</td>
                <td className="mono">{formatDbm(r.rx_dbm)}</td>
                <td>
                  <input
                    type="checkbox"
                    checked={selectedChartIfs.includes(r.if_index)}
                    onChange={(e) => {
                      if (e.target.checked) setSelectedChartIfs((p) => [...p, r.if_index]);
                      else setSelectedChartIfs((p) => p.filter((x) => x !== r.if_index));
                    }}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selectedChartRows.length > 0 && (
        <div ref={chartsRef} style={{ marginTop: 14, display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))", gap: 10 }}>
          {selectedChartRows.map((row) => (
            <div key={row.if_index} className="mk-noc-panel">
              <strong style={{ fontSize: 12 }}>{ifDisplayName(row)}</strong>
              <MiniTrafficChart points={trafficHistory[row.if_index] ?? []} />
            </div>
          ))}
        </div>
      )}
    </div>
  );

  return (
    <>
      <style>{`
        .mk-switch { position: relative; width: 36px; height: 20px; border-radius: 999px; border: 1px solid var(--border); background: var(--panel2); display: inline-block; }
        .mk-switch__knob { position: absolute; top: 1px; left: 1px; width: 16px; height: 16px; border-radius: 999px; background: var(--toggle-thumb-active); transition: transform 140ms; }
        .mk-switch--on { background: color-mix(in srgb, var(--ok) 45%, var(--panel2)); }
        .mk-switch--on .mk-switch__knob { transform: translateX(16px); }
        .mk-options-item { display: flex; padding: 8px 10px; text-decoration: none; border-radius: 6px; }
        .mk-options-item:hover { background: var(--hover-bg-menu); }
        @keyframes mk-spin { to { transform: rotate(360deg); } }
        .spin { animation: mk-spin 1s linear infinite; }
      `}</style>

      {devices.isLoading && <p>A carregar equipamentos…</p>}
      {devices.isError && <div className="msg msg--err">{(devices.error as Error).message}</div>}

      {!devices.isLoading && !devices.isError && (
        <>
          <div className="page-heading" style={{ marginBottom: 8 }}>
            <h1>Switch</h1>
            <PageCountPill label="Switches" count={rows.length} />
          </div>
          {rows.length === 0 ? (
            <p style={{ color: "var(--muted)" }}>Nenhum equipamento Switch cadastrado.</p>
          ) : selectedId && selectedDevice ? (
            initialDataLoading ? (
              <p style={{ color: "var(--muted)" }}>A carregar últimos dados coletados…</p>
            ) : (
            <MikrotikNocDashboard
              variant="switch"
              softwareLabel="NX-OS"
              monitorSubtitle="Monitor Switch"
              section={section}
              onSection={setSection}
              deviceLabel={selectedDevice.description ?? EM_DASH}
              deviceModel={selectedDevice.model}
              deviceIp={selectedDevice.ip}
              deviceOnline
              collectedAt={telemetry.data?.collected_at}
              formatCollectedAt={formatCollectedPt}
              metrics={telemetry.data?.metrics}
              ifaces={table}
              ifaceCollectedAt={iface.data?.collected_at}
              trafficHistory={trafficHistory}
              cpuHistory={cpuHistory}
              memHistory={memHistory}
              canMutate={canMutate}
              collecting={telCollecting}
              refreshingIf={refreshIf.isPending}
              onCollect={() => void runTelCollect()}
              onRefreshIf={() => selectedId && refreshIf.mutate(selectedId)}
              telnetProfileSelect={
                <>
                  {rows.length > 1 ? devicePicker : null}
                  {telnetProfileSelect}
                </>
              }
              collectionWarning={collectionWarning}
              interfacesPanel={interfacesPanel}
            />
            )
          ) : (
            <p style={{ color: "var(--muted)" }}>Nenhum equipamento Switch cadastrado.</p>
          )}
        </>
      )}

      {realtimeModalOpen && (
        <div style={{ position: "fixed", inset: 0, background: "var(--overlay)", display: "grid", placeItems: "center", zIndex: 40 }} onClick={() => setRealtimeModalOpen(false)}>
          <div className="card" style={{ width: "min(420px, 92vw)" }} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Intervalo tempo real (ms)</h3>
            <input className="input mono" type="number" min={1500} step={500} value={realtimeDraft} onChange={(e) => setRealtimeDraft(e.target.value)} />
            <div className="row" style={{ justifyContent: "flex-end", marginTop: 10, gap: 8 }}>
              <button type="button" className="btn" onClick={() => setRealtimeModalOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                onClick={() => {
                  setRealtimeMs(Math.max(1500, Number(realtimeDraft) || 3000));
                  setRealtimeOn(true);
                  setRealtimeModalOpen(false);
                }}
              >
                Iniciar
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
