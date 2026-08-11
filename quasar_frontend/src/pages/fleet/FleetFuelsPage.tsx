import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { createPortal } from "react-dom";
import { Plus } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type Fuel = { id: string; description: string; code?: string | null; fuel_type?: string | null; unit: string; active: boolean; notes?: string | null };

const empty = () => ({ description: "", code: "", fuel_type: "", unit: "L", active: true, notes: "" });

export function FleetFuelsPage({ embedded }: { embedded?: boolean } = {}) {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [editing, setEditing] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState(empty());
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
      if (editing) await apiFetch(`/api/v1/fleet/fuels/${editing}`, { method: "PATCH", json: payload });
      else await apiFetch("/api/v1/fleet/fuels", { method: "POST", json: payload });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Combustível atualizado" : "Combustível criado");
      closeForm();
      await qc.invalidateQueries({ queryKey: queryKeys.fleetFuels });
    },
    onError: (e) => toastErr(push, e),
  });

  function closeForm() {
    setFormOpen(false);
    setEditing(null);
    setForm(empty());
  }

  const body = (
    <>
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 12, gap: 10 }}>
        {embedded ? <div /> : <h1 style={{ margin: 0 }}>Frota — Combustíveis</h1>}
        {canMutate ? (
          <button
            type="button"
            className="btn btn--icon btn--icon-menu btn--primary"
            title="Novo combustível"
            aria-label="Novo combustível"
            onClick={() => { setEditing(null); setForm(empty()); setFormOpen(true); }}
          >
            <Plus size={18} aria-hidden />
          </button>
        ) : null}
      </div>

      {formOpen && canMutate ? createPortal(
        <div className="modal-backdrop" role="presentation" onMouseDown={closeForm}>
          <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
            <h3>{editing ? "Editar combustível" : "Novo combustível"}</h3>
            <div className="fleet-form-grid">
              <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
              <label>Código<input className="input" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} /></label>
              <label>Tipo<input className="input" value={form.fuel_type} onChange={(e) => setForm({ ...form, fuel_type: e.target.value })} /></label>
              <label>Unidade<input className="input" value={form.unit} onChange={(e) => setForm({ ...form, unit: e.target.value })} /></label>
              <label className="row" style={{ alignItems: "center", gap: 8 }}>
                <input type="checkbox" checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />
                Ativo
              </label>
              <label className="fleet-form-span">Observação<input className="input" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
            </div>
            <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
              <button type="button" className="btn" onClick={closeForm}>Cancelar</button>
              <button type="button" className="btn btn--primary" disabled={!form.description.trim() || save.isPending} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
            </div>
          </div>
        </div>,
        document.body,
      ) : null}

      <div className="card table-wrap">
        <table>
          <thead><tr><th>Descrição</th><th>Código</th><th>Tipo</th><th>Unidade</th><th>Ativo</th><th /></tr></thead>
          <tbody>
            {(q.data?.items ?? []).map((f) => (
              <tr key={f.id}>
                <td>{f.description}</td>
                <td>{f.code ?? "—"}</td>
                <td>{f.fuel_type ?? "—"}</td>
                <td>{f.unit}</td>
                <td>{f.active ? "Sim" : "Não"}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => {
                  setEditing(f.id);
                  setFormOpen(true);
                  setForm({ description: f.description, code: f.code ?? "", fuel_type: f.fuel_type ?? "", unit: f.unit, active: f.active, notes: f.notes ?? "" });
                }}>Editar</button> : null}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );

  if (embedded) return body;
  return <div className="fleet-page">{body}</div>;
}
