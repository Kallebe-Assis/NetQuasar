export type OltTelnetReportStep = {
  command: string;
  ok?: boolean;
  output?: string;
  error?: string;
};

export type OltTelnetReportField = {
  label: string;
  value: string;
};

export type OltTelnetReportSection = {
  id: string;
  title: string;
  command: string;
  ok: boolean;
  fields: OltTelnetReportField[];
  rawClean: string;
};

const LABEL_PT: Record<string, string> = {
  onuindex: "Índice",
  "onu interface": "Interface",
  interface: "Interface",
  name: "Nome",
  type: "Modelo",
  model: "Modelo",
  "onu type configured": "Modelo",
  "onu type reported": "Modelo reportado",
  profile: "Profile",
  mode: "Modo",
  authinfo: "SN",
  "auth information": "SN",
  "serial number": "SN",
  "sn reported": "SN",
  "sn bind": "SN bind",
  "admin state": "Admin",
  admin: "Admin",
  "omcc state": "OMCC",
  omcc: "OMCC",
  "phase state": "Estado",
  state: "Estado",
  channel: "Canal",
  "onu id": "ONU ID",
  "onu distance": "Distância",
  distance: "Distância",
  "online duration": "Tempo online",
  "hardware version": "HW",
  "software version": "SW",
  rx: "RX",
  tx: "TX",
  "authentication mode": "Autenticação",
  "configured speed mode": "Velocidade config.",
  "current speed mode": "Velocidade actual",
  "config state": "Config",
  "onu status": "Status ONU",
  fec: "FEC",
  "onu number": "ONU Number",
};

const FIELD_ORDER = [
  "Interface",
  "Índice",
  "Nome",
  "Modelo",
  "Modelo reportado",
  "Profile",
  "SN",
  "Modo",
  "Admin",
  "OMCC",
  "Estado",
  "Canal",
  "Status ONU",
  "Config",
  "Autenticação",
  "RX",
  "TX",
  "Distância",
  "Tempo online",
  "ONU ID",
  "ONU Number",
  "HW",
  "SW",
  "Velocidade config.",
  "Velocidade actual",
  "FEC",
  "SN bind",
];

function normalizeLabel(raw: string): string {
  const key = raw.trim().toLowerCase();
  return LABEL_PT[key] ?? raw.trim();
}

function normalizeValue(label: string, value: string): string {
  let v = value.trim();
  const snParen = v.match(/sn\(([A-Za-z0-9]+)\)/i);
  if (snParen) return snParen[1];
  if (label === "SN" && /^sn\s+/i.test(v)) return v.replace(/^sn\s+/i, "").trim();
  return v;
}

function fieldKey(label: string): string {
  return label.trim().toLowerCase();
}

/** Remove sequências ANSI/CSI e artefactos comuns de paginação telnet. */
export function cleanTelnetCliOutput(raw: string): string {
  let s = raw ?? "";
  s = s.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, "");
  s = s.replace(/\[[0-9]{1,4}[A-Za-z]/g, "");
  s = s.replace(/\x08+/g, "");
  s = s.replace(/\x00/g, "");
  s = s.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  s = s.replace(/[ \t]+\n/g, "\n");
  s = s.replace(/\n{3,}/g, "\n\n");
  return s.trim();
}

function stripCommandEcho(output: string, command: string): string {
  const cmdCompact = command.replace(/\s+/g, "").toLowerCase();
  const lines = output.split("\n");
  const kept: string[] = [];
  let echoSkips = 0;
  for (const line of lines) {
    const compact = line.replace(/\s+/g, "").toLowerCase();
    if (echoSkips < 4 && compact && (compact === cmdCompact || compact.includes(cmdCompact.slice(0, 12)))) {
      echoSkips++;
      continue;
    }
    kept.push(line);
  }
  return kept.join("\n").trim();
}

function extractKeyValueFields(text: string): OltTelnetReportField[] {
  const fields: OltTelnetReportField[] = [];
  const seen = new Set<string>();
  for (const line of text.split("\n")) {
    if (/^-{3,}/.test(line.trim())) continue;
    if (/^Authpass\s+Time/i.test(line.trim())) break;
    const m = line.match(/^\s{0,6}([A-Za-z0-9 /_-]{2,44}):\s+(.+?)\s*$/);
    if (!m) continue;
    const label = normalizeLabel(m[1].trim());
    const value = normalizeValue(label, m[2]);
    if (!value || label.length < 1) continue;
    const key = fieldKey(label);
    if (seen.has(key)) continue;
    seen.add(key);
    fields.push({ label, value });
  }
  return fields;
}

function extractPowerFields(text: string, labelRx = "RX", labelTx = "TX"): OltTelnetReportField[] {
  const fields: OltTelnetReportField[] = [];
  const lineMatch = text.match(/gpon_onu[^\n]+\s+(-?\d+(?:\.\d+)?)\s*\(dbm\)/i);
  if (lineMatch) {
    const isRx = /onu-rx|rx power/i.test(text) || Number(lineMatch[1]) < 0;
    fields.push({ label: isRx ? labelRx : labelTx, value: `${lineMatch[1]} dBm` });
  }
  return fields;
}

function dataRowsAfterHeader(text: string, headerRe: RegExp): string {
  const lines = text.split("\n");
  let headerIdx = -1;
  for (let i = 0; i < lines.length; i++) {
    if (headerRe.test(lines[i])) {
      headerIdx = i;
      break;
    }
  }
  if (headerIdx < 0) return "";
  const parts: string[] = [];
  for (let i = headerIdx + 1; i < lines.length; i++) {
    const t = lines[i].trim();
    if (!t) continue;
    if (/^---/.test(t)) continue;
    if (/^Authpass\s+Time/i.test(t)) break;
    if (/gpon-olt|olt-zte|^\$|^#/.test(t)) break;
    parts.push(t);
  }
  return parts.join(" ").replace(/\s+/g, " ").trim();
}

function extractVsolOnuInfo(text: string): OltTelnetReportField[] {
  const row = dataRowsAfterHeader(text, /Onuindex.*Model.*Profile/i);
  if (!row) return [];
  const m = row.match(/^(GPON[\d/:\w-]+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)/i);
  if (!m) return [];
  return [
    { label: "Índice", value: m[1] },
    { label: "Modelo", value: m[2] },
    { label: "Profile", value: m[3] },
    { label: "Modo", value: m[4] },
    { label: "SN", value: m[5] },
  ];
}

function extractVsolOnuState(text: string): OltTelnetReportField[] {
  const row = dataRowsAfterHeader(text, /OnuIndex.*Admin State/i);
  if (!row) return [];
  const m = row.match(/^(\d+\/\d+\/\d+:\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+?)(?:\s+ONU Number:|$)/i);
  const fields: OltTelnetReportField[] = [];
  if (m) {
    fields.push(
      { label: "Admin", value: m[2] },
      { label: "OMCC", value: m[3] },
      { label: "Estado", value: m[4] },
      { label: "Canal", value: m[5].trim() },
    );
  }
  const onuNum = text.match(/ONU Number:\s*(\S+)/i);
  if (onuNum) fields.push({ label: "ONU Number", value: onuNum[1] });
  return fields;
}

function extractFieldsForCommand(command: string, cleaned: string): OltTelnetReportField[] {
  const cmd = command.toLowerCase();
  if (/show\s+onu\s+info/.test(cmd)) {
    const vsol = extractVsolOnuInfo(cleaned);
    if (vsol.length > 0) return vsol;
  }
  if (/show\s+onu\s+state/.test(cmd)) {
    const vsol = extractVsolOnuState(cleaned);
    if (vsol.length > 0) return vsol;
  }
  if (/onu-rx/.test(cmd)) return extractPowerFields(cleaned, "RX", "TX");
  if (/onu-tx/.test(cmd)) return extractPowerFields(cleaned, "RX", "TX");

  let fields = extractKeyValueFields(cleaned);
  const power = extractPowerFields(cleaned);
  for (const pf of power) {
    if (!fields.some((f) => fieldKey(f.label) === fieldKey(pf.label))) fields.push(pf);
  }
  return fields;
}

function mergeFields(into: Map<string, OltTelnetReportField>, list: OltTelnetReportField[]) {
  for (const f of list) {
    const key = fieldKey(f.label);
    const prev = into.get(key);
    if (!prev || f.value.length > prev.value.length) {
      into.set(key, f);
    }
  }
}

function sortFields(fields: OltTelnetReportField[]): OltTelnetReportField[] {
  const orderIdx = new Map(FIELD_ORDER.map((l, i) => [fieldKey(l), i]));
  return [...fields].sort((a, b) => {
    const ia = orderIdx.get(fieldKey(a.label)) ?? 999;
    const ib = orderIdx.get(fieldKey(b.label)) ?? 999;
    if (ia !== ib) return ia - ib;
    return a.label.localeCompare(b.label, "pt");
  });
}

export function buildTelnetReportSections(steps: OltTelnetReportStep[]): OltTelnetReportSection[] {
  return steps.map((step, idx) => {
    const cleaned = cleanTelnetCliOutput(stripCommandEcho(step.output ?? "", step.command));
    const fields = extractFieldsForCommand(step.command, cleaned);
    return {
      id: `step-${idx}`,
      title: step.command,
      command: step.command,
      ok: step.ok !== false && !step.error,
      fields,
      rawClean: cleaned,
    };
  });
}

/** Tabela única 2 colunas — todos os campos deduplicados e ordenados. */
export function buildUnifiedReportTable(sections: OltTelnetReportSection[]): OltTelnetReportField[] {
  const map = new Map<string, OltTelnetReportField>();
  for (const sec of sections) {
    mergeFields(map, sec.fields);
  }
  return sortFields([...map.values()]);
}

export function formatTelnetReportPlainText(rows: OltTelnetReportField[], title: string): string {
  const lines: string[] = [title, ""];
  for (const r of rows) {
    lines.push(`${r.label}: ${r.value}`);
  }
  return lines.join("\n").trim();
}
