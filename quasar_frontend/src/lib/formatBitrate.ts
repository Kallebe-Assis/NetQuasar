import { EM_DASH } from "./formatDisplay";

/** `short`: bps, Kbps… · `perSecond`: b/s, Kb/s… (relatórios / interfaces) */
export type BitrateUnitStyle = "short" | "perSecond";

const UNIT_SETS: Record<BitrateUnitStyle, string[]> = {
  short: ["bps", "Kbps", "Mbps", "Gbps", "Tbps"],
  perSecond: ["b/s", "Kb/s", "Mb/s", "Gb/s", "Tb/s"],
};

export function formatBitrate(bps: number | null | undefined, style: BitrateUnitStyle = "short"): string {
  if (bps == null || !Number.isFinite(Number(bps)) || Number(bps) < 0) return EM_DASH;
  const units = UNIT_SETS[style];
  let v = Number(bps);
  let i = 0;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  const digits = v >= 100 ? 0 : v >= 10 ? 1 : 2;
  return `${v.toFixed(digits)} ${units[i]}`;
}
