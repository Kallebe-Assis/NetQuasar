import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Cable,
  Cpu,
  HeartPulse,
  KeyRound,
  LayoutDashboard,
  RefreshCw,
  Sliders,
  TriangleAlert,
  Users,
  Waypoints,
  Zap,
} from "lucide-react";
import { DeviceMonitorShell } from "../components/DeviceMonitorShell";
import { apiFetch } from "../lib/api";
import { can, isAdminUser } from "../lib/auth";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { APP_ROUTES } from "../app/routes";
import { formatDateTime, formatDuration } from "./bgp/bgpFormat";
import type { Report } from "./bgp/bgpTypes";
import { BgpPeersTab } from "./bgp/BgpPeersTab";
import { BgpInterfacesTab } from "./bgp/BgpInterfacesTab";
import { BgpOpticsTab } from "./bgp/BgpOpticsTab";
import { BgpCpuTab } from "./bgp/BgpCpuTab";
import { BgpChassisHealthTab } from "./bgp/BgpChassisHealthTab";
import { BgpQosTab } from "./bgp/BgpQosTab";
import { BgpRadiusTab } from "./bgp/BgpRadiusTab";
import { BgpLldpTab } from "./bgp/BgpLldpTab";
import { BgpTrafficPanel } from "./bgp/BgpTrafficPanel";

type BgpDevice = {
  id: string;
  description?: string;
  ip?: string;
  brand?: string;
  model?: string;
};

type ReportResponse = {
  device_id: string;
  collected_at?: string;
  note?: string;
  report?: Report;
};

type BgpTab = "overview" | "peers" | "interfaces" | "optics" | "cpu" | "chassis" | "qos" | "radius" | "lldp";

const BGP_TABS: Array<{ id: BgpTab; label: string; icon: typeof LayoutDashboard }> = [
  { id: "overview", label: "Visão Geral", icon: LayoutDashboard },
  { id: "peers", label: "Peers", icon: Users },
  { id: "interfaces", label: "Interfaces & LAG", icon: Cable },
  { id: "optics", label: "Óptica", icon: Zap },
  { id: "cpu", label: "CPU & Memória", icon: Cpu },
  { id: "chassis", label: "Saúde do Chassi", icon: HeartPulse },
  { id: "qos", label: "QoS", icon: Sliders },
  { id: "radius", label: "RADIUS", icon: KeyRound },
  { id: "lldp", label: "LLDP", icon: Waypoints },
];

function peerStateBadge(label?: string) {
  const established = label === "established";
  return (
    <span className={`badge ${established ? "badge--ok" : "badge--err"}`}>
      {label ?? "desconhecido"}
    </span>
  );
}

const HEALTH_LABELS: Record<string, string> = {
  cpu_usage: "CPU",
  memory_usage: "Memória",
  sys_uptime: "Uptime",
};

/** sys_uptime vem em TimeTicks (centésimos de segundo, padrão SNMP) — sem isto aparecia um
 * número cru tipo "267202500". cpu_usage/memory_usage ganham "%" quando o valor é numérico. */
function formatHealthValue(key: string, raw: string): string {
  if (key === "sys_uptime") {
    const n = Number(raw);
    return Number.isFinite(n) ? formatDuration(Math.floor(n / 100)) : raw;
  }
  if ((key === "cpu_usage" || key === "memory_usage") && Number.isFinite(Number(raw))) {
    return `${raw}%`;
  }
  return raw;
}

export function BgpPage() {
  const canMutate = isAdminUser() || can("bgp.collect");
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [sel, setSel] = useState<string | null>(null);
  const [tab, setTab] = useState<BgpTab>("overview");

  const devices = useQuery({
    queryKey: ["bgp-devices"],
    queryFn: () => apiFetch<{ devices: BgpDevice[] }>("/api/v1/bgp/devices"),
  });
  const rows = devices.data?.devices ?? [];
  const selectedId = sel ?? rows[0]?.id ?? null;
  const selectedDevice = rows.find((d) => d.id === selectedId) ?? null;

  const reportQ = useQuery({
    queryKey: ["bgp-report", selectedId],
    enabled: !!selectedId,
    placeholderData: keepPreviousData,
    queryFn: () => apiFetch<ReportResponse>(`/api/v1/bgp/devices/${selectedId}/report`),
    refetchInterval: 60_000,
  });

  // "Atualizar" fazia só um refetch das leituras (dados já em cache no banco, que não mudam
  // sozinhos sem o ciclo periódico de monitorização) — por isso "não funcionava" na prática.
  // Agora dispara uma coleta SNMP nova para o equipamento e só depois relê o relatório/histórico.
  const collectMut = useMutation({
    mutationFn: () => apiFetch(`/api/v1/bgp/devices/${selectedId}/collect`, { method: "POST", timeoutMs: 60_000 }),
    onSuccess: async () => {
      toastOk(pushToast, "Coleta BGP atualizada.");
      await qc.invalidateQueries({ queryKey: ["bgp-report", selectedId] });
      await qc.invalidateQueries({ queryKey: ["bgp-carrier-traffic-history", selectedId] });
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao coletar dados BGP."),
  });

  if (devices.isLoading) return <p>A carregar equipamentos BGP…</p>;
  if (devices.isError) return <div className="msg msg--err">{(devices.error as Error).message}</div>;

  const report = reportQ.data?.report;
  const peers = report?.peers ?? [];
  const interfaces = report?.interfaces ?? [];
  const health = report?.health ?? {};
  const established = peers.filter((p) => p.state_label === "established").length;
  const down = peers.filter((p) => p.state_label && p.state_label !== "established");

  const toolbar = (
    <>
      <select
        className="input mk-noc-btn"
        style={{ maxWidth: 260, fontSize: 12, padding: "6px 10px" }}
        value={selectedId ?? ""}
        onChange={(e) => setSel(e.target.value || null)}
      >
        {rows.map((d) => (
          <option key={d.id} value={d.id}>
            {d.description || d.ip} {d.ip ? `(${d.ip})` : ""}
          </option>
        ))}
      </select>
      {canMutate && selectedId && (
        <button
          type="button"
          className="mk-noc-btn mk-noc-btn--primary"
          onClick={() => collectMut.mutate()}
          disabled={collectMut.isPending}
          title="Faz uma coleta SNMP nova para este equipamento"
        >
          <RefreshCw size={14} className={collectMut.isPending ? "spin" : ""} />
          {collectMut.isPending ? "A coletar…" : "Atualizar"}
        </button>
      )}
    </>
  );

  return (
    <>
      <div className="page-heading" style={{ marginBottom: 8 }}>
        <h1>BGP</h1>
      </div>

      {rows.length === 0 ? (
        <div className="msg msg--warn">
          Nenhum equipamento com BGP activo. Active o switch em{" "}
          <Link to={APP_ROUTES.devices}>Equipamentos</Link>.
        </div>
      ) : (
        <DeviceMonitorShell
          tabs={BGP_TABS}
          activeTab={tab}
          onTab={setTab}
          title={selectedDevice?.description || selectedDevice?.ip || "BGP"}
          subtitle="Peers, interfaces, óptica, saúde do chassi e RADIUS"
          online={established > 0}
          meta={
            <>
              <span>
                <strong>IP</strong> <span className="mono">{selectedDevice?.ip || "—"}</span>
              </span>
              <span>
                <strong>Peers</strong> {established}/{peers.length} estabelecidos
              </span>
              <span>
                <strong>Últ. coleta</strong> {formatDateTime(reportQ.data?.collected_at)}
              </span>
            </>
          }
          toolbar={toolbar}
        >
          {reportQ.isLoading && !report ? (
            <p style={{ color: "var(--muted)", padding: 16 }}>A carregar dados BGP…</p>
          ) : !report || (peers.length === 0 && interfaces.length === 0) ? (
            <div className="msg msg--warn">
              {reportQ.data?.note ||
                "Sem coleta BGP persistida ainda. Verifique o perfil SNMP em Configurações → BGP e o pipeline de monitorização."}
            </div>
          ) : (
            <div className="mk-noc-panel" style={{ padding: 14 }}>
              {tab === "overview" && (
                <>
                  <div className="row" style={{ gap: 12, marginBottom: 16, flexWrap: "wrap" }}>
                    <div className="card" style={{ padding: "10px 14px", minWidth: 150 }}>
                      <div style={{ fontSize: 11, color: "var(--muted)" }}>Peers estabelecidos</div>
                      <strong style={{ fontSize: 18 }}>
                        {established}/{peers.length}
                      </strong>
                    </div>
                    {Object.entries(health).map(([key, value]) => (
                      <div key={key} className="card" style={{ padding: "10px 14px", minWidth: 130 }}>
                        <div style={{ fontSize: 11, color: "var(--muted)" }}>{HEALTH_LABELS[key] ?? key}</div>
                        <strong style={{ fontSize: 16 }}>{formatHealthValue(key, value)}</strong>
                      </div>
                    ))}
                  </div>

                  {down.length > 0 && (
                    <div className="msg msg--err" style={{ marginBottom: 16, display: "flex", gap: 8, alignItems: "flex-start" }}>
                      <TriangleAlert size={16} style={{ flexShrink: 0, marginTop: 2 }} />
                      <div>
                        <strong>Alertas de sessão</strong>
                        <ul style={{ margin: "4px 0 0", paddingLeft: 18 }}>
                          {down.map((p) => (
                            <li key={p.peer_ip}>
                              Peer <span className="mono">{p.peer_ip}</span>
                              {p.remote_as ? ` (AS${p.remote_as})` : ""} — estado: {peerStateBadge(p.state_label)}
                            </li>
                          ))}
                        </ul>
                      </div>
                    </div>
                  )}

                  <BgpTrafficPanel deviceId={selectedId} />

                  <div className="msg" style={{ fontSize: 12 }}>
                    Peers, interfaces, óptica, CPU/VS, saúde do chassi, QoS, RADIUS e LLDP têm as suas próprias abas acima,
                    com mais detalhe.
                  </div>
                </>
              )}

              {tab === "peers" && <BgpPeersTab peers={peers} bfdSessions={report.bfd_sessions ?? []} />}

              {tab === "interfaces" && (
                <BgpInterfacesTab
                  interfaces={interfaces}
                  etrunks={report.etrunks ?? []}
                  etrunkMembers={report.etrunk_members ?? []}
                />
              )}

              {tab === "optics" && <BgpOpticsTab optics={report.optics ?? []} />}

              {tab === "cpu" && (
                <BgpCpuTab
                  cpuCores={report.cpu_cores ?? []}
                  vsList={report.vs_list ?? []}
                  vsResources={report.vs_resources ?? []}
                />
              )}

              {tab === "chassis" && (
                <BgpChassisHealthTab
                  boardAlarms={report.board_alarms ?? []}
                  fans={report.fans ?? []}
                  powerSupplies={report.power_supplies ?? []}
                  temperatures={report.temperatures ?? []}
                  voltages={report.voltages ?? []}
                />
              )}

              {tab === "qos" && <BgpQosTab qosQueues={report.qos_queues ?? []} carStats={report.car_stats ?? []} />}

              {tab === "radius" && <BgpRadiusTab radiusServers={report.radius_servers ?? []} />}

              {tab === "lldp" && <BgpLldpTab lldpNeighbors={report.lldp_neighbors ?? []} />}
            </div>
          )}
        </DeviceMonitorShell>
      )}
    </>
  );
}
