import { useEffect, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";

type DebugSection = {
  id: string;
  title: string;
  oid_root: string;
  row_count: number;
  stats?: Record<string, unknown>;
  rows: Record<string, unknown>[];
};

type SnmpDebugPayload = {
  generated_at?: string;
  host?: string;
  walk_truncated?: boolean;
  walk_note?: string;
  sections?: DebugSection[];
  final_pons?: Record<string, unknown>[];
  if_mib_meta?: Record<string, unknown>;
};

function fmtCell(v: unknown): string {
  if (v == null) return "—";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

function DebugTable({ rows }: { rows: Record<string, unknown>[] }) {
  const cols = useMemo(() => {
    const set = new Set<string>();
    for (const r of rows.slice(0, 50)) {
      Object.keys(r).forEach((k) => set.add(k));
    }
    return [...set];
  }, [rows]);
  if (rows.length === 0) return <p style={{ fontSize: 12, color: "var(--muted)" }}>Sem linhas nesta secção.</p>;
  return (
    <div className="table-wrap" style={{ maxHeight: 280, overflow: "auto" }}>
      <table style={{ fontSize: 10 }}>
        <thead>
          <tr>
            {cols.map((c) => (
              <th key={c}>{c}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              {cols.map((c) => (
                <td key={c} className="mono">
                  {fmtCell(r[c])}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length > 50 && (
        <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 6 }}>Mostrando 50 de {rows.length} linhas.</p>
      )}
    </div>
  );
}

type Props = {
  deviceId: string;
  initialDebug?: SnmpDebugPayload | null;
  open: boolean;
  onClose: () => void;
};

export function OltSnmpDebugPanel({ deviceId, initialDebug, open, onClose }: Props) {
  const [debug, setDebug] = useState<SnmpDebugPayload | null>(initialDebug ?? null);
  const [fromSnap, setFromSnap] = useState(!!initialDebug);
  const [snapHint, setSnapHint] = useState<string | null>(null);

  useEffect(() => {
    if (initialDebug) {
      setDebug(initialDebug);
      setFromSnap(true);
    }
  }, [initialDebug]);

  const collect = useMutation({
    mutationFn: () =>
      apiFetch<{ snmp_debug: SnmpDebugPayload; live_collection?: boolean; message?: string }>(
        `/api/v1/olt/devices/${deviceId}/snmp-debug`,
        {
          method: "POST",
          json: {},
          timeoutMs: 15 * 60 * 1000,
        },
      ),
    onSuccess: (data) => {
      setDebug(data.snmp_debug ?? null);
      setFromSnap(false);
      setSnapHint(data.message ?? null);
    },
  });

  const loadSnap = useMutation({
    mutationFn: () =>
      apiFetch<{ snmp_debug: SnmpDebugPayload | null; message?: string; snapshot_summary_keys?: string[] }>(
        `/api/v1/olt/devices/${deviceId}/snmp-debug`,
        { timeoutMs: 120_000 },
      ),
    onSuccess: (data) => {
      setDebug(data.snmp_debug ?? null);
      setFromSnap(true);
      setSnapHint(
        data.snmp_debug
          ? null
          : data.message ??
              (data.snapshot_summary_keys?.length
                ? `Snapshot sem snmp_debug (chaves: ${data.snapshot_summary_keys.join(", ")}). Faça «Atualizar» na OLT.`
                : "Sem snmp_debug no snapshot. Faça «Atualizar» na OLT."),
      );
    },
  });

  if (!open) return null;

  const sections = debug?.sections ?? [];

  return (
    <div
      className="card"
      style={{
        marginTop: 16,
        border: "1px solid var(--border)",
        maxHeight: "85vh",
        overflow: "auto",
      }}
    >
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <div>
          <h2 style={{ margin: 0 }}>Debug SNMP (VSOL)</h2>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>
            Coleta via <code className="mono">snmpwalk</code> na tabela <code className="mono">gOnuAuthList</code>. Online/offline: <code className="mono">4.1.8</code> (1=on; 0 ou 2=off).
          </p>
        </div>
        <div className="row" style={{ gap: 8 }}>
          <button type="button" className="btn" disabled={collect.isPending} onClick={() => collect.mutate()}>
            {collect.isPending ? "Coletando…" : "Coletar agora"}
          </button>
          <button type="button" className="btn" disabled={loadSnap.isPending} onClick={() => loadSnap.mutate()}>
            Último snapshot
          </button>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>

      {collect.isError && <div className="msg msg--err">{(collect.error as Error).message}</div>}

      {debug && (
        <div style={{ marginTop: 12, fontSize: 12 }}>
          <p>
            <strong>Host:</strong> {debug.host ?? "—"} · <strong>Gerado:</strong> {debug.generated_at ?? "—"}
            {fromSnap ? " (snapshot)" : " (ao vivo)"}
            {debug.walk_truncated ? (
              <span className="msg msg--off" style={{ marginLeft: 8 }}>
                Walk truncado
              </span>
            ) : null}
          </p>
          {debug.walk_note ? (
            <p className="msg msg--off" style={{ fontSize: 11 }}>
              Nota: {String(debug.walk_note)}
            </p>
          ) : null}
        </div>
      )}

      {snapHint && (
        <p className="msg msg--off" style={{ fontSize: 12, marginTop: 12 }}>
          {snapHint}
        </p>
      )}

      {!debug && !collect.isPending && !loadSnap.isPending && (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 12 }}>
          Sem dados de debug. Clique em «Coletar agora» (snmpwalk na OLT) ou «Último snapshot» após «Atualizar» na OLT.
        </p>
      )}

      {sections.map((sec) => (
        <details
          key={sec.id}
          className="collapsible-section"
          style={{ marginTop: 12 }}
          open={sec.id === "passos" || sec.id === "onu_online" || sec.id === "merge_counts"}
        >
          <summary>
            {sec.title}{" "}
            <span style={{ fontWeight: 400, color: "var(--muted)" }}>
              ({sec.row_count} vars · {sec.oid_root})
            </span>
          </summary>
          <div className="collapsible-section__body">
            {sec.stats && (
              <pre
                className="mono"
                style={{
                  fontSize: 10,
                  background: "var(--bg-subtle)",
                  padding: 8,
                  borderRadius: 6,
                  overflow: "auto",
                  maxHeight: 120,
                }}
              >
                {JSON.stringify(sec.stats, null, 2)}
              </pre>
            )}
            <DebugTable rows={sec.rows ?? []} />
          </div>
        </details>
      ))}

      {debug?.final_pons && debug.final_pons.length > 0 && (
        <details className="collapsible-section" style={{ marginTop: 12 }} open>
          <summary>
            PONs finais (merge IF total + 4.1.8 on/off){" "}
            <span style={{ fontWeight: 400, color: "var(--muted)" }}>({debug.final_pons.length})</span>
          </summary>
          <div className="collapsible-section__body">
            <DebugTable rows={debug.final_pons as Record<string, unknown>[]} />
          </div>
        </details>
      )}
    </div>
  );
}
