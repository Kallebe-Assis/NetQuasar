/** Parser CSV de conexões (mesmo formato do modelo; separador ; ou ,). */

export type ConnCsvPayload = {
  client_name: string;
  address: string | null;
  neighborhood: string | null;
  login: string;
  password: string | null;
  ip_address: string | null;
  connection_kind: "pppoe" | "dhcp";
  medium_type: "fibra" | "radio" | "cabo_utp" | null;
  sales_plan: string | null;
  onu_mac_sn: string | null;
  rx_dbm: string | null;
  tx_dbm: string | null;
  transmitter: string | null;
  cto: string | null;
  port: string | null;
  latitude: number | null;
  longitude: number | null;
};

export type ConnCsvParsedRow = { line: number; payload: ConnCsvPayload };

export type ConnCsvParseError = { line: number; login?: string; error: string };

function detectComma(firstLine: string): string {
  const semi = (firstLine.match(/;/g) ?? []).length;
  const comma = (firstLine.match(/,/g) ?? []).length;
  return semi >= comma ? ";" : ",";
}

function parseCsvLine(line: string, sep: string): string[] {
  const out: string[] = [];
  let cur = "";
  let inQ = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (inQ) {
      if (ch === '"') {
        if (line[i + 1] === '"') {
          cur += '"';
          i++;
        } else inQ = false;
      } else cur += ch;
    } else if (ch === '"') inQ = true;
    else if (ch === sep) {
      out.push(cur);
      cur = "";
    } else cur += ch;
  }
  out.push(cur);
  return out.map((c) => c.trim());
}

function normHeader(h: string): string {
  return h
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "_")
    .replace(/ã/g, "a")
    .replace(/á/g, "a")
    .replace(/é/g, "e")
    .replace(/ó/g, "o");
}

function columnMap(headers: string[]): Record<string, number> {
  const m: Record<string, number> = {};
  headers.forEach((h, i) => {
    m[normHeader(h)] = i;
  });
  const aliases: Record<string, string[]> = {
    client_name: ["client_name", "nome", "nome_cliente", "cliente"],
    address: ["address", "endereco", "endereço"],
    neighborhood: ["neighborhood", "bairro"],
    login: ["login", "pppoe", "usuario", "user"],
    password: ["password", "senha"],
    ip_address: ["ip_address", "ip"],
    connection_kind: ["connection_kind", "tipo_conexao", "tipo"],
    medium_type: ["medium_type", "meio", "tipo_meio"],
    sales_plan: ["sales_plan", "plano", "plano_venda"],
    onu_mac_sn: ["onu_mac_sn", "mac", "sn", "mac_sn"],
    rx_dbm: ["rx_dbm", "rx"],
    tx_dbm: ["tx_dbm", "tx"],
    transmitter: ["transmitter", "transmissor"],
    cto: ["cto"],
    port: ["port", "porta"],
    latitude: ["latitude", "lat"],
    longitude: ["longitude", "lon", "lng"],
  };
  const out: Record<string, number> = {};
  for (const [canon, keys] of Object.entries(aliases)) {
    for (const k of keys) {
      if (m[k] != null) {
        out[canon] = m[k];
        break;
      }
    }
  }
  return out;
}

function getCell(rec: string[], col: Record<string, number>, key: string): string {
  const i = col[key];
  if (i == null || i >= rec.length) return "";
  return String(rec[i] ?? "").trim();
}

function normKind(v: string): "pppoe" | "dhcp" | null {
  const s = v.toLowerCase().trim();
  if (!s) return "pppoe";
  if (["pppoe", "ppoe", "ppp"].includes(s)) return "pppoe";
  if (s === "dhcp") return "dhcp";
  return null;
}

function normMedium(v: string): "fibra" | "radio" | "cabo_utp" | null {
  const s = v.toLowerCase().trim();
  if (!s) return null;
  if (["fibra", "fiber", "ftth"].includes(s)) return "fibra";
  if (["radio", "rádio", "wireless"].includes(s)) return "radio";
  if (["cabo_utp", "utp", "cabo", "cabo utp"].includes(s)) return "cabo_utp";
  return null;
}

function parseRow(rec: string[], col: Record<string, number>): ConnCsvPayload | string {
  const client_name = getCell(rec, col, "client_name");
  const login = getCell(rec, col, "login");
  if (!client_name || !login) return "client_name e login obrigatórios";

  const kind = normKind(getCell(rec, col, "connection_kind"));
  if (!kind) return "connection_kind inválido (pppoe ou dhcp)";

  const mediumRaw = getCell(rec, col, "medium_type");
  const medium = mediumRaw ? normMedium(mediumRaw) : null;
  if (mediumRaw && !medium) return "medium_type inválido (fibra, radio, cabo_utp)";

  const latS = getCell(rec, col, "latitude");
  const lonS = getCell(rec, col, "longitude");
  let latitude: number | null = null;
  let longitude: number | null = null;
  if (latS || lonS) {
    if (!latS || !lonS) return "latitude e longitude devem ser preenchidas juntas";
    latitude = Number(latS.replace(",", "."));
    longitude = Number(lonS.replace(",", "."));
    if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) return "latitude/longitude inválidas";
    if (latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180) return "coordenadas fora do intervalo";
  }

  const opt = (k: string) => {
    const v = getCell(rec, col, k);
    return v || null;
  };

  return {
    client_name,
    login,
    connection_kind: kind,
    address: opt("address"),
    neighborhood: opt("neighborhood"),
    password: opt("password"),
    ip_address: opt("ip_address"),
    medium_type: medium,
    sales_plan: opt("sales_plan"),
    onu_mac_sn: opt("onu_mac_sn"),
    rx_dbm: opt("rx_dbm"),
    tx_dbm: opt("tx_dbm"),
    transmitter: opt("transmitter"),
    cto: opt("cto"),
    port: opt("port"),
    latitude,
    longitude,
  };
}

function rowEmpty(rec: string[]): boolean {
  return rec.every((c) => !String(c ?? "").trim());
}

export async function parseConnectionsCsvFile(file: File): Promise<{
  rows: ConnCsvParsedRow[];
  errors: ConnCsvParseError[];
}> {
  let text = await file.text();
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
  const lines = text.split(/\r?\n/).filter((l, i, arr) => i < arr.length - 1 || l.trim() !== "");
  if (lines.length === 0) {
    return { rows: [], errors: [{ line: 1, error: "ficheiro vazio" }] };
  }
  const sep = detectComma(lines[0]);
  const headers = parseCsvLine(lines[0], sep);
  const col = columnMap(headers);
  if (col.client_name == null) {
    return {
      rows: [],
      errors: [{ line: 1, error: "cabeçalho inválido: coluna nome_cliente obrigatória" }],
    };
  }
  if (col.login == null) {
    return { rows: [], errors: [{ line: 1, error: "cabeçalho inválido: coluna login obrigatória" }] };
  }

  const rows: ConnCsvParsedRow[] = [];
  const errors: ConnCsvParseError[] = [];
  for (let i = 1; i < lines.length; i++) {
    const line = i + 1;
    const rec = parseCsvLine(lines[i], sep);
    if (rowEmpty(rec)) continue;
    const parsed = parseRow(rec, col);
    if (typeof parsed === "string") {
      errors.push({ line, login: getCell(rec, col, "login") || undefined, error: parsed });
      continue;
    }
    rows.push({ line, payload: parsed });
  }
  return { rows, errors };
}

export const CONN_IMPORT_BATCH_SIZE = 500;
