export type MikrotikCatalogEntry = {
  key: string;
  section: string;
  label: string;
  unit?: string;
  default_divisor?: number;
};

export type MikrotikMetricConfig = {
  enabled?: boolean;
  oid?: string;
  collect_mode?: string;
  value_divisor?: number;
};

export type MikrotikMetricCard =
  | { kind: "scalar"; key: string; label: string; section: string; value: string; error?: string; ok: boolean }
  | { kind: "optical"; key: string; label: string; section: string; ports: Array<Record<string, unknown>>; ok: boolean; error?: string }
  | { kind: "interface_status"; key: string; label: string; section: string; rows: Array<Record<string, unknown>>; ok: boolean; error?: string }
  | { kind: "pppoe"; key: string; label: string; section: string; sessions: Array<Record<string, unknown>>; ok: boolean; error?: string }
  | { kind: "count"; key: string; label: string; section: string; count: number; note?: string; ok: boolean; error?: string };

export function stripPPPoEName(name: string): string {
  return name.replace(/^<pppoe-?/i, "").replace(/>$/g, "").trim() || name;
}
