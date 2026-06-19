import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmModal } from "../../components/ConfirmModal";
import { PageCountPill } from "../../components/PageCountPill";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { queryKeys } from "../../lib/queryKeys";
import { toastErr, toastOk } from "../../lib/operationToast";
import { filtersToQueryParams, type ConnectionsTabId } from "../../lib/connectionsFilters";
import {
  CABLE_STATUSES,
  fmtCoord,
  parseCoordInput,
  type NetworkCable,
  type NetworkCto,
  type NetworkPole,
  type NetworkSpliceBox,
  cableStatusLabel,
} from "../../lib/networkInfrastructure";
import {
  CoordFields,
  FiberColorSelect,
  LocalitySelect,
  MaintenanceSwitch,
  ProjectSelect,
} from "./ConnectionsFormFields";
import { ConnectionsPager } from "./ConnectionsPager";
import type { ConnectionsTabProps } from "./shared";
import { useConnectionsLookups } from "./useConnectionsLookups";
import { usePagedRows } from "./usePagedRows";

type Variant = "cto" | "splice" | "cable" | "pole";

type Props = ConnectionsTabProps & {
  variant: Variant;
  tabId: ConnectionsTabId;
};

const VARIANT_META: Record<
  Variant,
  {
    label: string;
    singular: string;
    api: string;
    listKey: string;
    queryKey: readonly string[];
    idPrefix: string;
    needsLocality: boolean;
  }
> = {
  cto: {
    label: "CTOs",
    singular: "CTO",
    api: "/api/v1/commercial/network/ctos",
    listKey: "ctos",
    queryKey: queryKeys.networkCtos,
    idPrefix: "CTO",
    needsLocality: true,
  },
  splice: {
    label: "Caixas de emenda",
    singular: "Caixa de emenda",
    api: "/api/v1/commercial/network/splice-boxes",
    listKey: "splice_boxes",
    queryKey: queryKeys.networkSpliceBoxes,
    idPrefix: "Emenda",
    needsLocality: false,
  },
  cable: {
    label: "Cabos",
    singular: "Cabo",
    api: "/api/v1/commercial/network/cables",
    listKey: "cables",
    queryKey: queryKeys.networkCables,
    idPrefix: "Cabo",
    needsLocality: false,
  },
  pole: {
    label: "Postes",
    singular: "Poste",
    api: "/api/v1/commercial/network/poles",
    listKey: "poles",
    queryKey: queryKeys.networkPoles,
    idPrefix: "Poste",
    needsLocality: true,
  },
};

type Row = NetworkCto | NetworkSpliceBox | NetworkCable | NetworkPole;

export function InfrastructureTab({ variant, tabId, canMutate, filters, prefs }: Props) {
  const meta = VARIANT_META[variant];
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [formOpen, setFormOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, string | boolean>>({});

  const queryParams = useMemo(() => filtersToQueryParams(filters, tabId).toString(), [filters, tabId]);

  const listQ = useQuery({
    queryKey: [...meta.queryKey, queryParams],
    queryFn: async () => {
      const qs = queryParams ? `?${queryParams}` : "";
      return apiFetch<Record<string, Row[]>>(`${meta.api}${qs}`);
    },
    placeholderData: keepPreviousData,
  });

  const { localities, projects } = useConnectionsLookups(formOpen);

  const rows = listQ.data?.[meta.listKey] ?? [];

  const filteredRows = useMemo(() => {
    let out = rows;
    if (variant === "splice" && filters.splice_boxes.fiber_count_min.trim()) {
      const min = Number(filters.splice_boxes.fiber_count_min);
      if (Number.isFinite(min)) {
        out = out.filter((r) => (r as NetworkSpliceBox).fiber_count != null && (r as NetworkSpliceBox).fiber_count! >= min);
      }
    }
    return [...out].sort((a, b) =>
      prefs.sortDir === "asc" ? a.display_number - b.display_number : b.display_number - a.display_number,
    );
  }, [rows, variant, filters.splice_boxes.fiber_count_min, prefs.sortDir]);

  const { safePage, totalPages, pageRows, setPage, rangeFrom, rangeTo } = usePagedRows(
    filteredRows,
    prefs.pageSize,
    `${queryParams}:${prefs.sortDir}`,
  );

  function emptyForm(): Record<string, string | boolean> {
    const base: Record<string, string | boolean> = {
      description: "",
      latitude: "",
      longitude: "",
      project_id: "",
      needs_maintenance: false,
      notes: "",
    };
    if (variant === "cto") {
      return { ...base, splitter: "", fiber_color: "", locality_id: "" };
    }
    if (variant === "splice") {
      return { ...base, fiber_count: "" };
    }
    if (variant === "cable") {
      return { ...base, cable_type: "", fiber_count: "", status: "ativo" };
    }
    return { ...base, pole_type: "", locality_id: "" };
  }

  function rowToForm(row: Row): Record<string, string | boolean> {
    const r = row as Record<string, unknown>;
    const f = emptyForm();
    f.description = String(r.description ?? "");
    f.latitude = r.latitude != null ? String(r.latitude) : "";
    f.longitude = r.longitude != null ? String(r.longitude) : "";
    f.project_id = r.project_id ? String(r.project_id) : "";
    if ("needs_maintenance" in r) f.needs_maintenance = Boolean(r.needs_maintenance);
    if ("notes" in r && r.notes) f.notes = String(r.notes);
    if (variant === "cto") {
      f.splitter = r.splitter ? String(r.splitter) : "";
      f.fiber_color = r.fiber_color ? String(r.fiber_color) : "";
      f.locality_id = r.locality_id ? String(r.locality_id) : "";
    }
    if (variant === "splice" && r.fiber_count != null) f.fiber_count = String(r.fiber_count);
    if (variant === "cable") {
      f.cable_type = r.cable_type ? String(r.cable_type) : "";
      f.fiber_count = r.fiber_count != null ? String(r.fiber_count) : "";
      f.status = r.status ? String(r.status) : "ativo";
    }
    if (variant === "pole") {
      f.pole_type = r.pole_type ? String(r.pole_type) : "";
      f.locality_id = r.locality_id ? String(r.locality_id) : "";
    }
    return f;
  }

  function formToPayload() {
    const lat = parseCoordInput(String(form.latitude));
    const lon = parseCoordInput(String(form.longitude));
    const payload: Record<string, unknown> = {
      description: String(form.description).trim(),
      latitude: lat,
      longitude: lon,
      project_id: String(form.project_id).trim() || null,
    };
    if (variant === "cto" || variant === "splice") {
      payload.needs_maintenance = Boolean(form.needs_maintenance);
      payload.notes = String(form.notes).trim() || null;
    }
    if (variant === "cto") {
      payload.splitter = String(form.splitter).trim() || null;
      payload.fiber_color = String(form.fiber_color).trim() || null;
      payload.locality_id = String(form.locality_id).trim() || null;
    }
    if (variant === "splice") {
      const fc = String(form.fiber_count).trim();
      payload.fiber_count = fc ? Number(fc) : null;
    }
    if (variant === "cable") {
      payload.cable_type = String(form.cable_type).trim() || null;
      const fc = String(form.fiber_count).trim();
      payload.fiber_count = fc ? Number(fc) : null;
      payload.status = String(form.status).trim() || "ativo";
    }
    if (variant === "pole") {
      payload.pole_type = String(form.pole_type).trim() || null;
      payload.locality_id = String(form.locality_id).trim() || null;
    }
    return payload;
  }

  const saveMut = useMutation({
    mutationFn: async () => {
      const payload = formToPayload();
      if (!payload.description) throw new Error("Descrição obrigatória.");
      if (editId) {
        return apiFetch(`${meta.api}/${editId}`, { method: "PATCH", json: payload });
      }
      return apiFetch(meta.api, { method: "POST", json: payload });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...meta.queryKey] });
      qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      setFormOpen(false);
      setEditId(null);
      setForm(emptyForm());
      toastOk(pushToast, editId ? `${meta.singular} actualizada.` : `${meta.singular} criada.`);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar."),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => apiFetch(`${meta.api}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [meta.queryKey] });
      setDeleteId(null);
      toastOk(pushToast, `${meta.singular} removida.`);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao remover."),
  });

  if (listQ.isPending && !listQ.data) return <p>A carregar {meta.label.toLowerCase()}…</p>;
  if (listQ.isError && !listQ.data) return <div className="msg msg--err">{errorMessageFromUnknown(listQ.error)}</div>;

  return (
    <>
      <div className="conn-toolbar">
        <PageCountPill label={meta.label} count={filteredRows.length} />
        {canMutate ? (
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => {
              setEditId(null);
              setForm(emptyForm());
              setFormOpen(true);
            }}
          >
            Nova {meta.singular.toLowerCase()}
          </button>
        ) : null}
      </div>

      <div className="table-wrap">
        <table className="conn-table" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              <th>ID</th>
              <th>Descrição</th>
              {variant === "cto" ? (
                <>
                  <th>Splitter</th>
                  <th>Cor fibra</th>
                  <th>Localidade</th>
                  <th>Manutenção</th>
                </>
              ) : null}
              {variant === "splice" ? (
                <>
                  <th>Fibras</th>
                  <th>Manutenção</th>
                </>
              ) : null}
              {variant === "cable" ? (
                <>
                  <th>Tipo</th>
                  <th>Fibras</th>
                  <th>Status</th>
                </>
              ) : null}
              {variant === "pole" ? (
                <>
                  <th>Tipo</th>
                  <th>Localidade</th>
                </>
              ) : null}
              <th>Projeto</th>
              <th className="mono">Coord.</th>
              {canMutate ? <th style={{ width: 80 }} /> : null}
            </tr>
          </thead>
          <tbody>
            {pageRows.map((row) => {
              const r = row as Record<string, unknown>;
              return (
                <tr key={String(r.id)}>
                  <td className="mono">
                    {meta.idPrefix} {row.display_number}
                  </td>
                  <td>{row.description}</td>
                  {variant === "cto" ? (
                    <>
                      <td>{(r.splitter as string) ?? "—"}</td>
                      <td>{(r.fiber_color as string) ?? "—"}</td>
                      <td>{(r.locality_name as string) ?? "—"}</td>
                      <td>{r.needs_maintenance ? "Sim" : "Não"}</td>
                    </>
                  ) : null}
                  {variant === "splice" ? (
                    <>
                      <td>{(r.fiber_count as number) ?? "—"}</td>
                      <td>{r.needs_maintenance ? "Sim" : "Não"}</td>
                    </>
                  ) : null}
                  {variant === "cable" ? (
                    <>
                      <td>{(r.cable_type as string) ?? "—"}</td>
                      <td>{(r.fiber_count as number) ?? "—"}</td>
                      <td>{cableStatusLabel(String(r.status ?? ""))}</td>
                    </>
                  ) : null}
                  {variant === "pole" ? (
                    <>
                      <td>{(r.pole_type as string) ?? "—"}</td>
                      <td>{(r.locality_name as string) ?? "—"}</td>
                    </>
                  ) : null}
                  <td>{(r.project_label as string) ?? "—"}</td>
                  <td className="mono">
                    {r.latitude != null && r.longitude != null
                      ? `${fmtCoord(r.latitude as number)}, ${fmtCoord(r.longitude as number)}`
                      : "—"}
                  </td>
                  {canMutate ? (
                    <td>
                      <div className="conn-row-actions">
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Editar"
                          onClick={() => {
                            setEditId(String(r.id));
                            setForm(rowToForm(row));
                            setFormOpen(true);
                          }}
                        >
                          <Pencil size={15} />
                        </button>
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Remover"
                          onClick={() => setDeleteId(String(r.id))}
                        >
                          <Trash2 size={15} />
                        </button>
                      </div>
                    </td>
                  ) : null}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <ConnectionsPager
        safePage={safePage}
        totalPages={totalPages}
        total={filteredRows.length}
        rangeFrom={rangeFrom}
        rangeTo={rangeTo}
        onPrev={() => setPage((p) => p - 1)}
        onNext={() => setPage((p) => p + 1)}
      />

      {formOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !saveMut.isPending && setFormOpen(false)}>
          <div className="modal conn-form-modal" role="dialog" onMouseDown={(e) => e.stopPropagation()}>
            <h3>{editId ? `Editar ${meta.singular}` : `Nova ${meta.singular}`}</h3>
            {editId ? (
              <p style={{ fontSize: 12, color: "var(--muted)" }}>
                O ID numérico é permanente e não pode ser alterado.
              </p>
            ) : (
              <p style={{ fontSize: 12, color: "var(--muted)" }}>O ID será atribuído automaticamente na criação.</p>
            )}
            <label className="field">
              Descrição *
              <input className="input" value={String(form.description)} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </label>
            <CoordFields
              latitude={String(form.latitude)}
              longitude={String(form.longitude)}
              onChange={(lat, lon) => setForm({ ...form, latitude: lat, longitude: lon })}
            />
            <ProjectSelect
              value={String(form.project_id)}
              projects={projects}
              onChange={(id) => setForm({ ...form, project_id: id })}
            />
            {variant === "cto" ? (
              <>
                <LocalitySelect
                  value={String(form.locality_id)}
                  localities={localities}
                  onChange={(id) => setForm({ ...form, locality_id: id })}
                />
                <label className="field">
                  Splitter
                  <input className="input" value={String(form.splitter)} onChange={(e) => setForm({ ...form, splitter: e.target.value })} />
                </label>
                <FiberColorSelect
                  value={String(form.fiber_color)}
                  onChange={(v) => setForm({ ...form, fiber_color: v })}
                />
                <label className="field">
                  Observações
                  <textarea className="input" rows={2} value={String(form.notes)} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
                </label>
                <MaintenanceSwitch
                  checked={Boolean(form.needs_maintenance)}
                  onChange={(v) => setForm({ ...form, needs_maintenance: v })}
                />
              </>
            ) : null}
            {variant === "splice" ? (
              <>
                <label className="field">
                  Quantidade de fibras
                  <input className="input" type="number" min={0} value={String(form.fiber_count)} onChange={(e) => setForm({ ...form, fiber_count: e.target.value })} />
                </label>
                <label className="field">
                  Observações
                  <textarea className="input" rows={2} value={String(form.notes)} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
                </label>
                <MaintenanceSwitch
                  checked={Boolean(form.needs_maintenance)}
                  onChange={(v) => setForm({ ...form, needs_maintenance: v })}
                />
              </>
            ) : null}
            {variant === "cable" ? (
              <>
                <label className="field">
                  Tipo
                  <input className="input" value={String(form.cable_type)} onChange={(e) => setForm({ ...form, cable_type: e.target.value })} />
                </label>
                <label className="field">
                  Quantidade de fibras
                  <input className="input" type="number" min={0} value={String(form.fiber_count)} onChange={(e) => setForm({ ...form, fiber_count: e.target.value })} />
                </label>
                <label className="field">
                  Status
                  <select className="input" value={String(form.status)} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                    {CABLE_STATUSES.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </label>
              </>
            ) : null}
            {variant === "pole" ? (
              <>
                <label className="field">
                  Tipo
                  <input className="input" value={String(form.pole_type)} onChange={(e) => setForm({ ...form, pole_type: e.target.value })} />
                </label>
                <LocalitySelect
                  value={String(form.locality_id)}
                  localities={localities}
                  onChange={(id) => setForm({ ...form, locality_id: id })}
                />
              </>
            ) : null}
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setFormOpen(false)} disabled={saveMut.isPending}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" onClick={() => saveMut.mutate()} disabled={saveMut.isPending}>
                Guardar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleteId ? (
        <ConfirmModal
          title={`Remover ${meta.singular}`}
          message="Esta acção não pode ser desfeita. O ID numérico não será reutilizado."
          confirmLabel="Remover"
          danger
          onCancel={() => setDeleteId(null)}
          onConfirm={() => deleteMut.mutate(deleteId)}
          busy={deleteMut.isPending}
        />
      ) : null}
    </>
  );
}
