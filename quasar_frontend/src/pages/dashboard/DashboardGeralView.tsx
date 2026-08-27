import { Cell, Legend, Pie, PieChart, Tooltip } from "recharts";
import { displayAlertType } from "../../lib/alertLabels";
import type { DashboardAnalytics, DashboardTotals } from "./dashboardShared";
import { CHART_COLORS, ChartBox, Section, fmtInt, tooltipStyle } from "./dashboardShared";

type AlertsPieRow = { name: string; value: number; code: string };

export function DashboardGeralView({
  totals,
  telemetrySamples,
  alertsOpen,
  alertsByType,
  alertsPie,
}: {
  totals?: DashboardTotals;
  telemetrySamples?: number;
  alertsOpen?: number;
  alertsByType?: DashboardAnalytics["alerts_by_type_30d"];
  alertsPie: AlertsPieRow[];
}) {
  return (
    <>
      <div className="dashboard-kpi-row">
        <div className="stat">
          <div className="stat__k">Equipamentos</div>
          <div className="stat__v">{fmtInt(totals?.devices)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">POPs</div>
          <div className="stat__v">{fmtInt(totals?.pops)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Comercial (clientes, mês actual UTC)</div>
          <div className="stat__v">{fmtInt(totals?.commercial_clients_sum)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Ping ligado</div>
          <div className="stat__v">{fmtInt(totals?.ping_enabled_devices)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Telemetria ligada</div>
          <div className="stat__v">{fmtInt(totals?.telemetry_enabled_devices)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Monitorização</div>
          <div className="stat__v" style={{ fontSize: 14 }}>
            {totals?.monitoring_running ? <span className="badge badge--ok">A correr</span> : <span className="badge badge--off">Parada</span>}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Amostras telemetria</div>
          <div className="stat__v">{fmtInt(telemetrySamples)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Alertas abertos</div>
          <div className="stat__v">{fmtInt(alertsOpen)}</div>
        </div>
      </div>

      <Section
        id="sec-alertas"
        title="Alertas na janela"
        subtitle="Distribuição por tipo de alerta (equipamentos Ativos, active_since no período). O cartão de KPI mostra alertas ainda abertos."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 16 }}>
          <ChartBox h={280}>
            <PieChart margin={{ top: 8, right: 8, bottom: 56, left: 8 }}>
              <Pie
                data={alertsPie.length ? alertsPie : [{ name: "Sem dados", value: 1, code: "" }]}
                dataKey="value"
                nameKey="name"
                cx="50%"
                cy="42%"
                outerRadius={72}
                label={false}
              >
                {(alertsPie.length ? alertsPie : [{ name: "—", value: 1, code: "" }]).map((_, i) => (
                  <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Legend
                layout="horizontal"
                verticalAlign="bottom"
                align="center"
                wrapperStyle={{ fontSize: 11, lineHeight: 1.35, paddingTop: 8 }}
              />
            </PieChart>
          </ChartBox>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Totais por tipo</h3>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Tipo</th>
                    <th className="mono">Qtd</th>
                  </tr>
                </thead>
                <tbody>
                  {(alertsByType ?? []).map((r) => (
                    <tr key={String(r.alert_type)}>
                      <td>{displayAlertType(r.alert_type)}</td>
                      <td className="mono">{fmtInt(r.count)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </Section>
    </>
  );
}
