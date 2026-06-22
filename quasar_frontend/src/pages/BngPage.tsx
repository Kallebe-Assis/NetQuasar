import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { formatBitrate } from "../lib/formatBitrate";

type BngSession = {
  device_id: string;
  device_description?: string;
  device_ip?: string;
  login?: string;
  interface_name?: string;
  oper_status?: string;
  in_octets?: number;
  out_octets?: number;
  collected_at?: string;
  source?: string;
};

type BngSummary = {
  bng_devices?: number;
  sessions_total?: number;
  sessions_online?: number;
  sessions_offline?: number;
};

export function BngPage() {
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");

  const summary = useQuery({
    queryKey: ["bng-summary"],
    queryFn: () => apiFetch<BngSummary>("/api/v1/bng/stats/summary"),
    refetchInterval: 30_000,
  });

  const sessions = useQuery({
    queryKey: ["bng-sessions", q],
    queryFn: () =>
      q.trim()
        ? apiFetch<{ sessions: BngSession[] }>(`/api/v1/bng/sessions/search?q=${encodeURIComponent(q.trim())}`)
        : apiFetch<{ sessions: BngSession[]; note?: string }>("/api/v1/bng/sessions"),
    refetchInterval: 30_000,
  });

  const rows = useMemo(() => {
    const list = sessions.data?.sessions ?? [];
    const term = search.trim().toLowerCase();
    if (!term) return list;
    return list.filter((s) => {
      const hay = [s.login, s.interface_name, s.device_description, s.device_ip].join(" ").toLowerCase();
      return hay.includes(term);
    });
  }, [sessions.data?.sessions, search]);

  if (sessions.isLoading && !sessions.data) return <p>A carregar sessões BNG…</p>;
  if (sessions.isError) return <div className="msg msg--err">{(sessions.error as Error).message}</div>;

  const note = !q.trim() && "note" in (sessions.data ?? {}) ? (sessions.data as { note?: string }).note : undefined;

  return (
    <>
      <div className="page-heading">
        <h1>BNG / PPPoE</h1>
        <PageCountPill label="Sessões" count={rows.length} />
      </div>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Sessões PPPoE activas recolhidas via SNMP (MikroTik IF-MIB / telemetria). Apenas equipamentos com o switch BNG activo em Configurações → Equipamentos.
      </p>
      {note && (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>{note}</p>
      )}

      {summary.data && (
        <div className="row" style={{ gap: 12, marginBottom: 16, flexWrap: "wrap" }}>
          <div className="card" style={{ padding: "10px 14px", minWidth: 120 }}>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>Equipamentos BNG</div>
            <strong>{summary.data.bng_devices ?? 0}</strong>
          </div>
          <div className="card" style={{ padding: "10px 14px", minWidth: 120 }}>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>Sessões online</div>
            <strong>{summary.data.sessions_online ?? 0}</strong>
          </div>
          <div className="card" style={{ padding: "10px 14px", minWidth: 120 }}>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>Total detectado</div>
            <strong>{summary.data.sessions_total ?? 0}</strong>
          </div>
        </div>
      )}

      <div className="row" style={{ marginBottom: 12, gap: 8, flexWrap: "wrap" }}>
        <input
          className="input"
          style={{ maxWidth: 280 }}
          placeholder="Pesquisar login ou interface…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <input
          className="input"
          style={{ maxWidth: 220 }}
          placeholder="API search (q)…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <button type="button" className="btn" onClick={() => sessions.refetch()}>
          Actualizar
        </button>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Login</th>
              <th>Interface</th>
              <th>Equipamento</th>
              <th>IP</th>
              <th>Status</th>
              <th>Tráfego ↓</th>
              <th>Tráfego ↑</th>
              <th>Colecta</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={8} style={{ color: "var(--muted)" }}>
                  Nenhuma sessão PPPoE activa encontrada. Verifique monitoramento full e telemetria MikroTik nos concentradores.
                </td>
              </tr>
            ) : (
              rows.map((s, i) => (
                <tr key={`${s.device_id}-${s.interface_name}-${i}`}>
                  <td className="mono">{s.login || "—"}</td>
                  <td className="mono" style={{ maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis" }}>
                    {s.interface_name || "—"}
                  </td>
                  <td>{s.device_description || "—"}</td>
                  <td className="mono">{s.device_ip || "—"}</td>
                  <td>{s.oper_status || "—"}</td>
                  <td className="mono">{s.in_octets != null ? formatBitrate((s.in_octets * 8) / 300) : "—"}</td>
                  <td className="mono">{s.out_octets != null ? formatBitrate((s.out_octets * 8) / 300) : "—"}</td>
                  <td className="mono" style={{ whiteSpace: "nowrap", fontSize: 11 }}>
                    {s.collected_at ? new Date(s.collected_at).toLocaleString("pt-BR") : "—"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
