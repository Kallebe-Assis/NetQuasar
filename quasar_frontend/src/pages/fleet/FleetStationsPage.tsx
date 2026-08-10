import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { UF_OPTIONS } from "./fleetUtils";

type Station = {
  id: string; description: string; cnpj?: string | null; city?: string | null; uf?: string | null;
  station_kind: string; status: string; phone?: string | null; trade_name?: string | null;
};

export function FleetStationsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState({
    description: "", trade_name: "", cnpj: "", phone: "", city: "", uf: "",
    station_kind: "conveniado", status: "active", fuel_ids: [] as string[],
  });
  const stations = useQuery({ queryKey: queryKeys.fleetStations, queryFn: () => apiFetch<{ items: Station[] }>("/api/v1/fleet/stations") });
  const fuels = useQuery({ queryKey: queryKeys.fleetFuels, queryFn: () => apiFetch<{ items: { id: string; description: string }[] }>("/api/v1/fleet/fuels?active=1") });

  const save = useMutation({
    mutationFn: async () => {
      const payload = {
        description: form.description.trim(),
        trade_name: form.trade_name.trim() || null,
        cnpj: form.cnpj.trim() || null,
        phone: form.phone.trim() || null,
        city: form.city.trim() || null,
        uf: form.uf || null,
        station_kind: form.station_kind,
        status: form.status,
        fuel_ids: form.fuel_ids,
      };
      if (editing) await apiFetch(`/api/v1/fleet/stations/${editing}`, { method: "PATCH", body: JSON.stringify(payload) });
      else await apiFetch("/api/v1/fleet/stations", { method: "POST", body: JSON.stringify(payload) });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Posto actualizado" : "Posto criado");
      setEditing(null);
      setForm({ description: "", trade_name: "", cnpj: "", phone: "", city: "", uf: "", station_kind: "conveniado", status: "active", fuel_ids: [] });
      await qc.invalidateQueries({ queryKey: queryKeys.fleetStations });
    },
    onError: (e) => toastErr(push, e),
  });

  async function startEdit(id: string) {
    const st = await apiFetch<Station & { fuel_ids?: string[] }>(`/api/v1/fleet/stations/${id}`);
    setEditing(id);
    setForm({
      description: st.description,
      trade_name: st.trade_name ?? "",
      cnpj: st.cnpj ?? "",
      phone: st.phone ?? "",
      city: st.city ?? "",
      uf: st.uf ?? "",
      station_kind: st.station_kind,
      status: st.status,
      fuel_ids: st.fuel_ids ?? [],
    });
  }

  return (
    <div className="fleet-page">
      <h1>Frota — Postos</h1>
      {canMutate ? (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="fleet-form-grid">
            <label>Descrição*<input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
            <label>Nome fantasia<input className="input" value={form.trade_name} onChange={(e) => setForm({ ...form, trade_name: e.target.value })} /></label>
            <label>CNPJ<input className="input" value={form.cnpj} onChange={(e) => setForm({ ...form, cnpj: e.target.value })} /></label>
            <label>Telefone<input className="input" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} /></label>
            <label>Cidade<input className="input" value={form.city} onChange={(e) => setForm({ ...form, city: e.target.value })} /></label>
            <label>UF<select className="input" value={form.uf} onChange={(e) => setForm({ ...form, uf: e.target.value })}><option value="">—</option>{UF_OPTIONS.map((u) => <option key={u} value={u}>{u}</option>)}</select></label>
            <label>Tipo<select className="input" value={form.station_kind} onChange={(e) => setForm({ ...form, station_kind: e.target.value })}>
              <option value="conveniado">Conveniado</option><option value="proprio">Próprio</option><option value="fornecedor">Fornecedor</option><option value="other">Outro</option>
            </select></label>
            <label>Status<select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
              <option value="active">Activo</option><option value="inactive">Inactivo</option>
            </select></label>
            <label className="fleet-form-span">Combustíveis
              <select className="input" multiple value={form.fuel_ids} onChange={(e) => setForm({ ...form, fuel_ids: Array.from(e.target.selectedOptions).map((o) => o.value) })} style={{ minHeight: 90 }}>
                {(fuels.data?.items ?? []).map((f) => <option key={f.id} value={f.id}>{f.description}</option>)}
              </select>
            </label>
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button type="button" className="btn btn--primary" disabled={!form.description.trim() || save.isPending} onClick={() => save.mutate()}>{editing ? "Guardar" : "Criar"}</button>
            {editing ? <button type="button" className="btn" onClick={() => setEditing(null)}>Cancelar</button> : null}
          </div>
        </div>
      ) : null}
      <div className="card table-wrap">
        <table>
          <thead><tr><th>Descrição</th><th>CNPJ</th><th>Cidade</th><th>Tipo</th><th>Status</th><th /></tr></thead>
          <tbody>
            {(stations.data?.items ?? []).map((s) => (
              <tr key={s.id}>
                <td>{s.description}</td><td>{s.cnpj ?? "—"}</td><td>{[s.city, s.uf].filter(Boolean).join("/") || "—"}</td>
                <td>{s.station_kind}</td><td>{s.status}</td>
                <td>{canMutate ? <button type="button" className="btn btn--sm" onClick={() => void startEdit(s.id)}>Editar</button> : null}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
