import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { apiFetch, ApiError } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { can, isAdminUser } from "../lib/auth";
import { toastErr, toastOk } from "../lib/operationToast";
import { pageCachedQueryOptions, PAGE_DATA_GC_MS, PAGE_DATA_STALE_MS, wrapPageCachedQueryFn } from "../lib/pageDataCache";
import { queryKeys } from "../lib/queryKeys";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { PageCountPill } from "../components/PageCountPill";
import { PopLocationPicker } from "../components/PopLocationPicker";

const UF_OPTIONS = [
  "AC","AL","AP","AM","BA","CE","DF","ES","GO","MA","MT","MS","MG",
  "PA","PB","PR","PE","PI","RJ","RN","RS","RO","RR","SC","SP","SE","TO",
];

type Locality = {
  id: string;
  name: string;
  uf?: string | null;
  region_code?: string | null;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  client_count?: number | null;
  client_month?: string | null;
  vlans?: string[];
  pops?: { id: string; description: string; device_count: number }[];
  pop_id?: string | null;
  pop_description?: string | null;
};

type SharedVLAN = { vlan: string; localities: string[] };

type LocForm = {
  name: string;
  uf: string;
  address: string;
  lat: string;
  lon: string;
  createPop: boolean;
  popName: string;
  vlans: string[];
};

const emptyForm = (): LocForm => ({
  name: "",
  uf: "",
  address: "",
  lat: "",
  lon: "",
  createPop: false,
  popName: "",
  vlans: [],
});

function parseCoord(v: string): number | null {
  const s = String(v ?? "").trim().replace(",", ".");
  if (!s) return null;
  const n = Number(s);
  return Number.isFinite(n) ? n : null;
}

function formToPayload(f: LocForm, confirmShared = false) {
  const latitude = parseCoord(f.lat);
  const longitude = parseCoord(f.lon);
  return {
    name: f.name.trim(),
    uf: f.uf.trim() || null,
    region_code: f.uf.trim() || null,
    address: f.address.trim() || null,
    latitude,
    longitude,
    create_pop: f.createPop,
    pop_name: f.popName.trim() || null,
    vlans: f.vlans,
    confirm_shared_vlan: confirmShared,
  };
}

export function PopsPage() {
  const canMutate = isAdminUser() || can("pops.manage") || can("commercial.manage");
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();

  const list = useQuery({
    queryKey: queryKeys.commercialLocalities,
    queryFn: wrapPageCachedQueryFn(queryKeys.commercialLocalities, () =>
      apiFetch<{ localities: Locality[] }>("/api/v1/commercial/localities"),
    ),
    ...pageCachedQueryOptions<{ localities: Locality[] }>(
      queryKeys.commercialLocalities,
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
  });

  const bngVlans = useQuery({
    queryKey: ["bng-collected-vlans"],
    queryFn: () => apiFetch<{ vlans: string[] }>("/api/v1/commercial/bng-vlans"),
    staleTime: 60_000,
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [edit, setEdit] = useState<Locality | null>(null);
  const [form, setForm] = useState<LocForm>(emptyForm());
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [sharedWarn, setSharedWarn] = useState<{ shared: SharedVLAN[]; pending: "create" | "edit" } | null>(null);
  const [q, setQ] = useState("");

  const rows = useMemo(() => {
    const all = list.data?.localities ?? [];
    const needle = q.trim().toLowerCase();
    if (!needle) return all;
    return all.filter((l) => {
      const blob = [l.name, l.uf, l.address, ...(l.vlans ?? []), l.pop_description]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return blob.includes(needle);
    });
  }, [list.data, q]);

  const openCreate = () => {
    setForm(emptyForm());
    setCreateOpen(true);
  };

  const openEdit = (l: Locality) => {
    setEdit(l);
    setForm({
      name: l.name,
      uf: l.uf || l.region_code || "",
      address: l.address || "",
      lat: l.latitude != null ? String(l.latitude) : "",
      lon: l.longitude != null ? String(l.longitude) : "",
      createPop: false,
      popName: "",
      vlans: [...(l.vlans ?? [])],
    });
  };

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: queryKeys.commercialLocalities });
    qc.invalidateQueries({ queryKey: queryKeys.pops });
  };

  const handleSharedConflict = (err: unknown, pending: "create" | "edit") => {
    if (err instanceof ApiError && err.status === 409) {
      const body = err.body as { error?: string; shared_vlans?: SharedVLAN[]; requires_confirm?: boolean } | undefined;
      if (body?.error === "SHARED_VLAN" || body?.requires_confirm) {
        setSharedWarn({ shared: body.shared_vlans ?? [], pending });
        return true;
      }
    }
    return false;
  };

  const create = useMutation({
    mutationFn: (confirmShared: boolean) =>
      apiFetch<{ id: string }>("/api/v1/commercial/localities", {
        method: "POST",
        json: formToPayload(form, confirmShared),
      }),
    onSuccess: () => {
      invalidate();
      setCreateOpen(false);
      setSharedWarn(null);
      setForm(emptyForm());
      toastOk(pushToast, "Localidade criada.");
    },
    onError: (err) => {
      if (!handleSharedConflict(err, "create")) toastErr(pushToast, err, "Falha ao criar localidade.");
    },
  });

  const patch = useMutation({
    mutationFn: (confirmShared: boolean) =>
      apiFetch(`/api/v1/commercial/localities/${edit!.id}`, {
        method: "PATCH",
        json: formToPayload(form, confirmShared),
      }),
    onSuccess: () => {
      invalidate();
      setEdit(null);
      setSharedWarn(null);
      toastOk(pushToast, "Localidade actualizada.");
    },
    onError: (err) => {
      if (!handleSharedConflict(err, "edit")) toastErr(pushToast, err, "Falha ao actualizar localidade.");
    },
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/localities/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      invalidate();
      setDeleteId(null);
      toastOk(pushToast, "Localidade eliminada.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao eliminar localidade."),
  });

  const toggleVlan = (vlan: string) => {
    setForm((f) => ({
      ...f,
      vlans: f.vlans.includes(vlan) ? f.vlans.filter((v) => v !== vlan) : [...f.vlans, vlan].sort(),
    }));
  };

  const formModal = (mode: "create" | "edit") => {
    const open = mode === "create" ? createOpen : !!edit;
    if (!open) return null;
    const title = mode === "create" ? "Nova localidade" : "Editar localidade";
    const busy = create.isPending || patch.isPending;
    const collected = bngVlans.data?.vlans ?? [];
    const hasPop = mode === "edit" && (edit?.pops?.length ?? 0) > 0;

    return (
      <div className="modal-backdrop" role="presentation" onMouseDown={() => (mode === "create" ? setCreateOpen(false) : setEdit(null))}>
        <div
          className="modal"
          role="dialog"
          aria-modal="true"
          style={{ maxWidth: 720, width: "min(96vw, 720px)", maxHeight: "92vh", overflow: "auto" }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <h3 style={{ marginTop: 0 }}>{title}</h3>
          <div className="field">
            <label>Nome</label>
            <input className="input" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div className="row" style={{ gap: 10, flexWrap: "wrap" }}>
            <div className="field" style={{ flex: "1 1 120px" }}>
              <label>UF</label>
              <select className="input" value={form.uf} onChange={(e) => setForm((f) => ({ ...f, uf: e.target.value }))}>
                <option value="">—</option>
                {UF_OPTIONS.map((uf) => (
                  <option key={uf} value={uf}>
                    {uf}
                  </option>
                ))}
              </select>
            </div>
            <div className="field" style={{ flex: "3 1 240px" }}>
              <label>Endereço</label>
              <input className="input" value={form.address} onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))} />
            </div>
          </div>
          <div className="field">
            <label>Coordenadas</label>
            <div className="row" style={{ gap: 8 }}>
              <input className="input mono" placeholder="Latitude" value={form.lat} onChange={(e) => setForm((f) => ({ ...f, lat: e.target.value }))} />
              <input className="input mono" placeholder="Longitude" value={form.lon} onChange={(e) => setForm((f) => ({ ...f, lon: e.target.value }))} />
            </div>
            <div style={{ marginTop: 8 }}>
              <PopLocationPicker
                latitude={parseCoord(form.lat)}
                longitude={parseCoord(form.lon)}
                onChange={(la, lo) => setForm((f) => ({ ...f, lat: String(la), lon: String(lo) }))}
              />
            </div>
          </div>

          <div className="field" style={{ marginTop: 8 }}>
            <label style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <input
                type="checkbox"
                checked={hasPop || form.createPop}
                disabled={hasPop}
                onChange={(e) => setForm((f) => ({ ...f, createPop: e.target.checked }))}
              />
              {hasPop
                ? `POP associado: ${edit?.pops?.map((p) => p.description).join(", ")}`
                : "Criar POP para esta localidade (opcional)"}
            </label>
            {!hasPop && form.createPop && (
              <input
                className="input"
                style={{ marginTop: 6 }}
                placeholder="Nome do POP (predefinido = nome da localidade)"
                value={form.popName}
                onChange={(e) => setForm((f) => ({ ...f, popName: e.target.value }))}
              />
            )}
          </div>

          <div className="field">
            <label>VLANs do BNG</label>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>
              Seleccione VLANs colectadas no BNG para atrelar a esta localidade. A mesma VLAN pode existir em várias localidades (com aviso).
            </p>
            {collected.length === 0 ? (
              <p style={{ fontSize: 12, color: "var(--muted)" }}>
                Nenhuma VLAN encontrada. Execute a consulta completa SNMP no BNG primeiro.
              </p>
            ) : (
              <div style={{ display: "flex", flexWrap: "wrap", gap: 6, maxHeight: 160, overflow: "auto" }}>
                {collected.map((v) => {
                  const on = form.vlans.includes(v);
                  return (
                    <button
                      key={v}
                      type="button"
                      className={on ? "btn btn--primary btn--sm" : "btn btn--sm"}
                      onClick={() => toggleVlan(v)}
                    >
                      VLAN {v}
                    </button>
                  );
                })}
              </div>
            )}
            {form.vlans.length > 0 && (
              <p style={{ fontSize: 12, marginTop: 8 }}>
                Seleccionadas: <span className="mono">{form.vlans.join(", ")}</span>
              </p>
            )}
          </div>

          <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
            <button type="button" className="btn" onClick={() => (mode === "create" ? setCreateOpen(false) : setEdit(null))}>
              Cancelar
            </button>
            <button
              type="button"
              className="btn btn--primary"
              disabled={!form.name.trim() || busy}
              onClick={() => (mode === "create" ? create.mutate(false) : patch.mutate(false))}
            >
              {busy ? "A guardar…" : "Guardar"}
            </button>
          </div>
        </div>
      </div>
    );
  };

  return (
    <div className="page">
      <div className="page-header" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, flexWrap: "wrap" }}>
        <div>
          <h1 style={{ margin: 0, display: "flex", alignItems: "center", gap: 10 }}>
            Localidades
            <PageCountPill label="Localidades" count={rows.length} />
          </h1>
          <p style={{ color: "var(--muted)", fontSize: 13, margin: "6px 0 0" }}>
            Cadastro unificado com endereço, UF, coordenadas, POP opcional e VLANs do BNG. Contagem de clientes vem dos registos mensais.
          </p>
        </div>
        {canMutate && (
          <button type="button" className="btn btn--primary" onClick={openCreate}>
            Nova localidade
          </button>
        )}
      </div>

      <div className="card" style={{ padding: 12, marginBottom: 12 }}>
        <input
          className="input"
          placeholder="Pesquisar por nome, UF, VLAN, POP…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>

      {list.isLoading ? (
        <p style={{ color: "var(--muted)" }}>A carregar…</p>
      ) : rows.length === 0 ? (
        <p style={{ color: "var(--muted)" }}>Ainda não há localidades cadastradas.</p>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Nome</th>
                <th>UF</th>
                <th>Endereço</th>
                <th>Coords</th>
                <th>POP</th>
                <th>VLANs</th>
                <th>Clientes</th>
                <th style={{ width: 80 }} />
              </tr>
            </thead>
            <tbody>
              {rows.map((l) => (
                <tr key={l.id}>
                  <td>{l.name}</td>
                  <td className="mono">{l.uf || l.region_code || "—"}</td>
                  <td style={{ maxWidth: 220, overflow: "hidden", textOverflow: "ellipsis" }}>{l.address || "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {l.latitude != null && l.longitude != null
                      ? `${l.latitude.toFixed(5)}, ${l.longitude.toFixed(5)}`
                      : "—"}
                  </td>
                  <td>{l.pop_description || (l.pops?.length ? l.pops.map((p) => p.description).join(", ") : "—")}</td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {(l.vlans ?? []).length ? l.vlans!.join(", ") : "—"}
                  </td>
                  <td>
                    {l.client_count != null ? (
                      <span title={l.client_month ? `Mês ${l.client_month}` : undefined}>
                        {l.client_count.toLocaleString("pt-BR")}
                      </span>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td>
                    {canMutate && (
                      <ActionMenu
                        items={[
                          { id: "edit", label: "Editar", onClick: () => openEdit(l) },
                          { id: "del", label: "Eliminar", danger: true, onClick: () => setDeleteId(l.id) },
                        ]}
                      />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formModal("create")}
      {formModal("edit")}

      <ConfirmModal
        open={!!deleteId}
        title="Eliminar localidade"
        message="Isto remove a localidade e as VLANs associadas. Registos mensais de clientes também serão removidos (CASCADE). POPs associados bloqueiam a exclusão."
        confirmLabel="Eliminar"
        danger
        busy={del.isPending}
        onCancel={() => setDeleteId(null)}
        onConfirm={() => deleteId && del.mutate(deleteId)}
      />

      <ConfirmModal
        open={!!sharedWarn}
        title="VLAN já usada noutra localidade"
        message={
          sharedWarn
            ? `As seguintes VLANs já estão atreladas a outras localidades:\n\n${sharedWarn.shared
                .map((s) => `VLAN ${s.vlan}: ${s.localities.join(", ")}`)
                .join("\n")}\n\nDeseja continuar mesmo assim?`
            : ""
        }
        confirmLabel="Confirmar e guardar"
        busy={create.isPending || patch.isPending}
        onCancel={() => setSharedWarn(null)}
        onConfirm={() => {
          if (!sharedWarn) return;
          if (sharedWarn.pending === "create") create.mutate(true);
          else patch.mutate(true);
        }}
      />
    </div>
  );
}
