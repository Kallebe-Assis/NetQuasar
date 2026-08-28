// bgpTypes.ts — tipos partilhados entre BgpPage.tsx e as abas em pages/bgp/*.tsx. Espelham
// exactamente os campos JSON devolvidos por Report em quasar_backend/internal/bgpcollect/report.go
// (e ficheiros report_hardware.go/report_ext.go/report_radius.go/report_qos.go/report_lldp.go).

export type PeerReport = {
  peer_ip: string;
  remote_as?: string;
  state?: string;
  state_label?: string;
  established_seconds?: number;
  in_updates?: string;
  out_updates?: string;
  prefixes_received?: number;
  prefixes_active?: number;
  prefixes_advertised?: number;
};

export type InterfaceReport = {
  if_index: string;
  descr?: string;
  alias?: string;
  oper_status?: string;
  hc_in_octets?: string;
  hc_out_octets?: string;
  in_bit_rate?: string;
  out_bit_rate?: string;
};

export type OpticsReport = {
  physical_index: string;
  port_label?: string;
  rx_power?: string;
  tx_power?: string;
  temperature?: string;
  voltage?: string;
  bias_current?: string;
};

export type CpuCoreReport = {
  core_index: string;
  duty?: string;
  avg_duty_1min?: string;
  avg_duty_5min?: string;
};

export type FanReport = {
  slot: string;
  sn: string;
  speed?: string;
  present?: string;
  state?: string;
  state_label?: string;
};

export type PowerSupplyReport = {
  slot: string;
  sn: string;
  present?: string;
  state?: string;
  state_label?: string;
  current?: string;
  voltage?: string;
};

export type TemperatureReport = {
  slot_raw: string;
  chassis?: number;
  slot?: number;
  i2c: string;
  value?: string;
  status?: string;
  status_label?: string;
};

export type VoltageReport = {
  slot_raw: string;
  chassis?: number;
  slot?: number;
  i2c: string;
  value?: string;
  status?: string;
  status_label?: string;
};

export type BoardAlarmReport = {
  physical_index: string;
  name?: string;
  raw?: string;
  severity: string;
};

export type VSInfo = {
  vs_id: string;
  name?: string;
  status?: string;
};

export type VSResourceReport = {
  slot: string;
  cpu?: string;
  mem_used?: string;
  mem_total?: string;
};

export type BFDSessionReport = {
  sess_index: string;
  peer_addr?: string;
  bind_if_name?: string;
  state?: string;
  state_label?: string;
  diag?: string;
  vpn_name?: string;
  down_reason?: string;
};

export type ETrunkReport = {
  etrunk_id: string;
  status?: string;
  status_label?: string;
  status_reason?: string;
};

export type ETrunkMemberReport = {
  parent_id: string;
  member_id: string;
  status?: string;
  status_label?: string;
  status_reason?: string;
};

export type QosQueueReport = {
  if_index: string;
  if_name?: string;
  queue_key: string;
  forward_bytes?: string;
  forward_packets?: string;
  drop_bytes?: string;
  drop_packets?: string;
};

export type CarStatReport = {
  if_index: string;
  if_name?: string;
  class_key: string;
  conformed_bytes?: string;
  exceeded_bytes?: string;
  dropped_bytes?: string;
};

export type RadiusServerReport = {
  server_ip: string;
  authen_requests?: string;
  authen_accepts?: string;
  authen_rejects?: string;
  authen_timeouts?: string;
  authen_server_not_reply?: string;
  acct_requests?: string;
  acct_responses?: string;
  acct_timeouts?: string;
  acct_server_not_reply?: string;
};

export type LLDPNeighborReport = {
  local_port_num: string;
  local_if_name?: string;
  rem_key: string;
  chassis_id?: string;
  port_id?: string;
  port_desc?: string;
  sys_name?: string;
  sys_desc?: string;
};

export type Report = {
  collected_at?: string;
  peers: PeerReport[];
  interfaces: InterfaceReport[];
  health?: Record<string, string>;
  optics?: OpticsReport[];
  cpu_cores?: CpuCoreReport[];
  fans?: FanReport[];
  power_supplies?: PowerSupplyReport[];
  temperatures?: TemperatureReport[];
  voltages?: VoltageReport[];
  board_alarms?: BoardAlarmReport[];
  vs_list?: VSInfo[];
  vs_resources?: VSResourceReport[];
  bfd_sessions?: BFDSessionReport[];
  etrunks?: ETrunkReport[];
  etrunk_members?: ETrunkMemberReport[];
  qos_queues?: QosQueueReport[];
  car_stats?: CarStatReport[];
  radius_servers?: RadiusServerReport[];
  lldp_neighbors?: LLDPNeighborReport[];
};
