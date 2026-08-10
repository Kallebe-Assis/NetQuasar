import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { DRIVER_STATUS, UF_OPTIONS } from "./fleetUtils";

type Driver = {
  id: string;
  name: string;
  cpf?: string | null;
  phone?: string | null;
  email?: string | null;
  license_number?: string | null;
  license_category?: string | null;
  license_expires_on?: string | null;
  city?: string | null;
  uf?: string | null;
  user_id?: string | null;
  user_login?: string | null;
  status: string;
  notes?: string | null;
};

type Form = {
  name: string; cpf: string; phone: string; email: string; license_number: string; license_category: string;
  license_expires_on: string; city: string; uf: string; user_id: string; status: string; notes: string;
};

const empty = (): Form => ({
  name: "", cpf: "", phone: "", email: "", license_number: "", license_category: "",
  license_expires_on: "", city: "", uf: "", user_id: "", status: "active", notes: "",
});

export function FleetDriversPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState<Form>(empty());
  const [linkDriver, setLinkDriver] = useState("");
  const [linkVehicle, setLinkVehicle] = useState("");
  const [linkPrimary, setLinkPrimary] = useState(false);

  const drivers = useQuery({ queryKey: queryKeys.fleetDrivers, queryFn: () => apiFetch<{ items: Driver[] }>("/api/v1/fleet/drivers") });
  const users = useQuery({ queryKey: queryKeys.fleetUsers, queryFn: () => apiFetch<{ items: { id: string; login: string }[] }>("/api/v1/fleet/users") });
  const vehicles = useQuery({ queryKey: queryKeys.fleetVehicles, queryFn: () => apiFetch<{ items: { id: string; plate: string; description: string }[] }>("/api/v1/fleet/vehicles") });
  const links = useQuery({ queryKey: queryKeys.fleetDriverVehicles, queryFn: () => apiFetch<{ items: { id: string; driver_name: string; plate: string; is_primary: boolean; starts_on?: string | null }[] }>("/api/v1/fleet/driver-vehicles") });

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        name: form.name.trim(),
        cpf: form.cpf.trim() || null,
        phone: form.phone.trim() || null,
        email: form.email.trim() || null,
        license_number: form.license_number.trim() || null,
        license_category: form.license_category.trim() || null,
        license_expires_on: form.license_expires_on || null,
        city: form.city.trim() || null,
        uf: form.uf || null,
        user_id: form.user_id || null,
        status: form.status,
        notes: form.notes.trim() || null,
      };
      if (editing) await apiFetch(`/api/v1/fleet/drivers/${editing}`, { method: "PATCH", body: JSON.stringify(payload) });
      else await apiFetch("/api/v1/fleet/drivers", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Motorista actualizado" : "Motorista criado");
      setEditing(null); setForm(empty());
      await qc.invalidateQueries({ queryKey: queryKeys.fleetDrivers });
    },
    onError: (e) => toastErr(push, e),
  });

  const linkMut = useMutation({
    mutationFn: () => apiFetch("/api/v1/fleet/driver-vehicles", {
      method: "POST",
      body: JSON.stringify({ driver_id: linkDriver, vehicle_id: linkVehicle, is_primary: linkPrimary }),
    }),
    onSuccess: async () => {
      toastOk(push, "Vínculo criado");
      setLinkDriver(""); setLinkVehicle(""); setLinkPrimary(false);
      await qc.invalidateQueries({ queryKey: queryKeys.fleetDriverVehicles });
    },
    onError: (e) => toastErr(push, e),
  });

  const unlink = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/fleet/driver-vehicles/${id}`, { method: "DELETE" }),
    onSuccess: async () => {
      toastOk(push, "Vínculo removido");
      await qc.invalidateQueries({ queryKey: queryKeys.fleetDriverVehicles });
    },
    onError: (e) => toastErr(push, e),
  });

  const filtered = (drivers.data?.items ?? []).filter((d) => {
    const s = q.trim().toLowerCase();
    if (!s) return true;
    return d.name.toLowerCase().includes(s) || (d.cpf ?? "").includes(s);
  });

  return (
    <div className="fleet-page">
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 12 }}>
        <h1 style={{ margin: 0 }}>Frota — Motoristas</h1>
        <input className="input" placeholder="Pesquisar…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>

      {canMutate ? (
        <div className="card" style={{ marginBottom: 12 }}>
          <h3>{editing ? "Editar motorista" : "Novo motorista"}</h3>
          <div className="fleet-form-grid">
            <label>Nome*<input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
            <label>CPF<input className="input" value={form.cpf} onChange={(e) => setForm({ ...form, cpf: e.target.value })} /></label>
            <label>Telefone<input className="input" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} /></label>
            <label>E-mail<input className="input" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} /></label>
            <label>CNH<input className="input" value={form.license_number} onChange={(e) => setForm({ ...form, license_number: e.target.value })} /></label>
            <label>Categoria CNH<input className="input" value={form.license_category} onChange={(e) => setForm({ ...form, license_category: e.target.value })} /></label>
            <label>Validade CNH<input className="input" type="date" value={form.license_expires_on} onChange={(e) => setForm({ ...form, license_expires_on: e.target.value })} /></label>
            <label>Cidade<input className="input" value={form.city} onChange={(e) => setForm({ ...form, city: e.target.value })} /></label>
            <label>UF<select className="input" value={form.uf} onChange={(e) => setForm({ ...form, uf: e.target.value })}><option value="">—</option>{UF_OPTIONS.map((u) => <option key={u} value={u}>{u}</option>)}</select></label>
            <label>Utilizador<select className="input" value={form.user_id} onChange={(e) => setForm({ ...form, user_id: e.target.value })}><option value="">Nenhum</option>{(users.data?.items ?? []).map((u) => <option key={u.id} value={u.id}>{u.login}</option>)}</select></label>
            <label>Status<select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>{DRIVER_STATUS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}</select></label>
            <label className="fleet-form-span">Observação<input className="input" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button type="button" className="btn btn--primary" disabled={!form.name.trim() || save.isPending} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
            {editing ? <button type="button" className="btn" onClick={() => { setEditing(null); setForm(empty()); }}>Cancelar</button> : null}
          </div>
        </div>
      ) : null}

      <div className="card table-wrap" style={{ marginBottom: 12 }}>
        <table>
          <thead><tr><th>Nome</th><th>CPF</th><th>CNH</th><th>Utilizador</th><th>Status</th><th /></tr></thead>
          <tbody>
            {filtered.map((d) => (
              <tr key={d.id}>
                <td>{d.name}</td><td>{d.cpf ?? "—"}</td>
                <td>{d.license_number ?? "—"}{d.license_category ? ` (${d.license_category})` : ""}</td>
                <td>{d.user_login ?? "—"}</td>
                <td>{DRIVER_STATUS.find((s) => s.value === d.status)?.label ?? d.status}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => {
                  setEditing(d.id);
                  setForm({
                    name: d.name, cpf: d.cpf ?? "", phone: d.phone ?? "", email: d.email ?? "",
                    license_number: d.license_number ?? "", license_category: d.license_category ?? "",
                    license_expires_on: d.license_expires_on ?? "", city: d.city ?? "", uf: d.uf ?? "",
                    user_id: d.user_id ?? "", status: d.status, notes: d.notes ?? "",
                  });
                }}>Editar</button> : null}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {canMutate ? (
        <div className="card">
          <h3>Vínculos motorista × veículo</h3>
          <div className="row" style={{ marginBottom: 10 }}>
            <select className="input" value={linkDriver} onChange={(e) => setLinkDriver(e.target.value)}>
              <option value="">Motorista…</option>
              {(drivers.data?.items ?? []).map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
            <select className="input" value={linkVehicle} onChange={(e) => setLinkVehicle(e.target.value)}>
              <option value="">Veículo…</option>
              {(vehicles.data?.items ?? []).map((v) => <option key={v.id} value={v.id}>{v.plate} — {v.description}</option>)}
            </select>
            <label className="row"><input type="checkbox" checked={linkPrimary} onChange={(e) => setLinkPrimary(e.target.checked)} /> Principal</label>
            <button type="button" className="btn btn--primary" disabled={!linkDriver || !linkVehicle || linkMut.isPending} onClick={() => linkMut.mutate()}>Vincular</button>
          </div>
          <ul className="fleet-rank-list">
            {(links.data?.items ?? []).map((l) => (
              <li key={l.id}>
                <div><strong>{l.driver_name}</strong><span className="muted">{l.plate}{l.is_primary ? " · principal" : ""}</span></div>
                <button type="button" className="btn btn--sm" onClick={() => unlink.mutate(l.id)}>Remover</button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
