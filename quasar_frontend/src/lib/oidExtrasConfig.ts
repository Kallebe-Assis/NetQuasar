/** Tipos e helpers partilhados para OIDs SNMP extra por categoria. */

export type OidArrayKey =
  | "brand_oids"
  | "model_oids"
  | "serial_oids"
  | "software_oids"
  | "hardware_oids"
  | "sysname_oids"
  | "sysdescr_oids"
  | "interface_oids"
  | "traffic_oids"
  | "optical_oids"
  | "pon_oids"
  | "onu_oids"
  | "bridge_oids"
  | "custom_oids";

export type OidExtraKind =
  | "brand"
  | "model"
  | "serial"
  | "software"
  | "hardware"
  | "sysname"
  | "sysdescr"
  | "interface"
  | "traffic"
  | "optical"
  | "pon"
  | "onu"
  | "bridge"
  | "custom";

export type OidKindMeta = { value: OidExtraKind; label: string; jsonKey: OidArrayKey; defaultLabel: string };

export type ExtraOidRow = { id: string; kind: OidExtraKind; oid: string; label: string };

export type CategoryOverrides = {
  brand_oids?: string[];
  model_oids?: string[];
  serial_oids?: string[];
  software_oids?: string[];
  hardware_oids?: string[];
  sysname_oids?: string[];
  sysdescr_oids?: string[];
  interface_oids?: string[];
  optical_oids?: string[];
  pon_oids?: string[];
  onu_oids?: string[];
  bridge_oids?: string[];
  traffic_oids?: string[];
  custom_oids?: string[];
  oid_labels?: Record<string, string>;
};

export const OID_KIND_GROUPS: { label: string; kinds: OidKindMeta[] }[] = [
  {
    label: "Inventário / identificação",
    kinds: [
      { value: "brand", label: "Fabricante / marca", jsonKey: "brand_oids", defaultLabel: "Fabricante" },
      { value: "model", label: "Modelo", jsonKey: "model_oids", defaultLabel: "Modelo" },
      { value: "serial", label: "Número de série", jsonKey: "serial_oids", defaultLabel: "Número de série" },
      { value: "software", label: "Versão de software / firmware", jsonKey: "software_oids", defaultLabel: "Versão de software" },
      { value: "hardware", label: "Versão de hardware", jsonKey: "hardware_oids", defaultLabel: "Versão de hardware" },
      { value: "sysname", label: "Nome do sistema (sysName)", jsonKey: "sysname_oids", defaultLabel: "Nome do sistema" },
      { value: "sysdescr", label: "Descrição do sistema (sysDescr)", jsonKey: "sysdescr_oids", defaultLabel: "Descrição do sistema" },
    ],
  },
  {
    label: "Rede / telemetria",
    kinds: [
      { value: "interface", label: "Interface", jsonKey: "interface_oids", defaultLabel: "Interface" },
      { value: "traffic", label: "Tráfego (banda RX/TX etc.)", jsonKey: "traffic_oids", defaultLabel: "Tráfego" },
      { value: "optical", label: "Óptica / SFP", jsonKey: "optical_oids", defaultLabel: "Óptica" },
      { value: "pon", label: "PON", jsonKey: "pon_oids", defaultLabel: "PON" },
      { value: "onu", label: "ONU", jsonKey: "onu_oids", defaultLabel: "ONU" },
      { value: "bridge", label: "Bridge", jsonKey: "bridge_oids", defaultLabel: "Bridge" },
    ],
  },
  {
    label: "Outros",
    kinds: [{ value: "custom", label: "Outro / personalizado", jsonKey: "custom_oids", defaultLabel: "Leitura extra" }],
  },
];

export const OID_KIND_META: OidKindMeta[] = OID_KIND_GROUPS.flatMap((g) => g.kinds);
export const OID_KIND_BY_VALUE = Object.fromEntries(OID_KIND_META.map((k) => [k.value, k])) as Record<
  OidExtraKind,
  OidKindMeta
>;

export function newOidRowId(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `oid-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}

function compact(arr: Array<string | undefined | null>): string[] {
  return arr.map((s) => String(s ?? "").trim()).filter((s) => s.length > 0);
}

export function oidLabelMapFromUnknown(raw: unknown): Record<string, string> {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    const kk = String(k ?? "")
      .trim()
      .replace(/^\./, "");
    const vv = String(v ?? "").trim();
    if (kk && vv) out[kk] = vv;
  }
  return out;
}

export function bngExtraRowsFromBlock(block: CategoryOverrides | undefined): ExtraOidRow[] {
  const labels = oidLabelMapFromUnknown(block?.oid_labels);
  const rows: ExtraOidRow[] = [];
  for (const meta of OID_KIND_META) {
    for (const oid of block?.[meta.jsonKey] ?? []) {
      const o = String(oid).trim();
      if (!o) continue;
      const norm = o.replace(/^\./, "");
      rows.push({
        id: newOidRowId(),
        kind: meta.value,
        oid: o,
        label: String(labels[norm] ?? labels[o] ?? "").trim(),
      });
    }
  }
  return rows;
}

function mergeOidsByKind(rows: ExtraOidRow[]): Record<OidExtraKind, string[]> {
  const acc = Object.fromEntries(OID_KIND_META.map((m) => [m.value, [] as string[]])) as Record<OidExtraKind, string[]>;
  const seen = Object.fromEntries(OID_KIND_META.map((m) => [m.value, new Set<string>()])) as Record<
    OidExtraKind,
    Set<string>
  >;
  for (const r of rows) {
    const o = String(r.oid ?? "").trim();
    if (!o) continue;
    if (seen[r.kind].has(o)) continue;
    seen[r.kind].add(o);
    acc[r.kind].push(o);
  }
  return acc;
}

export function buildBngOverridesFromRows(
  rows: ExtraOidRow[],
  baseline?: CategoryOverrides,
): CategoryOverrides {
  const merged = mergeOidsByKind(rows);
  const block: CategoryOverrides = {};
  for (const meta of OID_KIND_META) {
    const combined = compact(merged[meta.value]);
    if (combined.length) (block as Record<string, unknown>)[meta.jsonKey] = combined;
  }
  const labels: Record<string, string> = { ...oidLabelMapFromUnknown(baseline?.oid_labels) };
  for (const r of rows) {
    const o = String(r.oid ?? "")
      .trim()
      .replace(/^\./, "");
    const lbl = String(r.label ?? "").trim();
    if (!o) continue;
    if (lbl) labels[o] = lbl;
    else delete labels[o];
  }
  const allowed = new Set<string>();
  for (const meta of OID_KIND_META) {
    for (const o of block[meta.jsonKey] ?? []) {
      allowed.add(String(o).trim().replace(/^\./, ""));
    }
  }
  const pruned: Record<string, string> = {};
  for (const [k, v] of Object.entries(labels)) {
    if (allowed.has(k.replace(/^\./, ""))) pruned[k.replace(/^\./, "")] = v;
  }
  if (Object.keys(pruned).length) block.oid_labels = pruned;
  return block;
}
