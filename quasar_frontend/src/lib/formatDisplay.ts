/** Travessão para células vazias (U+2014). Usar sempre esta constante para evitar problemas de encoding. */
export const EM_DASH = "\u2014";

export function formatNullable(value: unknown): string {
  if (value == null) return EM_DASH;
  if (typeof value === "number") {
    if (!Number.isFinite(value)) return EM_DASH;
    return String(value);
  }
  const s = String(value).trim();
  return s || EM_DASH;
}

export function formatNum(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(Number(n))) return EM_DASH;
  return String(Math.round(Number(n)));
}

export function format1f(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(Number(n))) return EM_DASH;
  return Number(n).toFixed(1);
}

export function formatDbm(n: number | null | undefined, digits = 3): string {
  if (n == null || !Number.isFinite(Number(n))) return EM_DASH;
  return `${Number(n).toFixed(digits)} dBm`;
}

/** VSOL e outros agents devolvem OctetString como hex com dois-pontos (ex.: 2d:31:39:2e:35:31 → "-19.51"). */
export function tryDecodeColonHexAscii(raw: string): string | null {
  const s = raw.trim();
  if (!s.includes(":")) return null;
  const parts = s.split(":");
  if (parts.length < 2) return null;
  const bytes: number[] = [];
  for (const p of parts) {
    if (p.length !== 2 || !/^[0-9a-fA-F]{2}$/.test(p)) return null;
    bytes.push(parseInt(p, 16));
  }
  let out = "";
  for (const b of bytes) {
    if (b < 32 || b > 126) return null;
    out += String.fromCharCode(b);
  }
  out = out.trim();
  if (!out) return null;
  if (/^-?\d+(\.\d+)?$/.test(out)) return out;
  if (parts.length === 6 && /^[\d.\-]+$/.test(out)) return out;
  if (parts.length !== 6 && out.length > 0) return out;
  return null;
}

/** Valor SNMP para células (RX/TX/temp) — decodifica hex ASCII quando aplicável. */
export function formatSnmpMetricCell(value: unknown): string {
  if (value == null) return EM_DASH;
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  const s = String(value).trim();
  if (!s) return EM_DASH;
  const decoded = tryDecodeColonHexAscii(s);
  if (decoded != null) return decoded;
  return s;
}
