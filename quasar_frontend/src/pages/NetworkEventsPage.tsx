import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, Download, PenLine } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch, downloadBlob } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { can, getAuthToken, isAdminUser } from "../lib/auth";
import { ifaceDisplayName } from "../lib/monitor/extract";
import { toastErr, toastOk } from "../lib/operationToast";
import { queryKeys } from "../lib/queryKeys";

const IFACE_OTHER = "__other__";

type DeviceIface = {
  if_index?: number;
  display_name?: string;
  if_name?: string;
  descr?: string;
  name?: string;
};

type Category = { code: string; label: string; subgroups?: string[]; fields: string[] };
type EventType = { code: string; category_code: string; subgroup?: string; label: string };
type ImpactOpt = { code: string; label: string };
type Lite = { id: string; description: string; pop_id?: string | null; project_id?: string | null };
type Tech = { id: string; label: string };

type Catalog = {
  categories: Category[];
  types: EventType[];
  impacts: ImpactOpt[];
};

type Lookups = {
  pops: Lite[];
  devices: Lite[];
  projects: Lite[];
  ctos: Lite[];
  cables: Lite[];
  splice_boxes: Lite[];
  poles: Lite[];
  technicians: Tech[];
};

type NetworkEvent = {
  id: string;
  occurred_at: string;
  category_code: string;
  category_label: string;
  type_code: string;
  type_label: string;
  impact: string;
  notes?: string | null;
  pop_id?: string | null;
  pop_name?: string | null;
  device_id?: string | null;
  device_name?: string | null;
  technician_id?: string | null;
  technician_name?: string | null;
  project_id?: string | null;
  project_name?: string | null;
  cto_id?: string | null;
  cto_name?: string | null;
  cable_id?: string | null;
  cable_name?: string | null;
  splice_box_id?: string | null;
  splice_box_name?: string | null;
  pole_id?: string | null;
  pole_name?: string | null;
  interface_name?: string | null;
  vlan?: string | null;
  created_by_name?: string | null;
};

type Summary = {
  total: number;
  this_month: number;
  incidents: number;
  by_category: { code: string; label: string; count: number }[];
};

type Form = {
  occurred_at: string;
  category_code: string;
  type_code: string;
  impact: string;
  notes: string;
  pop_id: string;
  device_id: string;
  technician_id: string;
  project_id: string;
  cto_id: string;
  cable_id: string;
  splice_box_id: string;
  pole_id: string;
  interface_name: string;
  vlan: string;
};

function pad(n: number) {
  return String(n).padStart(2, "0");
}

function toLocalInput(iso?: string) {
  const d = iso ? new Date(iso) : new Date();
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function formatWhen(iso: string) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
}

function emptyForm(): Form {
  return {
    occurred_at: toLocalInput(),
    category_code: "",
    type_code: "",
    impact: "none",
    notes: "",
    pop_id: "",
    device_id: "",
    technician_id: "",
    project_id: "",
    cto_id: "",
    cable_id: "",
    splice_box_id: "",
    pole_id: "",
    interface_name: "",
    vlan: "",
  };
}

function impactClass(code: string) {
  if (code === "critical") return "netev-impact netev-impact--crit";
  if (code === "high") return "netev-impact netev-impact--high";
  if (code === "medium") return "netev-impact netev-impact--med";
  if (code === "low") return "netev-impact netev-impact--low";
  return "netev-impact";
}

function opt(v: string) {
  return v.trim() || null;
}

export function NetworkEventsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("network_events.manage") || isAdminUser();

  const [q, setQ] = useState("");
  const [category, setCategory] = useState("");
  const [typeCode, setTypeCode] = useState("");
  const [impact, setImpact] = useState("");
  const [popId, setPopId] = useState("");
  const [techId, setTechId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const [formOpen, setFormOpen] = useState(false);
  const [formStep, setFormStep] = useState<1 | 2>(1);
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState<Form>(emptyForm());
  const [typeSearch, setTypeSearch] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<NetworkEvent | null>(null);
  const [ifaceOther, setIfaceOther] = useState(false);

  const filterKey = [q, category, typeCode, impact, popId, techId, from, to].join("|");

  const catalogQ = useQuery({
    queryKey: queryKeys.networkEventsCatalog,
    queryFn: () => apiFetch<Catalog>("/api/v1/network-events/catalog"),
  });
  const lookupsQ = useQuery({
    queryKey: queryKeys.networkEventsLookups,
    queryFn: () => apiFetch<Lookups>("/api/v1/network-events/lookups"),
  });
  const summaryQ = useQuery({
    queryKey: queryKeys.networkEventsSummary,
    queryFn: () => apiFetch<Summary>("/api/v1/network-events/summary"),
  });

  const listQs = useMemo(() => {
    const p = new URLSearchParams();
    if (q.trim()) p.set("q", q.trim());
    if (category) p.set("category", category);
    if (typeCode) p.set("type", typeCode);
    if (impact) p.set("impact", impact);
    if (popId) p.set("pop_id", popId);
    if (techId) p.set("technician_id", techId);
    if (from) p.set("from", from);
    if (to) p.set("to", to);
    p.set("limit", "300");
    return p.toString();
  }, [q, category, typeCode, impact, popId, techId, from, to]);

  const listQ = useQuery({
    queryKey: queryKeys.networkEvents(filterKey),
    queryFn: () => apiFetch<{ items: NetworkEvent[]; total: number }>(`/api/v1/network-events?${listQs}`),
  });

  const catalog = catalogQ.data;
  const lookups = lookupsQ.data;
  const items = listQ.data?.items ?? [];
  const total = listQ.data?.total ?? 0;

  const typesForFilter = useMemo(() => {
    const all = catalog?.types ?? [];
    if (!category) return all;
    return all.filter((t) => t.category_code === category);
  }, [catalog, category]);

  const formCategory = catalog?.categories.find((c) => c.code === form.category_code);
  const formTypes = useMemo(() => {
    const all = (catalog?.types ?? []).filter((t) => t.category_code === form.category_code);
    const s = typeSearch.trim().toLowerCase();
    if (!s) return all;
    return all.filter((t) => t.label.toLowerCase().includes(s) || t.code.toLowerCase().includes(s));
  }, [catalog, form.category_code, typeSearch]);

  const formTypeGroups = useMemo(() => {
    const map = new Map<string, EventType[]>();
    for (const t of formTypes) {
      const g = t.subgroup || "";
      const cur = map.get(g) ?? [];
      cur.push(t);
      map.set(g, cur);
    }
    return [...map.entries()];
  }, [formTypes]);

  function invalidateAll() {
    return Promise.all([
      qc.invalidateQueries({ queryKey: ["network-events"] }),
      qc.invalidateQueries({ queryKey: queryKeys.networkEventsSummary }),
    ]);
  }

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        occurred_at: form.occurred_at,
        type_code: form.type_code,
        impact: form.impact,
        notes: opt(form.notes),
        pop_id: opt(form.pop_id),
        device_id: opt(form.device_id),
        technician_id: opt(form.technician_id),
        project_id: opt(form.project_id),
        cto_id: opt(form.cto_id),
        cable_id: opt(form.cable_id),
        splice_box_id: opt(form.splice_box_id),
        pole_id: opt(form.pole_id),
        interface_name: opt(form.interface_name),
        vlan: opt(form.vlan),
      };
      if (editing) {
        await apiFetch(`/api/v1/network-events/${editing}`, { method: "PATCH", json: payload });
      } else {
        await apiFetch("/api/v1/network-events", { method: "POST", json: payload });
      }
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Evento actualizado" : "Evento registado");
      closeForm();
      await invalidateAll();
    },
    onError: (e) => toastErr(push, e),
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/network-events/${id}`, { method: "DELETE" }),
    onSuccess: async () => {
      toastOk(push, "Evento excluído");
      setDeleteTarget(null);
      await invalidateAll();
    },
    onError: (e) => toastErr(push, e),
  });

  function openCreate() {
    setEditing(null);
    setForm(emptyForm());
    setTypeSearch("");
    setIfaceOther(false);
    setFormStep(1);
    setFormOpen(true);
  }

  function closeForm() {
    setFormOpen(false);
    setEditing(null);
    setForm(emptyForm());
    setFormStep(1);
    setTypeSearch("");
    setIfaceOther(false);
  }

  function startEdit(ev: NetworkEvent) {
    setEditing(ev.id);
    setForm({
      occurred_at: toLocalInput(ev.occurred_at),
      category_code: ev.category_code,
      type_code: ev.type_code,
      impact: ev.impact || "none",
      notes: ev.notes ?? "",
      pop_id: ev.pop_id ?? "",
      device_id: ev.device_id ?? "",
      technician_id: ev.technician_id ?? "",
      project_id: ev.project_id ?? "",
      cto_id: ev.cto_id ?? "",
      cable_id: ev.cable_id ?? "",
      splice_box_id: ev.splice_box_id ?? "",
      pole_id: ev.pole_id ?? "",
      interface_name: ev.interface_name ?? "",
      vlan: ev.vlan ?? "",
    });
    setTypeSearch("");
    setIfaceOther(false);
    setFormStep(2);
    setFormOpen(true);
  }

  async function exportCsv() {
    try {
      const token = getAuthToken();
      const res = await fetch(`/api/v1/network-events/export.csv?${listQs}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) throw new Error(`Falha ao exportar (${res.status})`);
      downloadBlob("eventos-rede.csv", await res.blob());
    } catch (e) {
      toastErr(push, e);
    }
  }

  const fields = formCategory?.fields ?? [];
  const showIfaceField = fields.includes("interface");
  const deviceIfacesQ = useQuery({
    queryKey: ["network-event-device-ifaces", form.device_id],
    enabled: formOpen && showIfaceField && !!form.device_id,
    queryFn: () =>
      apiFetch<{ interface_table?: DeviceIface[] }>(`/api/v1/interfaces/devices/${form.device_id}`),
  });
  const ifaceOptions = useMemo(() => {
    const names = new Set<string>();
    const opts: { value: string; label: string }[] = [];
    for (const row of deviceIfacesQ.data?.interface_table ?? []) {
      const name = ifaceDisplayName(row);
      if (!name || name === "—" || names.has(name)) continue;
      names.add(name);
      opts.push({ value: name, label: name });
    }
    return opts;
  }, [deviceIfacesQ.data?.interface_table]);

  useEffect(() => {
    if (!form.device_id || deviceIfacesQ.isFetching) return;
    if (form.interface_name && !ifaceOptions.some((o) => o.value === form.interface_name)) {
      setIfaceOther(true);
    }
  }, [form.device_id, form.interface_name, deviceIfacesQ.isFetching, ifaceOptions]);

  const ifaceSelectValue = ifaceOther ? IFACE_OTHER : form.interface_name;
  const showIfaceFreeText = !form.device_id || ifaceOther || (!deviceIfacesQ.isFetching && ifaceOptions.length === 0);

  const devicesForForm = (lookups?.devices ?? []).filter((d) => !form.pop_id || d.pop_id === form.pop_id);
  const ctosForForm = (lookups?.ctos ?? []).filter((d) => !form.project_id || d.project_id === form.project_id);
  const cablesForForm = (lookups?.cables ?? []).filter((d) => !form.project_id || d.project_id === form.project_id);
  const splicesForForm = (lookups?.splice_boxes ?? []).filter((d) => !form.project_id || d.project_id === form.project_id);
  const polesForForm = (lookups?.poles ?? []).filter((d) => !form.project_id || d.project_id === form.project_id);

  return (
    <div className="netev-page">
      <div className="page-heading">
        <h1>
          Eventos da Rede
          <InfoHint>
            Histórico estruturado de alterações e incidentes. Escolha a categoria e o tipo — o código do tipo
            fica gravado para relatórios (trocas de GBIC, rompimentos, alterações numa OLT, etc.).
          </InfoHint>
        </h1>
        <PageCountPill label="eventos" count={total} />
      </div>

      <div className="netev-kpis">
        <div className="card netev-kpi">
          <span>Total</span>
          <strong>{summaryQ.data?.total ?? "—"}</strong>
        </div>
        <div className="card netev-kpi">
          <span>Este mês</span>
          <strong>{summaryQ.data?.this_month ?? "—"}</strong>
        </div>
        <div className="card netev-kpi">
          <span>Rompimentos</span>
          <strong>{summaryQ.data?.incidents ?? "—"}</strong>
        </div>
      </div>

      <div className="netev-toolbar">
        <input className="input" placeholder="Pesquisar notas, POP, equipamento…" value={q} onChange={(e) => setQ(e.target.value)} />
        <select className="input" value={category} onChange={(e) => { setCategory(e.target.value); setTypeCode(""); }} aria-label="Categoria">
          <option value="">Todas as categorias</option>
          {(catalog?.categories ?? []).map((c) => (
            <option key={c.code} value={c.code}>{c.label}</option>
          ))}
        </select>
        <select className="input" value={typeCode} onChange={(e) => setTypeCode(e.target.value)} aria-label="Tipo" disabled={!category}>
          <option value="">{category ? "Todos os tipos" : "Escolha a categoria"}</option>
          {typesForFilter.map((t) => (
            <option key={t.code} value={t.code}>{t.label}</option>
          ))}
        </select>
        <select className="input" value={impact} onChange={(e) => setImpact(e.target.value)} aria-label="Impacto">
          <option value="">Qualquer impacto</option>
          {(catalog?.impacts ?? []).map((i) => (
            <option key={i.code} value={i.code}>{i.label}</option>
          ))}
        </select>
        <select className="input" value={popId} onChange={(e) => setPopId(e.target.value)} aria-label="POP">
          <option value="">Todos os POPs</option>
          {(lookups?.pops ?? []).map((p) => (
            <option key={p.id} value={p.id}>{p.description}</option>
          ))}
        </select>
        <select className="input" value={techId} onChange={(e) => setTechId(e.target.value)} aria-label="Técnico">
          <option value="">Qualquer técnico</option>
          {(lookups?.technicians ?? []).map((u) => (
            <option key={u.id} value={u.id}>{u.label}</option>
          ))}
        </select>
        <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="De" />
        <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label="Até" />
        <button type="button" className="btn btn--icon btn--icon-menu" title="Exportar CSV" aria-label="Exportar CSV" onClick={() => void exportCsv()}>
          <Download size={18} aria-hidden />
        </button>
        {canMutate ? (
          <button type="button" className="btn btn--icon btn--icon-menu btn--primary" title="Novo evento" aria-label="Novo evento" onClick={openCreate}>
            <CirclePlus size={18} aria-hidden />
          </button>
        ) : null}
      </div>

      <div className="card table-wrap">
        <table>
          <thead>
            <tr>
              <th>Data</th>
              <th>Tipo</th>
              <th>Categoria</th>
              <th>POP / Equipamento</th>
              <th>Impacto</th>
              <th>Técnico</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((ev) => (
              <tr key={ev.id}>
                <td className="netev-when">{formatWhen(ev.occurred_at)}</td>
                <td>
                  <strong>{ev.type_label}</strong>
                  {ev.notes ? <div className="muted netev-notes">{ev.notes}</div> : null}
                  {ev.interface_name || ev.vlan ? (
                    <div className="muted">{[ev.interface_name, ev.vlan ? `VLAN ${ev.vlan}` : ""].filter(Boolean).join(" · ")}</div>
                  ) : null}
                </td>
                <td>{ev.category_label}</td>
                <td>
                  {ev.pop_name || ev.device_name || ev.project_name || ev.cto_name || ev.cable_name || "—"}
                  {ev.device_name && ev.pop_name ? <div className="muted">{ev.device_name}</div> : null}
                  {ev.cto_name || ev.cable_name || ev.splice_box_name || ev.pole_name ? (
                    <div className="muted">{[ev.cto_name, ev.cable_name, ev.splice_box_name, ev.pole_name].filter(Boolean).join(" · ")}</div>
                  ) : null}
                </td>
                <td>
                  <span className={impactClass(ev.impact)}>
                    {(catalog?.impacts ?? []).find((i) => i.code === ev.impact)?.label ?? ev.impact}
                  </span>
                </td>
                <td>{ev.technician_name || "—"}</td>
                <td>
                  {canMutate ? (
                    <ActionMenu
                      title="Opções do evento"
                      icon={<PenLine size={16} aria-hidden />}
                      items={[
                        { id: "edit", label: "Editar", onClick: () => startEdit(ev) },
                        { id: "del", label: "Excluir", danger: true, onClick: () => setDeleteTarget(ev) },
                      ]}
                    />
                  ) : null}
                </td>
              </tr>
            ))}
            {listQ.isLoading ? (
              <tr><td colSpan={7} className="muted">A carregar…</td></tr>
            ) : listQ.isError ? (
              <tr><td colSpan={7} className="err">Falha ao carregar eventos.</td></tr>
            ) : items.length === 0 ? (
              <tr><td colSpan={7} className="muted">Nenhum evento para os filtros.</td></tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {formOpen && canMutate
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={closeForm}>
              <div className="modal modal--wide netev-modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <h3>{editing ? "Editar evento" : "Novo evento da rede"}</h3>
                {formStep === 1 ? (
                  <>
                    <p className="muted">Escolha a categoria. Os tipos aparecem a seguir, já filtrados.</p>
                    <div className="netev-cats">
                      {(catalog?.categories ?? []).map((c) => (
                        <button
                          key={c.code}
                          type="button"
                          className={`netev-cat${form.category_code === c.code ? " is-on" : ""}`}
                          onClick={() => {
                            setForm({ ...form, category_code: c.code, type_code: "" });
                            setTypeSearch("");
                            setFormStep(2);
                          }}
                        >
                          {c.label}
                        </button>
                      ))}
                    </div>
                  </>
                ) : (
                  <>
                    <div className="netev-step-head">
                      <button
                        type="button"
                        className="btn"
                        onClick={() => {
                          setForm({ ...form, type_code: "" });
                          setFormStep(1);
                        }}
                      >
                        ← {formCategory?.label ?? "Categoria"}
                      </button>
                    </div>
                    <div className="fleet-form-grid netev-form">
                      <label className="fleet-form-span">
                        Tipo de evento*
                        <input
                          className="input"
                          placeholder="Filtrar tipos desta categoria…"
                          value={typeSearch}
                          onChange={(e) => setTypeSearch(e.target.value)}
                        />
                        <select
                          className="input"
                          value={form.type_code}
                          onChange={(e) => setForm({ ...form, type_code: e.target.value })}
                          size={Math.min(10, Math.max(4, formTypes.length))}
                        >
                          <option value="">Seleccione o tipo…</option>
                          {formTypeGroups.map(([g, list]) =>
                            g ? (
                              <optgroup key={g} label={g}>
                                {list.map((t) => (
                                  <option key={t.code} value={t.code}>{t.label}</option>
                                ))}
                              </optgroup>
                            ) : (
                              list.map((t) => (
                                <option key={t.code} value={t.code}>{t.label}</option>
                              ))
                            ),
                          )}
                        </select>
                      </label>
                      <label>
                        Data e hora*
                        <input className="input" type="datetime-local" value={form.occurred_at} onChange={(e) => setForm({ ...form, occurred_at: e.target.value })} />
                      </label>
                      <label>
                        Impacto
                        <select className="input" value={form.impact} onChange={(e) => setForm({ ...form, impact: e.target.value })}>
                          {(catalog?.impacts ?? []).map((i) => (
                            <option key={i.code} value={i.code}>{i.label}</option>
                          ))}
                        </select>
                      </label>
                      <label>
                        Técnico
                        <select className="input" value={form.technician_id} onChange={(e) => setForm({ ...form, technician_id: e.target.value })}>
                          <option value="">—</option>
                          {(lookups?.technicians ?? []).map((u) => (
                            <option key={u.id} value={u.id}>{u.label}</option>
                          ))}
                        </select>
                      </label>
                      {fields.includes("pop") ? (
                        <label>
                          POP
                          <select className="input" value={form.pop_id} onChange={(e) => { setIfaceOther(false); setForm({ ...form, pop_id: e.target.value, device_id: "", interface_name: "" }); }}>
                            <option value="">—</option>
                            {(lookups?.pops ?? []).map((p) => (
                              <option key={p.id} value={p.id}>{p.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("device") ? (
                        <label>
                          Equipamento
                          <select className="input" value={form.device_id} onChange={(e) => { setIfaceOther(false); setForm({ ...form, device_id: e.target.value, interface_name: "" }); }}>
                            <option value="">—</option>
                            {devicesForForm.map((d) => (
                              <option key={d.id} value={d.id}>{d.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("project") ? (
                        <label>
                          Projeto FTTH
                          <select className="input" value={form.project_id} onChange={(e) => setForm({ ...form, project_id: e.target.value, cto_id: "", cable_id: "", splice_box_id: "", pole_id: "" })}>
                            <option value="">—</option>
                            {(lookups?.projects ?? []).map((p) => (
                              <option key={p.id} value={p.id}>{p.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("cto") ? (
                        <label>
                          CTO
                          <select className="input" value={form.cto_id} onChange={(e) => setForm({ ...form, cto_id: e.target.value })}>
                            <option value="">—</option>
                            {ctosForForm.map((p) => (
                              <option key={p.id} value={p.id}>{p.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("cable") ? (
                        <label>
                          Cabo
                          <select className="input" value={form.cable_id} onChange={(e) => setForm({ ...form, cable_id: e.target.value })}>
                            <option value="">—</option>
                            {cablesForForm.map((p) => (
                              <option key={p.id} value={p.id}>{p.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("splice") ? (
                        <label>
                          Caixa de emenda
                          <select className="input" value={form.splice_box_id} onChange={(e) => setForm({ ...form, splice_box_id: e.target.value })}>
                            <option value="">—</option>
                            {splicesForForm.map((p) => (
                              <option key={p.id} value={p.id}>{p.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("pole") ? (
                        <label>
                          Poste
                          <select className="input" value={form.pole_id} onChange={(e) => setForm({ ...form, pole_id: e.target.value })}>
                            <option value="">—</option>
                            {polesForForm.map((p) => (
                              <option key={p.id} value={p.id}>{p.description}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                      {fields.includes("interface") ? (
                        <label>
                          Interface / PON
                          <div className="netev-iface-stack">
                            {form.device_id ? (
                              <select
                                className="input"
                                value={ifaceSelectValue}
                                onChange={(e) => {
                                  const v = e.target.value;
                                  if (v === IFACE_OTHER) {
                                    setIfaceOther(true);
                                    setForm({ ...form, interface_name: ifaceOptions.some((o) => o.value === form.interface_name) ? "" : form.interface_name });
                                    return;
                                  }
                                  setIfaceOther(false);
                                  setForm({ ...form, interface_name: v });
                                }}
                              >
                                <option value="">—</option>
                                {ifaceOptions.map((o) => (
                                  <option key={o.value} value={o.value}>{o.label}</option>
                                ))}
                                <option value={IFACE_OTHER}>Outros…</option>
                              </select>
                            ) : null}
                            {form.device_id && deviceIfacesQ.isFetching ? (
                              <span className="muted">A carregar interfaces do equipamento…</span>
                            ) : null}
                            {form.device_id && !deviceIfacesQ.isFetching && ifaceOptions.length === 0 ? (
                              <span className="muted">Sem interfaces recolhidas — escreva o nome ou execute uma recolha SNMP no equipamento.</span>
                            ) : null}
                            {!form.device_id ? (
                              <span className="muted">Seleccione o equipamento para listar as interfaces, ou escreva em texto livre.</span>
                            ) : null}
                            {showIfaceFreeText ? (
                              <input
                                className="input"
                                value={form.interface_name}
                                onChange={(e) => setForm({ ...form, interface_name: e.target.value })}
                                placeholder="ex.: GPON0/1, ether10"
                              />
                            ) : null}
                          </div>
                        </label>
                      ) : null}
                      {fields.includes("vlan") ? (
                        <label>
                          VLAN
                          <input className="input" value={form.vlan} onChange={(e) => setForm({ ...form, vlan: e.target.value })} placeholder="ex.: 300" />
                        </label>
                      ) : null}
                      <label className="fleet-form-span">
                        Notas
                        <textarea className="input" rows={3} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} placeholder="Detalhe complementar (opcional)" />
                      </label>
                    </div>
                    <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
                      <button type="button" className="btn" onClick={closeForm}>Cancelar</button>
                      <button
                        type="button"
                        className="btn btn--primary"
                        disabled={save.isPending || !form.type_code || !form.occurred_at}
                        onClick={() => save.mutate()}
                      >
                        {editing ? "Guardar" : "Registar"}
                      </button>
                    </div>
                  </>
                )}
              </div>
            </div>,
            document.body,
          )
        : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Excluir evento"
        message={deleteTarget ? `Excluir «${deleteTarget.type_label}» de ${formatWhen(deleteTarget.occurred_at)}?` : ""}
        confirmLabel="Excluir"
        danger
        busy={del.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && del.mutate(deleteTarget.id)}
      />
    </div>
  );
}
