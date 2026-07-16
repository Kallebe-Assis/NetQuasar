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

/** VSOL e outros agents devolvem OctetString como hex com dois-pontos (ex.: 2d:31:39:2e:35:31 → "-19.51").
 * Também descodifica texto Latin-1 (ex.: nomes com acentos gravados como 43:4f:…:c9). */
export function tryDecodeColonHexAscii(raw: string): string | null {
  const s = raw.trim();
  if (!s.includes(":")) return null;
  const parts = s.split(":");
  if (parts.length < 2 || parts.length > 128) return null;
  const bytes: number[] = [];
  for (const p of parts) {
    if (p.length !== 2 || !/^[0-9a-fA-F]{2}$/.test(p)) return null;
    bytes.push(parseInt(p, 16));
  }
  const u8 = Uint8Array.from(bytes);
  const letterCount = (t: string) => {
    let n = 0;
    for (const ch of t) {
      if (/[A-Za-zÀ-ÿ]/.test(ch)) n++;
    }
    return n;
  };
  try {
    const utf8 = new TextDecoder("utf-8", { fatal: true }).decode(u8);
    if (letterCount(utf8) >= 3) return utf8.trim();
  } catch {
    /* fallback Latin-1 */
  }
  const latin1 = new TextDecoder("iso-8859-1").decode(u8);
  if (letterCount(latin1) >= 3) return latin1.trim();
  // ASCII estrito (compatível com métricas numéricas curtas)
  let ascii = "";
  for (const b of bytes) {
    if (b < 32 || b > 126) return null;
    ascii += String.fromCharCode(b);
  }
  ascii = ascii.trim();
  if (!ascii) return null;
  if (/^-?\d+(\.\d+)?$/.test(ascii)) return ascii;
  if (parts.length === 6 && /^[\d.\-]+$/.test(ascii)) return ascii;
  if (parts.length !== 6 && ascii.length > 0) return ascii;
  return null;
}

/** Mostra texto SNMP; se estiver em hex `aa:bb:…`, tenta descodificar. */
export function formatSnmpDisplayText(raw: string | null | undefined): string {
  if (raw == null) return EM_DASH;
  const s = String(raw).trim();
  if (!s) return EM_DASH;
  return tryDecodeColonHexAscii(s) ?? s;
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
