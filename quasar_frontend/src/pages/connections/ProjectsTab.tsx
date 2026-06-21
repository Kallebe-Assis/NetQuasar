import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, MapPin, Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmModal } from "../../components/ConfirmModal";
import { LocationMapModal, type LocationMapPreview } from "../../components/LocationMapModal";
import { PageCountPill } from "../../components/PageCountPill";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { queryKeys } from "../../lib/queryKeys";
import { toastErr, toastOk } from "../../lib/operationToast";
import { filterProjectRows, NETWORK_INFRA_GC_MS, NETWORK_INFRA_STALE_MS } from "../../lib/networkInfraCache";
import {
  PROJECT_STATUSES,
  fmtCoord,
  parseCoordInput,
  projectStatusLabel,
  type NetworkProject,
} from "../../lib/networkInfrastructure";
import { CoordFields, LocalitySelect } from "./ConnectionsFormFields";
import { ConnectionsPager } from "./ConnectionsPager";
import { ConnectionsTabToolbar } from "./ConnectionsTabToolbar";
import type { ConnectionsTabProps } from "./shared";
import { useConnectionsLookups } from "./useConnectionsLookups";
import { usePagedRows } from "./usePagedRows";

const EMPTY = {
  description: "",
  locality_id: "",
  color: "#3b82f6",
  status: "planejamento",
  latitude: "",
  longitude: "",
};

export function ProjectsTab({
  canMutate,
  filters,
  prefs,
  onSearchChange,
  onOpenFilters,
  onOpenSettings,
  activeFilterCount,
}: ConnectionsTabProps) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [formOpen, setFormOpen] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [editId, setEditId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [form, setForm] = useState(EMPTY);
  const [mapPreview, setMapPreview] = useState<LocationMapPreview | null>(null);

  const debouncedQ = useDebouncedValue(filters.q, 320);
  const filterKey = useMemo(
    () =>
      JSON.stringify({
        q: debouncedQ,
        locality_id: filters.locality_id,
        status: filters.projects.status,
        sortDir: prefs.sortDir,
      }),
    [debouncedQ, filters.locality_id, filters.projects.status, prefs.sortDir],
  );

  const listQ = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: async () => apiFetch<{ projects: NetworkProject[] }>("/api/v1/commercial/network/projects"),
    staleTime: NETWORK_INFRA_STALE_MS,
    gcTime: NETWORK_INFRA_GC_MS,
    refetchOnWindowFocus: false,
    placeholderData: keepPreviousData,
  });

  const sorted = useMemo(() => {
    const rows = filterProjectRows(listQ.data?.projects ?? [], filters, debouncedQ);
    return [...rows].sort((a, b) =>
      prefs.sortDir === "asc" ? a.display_number - b.display_number : b.display_number - a.display_number,
    );
  }, [listQ.data?.projects, filters, debouncedQ, prefs.sortDir]);

  const detailQ = useQuery({
    queryKey: ["network-project", detailId],
    queryFn: () => apiFetch<NetworkProject>(`/api/v1/commercial/network/projects/${detailId}`),
    enabled: !!detailId,
  });

  const { localities } = useConnectionsLookups(formOpen);

  const { safePage, totalPages, pageRows, setPage, rangeFrom, rangeTo } = usePagedRows(
    sorted,
    prefs.pageSize,
    filterKey,
  );

  async function reloadFromDb() {
    try {
      const r = await listQ.refetch();
      if (r.error) {
        toastErr(pushToast, r.error);
      } else {
        toastOk(pushToast, "Projetos recarregados da base de dados.");
      }
    } catch (e) {
      toastErr(pushToast, e);
    }
  }

  const saveMut = useMutation({
    mutationFn: async () => {
      const payload = {
        description: form.description.trim(),
        locality_id: form.locality_id.trim() || null,
        color: form.color.trim() || null,
        status: form.status,
        latitude: parseCoordInput(form.latitude),
        longitude: parseCoordInput(form.longitude),
      };
      if (!payload.description) throw new Error("Descrição obrigatória.");
      if (editId) return apiFetch(`/api/v1/commercial/network/projects/${editId}`, { method: "PATCH", json: payload });
      return apiFetch("/api/v1/commercial/network/projects", { method: "POST", json: payload });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      setFormOpen(false);
      setEditId(null);
      setForm(EMPTY);
      toastOk(pushToast, editId ? "Projeto actualizado." : "Projeto criado.");
    },
    onError: (e) => toastErr(pushToast, e),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/network/projects/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      setDeleteId(null);
      toastOk(pushToast, "Projeto removido.");
    },
    onError: (e) => toastErr(pushToast, e),
  });

  if (listQ.isPending && !listQ.data) return <p>A carregar projetos…</p>;
  if (listQ.isError && !listQ.data) return <div className="msg msg--err">{errorMessageFromUnknown(listQ.error)}</div>;

  const detail = detailQ.data;

  function renderElements(title: string, items?: Array<{ display_number: number; description: string }>, prefix?: string) {
    if (!items?.length) return null;
    return (
      <div style={{ marginTop: 10 }}>
        <strong style={{ fontSize: 12 }}>{title}</strong>
        <ul style={{ margin: "6px 0 0", paddingLeft: 18, fontSize: 12 }}>
          {items.map((el) => (
            <li key={`${prefix}-${el.display_number}`}>
              {prefix} {el.display_number} — {el.description}
            </li>
          ))}
        </ul>
      </div>
    );
  }

  return (
    <>
      <ConnectionsTabToolbar
        search={filters.q}
        onSearchChange={onSearchChange}
        searchPlaceholder="Descrição, ID do projeto…"
        onOpenFilters={onOpenFilters}
        onOpenSettings={onOpenSettings}
        activeFilterCount={activeFilterCount}
        onReload={() => void reloadFromDb()}
        reloading={listQ.isFetching}
        reloadTitle="Recarregar projetos da base de dados"
      >
        <PageCountPill label="Projetos" count={sorted.length} />
        {canMutate ? (
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => {
              setEditId(null);
              setForm(EMPTY);
              setFormOpen(true);
            }}
          >
            Novo projeto
          </button>
        ) : null}
      </ConnectionsTabToolbar>

      <div className="table-wrap">
        <table className="conn-table conn-table--center" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              <th>ID</th>
              <th>Descrição</th>
              <th>Localidade</th>
              <th>Status</th>
              <th>Cor</th>
              <th className="mono">Coord.</th>
              <th style={{ width: 110 }} />
            </tr>
          </thead>
          <tbody>
            {pageRows.map((p) => {
              const hasCoords = p.latitude != null && p.longitude != null;
              return (
              <tr key={p.id}>
                <td className="mono">Projeto {p.display_number}</td>
                <td>{p.description}</td>
                <td>{p.locality_name ?? "—"}</td>
                <td>{projectStatusLabel(p.status)}</td>
                <td>
                  {p.color ? (
                    <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                      <span style={{ width: 12, height: 12, borderRadius: 3, background: p.color, border: "1px solid var(--border)" }} />
                      {p.color}
                    </span>
                  ) : (
                    "—"
                  )}
                </td>
                <td className="mono">
                  {hasCoords ? `${fmtCoord(p.latitude!)}, ${fmtCoord(p.longitude!)}` : "—"}
                </td>
                <td>
                  <div className="conn-row-actions">
                    {hasCoords ? (
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Ver no mapa"
                        onClick={() =>
                          setMapPreview({
                            title: p.description,
                            subtitle: `Projeto ${p.display_number}`,
                            lat: p.latitude!,
                            lng: p.longitude!,
                            kind: "project",
                            color: p.color,
                          })
                        }
                      >
                        <MapPin size={15} />
                      </button>
                    ) : null}
                    <button type="button" className="btn btn--icon" title="Ver elementos" onClick={() => setDetailId(p.id)}>
                      <Eye size={15} />
                    </button>
                    {canMutate ? (
                      <>
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Editar"
                          onClick={() => {
                            setEditId(p.id);
                            setForm({
                              description: p.description,
                              locality_id: p.locality_id ?? "",
                              color: p.color ?? "#3b82f6",
                              status: p.status,
                              latitude: p.latitude != null ? String(p.latitude) : "",
                              longitude: p.longitude != null ? String(p.longitude) : "",
                            });
                            setFormOpen(true);
                          }}
                        >
                          <Pencil size={15} />
                        </button>
                        <button type="button" className="btn btn--icon" title="Remover" onClick={() => setDeleteId(p.id)}>
                          <Trash2 size={15} />
                        </button>
                      </>
                    ) : null}
                  </div>
                </td>
              </tr>
            );
            })}
          </tbody>
        </table>
      </div>

      <ConnectionsPager
        safePage={safePage}
        totalPages={totalPages}
        total={sorted.length}
        rangeFrom={rangeFrom}
        rangeTo={rangeTo}
        onPrev={() => setPage((p) => p - 1)}
        onNext={() => setPage((p) => p + 1)}
      />

      {formOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !saveMut.isPending && setFormOpen(false)}>
          <div
            className="modal conn-form-modal conn-form-modal--infra"
            role="dialog"
            aria-modal="true"
            aria-labelledby="project-form-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="conn-form-modal__head">
              <h2 id="project-form-title">{editId ? "Editar projeto" : "Novo projeto"}</h2>
              <p>Agrupa CTOs, emendas, cabos e postes numa área de implantação.</p>
            </div>
            <div className="conn-form-modal__body">
              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Identificação</h3>
                <div className="conn-form-modal__grid">
                  <div className="conn-form-modal__field field--full">
                    <span className="conn-form-modal__field-label">Descrição *</span>
                    <input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
                  </div>
                  <LocalitySelect
                    value={form.locality_id}
                    localities={localities}
                    onChange={(id) => setForm({ ...form, locality_id: id })}
                  />
                  <div className="conn-form-modal__field">
                    <span className="conn-form-modal__field-label">Cor</span>
                    <input className="input" type="color" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} />
                  </div>
                  <div className="conn-form-modal__field">
                    <span className="conn-form-modal__field-label">Status</span>
                    <select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                      {PROJECT_STATUSES.map((s) => (
                        <option key={s.value} value={s.value}>
                          {s.label}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              </section>
              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Localização</h3>
                <div className="conn-form-modal__grid">
                  <CoordFields
                    latitude={form.latitude}
                    longitude={form.longitude}
                    onChange={(lat, lon) => setForm({ ...form, latitude: lat, longitude: lon })}
                  />
                </div>
              </section>
            </div>
            <div className="conn-form-modal__foot">
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

      {detailId && detail ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setDetailId(null)}>
          <div className="modal modal--wide" onMouseDown={(e) => e.stopPropagation()}>
            <h3>
              Projeto {detail.display_number} — {detail.description}
            </h3>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              {projectStatusLabel(detail.status)}
              {detail.locality_name ? ` · ${detail.locality_name}` : ""}
            </p>
            {renderElements("CTOs", detail.elements?.ctos, "CTO")}
            {renderElements("Caixas de emenda", detail.elements?.splice_boxes, "Emenda")}
            {renderElements("Cabos", detail.elements?.cables, "Cabo")}
            {renderElements("Postes", detail.elements?.poles, "Poste")}
            {!detail.elements?.ctos?.length &&
            !detail.elements?.splice_boxes?.length &&
            !detail.elements?.cables?.length &&
            !detail.elements?.poles?.length ? (
              <p style={{ color: "var(--muted)", fontSize: 12 }}>Nenhum elemento vinculado a este projeto.</p>
            ) : null}
            <div className="row" style={{ justifyContent: "flex-end", marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setDetailId(null)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleteId ? (
        <ConfirmModal
          open
          title="Remover projeto"
          message="Os elementos vinculados permanecem no sistema, mas deixam de estar associados a este projeto."
          confirmLabel="Remover"
          danger
          onCancel={() => setDeleteId(null)}
          onConfirm={() => deleteMut.mutate(deleteId)}
          busy={deleteMut.isPending}
        />
      ) : null}

      <LocationMapModal preview={mapPreview} onClose={() => setMapPreview(null)} />
    </>
  );
}
