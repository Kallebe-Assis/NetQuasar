import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { apiFetch } from "../lib/api";
import { errorMessageFromUnknown } from "../lib/apiErrors";
import { useAppToast } from "../lib/appToast";
import { copyTextToClipboard } from "../lib/clipboard";
import { isAdminUser } from "../lib/auth";
import { toastErr, toastInfo, toastOk } from "../lib/operationToast";
import { pageCachedQueryOptions, PAGE_DATA_GC_MS, PAGE_DATA_STALE_MS, wrapPageCachedQueryFn } from "../lib/pageDataCache";
import { queryKeys } from "../lib/queryKeys";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { EquipmentMap } from "../components/EquipmentMap";
import { PageCountPill } from "../components/PageCountPill";
import { PopLocationPicker } from "../components/PopLocationPicker";

type Pop = {
  id: string;
  description: string;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  device_count: number;
};

type DeviceRow = {
  id: string;
  description: string;
  category: string;
  ip?: string | null;
  pop_id?: string | null;
};
type PopContact = {
  id: string;
  pop_id: string;
  name: string;
  contact: string;
  shift_label?: string | null;
  is_primary?: boolean;
  notes?: string | null;
};

function fmtCoord(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(n)) return "—";
  return n.toFixed(5);
}

function coordsCell(p: Pop): string {
  if (p.latitude == null && p.longitude == null) return "—";
  return `${fmtCoord(p.latitude)}, ${fmtCoord(p.longitude)}`;
}

function parseCoordInput(v: string): number | null {
  const s = String(v ?? "").trim().replace(",", ".");
  if (!s) return null;
  const n = Number(s);
  return Number.isFinite(n) ? n : null;
}

function validateCoords(lat: number | null, lon: number | null): string | null {
  if (lat == null && lon == null) return null;
  if (lat == null || lon == null) return "Preencha latitude e longitude juntas.";
  if (!Number.isFinite(lat) || !Number.isFinite(lon)) return "Coordenadas inválidas.";
  if (lat < -90 || lat > 90) return "Latitude deve estar entre -90 e 90.";
  if (lon < -180 || lon > 180) return "Longitude deve estar entre -180 e 180.";
  return null;
}

export function PopsPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const list = useQuery({
    queryKey: queryKeys.pops,
    queryFn: wrapPageCachedQueryFn(queryKeys.pops, () => apiFetch<{ pops: Pop[] }>("/api/v1/pops")),
    ...pageCachedQueryOptions<{ pops: Pop[] }>(queryKeys.pops, PAGE_DATA_STALE_MS, PAGE_DATA_GC_MS),
  });
  const devices = useQuery({
    queryKey: queryKeys.devices,
    queryFn: wrapPageCachedQueryFn(queryKeys.devices, () => apiFetch<{ devices: DeviceRow[] }>("/api/v1/devices")),
    ...pageCachedQueryOptions<{ devices: DeviceRow[] }>(queryKeys.devices, PAGE_DATA_STALE_MS, PAGE_DATA_GC_MS),
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [desc, setDesc] = useState("");
  const [addr, setAddr] = useState("");
  const [lat, setLat] = useState("");
  const [lon, setLon] = useState("");

  const parseLatLon = (la: string, lo: string): { latitude: number | null; longitude: number | null } => ({
    latitude: parseCoordInput(la),
    longitude: parseCoordInput(lo),
  });

  const create = useMutation({
    mutationFn: () => {
      const { latitude, longitude } = parseLatLon(lat, lon);
      return apiFetch<{ id: string }>("/api/v1/pops", {
        method: "POST",
        json: {
          description: desc,
          address: addr.trim() || null,
          latitude: latitude != null && Number.isFinite(latitude) ? latitude : null,
          longitude: longitude != null && Number.isFinite(longitude) ? longitude : null,
        },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pops });
      setDesc("");
      setAddr("");
      setLat("");
      setLon("");
      setCreateOpen(false);
      toastOk(pushToast, "POP criado com sucesso.");
    },
    onError: (err: Error) => toastErr(pushToast, err, "Falha ao criar POP."),
  });

  const [edit, setEdit] = useState<Pop | null>(null);
  const [eDesc, setEDesc] = useState("");
  const [eAddr, setEAddr] = useState("");
  const [eLat, setELat] = useState("");
  const [eLon, setELon] = useState("");

  function openEdit(p: Pop) {
    setEdit(p);
    setEDesc(p.description);
    setEAddr(p.address ?? "");
    setELat(p.latitude != null ? String(p.latitude) : "");
    setELon(p.longitude != null ? String(p.longitude) : "");
  }

  const patch = useMutation({
    mutationFn: () => {
      const latitude = parseCoordInput(eLat);
      const longitude = parseCoordInput(eLon);
      return apiFetch<Pop>(`/api/v1/pops/${edit!.id}`, {
        method: "PATCH",
        json: {
          description: eDesc.trim(),
          address: eAddr.trim() || null,
          latitude: latitude != null && Number.isFinite(latitude) ? latitude : null,
          longitude: longitude != null && Number.isFinite(longitude) ? longitude : null,
        },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pops });
      setEdit(null);
      toastOk(pushToast, "POP actualizado com sucesso.");
    },
    onError: (err: Error) => toastErr(pushToast, err, "Falha ao guardar POP."),
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/pops/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pops });
      toastOk(pushToast, "POP excluído.");
    },
    onError: (err: Error) => toastErr(pushToast, err, "Falha ao excluir POP."),
  });

  const [assignPop, setAssignPop] = useState<Pop | null>(null);
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(new Set());
  const [contactsPop, setContactsPop] = useState<Pop | null>(null);
  const [viewPop, setViewPop] = useState<Pop | null>(null);
  const [pendingDeletePop, setPendingDeletePop] = useState<Pop | null>(null);
  const [pendingDeleteContactId, setPendingDeleteContactId] = useState<string | null>(null);
  const [contactName, setContactName] = useState("");
  const [contactInfo, setContactInfo] = useState("");
  const [contactShift, setContactShift] = useState("");
  const [contactNotes, setContactNotes] = useState("");
  const contactsKey = queryKeys.popContacts(contactsPop?.id ?? "");
  const contacts = useQuery({
    queryKey: contactsKey,
    enabled: !!contactsPop,
    queryFn: wrapPageCachedQueryFn(contactsKey, () =>
      apiFetch<{ items: PopContact[] }>(`/api/v1/pops/${contactsPop!.id}/contacts`),
    ),
    ...pageCachedQueryOptions<{ items: PopContact[] }>(contactsKey, PAGE_DATA_STALE_MS, PAGE_DATA_GC_MS),
  });
  const addContact = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/pops/${contactsPop!.id}/contacts`, {
        method: "POST",
        json: { name: contactName, contact: contactInfo, shift_label: contactShift || null, notes: contactNotes || null },
      }),
    onSuccess: () => {
      contacts.refetch();
      setContactName("");
      setContactInfo("");
      setContactShift("");
      setContactNotes("");
      toastOk(pushToast, "Responsável adicionado.");
    },
    onError: (err: Error) => toastErr(pushToast, err, "Falha ao guardar responsável."),
  });
  const delContact = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/pops/contacts/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      contacts.refetch();
      toastOk(pushToast, "Responsável removido.");
    },
    onError: (err: Error) => toastErr(pushToast, err, "Falha ao remover responsável."),
  });

  const devicesWithoutPop = useMemo(() => {
    return (devices.data?.devices ?? []).filter((d) => d.pop_id == null || String(d.pop_id).trim() === "");
  }, [devices.data?.devices]);

  const assignDevices = useMutation({
    mutationFn: async ({ popId, ids }: { popId: string; ids: string[] }) => {
      for (const id of ids) {
        await apiFetch(`/api/v1/devices/${id}`, { method: "PATCH", json: { pop_id: popId } });
      }
    },
    onSuccess: (_, { ids }) => {
      qc.invalidateQueries({ queryKey: queryKeys.pops });
      qc.invalidateQueries({ queryKey: queryKeys.devices });
      setAssignPop(null);
      setSelectedDeviceIds(new Set());
      toastOk(pushToast, `${ids.length} equipamento(s) associado(s) ao POP.`);
    },
    onError: (err: Error) => toastErr(pushToast, err, "Falha ao associar equipamentos."),
  });

  function toggleDevice(id: string) {
    setSelectedDeviceIds((prev) => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }

  if (list.isLoading) return <p>Carregando POPs…</p>;
  if (list.isError) return <div className="msg msg--err">{errorMessageFromUnknown(list.error)}</div>;

  const createLatNum = parseCoordInput(lat);
  const createLonNum = parseCoordInput(lon);
  const editLatNum = parseCoordInput(eLat);
  const editLonNum = parseCoordInput(eLon);
  const createCoordErr = validateCoords(createLatNum, createLonNum);
  const editCoordErr = validateCoords(editLatNum, editLonNum);

  const copyCoords = async (p: Pop) => {
    if (p.latitude == null || p.longitude == null) {
      toastInfo(pushToast, "Este POP ainda não possui coordenadas.");
      return;
    }
    const txt = `${p.latitude}, ${p.longitude}`;
    const ok = await copyTextToClipboard(txt);
    if (ok) {
      toastOk(pushToast, "Coordenadas copiadas.");
    } else {
      toastErr(pushToast, new Error(`Não foi possível copiar automaticamente. Copie manualmente: ${txt}`));
    }
  };

  return (
    <>
      <div className="page-heading">
        <h1>POPs</h1>
        <PageCountPill label="POPs" count={(list.data?.pops ?? []).length} />
      </div>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>
        Lista de pontos de presença. Cada equipamento pode estar associado a no máximo um POP.
      </p>

      {canMutate ? (
        <div className="row" style={{ marginBottom: 12 }}>
          <button type="button" className="btn btn--primary" onClick={() => setCreateOpen(true)}>
            Adicionar novo POP
          </button>
        </div>
      ) : (
        <p style={{ color: "var(--muted)", fontSize: 12, marginBottom: 12 }}>Modo só leitura: consulte POPs e mapas; alterações ficam reservadas a administradores.</p>
      )}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Descrição</th>
              <th>Endereço</th>
              <th>Coordenadas</th>
              <th>Equip.</th>
              <th style={{ minWidth: 140 }}>Acções</th>
            </tr>
          </thead>
          <tbody>
            {(list.data?.pops ?? []).map((p) => (
              <tr key={p.id}>
                <td>{p.description}</td>
                <td>{p.address ?? "—"}</td>
                <td className="mono" style={{ fontSize: 11 }} title={coordsCell(p)}>
                  {coordsCell(p)}
                </td>
                <td className="mono">{p.device_count}</td>
                <td>
                  <ActionMenu
                    items={[
                      ...(canMutate ? [{ id: "edit", label: "Editar", onClick: () => openEdit(p) }] : []),
                      { id: "view", label: "Visualizar local", onClick: () => setViewPop(p) },
                      { id: "copy", label: "Copiar coordenadas", onClick: () => void copyCoords(p) },
                      ...(canMutate
                        ? [
                            {
                              id: "add-dev",
                              label: "Adicionar equipamento",
                              onClick: () => {
                                setAssignPop(p);
                                setSelectedDeviceIds(new Set());
                              },
                            },
                          ]
                        : []),
                      { id: "contacts", label: "Responsáveis", onClick: () => setContactsPop(p) },
                      ...(canMutate ? [{ id: "del", label: "Excluir", danger: true, disabled: del.isPending, onClick: () => setPendingDeletePop(p) }] : []),
                    ]}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {viewPop && (
        <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && setViewPop(null)}>
          <div className="modal modal--wide" style={{ maxWidth: 1100 }} onClick={(e) => e.stopPropagation()}>
            <h3>Local do POP: {viewPop.description}</h3>
            <p className="mono" style={{ color: "var(--muted)", fontSize: 12 }}>
              {coordsCell(viewPop)}
            </p>
            {viewPop.latitude != null && viewPop.longitude != null ? (
              <EquipmentMap
                points={[
                  {
                    id: viewPop.id,
                    description: viewPop.description,
                    lat: viewPop.latitude,
                    lng: viewPop.longitude,
                    category: "pop",
                    status: "online",
                  },
                ]}
                displayMode="cluster"
                flyTo={null}
                flyKey={0}
                fitBoundsVersion={0}
              />
            ) : (
              <p style={{ color: "var(--muted)" }}>POP sem coordenadas.</p>
            )}
            <div className="row" style={{ marginTop: 10 }}>
              <button type="button" className="btn" onClick={() => void copyCoords(viewPop)}>
                Copiar coordenadas
              </button>
              <button type="button" className="btn" onClick={() => setViewPop(null)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      )}

      {createOpen && canMutate && (
        <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && setCreateOpen(false)}>
          <div className="modal modal--wide" onClick={(e) => e.stopPropagation()}>
            <h3>Novo POP</h3>
            {create.isError && <div className="msg msg--err">{errorMessageFromUnknown(create.error)}</div>}
            <div className="field">
              <label>Descrição *</label>
              <input className="input" style={{ width: "100%" }} value={desc} onChange={(e) => setDesc(e.target.value)} />
            </div>
            <div className="field">
              <label>Endereço</label>
              <input className="input" style={{ width: "100%" }} value={addr} onChange={(e) => setAddr(e.target.value)} />
            </div>
            <div className="row">
              <div className="field" style={{ flex: 1 }}>
                <label>Latitude</label>
                <input className="input mono" style={{ width: "100%" }} value={lat} onChange={(e) => setLat(e.target.value)} />
              </div>
              <div className="field" style={{ flex: 1 }}>
                <label>Longitude</label>
                <input className="input mono" style={{ width: "100%" }} value={lon} onChange={(e) => setLon(e.target.value)} />
              </div>
            </div>
            {createCoordErr && <div className="msg msg--err">{createCoordErr}</div>}
            <PopLocationPicker
              latitude={createLatNum != null && Number.isFinite(createLatNum) ? createLatNum : null}
              longitude={createLonNum != null && Number.isFinite(createLonNum) ? createLonNum : null}
              onChange={(la, lo) => {
                setLat(la.toFixed(6));
                setLon(lo.toFixed(6));
              }}
            />
            <div className="row" style={{ marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setCreateOpen(false)}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" disabled={!desc.trim() || !!createCoordErr || create.isPending} onClick={() => create.mutate()}>
                Criar POP
              </button>
            </div>
          </div>
        </div>
      )}

      {contactsPop && (
        <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && setContactsPop(null)}>
          <div className="modal modal--wide" onClick={(e) => e.stopPropagation()}>
            <h3>Responsáveis do POP: {contactsPop.description}</h3>
            {canMutate ? (
              <>
                <div className="row" style={{ marginBottom: 8 }}>
                  <input className="input" placeholder="Nome" value={contactName} onChange={(e) => setContactName(e.target.value)} />
                  <input className="input" placeholder="Contato (fone/email)" value={contactInfo} onChange={(e) => setContactInfo(e.target.value)} />
                  <input className="input" placeholder="Plantão/turno" value={contactShift} onChange={(e) => setContactShift(e.target.value)} />
                </div>
                <textarea className="input" style={{ width: "100%", minHeight: 56 }} placeholder="Notas" value={contactNotes} onChange={(e) => setContactNotes(e.target.value)} />
                <div className="row" style={{ marginTop: 8 }}>
                  <button type="button" className="btn btn--primary" disabled={!contactName.trim() || !contactInfo.trim() || addContact.isPending} onClick={() => addContact.mutate()}>
                    Salvar responsável
                  </button>
                  <button type="button" className="btn" onClick={() => setContactsPop(null)}>
                    Fechar
                  </button>
                </div>
              </>
            ) : (
              <div className="row" style={{ marginTop: 8 }}>
                <button type="button" className="btn" onClick={() => setContactsPop(null)}>
                  Fechar
                </button>
              </div>
            )}
            <div className="table-wrap" style={{ marginTop: 10 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Nome</th>
                    <th>Contato</th>
                    <th>Plantão</th>
                    <th>Notas</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {(contacts.data?.items ?? []).map((c) => (
                    <tr key={c.id}>
                      <td>{c.name}</td>
                      <td>{c.contact}</td>
                      <td>{c.shift_label ?? "—"}</td>
                      <td>{c.notes ?? "—"}</td>
                      <td>
                        {canMutate ? (
                          <button type="button" className="btn" onClick={() => setPendingDeleteContactId(c.id)}>
                            Excluir
                          </button>
                        ) : (
                          "—"
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {edit && canMutate && (
        <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && setEdit(null)}>
          <div className="modal modal--wide" onClick={(e) => e.stopPropagation()}>
            <h3>Editar POP</h3>
            {patch.isError && <div className="msg msg--err">{errorMessageFromUnknown(patch.error)}</div>}
            <div className="field">
              <label>Descrição *</label>
              <input className="input" style={{ width: "100%" }} value={eDesc} onChange={(e) => setEDesc(e.target.value)} />
            </div>
            <div className="field">
              <label>Endereço</label>
              <input className="input" style={{ width: "100%" }} value={eAddr} onChange={(e) => setEAddr(e.target.value)} />
            </div>
            <div className="row">
              <div className="field" style={{ flex: 1 }}>
                <label>Latitude</label>
                <input className="input mono" style={{ width: "100%" }} value={eLat} onChange={(e) => setELat(e.target.value)} />
              </div>
              <div className="field" style={{ flex: 1 }}>
                <label>Longitude</label>
                <input className="input mono" style={{ width: "100%" }} value={eLon} onChange={(e) => setELon(e.target.value)} />
              </div>
            </div>
            {editCoordErr && <div className="msg msg--err">{editCoordErr}</div>}
            <PopLocationPicker
              latitude={editLatNum != null && Number.isFinite(editLatNum) ? editLatNum : null}
              longitude={editLonNum != null && Number.isFinite(editLonNum) ? editLonNum : null}
              onChange={(la, lo) => {
                setELat(la.toFixed(6));
                setELon(lo.toFixed(6));
              }}
            />
            <div className="row" style={{ marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setEdit(null)}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" disabled={!eDesc.trim() || !!editCoordErr || patch.isPending} onClick={() => patch.mutate()}>
                Salvar
              </button>
            </div>
          </div>
        </div>
      )}

      {assignPop && canMutate && (
        <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && setAssignPop(null)}>
          <div className="modal modal--wide" onClick={(e) => e.stopPropagation()}>
            <h3>Associar equipamentos ao POP</h3>
            <p style={{ color: "var(--muted)", fontSize: 12 }}>
              <strong>{assignPop.description}</strong> — só aparecem equipamentos <strong>sem POP</strong> (cada equipamento pode ter no máximo um POP).
            </p>
            {devices.isLoading && <p>A carregar equipamentos…</p>}
            {devices.isError && <div className="msg msg--err">{errorMessageFromUnknown(devices.error)}</div>}
            {devices.data && (
              <>
                {devicesWithoutPop.length === 0 ? (
                  <p style={{ color: "var(--muted)" }}>Não há equipamentos sem POP para associar.</p>
                ) : (
                  <div className="table-wrap" style={{ maxHeight: 360, overflow: "auto" }}>
                    <table>
                      <thead>
                        <tr>
                          <th style={{ width: 40 }} />
                          <th>Descrição</th>
                          <th>Categoria</th>
                          <th>IP</th>
                        </tr>
                      </thead>
                      <tbody>
                        {devicesWithoutPop.map((d) => (
                          <tr key={d.id}>
                            <td>
                              <input
                                type="checkbox"
                                checked={selectedDeviceIds.has(d.id)}
                                onChange={() => toggleDevice(d.id)}
                                aria-label={`Seleccionar ${d.description}`}
                              />
                            </td>
                            <td>{d.description}</td>
                            <td>{d.category}</td>
                            <td className="mono">{d.ip ?? "—"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                {assignDevices.isError && <div className="msg msg--err">{errorMessageFromUnknown(assignDevices.error)}</div>}
                <div className="row" style={{ marginTop: 12 }}>
                  <button type="button" className="btn" onClick={() => setAssignPop(null)}>
                    Cancelar
                  </button>
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={selectedDeviceIds.size === 0 || assignDevices.isPending || devicesWithoutPop.length === 0}
                    onClick={() => assignDevices.mutate({ popId: assignPop.id, ids: [...selectedDeviceIds] })}
                  >
                    Associar equipamentos seleccionados ({selectedDeviceIds.size})
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
      <ConfirmModal
        open={!!pendingDeletePop}
        title="Excluir POP"
        message={`Confirma excluir o POP «${pendingDeletePop?.description ?? ""}»?`}
        confirmLabel="Excluir"
        danger
        busy={del.isPending}
        onCancel={() => setPendingDeletePop(null)}
        onConfirm={() => {
          if (!pendingDeletePop) return;
          del.mutate(pendingDeletePop.id, { onSuccess: () => setPendingDeletePop(null) });
        }}
      />
      <ConfirmModal
        open={!!pendingDeleteContactId}
        title="Excluir responsável"
        message="Confirma remover este responsável do POP?"
        confirmLabel="Excluir"
        danger
        busy={delContact.isPending}
        onCancel={() => setPendingDeleteContactId(null)}
        onConfirm={() => {
          if (!pendingDeleteContactId) return;
          delContact.mutate(pendingDeleteContactId, { onSuccess: () => setPendingDeleteContactId(null) });
        }}
      />
    </>
  );
}
