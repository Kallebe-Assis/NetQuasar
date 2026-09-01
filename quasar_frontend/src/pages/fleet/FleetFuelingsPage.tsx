import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { BanknoteArrowDown, Filter, Fuel, Pencil, Plus, Trash2 } from "lucide-react";
import { ConfirmModal } from "../../components/ConfirmModal";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastInfo, toastOk } from "../../lib/operationToast";
import { invalidateFleetOperationalQueries, queryKeys } from "../../lib/queryKeys";
import {
  formatFleetPlateOrUnknown,
  fleetMoney,
  fleetNum,
  formatISODateBR,
  isFleetVehicleLaunchBlocked,
  monthStartISO,
  toDatetimeLocalInput,
  todayISO,
} from "./fleetUtils";

type FleetVehicleOpt = { id: string; plate: string; description: string; status?: string };

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
  odometer_current?: number | null;
  notes?: string | null;
};

type Expense = {
  id: string;
  number: number;
  occurred_at: string;
  vehicle_id: string;
  plate?: string;
  expense_type_id?: string;
  expense_type: string;
  type_label?: string;
  description: string;
  unit_price: number;
  quantity: number;
  total_amount: number;
  odometer?: number | null;
  notes?: string | null;
  items?: { description: string; quantity: number; unit_price: number; total_amount: number }[];
};

type Row = {
  id: string;
  kind: "fueling" | "expense";
  at: string;
  plate: string;
  typeLabel: string;
  description: string;
  quantity: number | null;
  unitPrice: number | null;
  total: number;
  km: number | null;
  notes?: string | null;
  /** Só preenchido para kind==="expense" — referência ao registo completo, usada por Editar/Remover. */
  expense?: Expense;
};

const emptyFueling = () => ({
  vehicle_id: "",
  driver_id: "",
  station_id: "",
  fuel_id: "",
  cost_center_id: "",
  liters: "",
  price_per_liter: "",
  odometer_previous: "",
  odometer_current: "",
  notes: "",
  fueled_at: "",
});

const emptyExpenseItem = () => ({ description: "", quantity: "1", unit_price: "" });

const emptyExpense = () => ({
  vehicle_id: "",
  expense_type_id: "",
  description: "",
  occurred_at: "",
  odometer: "",
  notes: "",
  items: [emptyExpenseItem()],
});

export function FleetFuelingsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [from, setFrom] = useState(monthStartISO());
  const [to, setTo] = useState(todayISO());
  const [vehicleId, setVehicleId] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [modal, setModal] = useState<"expense" | "fueling" | null>(null);
  const [quick, setQuick] = useState(true);
  const [fuelForm, setFuelForm] = useState(emptyFueling());
  const [expForm, setExpForm] = useState(emptyExpense());
  const [preview, setPreview] = useState<{ total: number; km: number | null; kpl: number | null; cpk: number | null } | null>(null);
  const [odoConfirmOpen, setOdoConfirmOpen] = useState(false);
  const [odoBaseline, setOdoBaseline] = useState("");
  const [editingExpenseId, setEditingExpenseId] = useState<string | null>(null);
  const [deleteExpenseTarget, setDeleteExpenseTarget] = useState<Expense | null>(null);

  const listQs = useMemo(() => {
    const qs = new URLSearchParams({ limit: "10000" });
    if (from) qs.set("from", from);
    if (to) qs.set("to", to);
    if (vehicleId) qs.set("vehicle_id", vehicleId);
    return qs;
  }, [from, to, vehicleId]);
  const showFuelings = typeFilter === "" || typeFilter === "fueling";
  const showExpenses = typeFilter !== "fueling";

  const fuelings = useQuery({
    queryKey: [...queryKeys.fleetFuelings, from, to, vehicleId, typeFilter],
    queryFn: () => apiFetch<{ items: Fueling[] }>(`/api/v1/fleet/fuelings?${listQs}`),
    enabled: showFuelings,
  });
  const expenses = useQuery({
    queryKey: [...queryKeys.fleetExpenses, from, to, vehicleId, typeFilter],
    queryFn: () => {
      const qs = new URLSearchParams(listQs);
      if (typeFilter && typeFilter !== "fueling") qs.set("expense_type_id", typeFilter);
      return apiFetch<{ items: Expense[] }>(`/api/v1/fleet/expenses?${qs}`);
    },
    enabled: showExpenses,
  });
  const vehicles = useQuery({ queryKey: queryKeys.fleetVehicles, queryFn: () => apiFetch<{ items: FleetVehicleOpt[] }>("/api/v1/fleet/vehicles") });
  const launchVehicles = useMemo(
    () => (vehicles.data?.items ?? []).filter((v) => !isFleetVehicleLaunchBlocked(v.status)),
    [vehicles.data?.items],
  );
  const drivers = useQuery({ queryKey: queryKeys.fleetDrivers, queryFn: () => apiFetch<{ items: { id: string; name: string }[] }>("/api/v1/fleet/drivers?status=active") });
  const fuels = useQuery({ queryKey: queryKeys.fleetFuels, queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/fuels?active=1") });
  const stations = useQuery({ queryKey: queryKeys.fleetStations, queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/stations?status=active") });
  const ccs = useQuery({ queryKey: queryKeys.fleetCostCenters, queryFn: () => apiFetch<{ items: { id: string; code: string; description: string }[] }>("/api/v1/fleet/cost-centers?status=active") });
  const expenseTypes = useQuery({
    queryKey: queryKeys.fleetExpenseTypes,
    queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/expense-types?active=1"),
  });
  const meDriver = useQuery({ queryKey: queryKeys.fleetMeDriver, queryFn: () => apiFetch<{ driver: { id: string; name: string } | null }>("/api/v1/fleet/me/driver") });

  useEffect(() => {
    if (meDriver.data?.driver?.id && !fuelForm.driver_id) {
      setFuelForm((f) => ({ ...f, driver_id: meDriver.data!.driver!.id }));
    }
  }, [meDriver.data?.driver?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const rows = useMemo<Row[]>(() => {
    const fuelRows: Row[] = (fuelings.data?.items ?? []).map((f) => ({
      id: `fueling-${f.id}`,
      kind: "fueling",
      at: f.fueled_at,
      plate: formatFleetPlateOrUnknown(f.plate ?? ""),
      typeLabel: "Abastecimento",
      description: f.fuel_name ?? "Combustível",
      quantity: f.liters,
      unitPrice: f.price_per_liter,
      total: f.total_amount,
      km: f.odometer_current ?? f.km_driven ?? null,
      notes: f.notes,
    }));
    const expRows: Row[] = (expenses.data?.items ?? []).map((e) => ({
      id: `expense-${e.id}`,
      kind: "expense",
      at: e.occurred_at,
      plate: formatFleetPlateOrUnknown(e.plate ?? ""),
      typeLabel: e.type_label || e.expense_type,
      description: (e.items?.length ?? 0) > 1
        ? `${e.items![0].description} +${e.items!.length - 1}`
        : e.description,
      quantity: e.quantity,
      unitPrice: e.unit_price,
      total: e.total_amount,
      km: e.odometer ?? null,
      notes: e.notes,
      expense: e,
    }));
    return [...(showFuelings ? fuelRows : []), ...(showExpenses ? expRows : [])].sort(
      (a, b) => new Date(b.at).getTime() - new Date(a.at).getTime(),
    );
  }, [fuelings.data?.items, expenses.data?.items, showFuelings, showExpenses]);

  const totals = useMemo(() => {
    const count = rows.length;
    const amount = rows.reduce((s, r) => s + (Number.isFinite(r.total) ? r.total : 0), 0);
    const fuelingRows = showFuelings ? (fuelings.data?.items ?? []) : [];
    const liters = fuelingRows.reduce((s, f) => s + (Number.isFinite(f.liters) ? f.liters : 0), 0);
    const kmSpan = fuelingRows.reduce((s, f) => {
      const km = f.km_driven;
      return s + (km != null && Number.isFinite(km) && km > 0 ? km : 0);
    }, 0);
    return {
      count,
      amount,
      showFuel: typeFilter === "" || typeFilter === "fueling",
      liters,
      kmSpan,
    };
  }, [rows, fuelings.data?.items, showFuelings, typeFilter]);

  const expenseTotal = useMemo(() => {
    let sum = 0;
    let ok = false;
    for (const it of expForm.items) {
      const unit = Number(String(it.unit_price).replace(",", "."));
      const qty = Number(String(it.quantity).replace(",", "."));
      if (!Number.isFinite(unit) || !Number.isFinite(qty)) continue;
      sum += unit * qty;
      ok = true;
    }
    return ok ? sum : null;
  }, [expForm.items]);

  const itemsValid = expForm.items.some((it) => {
    const unit = Number(String(it.unit_price).replace(",", "."));
    const qty = Number(String(it.quantity).replace(",", "."));
    return it.description.trim() && Number.isFinite(unit) && unit >= 0 && Number.isFinite(qty) && qty > 0;
  });

  function recalc(next = fuelForm) {
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

  async function onFuelVehicle(id: string) {
    setFuelForm((f) => ({ ...f, vehicle_id: id }));
    if (!id) return;
    try {
      const af = await apiFetch<{
        odometer_previous: number;
        primary_driver_id?: string | null;
        primary_fuel_id?: string | null;
        cost_center_id?: string | null;
      }>(`/api/v1/fleet/vehicles/${id}/autofill`);
      setFuelForm((f) => {
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

  async function onExpenseVehicle(id: string) {
    setExpForm((f) => ({ ...f, vehicle_id: id, odometer: "" }));
    setOdoBaseline("");
    if (!id) return;
    try {
      const af = await apiFetch<{ odometer_previous: number }>(`/api/v1/fleet/vehicles/${id}/autofill`);
      const odo = af.odometer_previous != null ? String(af.odometer_previous) : "";
      setOdoBaseline(odo);
      setExpForm((f) => ({ ...f, vehicle_id: id, odometer: odo }));
    } catch (e) {
      toastErr(push, e);
    }
  }

  function closeModal() {
    setModal(null);
    setPreview(null);
    setOdoConfirmOpen(false);
    setOdoBaseline("");
    setEditingExpenseId(null);
    setExpForm(emptyExpense());
  }

  function startEditExpense(e: Expense) {
    setEditingExpenseId(e.id);
    setOdoBaseline(e.odometer != null ? String(e.odometer) : "");
    setExpForm({
      vehicle_id: e.vehicle_id,
      expense_type_id: e.expense_type_id ?? "",
      description: e.description ?? "",
      occurred_at: toDatetimeLocalInput(e.occurred_at),
      odometer: e.odometer != null ? String(e.odometer) : "",
      notes: e.notes ?? "",
      items:
        e.items && e.items.length > 0
          ? e.items.map((it) => ({ description: it.description, quantity: String(it.quantity), unit_price: String(it.unit_price) }))
          : [{ description: e.description ?? "", quantity: String(e.quantity), unit_price: String(e.unit_price) }],
    });
    setModal("expense");
  }

  function vehicleById(id: string) {
    return (vehicles.data?.items ?? []).find((v) => v.id === id);
  }

  function assertVehicleActive(id: string, kind: "despesa" | "abastecimento") {
    if (isFleetVehicleLaunchBlocked(vehicleById(id)?.status)) {
      throw new Error(`Não é possível lançar ${kind} em veículo inativo, vendido ou baixado`);
    }
  }

  const saveFueling = useMutation({
    mutationFn: async () => {
      assertVehicleActive(fuelForm.vehicle_id, "abastecimento");
      const payload = {
        fueled_at: fuelForm.fueled_at || undefined,
        vehicle_id: fuelForm.vehicle_id,
        driver_id: fuelForm.driver_id || null,
        station_id: fuelForm.station_id || null,
        fuel_id: fuelForm.fuel_id,
        cost_center_id: fuelForm.cost_center_id || null,
        liters: Number(String(fuelForm.liters).replace(",", ".")),
        price_per_liter: Number(String(fuelForm.price_per_liter).replace(",", ".")),
        odometer_previous: fuelForm.odometer_previous ? Number(String(fuelForm.odometer_previous).replace(",", ".")) : null,
        odometer_current: fuelForm.odometer_current ? Number(String(fuelForm.odometer_current).replace(",", ".")) : null,
        notes: fuelForm.notes.trim() || null,
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
      setFuelForm(emptyFueling());
      setPreview(null);
      setModal(null);
      await invalidateFleetOperationalQueries(qc);
    },
    onError: (e) => toastErr(push, e),
  });

  const saveExpense = useMutation({
    mutationFn: async (updateOdometer: boolean) => {
      assertVehicleActive(expForm.vehicle_id, "despesa");
      const items = expForm.items
        .map((it) => ({
          description: it.description.trim(),
          quantity: Number(String(it.quantity).replace(",", ".")),
          unit_price: Number(String(it.unit_price).replace(",", ".")),
        }))
        .filter((it) => it.description && Number.isFinite(it.quantity) && it.quantity > 0 && Number.isFinite(it.unit_price) && it.unit_price >= 0);
      const payload = {
        occurred_at: expForm.occurred_at || undefined,
        vehicle_id: expForm.vehicle_id,
        expense_type_id: expForm.expense_type_id || null,
        description: expForm.description.trim() || items.map((it) => it.description).join(", "),
        items,
        odometer: expForm.odometer ? Number(String(expForm.odometer).replace(",", ".")) : null,
        // Editar não arrasta o hodômetro actual do veículo (só lançar uma despesa nova faz isso)
        // — ver comentário em patchFleetExpense, handlers_fleet_expenses.go.
        update_odometer: editingExpenseId ? false : updateOdometer,
        notes: expForm.notes.trim() || null,
      };
      if (editingExpenseId) {
        return apiFetch(`/api/v1/fleet/expenses/${editingExpenseId}`, { method: "PATCH", body: JSON.stringify(payload) });
      }
      return apiFetch("/api/v1/fleet/expenses", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editingExpenseId ? "Despesa atualizada" : "Despesa registada");
      setOdoConfirmOpen(false);
      setEditingExpenseId(null);
      setExpForm(emptyExpense());
      setModal(null);
      await invalidateFleetOperationalQueries(qc);
    },
    onError: (e) => toastErr(push, e),
  });

  const deleteExpenseM = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/fleet/expenses/${id}`, { method: "DELETE" }),
    onSuccess: async () => {
      toastOk(push, "Despesa removida");
      setDeleteExpenseTarget(null);
      await invalidateFleetOperationalQueries(qc);
    },
    onError: (e) => toastErr(push, e),
  });

  function requestSaveExpense() {
    try {
      assertVehicleActive(expForm.vehicle_id, "despesa");
    } catch (e) {
      toastErr(push, e);
      return;
    }
    if (!editingExpenseId && expForm.odometer.trim() && expForm.odometer.trim() !== odoBaseline.trim()) {
      setOdoConfirmOpen(true);
      return;
    }
    saveExpense.mutate(false);
  }

  function requestSaveFueling() {
    try {
      assertVehicleActive(fuelForm.vehicle_id, "abastecimento");
    } catch (e) {
      toastErr(push, e);
      return;
    }
    const liters = Number(String(fuelForm.liters).replace(",", "."));
    const price = Number(String(fuelForm.price_per_liter).replace(",", "."));
    if (!Number.isFinite(liters) || liters <= 0) {
      toastErr(push, new Error("Litros deve ser maior que zero"));
      return;
    }
    if (!Number.isFinite(price) || price < 0) {
      toastErr(push, new Error("Preço por litro inválido"));
      return;
    }
    const prev = fuelForm.odometer_previous ? Number(String(fuelForm.odometer_previous).replace(",", ".")) : null;
    const curr = fuelForm.odometer_current ? Number(String(fuelForm.odometer_current).replace(",", ".")) : null;
    if (prev != null && curr != null && Number.isFinite(prev) && Number.isFinite(curr) && curr < prev) {
      toastErr(push, new Error("Hodômetro atual não pode ser menor que o anterior"));
      return;
    }
    saveFueling.mutate();
  }

  const odoConfirmLabel = (() => {
    const n = Number(String(expForm.odometer).replace(",", "."));
    return Number.isFinite(n) ? n.toLocaleString("pt-BR") : expForm.odometer;
  })();

  return (
    <div className="fleet-page fleet-page--with-totals">
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 12, gap: 10, flexWrap: "wrap" }}>
        <h1 style={{ margin: 0 }}>Frota — Despesas</h1>
        <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
          <button
            type="button"
            className={`btn btn--with-icon${filtersOpen || vehicleId || typeFilter ? " btn--filter-active" : ""}`}
            onClick={() => setFiltersOpen((v) => !v)}
          >
            <Filter size={16} aria-hidden />
            Filtros
          </button>
          {canMutate ? (
            <>
              <button
                type="button"
                className="btn btn--primary btn--with-icon"
                onClick={() => {
                  setExpForm(emptyExpense());
                  setOdoBaseline("");
                  setEditingExpenseId(null);
                  setModal("expense");
                }}
              >
                <BanknoteArrowDown size={18} aria-hidden />
                Adicionar nova despesa
              </button>
              <button
                type="button"
                className="btn btn--with-icon"
                onClick={() => {
                  setFuelForm({ ...emptyFueling(), driver_id: meDriver.data?.driver?.id || "" });
                  setPreview(null);
                  setQuick(true);
                  setModal("fueling");
                }}
              >
                <Fuel size={18} aria-hidden />
                Adicionar abastecimento
              </button>
            </>
          ) : null}
        </div>
      </div>

      {filtersOpen ? (
        <div className="card fleet-filter-panel">
          <div className="fleet-form-grid">
            <label>
              De
              <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
            </label>
            <label>
              Até
              <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
            </label>
            <label>
              Veículo
              <select className="input" value={vehicleId} onChange={(e) => setVehicleId(e.target.value)}>
                <option value="">Todos</option>
                {(vehicles.data?.items ?? []).map((v) => (
                  <option key={v.id} value={v.id}>
                    {formatFleetPlateOrUnknown(v.plate)} — {v.description}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Tipo de despesa
              <select className="input" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
                <option value="">Todos</option>
                <option value="fueling">Abastecimento</option>
                {(expenseTypes.data?.items ?? []).map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.description}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button
              type="button"
              className="btn btn--sm"
              onClick={() => {
                setFrom(monthStartISO());
                setTo(todayISO());
                setVehicleId("");
                setTypeFilter("");
              }}
            >
              Limpar filtros
            </button>
          </div>
        </div>
      ) : (
        <p className="muted" style={{ marginTop: 0 }}>
          Período {formatISODateBR(from)} a {formatISODateBR(to)}
          {vehicleId ? " · veículo filtrado" : ""}
          {typeFilter ? " · tipo filtrado" : ""}. Os filtros começam no mês corrente.
        </p>
      )}

      <div className="card table-wrap">
        <table>
          <thead>
            <tr>
              <th>Data</th>
              <th>Placa</th>
              <th>Tipo</th>
              <th>Descrição</th>
              <th>Qtd</th>
              <th>Unitário</th>
              <th>Total</th>
              <th>KM</th>
              <th>Obs.</th>
              {canMutate ? <th style={{ width: 76 }} /> : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id}>
                <td>{new Date(r.at).toLocaleString("pt-BR")}</td>
                <td>{r.plate}</td>
                <td>{r.typeLabel}</td>
                <td>{r.description}</td>
                <td>{fleetNum(r.quantity, 2)}</td>
                <td>{fleetMoney(r.unitPrice)}</td>
                <td>{fleetMoney(r.total)}</td>
                <td>{fleetNum(r.km, 1)}</td>
                <td>{r.notes || "—"}</td>
                {canMutate ? (
                  <td>
                    {r.kind === "expense" && r.expense ? (
                      <div className="row" style={{ gap: 4, justifyContent: "flex-end" }}>
                        <button type="button" className="btn btn--icon" title="Editar despesa" onClick={() => startEditExpense(r.expense!)}>
                          <Pencil size={13} />
                        </button>
                        <button type="button" className="btn btn--icon" title="Remover despesa" onClick={() => setDeleteExpenseTarget(r.expense!)}>
                          <Trash2 size={13} />
                        </button>
                      </div>
                    ) : null}
                  </td>
                ) : null}
              </tr>
            ))}
            {fuelings.isLoading || expenses.isLoading ? (
              <tr><td colSpan={10} className="muted">A carregar…</td></tr>
            ) : fuelings.isError || expenses.isError ? (
              <tr><td colSpan={10} className="err">Falha ao carregar lançamentos.</td></tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={10} className="muted">Sem lançamentos no período selecionado.</td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="fleet-expense-totals" aria-live="polite">
        <div>
          <span>Despesas</span>
          <strong>{totals.count}</strong>
        </div>
        <div>
          <span>Valor total</span>
          <strong>{fleetMoney(totals.amount)}</strong>
        </div>
        {totals.showFuel ? (
          <>
            <div>
              <span>Litros</span>
              <strong>{fleetNum(totals.liters, 2)}</strong>
            </div>
            <div>
              <span>KM percorridos</span>
              <strong>{fleetNum(totals.kmSpan, 1)}</strong>
            </div>
          </>
        ) : null}
      </div>

      {modal === "expense" && canMutate
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={closeModal}>
              <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <h3>{editingExpenseId ? "Editar despesa" : "Nova despesa"}</h3>
                <div className="fleet-form-grid">
                  <label>
                    Veículo*
                    <select className="input" value={expForm.vehicle_id} onChange={(e) => void onExpenseVehicle(e.target.value)}>
                      <option value="">Selecione…</option>
                      {launchVehicles.map((v) => (
                        <option key={v.id} value={v.id}>
                          {formatFleetPlateOrUnknown(v.plate)} — {v.description}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Tipo de despesa*
                    <select className="input" value={expForm.expense_type_id} onChange={(e) => setExpForm({ ...expForm, expense_type_id: e.target.value })}>
                      <option value="">Selecione…</option>
                      {(expenseTypes.data?.items ?? []).map((t) => (
                        <option key={t.id} value={t.id}>
                          {t.description}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="fleet-form-span">
                    Referência
                    <input className="input" placeholder="Ex.: compra de peças (opcional)" value={expForm.description} onChange={(e) => setExpForm({ ...expForm, description: e.target.value })} />
                  </label>
                  <label>
                    Data
                    <input className="input" type="datetime-local" value={expForm.occurred_at} onChange={(e) => setExpForm({ ...expForm, occurred_at: e.target.value })} />
                  </label>
                  <label>
                    KM do veículo
                    <input className="input" value={expForm.odometer} onChange={(e) => setExpForm({ ...expForm, odometer: e.target.value })} />
                  </label>
                  <label className="fleet-form-span">
                    Observação
                    <input className="input" value={expForm.notes} onChange={(e) => setExpForm({ ...expForm, notes: e.target.value })} />
                  </label>
                </div>
                <div className="fleet-expense-items">
                  <div className="row" style={{ justifyContent: "space-between", marginBottom: 8 }}>
                    <strong>Itens</strong>
                    <button
                      type="button"
                      className="btn btn--sm btn--with-icon"
                      onClick={() => setExpForm({ ...expForm, items: [...expForm.items, emptyExpenseItem()] })}
                    >
                      <Plus size={14} aria-hidden />
                      Adicionar item
                    </button>
                  </div>
                  {expForm.items.map((it, idx) => {
                    const unit = Number(String(it.unit_price).replace(",", "."));
                    const qty = Number(String(it.quantity).replace(",", "."));
                    const line = Number.isFinite(unit) && Number.isFinite(qty) ? unit * qty : null;
                    return (
                      <div key={idx} className="fleet-expense-item-row">
                        <input
                          className="input"
                          placeholder="Peça / serviço"
                          value={it.description}
                          onChange={(e) => {
                            const items = expForm.items.slice();
                            items[idx] = { ...it, description: e.target.value };
                            setExpForm({ ...expForm, items });
                          }}
                        />
                        <input
                          className="input"
                          placeholder="Qtd"
                          value={it.quantity}
                          onChange={(e) => {
                            const items = expForm.items.slice();
                            items[idx] = { ...it, quantity: e.target.value };
                            setExpForm({ ...expForm, items });
                          }}
                        />
                        <input
                          className="input"
                          placeholder="Valor unit."
                          value={it.unit_price}
                          onChange={(e) => {
                            const items = expForm.items.slice();
                            items[idx] = { ...it, unit_price: e.target.value };
                            setExpForm({ ...expForm, items });
                          }}
                        />
                        <input className="input" readOnly value={line != null ? fleetMoney(line) : ""} />
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Remover item"
                          disabled={expForm.items.length <= 1}
                          onClick={() => setExpForm({ ...expForm, items: expForm.items.filter((_, i) => i !== idx) })}
                        >
                          <Trash2 size={15} aria-hidden />
                        </button>
                      </div>
                    );
                  })}
                  <p className="row" style={{ justifyContent: "flex-end", margin: "8px 0 0", gap: 8 }}>
                    <span className="muted">Total</span>
                    <strong>{expenseTotal != null ? fleetMoney(expenseTotal) : "—"}</strong>
                  </p>
                </div>
                <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
                  <button type="button" className="btn" onClick={closeModal}>
                    Cancelar
                  </button>
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={saveExpense.isPending || !expForm.vehicle_id || !expForm.expense_type_id || !itemsValid || isFleetVehicleLaunchBlocked(vehicleById(expForm.vehicle_id)?.status)}
                    onClick={requestSaveExpense}
                  >
                    {saveExpense.isPending ? "A guardar…" : editingExpenseId ? "Salvar alterações" : "Registar despesa"}
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}

      {modal === "fueling" && canMutate
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={closeModal}>
              <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <h3 style={{ margin: 0 }}>{quick ? "Abastecimento rápido" : "Abastecimento completo"}</h3>
                  <button type="button" className="btn btn--sm" onClick={() => setQuick((v) => !v)}>
                    {quick ? "Modo completo" : "Modo rápido"}
                  </button>
                </div>
                <div className="fleet-form-grid" style={{ marginTop: 10 }}>
                  <label>
                    Veículo*
                    <select className="input" value={fuelForm.vehicle_id} onChange={(e) => void onFuelVehicle(e.target.value)}>
                      <option value="">Selecione…</option>
                      {launchVehicles.map((v) => (
                        <option key={v.id} value={v.id}>
                          {formatFleetPlateOrUnknown(v.plate)} — {v.description}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Motorista
                    <select className="input" value={fuelForm.driver_id} onChange={(e) => setFuelForm({ ...fuelForm, driver_id: e.target.value })}>
                      <option value="">—</option>
                      {(drivers.data?.items ?? []).map((d) => (
                        <option key={d.id} value={d.id}>
                          {d.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Combustível*
                    <select className="input" value={fuelForm.fuel_id} onChange={(e) => setFuelForm({ ...fuelForm, fuel_id: e.target.value })}>
                      <option value="">—</option>
                      {(fuels.data?.items ?? []).map((f) => (
                        <option key={f.id} value={f.id}>
                          {f.description}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Posto
                    <select className="input" value={fuelForm.station_id} onChange={(e) => setFuelForm({ ...fuelForm, station_id: e.target.value })}>
                      <option value="">—</option>
                      {(stations.data?.items ?? []).map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.description}
                        </option>
                      ))}
                    </select>
                  </label>
                  {!quick ? (
                    <label>
                      Centro de custo
                      <select className="input" value={fuelForm.cost_center_id} onChange={(e) => setFuelForm({ ...fuelForm, cost_center_id: e.target.value })}>
                        <option value="">—</option>
                        {(ccs.data?.items ?? []).map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.code} — {c.description}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                  <label>
                    Hodômetro anterior
                    <input
                      className="input"
                      value={fuelForm.odometer_previous}
                      onChange={(e) => {
                        const next = { ...fuelForm, odometer_previous: e.target.value };
                        setFuelForm(next);
                        recalc(next);
                      }}
                    />
                  </label>
                  <label>
                    Hodômetro atual
                    <input
                      className="input"
                      value={fuelForm.odometer_current}
                      onChange={(e) => {
                        const next = { ...fuelForm, odometer_current: e.target.value };
                        setFuelForm(next);
                        recalc(next);
                      }}
                    />
                  </label>
                  <label>
                    Litros*
                    <input
                      className="input"
                      value={fuelForm.liters}
                      onChange={(e) => {
                        const next = { ...fuelForm, liters: e.target.value };
                        setFuelForm(next);
                        recalc(next);
                      }}
                    />
                  </label>
                  <label>
                    Preço/L*
                    <input
                      className="input"
                      value={fuelForm.price_per_liter}
                      onChange={(e) => {
                        const next = { ...fuelForm, price_per_liter: e.target.value };
                        setFuelForm(next);
                        recalc(next);
                      }}
                    />
                  </label>
                  {!quick ? (
                    <label>
                      Data/hora
                      <input className="input" type="datetime-local" value={fuelForm.fueled_at} onChange={(e) => setFuelForm({ ...fuelForm, fueled_at: e.target.value })} />
                    </label>
                  ) : null}
                  {!quick ? (
                    <label className="fleet-form-span">
                      Observação
                      <input className="input" value={fuelForm.notes} onChange={(e) => setFuelForm({ ...fuelForm, notes: e.target.value })} />
                    </label>
                  ) : null}
                </div>
                {preview ? (
                  <div className="fleet-preview row" style={{ marginTop: 10, gap: 16 }}>
                    <span>
                      Total: <strong>{fleetMoney(preview.total)}</strong>
                    </span>
                    <span>
                      KM: <strong>{fleetNum(preview.km, 1)}</strong>
                    </span>
                    <span>
                      KM/L: <strong>{fleetNum(preview.kpl)}</strong>
                    </span>
                    <span>
                      R$/KM: <strong>{fleetMoney(preview.cpk)}</strong>
                    </span>
                  </div>
                ) : null}
                <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
                  <button type="button" className="btn" onClick={closeModal}>
                    Cancelar
                  </button>
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={saveFueling.isPending || !fuelForm.vehicle_id || !fuelForm.fuel_id || !fuelForm.liters || !fuelForm.price_per_liter || isFleetVehicleLaunchBlocked(vehicleById(fuelForm.vehicle_id)?.status)}
                    onClick={requestSaveFueling}
                  >
                    {saveFueling.isPending ? "A guardar…" : "Registar abastecimento"}
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}

      <ConfirmModal
        open={odoConfirmOpen}
        title="Atualizar hodômetro?"
        message={`O hodômetro informado (${odoConfirmLabel} km) é diferente do cadastro do veículo. Deseja atualizar o hodômetro do veículo?`}
        confirmLabel="Atualizar e salvar"
        secondaryLabel="Salvar sem atualizar"
        cancelLabel="Cancelar"
        busy={saveExpense.isPending}
        onCancel={() => setOdoConfirmOpen(false)}
        onSecondary={() => {
          setOdoConfirmOpen(false);
          saveExpense.mutate(false);
        }}
        onConfirm={() => {
          setOdoConfirmOpen(false);
          saveExpense.mutate(true);
        }}
      />

      <ConfirmModal
        open={!!deleteExpenseTarget}
        title="Remover despesa"
        message={
          deleteExpenseTarget
            ? `Remover a despesa "${deleteExpenseTarget.type_label || deleteExpenseTarget.expense_type}" de ${formatFleetPlateOrUnknown(deleteExpenseTarget.plate ?? "")} (${fleetMoney(deleteExpenseTarget.total_amount)})? Esta ação não pode ser desfeita.`
            : ""
        }
        danger
        confirmLabel="Remover"
        busy={deleteExpenseM.isPending}
        onCancel={() => setDeleteExpenseTarget(null)}
        onConfirm={() => deleteExpenseTarget && deleteExpenseM.mutate(deleteExpenseTarget.id)}
      />
    </div>
  );
}
