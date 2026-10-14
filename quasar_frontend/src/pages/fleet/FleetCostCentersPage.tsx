import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { createPortal } from "react-dom";
import { Plus } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { ExportMenu } from "../../components/ExportMenu";

type CC = { id: string; code: string; description: string; parent_id?: string | null; status: string; notes?: string | null };

const empty = () => ({ code: "", description: "", parent_id: "", status: "active", notes: "" });

export function FleetCostCentersPage({ embedded }: { embedded?: boolean } = {}) {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [editing, setEditing] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState(empty());
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
      toastOk(push, editing ? "Centro de custo atualizado" : "Centro de custo criado");
      closeForm();
      await qc.invalidateQueries({ queryKey: queryKeys.fleetCostCenters });
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
        <div className="row" style={{ gap: 10, alignItems: "center" }}>
          {embedded ? null : <h1 style={{ margin: 0 }}>Frota — Centros de custo</h1>}
          <ExportMenu
            filename="frota-centros-de-custo"
            title="Frota — Centros de custo"
            rows={q.data?.items ?? []}
            columns={[
              { header: "Código", value: (c) => c.code },
              { header: "Descrição", value: (c) => c.description },
              { header: "Status", value: (c) => (c.status === "active" ? "Ativo" : c.status === "inactive" ? "Inativo" : c.status) },
              { header: "Observação", value: (c) => c.notes },
            ]}
          />
        </div>
        {canMutate ? (
          <button
            type="button"
            className="btn btn--icon btn--icon-menu btn--primary"
            title="Novo centro de custo"
            aria-label="Novo centro de custo"
            onClick={() => { setEditing(null); setForm(empty()); setFormOpen(true); }}
          >
            <Plus size={18} aria-hidden />
          </button>
        ) : null}
      </div>

      {formOpen && canMutate ? createPortal(
        <div className="modal-backdrop" role="presentation" onMouseDown={closeForm}>
          <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
            <h3>{editing ? "Editar centro de custo" : "Novo centro de custo"}</h3>
            <div className="fleet-form-grid">
              <label>Código*<input className="input" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} /></label>
              <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
              <label>Pai<select className="input" value={form.parent_id} onChange={(e) => setForm({ ...form, parent_id: e.target.value })}>
                <option value="">Nenhum</option>
                {(q.data?.items ?? []).filter((c) => c.id !== editing).map((c) => <option key={c.id} value={c.id}>{c.code} — {c.description}</option>)}
              </select></label>
              <label>Status<select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                <option value="active">Ativo</option><option value="inactive">Inativo</option>
              </select></label>
              <label className="fleet-form-span">Observação<input className="input" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
            </div>
            <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
              <button type="button" className="btn" onClick={closeForm}>Cancelar</button>
              <button type="button" className="btn btn--primary" disabled={!form.code.trim() || !form.description.trim() || save.isPending} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
            </div>
          </div>
        </div>,
        document.body,
      ) : null}

      <div className="card table-wrap">
        <table>
          <thead><tr><th>Código</th><th>Descrição</th><th>Status</th><th /></tr></thead>
          <tbody>
            {(q.data?.items ?? []).map((c) => (
              <tr key={c.id}>
                <td>{c.code}</td><td>{c.description}</td><td>{c.status === "active" ? "Ativo" : c.status === "inactive" ? "Inativo" : c.status}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => {
                  setEditing(c.id);
                  setFormOpen(true);
                  setForm({ code: c.code, description: c.description, parent_id: c.parent_id ?? "", status: c.status, notes: c.notes ?? "" });
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
