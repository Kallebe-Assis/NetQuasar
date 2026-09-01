import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { APP_ROUTES } from "../../app/routes";
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { addDaysISO, fleetMoney, fleetNum, formatFleetPlateOrUnknown, isFleetVehicleInactive, monthEndISO, monthStartISO, todayISO } from "./fleetUtils";

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
  expenses: { amount: number; count: number };
  totals: { fuel_amount: number; expense_amount: number; grand_total: number; fuel_count: number; expense_count: number };
  rankings: {
    most_efficient: { plate: string; description: string; value: number; liters: number; amount: number }[];
    highest_consumption: { plate: string; description: string; value: number }[];
    highest_cost_per_km: { plate: string; description: string; value: number }[];
    cheapest_stations: { description: string; avg_price: number; liters: number }[];
  };
  daily_series: { date: string; liters: number; fuel_amount: number; expense_amount: number; amount: number }[];
  open_alerts: number;
};

type PeriodKind = "custom" | "day" | "month" | "7" | "15" | "30" | "60" | "150" | "300";

function periodRange(kind: PeriodKind, day: string, month: string, from: string, to: string): { from: string; to: string } {
  const today = todayISO();
  if (kind === "day") return { from: day, to: day };
  if (kind === "month") return { from: `${month}-01`, to: monthEndISO(month) };
  const n = Number(kind);
  if (Number.isFinite(n) && n > 0) return { from: addDaysISO(today, 1 - n), to: today };
  return { from, to };
}

function chartTitle(kind: PeriodKind) {
  if (kind === "day") return "Gastos do dia";
  if (kind === "month") return "Gastos diários do mês";
  if (kind === "7") return "Gastos da semana";
  if (kind === "custom") return "Gastos diários do período";
  return `Gastos diários (últimos ${kind} dias)`;
}

function formatDayLabel(iso: string, kind: PeriodKind) {
  const [, m, d] = iso.split("-");
  if (kind === "day") return `${d}/${m}`;
  if (kind === "month" || kind === "7" || kind === "15" || kind === "30") return d;
  return `${d}/${m}`;
}

export function FleetDashboardPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [period, setPeriod] = useState<PeriodKind>("month");
  const [day, setDay] = useState(todayISO());
  const [month, setMonth] = useState(() => monthStartISO().slice(0, 7));
  const [customFrom, setCustomFrom] = useState(monthStartISO());
  const [customTo, setCustomTo] = useState(todayISO());
  const [vehicleId, setVehicleId] = useState(() => searchParams.get("vehicle_id") ?? "");
  const { from, to } = periodRange(period, day, month, customFrom, customTo);

  function onVehicleChange(id: string) {
    setVehicleId(id);
    const next = new URLSearchParams(searchParams);
    if (id) next.set("vehicle_id", id);
    else next.delete("vehicle_id");
    setSearchParams(next, { replace: true });
  }
  const vehicles = useQuery({
    queryKey: queryKeys.fleetVehicles,
    queryFn: () => apiFetch<{ items: { id: string; plate: string; description: string; status?: string }[] }>("/api/v1/fleet/vehicles"),
  });
  const selectableVehicles = useMemo(
    () => (vehicles.data?.items ?? []).filter((v) => !isFleetVehicleInactive(v.status)),
    [vehicles.data],
  );
  useEffect(() => {
    if (!vehicleId) return;
    const selectedVeh = (vehicles.data?.items ?? []).find((v) => v.id === vehicleId);
    if (selectedVeh && isFleetVehicleInactive(selectedVeh.status)) onVehicleChange("");
  }, [vehicleId, vehicles.data]);
  const q = useQuery({
    queryKey: queryKeys.fleetDashboard(from, to, vehicleId),
    queryFn: () => {
      const qs = new URLSearchParams({ from, to });
      if (vehicleId) qs.set("vehicle_id", vehicleId);
      return apiFetch<Dash>(`/api/v1/fleet/dashboard?${qs}`);
    },
  });
  const d = q.data;
  const selected = (vehicles.data?.items ?? []).find((v) => v.id === vehicleId);
  const chartData = useMemo(
    () =>
      (d?.daily_series ?? []).map((x) => ({
        ...x,
        label: formatDayLabel(x.date, period),
        Abastecimentos: x.fuel_amount,
        Despesas: x.expense_amount,
      })),
    [d, period],
  );
  const tickInterval = chartData.length > 45 ? Math.ceil(chartData.length / 15) : chartData.length > 20 ? 1 : 0;
  const chartHasSpend = chartData.some((x) => (x.Abastecimentos ?? 0) > 0 || (x.Despesas ?? 0) > 0);

  return (
    <div className="fleet-page">
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 12, gap: 10, flexWrap: "wrap" }}>
        <h1 style={{ margin: 0 }}>Frota — Dashboard</h1>
        <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
          <label className="muted" style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            Veículo
          <select className="input" value={vehicleId} onChange={(e) => onVehicleChange(e.target.value)} style={{ minWidth: 220 }}>
            <option value="">Todos os Veículos</option>
            {selectableVehicles.map((v) => (
              <option key={v.id} value={v.id}>
                {formatFleetPlateOrUnknown(v.plate)} — {v.description}
              </option>
            ))}
          </select>
          </label>
          <label className="muted" style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            Período
          <select className="input" value={period} onChange={(e) => setPeriod(e.target.value as PeriodKind)} style={{ minWidth: 180 }}>
            <option value="month">Mês específico</option>
            <option value="day">Data específica</option>
            <option value="7">Últimos 7 dias</option>
            <option value="15">Últimos 15 dias</option>
            <option value="30">Últimos 30 dias</option>
            <option value="60">Últimos 60 dias</option>
            <option value="150">Últimos 150 dias</option>
            <option value="300">Últimos 300 dias</option>
            <option value="custom">Intervalo personalizado</option>
          </select>
          </label>
          {period === "day" ? (
            <input className="input" type="date" value={day} onChange={(e) => setDay(e.target.value)} />
          ) : null}
          {period === "month" ? (
            <input className="input" type="month" value={month} onChange={(e) => setMonth(e.target.value)} />
          ) : null}
          {period === "custom" ? (
            <>
              <input className="input" type="date" value={customFrom} onChange={(e) => setCustomFrom(e.target.value)} />
              <input className="input" type="date" value={customTo} onChange={(e) => setCustomTo(e.target.value)} />
            </>
          ) : null}
        </div>
      </div>
      {selected ? (
        <p className="muted" style={{ marginTop: 0 }}>
          Exibindo apenas {formatFleetPlateOrUnknown(selected.plate)} — {selected.description}.
        </p>
      ) : null}
      {q.isLoading ? <p className="muted">A carregar…</p> : null}
      {q.isError ? <p className="err">Falha ao carregar dashboard.</p> : null}
      {d ? (
        <>
          <div className="fleet-kpi-grid">
            <div className="card fleet-kpi">
              <span>{vehicleId ? "Veículo" : "Veículos"}</span>
              <strong>{vehicleId ? (selected ? formatFleetPlateOrUnknown(selected.plate) : "1") : d.fleet.vehicles_total}</strong>
              <small>{vehicleId ? (selected?.description ?? "") : `${d.fleet.vehicles_active} ativos`}</small>
            </div>
            <div className="card fleet-kpi">
              <span>Total abastecimentos</span>
              <strong>{fleetMoney(d.fuel.amount)}</strong>
              <small>{d.fuel.count} lançamento(s)</small>
            </div>
            <div className="card fleet-kpi">
              <span>Total despesas</span>
              <strong>{fleetMoney(d.expenses?.amount ?? 0)}</strong>
              <small>{d.expenses?.count ?? 0} lançamento(s)</small>
            </div>
            <div className="card fleet-kpi">
              <span>Total geral</span>
              <strong>{fleetMoney(d.totals?.grand_total ?? d.fuel.amount + (d.expenses?.amount ?? 0))}</strong>
              <small>{(d.fuel.count ?? 0) + (d.expenses?.count ?? 0)} lançamento(s)</small>
            </div>
            <div className="card fleet-kpi"><span>Litros no período</span><strong>{fleetNum(d.fuel.liters, 1)}</strong></div>
            <div className="card fleet-kpi"><span>Preço médio/L</span><strong>{fleetMoney(d.fuel.avg_price_per_liter ?? null)}</strong></div>
            <div className="card fleet-kpi"><span>Consumo médio</span><strong>{fleetNum(d.fuel.avg_km_per_liter)} KM/L</strong></div>
            <div className="card fleet-kpi"><span>Custo médio/KM</span><strong>{fleetMoney(d.fuel.avg_cost_per_km ?? null)}</strong></div>
            <Link to={APP_ROUTES.fleetAlerts} className="card fleet-kpi" style={{ textDecoration: "none", color: "inherit" }}>
              <span>Alertas abertos</span>
              <strong>{d.open_alerts}</strong>
              <small>Ver lista</small>
            </Link>
          </div>

          <div className="card" style={{ marginTop: 12 }}>
            <h3>{chartTitle(period)}</h3>
            {chartData.length === 0 || !chartHasSpend ? (
              <p className="muted">Sem gastos no período.</p>
            ) : (
              <div className="fleet-spend-chart">
                <ResponsiveContainer width="100%" height={280}>
                  <BarChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 4 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis dataKey="label" tick={{ fill: "var(--muted)", fontSize: 10 }} interval={tickInterval} />
                    <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} tickFormatter={(v) => Number(v).toLocaleString("pt-BR", { maximumFractionDigits: 0 })} />
                    <Tooltip
                      formatter={(v: number, name: string) => [fleetMoney(Number(v) || 0), name]}
                      labelFormatter={(_, payload) => {
                        const iso = payload?.[0]?.payload?.date as string | undefined;
                        if (!iso) return "";
                        const [y, m, dd] = iso.split("-");
                        return `${dd}/${m}/${y}`;
                      }}
                      contentStyle={{ background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 8, color: "var(--text)" }}
                      itemStyle={{ color: "var(--text)" }}
                      labelStyle={{ color: "var(--text)" }}
                    />
                    <Legend />
                    <Bar dataKey="Abastecimentos" stackId="g" fill="var(--accent)" radius={[0, 0, 0, 0]} />
                    <Bar dataKey="Despesas" stackId="g" fill="#a371f7" radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>

          <div className="fleet-rank-grid">
            <RankCard title="Mais econômicos (KM/L)" rows={d.rankings.most_efficient.map((r) => ({ label: formatFleetPlateOrUnknown(r.plate), sub: r.description, value: `${fleetNum(r.value)} KM/L` }))} />
            <RankCard title="Maior consumo (litros)" rows={d.rankings.highest_consumption.map((r) => ({ label: formatFleetPlateOrUnknown(r.plate), sub: r.description, value: `${fleetNum(r.value, 1)} L` }))} />
            <RankCard title="Maiores gastos" rows={d.rankings.highest_cost_per_km.map((r) => ({ label: formatFleetPlateOrUnknown(r.plate), sub: r.description, value: fleetMoney(r.value) }))} />
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
