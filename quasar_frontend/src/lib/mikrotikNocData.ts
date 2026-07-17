import { EM_DASH, formatDbm, formatSnmpDisplayText } from "./formatDisplay";
import { formatBytes, formatKilobytes, formatUptimeTicks } from "./formatBytes";
import { stripPPPoEName } from "./mikrotikMetricsDisplay";

type FieldRaw = {
  ok?: boolean;
  value?: unknown;
  error?: string;
  pppoe_sessions?: Array<Record<string, unknown>>;
  optical_ports?: Array<Record<string, unknown>>;
};

export type MikrotikIfRow = {
  if_index: number;
  descr?: string;
  if_name?: string;
  if_alias?: string;
  if_type?: number;
  display_name?: string;
  custom_description?: string;
  custom_type?: "ether" | "sfp" | string;
  metadata_if_name?: string;
  sfp?: boolean;
  tx_dbm?: number;
  rx_dbm?: number;
  speed_bps?: number;
  admin_status?: string;
  oper_status?: string;
  in_octets?: number;
  out_octets?: number;
  in_bps?: number;
  out_bps?: number;
  in_packets?: number;
  out_packets?: number;
  rx_errors?: number;
  tx_errors?: number;
  mtu?: number;
  vlan_mode?: string;
  vlan_label?: string;
  vlans?: number[];
};

export type MikrotikNocKpis = {
  identity: string;
  uptime: string;
  cpuPct: number | null;
  cpuFreq: string | null;
  memPct: number | null;
  memFree: string;
  memTotal: string;
  diskPct: number | null;
  diskFree: string;
  diskTotal: string;
  tempC: number | null;
  tempCpu: string | null;
  tempBoard: string | null;
  voltageV: number | null;
  currentA: string | null;
  powerW: string | null;
  pppoeCount: number;
};

export type MikrotikSfpPanel = {
  name: string;
  online: boolean;
  temperatureC: string;
  voltageV: string;
  txBiasMa: string;
  txDbm: string;
  rxDbm: string;
  vendor: string;
  model: string;
  serial: string;
};

export type MikrotikSystemInfo = {
  model: string;
  arch: string;
  cpu: string;
  cpuFreq: string;
  memTotal: string;
  memFree: string;
  version: string;
  build: string;
  board: string;
  platform: string;
  uptime: string;
  datetime: string;
};

export type MikrotikPppoeRow = {
  name: string;
  uptime: string;
  ifIndex: number;
};

const TELNET_RESOURCE_KEYS: Array<{ key: string; field: string }> = [
  { key: "telnet_sys_uptime", field: "uptime" },
  { key: "telnet_sys_cpu_load", field: "cpu-load" },
  { key: "telnet_sys_cpu_frequency", field: "cpu-frequency" },
  { key: "telnet_sys_free_memory", field: "free-memory" },
  { key: "telnet_sys_total_memory", field: "total-memory" },
  { key: "telnet_sys_free_hdd", field: "free-hdd-space" },
  { key: "telnet_sys_total_hdd", field: "total-hdd-space" },
  { key: "telnet_sys_version", field: "version" },
  { key: "telnet_sys_board", field: "board-name" },
  { key: "telnet_sys_platform", field: "platform" },
  { key: "telnet_sys_architecture", field: "architecture-name" },
];

function num(v: unknown): number | null {
  if (v == null) return null;
  const n = typeof v === "number" ? v : Number(String(v).replace(/[^\d.-]/g, ""));
  return Number.isFinite(n) ? n : null;
}

function str(v: unknown): string {
  if (v == null) return "";
  return String(v).trim();
}

function snmpField(metrics: Record<string, unknown> | undefined, key: string): FieldRaw | undefined {
  const coll = metrics?.mikrotik_collection as { fields?: Record<string, FieldRaw> } | undefined;
  return coll?.fields?.[key];
}

function telnetField(metrics: Record<string, unknown> | undefined, key: string): FieldRaw | undefined {
  const coll = metrics?.mikrotik_telnet_collection as { fields?: Record<string, FieldRaw> } | undefined;
  return coll?.fields?.[key];
}

function extractFromTelnetValue(value: unknown, field: string): unknown {
  if (value == null) return undefined;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return field === "" ? value : undefined;
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return undefined;
    const first = value[0];
    if (typeof first === "object" && first != null) {
      const row = first as Record<string, unknown>;
      const data = (row.data ?? row) as Record<string, unknown>;
      return extractFromTelnetValue(data, field);
    }
    return undefined;
  }
  if (typeof value !== "object") return undefined;
  const v = value as Record<string, unknown>;
  if (field in v && v[field] != null && v[field] !== "") return v[field];
  const alt = field.replace(/-/g, "_");
  if (alt in v && v[alt] != null && v[alt] !== "") return v[alt];
  if (field === "cpu-load" && "cpu" in v && v.cpu != null && v.cpu !== "") return v.cpu;
  const keys = Object.keys(v).filter((k) => k !== "raw" && v[k] != null && v[k] !== "");
  if (keys.length === 1 && (field === "" || keys[0] === field || keys[0] === alt || keys[0].replace(/_/g, "-") === field)) {
    return v[keys[0]];
  }
  return undefined;
}

function mergedTelnetScalar(metrics: Record<string, unknown> | undefined, field: string): unknown {
  const coll = metrics?.mikrotik_telnet_collection as { fields?: Record<string, FieldRaw> } | undefined;
  for (const fr of Object.values(coll?.fields ?? {})) {
    const v = extractFromTelnetValue(fr?.value, field);
    if (v != null && String(v).trim() !== "") return v;
  }
  return undefined;
}

function telnetScalar(metrics: Record<string, unknown> | undefined, key: string, field: string): unknown {
  const fr = telnetField(metrics, key);
  if (fr?.value != null) {
    const extracted = extractFromTelnetValue(fr.value, field);
    if (extracted != null && String(extracted).trim() !== "") return extracted;
  }
  if (fr?.ok === false) return mergedTelnetScalar(metrics, field);
  return mergedTelnetScalar(metrics, field);
}

function snmpScalar(metrics: Record<string, unknown> | undefined, key: string): unknown {
  const fr = snmpField(metrics, key);
  if (!fr?.ok || fr.value == null) return undefined;
  return fr.value;
}

function firstNum(...vals: unknown[]): number | null {
  for (const v of vals) {
    const n = num(v);
    if (n != null) return n;
  }
  return null;
}

function normalizeCpuPct(v: number | null): number | null {
  if (v == null) return null;
  if (v > 100) return v / 10;
  return v;
}

function mergeTelnetResource(metrics: Record<string, unknown> | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const { key, field } of TELNET_RESOURCE_KEYS) {
    const v = telnetScalar(metrics, key, field);
    if (v != null && str(v) !== "") out[field] = v;
  }
  const cpu = telnetScalar(metrics, "telnet_sys_cpu_load", "cpu");
  if (cpu != null && !("cpu-load" in out) && !("cpu" in out)) out.cpu = cpu;
  return out;
}

function telnetPerInterfaceRows(metrics: Record<string, unknown> | undefined): Map<string, Record<string, unknown>> {
  const map = new Map<string, Record<string, unknown>>();
  const mergeKeys: Array<{ metricKey: string; field: string }> = [
    { metricKey: "telnet_sfp_rx_power", field: "sfp-rx-power" },
    { metricKey: "telnet_sfp_tx_power", field: "sfp-tx-power" },
    { metricKey: "telnet_sfp_temperature", field: "sfp-temperature" },
    { metricKey: "telnet_sfp_voltage", field: "sfp-supply-voltage" },
    { metricKey: "telnet_sfp_bias_current", field: "sfp-tx-bias-current" },
    { metricKey: "telnet_sfp_vendor", field: "sfp-vendor-name" },
  ];
  const put = (iface: string, patch: Record<string, unknown>) => {
    for (const alias of nxosIfAliases(iface)) {
      const existing = map.get(alias) ?? { interface: alias };
      Object.assign(existing, patch, { interface: alias });
      map.set(alias, existing);
    }
  };
  for (const { metricKey, field } of mergeKeys) {
    const fr = telnetField(metrics, metricKey);
    if (!fr?.ok || !Array.isArray(fr.value)) continue;
    for (const item of fr.value) {
      const row = item as Record<string, unknown>;
      const iface = str(row.interface);
      if (!iface) continue;
      const patch: Record<string, unknown> = {};
      const val = row[field];
      if (val != null) patch[field] = val;
      if (metricKey === "telnet_sfp_vendor") {
        if (row["sfp-vendor-part-number"] != null) patch["sfp-vendor-part-number"] = row["sfp-vendor-part-number"];
        if (row["sfp-serial"] != null) patch["sfp-serial"] = row["sfp-serial"];
      }
      put(iface, patch);
    }
  }
  return map;
}

/** Aliases Eth1/23 ↔ Ethernet1/23 e Po1 ↔ port-channel1 para cruzar SNMP/CLI. */
function nxosIfAliases(name: string): string[] {
  const n = name.trim();
  if (!n) return [];
  const out = new Set<string>([n]);
  const low = n.toLowerCase();
  if (low.startsWith("ethernet")) {
    out.add("Eth" + n.slice("Ethernet".length));
    out.add("Ethernet" + n.slice("Ethernet".length));
  } else if (low.startsWith("eth") && !low.startsWith("ether")) {
    out.add("Ethernet" + n.slice(3));
    out.add(n);
  } else if (low.startsWith("port-channel")) {
    out.add("Po" + n.slice("port-channel".length));
  } else if (/^po\d+/i.test(n)) {
    out.add("port-channel" + n.slice(2));
  }
  return [...out];
}

function formatDbmFromRaw(v: unknown): string {
  if (v == null) return EM_DASH;
  const n = num(String(v).replace(/dbm/i, ""));
  if (n != null) return formatDbm(n);
  const s = str(v);
  return s || EM_DASH;
}

function formatTempFromRaw(v: unknown): string {
  if (v == null) return EM_DASH;
  const n = num(String(v).replace(/[^\d.-]/g, ""));
  if (n != null) return `${n.toFixed(1)} °C`;
  return str(v) || EM_DASH;
}

function formatVoltageFromRaw(v: unknown): string {
  if (v == null) return EM_DASH;
  const n = num(String(v).replace(/[^\d.-]/g, ""));
  if (n != null) return `${n.toFixed(2)} V`;
  return str(v) || EM_DASH;
}

function formatBiasFromRaw(v: unknown): string {
  if (v == null) return EM_DASH;
  const n = num(String(v).replace(/[^\d.-]/g, ""));
  if (n != null) return `${n.toFixed(1)} mA`;
  return str(v) || EM_DASH;
}

/** Formata uptime RouterOS (15d8h32m47s) ou timeticks SNMP. */
export function formatMikrotikUptime(v: unknown): string {
  if (v == null) return EM_DASH;
  const s = String(v).trim();
  if (!s) return EM_DASH;
  if (/^\d+$/.test(s)) return formatUptimeTicks(s);
  return s
    .replace(/(\d+)w/g, "$1sem ")
    .replace(/(\d+)d/g, "$1d ")
    .replace(/(\d+)h/g, "$1h ")
    .replace(/(\d+)m/g, "$1m ")
    .replace(/(\d+)s/g, "$1s")
    .trim();
}

function pct(used: number | null, total: number | null): number | null {
  if (used == null || total == null || total <= 0) return null;
  return Math.min(100, Math.max(0, (used / total) * 100));
}

export function buildMikrotikNocKpis(metrics: Record<string, unknown> | undefined, deviceName: string): MikrotikNocKpis {
  const identity =
    str(telnetScalar(metrics, "telnet_sys_identity", "name")) ||
    str(snmpScalar(metrics, "sys_name")) ||
    deviceName ||
    EM_DASH;

  const uptimeRaw =
    telnetScalar(metrics, "telnet_sys_uptime", "uptime") ??
    mergedTelnetScalar(metrics, "uptime") ??
    snmpScalar(metrics, "sys_uptime");
  const uptime = formatMikrotikUptime(uptimeRaw);

  const cpuPct = normalizeCpuPct(
    firstNum(
      snmpScalar(metrics, "cpu_load"),
      snmpScalar(metrics, "cpu_hr"),
      telnetScalar(metrics, "telnet_sys_cpu_load", "cpu-load"),
      telnetScalar(metrics, "telnet_sys_cpu_load", "cpu"),
    ),
  );

  const cpuFreqRaw = telnetScalar(metrics, "telnet_sys_cpu_frequency", "cpu-frequency");
  const cpuFreq = cpuFreqRaw != null ? `${cpuFreqRaw} MHz` : null;

  const memTotalKb = firstNum(snmpScalar(metrics, "memory_total"));
  const memUsedKb = firstNum(snmpScalar(metrics, "memory_used"));
  const memFreeKbSnmp = firstNum(snmpScalar(metrics, "memory_free"));
  const memFreeBytes = num(telnetScalar(metrics, "telnet_sys_free_memory", "free-memory"));
  const memTotalBytes = num(telnetScalar(metrics, "telnet_sys_total_memory", "total-memory"));

  let memPct: number | null = null;
  let memFree = EM_DASH;
  let memTotal = EM_DASH;
  if (memFreeBytes != null && memTotalBytes != null && memTotalBytes > 0) {
    memPct = pct(memTotalBytes - memFreeBytes, memTotalBytes);
    memFree = formatBytes(memFreeBytes);
    memTotal = formatBytes(memTotalBytes);
  } else if (memUsedKb != null && memFreeKbSnmp != null && memUsedKb + memFreeKbSnmp > 0) {
    const totalKb = memUsedKb + memFreeKbSnmp;
    memPct = pct(memUsedKb, totalKb);
    memFree = formatKilobytes(memFreeKbSnmp);
    memTotal = formatKilobytes(totalKb);
  } else if (memTotalKb != null && memUsedKb != null && memTotalKb > 0) {
    memPct = pct(memUsedKb, memTotalKb);
    memFree = formatKilobytes(memTotalKb - memUsedKb);
    memTotal = formatKilobytes(memTotalKb);
  }

  const diskFreeBytes = num(
    telnetScalar(metrics, "telnet_sys_free_hdd", "free-hdd-space") ??
      mergedTelnetScalar(metrics, "free-hdd-space"),
  );
  const diskTotalBytes = num(
    telnetScalar(metrics, "telnet_sys_total_hdd", "total-hdd-space") ??
      mergedTelnetScalar(metrics, "total-hdd-space"),
  );
  let diskPct: number | null = null;
  let diskFree = EM_DASH;
  let diskTotal = EM_DASH;
  if (diskFreeBytes != null && diskTotalBytes != null && diskTotalBytes > 0) {
    diskPct = pct(diskTotalBytes - diskFreeBytes, diskTotalBytes);
    diskFree = formatBytes(diskFreeBytes);
    diskTotal = formatBytes(diskTotalBytes);
  } else if (diskFreeBytes != null) {
    diskFree = formatBytes(diskFreeBytes);
  }

  const tempTelnet = telnetScalar(metrics, "telnet_sys_temperature", "temperature");
  const tempC = firstNum(
    snmpScalar(metrics, "temperature"),
    snmpScalar(metrics, "board_temperature"),
    snmpScalar(metrics, "cpu_temperature"),
    num(String(tempTelnet ?? "").replace(/[^\d.-]/g, "")),
  );

  const tempCpu = str(snmpScalar(metrics, "cpu_temperature")) || null;
  const tempBoard = str(snmpScalar(metrics, "board_temperature")) || (tempTelnet ? str(tempTelnet) : null);

  const voltageRaw =
    telnetScalar(metrics, "telnet_sys_voltage", "voltage") ?? mergedTelnetScalar(metrics, "voltage");
  const voltageV = firstNum(
    num(String(voltageRaw ?? "").replace(/[^\d.-]/g, "")),
    snmpScalar(metrics, "voltage"),
  );

  const pppoeField = snmpField(metrics, "pppoe_active_sessions");
  const pppoeSessions = pppoeField?.pppoe_sessions ?? [];
  const pppoeCount = pppoeSessions.length || num(snmpScalar(metrics, "pppoe_active_sessions")) || 0;

  return {
    identity,
    uptime,
    cpuPct,
    cpuFreq,
    memPct,
    memFree,
    memTotal,
    diskPct,
    diskFree,
    diskTotal,
    tempC: tempC != null && tempC > 150 ? tempC / 10 : tempC,
    tempCpu: tempCpu ? `${tempCpu}°C` : null,
    tempBoard: tempBoard ? `${tempBoard}` : null,
    voltageV,
    currentA: null,
    powerW: null,
    pppoeCount: Math.round(pppoeCount),
  };
}

export function buildMikrotikPppoeTop(metrics: Record<string, unknown> | undefined, limit = 5): MikrotikPppoeRow[] {
  const fr = snmpField(metrics, "pppoe_active_sessions");
  const sessions = fr?.pppoe_sessions ?? [];
  return sessions.slice(0, limit).map((s) => ({
    name: stripPPPoEName(String(s.name ?? s.if_descr ?? "—")),
    uptime: EM_DASH,
    ifIndex: Number(s.if_index ?? 0),
  }));
}

export function buildMikrotikSfpPanels(
  metrics: Record<string, unknown> | undefined,
  ifaces: MikrotikIfRow[],
): MikrotikSfpPanel[] {
  const telnetRows = telnetPerInterfaceRows(metrics);
  const ifaceByName = new Map<string, MikrotikIfRow>();
  for (const r of ifaces) {
    const names = [r.if_name, r.display_name, r.descr].map((n) => str(n).toLowerCase()).filter(Boolean);
    for (const n of names) {
      ifaceByName.set(n, r);
      for (const alias of nxosIfAliases(n)) {
        ifaceByName.set(alias.toLowerCase(), r);
      }
    }
  }

  const preferDbm = (telnet: unknown, snmp?: number): string => {
    const fromTelnet = formatDbmFromRaw(telnet);
    if (fromTelnet !== EM_DASH) return fromTelnet;
    return formatDbm(snmp);
  };

  const panelFromRow = (ifaceName: string, row: Record<string, unknown>, iface?: MikrotikIfRow): MikrotikSfpPanel => {
    const online = iface ? String(iface.oper_status ?? "").toLowerCase() === "up" : true;
    return {
      name: ifaceName,
      online,
      temperatureC: formatTempFromRaw(row["sfp-temperature"]),
      voltageV: formatVoltageFromRaw(row["sfp-supply-voltage"]),
      txBiasMa: formatBiasFromRaw(row["sfp-tx-bias-current"]),
      txDbm: preferDbm(row["sfp-tx-power"], iface?.tx_dbm),
      rxDbm: preferDbm(row["sfp-rx-power"], iface?.rx_dbm),
      vendor: str(row["sfp-vendor-name"]) || EM_DASH,
      model: str(row["sfp-vendor-part-number"]) || EM_DASH,
      serial: str(row["sfp-serial"]) || EM_DASH,
    };
  };

  // Preferir interfaces SFP com TX/RX SNMP do snapshot; enriquecer com telnet quando existir.
  const sfpIfaces = ifaces.filter((r) => r.sfp || /sfp/i.test(String(r.if_name ?? r.display_name ?? r.descr ?? "")));
  const withSnmpPower = sfpIfaces.filter((r) => r.tx_dbm != null || r.rx_dbm != null);
  if (withSnmpPower.length > 0) {
    return withSnmpPower.slice(0, 8).map((r) => {
      const name = String(r.display_name ?? r.if_name ?? r.descr ?? `if${r.if_index}`);
      const telnet =
        telnetRows.get(name) ??
        telnetRows.get(String(r.if_name ?? "")) ??
        nxosIfAliases(name).map((a) => telnetRows.get(a)).find(Boolean) ??
        {};
      return panelFromRow(name, telnet, r);
    });
  }

  if (telnetRows.size > 0) {
    const seen = new Set<string>();
    const panels: MikrotikSfpPanel[] = [];
    for (const [ifaceName, row] of telnetRows.entries()) {
      const canon = nxosIfAliases(ifaceName)[0] ?? ifaceName;
      if (seen.has(canon.toLowerCase())) continue;
      seen.add(canon.toLowerCase());
      const iface =
        ifaceByName.get(ifaceName.toLowerCase()) ??
        nxosIfAliases(ifaceName).map((a) => ifaceByName.get(a.toLowerCase())).find(Boolean);
      panels.push(panelFromRow(canon, row, iface));
    }
    return panels;
  }

  const optical = snmpField(metrics, "optical_table")?.optical_ports ?? [];
  if (optical.length > 0) {
    return optical.map((p) => ({
      name: String(p.name ?? p.if_name ?? "SFP"),
      online: true,
      temperatureC: p.temperature_c != null ? `${Number(p.temperature_c).toFixed(1)} °C` : EM_DASH,
      voltageV: p.supply_voltage_v != null ? `${Number(p.supply_voltage_v).toFixed(2)} V` : EM_DASH,
      txBiasMa: p.bias_current_ma != null ? `${Number(p.bias_current_ma).toFixed(1)} mA` : EM_DASH,
      txDbm: formatDbm(p.tx_dbm as number | undefined),
      rxDbm: formatDbm(p.rx_dbm as number | undefined),
      vendor: String(p.vendor ?? EM_DASH),
      model: String(p.part_number ?? p.model ?? EM_DASH),
      serial: String(p.serial ?? EM_DASH),
    }));
  }

  return sfpIfaces.slice(0, 4).map((r) => ({
    name: String(r.display_name ?? r.if_name ?? r.descr ?? `if${r.if_index}`),
    online: String(r.oper_status ?? "").toLowerCase() === "up",
    temperatureC: EM_DASH,
    voltageV: EM_DASH,
    txBiasMa: EM_DASH,
    txDbm: formatDbm(r.tx_dbm),
    rxDbm: formatDbm(r.rx_dbm),
    vendor: EM_DASH,
    model: EM_DASH,
    serial: EM_DASH,
  }));
}

export function buildMikrotikSystemInfo(metrics: Record<string, unknown> | undefined, kpis: MikrotikNocKpis): MikrotikSystemInfo {
  const res = mergeTelnetResource(metrics);
  const sysDescr = str(snmpScalar(metrics, "sys_descr"));
  const firmware = str(snmpScalar(metrics, "firmware_version"));
  const boardRaw = str(res["board-name"]) || sysDescr.split("\n")[0] || EM_DASH;
  const board = boardRaw === EM_DASH ? EM_DASH : formatSnmpDisplayText(boardRaw);
  return {
    model: board,
    arch: str(res["architecture-name"]) || EM_DASH,
    cpu: str(res.cpu) || str(res["cpu-count"]) || EM_DASH,
    cpuFreq: kpis.cpuFreq ?? (str(res["cpu-frequency"]) ? `${res["cpu-frequency"]} MHz` : EM_DASH),
    memTotal: kpis.memTotal,
    memFree: kpis.memFree,
    version: str(res.version) || firmware || EM_DASH,
    build: EM_DASH,
    board,
    platform: str(res.platform) || EM_DASH,
    uptime: kpis.uptime,
    datetime: new Date().toLocaleString("pt-BR"),
  };
}

export function pickPrimaryIface(ifaces: MikrotikIfRow[]): MikrotikIfRow | null {
  const sorted = [...ifaces].sort((a, b) => {
    const ta = Number(a.out_bps ?? 0) + Number(a.in_bps ?? 0);
    const tb = Number(b.out_bps ?? 0) + Number(b.in_bps ?? 0);
    return tb - ta;
  });
  const ether = sorted.find((r) => /ether/i.test(String(r.if_name ?? r.display_name ?? "")));
  return ether ?? sorted[0] ?? null;
}

export function ifDisplayName(r: MikrotikIfRow): string {
  return String(r.display_name ?? r.if_name ?? r.descr ?? "").trim() || EM_DASH;
}

export function ifOperUp(r: MikrotikIfRow): boolean {
  return String(r.oper_status ?? "").toLowerCase() === "up";
}

export function inferIfType(r: MikrotikIfRow): string {
  const n = String(r.if_name ?? r.display_name ?? "").toLowerCase();
  if (n.includes("wlan") || n.includes("wifi")) return "wireless";
  if (n.includes("sfp")) return "ethernet";
  if (n.includes("bridge")) return "bridge";
  if (n.includes("pppoe")) return "pppoe";
  if (n.includes("vlan")) return "vlan";
  return "ethernet";
}
