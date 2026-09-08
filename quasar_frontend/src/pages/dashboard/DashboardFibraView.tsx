import { Bar, BarChart, CartesianGrid, Legend, Tooltip, XAxis, YAxis } from "recharts";
import type { CtoPortsSummary, OltCapacity, OltOnu } from "./dashboardShared";
import { ChartBox, Section, fmtInt, tooltipStyle, trunc } from "./dashboardShared";
import { ctoOccupancyColor } from "../../lib/ctoPorts";

type OltOnuBarRow = { name: string; Online: number; Offline: number; Total: number; brand: string };

export function DashboardFibraView({
  oltOnuBar,
  oltOnuByDevice,
  oltFleetTotals,
  capacity,
  capacityError,
  ctoPorts,
}: {
  oltOnuBar: OltOnuBarRow[];
  oltOnuByDevice?: OltOnu[];
  oltFleetTotals: { total: number; online: number; offline: number };
  capacity?: OltCapacity;
  capacityError: string | null;
  ctoPorts?: CtoPortsSummary;
}) {
  const ctoPortsTotal = ctoPorts?.ports_total ?? 0;
  const ctoPortsUsed = ctoPorts?.ports_used ?? 0;
  const ctoPortsFree = ctoPorts?.ports_free ?? 0;
  const ctoOccupancyPct = ctoPortsTotal > 0 ? (ctoPortsUsed / ctoPortsTotal) * 100 : 0;
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

      <Section
        id="sec-cto-ports"
        title="Portas de CTO"
        subtitle="Ocupação dos splitters cadastrados (aba Elementos → CTOs → esquema de fibras). Cadastre o status de cada porta para estes números ficarem completos."
      >
        {(ctoPorts?.ctos_with_ports ?? 0) === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 13 }}>
            Nenhuma CTO com status de porta cadastrado ainda ({fmtInt(ctoPorts?.total_ctos)} CTO(s) no total).
          </p>
        ) : (
          <>
            <div className="row" style={{ gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
              <div className="stat" style={{ minWidth: 150 }}>
                <div className="stat__k">CTOs com portas cadastradas</div>
                <div className="stat__v">
                  {fmtInt(ctoPorts?.ctos_with_ports)} / {fmtInt(ctoPorts?.total_ctos)}
                </div>
              </div>
              <div className="stat" style={{ minWidth: 120 }}>
                <div className="stat__k">Portas ocupadas</div>
                <div className="stat__v">{fmtInt(ctoPortsUsed)}</div>
              </div>
              <div className="stat" style={{ minWidth: 120 }}>
                <div className="stat__k">Portas livres</div>
                <div className="stat__v">{fmtInt(ctoPortsFree)}</div>
              </div>
              <div className="stat" style={{ minWidth: 140 }}>
                <div className="stat__k">Ocupação geral</div>
                <div className="stat__v">{ctoOccupancyPct.toFixed(1)}%</div>
              </div>
            </div>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>CTOs mais próximas da capacidade máxima:</p>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>CTO</th>
                    <th className="mono">Ocupadas</th>
                    <th className="mono">Livres</th>
                    <th className="mono">Total</th>
                    <th className="mono">% uso</th>
                  </tr>
                </thead>
                <tbody>
                  {(ctoPorts?.top_occupied ?? []).map((c) => {
                    const total = c.ports_total ?? 0;
                    const used = c.ports_used ?? 0;
                    const pct = total > 0 ? (used / total) * 100 : 0;
                    return (
                      <tr key={c.id}>
                        <td>
                          #{c.display_number} — {c.description}
                        </td>
                        <td className="mono">{fmtInt(used)}</td>
                        <td className="mono">{fmtInt(c.ports_free)}</td>
                        <td className="mono">{fmtInt(total)}</td>
                        <td className="mono">
                          <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                            <span
                              aria-hidden
                              style={{ width: 8, height: 8, borderRadius: "50%", background: ctoOccupancyColor(c), display: "inline-block" }}
                            />
                            {pct.toFixed(1)}%
                          </span>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Section>
    </>
  );
}
