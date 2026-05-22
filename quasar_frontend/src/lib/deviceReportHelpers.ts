/** Funções puras partilhadas pelo modal de relatório de equipamento (Equipamentos + Monitoramento). */

export type ReportPeriod = "24h" | "7d" | "30d";

export type PingHistorySample = {
  id: number;
  checked_at: string;
  ok: boolean;
  latency_ms?: number;
  method?: string;
};

export type TelemetryHistorySample = {
  id: number;
  collected_at: string;
  metrics?: Record<string, unknown>;
};

export function reportPeriodMs(p: ReportPeriod): number {
  switch (p) {
    case "24h":
      return 24 * 60 * 60 * 1000;
    case "7d":
      return 7 * 24 * 60 * 60 * 1000;
    case "30d":
      return 30 * 24 * 60 * 60 * 1000;
    default:
      return 7 * 24 * 60 * 60 * 1000;
  }
}

export function reportWindowIso(p: ReportPeriod, nowMs = Date.now()) {
  const ms = reportPeriodMs(p);
  const to = nowMs;
  const from = to - ms;
  const prevTo = from;
  const prevFrom = from - ms;
  return {
    fromIso: new Date(from).toISOString(),
    toIso: new Date(to).toISOString(),
    prevFromIso: new Date(prevFrom).toISOString(),
    prevToIso: new Date(prevTo).toISOString(),
  };
}

export function escapeCsvCell(raw: string): string {
  const s = String(raw ?? "");
  if (/[",\n\r]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
  return s;
}

export function aggregatePingSamples(samples: PingHistorySample[]) {
  const lats = samples.map((s) => s.latency_ms).filter((x): x is number => typeof x === "number" && Number.isFinite(x));
  const okCount = samples.filter((s) => s.ok).length;
  return {
    avgLatency: lats.length ? lats.reduce((a, b) => a + b, 0) / lats.length : null,
    maxLatency: lats.length ? Math.max(...lats) : null,
    okRatio: samples.length ? okCount / samples.length : null,
    n: samples.length,
  };
}

export function formatNum(n: number | null, decimals = 1, suffix = ""): string {
  if (n == null || !Number.isFinite(n)) return "—";
  return `${n.toFixed(decimals)}${suffix}`;
}

export function formatDelta(prev: number | null, curr: number | null, decimals = 1, suffix = ""): string {
  if (prev == null || curr == null || !Number.isFinite(prev) || !Number.isFinite(curr)) return "—";
  const d = curr - prev;
  const sign = d > 0 ? "+" : "";
  return `${sign}${d.toFixed(decimals)}${suffix}`;
}

/** Valores brutos por OID na última colecta SNMP (métricas.telemetria). */
export function snmpVarsFromMetrics(metrics: Record<string, unknown> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  const vars = (metrics?.snmp as { vars?: Array<{ oid?: string; value?: string }> } | undefined)?.vars ?? [];
  for (const v of vars) {
    const oid = String(v?.oid ?? "").trim().replace(/^\./, "");
    const value = String(v?.value ?? "").trim();
    if (!oid) continue;
    out[oid] = value;
  }
  return out;
}

function parseNumber(v: string | undefined): number | null {
  if (!v) return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

export function parseTelemetryKPIs(sample: TelemetryHistorySample): { cpu: number | null; memory: number | null; temp: number | null } {
  const metrics = sample.metrics ?? {};
  const profile = (metrics.profile as Record<string, unknown> | undefined) ?? {};
  const vars = snmpVarsFromMetrics(metrics);
  const cpuOID = String(profile.cpu_primary_oid ?? "").trim().replace(/^\./, "");
  const cpuAvailOID = String(profile.cpu_available_oid ?? "").trim().replace(/^\./, "");
  const memUsedOID = String(profile.memory_used_oid ?? "").trim().replace(/^\./, "");
  const memSizeOID = String(profile.memory_size_oid ?? "").trim().replace(/^\./, "");
  const tempOID = String(profile.temp_primary_oid ?? "").trim().replace(/^\./, "");

  let cpu: number | null = null;
  if (cpuOID && vars[cpuOID] != null) {
    const v = parseNumber(vars[cpuOID]);
    if (v != null) {
      if (cpuOID === "1.3.6.1.4.1.2021.11.11.0") cpu = 100 - v;
      else if (cpuOID === "1.3.6.1.4.1.14988.1.1.3.10.0") cpu = v > 100 ? v / 10 : v;
      else cpu = v;
    }
  } else if (cpuAvailOID && vars[cpuAvailOID] != null) {
    const v = parseNumber(vars[cpuAvailOID]);
    if (v != null) cpu = 100 - v;
  }

  let memory: number | null = null;
  if (memUsedOID && memSizeOID && vars[memUsedOID] != null && vars[memSizeOID] != null) {
    const used = parseNumber(vars[memUsedOID]);
    const size = parseNumber(vars[memSizeOID]);
    if (used != null && size != null && size > 0) {
      if (memUsedOID === "1.3.6.1.4.1.2021.4.6.0") memory = ((size - used) / size) * 100;
      else memory = (used / size) * 100;
    }
  }

  let temp: number | null = null;
  if (tempOID && vars[tempOID] != null) {
    const t = parseNumber(vars[tempOID]);
    if (t != null) temp = t > 100 ? t / 10 : t;
  }

  const normalize = (n: number | null): number | null => {
    if (n == null || !Number.isFinite(n)) return null;
    if (n < -273 || n > 10000) return null;
    return n;
  };
  return { cpu: normalize(cpu), memory: normalize(memory), temp: normalize(temp) };
}

/** OIDs já representados pelos indicadores principais (CPU, memória, temperatura, etc.) — não repetir como “extra”. */
export function collectProfileOidExclusions(profile: Record<string, unknown>): Set<string> {
  const s = new Set<string>();
  const add = (raw: unknown) => {
    const v = String(raw ?? "").trim().replace(/^\./, "");
    if (v) s.add(v);
  };
  add(profile.uptime_oid);
  add(profile.sysname_oid);
  add(profile.sysdescr_oid);
  add(profile.cpu_primary_oid);
  add(profile.cpu_available_oid);
  add(profile.memory_used_oid);
  add(profile.memory_size_oid);
  add(profile.temp_primary_oid);
  const addArr = (key: string) => {
    const a = profile[key];
    if (!Array.isArray(a)) return;
    for (const x of a) add(x);
  };
  addArr("cpu_oids");
  addArr("memory_oids");
  addArr("temp_oids");
  return s;
}

/** Rótulo legível para OIDs conhecidos; `null` se não houver nome estável (o chamador numera “Métrica SNMP adicional”). */
export function oidFriendlyDescription(oid: string): string | null {
  const o = String(oid ?? "").trim().replace(/^\./, "");
  if (!o) return null;
  const ifMib = (sub: string, labelPt: string) => {
    const m = new RegExp(`^1\\.3\\.6\\.1\\.2\\.1\\.2\\.2\\.1\\.${sub}\\.(\\d+)$`).exec(o);
    if (!m) return null;
    return `${labelPt} (interface índice ${m[1]})`;
  };
  return (
    ifMib("2", "Descrição da interface") ??
    ifMib("10", "Octetos recebidos (entrada)") ??
    ifMib("16", "Octetos enviados (saída)") ??
    ifMib("5", "Velocidade da interface (bps)") ??
    ifMib("7", "Estado administrativo da interface") ??
    ifMib("8", "Estado operacional da interface") ??
    ({
      "1.3.6.1.2.1.1.1.0": "Descrição do sistema",
      "1.3.6.1.2.1.1.3.0": "Tempo ativo do sistema",
      "1.3.6.1.2.1.1.5.0": "Nome do sistema",
      "1.3.6.1.2.1.1.6.0": "Localização do sistema",
      "1.3.6.1.4.1.14988.1.1.3.10.0": "CPU (equipamento Mikrotik)",
      "1.3.6.1.4.1.2021.11.11.0": "CPU — tempo inativo (UCD-SNMP)",
      "1.3.6.1.4.1.2021.4.6.0": "Memória RAM livre (UCD-SNMP)",
      "1.3.6.1.4.1.2021.4.5.0": "Memória RAM total (UCD-SNMP)",
    } as Record<string, string>)[o] ??
    (/^1\.3\.6\.1\.2\.1\.25\.3\.3\.1\.2\.\d+$/.test(o) ? "Carga do processador (HOST-RESOURCES)" : null) ??
    (/^1\.3\.6\.1\.2\.1\.99\.1\.1\.1\.4\.\d+$/.test(o) ? "Temperatura (ENTITY-SENSOR)" : null) ??
    null
  );
}

export type ReportMainTableRow = { description: string; value: string };

/** Linhas da tabela principal do relatório: estado + leituras SNMP extra (sem mostrar OID ao utilizador). */
export function buildDeviceReportMainTable(args: {
  pingLatest?: Record<string, unknown> | null;
  telemetryLatest?: { collected_at?: string; metrics?: Record<string, unknown> } | null;
  fallbackKpis?: { cpu: number | null; memory: number | null; temp: number | null };
}): ReportMainTableRow[] {
  const rows: ReportMainTableRow[] = [];
  const ok = args.pingLatest?.ok;
  rows.push({
    description: "Estado do ping",
    value: typeof ok === "boolean" ? (ok ? "OK" : "Sem resposta") : "—",
  });
  const lat = args.pingLatest?.latency_ms;
  rows.push({
    description: "Latência do ping (ms)",
    value: typeof lat === "number" && Number.isFinite(lat) ? String(lat) : "—",
  });
  const m = args.telemetryLatest?.metrics;
  const kpis =
    m && typeof m === "object"
      ? parseTelemetryKPIs({ id: 0, collected_at: String(args.telemetryLatest?.collected_at ?? ""), metrics: m as Record<string, unknown> })
      : args.fallbackKpis ?? { cpu: null, memory: null, temp: null };
  rows.push({
    description: "Uso de CPU (%)",
    value: kpis.cpu != null ? kpis.cpu.toFixed(1) : "—",
  });
  rows.push({
    description: "Uso de memória (%)",
    value: kpis.memory != null ? kpis.memory.toFixed(1) : "—",
  });
  rows.push({
    description: "Temperatura (°C)",
    value: kpis.temp != null ? kpis.temp.toFixed(1) : "—",
  });
  if (!m || typeof m !== "object") return rows;
  const profile = (m.profile as Record<string, unknown> | undefined) ?? {};
  const excluded = collectProfileOidExclusions(profile);
  const vars = snmpVarsFromMetrics(m as Record<string, unknown>);
  const extraLabelsRaw = profile.extra_oid_labels as Record<string, unknown> | undefined;
  const extraLabels: Record<string, string> = {};
  if (extraLabelsRaw && typeof extraLabelsRaw === "object") {
    for (const [k, v] of Object.entries(extraLabelsRaw)) {
      const kk = String(k ?? "")
        .trim()
        .replace(/^\./, "");
      const vv = String(v ?? "").trim();
      if (kk && vv) extraLabels[kk] = vv;
    }
  }
  const userLabelForOid = (oid: string): string | undefined => {
    const o = String(oid ?? "")
      .trim()
      .replace(/^\./, "");
    if (!o) return undefined;
    return extraLabels[o];
  };
  let extraIdx = 0;
  for (const [oid, value] of Object.entries(vars)) {
    if (excluded.has(oid)) continue;
    const userLab = userLabelForOid(oid);
    const friendly = oidFriendlyDescription(oid);
    const description = userLab ?? friendly ?? `Métrica SNMP adicional ${++extraIdx}`;
    rows.push({ description, value: value === "" ? "—" : value });
  }
  return rows;
}

/** Converte o JSON salvo em `interface_snapshots` (lista { oid, value, type }) em linhas para tabela. */
/** Mikrotik / rádio / OLT: mostrar interfaces no relatório pela categoria ou pela marca. */
export function deviceShowsInterfaceMonitorSection(category: string, brand?: string | null): boolean {
  const c = (category ?? "").trim().toLowerCase();
  if (c === "olt") return true;
  if (/^mikrotik$|^rádio$|^radio$/i.test((category ?? "").trim())) return true;
  const b = String(brand ?? "")
    .trim()
    .toLowerCase();
  return b.includes("mikrotik");
}

export type InterfaceMonitorTableRow = {
  if_index?: number;
  descr?: string;
  if_name?: string;
  display_name?: string;
  speed_bps?: number;
  admin_status?: string;
  oper_status?: string;
  in_octets?: number;
  out_octets?: number;
  in_bps?: number;
  out_bps?: number;
  sfp?: boolean;
  tx_dbm?: number;
  rx_dbm?: number;
  /** ge_vlan | vlan | pon | onu | other — preenchido pelo backend para categoria OLT */
  olt_iface_kind?: string;
  octets_saturated_32bit?: boolean;
};

export function interfaceMonitorRowsFromApi(data: Record<string, unknown> | undefined): InterfaceMonitorTableRow[] {
  const raw = data?.interface_table;
  if (!Array.isArray(raw)) return [];
  const out: InterfaceMonitorTableRow[] = [];
  for (const item of raw) {
    if (!item || typeof item !== "object") continue;
    out.push(item as InterfaceMonitorTableRow);
  }
  return out;
}

/** Agrupa linhas OLT (campo olt_iface_kind vindo do backend). */
export function groupOltInterfaceRows(rows: InterfaceMonitorTableRow[]) {
  const geVlan: InterfaceMonitorTableRow[] = [];
  const pon: InterfaceMonitorTableRow[] = [];
  const onu: InterfaceMonitorTableRow[] = [];
  const other: InterfaceMonitorTableRow[] = [];
  for (const r of rows) {
    const k = String(r.olt_iface_kind ?? "");
    if (k === "ge_vlan" || k === "vlan") geVlan.push(r);
    else if (k === "pon") pon.push(r);
    else if (k === "onu") onu.push(r);
    else other.push(r);
  }
  return { geVlan, pon, onu, other };
}

export function interfaceSnapshotTableRows(interfaces: unknown): ReportMainTableRow[] {
  if (!Array.isArray(interfaces)) return [];
  const rows: ReportMainTableRow[] = [];
  for (const item of interfaces) {
    if (!item || typeof item !== "object") continue;
    const rec = item as Record<string, unknown>;
    const oid = String(rec.oid ?? "").trim().replace(/^\./, "");
    const value = String(rec.value ?? "").trim();
    if (!oid) continue;
    const desc = oidFriendlyDescription(oid) ?? "Leitura de interface";
    rows.push({ description: desc, value: value === "" ? "—" : value });
  }
  return rows;
}

export function aggregateTelemetryFromSamples(samples: TelemetryHistorySample[]) {
  const kpis = samples.map((s) => parseTelemetryKPIs(s));
  const avg = (key: "cpu" | "memory" | "temp") => {
    const vals = kpis.map((k) => k[key]).filter((v): v is number => v != null && Number.isFinite(v));
    if (!vals.length) return null;
    return vals.reduce((a, b) => a + b, 0) / vals.length;
  };
  return { cpu: avg("cpu"), memory: avg("memory"), temp: avg("temp"), n: samples.length };
}

export function shortFmtIso(iso: string): string {
  try {
    return new Date(iso).toLocaleString("pt-PT", { dateStyle: "short", timeStyle: "short" });
  } catch {
    return iso;
  }
}

/** Data/hora discreta para «coletado em …» (OLT, Mikrotik, relatório). */
export function formatCollectedPt(iso: string | null | undefined): string {
  if (!iso || typeof iso !== "string") return "—";
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "—";
    return d.toLocaleString("pt-PT", {
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return "—";
  }
}

export function buildFullDeviceReportCsv(args: {
  device: { id: string; description: string; ip?: string | null; category: string; brand?: string | null };
  period: ReportPeriod;
  win: ReturnType<typeof reportWindowIso>;
  pingLatest?: Record<string, unknown>;
  telemetryLatest?: Record<string, unknown>;
  pingCurr: PingHistorySample[];
  pingPrev: PingHistorySample[];
  telCurr: TelemetryHistorySample[];
  telPrev: TelemetryHistorySample[];
  alertsCurr: Array<Record<string, unknown>>;
  alertsPrev: Array<Record<string, unknown>>;
  snapshots: Array<{ id: number; collected_at: string; interfaces?: unknown }>;
  inventory?: Record<string, unknown>;
  interfacesLatest?: Record<string, unknown> | null;
  /** GET /olt/devices/{id} — contagens por PON para relatório de OLT */
  oltDeviceLatest?: Record<string, unknown> | null;
}): string {
  const row = (...cells: (string | number | boolean | null | undefined)[]) =>
    cells.map((c) => escapeCsvCell(c === null || c === undefined ? "" : String(c))).join(",");
  const lines: string[] = [];
  const { device: d, period, win, pingLatest, telemetryLatest } = args;
  lines.push(row("secção", "campo", "valor"));
  lines.push(row("meta", "id", d.id));
  lines.push(row("meta", "descrição", d.description));
  lines.push(row("meta", "ip", d.ip ?? ""));
  lines.push(row("meta", "categoria", d.category));
  lines.push(row("meta", "marca", d.brand ?? ""));
  lines.push(row("meta", "período_gráficos", period));
  lines.push(row("meta", "intervalo_actual_de", win.fromIso));
  lines.push(row("meta", "intervalo_actual_até", win.toIso));
  lines.push(row("meta", "intervalo_anterior_de", win.prevFromIso));
  lines.push(row("meta", "intervalo_anterior_até", win.prevToIso));
  lines.push(row("último_ping", "ok", String(pingLatest?.ok ?? "")));
  lines.push(row("último_ping", "latency_ms", String(pingLatest?.latency_ms ?? "")));
  lines.push(row("última_telemetria", "json", JSON.stringify(telemetryLatest ?? {})));
  const telLatestShape =
    telemetryLatest && typeof telemetryLatest === "object" && (telemetryLatest as Record<string, unknown>).metrics != null
      ? {
          collected_at: String((telemetryLatest as Record<string, unknown>).collected_at ?? ""),
          metrics: (telemetryLatest as Record<string, unknown>).metrics as Record<string, unknown>,
        }
      : null;
  const telSorted = [...args.telCurr].sort((a, b) => new Date(b.collected_at).getTime() - new Date(a.collected_at).getTime());
  const fbKpis = telSorted[0] ? parseTelemetryKPIs(telSorted[0]) : undefined;
  lines.push(row("estado_relatorio", "descrição", "valor"));
  for (const r of buildDeviceReportMainTable({
    pingLatest: pingLatest ?? null,
    telemetryLatest: telLatestShape,
    fallbackKpis: fbKpis,
  })) {
    lines.push(row("estado_relatorio", r.description, r.value));
  }
  const pC = aggregatePingSamples(args.pingCurr);
  const pP = aggregatePingSamples(args.pingPrev);
  const tC = aggregateTelemetryFromSamples(args.telCurr);
  const tP = aggregateTelemetryFromSamples(args.telPrev);
  lines.push(row("agregados_anterior", "ping_n", pP.n));
  lines.push(row("agregados_anterior", "ping_lat_media", pP.avgLatency ?? ""));
  lines.push(row("agregados_anterior", "ping_lat_max", pP.maxLatency ?? ""));
  lines.push(row("agregados_anterior", "ping_ok_ratio", pP.okRatio ?? ""));
  lines.push(row("agregados_anterior", "cpu_media", tP.cpu ?? ""));
  lines.push(row("agregados_anterior", "mem_media", tP.memory ?? ""));
  lines.push(row("agregados_anterior", "temp_media", tP.temp ?? ""));
  lines.push(row("agregados_anterior", "alertas_n", args.alertsPrev.length));
  lines.push(row("agregados_actual", "ping_n", pC.n));
  lines.push(row("agregados_actual", "ping_lat_media", pC.avgLatency ?? ""));
  lines.push(row("agregados_actual", "ping_lat_max", pC.maxLatency ?? ""));
  lines.push(row("agregados_actual", "ping_ok_ratio", pC.okRatio ?? ""));
  lines.push(row("agregados_actual", "cpu_media", tC.cpu ?? ""));
  lines.push(row("agregados_actual", "mem_media", tC.memory ?? ""));
  lines.push(row("agregados_actual", "temp_media", tC.temp ?? ""));
  lines.push(row("agregados_actual", "alertas_n", args.alertsCurr.length));
  lines.push(row("ping_histórico", "checked_at", "ok", "latency_ms"));
  for (const s of args.pingCurr) {
    lines.push(row("ping_histórico", s.checked_at, String(s.ok), s.latency_ms ?? ""));
  }
  lines.push(row("telemetria_histórico", "collected_at", "metrics_json"));
  for (const s of args.telCurr) {
    lines.push(row("telemetria_histórico", s.collected_at, JSON.stringify(s.metrics ?? {})));
  }
  lines.push(row("alertas", "active_since", "severity", "type", "message"));
  for (const e of args.alertsCurr) {
    lines.push(
      row(
        "alertas",
        String(e.active_since ?? ""),
        String(e.severity ?? ""),
        String(e.type ?? e.event_type ?? ""),
        String(e.message ?? ""),
      ),
    );
  }
  lines.push(row("snapshots_interfaces", "json", JSON.stringify(args.snapshots)));
  const ifLatest = args.interfacesLatest;
  if (ifLatest && typeof ifLatest === "object") {
    lines.push(row("interfaces_última_colecta", "collected_at", String(ifLatest.collected_at ?? "")));
    lines.push(
      row(
        "interfaces_última_colecta",
        "cabeçalho",
        "if_index",
        "tipo_olt",
        "nome",
        "admin",
        "oper",
        "in_octets",
        "out_octets",
        "sfp",
        "rx_dbm",
        "tx_dbm",
      ),
    );
    for (const r of interfaceMonitorRowsFromApi(ifLatest)) {
      lines.push(
        row(
          "interfaces_última_colecta",
          "linha",
          r.if_index ?? "",
          r.olt_iface_kind ?? "",
          String(r.display_name ?? r.if_name ?? r.descr ?? "").trim() || "—",
          r.admin_status ?? "",
          r.oper_status ?? "",
          r.octets_saturated_32bit ? "—" : r.in_octets ?? "",
          r.octets_saturated_32bit ? "—" : r.out_octets ?? "",
          r.sfp === true ? "sim" : r.sfp === false ? "não" : "",
          r.rx_dbm ?? "",
          r.tx_dbm ?? "",
        ),
      );
    }
  }
  const olt = args.oltDeviceLatest;
  if (olt && typeof olt === "object") {
    lines.push(row("olt_snapshot", "pons_json", JSON.stringify((olt as Record<string, unknown>).pons ?? [])));
    lines.push(row("olt_snapshot", "pons_table_json", JSON.stringify((olt as Record<string, unknown>).pons_table ?? [])));
    lines.push(row("olt_snapshot", "computed_json", JSON.stringify((olt as Record<string, unknown>).computed ?? {})));
  }
  lines.push(row("inventário_snmp", "json", JSON.stringify(args.inventory ?? {})));
  return lines.join("\n");
}

const CADASTRAL_LABELS: Record<string, string> = {
  id: "ID",
  description: "Descrição",
  category: "Categoria",
  ip: "IP",
  network_status: "Estado da rede",
  pop_id: "POP (id)",
  locality_id: "Localidade (id)",
  access_mode: "Modo de acesso",
  telemetry_mode: "Modo de telemetria",
  ping_enabled: "Monitorizar com ping",
  telemetry_enabled: "Telemetria ativa",
  operational_mode: "Modo operacional",
  latitude: "Latitude",
  longitude: "Longitude",
  brand: "Marca",
  model: "Modelo",
  mac: "MAC",
  serial_number: "Número de série",
  software_version: "Versão firmware / software",
  hardware_version: "Versão hardware",
  acquired_at: "Data de aquisição",
  snmp_community: "Community SNMP",
  mib_folder_path: "Pasta MIBs (servidor)",
  telemetry_oid_strategy: "Estratégia de OIDs (telemetria)",
  telemetry_oid_overrides: "OIDs manuais (JSON)",
};

const CADASTRAL_ORDER = [
  "id",
  "description",
  "category",
  "ip",
  "network_status",
  "pop_id",
  "locality_id",
  "access_mode",
  "telemetry_mode",
  "ping_enabled",
  "telemetry_enabled",
  "operational_mode",
  "latitude",
  "longitude",
  "brand",
  "model",
  "mac",
  "serial_number",
  "software_version",
  "hardware_version",
  "acquired_at",
  "snmp_community",
  "mib_folder_path",
  "telemetry_oid_strategy",
  "telemetry_oid_overrides",
];

export function formatCadastroCellValue(val: unknown): string {
  if (val === null || val === undefined) return "—";
  if (typeof val === "boolean") return val ? "sim" : "não";
  if (typeof val === "number" && Number.isFinite(val)) return String(val);
  if (typeof val === "string") return val.trim() === "" ? "—" : val;
  if (typeof val === "object") {
    try {
      return JSON.stringify(val);
    } catch {
      return String(val);
    }
  }
  return String(val);
}

/** Linhas da tabela de ficha completa do equipamento (resposta GET /devices/:id). */
export function buildDeviceCadastroRows(device: Record<string, unknown>): { label: string; value: string }[] {
  const seen = new Set<string>();
  const rows: { label: string; value: string }[] = [];
  for (const key of CADASTRAL_ORDER) {
    if (!(key in device)) continue;
    seen.add(key);
    rows.push({
      label: CADASTRAL_LABELS[key] ?? key,
      value: formatCadastroCellValue(device[key]),
    });
  }
  const rest = Object.keys(device)
    .filter((k) => !seen.has(k))
    .sort();
  for (const key of rest) {
    rows.push({
      label: CADASTRAL_LABELS[key] ?? key,
      value: formatCadastroCellValue(device[key]),
    });
  }
  return rows;
}

export function cadastroPlainTextForClipboard(rows: { label: string; value: string }[]): string {
  return rows.map((r) => `${r.label}: ${r.value}`).join("\n");
}
