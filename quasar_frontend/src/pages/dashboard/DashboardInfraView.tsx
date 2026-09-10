import { Bar, BarChart, CartesianGrid, Tooltip, XAxis, YAxis } from "recharts";
import type { InfraOverview } from "./dashboardShared";
import { ChartBox, Section, fmtInt, tooltipStyle } from "./dashboardShared";
import { EmptyState } from "../../components/EmptyState";

const CTO_STATUS_META: Array<{ key: keyof NonNullable<InfraOverview["cto_status"]>; label: string; color: string }> = [
  { key: "vazia", label: "Vazias", color: "var(--muted)" },
  { key: "disponivel", label: "Disponíveis", color: "#3fb950" },
  { key: "proxima_saturacao", label: "Próx. saturação", color: "#d29922" },
  { key: "lotada", label: "Lotadas", color: "#f85149" },
  { key: "sem_portas", label: "Sem portas cadastradas", color: "var(--border)" },
];

export function DashboardInfraView({ infra }: { infra?: InfraOverview }) {
  const status = infra?.cto_status ?? {};
  const bySplitter = infra?.ctos_by_splitter ?? [];
  const splice = infra?.splice_boxes ?? {};
  const byProject = infra?.by_project ?? [];
  const totals = infra?.totals ?? {};

  return (
    <>
      <Section
        id="sec-infra-totais"
        title="Infraestrutura — totais"
        subtitle="Contagem de elementos cadastrados na infraestrutura óptica (aba Elementos / Mapa)."
      >
        <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
          <div className="stat" style={{ minWidth: 120 }}>
            <div className="stat__k">Projetos</div>
            <div className="stat__v">{fmtInt(totals.projects)}</div>
          </div>
          <div className="stat" style={{ minWidth: 120 }}>
            <div className="stat__k">CTOs</div>
            <div className="stat__v">{fmtInt(totals.ctos ?? infra?.total_ctos)}</div>
          </div>
          <div className="stat" style={{ minWidth: 140 }}>
            <div className="stat__k">Caixas de emenda</div>
            <div className="stat__v">{fmtInt(splice.emenda)}</div>
          </div>
          <div className="stat" style={{ minWidth: 150 }}>
            <div className="stat__k">Caixas de distribuição</div>
            <div className="stat__v">{fmtInt(splice.distribuicao)}</div>
          </div>
          <div className="stat" style={{ minWidth: 110 }}>
            <div className="stat__k">Cabos</div>
            <div className="stat__v">{fmtInt(totals.cables)}</div>
          </div>
          <div className="stat" style={{ minWidth: 110 }}>
            <div className="stat__k">Postes</div>
            <div className="stat__v">{fmtInt(totals.poles)}</div>
          </div>
        </div>
      </Section>

      <Section
        id="sec-infra-cto-status"
        title="Estado das CTOs"
        subtitle="Vazia = nenhuma porta ocupada · Disponível = tem porta livre · Próx. saturação = ≥ 80% ocupada · Lotada = sem porta livre. Cadastre o status das portas no esquema de fibras de cada CTO."
      >
        <div className="row" style={{ gap: 12, flexWrap: "wrap", marginBottom: 12 }}>
          {CTO_STATUS_META.map((m) => (
            <div className="stat" style={{ minWidth: 130 }} key={m.key}>
              <div className="stat__k">{m.label}</div>
              <div className="stat__v" style={{ color: m.color }}>{fmtInt(status[m.key])}</div>
            </div>
          ))}
        </div>
        <ChartBox h={220}>
          <BarChart
            data={CTO_STATUS_META.map((m) => ({ name: m.label, CTOs: Number(status[m.key] ?? 0) }))}
            margin={{ left: 8, right: 8, top: 8, bottom: 40 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
            <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 10 }} interval={0} angle={-18} textAnchor="end" height={54} />
            <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
            <Tooltip contentStyle={tooltipStyle} itemStyle={tooltipStyle} labelStyle={tooltipStyle} />
            <Bar dataKey="CTOs" fill="#58a6ff" radius={[4, 4, 0, 0]} />
          </BarChart>
        </ChartBox>
      </Section>

      <Section
        id="sec-infra-splitter"
        title="CTOs por tipo de splitter"
        subtitle="Contagem de CTOs por razão de splitter (1x8, 1x16, …)."
      >
        {bySplitter.length === 0 ? (
          <EmptyState variant="inline" title="Nenhuma CTO cadastrada." hint="Cadastre CTOs em Elementos → CTOs." />
        ) : (
          <div className="table-wrap">
            <table style={{ fontSize: 11 }}>
              <thead>
                <tr>
                  <th>Splitter</th>
                  <th className="mono">CTOs</th>
                </tr>
              </thead>
              <tbody>
                {bySplitter.map((s) => (
                  <tr key={s.splitter}>
                    <td className="mono">{s.splitter}</td>
                    <td className="mono">{fmtInt(s.count)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <Section
        id="sec-infra-projetos"
        title="Elementos por projeto"
        subtitle="CTOs, caixas de emenda/distribuição, cabos e postes cadastrados em cada projeto de rede."
      >
        {byProject.length === 0 ? (
          <EmptyState variant="inline" title="Nenhum projeto de rede cadastrado." hint="Crie projetos em Elementos → Projetos para agrupar CTOs, cabos e postes." />
        ) : (
          <div className="table-wrap">
            <table style={{ fontSize: 11 }}>
              <thead>
                <tr>
                  <th>Projeto</th>
                  <th>Estado</th>
                  <th className="mono">CTOs</th>
                  <th className="mono">Emendas</th>
                  <th className="mono">Distribuição</th>
                  <th className="mono">Cabos</th>
                  <th className="mono">Postes</th>
                </tr>
              </thead>
              <tbody>
                {byProject.map((p) => (
                  <tr key={p.project_id}>
                    <td>{p.description}</td>
                    <td style={{ color: "var(--muted)" }}>{p.status}</td>
                    <td className="mono">{fmtInt(p.ctos)}</td>
                    <td className="mono">{fmtInt(p.emendas)}</td>
                    <td className="mono">{fmtInt(p.distribuicoes)}</td>
                    <td className="mono">{fmtInt(p.cables)}</td>
                    <td className="mono">{fmtInt(p.poles)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </>
  );
}
