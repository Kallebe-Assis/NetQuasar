import { Bar, BarChart, CartesianGrid, Legend, Tooltip, XAxis, YAxis } from "recharts";
import type { OltCapacity, OltOnu } from "./dashboardShared";
import { ChartBox, Section, fmtInt, tooltipStyle, trunc } from "./dashboardShared";

type OltOnuBarRow = { name: string; Online: number; Offline: number; Total: number; brand: string };

export function DashboardFibraView({
  oltOnuBar,
  oltOnuByDevice,
  oltFleetTotals,
  capacity,
  capacityError,
}: {
  oltOnuBar: OltOnuBarRow[];
  oltOnuByDevice?: OltOnu[];
  oltFleetTotals: { total: number; online: number; offline: number };
  capacity?: OltCapacity;
  capacityError: string | null;
}) {
  return (
    <>
      <Section
        id="sec-olt"
        title="ONUs por OLT (snapshot)"
        subtitle="OLTs em operação Ativo: soma onu_total / onu_online / onu_offline nas PONs do último snapshot."
      >
        {oltOnuBar.length === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 13 }}>Sem snapshots OLT. Associe equipamentos OLT e execute refresh de dados OLT.</p>
        ) : (
          <>
            <div className="row" style={{ gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
              <div className="stat" style={{ minWidth: 140 }}>
                <div className="stat__k">ONUs total (todas as OLTs)</div>
                <div className="stat__v">{fmtInt(oltFleetTotals.total)}</div>
              </div>
              <div className="stat" style={{ minWidth: 120 }}>
                <div className="stat__k">Online</div>
                <div className="stat__v">{fmtInt(oltFleetTotals.online)}</div>
              </div>
              <div className="stat" style={{ minWidth: 120 }}>
                <div className="stat__k">Offline</div>
                <div className="stat__v">{fmtInt(oltFleetTotals.offline)}</div>
              </div>
            </div>
            <ChartBox h={300}>
              <BarChart data={oltOnuBar} margin={{ left: 8, right: 8, top: 12, bottom: 52 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-28} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip
                  contentStyle={tooltipStyle} itemStyle={tooltipStyle} labelStyle={tooltipStyle}
                  formatter={(v: number, name: string) => [`${fmtInt(v)}`, name]}
                  labelFormatter={(label, p) => {
                    const b = (p as { payload?: { brand?: string } })?.payload?.brand;
                    return b ? `${label} (${b})` : String(label);
                  }}
                />
                <Bar dataKey="Online" stackId="onu" fill="#3fb950" radius={[0, 0, 0, 0]} />
                <Bar dataKey="Offline" stackId="onu" fill="#f85149" radius={[4, 4, 0, 0]} />
                <Legend />
              </BarChart>
            </ChartBox>
            <div className="table-wrap" style={{ marginTop: 10 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>OLT</th>
                    <th>Marca</th>
                    <th className="mono">Total</th>
                    <th className="mono">Online</th>
                    <th className="mono">Offline</th>
                    <th>Snapshot</th>
                  </tr>
                </thead>
                <tbody>
                  {(oltOnuByDevice ?? []).map((r) => (
                    <tr key={r.device_id}>
                      <td>{r.description}</td>
                      <td>{r.brand ?? "—"}</td>
                      <td className="mono">{fmtInt(r.onu_count)}</td>
                      <td className="mono">{fmtInt(r.onu_online)}</td>
                      <td className="mono">{fmtInt(r.onu_offline)}</td>
                      <td className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                        {r.snapshot_at ? new Date(r.snapshot_at).toLocaleString("pt-PT") : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Section>

      <Section
        id="sec-olt-capacity"
        title="Capacidade OLT por PON"
        subtitle="Percentual de ocupação por PON (base 128 ONUs/PON) e tendência total de ONUs nos últimos 7 dias."
      >
        {capacityError && <div className="msg msg--err">{capacityError}</div>}
        {capacity && (
          <>
            <ChartBox h={280}>
              <BarChart
                data={(capacity.pon_rows ?? [])
                  .slice(0, 20)
                  .map((p) => ({ name: `${trunc(p.olt, 12)}:${p.pon_id}`, "% uso": Number(p.usage_percent ?? 0) }))}
                margin={{ left: 8, right: 8, top: 12, bottom: 52 }}
              >
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-28} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} />
                <Tooltip contentStyle={tooltipStyle} itemStyle={tooltipStyle} labelStyle={tooltipStyle} />
                <Bar dataKey="% uso" fill="#d29922" />
              </BarChart>
            </ChartBox>
            <div className="table-wrap" style={{ marginTop: 10 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>OLT</th>
                    <th>PON</th>
                    <th className="mono">ONU total</th>
                    <th className="mono">% uso</th>
                    <th>Alerta</th>
                  </tr>
                </thead>
                <tbody>
                  {(capacity.pon_rows ?? []).slice(0, 30).map((p, i) => (
                    <tr key={`${p.olt_id}-${p.pon_id}-${i}`}>
                      <td>{p.olt}</td>
                      <td className="mono">{p.pon_id}</td>
                      <td className="mono">{fmtInt(p.onu_total)}</td>
                      <td className="mono">{Number(p.usage_percent ?? 0).toFixed(1)}%</td>
                      <td>{p.near_saturation ? <span className="badge badge--err">próx. saturação</span> : <span className="badge badge--ok">ok</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Section>
    </>
  );
}
