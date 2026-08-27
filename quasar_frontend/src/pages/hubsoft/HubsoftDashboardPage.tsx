import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { Bar, BarChart, CartesianGrid, Cell, Legend, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { HubsoftHeader } from "./HubsoftHeader";
import { HubsoftFinancialSummaryView } from "./HubsoftFinancialSummaryView";
import type { HubsoftDashboardResponse, HubsoftFinancialSummaryResponse, NamedCount, RecentActivityResponse } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";

type DashboardMode = "clients" | "support" | "financial";

const MODE_OPTIONS: { id: DashboardMode; label: string }[] = [
  { id: "clients", label: "Clientes e serviços" },
  { id: "support", label: "Atendimentos e ordens de serviço" },
  { id: "financial", label: "Financeiro" },
];

function fmtInt(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR");
}

function fmtCurrency(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

function truncateLabel(s: string, max = 22): string {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}

/** Barra horizontal categórica — uma cor só (magnitude por categoria, não identidade), como
 * já é usado nos gráficos de barras existentes do NetQuasar (ver CommercialPage). */
function CategoryBarChart({ title, data, unit = "Clientes" }: { title: string; data: NamedCount[]; unit?: string }) {
  if (data.length === 0) return null;
  const chartData = data.map((d) => ({ name: truncateLabel(d.name), fullName: d.name, count: d.count }));
  const height = Math.max(160, chartData.length * 32);
  return (
    <div className="report-chart-cell" style={{ minHeight: height + 56 }}>
      <h3 style={{ marginTop: 0, fontSize: 13 }}>{title}</h3>
      <ResponsiveContainer width="100%" height={height}>
        <BarChart data={chartData} layout="vertical" margin={{ top: 4, right: 24, left: 4, bottom: 4 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" horizontal={false} />
          <XAxis type="number" tick={{ fontSize: 10 }} allowDecimals={false} />
          <YAxis type="category" dataKey="name" tick={{ fontSize: 10 }} width={140} />
          <Tooltip
            formatter={(v: number) => [v.toLocaleString("pt-BR"), unit]}
            labelFormatter={(_l, payload) => (payload && payload[0] ? String(payload[0].payload.fullName) : "")}
            contentStyle={{ background: "var(--panel)", border: "1px solid var(--border)", fontSize: 11, color: "var(--text)" }}
            itemStyle={{ color: "var(--text)" }}
            labelStyle={{ color: "var(--text)" }}
          />
          <Bar dataKey="count" fill="var(--accent)" radius={[0, 4, 4, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

/** Conectividade é um estado (bom/crítico/neutro), não uma identidade arbitrária — usa as
 * mesmas cores de status já usadas nos badges do resto do app (--ok/--err/--muted). */
const CONNECTED_COLORS: Record<string, string> = {
  Conectado: "var(--ok)",
  Desconectado: "var(--err)",
  "Sem dado": "var(--muted)",
};

function ConnectedPie({ data }: { data: NamedCount[] }) {
  if (data.length === 0) return null;
  return (
    <div className="report-chart-cell" style={{ minHeight: 260 }}>
      <h3 style={{ marginTop: 0, fontSize: 13 }}>Conectividade dos serviços (amostra)</h3>
      <ResponsiveContainer width="100%" height={220}>
        <PieChart>
          <Pie data={data} dataKey="count" nameKey="name" innerRadius={48} outerRadius={78} paddingAngle={2}>
            {data.map((d) => (
              <Cell key={d.name} fill={CONNECTED_COLORS[d.name] || "var(--muted)"} stroke="var(--panel)" strokeWidth={2} />
            ))}
          </Pie>
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Tooltip
            formatter={(v: number) => [v.toLocaleString("pt-BR"), "Serviços"]}
            contentStyle={{ background: "var(--panel)", border: "1px solid var(--border)", fontSize: 11, color: "var(--text)" }}
            itemStyle={{ color: "var(--text)" }}
            labelStyle={{ color: "var(--text)" }}
          />
        </PieChart>
      </ResponsiveContainer>
    </div>
  );
}

function ClientsDashboard({ d }: { d: HubsoftDashboardResponse }) {
  const cancelRate = d.sample_services > 0 ? `${((d.canceled_services / d.sample_services) * 100).toFixed(1)}%` : "—";
  const connectedCount = d.connected_breakdown.find((c) => c.name === "Conectado")?.count ?? 0;
  const connectedRate = d.sample_services > 0 ? `${((connectedCount / d.sample_services) * 100).toFixed(1)}%` : "—";
  return (
    <>
      <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(6, minmax(0, 1fr))" }}>
        <div className="stat">
          <div className="stat__k">Clientes na amostra</div>
          <div className="stat__v">{fmtInt(d.sample_clients)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Serviços na amostra</div>
          <div className="stat__v">{fmtInt(d.sample_services)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Serviços ativos</div>
          <div className="stat__v">{fmtInt(d.active_services)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Taxa de cancelamento</div>
          <div className="stat__v">{cancelRate}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Taxa de conectados</div>
          <div className="stat__v">{connectedRate}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Receita mensal estimada</div>
          <div className="stat__v" style={{ fontSize: 15 }}>
            {fmtCurrency(d.estimated_monthly_revenue)}
          </div>
        </div>
      </div>

      <div className="report-chart-grid" style={{ marginTop: 16 }}>
        <ConnectedPie data={d.connected_breakdown} />
        <CategoryBarChart title="Status dos serviços (amostra)" data={d.status_breakdown} />
        <CategoryBarChart title="Tecnologia (amostra)" data={d.technology_breakdown} />
        <CategoryBarChart title="Planos mais comuns (amostra)" data={d.top_plans} />
        <CategoryBarChart title="Cidades mais comuns (amostra)" data={d.top_cities} />
      </div>
    </>
  );
}

function SupportDashboard({ d }: { d: RecentActivityResponse }) {
  return (
    <>
      <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(3, minmax(0, 1fr))" }}>
        <div className="stat">
          <div className="stat__k">Clientes na amostra</div>
          <div className="stat__v">{fmtInt(d.sample_clients)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Atendimentos encontrados</div>
          <div className="stat__v">{fmtInt(d.total_attendance_found)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Ordens de serviço encontradas</div>
          <div className="stat__v">{fmtInt(d.total_work_orders_found)}</div>
        </div>
      </div>

      <div className="report-chart-grid" style={{ marginTop: 16 }}>
        <CategoryBarChart title="Atendimentos por status (amostra)" data={d.attendance_status_breakdown} unit="Atendimentos" />
        <CategoryBarChart title="Ordens de serviço por status (amostra)" data={d.work_order_status_breakdown} unit="Ordens" />
      </div>
    </>
  );
}

/**
 * Dashboard da HubSoft — 3 modos (Clientes e serviços / Atendimentos e ordens de serviço /
 * Financeiro), cada um consumindo o mesmo endpoint/cache já usado pelas abas dedicadas
 * (queryKeys.hubsoftDashboard, hubsoftRecentActivity, hubsoftFinancialSummary) — trocar de
 * modo aqui não refaz a varredura se a aba correspondente já tiver sido aberta. A API não
 * expõe totais/listagem completa da base (todo endpoint de cliente exige busca+termo_busca,
 * confirmado em produção), então os números vêm de uma varredura de amostra — indicativo,
 * não o total exato da operadora.
 */
export function HubsoftDashboardPage() {
  const [mode, setMode] = useState<DashboardMode>("clients");

  const dashQ = useQuery({
    queryKey: queryKeys.hubsoftDashboard,
    queryFn: () => apiFetch<HubsoftDashboardResponse>("/api/v1/integrations/hubsoft/hubsoft/dashboard"),
    staleTime: Infinity,
    gcTime: 60 * 60 * 1000,
    enabled: mode === "clients",
  });
  const supportQ = useQuery({
    queryKey: queryKeys.hubsoftRecentActivity,
    queryFn: () => apiFetch<RecentActivityResponse>("/api/v1/integrations/hubsoft/hubsoft/recent-activity"),
    staleTime: Infinity,
    gcTime: 60 * 60 * 1000,
    enabled: mode === "support",
  });
  const finQ = useQuery({
    queryKey: queryKeys.hubsoftFinancialSummary,
    queryFn: () => apiFetch<HubsoftFinancialSummaryResponse>("/api/v1/integrations/hubsoft/hubsoft/financial-summary"),
    staleTime: Infinity,
    gcTime: 60 * 60 * 1000,
    enabled: mode === "financial",
  });

  const active = mode === "clients" ? dashQ : mode === "support" ? supportQ : finQ;
  const description =
    mode === "clients"
      ? "Varredura de clientes coletada em tempo real (cobre a base quase toda, mas ainda é uma amostra)."
      : mode === "support"
        ? "Amostra rápida de clientes — atendimentos e ordens de serviço encontrados neles, por status."
        : "Amostra rápida de clientes — soma do financeiro (a receber, vencido, pendente, pago).";

  return (
    <div className="integration-consult">
      <HubsoftHeader />
      <div className="card">
        <div className="hubsoft-page-head">
          <div>
            <h2 style={{ margin: 0 }}>Dashboard</h2>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              A HubSoft não tem um endpoint de totais/listagem completa da base. {description}
            </p>
          </div>
          <div className="hubsoft-page-head__controls">
            <select className="input" value={mode} onChange={(e) => setMode(e.target.value as DashboardMode)}>
              {MODE_OPTIONS.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.label}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="btn btn--icon hubsoft-refresh-btn"
              aria-label="Atualizar"
              title="Atualizar"
              disabled={active.isFetching}
              onClick={() => void active.refetch()}
            >
              <RefreshCw size={16} className={active.isFetching ? "map-refresh-spin" : undefined} />
            </button>
          </div>
        </div>

        {active.isLoading ? (
          <div className="hubsoft-loading">
            <RefreshCw size={18} className="map-refresh-spin" />
            <span>A coletar dados da HubSoft — pode demorar até {mode === "clients" ? "um a dois minutos" : "cerca de um minuto"}…</span>
          </div>
        ) : active.isError ? (
          <div className="msg msg--err">{(active.error as Error).message}</div>
        ) : mode === "clients" ? (
          !dashQ.data?.ok ? (
            <div className="msg msg--err">{dashQ.data?.message || "Falha ao calcular o dashboard."}</div>
          ) : (
            <ClientsDashboard d={dashQ.data} />
          )
        ) : mode === "support" ? (
          !supportQ.data?.ok ? (
            <div className="msg msg--err">{supportQ.data?.message || "Falha ao coletar atendimentos/ordens."}</div>
          ) : (
            <SupportDashboard d={supportQ.data} />
          )
        ) : !finQ.data?.ok ? (
          <div className="msg msg--err">{finQ.data?.message || "Falha ao calcular o resumo financeiro."}</div>
        ) : (
          <HubsoftFinancialSummaryView d={finQ.data} />
        )}
      </div>
    </div>
  );
}
