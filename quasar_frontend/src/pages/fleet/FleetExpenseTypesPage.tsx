import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { createPortal } from "react-dom";
import { Plus } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type ExpenseType = { id: string; description: string; code?: string | null; active: boolean; notes?: string | null };

const empty = () => ({ description: "", code: "", active: true, notes: "" });

export function FleetExpenseTypesPage({ embedded }: { embedded?: boolean } = {}) {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [editing, setEditing] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState(empty());
  const q = useQuery({
    queryKey: queryKeys.fleetExpenseTypes,
    queryFn: () => apiFetch<{ items: ExpenseType[] }>("/api/v1/fleet/expense-types"),
  });

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        description: form.description.trim(),
        code: form.code.trim() || null,
        active: form.active,
        notes: form.notes.trim() || null,
      };
      if (editing) await apiFetch(`/api/v1/fleet/expense-types/${editing}`, { method: "PATCH", body: JSON.stringify(payload) });
      else await apiFetch("/api/v1/fleet/expense-types", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Tipo atualizado" : "Tipo criado");
      closeForm();
      await qc.invalidateQueries({ queryKey: queryKeys.fleetExpenseTypes });
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
        {embedded ? <div /> : <h1 style={{ margin: 0 }}>Frota — Tipos de despesa</h1>}
        {canMutate ? (
          <button
            type="button"
            className="btn btn--icon btn--icon-menu btn--primary"
            title="Novo tipo de despesa"
            aria-label="Novo tipo de despesa"
            onClick={() => { setEditing(null); setForm(empty()); setFormOpen(true); }}
          >
            <Plus size={18} aria-hidden />
          </button>
        ) : null}
      </div>

      {formOpen && canMutate ? createPortal(
        <div className="modal-backdrop" role="presentation" onMouseDown={closeForm}>
          <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
            <h3>{editing ? "Editar tipo de despesa" : "Novo tipo de despesa"}</h3>
            <div className="fleet-form-grid">
              <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
              <label>Código<input className="input" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} /></label>
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
          <thead><tr><th>Descrição</th><th>Código</th><th>Ativo</th><th /></tr></thead>
          <tbody>
            {(q.data?.items ?? []).map((t) => (
              <tr key={t.id}>
                <td>{t.description}</td>
                <td>{t.code ?? "—"}</td>
                <td>{t.active ? "Sim" : "Não"}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => {
                  setEditing(t.id);
                  setFormOpen(true);
                  setForm({ description: t.description, code: t.code ?? "", active: t.active, notes: t.notes ?? "" });
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
