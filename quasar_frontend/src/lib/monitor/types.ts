/** Tipos unificados de monitoramento — espelham monitorview no backend. */

export type MonitorHealthStatus = "unknown" | "ok" | "partial" | "failed";

export type MonitorReachability = {
  online: boolean;
  latency_ms?: number | null;
  method?: string | null;
  ping_fail_streak?: number;
  checked_at?: string | null;
};

export type MonitorDeviceKPIs = {
  cpu_percent?: number | null;
  memory_percent?: number | null;
  temperature_c?: number | null;
  uptime?: string | null;
  collected_at?: string | null;
};

export type MonitorInterfaceRow = {
  if_index: number;
  name?: string;
  if_name?: string;
  descr?: string;
  if_alias?: string;
  if_type?: number;
  display_name?: string;
  type?: string;
  custom_description?: string;
  custom_type?: string;
  metadata_if_name?: string;
  admin_status?: string;
  oper_status?: string;
  in_bps?: number;
  out_bps?: number;
  rx_dbm?: number | null;
  tx_dbm?: number | null;
  speed_bps?: number | null;
};

export type MonitorTrafficPoint = {
  ts: number;
  rx_bps: number;
  tx_bps: number;
};

export type MonitorGaugePoint = {
  ts: number;
  value: number;
};

export type ActiveEquipmentRow = {
  id: string;
  description?: string;
  category?: string;
  brand?: string;
  ip?: string;
  checked_at?: string | null;
  latency_ms?: number | null;
  probe_ok?: boolean | null;
  ping_reachable?: boolean | null;
  ping_fail_streak?: number;
  cpu_percent?: number | null;
  memory_percent?: number | null;
  uptime?: string | null;
  temperature_c?: number | null;
  telemetry_collected_at?: string | null;
};

export type InterfaceSnapshotResponse = {
  device_id?: string;
  collected_at?: string;
  interface_table?: MonitorInterfaceRow[];
  interface_count?: number;
  walk_truncated?: boolean;
  walk_note?: string;
  note?: string;
};
