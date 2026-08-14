import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { apiFetch, ApiError } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { can, isAdminUser } from "../lib/auth";
import { copyTextToClipboard } from "../lib/clipboard";
import { formatLatLng, googleMapsUrl } from "../lib/geoClipboard";
import { toastErr, toastInfo, toastOk } from "../lib/operationToast";
import { pageCachedQueryOptions, PAGE_DATA_GC_MS, PAGE_DATA_STALE_MS, wrapPageCachedQueryFn } from "../lib/pageDataCache";
import { queryKeys } from "../lib/queryKeys";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { PageCountPill } from "../components/PageCountPill";
import { PopLocationPicker } from "../components/PopLocationPicker";

const UF_OPTIONS = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG",
  "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO",
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

type PopRow = {
  id: string;
  description: string;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  locality_id?: string | null;
  locality_name?: string | null;
  device_count?: number;
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

type PopForm = {
  description: string;
  address: string;
  lat: string;
  lon: string;
  localityId: string; // "" = sem localidade
};

type Tab = "localidades" | "pops";

const emptyLocForm = (): LocForm => ({
  name: "",
  uf: "",
  address: "",
  lat: "",
  lon: "",
  createPop: false,
  popName: "",
  vlans: [],
});

const emptyPopForm = (): PopForm => ({
  description: "",
  address: "",
  lat: "",
  lon: "",
  localityId: "",
});

function parseCoord(v: string): number | null {
  const s = String(v ?? "").trim().replace(",", ".");
  if (!s) return null;
  const n = Number(s);
  return Number.isFinite(n) ? n : null;
}

function locFormToPayload(f: LocForm, confirmShared = false) {
  return {
    name: f.name.trim(),
    uf: f.uf.trim() || null,
    region_code: f.uf.trim() || null,
    address: f.address.trim() || null,
    latitude: parseCoord(f.lat),
    longitude: parseCoord(f.lon),
    create_pop: f.createPop,
    pop_name: f.popName.trim() || null,
    vlans: f.vlans,
    confirm_shared_vlan: confirmShared,
  };
}

function popFormToPayload(f: PopForm) {
  return {
    description: f.description.trim(),
    address: f.address.trim() || null,
    latitude: parseCoord(f.lat),
    longitude: parseCoord(f.lon),
    locality_id: f.localityId.trim() || null,
  };
}

export function PopsPage() {
  const canMutate = isAdminUser() || can("pops.manage") || can("commercial.manage");
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [tab, setTab] = useState<Tab>("localidades");
  const [q, setQ] = useState("");

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

  const popsList = useQuery({
    queryKey: queryKeys.pops,
    queryFn: wrapPageCachedQueryFn(queryKeys.pops, () =>
      apiFetch<{ pops: PopRow[] }>("/api/v1/pops"),
    ),
    ...pageCachedQueryOptions<{ pops: PopRow[] }>(queryKeys.pops, PAGE_DATA_STALE_MS, PAGE_DATA_GC_MS),
  });

  const bngVlans = useQuery({
    queryKey: ["bng-collected-vlans"],
    queryFn: () => apiFetch<{ vlans: string[] }>("/api/v1/commercial/bng-vlans"),
    staleTime: 60_000,
  });

  const [createLocOpen, setCreateLocOpen] = useState(false);
  const [editLoc, setEditLoc] = useState<Locality | null>(null);
  const [locForm, setLocForm] = useState<LocForm>(emptyLocForm());
  const [deleteLocId, setDeleteLocId] = useState<string | null>(null);
  const [sharedWarn, setSharedWarn] = useState<{ shared: SharedVLAN[]; pending: "create" | "edit" } | null>(null);

  const [createPopOpen, setCreatePopOpen] = useState(false);
  const [editPop, setEditPop] = useState<PopRow | null>(null);
  const [popForm, setPopForm] = useState<PopForm>(emptyPopForm());
  const [deletePopId, setDeletePopId] = useState<string | null>(null);

  function hasCoords(lat?: number | null, lon?: number | null) {
    return lat != null && lon != null && Number.isFinite(lat) && Number.isFinite(lon);
  }

  async function copyCoords(lat?: number | null, lon?: number | null) {
    if (!hasCoords(lat, lon)) {
      toastInfo(pushToast, "Este POP ainda não possui coordenadas.");
      return;
    }
    const txt = formatLatLng(lat!, lon!);
    const ok = await copyTextToClipboard(txt);
    if (ok) toastOk(pushToast, "Coordenadas copiadas.");
    else toastErr(pushToast, new Error(`Não foi possível copiar automaticamente. Copie manualmente: ${txt}`));
  }

  async function copyMapsLink(lat?: number | null, lon?: number | null) {
    if (!hasCoords(lat, lon)) {
      toastInfo(pushToast, "Este POP ainda não possui coordenadas.");
      return;
    }
    const url = googleMapsUrl(lat!, lon!);
    const ok = await copyTextToClipboard(url);
    if (ok) toastOk(pushToast, "Link do Google Maps copiado.");
    else toastErr(pushToast, new Error(`Não foi possível copiar automaticamente. Copie manualmente: ${url}`));
  }

  const locRows = useMemo(() => {
    const all = list.data?.localities ?? [];
    const needle = q.trim().toLowerCase();
    if (!needle || tab !== "localidades") return all;
    return all.filter((l) => {
      const blob = [l.name, l.uf, l.address, ...(l.vlans ?? []), ...(l.pops ?? []).map((p) => p.description)]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return blob.includes(needle);
    });
  }, [list.data, q, tab]);

  const popRows = useMemo(() => {
    const all = popsList.data?.pops ?? [];
    const needle = q.trim().toLowerCase();
    if (!needle || tab !== "pops") return all;
    return all.filter((p) => {
      const blob = [p.description, p.address, p.locality_name].filter(Boolean).join(" ").toLowerCase();
      return blob.includes(needle);
    });
  }, [popsList.data, q, tab]);

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

  const openCreateLoc = () => {
    setLocForm(emptyLocForm());
    setCreateLocOpen(true);
  };

  const openEditLoc = (l: Locality) => {
    setEditLoc(l);
    setLocForm({
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

  const openCreatePop = () => {
    setPopForm(emptyPopForm());
    setCreatePopOpen(true);
  };

  const openEditPop = (p: PopRow) => {
    setEditPop(p);
    setPopForm({
      description: p.description,
      address: p.address || "",
      lat: p.latitude != null ? String(p.latitude) : "",
      lon: p.longitude != null ? String(p.longitude) : "",
      localityId: p.locality_id ?? "",
    });
  };

  const createLoc = useMutation({
    mutationFn: (confirmShared: boolean) =>
      apiFetch<{ id: string }>("/api/v1/commercial/localities", {
        method: "POST",
        json: locFormToPayload(locForm, confirmShared),
      }),
    onSuccess: () => {
      invalidate();
      setCreateLocOpen(false);
      setSharedWarn(null);
      setLocForm(emptyLocForm());
      toastOk(pushToast, "Localidade criada.");
    },
    onError: (err) => {
      if (!handleSharedConflict(err, "create")) toastErr(pushToast, err, "Falha ao criar localidade.");
    },
  });

  const patchLoc = useMutation({
    mutationFn: (confirmShared: boolean) =>
      apiFetch(`/api/v1/commercial/localities/${editLoc!.id}`, {
        method: "PATCH",
        json: locFormToPayload(locForm, confirmShared),
      }),
    onSuccess: () => {
      invalidate();
      setEditLoc(null);
      setSharedWarn(null);
      toastOk(pushToast, "Localidade atualizada.");
    },
    onError: (err) => {
      if (!handleSharedConflict(err, "edit")) toastErr(pushToast, err, "Falha ao atualizar localidade.");
    },
  });

  const delLoc = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/localities/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      invalidate();
      setDeleteLocId(null);
      toastOk(pushToast, "Localidade eliminada.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao eliminar localidade."),
  });

  const createPop = useMutation({
    mutationFn: () => apiFetch<{ id: string }>("/api/v1/pops", { method: "POST", json: popFormToPayload(popForm) }),
    onSuccess: () => {
      invalidate();
      setCreatePopOpen(false);
      setPopForm(emptyPopForm());
      toastOk(pushToast, "POP criado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao criar POP."),
  });

  const patchPop = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/pops/${editPop!.id}`, { method: "PATCH", json: popFormToPayload(popForm) }),
    onSuccess: () => {
      invalidate();
      setEditPop(null);
      toastOk(pushToast, "POP atualizado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao atualizar POP."),
  });

  const unlinkPop = useMutation({
    mutationFn: (popId: string) =>
      apiFetch(`/api/v1/pops/${popId}`, { method: "PATCH", json: { locality_id: null } }),
    onSuccess: (_data, popId) => {
      invalidate();
      setEditLoc((prev) =>
        prev
          ? {
              ...prev,
              pops: (prev.pops ?? []).filter((p) => p.id !== popId),
              pop_id: prev.pop_id === popId ? null : prev.pop_id,
              pop_description: prev.pop_id === popId ? null : prev.pop_description,
            }
          : null,
      );
      toastOk(pushToast, "POP desvinculado da localidade.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao desvincular POP."),
  });

  const delPop = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/pops/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      invalidate();
      setDeletePopId(null);
      toastOk(pushToast, "POP eliminado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao eliminar POP."),
  });

  const toggleVlan = (vlan: string) => {
    setLocForm((f) => ({
      ...f,
      vlans: f.vlans.includes(vlan) ? f.vlans.filter((v) => v !== vlan) : [...f.vlans, vlan].sort(),
    }));
  };

  const localityOptions = list.data?.localities ?? [];

  const locModal = (mode: "create" | "edit") => {
    const open = mode === "create" ? createLocOpen : !!editLoc;
    if (!open) return null;
    const title = mode === "create" ? "Nova localidade" : "Editar localidade";
    const busy = createLoc.isPending || patchLoc.isPending;
    const collected = bngVlans.data?.vlans ?? [];
    const linkedPops = mode === "edit" ? editLoc?.pops ?? [] : [];

    return (
      <div className="modal-backdrop" role="presentation" onMouseDown={() => (mode === "create" ? setCreateLocOpen(false) : setEditLoc(null))}>
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
            <input className="input" value={locForm.name} onChange={(e) => setLocForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div className="row" style={{ gap: 10, flexWrap: "wrap" }}>
            <div className="field" style={{ flex: "1 1 120px" }}>
              <label>UF</label>
              <select className="input" value={locForm.uf} onChange={(e) => setLocForm((f) => ({ ...f, uf: e.target.value }))}>
                <option value="">—</option>
                {UF_OPTIONS.map((uf) => (
                  <option key={uf} value={uf}>{uf}</option>
                ))}
              </select>
            </div>
            <div className="field" style={{ flex: "3 1 240px" }}>
              <label>Endereço</label>
              <input className="input" value={locForm.address} onChange={(e) => setLocForm((f) => ({ ...f, address: e.target.value }))} />
            </div>
          </div>
          <div className="field">
            <label>Coordenadas</label>
            <div className="row" style={{ gap: 8 }}>
              <input className="input mono" placeholder="Latitude" value={locForm.lat} onChange={(e) => setLocForm((f) => ({ ...f, lat: e.target.value }))} />
              <input className="input mono" placeholder="Longitude" value={locForm.lon} onChange={(e) => setLocForm((f) => ({ ...f, lon: e.target.value }))} />
            </div>
            <div style={{ marginTop: 8 }}>
              <PopLocationPicker
                latitude={parseCoord(locForm.lat)}
                longitude={parseCoord(locForm.lon)}
                onChange={(la, lo) => setLocForm((f) => ({ ...f, lat: String(la), lon: String(lo) }))}
              />
            </div>
          </div>

          {linkedPops.length > 0 && (
            <div className="field" style={{ marginTop: 8 }}>
              <label>POPs vinculados ({linkedPops.length})</label>
              <ul style={{ margin: "6px 0 0", paddingLeft: 18 }}>
                {linkedPops.map((p) => (
                  <li key={p.id} style={{ marginBottom: 6, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                    <span>{p.description}</span>
                    <span className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
                      {p.device_count} equip.
                    </span>
                    {canMutate && (
                      <button
                        type="button"
                        className="btn btn--sm"
                        disabled={unlinkPop.isPending}
                        onClick={() => unlinkPop.mutate(p.id)}
                      >
                        Desvincular
                      </button>
                    )}
                  </li>
                ))}
              </ul>
              <p style={{ fontSize: 12, color: "var(--muted)", margin: "6px 0 0" }}>
                Desvincular mantém o POP (aba POPs) sem localidade. Pode voltar a associá-lo depois.
              </p>
            </div>
          )}

          <div className="field" style={{ marginTop: 8 }}>
            <label style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <input
                type="checkbox"
                checked={locForm.createPop}
                onChange={(e) => setLocForm((f) => ({ ...f, createPop: e.target.checked }))}
              />
              {mode === "edit" && linkedPops.length > 0
                ? "Criar outro POP nesta localidade"
                : "Criar POP para esta localidade (opcional)"}
            </label>
            {locForm.createPop && (
              <input
                className="input"
                style={{ marginTop: 6 }}
                placeholder="Nome do POP (predefinido = nome da localidade)"
                value={locForm.popName}
                onChange={(e) => setLocForm((f) => ({ ...f, popName: e.target.value }))}
              />
            )}
          </div>

          <div className="field">
            <label>VLANs do BNG</label>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>
              Seleccione VLANs colectadas no BNG para atrelar a esta localidade.
            </p>
            {collected.length === 0 ? (
              <p style={{ fontSize: 12, color: "var(--muted)" }}>
                Nenhuma VLAN encontrada. Execute a consulta completa SNMP no BNG primeiro.
              </p>
            ) : (
              <div style={{ display: "flex", flexWrap: "wrap", gap: 6, maxHeight: 160, overflow: "auto" }}>
                {collected.map((v) => {
                  const on = locForm.vlans.includes(v);
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
            {locForm.vlans.length > 0 && (
              <p style={{ fontSize: 12, marginTop: 8 }}>
                Seleccionadas: <span className="mono">{locForm.vlans.join(", ")}</span>
              </p>
            )}
          </div>

          <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
            <button type="button" className="btn" onClick={() => (mode === "create" ? setCreateLocOpen(false) : setEditLoc(null))}>
              Cancelar
            </button>
            <button
              type="button"
              className="btn btn--primary"
              disabled={!locForm.name.trim() || busy}
              onClick={() => (mode === "create" ? createLoc.mutate(false) : patchLoc.mutate(false))}
            >
              {busy ? "A guardar…" : "Guardar"}
            </button>
          </div>
        </div>
      </div>
    );
  };

  const popModal = (mode: "create" | "edit") => {
    const open = mode === "create" ? createPopOpen : !!editPop;
    if (!open) return null;
    const title = mode === "create" ? "Novo POP" : "Editar POP";
    const busy = createPop.isPending || patchPop.isPending;

    return (
      <div className="modal-backdrop" role="presentation" onMouseDown={() => (mode === "create" ? setCreatePopOpen(false) : setEditPop(null))}>
        <div
          className="modal"
          role="dialog"
          aria-modal="true"
          style={{ maxWidth: 640, width: "min(96vw, 640px)", maxHeight: "92vh", overflow: "auto" }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <h3 style={{ marginTop: 0 }}>{title}</h3>
          <div className="field">
            <label>Nome / descrição</label>
            <input
              className="input"
              value={popForm.description}
              onChange={(e) => setPopForm((f) => ({ ...f, description: e.target.value }))}
            />
          </div>
          <div className="field">
            <label>Localidade</label>
            <select
              className="input"
              value={popForm.localityId}
              onChange={(e) => setPopForm((f) => ({ ...f, localityId: e.target.value }))}
            >
              <option value="">— Sem localidade —</option>
              {localityOptions.map((l) => (
                <option key={l.id} value={l.id}>{l.name}</option>
              ))}
            </select>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "6px 0 0" }}>
              Pode deixar sem localidade ou mudar a associação a qualquer momento.
            </p>
          </div>
          <div className="field">
            <label>Endereço</label>
            <input className="input" value={popForm.address} onChange={(e) => setPopForm((f) => ({ ...f, address: e.target.value }))} />
          </div>
          <div className="field">
            <label>Coordenadas</label>
            <div className="row" style={{ gap: 8 }}>
              <input className="input mono" placeholder="Latitude" value={popForm.lat} onChange={(e) => setPopForm((f) => ({ ...f, lat: e.target.value }))} />
              <input className="input mono" placeholder="Longitude" value={popForm.lon} onChange={(e) => setPopForm((f) => ({ ...f, lon: e.target.value }))} />
            </div>
            <div style={{ marginTop: 8 }}>
              <PopLocationPicker
                latitude={parseCoord(popForm.lat)}
                longitude={parseCoord(popForm.lon)}
                onChange={(la, lo) => setPopForm((f) => ({ ...f, lat: String(la), lon: String(lo) }))}
              />
            </div>
          </div>
          <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
            <button type="button" className="btn" onClick={() => (mode === "create" ? setCreatePopOpen(false) : setEditPop(null))}>
              Cancelar
            </button>
            <button
              type="button"
              className="btn btn--primary"
              disabled={!popForm.description.trim() || busy}
              onClick={() => (mode === "create" ? createPop.mutate() : patchPop.mutate())}
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
            <PageCountPill
              label={tab === "localidades" ? "Localidades" : "POPs"}
              count={tab === "localidades" ? locRows.length : popRows.length}
            />
          </h1>
          <p style={{ color: "var(--muted)", fontSize: 13, margin: "6px 0 0" }}>
            Cada localidade pode ter 0, 1 ou vários POPs. POPs podem existir sem localidade.
          </p>
        </div>
        {canMutate && (
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => (tab === "localidades" ? openCreateLoc() : openCreatePop())}
          >
            {tab === "localidades" ? "Nova localidade" : "Novo POP"}
          </button>
        )}
      </div>

      <div className="tabs" style={{ marginBottom: 12 }}>
        <button type="button" className={tab === "localidades" ? "active" : ""} onClick={() => { setTab("localidades"); setQ(""); }}>
          Localidades
        </button>
        <button type="button" className={tab === "pops" ? "active" : ""} onClick={() => { setTab("pops"); setQ(""); }}>
          POPs
        </button>
      </div>

      <div className="card" style={{ padding: 12, marginBottom: 12 }}>
        <input
          className="input"
          placeholder={tab === "localidades" ? "Pesquisar por nome, UF, VLAN, POP…" : "Pesquisar POP ou localidade…"}
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>

      {tab === "localidades" && (
        <>
          {list.isLoading ? (
            <p style={{ color: "var(--muted)" }}>A carregar…</p>
          ) : locRows.length === 0 ? (
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
                    <th>POPs</th>
                    <th>VLANs</th>
                    <th>Clientes</th>
                    <th style={{ width: 80 }} />
                  </tr>
                </thead>
                <tbody>
                  {locRows.map((l) => {
                    const pops = l.pops ?? [];
                    return (
                      <tr key={l.id}>
                        <td>{l.name}</td>
                        <td className="mono">{l.uf || l.region_code || "—"}</td>
                        <td style={{ maxWidth: 220, overflow: "hidden", textOverflow: "ellipsis" }}>{l.address || "—"}</td>
                        <td className="mono" style={{ fontSize: 11 }}>
                          {l.latitude != null && l.longitude != null
                            ? `${l.latitude.toFixed(5)}, ${l.longitude.toFixed(5)}`
                            : "—"}
                        </td>
                        <td>
                          {pops.length === 0 ? (
                            <span style={{ color: "var(--muted)" }}>0</span>
                          ) : (
                            <span title={pops.map((p) => p.description).join(", ")}>
                              {pops.length}: {pops.map((p) => p.description).join(", ")}
                            </span>
                          )}
                        </td>
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
                                { id: "edit", label: "Editar", onClick: () => openEditLoc(l) },
                                { id: "del", label: "Eliminar", danger: true, onClick: () => setDeleteLocId(l.id) },
                              ]}
                            />
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      {tab === "pops" && (
        <>
          {popsList.isLoading ? (
            <p style={{ color: "var(--muted)" }}>A carregar…</p>
          ) : popRows.length === 0 ? (
            <p style={{ color: "var(--muted)" }}>Ainda não há POPs cadastrados.</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Nome</th>
                    <th>Localidade</th>
                    <th>Endereço</th>
                    <th>Coords</th>
                    <th>Equipamentos</th>
                    <th style={{ width: 80 }} />
                  </tr>
                </thead>
                <tbody>
                  {popRows.map((p) => (
                    <tr key={p.id}>
                      <td>{p.description}</td>
                      <td>
                        {p.locality_name ? (
                          p.locality_name
                        ) : (
                          <span style={{ color: "var(--muted)" }}>Sem localidade</span>
                        )}
                      </td>
                      <td style={{ maxWidth: 220, overflow: "hidden", textOverflow: "ellipsis" }}>{p.address || "—"}</td>
                      <td className="mono" style={{ fontSize: 11 }}>
                        {p.latitude != null && p.longitude != null
                          ? `${p.latitude.toFixed(5)}, ${p.longitude.toFixed(5)}`
                          : "—"}
                      </td>
                      <td>{p.device_count ?? 0}</td>
                      <td>
                        <ActionMenu
                          items={[
                            {
                              id: "copy",
                              label: "Copiar coordenadas",
                              onClick: () => void copyCoords(p.latitude, p.longitude),
                            },
                            {
                              id: "maps",
                              label: "Copiar link Google Maps",
                              onClick: () => void copyMapsLink(p.latitude, p.longitude),
                            },
                            ...(canMutate
                              ? [
                                  { id: "edit", label: "Editar", onClick: () => openEditPop(p) },
                                  ...(p.locality_id
                                    ? [
                                        {
                                          id: "unlink",
                                          label: "Desvincular localidade",
                                          onClick: () => unlinkPop.mutate(p.id),
                                        },
                                      ]
                                    : []),
                                  { id: "del", label: "Eliminar", danger: true, onClick: () => setDeletePopId(p.id) },
                                ]
                              : []),
                          ]}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      {locModal("create")}
      {locModal("edit")}
      {popModal("create")}
      {popModal("edit")}

      <ConfirmModal
        open={!!deleteLocId}
        title="Eliminar localidade"
        message="Isto remove a localidade e as VLANs associadas. Registos mensais de clientes também serão removidos (CASCADE). POPs ainda vinculados bloqueiam a exclusão — desvincule-os antes."
        confirmLabel="Eliminar"
        danger
        busy={delLoc.isPending}
        onCancel={() => setDeleteLocId(null)}
        onConfirm={() => deleteLocId && delLoc.mutate(deleteLocId)}
      />

      <ConfirmModal
        open={!!deletePopId}
        title="Eliminar POP"
        message="Equipamentos associados a este POP ficarão sem POP (pop_id). Confirma a eliminação?"
        confirmLabel="Eliminar"
        danger
        busy={delPop.isPending}
        onCancel={() => setDeletePopId(null)}
        onConfirm={() => deletePopId && delPop.mutate(deletePopId)}
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
        busy={createLoc.isPending || patchLoc.isPending}
        onCancel={() => setSharedWarn(null)}
        onConfirm={() => {
          if (!sharedWarn) return;
          if (sharedWarn.pending === "create") createLoc.mutate(true);
          else patchLoc.mutate(true);
        }}
      />
    </div>
  );
}
