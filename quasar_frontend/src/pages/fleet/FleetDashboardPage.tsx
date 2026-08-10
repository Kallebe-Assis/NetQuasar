import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { fleetMoney, fleetNum, monthStartISO, todayISO } from "./fleetUtils";

type Dash = {
  fleet: { vehicles_total: number; vehicles_active: number };
  fuel: {
    liters: number;
    amount: number;
    count: number;
    avg_price_per_liter?: number | null;
    avg_km_per_liter?: number | null;
    avg_cost_per_km?: number | null;
  };
  rankings: {
    most_efficient: { plate: string; description: string; value: number; liters: number; amount: number }[];
    highest_consumption: { plate: string; description: string; value: number }[];
    highest_cost_per_km: { plate: string; description: string; value: number }[];
    cheapest_stations: { description: string; avg_price: number; liters: number }[];
  };
  monthly_series: { month: string; liters: number; amount: number }[];
  open_alerts: number;
};

export function FleetDashboardPage() {
  const [from, setFrom] = useState(monthStartISO());
  const [to, setTo] = useState(todayISO());
  const q = useQuery({
    queryKey: queryKeys.fleetDashboard(from, to),
    queryFn: () => apiFetch<Dash>(`/api/v1/fleet/dashboard?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  });
  const d = q.data;
  const maxAmount = useMemo(() => Math.max(1, ...(d?.monthly_series.map((x) => x.amount) ?? [1])), [d]);

  return (
    <div className="fleet-page">
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 12 }}>
        <h1 style={{ margin: 0 }}>Frota — Dashboard</h1>
        <div className="row">
          <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </div>
      </div>
      {q.isLoading ? <p className="muted">A carregar…</p> : null}
      {q.isError ? <p className="err">Falha ao carregar dashboard.</p> : null}
      {d ? (
        <>
          <div className="fleet-kpi-grid">
            <div className="card fleet-kpi"><span>Veículos</span><strong>{d.fleet.vehicles_total}</strong><small>{d.fleet.vehicles_active} activos</small></div>
            <div className="card fleet-kpi"><span>Litros no período</span><strong>{fleetNum(d.fuel.liters, 1)}</strong></div>
            <div className="card fleet-kpi"><span>Gasto no período</span><strong>{fleetMoney(d.fuel.amount)}</strong></div>
            <div className="card fleet-kpi"><span>Abastecimentos</span><strong>{d.fuel.count}</strong></div>
            <div className="card fleet-kpi"><span>Preço médio/L</span><strong>{fleetMoney(d.fuel.avg_price_per_liter ?? null)}</strong></div>
            <div className="card fleet-kpi"><span>Consumo médio</span><strong>{fleetNum(d.fuel.avg_km_per_liter)} KM/L</strong></div>
            <div className="card fleet-kpi"><span>Custo médio/KM</span><strong>{fleetMoney(d.fuel.avg_cost_per_km ?? null)}</strong></div>
            <div className="card fleet-kpi"><span>Alertas abertos</span><strong>{d.open_alerts}</strong></div>
          </div>

          <div className="card" style={{ marginTop: 12 }}>
            <h3>Gastos mensais</h3>
            <div className="fleet-bars">
              {d.monthly_series.map((m) => (
                <div key={m.month} className="fleet-bars__item" title={`${m.month}: ${fleetMoney(m.amount)}`}>
                  <div className="fleet-bars__fill" style={{ height: `${Math.max(4, (m.amount / maxAmount) * 100)}%` }} />
                  <span>{m.month.slice(5)}</span>
                </div>
              ))}
              {d.monthly_series.length === 0 ? <p className="muted">Sem dados ainda.</p> : null}
            </div>
          </div>

          <div className="fleet-rank-grid">
            <RankCard title="Mais económicos (KM/L)" rows={d.rankings.most_efficient.map((r) => ({ label: r.plate, sub: r.description, value: `${fleetNum(r.value)} KM/L` }))} />
            <RankCard title="Maior consumo" rows={d.rankings.highest_consumption.map((r) => ({ label: r.plate, sub: r.description, value: `${fleetNum(r.value)} KM/L` }))} />
            <RankCard title="Maior R$/KM" rows={d.rankings.highest_cost_per_km.map((r) => ({ label: r.plate, sub: r.description, value: fleetMoney(r.value) }))} />
            <RankCard title="Postos mais baratos" rows={d.rankings.cheapest_stations.map((r) => ({ label: r.description, sub: `${fleetNum(r.liters, 0)} L`, value: fleetMoney(r.avg_price) }))} />
          </div>
        </>
      ) : null}
    </div>
  );
}

function RankCard({ title, rows }: { title: string; rows: { label: string; sub: string; value: string }[] }) {
  return (
    <div className="card">
      <h3>{title}</h3>
      {rows.length === 0 ? <p className="muted">Sem dados.</p> : (
        <ul className="fleet-rank-list">
          {rows.map((r, i) => (
            <li key={`${r.label}-${i}`}>
              <div><strong>{r.label}</strong><span className="muted">{r.sub}</span></div>
              <em>{r.value}</em>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
