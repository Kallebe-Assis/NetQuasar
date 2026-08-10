import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastInfo, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { fleetMoney, fleetNum, monthStartISO, todayISO } from "./fleetUtils";

type Fueling = {
  id: string;
  number: number;
  fueled_at: string;
  vehicle_id: string;
  plate?: string;
  driver_name?: string | null;
  station_name?: string | null;
  fuel_name?: string;
  liters: number;
  price_per_liter: number;
  total_amount: number;
  km_driven?: number | null;
  km_per_liter?: number | null;
  cost_per_km?: number | null;
};

export function FleetFuelingsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [from, setFrom] = useState(monthStartISO());
  const [to, setTo] = useState(todayISO());
  const [quick, setQuick] = useState(true);
  const [form, setForm] = useState({
    vehicle_id: "", driver_id: "", station_id: "", fuel_id: "", cost_center_id: "",
    liters: "", price_per_liter: "", odometer_previous: "", odometer_current: "", notes: "", fueled_at: "",
  });
  const [preview, setPreview] = useState<{ total: number; km: number | null; kpl: number | null; cpk: number | null } | null>(null);

  const list = useQuery({
    queryKey: [...queryKeys.fleetFuelings, from, to],
    queryFn: () => apiFetch<{ items: Fueling[] }>(`/api/v1/fleet/fuelings?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  });
  const vehicles = useQuery({ queryKey: queryKeys.fleetVehicles, queryFn: () => apiFetch<{ items: { id: string; plate: string; description: string }[] }>("/api/v1/fleet/vehicles?status=active") });
  const drivers = useQuery({ queryKey: queryKeys.fleetDrivers, queryFn: () => apiFetch<{ items: { id: string; name: string }[] }>("/api/v1/fleet/drivers?status=active") });
  const fuels = useQuery({ queryKey: queryKeys.fleetFuels, queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/fuels?active=1") });
  const stations = useQuery({ queryKey: queryKeys.fleetStations, queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/stations?status=active") });
  const ccs = useQuery({ queryKey: queryKeys.fleetCostCenters, queryFn: () => apiFetch<{ items: { id: string; code: string; description: string }[] }>("/api/v1/fleet/cost-centers?status=active") });
  const meDriver = useQuery({ queryKey: queryKeys.fleetMeDriver, queryFn: () => apiFetch<{ driver: { id: string; name: string } | null }>("/api/v1/fleet/me/driver") });

  useEffect(() => {
    if (meDriver.data?.driver?.id && !form.driver_id) {
      setForm((f) => ({ ...f, driver_id: meDriver.data!.driver!.id }));
    }
  }, [meDriver.data?.driver?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  function recalc(next = form) {
    const liters = Number(String(next.liters).replace(",", "."));
    const price = Number(String(next.price_per_liter).replace(",", "."));
    const prev = Number(String(next.odometer_previous).replace(",", "."));
    const curr = Number(String(next.odometer_current).replace(",", "."));
    if (!Number.isFinite(liters) || !Number.isFinite(price)) {
      setPreview(null);
      return;
    }
    const total = liters * price;
    let km: number | null = null;
    let kpl: number | null = null;
    let cpk: number | null = null;
    if (Number.isFinite(prev) && Number.isFinite(curr) && curr >= prev) {
      km = curr - prev;
      if (km > 0 && liters > 0) {
        kpl = km / liters;
        cpk = total / km;
      }
    }
    setPreview({ total, km, kpl, cpk });
  }

  async function onVehicle(id: string) {
    setForm((f) => ({ ...f, vehicle_id: id }));
    if (!id) return;
    try {
      const af = await apiFetch<{
        odometer_previous: number;
        primary_driver_id?: string | null;
        primary_fuel_id?: string | null;
        cost_center_id?: string | null;
      }>(`/api/v1/fleet/vehicles/${id}/autofill`);
      setForm((f) => {
        const next = {
          ...f,
          vehicle_id: id,
          odometer_previous: String(af.odometer_previous ?? 0),
          odometer_current: "",
          driver_id: af.primary_driver_id || f.driver_id || meDriver.data?.driver?.id || "",
          fuel_id: af.primary_fuel_id || f.fuel_id,
          cost_center_id: af.cost_center_id || f.cost_center_id,
        };
        setTimeout(() => recalc(next), 0);
        return next;
      });
    } catch (e) {
      toastErr(push, e);
    }
  }

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        fueled_at: form.fueled_at || undefined,
        vehicle_id: form.vehicle_id,
        driver_id: form.driver_id || null,
        station_id: form.station_id || null,
        fuel_id: form.fuel_id,
        cost_center_id: form.cost_center_id || null,
        liters: Number(String(form.liters).replace(",", ".")),
        price_per_liter: Number(String(form.price_per_liter).replace(",", ".")),
        odometer_previous: form.odometer_previous ? Number(String(form.odometer_previous).replace(",", ".")) : null,
        odometer_current: form.odometer_current ? Number(String(form.odometer_current).replace(",", ".")) : null,
        notes: form.notes.trim() || null,
      };
      return apiFetch<{ alerts?: { title: string; message: string; severity: string }[] }>("/api/v1/fleet/fuelings", {
        method: "POST",
        body: JSON.stringify(payload),
      });
    },
    onSuccess: async (res) => {
      toastOk(push, "Abastecimento registado");
      if (res.alerts?.length) {
        for (const a of res.alerts) toastInfo(push, `${a.title}: ${a.message}`);
      }
      setForm((f) => ({ ...f, liters: "", price_per_liter: "", odometer_current: "", notes: "" }));
      setPreview(null);
      await Promise.all([
        qc.invalidateQueries({ queryKey: queryKeys.fleetFuelings }),
        qc.invalidateQueries({ queryKey: queryKeys.fleetVehicles }),
        qc.invalidateQueries({ queryKey: queryKeys.fleetAlerts }),
      ]);
    },
    onError: (e) => toastErr(push, e),
  });

  return (
    <div className="fleet-page">
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 12 }}>
        <h1 style={{ margin: 0 }}>Frota — Abastecimentos</h1>
        <div className="row">
          <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </div>
      </div>

      {canMutate ? (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h3 style={{ margin: 0 }}>{quick ? "Abastecimento rápido" : "Abastecimento completo"}</h3>
            <button type="button" className="btn btn--sm" onClick={() => setQuick((v) => !v)}>{quick ? "Modo completo" : "Modo rápido"}</button>
          </div>
          <div className="fleet-form-grid" style={{ marginTop: 10 }}>
            <label>Veículo*<select className="input" value={form.vehicle_id} onChange={(e) => void onVehicle(e.target.value)}>
              <option value="">Seleccione…</option>
              {(vehicles.data?.items ?? []).map((v) => <option key={v.id} value={v.id}>{v.plate} — {v.description}</option>)}
            </select></label>
            <label>Motorista<select className="input" value={form.driver_id} onChange={(e) => setForm({ ...form, driver_id: e.target.value })}>
              <option value="">—</option>
              {(drivers.data?.items ?? []).map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select></label>
            <label>Combustível*<select className="input" value={form.fuel_id} onChange={(e) => setForm({ ...form, fuel_id: e.target.value })}>
              <option value="">—</option>
              {(fuels.data?.items ?? []).map((f) => <option key={f.id} value={f.id}>{f.description}</option>)}
            </select></label>
            {!quick ? (
              <label>Posto<select className="input" value={form.station_id} onChange={(e) => setForm({ ...form, station_id: e.target.value })}>
                <option value="">—</option>
                {(stations.data?.items ?? []).map((s) => <option key={s.id} value={s.id}>{s.description}</option>)}
              </select></label>
            ) : null}
            {!quick ? (
              <label>Centro de custo<select className="input" value={form.cost_center_id} onChange={(e) => setForm({ ...form, cost_center_id: e.target.value })}>
                <option value="">—</option>
                {(ccs.data?.items ?? []).map((c) => <option key={c.id} value={c.id}>{c.code} — {c.description}</option>)}
              </select></label>
            ) : null}
            <label>Hodómetro anterior<input className="input" value={form.odometer_previous} onChange={(e) => { const next = { ...form, odometer_previous: e.target.value }; setForm(next); recalc(next); }} /></label>
            <label>Hodómetro actual<input className="input" value={form.odometer_current} onChange={(e) => { const next = { ...form, odometer_current: e.target.value }; setForm(next); recalc(next); }} /></label>
            <label>Litros*<input className="input" value={form.liters} onChange={(e) => { const next = { ...form, liters: e.target.value }; setForm(next); recalc(next); }} /></label>
            <label>Preço/L*<input className="input" value={form.price_per_liter} onChange={(e) => { const next = { ...form, price_per_liter: e.target.value }; setForm(next); recalc(next); }} /></label>
            {!quick ? <label>Data/hora<input className="input" type="datetime-local" value={form.fueled_at} onChange={(e) => setForm({ ...form, fueled_at: e.target.value })} /></label> : null}
            {!quick ? <label className="fleet-form-span">Observação<input className="input" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label> : null}
          </div>
          {preview ? (
            <div className="fleet-preview row" style={{ marginTop: 10, gap: 16 }}>
              <span>Total: <strong>{fleetMoney(preview.total)}</strong></span>
              <span>KM: <strong>{fleetNum(preview.km, 1)}</strong></span>
              <span>KM/L: <strong>{fleetNum(preview.kpl)}</strong></span>
              <span>R$/KM: <strong>{fleetMoney(preview.cpk)}</strong></span>
            </div>
          ) : null}
          <div className="row" style={{ marginTop: 10 }}>
            <button type="button" className="btn btn--primary" disabled={save.isPending || !form.vehicle_id || !form.fuel_id || !form.liters || !form.price_per_liter} onClick={() => save.mutate()}>
              Registar abastecimento
            </button>
          </div>
        </div>
      ) : null}

      <div className="card table-wrap">
        <table>
          <thead><tr><th>#</th><th>Data</th><th>Placa</th><th>Motorista</th><th>Combustível</th><th>Litros</th><th>Total</th><th>KM/L</th><th>R$/KM</th></tr></thead>
          <tbody>
            {(list.data?.items ?? []).map((f) => (
              <tr key={f.id}>
                <td>{f.number}</td>
                <td>{new Date(f.fueled_at).toLocaleString("pt-BR")}</td>
                <td>{f.plate}</td>
                <td>{f.driver_name ?? "—"}</td>
                <td>{f.fuel_name}</td>
                <td>{fleetNum(f.liters, 2)}</td>
                <td>{fleetMoney(f.total_amount)}</td>
                <td>{fleetNum(f.km_per_liter)}</td>
                <td>{fleetMoney(f.cost_per_km ?? null)}</td>
              </tr>
            ))}
            {(list.data?.items ?? []).length === 0 ? <tr><td colSpan={9} className="muted">Sem abastecimentos no período.</td></tr> : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
