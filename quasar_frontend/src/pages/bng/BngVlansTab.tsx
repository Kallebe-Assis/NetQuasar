import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDownAZ, ArrowUpAZ, ChevronLeft, ChevronRight, Eye, Pencil, Plus, RefreshCw, Trash2, Users } from "lucide-react";
import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ConfirmModal } from "../../components/ConfirmModal";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { can, isAdminUser } from "../../lib/auth";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

export type VlanKind = "pppoe" | "gerencia" | "transporte";
export type VlanStatus = "active" | "inactive";

export type NetworkVlan = {
  id?: string;
  vlan_id: string;
  name: string;
  description: string;
  kind: VlanKind | string;
  status: VlanStatus | string;
  capacity?: number | null;
  connections: number;
  equipment: number;
  utilization?: number | null;
  catalogued: boolean;
};

type Summary = {
  total: number;
  active: number;
  connections: number;
  equipment: number;
  critical: number;
};

const KIND_LABEL: Record<string, string> = {
  pppoe: "PPPoE",
  gerencia: "Gerência",
  transporte: "Transporte",
};

const PAGE_SIZE = 10;

type SortKey = "connections" | "vlan" | "name" | "utilization";

type FormState = {
  vlan_id: string;
  name: string;
  description: string;
  kind: VlanKind;
  status: VlanStatus;
  capacity: string;
};

function emptyForm(): FormState {
  return { vlan_id: "", name: "", description: "", kind: "pppoe", status: "active", capacity: "" };
}

function fromRow(v: NetworkVlan): FormState {
  return {
    vlan_id: v.vlan_id,
    name: v.name || "",
    description: v.description || "",
    kind: (v.kind === "gerencia" || v.kind === "transporte" ? v.kind : "pppoe") as VlanKind,
    status: v.status === "inactive" ? "inactive" : "active",
    capacity: v.capacity != null ? String(v.capacity) : "",
  };
}

function utilClass(pct: number | null | undefined) {
  if (pct == null) return "";
  if (pct >= 80) return "bng-vlan-bar--crit";
  if (pct >= 60) return "bng-vlan-bar--warn";
  return "bng-vlan-bar--ok";
}

export function BngVlansTab({ deviceId }: { deviceId: string | null }) {
  const qc = useQueryClient();
  const { push } = useAppToast();
  const canMutate = isAdminUser() || can("bng.collect") || can("devices.manage");

  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [kind, setKind] = useState("");
  const [sortBy, setSortBy] = useState<SortKey>("connections");
  const [sortAsc, setSortAsc] = useState(false);
  const [page, setPage] = useState(1);
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState<FormState>(emptyForm());
  const [editing, setEditing] = useState<NetworkVlan | null>(null);
  const [viewing, setViewing] = useState<NetworkVlan | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<NetworkVlan | null>(null);

  const listQ = useQuery({
    queryKey: queryKeys.networkVlans(deviceId ?? ""),
    queryFn: () =>
      apiFetch<{ vlans: NetworkVlan[]; summary: Summary }>(
        `/api/v1/network-vlans${deviceId ? `?device_id=${encodeURIComponent(deviceId)}` : ""}`,
      ),
  });

  const items = listQ.data?.vlans ?? [];
  const summary = listQ.data?.summary;

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    let rows = items.filter((v) => {
      if (status && v.status !== status) return false;
      if (kind && v.kind !== kind) return false;
      if (!s) return true;
      return (
        v.vlan_id.toLowerCase().includes(s) ||
        (v.name || "").toLowerCase().includes(s) ||
        (v.description || "").toLowerCase().includes(s) ||
        (KIND_LABEL[v.kind] || v.kind).toLowerCase().includes(s)
      );
    });
    rows = [...rows].sort((a, b) => {
      const dir = sortAsc ? 1 : -1;
      switch (sortBy) {
        case "vlan":
          return a.vlan_id.localeCompare(b.vlan_id, "pt", { numeric: true }) * (sortAsc ? 1 : -1);
        case "name":
          return (a.name || "").localeCompare(b.name || "", "pt") * (sortAsc ? 1 : -1);
        case "utilization":
          return ((a.utilization ?? -1) - (b.utilization ?? -1)) * dir;
        default:
          return (a.connections - b.connections) * dir || a.vlan_id.localeCompare(b.vlan_id, "pt", { numeric: true });
      }
    });
    return rows;
  }, [items, q, status, kind, sortBy, sortAsc]);

  const pages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const pageSafe = Math.min(page, pages);
  const slice = filtered.slice((pageSafe - 1) * PAGE_SIZE, pageSafe * PAGE_SIZE);

  function invalidate() {
    return qc.invalidateQueries({ queryKey: ["network-vlans"] });
  }

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        vlan_id: form.vlan_id,
        name: form.name,
        description: form.description,
        kind: form.kind,
        status: form.status,
        capacity: form.capacity.trim() ? Number(form.capacity) : null,
        clear_capacity: form.capacity.trim() === "",
      };
      if (editing?.id) {
        await apiFetch(`/api/v1/network-vlans/${editing.id}`, { method: "PATCH", json: payload });
      } else if (editing && !editing.catalogued) {
        await apiFetch("/api/v1/network-vlans/upsert", { method: "POST", json: payload });
      } else {
        await apiFetch("/api/v1/network-vlans", { method: "POST", json: payload });
      }
    },
    onSuccess: async () => {
      toastOk(push, editing ? "VLAN actualizada" : "VLAN criada");
      setFormOpen(false);
      setEditing(null);
      await invalidate();
    },
    onError: (e) => toastErr(push, e),
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/network-vlans/${id}`, { method: "DELETE" }),
    onSuccess: async () => {
      toastOk(push, "VLAN removida do catálogo");
      setDeleteTarget(null);
      await invalidate();
    },
    onError: (e) => toastErr(push, e),
  });

  function openCreate() {
    setEditing(null);
    setForm(emptyForm());
    setFormOpen(true);
  }

  function openEdit(v: NetworkVlan) {
    setEditing(v);
    setForm(fromRow(v));
    setFormOpen(true);
  }

  return (
    <div className="bng-vlans">
      <div className="bng-vlans__head">
        <div>
          <h2 style={{ margin: 0, fontSize: 18 }}>
            VLANs
            <InfoHint>
              Catálogo da rede: categorize em PPPoE, gerência ou transporte. As VLANs vistas nas sessões do BNG
              aparecem automaticamente até as guardar.
            </InfoHint>
          </h2>
          <p className="bng-vlans__sub">Gerencie as VLANs da rede e acompanhe a quantidade de conexões</p>
        </div>
        {canMutate ? (
          <button type="button" className="btn btn--primary" onClick={openCreate}>
            <Plus size={16} style={{ marginRight: 6, verticalAlign: -3 }} />
            Nova VLAN
          </button>
        ) : null}
      </div>

      <div className="bng-vlans__kpis">
        <div className="card bng-vlans__kpi">
          <span>Total de VLANs</span>
          <strong>{summary?.total ?? "—"}</strong>
          <em>{summary?.active ?? 0} ativas na rede</em>
        </div>
        <div className="card bng-vlans__kpi">
          <span>Conexões totais</span>
          <strong>{(summary?.connections ?? 0).toLocaleString("pt-BR")}</strong>
          <em>clientes online</em>
        </div>
        <div className="card bng-vlans__kpi">
          <span>Equipamentos</span>
          <strong>{summary?.equipment ?? "—"}</strong>
          <em>utilizando VLANs</em>
        </div>
        <div className={`card bng-vlans__kpi${(summary?.critical ?? 0) > 0 ? " bng-vlans__kpi--warn" : ""}`}>
          <span>VLANs críticas</span>
          <strong>{summary?.critical ?? 0}</strong>
          <em>com alta utilização</em>
        </div>
      </div>

      <div className="bng-vlans__toolbar">
        <input
          className="input"
          placeholder="Pesquisar VLAN..."
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setPage(1);
          }}
        />
        <select
          className="input"
          value={status}
          onChange={(e) => {
            setStatus(e.target.value);
            setPage(1);
          }}
        >
          <option value="">Todos os Status</option>
          <option value="active">Ativa</option>
          <option value="inactive">Inativa</option>
        </select>
        <select
          className="input"
          value={kind}
          onChange={(e) => {
            setKind(e.target.value);
            setPage(1);
          }}
        >
          <option value="">Todos os Tipos</option>
          <option value="pppoe">PPPoE</option>
          <option value="gerencia">Gerência</option>
          <option value="transporte">Transporte</option>
        </select>
        <button type="button" className="btn btn--icon-menu" title="Actualizar" onClick={() => void listQ.refetch()}>
          <RefreshCw size={16} className={listQ.isFetching ? "spin" : ""} />
        </button>
        <label className="bng-vlans__sort">
          Ordenar por:
          <select className="input" value={sortBy} onChange={(e) => setSortBy(e.target.value as SortKey)}>
            <option value="connections">Clientes</option>
            <option value="vlan">VLAN</option>
            <option value="name">Nome</option>
            <option value="utilization">Utilização</option>
          </select>
        </label>
        <button
          type="button"
          className="btn btn--icon-menu"
          title={sortAsc ? "Ascendente" : "Descendente"}
          onClick={() => setSortAsc((v) => !v)}
        >
          {sortAsc ? <ArrowUpAZ size={16} /> : <ArrowDownAZ size={16} />}
        </button>
      </div>

      {listQ.isLoading ? (
        <p style={{ color: "var(--muted)" }}>A carregar VLANs…</p>
      ) : slice.length === 0 ? (
        <p style={{ color: "var(--muted)" }}>Nenhuma VLAN corresponde aos filtros. Crie uma ou execute a coleta de sessões no BNG.</p>
      ) : (
        <div className="table-wrap card" style={{ padding: 0 }}>
          <table className="bng-vlans__table">
            <thead>
              <tr>
                <th>VLAN</th>
                <th>Nome</th>
                <th>Descrição</th>
                <th>Tipo</th>
                <th>Conexões</th>
                <th>Equipamentos</th>
                <th>Status</th>
                <th>Utilização</th>
                <th style={{ width: 120 }}>Ações</th>
              </tr>
            </thead>
            <tbody>
              {slice.map((v) => (
                <tr key={v.id || v.vlan_id}>
                  <td>
                    <span className="bng-vlans__vid">{v.vlan_id}</span>
                  </td>
                  <td>
                    <strong>{v.name || "—"}</strong>
                    {!v.catalogued ? <div className="muted" style={{ fontSize: 11 }}>Por categorizar</div> : null}
                  </td>
                  <td className="muted">{v.description || "—"}</td>
                  <td>{KIND_LABEL[v.kind] || v.kind}</td>
                  <td>
                    <span className="bng-vlans__conn">
                      <Users size={13} />
                      {v.connections.toLocaleString("pt-BR")}
                    </span>
                  </td>
                  <td>{v.equipment}</td>
                  <td>
                    <span className={`bng-vlans__st bng-vlans__st--${v.status === "inactive" ? "off" : "on"}`}>
                      {v.status === "inactive" ? "Inativa" : "Ativa"}
                    </span>
                  </td>
                  <td>
                    {v.utilization != null ? (
                      <div className="bng-vlans__util">
                        <div className={`bng-vlans__bar ${utilClass(v.utilization)}`} style={{ width: `${v.utilization}%` }} />
                        <span>{v.utilization}%</span>
                      </div>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td>
                    <div className="row" style={{ gap: 4 }}>
                      <button type="button" className="btn btn--icon-menu" title="Ver" onClick={() => setViewing(v)}>
                        <Eye size={15} />
                      </button>
                      {canMutate ? (
                        <button type="button" className="btn btn--icon-menu" title="Editar" onClick={() => openEdit(v)}>
                          <Pencil size={15} />
                        </button>
                      ) : null}
                      {canMutate && v.id ? (
                        <button
                          type="button"
                          className="btn btn--icon-menu"
                          title="Excluir"
                          onClick={() => setDeleteTarget(v)}
                          style={{ color: "var(--err, #ef4444)" }}
                        >
                          <Trash2 size={15} />
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {filtered.length > 0 ? (
        <div className="bng-vlans__pager">
          <span>
            Mostrando {(pageSafe - 1) * PAGE_SIZE + 1} a {Math.min(pageSafe * PAGE_SIZE, filtered.length)} de {filtered.length}{" "}
            VLANs
          </span>
          <div className="row" style={{ gap: 4 }}>
            <button type="button" className="btn btn--icon-menu" disabled={pageSafe <= 1} onClick={() => setPage((p) => p - 1)}>
              <ChevronLeft size={16} />
            </button>
            {Array.from({ length: pages }, (_, i) => i + 1)
              .filter((n) => n === 1 || n === pages || Math.abs(n - pageSafe) <= 2)
              .map((n, idx, arr) => (
                <span key={n} className="row" style={{ gap: 4 }}>
                  {idx > 0 && arr[idx - 1] !== n - 1 ? <span className="muted">…</span> : null}
                  <button type="button" className={`btn btn--sm${n === pageSafe ? " btn--primary" : ""}`} onClick={() => setPage(n)}>
                    {n}
                  </button>
                </span>
              ))}
            <button type="button" className="btn btn--icon-menu" disabled={pageSafe >= pages} onClick={() => setPage((p) => p + 1)}>
              <ChevronRight size={16} />
            </button>
          </div>
        </div>
      ) : null}

      {formOpen
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onClick={() => setFormOpen(false)}>
              <div className="modal card" role="dialog" style={{ width: "min(480px, 94vw)", padding: 20 }} onClick={(e) => e.stopPropagation()}>
                <h2 style={{ margin: "0 0 12px", fontSize: 17 }}>{editing ? "Editar VLAN" : "Nova VLAN"}</h2>
                <div className="fleet-form-grid">
                  <label>
                    VLAN
                    <input className="input" value={form.vlan_id} onChange={(e) => setForm({ ...form, vlan_id: e.target.value })} placeholder="ex.: 100" disabled={!!editing?.catalogued} />
                  </label>
                  <label>
                    Nome
                    <input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="INTERNET" />
                  </label>
                  <label>
                    Tipo
                    <select className="input" value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value as VlanKind })}>
                      <option value="pppoe">PPPoE</option>
                      <option value="gerencia">Gerência</option>
                      <option value="transporte">Transporte</option>
                    </select>
                  </label>
                  <label>
                    Status
                    <select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value as VlanStatus })}>
                      <option value="active">Ativa</option>
                      <option value="inactive">Inativa</option>
                    </select>
                  </label>
                  <label className="fleet-form-span">
                    Descrição
                    <input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} placeholder="Uso desta VLAN" />
                  </label>
                  <label>
                    Capacidade (conexões)
                    <input className="input" type="number" min={1} value={form.capacity} onChange={(e) => setForm({ ...form, capacity: e.target.value })} placeholder="opcional, p/ utilização" />
                  </label>
                </div>
                <div className="row" style={{ marginTop: 14, justifyContent: "flex-end", gap: 8 }}>
                  <button type="button" className="btn" onClick={() => setFormOpen(false)}>
                    Cancelar
                  </button>
                  <button type="button" className="btn btn--primary" disabled={save.isPending || !form.vlan_id.trim()} onClick={() => save.mutate()}>
                    Guardar
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}

      {viewing
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onClick={() => setViewing(null)}>
              <div className="modal card" role="dialog" style={{ width: "min(420px, 94vw)", padding: 20 }} onClick={(e) => e.stopPropagation()}>
                <h2 style={{ margin: "0 0 8px", fontSize: 17 }}>
                  VLAN {viewing.vlan_id} {viewing.name ? `· ${viewing.name}` : ""}
                </h2>
                <p className="muted" style={{ marginTop: 0 }}>
                  {viewing.description || "Sem descrição"}
                </p>
                <dl className="bng-vlans__dl">
                  <div>
                    <dt>Tipo</dt>
                    <dd>{KIND_LABEL[viewing.kind] || viewing.kind}</dd>
                  </div>
                  <div>
                    <dt>Status</dt>
                    <dd>{viewing.status === "inactive" ? "Inativa" : "Ativa"}</dd>
                  </div>
                  <div>
                    <dt>Conexões</dt>
                    <dd>{viewing.connections.toLocaleString("pt-BR")}</dd>
                  </div>
                  <div>
                    <dt>Equipamentos</dt>
                    <dd>{viewing.equipment}</dd>
                  </div>
                  <div>
                    <dt>Capacidade</dt>
                    <dd>{viewing.capacity ?? "—"}</dd>
                  </div>
                  <div>
                    <dt>Utilização</dt>
                    <dd>{viewing.utilization != null ? `${viewing.utilization}%` : "—"}</dd>
                  </div>
                </dl>
                <div className="row" style={{ justifyContent: "flex-end", marginTop: 12 }}>
                  <button type="button" className="btn" onClick={() => setViewing(null)}>
                    Fechar
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Excluir VLAN"
        message={deleteTarget ? `Remover VLAN ${deleteTarget.vlan_id} do catálogo? As sessões do BNG não são apagadas.` : ""}
        confirmLabel="Excluir"
        danger
        busy={del.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget?.id && del.mutate(deleteTarget.id)}
      />
    </div>
  );
}
