import { EM_DASH } from "../../lib/formatDisplay";

// bgpFormat.tsx — helpers de formatação partilhados entre BgpPage.tsx e as abas em
// pages/bgp/*.tsx (extraídos de BgpPage.tsx para não duplicar em cada aba nova).

export function formatDuration(seconds?: number): string {
  if (!seconds || seconds <= 0) return EM_DASH;
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0 || d > 0) parts.push(`${h}h`);
  parts.push(`${m}m`);
  return parts.join(" ");
}

export function formatDateTime(iso?: string): string {
  if (!iso) return EM_DASH;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return EM_DASH;
  return d.toLocaleString("pt-BR");
}

export function num(v: string | undefined): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

export function peerStateBadge(label?: string) {
  const established = label === "established";
  return <span className={`badge ${established ? "badge--ok" : "badge--err"}`}>{label ?? "desconhecido"}</span>;
}

export function ifaceStatusBadge(operStatus?: string) {
  const up = operStatus === "1";
  return <span className={`badge ${up ? "badge--ok" : "badge--err"}`}>{up ? "Up" : operStatus ? "Down" : EM_DASH}</span>;
}

/** Badge de 3 níveis (normal/aviso/grave-fatal) reutilizado por óptica/ventoinhas/fontes/
 * temperatura/tensão/luz de alarme — todos já vêm do equipamento com um "label" textual em
 * português (normal/aviso/grave/fatal) montado no backend (report_hardware.go). */
export function severityBadge(label?: string) {
  const l = (label ?? "").toLowerCase();
  if (l === "normal") return <span className="badge badge--ok">normal</span>;
  if (l === "aviso") return <span className="badge badge--warn">aviso</span>;
  if (l === "grave" || l === "fatal") return <span className="badge badge--err">{l}</span>;
  return <span className="badge badge--off">{label || EM_DASH}</span>;
}

export { EM_DASH };
