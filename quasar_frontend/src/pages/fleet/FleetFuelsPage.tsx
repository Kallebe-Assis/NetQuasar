import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type Fuel = { id: string; description: string; code?: string | null; fuel_type?: string | null; unit: string; active: boolean; notes?: string | null };

export function FleetFuelsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [form, setForm] = useState({ description: "", code: "", fuel_type: "", unit: "L", active: true, notes: "" });
  const [editing, setEditing] = useState<string | null>(null);
  const q = useQuery({ queryKey: queryKeys.fleetFuels, queryFn: () => apiFetch<{ items: Fuel[] }>("/api/v1/fleet/fuels") });

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        description: form.description.trim(),
        code: form.code.trim() || null,
        fuel_type: form.fuel_type.trim() || null,
        unit: form.unit || "L",
        active: form.active,
        notes: form.notes.trim() || null,
      };
      if (editing) await apiFetch(`/api/v1/fleet/fuels/${editing}`, { method: "PATCH", body: JSON.stringify(payload) });
      else await apiFetch("/api/v1/fleet/fuels", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Actualizado" : "Criado");
      setEditing(null);
      setForm({ description: "", code: "", fuel_type: "", unit: "L", active: true, notes: "" });
      await qc.invalidateQueries({ queryKey: queryKeys.fleetFuels });
    },
    onError: (e) => toastErr(push, e),
  });

  return (
    <div className="fleet-page">
      <h1>Frota — Combustíveis</h1>
      {canMutate ? (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="fleet-form-grid">
            <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
            <label>Código<input className="input" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} /></label>
            <label>Tipo<input className="input" value={form.fuel_type} onChange={(e) => setForm({ ...form, fuel_type: e.target.value })} /></label>
            <label>Unidade<input className="input" value={form.unit} onChange={(e) => setForm({ ...form, unit: e.target.value })} /></label>
            <label className="row"><input type="checkbox" checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} /> Activo</label>
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button type="button" className="btn btn--primary" disabled={!form.description.trim() || save.isPending} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
            {editing ? <button type="button" className="btn" onClick={() => { setEditing(null); setForm({ description: "", code: "", fuel_type: "", unit: "L", active: true, notes: "" }); }}>Cancelar</button> : null}
          </div>
        </div>
      ) : null}
      <div className="card table-wrap">
        <table>
          <thead><tr><th>Descrição</th><th>Código</th><th>Tipo</th><th>Unidade</th><th>Activo</th><th /></tr></thead>
          <tbody>
            {(q.data?.items ?? []).map((f) => (
              <tr key={f.id}>
                <td>{f.description}</td><td>{f.code ?? "—"}</td><td>{f.fuel_type ?? "—"}</td><td>{f.unit}</td><td>{f.active ? "Sim" : "Não"}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => { setEditing(f.id); setForm({ description: f.description, code: f.code ?? "", fuel_type: f.fuel_type ?? "", unit: f.unit, active: f.active, notes: f.notes ?? "" }); }}>Editar</button> : null}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
