import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import { Archive, Ban, Check, CirclePlus, KeyRound, Pause, PenLine, Wrench, X } from "lucide-react";
import { APP_ROUTES } from "../../app/routes";
import { ActionMenu } from "../../components/ActionMenu";
import { ConfirmModal } from "../../components/ConfirmModal";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { formatFleetPlate, formatFleetPlateOrUnknown, isValidFleetPlate, UF_OPTIONS, VEHICLE_STATUS } from "./fleetUtils";

function VehicleStatusIcon({ status }: { status: string }) {
  const label = VEHICLE_STATUS.find((s) => s.value === status)?.label ?? status;
  const cls =
    status === "active"
      ? "fleet-vehicle-status fleet-vehicle-status--ok"
      : status === "inactive"
        ? "fleet-vehicle-status fleet-vehicle-status--off"
        : "fleet-vehicle-status fleet-vehicle-status--other";
  const icon =
    status === "active" ? (
      <Check size={18} strokeWidth={2.5} aria-hidden />
    ) : status === "inactive" ? (
      <X size={18} strokeWidth={2.5} aria-hidden />
    ) : status === "maintenance" ? (
      <Wrench size={18} aria-hidden />
    ) : status === "stopped" ? (
      <Pause size={18} aria-hidden />
    ) : status === "sold" ? (
      <Ban size={18} aria-hidden />
    ) : status === "written_off" ? (
      <Archive size={18} aria-hidden />
    ) : status === "rented" ? (
      <KeyRound size={18} aria-hidden />
    ) : (
      <X size={18} aria-hidden />
    );
  return (
    <span className={cls} title={label} aria-label={label}>
      {icon}
    </span>
  );
}

type Vehicle = {
  id: string;
  description: string;
  plate: string;
  year?: number | null;
  model?: string | null;
  color?: string | null;
  city?: string | null;
  uf?: string | null;
  primary_fuel_id?: string | null;
  primary_fuel_name?: string | null;
  tank_capacity_liters?: number | null;
  expected_km_per_liter?: number | null;
  min_km_per_liter?: number | null;
  max_km_per_liter?: number | null;
  odometer_current: number;
  cost_center_id?: string | null;
  cost_center_name?: string | null;
  status: string;
  notes?: string | null;
};

type Form = {
  description: string;
  plate: string;
  year: string;
  model: string;
  color: string;
  city: string;
  uf: string;
  primary_fuel_id: string;
  tank_capacity_liters: string;
  expected_km_per_liter: string;
  min_km_per_liter: string;
  max_km_per_liter: string;
  odometer_current: string;
  cost_center_id: string;
  status: string;
  notes: string;
};

const empty = (): Form => ({
  description: "", plate: "", year: "", model: "", color: "", city: "", uf: "",
  primary_fuel_id: "", tank_capacity_liters: "", expected_km_per_liter: "", min_km_per_liter: "",
  max_km_per_liter: "", odometer_current: "0", cost_center_id: "", status: "active", notes: "",
});

function numOrNull(s: string) {
  const t = s.trim().replace(",", ".");
  if (!t) return null;
  const n = Number(t);
  return Number.isFinite(n) ? n : null;
}

export function FleetVehiclesPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [q, setQ] = useState("");
  const [statusFilter, setStatusFilter] = useState("active");
  const [editing, setEditing] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState<Form>(empty());
  const [inactivateTarget, setInactivateTarget] = useState<Vehicle | null>(null);

  const vehicles = useQuery({
    queryKey: queryKeys.fleetVehicles,
    queryFn: () => apiFetch<{ items: Vehicle[] }>("/api/v1/fleet/vehicles"),
  });
  const fuels = useQuery({
    queryKey: queryKeys.fleetFuels,
    queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/fuels?active=1"),
  });
  const ccs = useQuery({
    queryKey: queryKeys.fleetCostCenters,
    queryFn: () => apiFetch<{ items: { id: string; description: string; code: string }[] }>("/api/v1/fleet/cost-centers"),
  });

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        description: form.description.trim(),
        plate: formatFleetPlate(form.plate),
        year: numOrNull(form.year),
        model: form.model.trim() || null,
        color: form.color.trim() || null,
        city: form.city.trim() || null,
        uf: form.uf || null,
        primary_fuel_id: form.primary_fuel_id || null,
        tank_capacity_liters: numOrNull(form.tank_capacity_liters),
        expected_km_per_liter: numOrNull(form.expected_km_per_liter),
        min_km_per_liter: numOrNull(form.min_km_per_liter),
        max_km_per_liter: numOrNull(form.max_km_per_liter),
        odometer_current: numOrNull(form.odometer_current) ?? 0,
        cost_center_id: form.cost_center_id || null,
        status: form.status,
        notes: form.notes.trim() || null,
      };
      if (editing) await apiFetch(`/api/v1/fleet/vehicles/${editing}`, { method: "PATCH", body: JSON.stringify(payload) });
      else await apiFetch("/api/v1/fleet/vehicles", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Veículo atualizado" : "Veículo criado");
      setEditing(null);
      setFormOpen(false);
      setForm(empty());
      await qc.invalidateQueries({ queryKey: queryKeys.fleetVehicles });
    },
    onError: (e) => toastErr(push, e),
  });

  const inactivate = useMutation({
    mutationFn: async (v: Vehicle) => {
      await apiFetch(`/api/v1/fleet/vehicles/${v.id}`, {
        method: "PATCH",
        json: {
          description: v.description,
          plate: formatFleetPlate(v.plate),
          year: v.year,
          model: v.model,
          color: v.color,
          city: v.city,
          uf: v.uf,
          primary_fuel_id: v.primary_fuel_id,
          tank_capacity_liters: v.tank_capacity_liters,
          expected_km_per_liter: v.expected_km_per_liter,
          min_km_per_liter: v.min_km_per_liter,
          max_km_per_liter: v.max_km_per_liter,
          odometer_current: v.odometer_current,
          cost_center_id: v.cost_center_id,
          status: "inactive",
          notes: v.notes,
        },
      });
    },
    onSuccess: async () => {
      toastOk(push, "Veículo inativado");
      setInactivateTarget(null);
      await qc.invalidateQueries({ queryKey: queryKeys.fleetVehicles });
    },
    onError: (e) => toastErr(push, e),
  });

  const reactivate = useMutation({
    mutationFn: async (v: Vehicle) => {
      await apiFetch(`/api/v1/fleet/vehicles/${v.id}`, {
        method: "PATCH",
        json: {
          description: v.description,
          plate: formatFleetPlate(v.plate),
          year: v.year,
          model: v.model,
          color: v.color,
          city: v.city,
          uf: v.uf,
          primary_fuel_id: v.primary_fuel_id,
          tank_capacity_liters: v.tank_capacity_liters,
          expected_km_per_liter: v.expected_km_per_liter,
          min_km_per_liter: v.min_km_per_liter,
          max_km_per_liter: v.max_km_per_liter,
          odometer_current: v.odometer_current,
          cost_center_id: v.cost_center_id,
          status: "active",
          notes: v.notes,
        },
      });
    },
    onSuccess: async () => {
      toastOk(push, "Veículo reativado");
      await qc.invalidateQueries({ queryKey: queryKeys.fleetVehicles });
    },
    onError: (e) => toastErr(push, e),
  });

  const filtered = (vehicles.data?.items ?? []).filter((v) => {
    if (statusFilter && v.status !== statusFilter) return false;
    const s = q.trim().toLowerCase();
    if (!s) return true;
    const plate = formatFleetPlateOrUnknown(v.plate).toLowerCase();
    const plateKey = plate.replace(/-/g, "");
    const qKey = s.replace(/-/g, "");
    return plate.includes(s) || plateKey.includes(qKey) || v.description.toLowerCase().includes(s) || (v.model ?? "").toLowerCase().includes(s);
  });

  function openCreate() {
    setEditing(null);
    setForm(empty());
    setFormOpen(true);
  }

  function closeForm() {
    setFormOpen(false);
    setEditing(null);
    setForm(empty());
  }

  function startEdit(v: Vehicle) {
    setEditing(v.id);
    setFormOpen(true);
    setForm({
      description: v.description,
      plate: formatFleetPlate(v.plate),
      year: v.year != null ? String(v.year) : "",
      model: v.model ?? "",
      color: v.color ?? "",
      city: v.city ?? "",
      uf: v.uf ?? "",
      primary_fuel_id: v.primary_fuel_id ?? "",
      tank_capacity_liters: v.tank_capacity_liters != null ? String(v.tank_capacity_liters) : "",
      expected_km_per_liter: v.expected_km_per_liter != null ? String(v.expected_km_per_liter) : "",
      min_km_per_liter: v.min_km_per_liter != null ? String(v.min_km_per_liter) : "",
      max_km_per_liter: v.max_km_per_liter != null ? String(v.max_km_per_liter) : "",
      odometer_current: String(v.odometer_current ?? 0),
      cost_center_id: v.cost_center_id ?? "",
      status: v.status,
      notes: v.notes ?? "",
    });
  }

  return (
    <div className="fleet-page">
      <h1>Frota — Veículos</h1>
      <div className="fleet-vehicles-toolbar">
        <input className="input" placeholder="Pesquisar placa/descrição…" value={q} onChange={(e) => setQ(e.target.value)} />
        <select className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} aria-label="Filtrar status">
          <option value="">Todos os status</option>
          {VEHICLE_STATUS.map((s) => (
            <option key={s.value} value={s.value}>{s.label}</option>
          ))}
        </select>
        {canMutate ? (
          <button type="button" className="btn btn--icon btn--icon-menu btn--primary" title="Novo veículo" aria-label="Novo veículo" onClick={openCreate}>
            <CirclePlus size={18} aria-hidden />
          </button>
        ) : null}
      </div>

      {formOpen && canMutate ? createPortal(
        <div className="modal-backdrop" role="presentation" onMouseDown={closeForm}>
          <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
          <h3>{editing ? "Editar veículo" : "Novo veículo"}</h3>
          <div className="fleet-form-grid">
            <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
            <label>Placa (opcional)<input className="input" placeholder="AAA-0000 ou AAA-0A00" value={form.plate} onChange={(e) => setForm({ ...form, plate: formatFleetPlate(e.target.value) })} /></label>
            <label>Ano<input className="input" value={form.year} onChange={(e) => setForm({ ...form, year: e.target.value })} /></label>
            <label>Modelo<input className="input" value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} /></label>
            <label>Cor<input className="input" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} /></label>
            <label>Cidade<input className="input" value={form.city} onChange={(e) => setForm({ ...form, city: e.target.value })} /></label>
            <label>UF<select className="input" value={form.uf} onChange={(e) => setForm({ ...form, uf: e.target.value })}><option value="">—</option>{UF_OPTIONS.map((u) => <option key={u} value={u}>{u}</option>)}</select></label>
            <label>Combustível<select className="input" value={form.primary_fuel_id} onChange={(e) => setForm({ ...form, primary_fuel_id: e.target.value })}><option value="">—</option>{(fuels.data?.items ?? []).map((f) => <option key={f.id} value={f.id}>{f.description}</option>)}</select></label>
            <label>Capacidade tanque (L)<input className="input" value={form.tank_capacity_liters} onChange={(e) => setForm({ ...form, tank_capacity_liters: e.target.value })} /></label>
            <label>Consumo esperado KM/L<input className="input" value={form.expected_km_per_liter} onChange={(e) => setForm({ ...form, expected_km_per_liter: e.target.value })} /></label>
            <label>Consumo mín. KM/L<input className="input" value={form.min_km_per_liter} onChange={(e) => setForm({ ...form, min_km_per_liter: e.target.value })} /></label>
            <label>Consumo máx. KM/L<input className="input" value={form.max_km_per_liter} onChange={(e) => setForm({ ...form, max_km_per_liter: e.target.value })} /></label>
            <label>Hodômetro atual<input className="input" value={form.odometer_current} onChange={(e) => setForm({ ...form, odometer_current: e.target.value })} /></label>
            <label>Centro de custo<select className="input" value={form.cost_center_id} onChange={(e) => setForm({ ...form, cost_center_id: e.target.value })}><option value="">—</option>{(ccs.data?.items ?? []).map((c) => <option key={c.id} value={c.id}>{c.code} — {c.description}</option>)}</select></label>
            <label>Status<select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>{VEHICLE_STATUS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}</select></label>
            <label className="fleet-form-span">Observação<input className="input" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
          </div>
          <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
            <button type="button" className="btn" onClick={closeForm}>Cancelar</button>
            <button type="button" className="btn btn--primary" disabled={save.isPending || !form.description.trim() || (!!form.plate.trim() && !isValidFleetPlate(form.plate))} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
          </div>
          </div>
        </div>,
        document.body,
      ) : null}

      <div className="card table-wrap">
        <table>
          <thead><tr><th>Placa</th><th>Descrição</th><th>Modelo</th><th>Hodômetro</th><th>Status</th><th /></tr></thead>
          <tbody>
            {filtered.map((v) => (
              <tr key={v.id}>
                <td><strong>{formatFleetPlateOrUnknown(v.plate)}</strong></td>
                <td>{v.description}</td>
                <td>{v.model ?? "—"}</td>
                <td>{v.odometer_current?.toLocaleString("pt-BR")}</td>
                <td><VehicleStatusIcon status={v.status} /></td>
                <td>
                  <ActionMenu
                    title="Opções do veículo"
                    icon={<PenLine size={16} aria-hidden />}
                    items={[
                      ...(canMutate ? [{ id: "edit", label: "Editar", onClick: () => startEdit(v) }] : []),
                      {
                        id: "dashboard",
                        label: "Dashboard",
                        onClick: () =>
                          navigate(`${APP_ROUTES.fleetDashboard}?vehicle_id=${encodeURIComponent(v.id)}`),
                      },
                      ...(canMutate && v.status !== "inactive"
                        ? [{ id: "inactivate", label: "Inativar", danger: true, onClick: () => setInactivateTarget(v) }]
                        : []),
                      ...(canMutate && v.status === "inactive"
                        ? [{ id: "reactivate", label: "Reativar", onClick: () => reactivate.mutate(v) }]
                        : []),
                    ]}
                  />
                </td>
              </tr>
            ))}
            {vehicles.isLoading ? (
              <tr><td colSpan={6} className="muted">A carregar…</td></tr>
            ) : vehicles.isError ? (
              <tr><td colSpan={6} className="err">Falha ao carregar veículos.</td></tr>
            ) : filtered.length === 0 ? (
              <tr>
                <td colSpan={6} className="muted">
                  {q.trim() || statusFilter ? "Nenhum veículo para a busca/filtro." : "Nenhum veículo cadastrado."}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <ConfirmModal
        open={!!inactivateTarget}
        title="Inativar veículo"
        message={
          inactivateTarget
            ? `Inativar ${formatFleetPlateOrUnknown(inactivateTarget.plate)} — ${inactivateTarget.description}? O veículo deixa de contar como ativo na frota.`
            : ""
        }
        confirmLabel="Inativar"
        danger
        busy={inactivate.isPending}
        onCancel={() => setInactivateTarget(null)}
        onConfirm={() => inactivateTarget && inactivate.mutate(inactivateTarget)}
      />
    </div>
  );
}
