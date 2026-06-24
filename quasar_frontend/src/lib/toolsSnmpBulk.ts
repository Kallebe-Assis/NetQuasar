import { escapeCsvCell } from "./deviceReportHelpers";
import { downloadBlob } from "./api";

export type SnmpBulkCell = { value: string; type?: string };
export type SnmpBulkHostRow = {
  host: string;
  ok: boolean;
  error?: string;
  values: Record<string, SnmpBulkCell>;
};

export type SnmpBulkResult = {
  hosts: SnmpBulkHostRow[];
  oids: string[];
  note: string;
};

export function normalizeSnmpOid(oid: string): string {
  return oid.trim().replace(/^\.+/, "");
}

export function oidColumnTitle(oid: string): string {
  const n = normalizeSnmpOid(oid);
  const parts = n.split(".");
  if (parts.length >= 2) {
    return `…${parts[parts.length - 2]}.${parts[parts.length - 1]}`;
  }
  return n.length > 28 ? `${n.slice(0, 25)}…` : n;
}

export function matchSnmpVar(
  vars: Array<{ oid?: string; type?: string; value?: unknown }>,
  wantedOid: string,
): SnmpBulkCell | undefined {
  const w = normalizeSnmpOid(wantedOid);
  for (const v of vars) {
    const o = normalizeSnmpOid(String(v.oid ?? ""));
    if (o === w) {
      return {
        value: v.value != null ? String(v.value) : "",
        type: v.type != null ? String(v.type) : undefined,
      };
    }
  }
  return undefined;
}

export function buildSnmpBulkResult(args: {
  hosts: string[];
  oids: string[];
  fetchHost: (host: string) => Promise<{ ok?: boolean; error?: string; vars?: Array<{ oid?: string; type?: string; value?: unknown }> }>;
}): Promise<SnmpBulkResult> {
  const { hosts, oids, fetchHost } = args;
  return (async () => {
    const out: SnmpBulkHostRow[] = [];
    for (const host of hosts) {
      try {
        const r = await fetchHost(host);
        if (r.ok === true && Array.isArray(r.vars) && r.vars.length > 0) {
          const values: Record<string, SnmpBulkCell> = {};
          for (const oid of oids) {
            const cell = matchSnmpVar(r.vars, oid);
            if (cell) values[oid] = cell;
          }
          out.push({ host, ok: true, values });
        } else {
          out.push({
            host,
            ok: false,
            error: typeof r.error === "string" && r.error ? r.error : "SNMP bulk-get sem variáveis",
            values: {},
          });
        }
      } catch (e) {
        out.push({
          host,
          ok: false,
          error: e instanceof Error ? e.message : String(e),
          values: {},
        });
      }
    }
    return {
      hosts: out,
      oids,
      note: `Até 50 hosts; uma linha por host e OID na tabela; valores formatados no servidor (ex.: MAC em aa:bb:cc:dd:ee:ff).`,
    };
  })();
}

export type SnmpBulkFlatRow = {
  host: string;
  oid: string;
  value: string;
  snmpType?: string;
  hostOk: boolean;
  hostError?: string;
};

export function flattenSnmpBulkRows(data: SnmpBulkResult): SnmpBulkFlatRow[] {
  const out: SnmpBulkFlatRow[] = [];
  for (const h of data.hosts) {
    if (!h.ok) {
      out.push({
        host: h.host,
        oid: "—",
        value: "—",
        hostOk: false,
        hostError: h.error,
      });
      continue;
    }
    for (const oid of data.oids) {
      const cell = h.values[oid];
      out.push({
        host: h.host,
        oid,
        value: cell?.value?.trim() ? cell.value : "—",
        snmpType: cell?.type,
        hostOk: true,
      });
    }
  }
  return out;
}

export function exportSnmpBulkCsv(data: SnmpBulkResult): void {
  const header = ["Host", "Estado", "OID", "Valor", "Tipo SNMP", "Detalhe"];
  const lines = [header.map(escapeCsvCell).join(",")];
  for (const row of flattenSnmpBulkRows(data)) {
    lines.push(
      [
        row.host,
        row.hostOk ? "OK" : "Erro",
        row.hostOk ? row.oid : "—",
        row.hostOk ? row.value : "—",
        row.snmpType ?? "",
        row.hostError ?? "",
      ]
        .map(escapeCsvCell)
        .join(","),
    );
  }
  const blob = new Blob(["\uFEFF" + lines.join("\r\n")], { type: "text/csv;charset=utf-8" });
  const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-");
  downloadBlob(`snmp_bulk_${stamp}.csv`, blob);
}
