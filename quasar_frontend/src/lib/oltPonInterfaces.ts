export type OltPonCatalog = {
  id: string;
  description?: string | null;
  max_pons?: number | null;
  pon_descriptions?: Record<string, string> | null;
  pon_vlans?: Record<string, number | string> | null;
};

export type OltPonChoice = {
  pon: number;
  description: string;
  vlan: string;
  label: string;
};

export function oltDisplayName(olt: OltPonCatalog): string {
  return String(olt.description ?? "").trim() || olt.id;
}

export function matchOltByTransmitter(olts: OltPonCatalog[], transmitter?: string | null, oltDeviceId?: string | null) {
  if (oltDeviceId) {
    const byId = olts.find((o) => o.id === oltDeviceId);
    if (byId) return byId;
  }
  const t = String(transmitter ?? "").trim().toLowerCase();
  if (!t) return undefined;
  return olts.find((o) => oltDisplayName(o).toLowerCase() === t);
}

function vlanOf(olt: OltPonCatalog, pon: number): string {
  const raw = olt.pon_vlans?.[String(pon)];
  if (raw == null || raw === "") return "";
  return String(raw);
}

function descOf(olt: OltPonCatalog, pon: number): string {
  return String(olt.pon_descriptions?.[String(pon)] ?? "").trim();
}

export function formatOltPonLabel(pon: number, description: string, vlan: string): string {
  const parts = [`PON ${pon}`];
  if (description) parts.push(description);
  if (vlan) parts.push(`VLAN ${vlan}`);
  return parts.join(" — ");
}

export function buildOltPonChoices(olt: OltPonCatalog | undefined): OltPonChoice[] {
  if (!olt) return [];
  const max = Number(olt.max_pons);
  const fromMax = Number.isFinite(max) && max > 0 ? max : 0;
  const keys = new Set<number>();
  for (const k of Object.keys(olt.pon_descriptions ?? {})) {
    const n = Number(k);
    if (Number.isFinite(n) && n > 0) keys.add(n);
  }
  for (const k of Object.keys(olt.pon_vlans ?? {})) {
    const n = Number(k);
    if (Number.isFinite(n) && n > 0) keys.add(n);
  }
  const count = fromMax > 0 ? fromMax : keys.size > 0 ? Math.max(...keys) : 0;
  if (count <= 0) return [];
  const out: OltPonChoice[] = [];
  for (let pon = 1; pon <= count; pon++) {
    const description = descOf(olt, pon);
    const vlan = vlanOf(olt, pon);
    out.push({
      pon,
      description,
      vlan,
      label: formatOltPonLabel(pon, description, vlan),
    });
  }
  return out;
}
