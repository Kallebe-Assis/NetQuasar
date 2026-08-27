import { Bar, BarChart, CartesianGrid, Cell, Legend, Pie, PieChart, Tooltip, XAxis, YAxis } from "recharts";
import type { LatRank, TopRow } from "./dashboardShared";
import { CHART_COLORS, ChartBox, Section, fmt1, fmtInt, num, tooltipStyle, trunc } from "./dashboardShared";

type CatRow = { name: string; value: number };
type CatBarRow = { name: string; Equipamentos: number };
type PopLocRow = { name: string; Equipamentos: number };
type OpModeRow = { name: string; Quantidade: number };
type NetDonutRow = { name: string; value: number };

export function DashboardEquipamentosView({
  catViz,
  onCatVizChange,
  catPie,
  catBar,
  netDonut,
  opModeBar,
  popBar,
  locBar,
  worstLatency,
  bestLatency,
  topLatency,
  topLatencyLoading,
  topLatencyError,
}: {
  catViz: "pie" | "bar";
  onCatVizChange: (v: "pie" | "bar") => void;
  catPie: CatRow[];
  catBar: CatBarRow[];
  netDonut: NetDonutRow[];
  opModeBar: OpModeRow[];
  popBar: PopLocRow[];
  locBar: PopLocRow[];
  worstLatency?: LatRank[];
  bestLatency?: LatRank[];
  topLatency?: TopRow[];
  topLatencyLoading: boolean;
  topLatencyError: string | null;
}) {
  return (
    <>
      <Section
        id="sec-inventario"
        title="Inventário e estado operacional"
        subtitle="Distribuição por categoria, estado de rede (Normal/Bridge) e modo operacional (Ativo, Manutenção, etc.). Escolha torta ou barras para categorias."
      >
        <div className="row" style={{ flexWrap: "wrap", gap: 8, marginBottom: 10 }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Visualização categorias:</span>
          <button type="button" className={catViz === "pie" ? "btn btn--primary" : "btn"} onClick={() => onCatVizChange("pie")}>
            Torta
          </button>
          <button type="button" className={catViz === "bar" ? "btn btn--primary" : "btn"} onClick={() => onCatVizChange("bar")}>
            Barras
          </button>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: 16 }}>
          {catViz === "pie" ? (
            <ChartBox h={300}>
              <PieChart margin={{ top: 8, right: 8, bottom: 48, left: 8 }}>
                <Pie data={catPie} dataKey="value" nameKey="name" cx="50%" cy="42%" outerRadius={78} label={false}>
                  {catPie.map((_, i) => (
                    <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Legend layout="horizontal" verticalAlign="bottom" align="center" wrapperStyle={{ fontSize: 11, lineHeight: 1.35, paddingTop: 8 }} />
              </PieChart>
            </ChartBox>
          ) : (
            <ChartBox h={280}>
              <BarChart data={catBar} margin={{ left: 4, right: 8, top: 8, bottom: 40 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 10 }} interval={0} angle={-25} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="Equipamentos" fill="var(--accent)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          )}
          <ChartBox h={280}>
            <PieChart>
              <Pie data={netDonut} dataKey="value" nameKey="name" innerRadius={55} outerRadius={100} paddingAngle={2}>
                {netDonut.map((_, i) => (
                  <Cell key={i} fill={CHART_COLORS[(i + 2) % CHART_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Legend />
            </PieChart>
          </ChartBox>
          <ChartBox h={280}>
            <BarChart data={opModeBar} layout="vertical" margin={{ left: 8, right: 16, top: 8, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" horizontal={false} />
              <XAxis type="number" tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
              <YAxis type="category" dataKey="name" width={100} tick={{ fill: "var(--muted)", fontSize: 10 }} />
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Bar dataKey="Quantidade" fill="#a371f7" radius={[0, 4, 4, 0]} />
            </BarChart>
          </ChartBox>
        </div>
      </Section>

      <Section
        id="sec-localidades"
        title="POPs e localidades comerciais"
        subtitle="Quantidade de equipamentos associados a cada POP e a cada localidade cadastrada no módulo comercial (inclui zeros à esquerda para referência)."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))", gap: 16 }}>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Por POP</h3>
            <ChartBox h={300}>
              <BarChart data={popBar} margin={{ left: 0, right: 8, top: 8, bottom: 48 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-30} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="Equipamentos" fill="var(--ok)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Por localidade</h3>
            <ChartBox h={300}>
              <BarChart data={locBar} margin={{ left: 0, right: 8, top: 8, bottom: 48 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-30} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="Equipamentos" fill="var(--warn)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          </div>
        </div>
      </Section>

      <Section
        id="sec-rankings"
        title="Rankings de latência (média na janela)"
        subtitle="Apenas equipamentos Ativos com pelo menos 3 amostras válidas na janela. À direita, última latência na cache de sondas."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 14 }}>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Piores médias (ms)</h3>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Equipamento</th>
                    <th className="mono">Média</th>
                  </tr>
                </thead>
                <tbody>
                  {(worstLatency ?? []).map((r, i) => (
                    <tr key={r.device_id ?? i}>
                      <td>{i + 1}</td>
                      <td>{trunc(String(r.description ?? ""), 28)}</td>
                      <td className="mono">{fmt1(r.avg_latency_ms)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Melhores médias (ms)</h3>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Equipamento</th>
                    <th className="mono">Média</th>
                  </tr>
                </thead>
                <tbody>
                  {(bestLatency ?? []).map((r, i) => (
                    <tr key={r.device_id ?? i}>
                      <td>{i + 1}</td>
                      <td>{trunc(String(r.description ?? ""), 28)}</td>
                      <td className="mono">{fmt1(r.avg_latency_ms)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Top latência (cache actual)</h3>
            {topLatencyLoading ? (
              <p style={{ fontSize: 12 }}>…</p>
            ) : topLatencyError ? (
              <div className="msg msg--err" style={{ fontSize: 11 }}>
                {topLatencyError}
              </div>
            ) : (
              <div className="table-wrap">
                <table style={{ fontSize: 11 }}>
                  <thead>
                    <tr>
                      <th>Equipamento</th>
                      <th className="mono">ms</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(topLatency ?? []).map((r) => (
                      <tr key={r.device_id}>
                        <td>{trunc(r.description, 24)}</td>
                        <td className="mono">{r.latency_ms}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
        <ChartBox h={260}>
          <BarChart
            data={(worstLatency ?? []).map((r) => ({
              name: trunc(String(r.description ?? ""), 12),
              "Latência média (ms)": num(r.avg_latency_ms),
            }))}
            margin={{ left: 8, right: 8, top: 8, bottom: 56 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
            <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-35} textAnchor="end" height={70} />
            <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} />
            <Tooltip formatter={(v: number) => fmt1(v)} contentStyle={tooltipStyle} />
            <Bar dataKey="Latência média (ms)" fill="var(--err)" radius={[4, 4, 0, 0]} />
          </BarChart>
        </ChartBox>
      </Section>
    </>
  );
}
