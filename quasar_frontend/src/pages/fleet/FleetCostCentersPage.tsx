import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type CC = { id: string; code: string; description: string; parent_id?: string | null; status: string; notes?: string | null };

export function FleetCostCentersPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState({ code: "", description: "", parent_id: "", status: "active", notes: "" });
  const q = useQuery({ queryKey: queryKeys.fleetCostCenters, queryFn: () => apiFetch<{ items: CC[] }>("/api/v1/fleet/cost-centers") });

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        code: form.code.trim(),
        description: form.description.trim(),
        parent_id: form.parent_id || null,
        status: form.status,
        notes: form.notes.trim() || null,
      };
      if (editing) await apiFetch(`/api/v1/fleet/cost-centers/${editing}`, { method: "PATCH", body: JSON.stringify(payload) });
      else await apiFetch("/api/v1/fleet/cost-centers", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Actualizado" : "Criado");
      setEditing(null);
      setForm({ code: "", description: "", parent_id: "", status: "active", notes: "" });
      await qc.invalidateQueries({ queryKey: queryKeys.fleetCostCenters });
    },
    onError: (e) => toastErr(push, e),
  });

  return (
    <div className="fleet-page">
      <h1>Frota — Centros de custo</h1>
      {canMutate ? (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="fleet-form-grid">
            <label>Código*<input className="input" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} /></label>
            <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
            <label>Pai<select className="input" value={form.parent_id} onChange={(e) => setForm({ ...form, parent_id: e.target.value })}>
              <option value="">Nenhum</option>
              {(q.data?.items ?? []).filter((c) => c.id !== editing).map((c) => <option key={c.id} value={c.id}>{c.code} — {c.description}</option>)}
            </select></label>
            <label>Status<select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
              <option value="active">Activo</option><option value="inactive">Inactivo</option>
            </select></label>
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button type="button" className="btn btn--primary" disabled={!form.code.trim() || !form.description.trim() || save.isPending} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
            {editing ? <button type="button" className="btn" onClick={() => setEditing(null)}>Cancelar</button> : null}
          </div>
        </div>
      ) : null}
      <div className="card table-wrap">
        <table>
          <thead><tr><th>Código</th><th>Descrição</th><th>Status</th><th /></tr></thead>
          <tbody>
            {(q.data?.items ?? []).map((c) => (
              <tr key={c.id}>
                <td>{c.code}</td><td>{c.description}</td><td>{c.status}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => { setEditing(c.id); setForm({ code: c.code, description: c.description, parent_id: c.parent_id ?? "", status: c.status, notes: c.notes ?? "" }); }}>Editar</button> : null}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
