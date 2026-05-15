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
