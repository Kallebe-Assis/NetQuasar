/** Parser CSV de infraestrutura (CTOs, emendas, cabos, postes). */

import { CABLE_STATUSES, FIBER_COLORS, normalizeSplitterInput } from "./networkInfrastructure";

export type InfraVariant = "cto" | "splice" | "cable" | "pole";

export type InfraCsvParseError = { line: number; description?: string; error: string };

export type InfraCsvParsedRow<T> = { line: number; payload: T };

export type InfraCtoPayload = {
  description: string;
  latitude: number | null;
  longitude: number | null;
  splitter: string | null;
  transmitter: string | null;
  fiber_color: string | null;
  locality_name: string | null;
  project_number: number | null;
  needs_maintenance: boolean;
  notes: string | null;
};

export type InfraSplicePayload = {
  description: string;
  latitude: number | null;
  longitude: number | null;
  fiber_count: number | null;
  needs_maintenance: boolean;
  notes: string | null;
  project_number: number | null;
};

export type InfraCablePayload = {
  description: string;
  cable_type: string | null;
  fiber_count: number | null;
  status: string;
  latitude: number | null;
  longitude: number | null;
  project_number: number | null;
};

export type InfraPolePayload = {
  description: string;
  pole_type: string | null;
  locality_name: string | null;
  latitude: number | null;
  longitude: number | null;
  project_number: number | null;
};

export const INFRA_IMPORT_BATCH_SIZE = 500;

export const INFRA_CSV_TEMPLATES: Record<
  InfraVariant,
  { headers: string[]; sample: string[]; fileName: string }
> = {
  cto: {
    fileName: "modelo_ctos.csv",
    headers: [
      "descricao",
      "latitude",
      "longitude",
      "splitter",
      "transmissor",
      "cor_fibra",
      "localidade",
      "numero_projeto",
      "manutencao",
      "observacoes",
    ],
    sample: [
      "CTO Rua das Flores",
      "-23,55052",
      "-46,63331",
      "1x8",
      "OLT-01",
      "Verde",
      "Centro",
      "1",
      "nao",
      "Próximo ao poste 12",
    ],
  },
  splice: {
    fileName: "modelo_caixas_emenda.csv",
    headers: ["descricao", "latitude", "longitude", "fibras", "manutencao", "observacoes", "numero_projeto"],
    sample: ["CE-01 Av. Principal", "-23,55100", "-46,63400", "24", "nao", "", "1"],
  },
  cable: {
    fileName: "modelo_cabos.csv",
    headers: ["descricao", "tipo", "fibras", "status", "latitude", "longitude", "numero_projeto"],
    sample: ["Cabo backbone A-B", "AS-80", "12", "ativo", "-23,55000", "-46,63300", "1"],
  },
  pole: {
    fileName: "modelo_postes.csv",
    headers: ["descricao", "tipo", "localidade", "latitude", "longitude", "numero_projeto"],
    sample: ["Poste 45", "concreto", "Centro", "-23,55020", "-46,63350", "1"],
  },
};

function detectSep(firstLine: string): string {
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
    .replace(/ó/g, "o")
    .replace(/ç/g, "c");
}

function columnMap(headers: string[], aliases: Record<string, string[]>): Record<string, number> {
  const m: Record<string, number> = {};
  headers.forEach((h, i) => {
    m[normHeader(h)] = i;
  });
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

function parseCoords(rec: string[], col: Record<string, number>): { lat: number | null; lon: number | null; err?: string } {
  const latS = getCell(rec, col, "latitude");
  const lonS = getCell(rec, col, "longitude");
  if (!latS && !lonS) return { lat: null, lon: null };
  if (!latS || !lonS) return { lat: null, lon: null, err: "latitude e longitude devem ser preenchidas juntas" };
  const lat = Number(latS.replace(",", "."));
  const lon = Number(lonS.replace(",", "."));
  if (!Number.isFinite(lat) || !Number.isFinite(lon)) return { lat: null, lon: null, err: "latitude/longitude inválidas" };
  if (lat < -90 || lat > 90 || lon < -180 || lon > 180) return { lat: null, lon: null, err: "coordenadas fora do intervalo" };
  return { lat, lon };
}

function parseBool(v: string): boolean {
  const s = v.toLowerCase().trim();
  return ["1", "true", "sim", "s", "yes", "y"].includes(s);
}

function parseOptInt(v: string): number | null {
  const t = v.trim();
  if (!t) return null;
  const n = Number(t);
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : null;
}

function normFiberColor(v: string): string | null {
  const s = v.trim();
  if (!s) return null;
  const found = FIBER_COLORS.find((c) => c.toLowerCase() === s.toLowerCase());
  return found ?? null;
}

function normCableStatus(v: string): string | null {
  const s = v.toLowerCase().trim();
  if (!s) return "ativo";
  const found = CABLE_STATUSES.find((c) => c.value === s);
  return found ? found.value : null;
}

function rowEmpty(rec: string[]): boolean {
  return rec.every((c) => !String(c ?? "").trim());
}

const COMMON_ALIASES = {
  description: ["description", "descricao", "descrição", "nome"],
  latitude: ["latitude", "lat"],
  longitude: ["longitude", "lon", "lng"],
  project_number: ["project_number", "numero_projeto", "projeto", "num_projeto"],
};

async function parseInfraCsv<T>(
  file: File,
  aliases: Record<string, string[]>,
  parseRow: (rec: string[], col: Record<string, number>, line: number) => T | string,
): Promise<{ rows: InfraCsvParsedRow<T>[]; errors: InfraCsvParseError[] }> {
  let text = await file.text();
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
  const lines = text.split(/\r?\n/).filter((l, i, arr) => i < arr.length - 1 || l.trim() !== "");
  if (lines.length === 0) {
    return { rows: [], errors: [{ line: 1, error: "ficheiro vazio" }] };
  }
  const sep = detectSep(lines[0]);
  const headers = parseCsvLine(lines[0], sep);
  const col = columnMap(headers, aliases);
  if (col.description == null) {
    return { rows: [], errors: [{ line: 1, error: "cabeçalho inválido: coluna descricao obrigatória" }] };
  }

  const rows: InfraCsvParsedRow<T>[] = [];
  const errors: InfraCsvParseError[] = [];
  for (let i = 1; i < lines.length; i++) {
    const line = i + 1;
    const rec = parseCsvLine(lines[i], sep);
    if (rowEmpty(rec)) continue;
    const parsed = parseRow(rec, col, line);
    if (typeof parsed === "string") {
      errors.push({ line, description: getCell(rec, col, "description") || undefined, error: parsed });
      continue;
    }
    rows.push({ line, payload: parsed });
  }
  return { rows, errors };
}

export function parseInfraCtoCsv(file: File) {
  return parseInfraCsv<InfraCtoPayload>(file, {
    ...COMMON_ALIASES,
    splitter: ["splitter"],
    transmitter: ["transmitter", "transmissor"],
    fiber_color: ["fiber_color", "cor_fibra", "cor"],
    locality_name: ["locality_name", "localidade"],
    needs_maintenance: ["needs_maintenance", "manutencao", "manutenção"],
    notes: ["notes", "observacoes", "observações", "obs"],
  }, (rec, col) => {
    const description = getCell(rec, col, "description");
    if (!description) return "descricao obrigatória";
    const coords = parseCoords(rec, col);
    if (coords.err) return coords.err;
    const fcRaw = getCell(rec, col, "fiber_color");
    const fiber_color = fcRaw ? normFiberColor(fcRaw) : null;
    if (fcRaw && !fiber_color) return `cor_fibra inválida (${fcRaw})`;
    return {
      description,
      latitude: coords.lat,
      longitude: coords.lon,
      splitter: normalizeSplitterInput(getCell(rec, col, "splitter") || "") || null,
      transmitter: getCell(rec, col, "transmitter") || null,
      fiber_color,
      locality_name: getCell(rec, col, "locality_name") || null,
      project_number: parseOptInt(getCell(rec, col, "project_number")),
      needs_maintenance: parseBool(getCell(rec, col, "needs_maintenance")),
      notes: getCell(rec, col, "notes") || null,
    };
  });
}

export function parseInfraSpliceCsv(file: File) {
  return parseInfraCsv<InfraSplicePayload>(file, {
    ...COMMON_ALIASES,
    fiber_count: ["fiber_count", "fibras", "quantidade_fibras"],
    needs_maintenance: ["needs_maintenance", "manutencao", "manutenção"],
    notes: ["notes", "observacoes", "observações"],
  }, (rec, col) => {
    const description = getCell(rec, col, "description");
    if (!description) return "descricao obrigatória";
    const coords = parseCoords(rec, col);
    if (coords.err) return coords.err;
    const fcS = getCell(rec, col, "fiber_count");
    let fiber_count: number | null = null;
    if (fcS) {
      const n = Number(fcS);
      if (!Number.isFinite(n) || n < 0) return "fibras inválidas";
      fiber_count = n;
    }
    return {
      description,
      latitude: coords.lat,
      longitude: coords.lon,
      fiber_count,
      needs_maintenance: parseBool(getCell(rec, col, "needs_maintenance")),
      notes: getCell(rec, col, "notes") || null,
      project_number: parseOptInt(getCell(rec, col, "project_number")),
    };
  });
}

export function parseInfraCableCsv(file: File) {
  return parseInfraCsv<InfraCablePayload>(file, {
    ...COMMON_ALIASES,
    cable_type: ["cable_type", "tipo"],
    fiber_count: ["fiber_count", "fibras"],
    status: ["status", "estado"],
  }, (rec, col) => {
    const description = getCell(rec, col, "description");
    if (!description) return "descricao obrigatória";
    const coords = parseCoords(rec, col);
    if (coords.err) return coords.err;
    const statusRaw = getCell(rec, col, "status");
    const status = normCableStatus(statusRaw);
    if (!status) return `status inválido (${statusRaw})`;
    const fcS = getCell(rec, col, "fiber_count");
    let fiber_count: number | null = null;
    if (fcS) {
      const n = Number(fcS);
      if (!Number.isFinite(n) || n < 0) return "fibras inválidas";
      fiber_count = n;
    }
    return {
      description,
      cable_type: getCell(rec, col, "cable_type") || null,
      fiber_count,
      status,
      latitude: coords.lat,
      longitude: coords.lon,
      project_number: parseOptInt(getCell(rec, col, "project_number")),
    };
  });
}

export function parseInfraPoleCsv(file: File) {
  return parseInfraCsv<InfraPolePayload>(file, {
    ...COMMON_ALIASES,
    pole_type: ["pole_type", "tipo"],
    locality_name: ["locality_name", "localidade"],
  }, (rec, col) => {
    const description = getCell(rec, col, "description");
    if (!description) return "descricao obrigatória";
    const coords = parseCoords(rec, col);
    if (coords.err) return coords.err;
    return {
      description,
      pole_type: getCell(rec, col, "pole_type") || null,
      locality_name: getCell(rec, col, "locality_name") || null,
      latitude: coords.lat,
      longitude: coords.lon,
      project_number: parseOptInt(getCell(rec, col, "project_number")),
    };
  });
}

export function parseInfraCsvFile(variant: InfraVariant, file: File) {
  switch (variant) {
    case "cto":
      return parseInfraCtoCsv(file);
    case "splice":
      return parseInfraSpliceCsv(file);
    case "cable":
      return parseInfraCableCsv(file);
    case "pole":
      return parseInfraPoleCsv(file);
  }
}
