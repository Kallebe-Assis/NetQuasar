import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Pencil, Plus, RefreshCw, Trash2, X } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { EM_DASH } from "../../lib/formatDisplay";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { ConfirmModal } from "../../components/ConfirmModal";
import { InfoHint } from "../../components/InfoHint";

export type BgpCarrier = {
  id: string;
  name: string;
  document: string;
  address: string;
  bandwidth_limit_mbps?: number | null;
  as_numbers: number[];
};

/** Modal de cadastro/edição de operadora — nome, CNPJ, endereço, 1+ AS e limite de banda
 * (Mbps/Gbps, sempre convertido e gravado em Mbps). Cadastro global (não por equipamento) —
 * usado tanto na secção "Operadoras cadastradas" abaixo como inline ao ligar uma interface. */
function CarrierFormModal({ editing, onClose, onSaved }: { editing: BgpCarrier | null; onClose: () => void; onSaved: (carrier: BgpCarrier) => void }) {
  const { push: pushToast } = useAppToast();
  const [name, setName] = useState(editing?.name ?? "");
  const [document_, setDocument] = useState(editing?.document ?? "");
  const [address, setAddress] = useState(editing?.address ?? "");
  const [asInput, setAsInput] = useState("");
  const [asNumbers, setAsNumbers] = useState<number[]>(editing?.as_numbers ?? []);
  const initialMbps = editing?.bandwidth_limit_mbps ?? null;
  const [unit, setUnit] = useState<"Mbps" | "Gbps">(initialMbps && initialMbps >= 1000 ? "Gbps" : "Mbps");
  const [limitValue, setLimitValue] = useState<string>(() => {
    if (initialMbps == null) return "";
    return unit === "Gbps" ? String(initialMbps / 1000) : String(initialMbps);
  });
  const [saving, setSaving] = useState(false);

  function addAS() {
    const n = Number(asInput.trim());
    if (!Number.isFinite(n) || n <= 0) return;
    if (!asNumbers.includes(n)) setAsNumbers((prev) => [...prev, n]);
    setAsInput("");
  }

  async function save() {
    if (!name.trim()) {
      toastErr(pushToast, new Error("Nome da operadora é obrigatório."));
      return;
    }
    const n = limitValue.trim() === "" ? null : Number(limitValue);
    const mbps = n == null ? null : unit === "Gbps" ? n * 1000 : n;
    setSaving(true);
    try {
      const body = { name: name.trim(), document: document_.trim(), address: address.trim(), bandwidth_limit_mbps: mbps, as_numbers: asNumbers };
      let saved: BgpCarrier;
      if (editing) {
        await apiFetch(`/api/v1/bgp/carriers/${editing.id}`, { method: "PATCH", json: body });
        saved = { id: editing.id, ...body };
      } else {
        const res = await apiFetch<{ id: string }>("/api/v1/bgp/carriers", { method: "POST", json: body });
        saved = { id: res.id, ...body };
      }
      toastOk(pushToast, editing ? "Operadora atualizada." : "Operadora cadastrada.");
      onSaved(saved);
      onClose();
    } catch (e) {
      toastErr(pushToast, e, "Falha ao gravar a operadora.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !saving && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 520 }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>{editing ? "Editar operadora" : "Nova operadora"}</h3>
        <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 12 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            Nome
            <input className="input" style={{ display: "block", width: "100%", marginTop: 4 }} value={name} onChange={(e) => setName(e.target.value)} placeholder="Ex.: K2, FORTE" />
          </label>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            CNPJ
            <input className="input" style={{ display: "block", width: "100%", marginTop: 4 }} value={document_} onChange={(e) => setDocument(e.target.value)} placeholder="00.000.000/0000-00" />
          </label>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            Endereço
            <input className="input" style={{ display: "block", width: "100%", marginTop: 4 }} value={address} onChange={(e) => setAddress(e.target.value)} placeholder="Endereço da operadora" />
          </label>
          <div style={{ fontSize: 12, color: "var(--muted)" }}>
            AS (Autonomous System)
            <div className="row" style={{ gap: 4, marginTop: 4 }}>
              <input
                className="input"
                style={{ flex: 1 }}
                value={asInput}
                onChange={(e) => setAsInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addAS();
                  }
                }}
                placeholder="Ex.: 65000"
                inputMode="numeric"
              />
              <button type="button" className="btn btn--sm" onClick={addAS}>
                Adicionar
              </button>
            </div>
            {asNumbers.length > 0 && (
              <div className="row" style={{ gap: 6, marginTop: 6, flexWrap: "wrap" }}>
                {asNumbers.map((n) => (
                  <span key={n} className="badge" style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    AS{n}
                    <button
                      type="button"
                      onClick={() => setAsNumbers((prev) => prev.filter((x) => x !== n))}
                      style={{ background: "none", border: "none", cursor: "pointer", padding: 0, display: "flex" }}
                      title="Remover"
                    >
                      <X size={11} />
                    </button>
                  </span>
                ))}
              </div>
            )}
          </div>
          <div className="row" style={{ gap: 4, alignItems: "center" }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Limite de banda</span>
            <input
              type="number"
              min={0}
              step="any"
              className="input"
              style={{ width: 100, fontSize: 12 }}
              value={limitValue}
              onChange={(e) => setLimitValue(e.target.value)}
              placeholder="—"
            />
            <select className="input" style={{ fontSize: 12 }} value={unit} onChange={(e) => setUnit(e.target.value as "Mbps" | "Gbps")}>
              <option value="Mbps">Mbps</option>
              <option value="Gbps">Gbps</option>
            </select>
          </div>
        </div>
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 18 }}>
          <button type="button" className="btn" disabled={saving} onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={saving} onClick={() => void save()}>
            {saving ? "A gravar…" : "Guardar"}
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * Operadoras cadastradas — CNPJ, endereço, AS(s) e limite de banda. Fonte única usada pelo
 * seletor de operadora ao ligar uma interface (abaixo) e pelo gráfico de tráfego por operadora
 * na tela BGP (teto do eixo Y).
 */
function CarrierRegistrySection({ carriers, onChanged }: { carriers: BgpCarrier[]; onChanged: () => void }) {
  const { push: pushToast } = useAppToast();
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BgpCarrier | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BgpCarrier | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function removeCarrier() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await apiFetch(`/api/v1/bgp/carriers/${deleteTarget.id}`, { method: "DELETE" });
      toastOk(pushToast, "Operadora removida.");
      setDeleteTarget(null);
      onChanged();
    } catch (e) {
      toastErr(pushToast, e, "Falha ao remover — verifique se ela ainda tem interfaces ligadas.");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="card" style={{ padding: "12px 16px" }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <h2 style={{ margin: "0 0 6px", fontSize: 16, display: "flex", alignItems: "center", gap: 6 }}>
          Operadoras cadastradas
          <InfoHint label="Sobre o cadastro de operadoras">
            <p>
              Cadastro de fornecedores de link (operadora, CNPJ, endereço, AS e limite de banda
              contratado). É a partir daqui que se escolhe a operadora ao ligar uma interface —
              já não é mais um campo de texto livre.
            </p>
          </InfoHint>
        </h2>
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => {
            setEditing(null);
            setFormOpen(true);
          }}
        >
          <Plus size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
          Nova operadora
        </button>
      </div>

      {carriers.length === 0 ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>Nenhuma operadora cadastrada ainda.</p>
      ) : (
        <div className="table-wrap">
          <table style={{ fontSize: 12 }}>
            <thead>
              <tr>
                <th>Operadora</th>
                <th>CNPJ</th>
                <th>Endereço</th>
                <th>AS</th>
                <th>Limite de banda</th>
                <th style={{ width: 90 }} />
              </tr>
            </thead>
            <tbody>
              {carriers.map((c) => (
                <tr key={c.id}>
                  <td>{c.name}</td>
                  <td>{c.document || EM_DASH}</td>
                  <td>{c.address || EM_DASH}</td>
                  <td>{c.as_numbers.length ? c.as_numbers.map((n) => `AS${n}`).join(", ") : EM_DASH}</td>
                  <td>{c.bandwidth_limit_mbps ? (c.bandwidth_limit_mbps >= 1000 ? `${c.bandwidth_limit_mbps / 1000} Gbps` : `${c.bandwidth_limit_mbps} Mbps`) : EM_DASH}</td>
                  <td>
                    <div className="row" style={{ gap: 4, justifyContent: "flex-end" }}>
                      <button type="button" className="btn btn--icon" title="Editar" onClick={() => { setEditing(c); setFormOpen(true); }}>
                        <Pencil size={13} />
                      </button>
                      <button type="button" className="btn btn--icon" title="Remover" onClick={() => setDeleteTarget(c)}>
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formOpen ? (
        <CarrierFormModal
          editing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => onChanged()}
        />
      ) : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Remover operadora"
        message={deleteTarget ? `Remover "${deleteTarget.name}"? Só é possível se não houver interfaces ligadas a ela.` : ""}
        confirmLabel="Remover"
        busy={deleting}
        onCancel={() => !deleting && setDeleteTarget(null)}
        onConfirm={() => void removeCarrier()}
      />
    </div>
  );
}

type BgpDevice = { id: string; description: string; ip: string };

export type BgpUplink = {
  id: string;
  device_id: string;
  carrier_id: string;
  carrier_label: string;
  interface_label: string;
  if_descr: string;
  if_name: string;
  if_index_hint?: number | null;
  is_primary_traffic: boolean;
  sort_order: number;
};

type ReportInterface = {
  if_index: string;
  descr?: string;
  alias?: string;
  oper_status?: string;
};

function UplinkFormModal({
  deviceId,
  editing,
  carriers,
  onClose,
  onSaved,
  onNewCarrier,
}: {
  deviceId: string;
  editing: BgpUplink | null;
  carriers: BgpCarrier[];
  onClose: () => void;
  onSaved: () => void;
  onNewCarrier: () => void;
}) {
  const { push: pushToast } = useAppToast();
  const [carrierId, setCarrierId] = useState(editing?.carrier_id ?? "");
  const [interfaceLabel, setInterfaceLabel] = useState(editing?.interface_label ?? "");
  const [selectedKey, setSelectedKey] = useState<string>(editing ? `${editing.if_descr}|${editing.if_name}` : "");
  const [isPrimary, setIsPrimary] = useState(editing?.is_primary_traffic ?? true);
  const [saving, setSaving] = useState(false);

  const reportQ = useQuery({
    queryKey: ["bgp-uplink-candidates", deviceId],
    queryFn: () => apiFetch<{ report?: { interfaces: ReportInterface[] } }>(`/api/v1/bgp/devices/${deviceId}/report`),
    staleTime: 15_000,
  });
  const candidates = reportQ.data?.report?.interfaces ?? [];

  async function save() {
    if (!carrierId || !interfaceLabel.trim()) {
      toastErr(pushToast, new Error("Escolha a operadora e preencha a descrição da interface."));
      return;
    }
    const picked = candidates.find((c) => `${c.descr ?? ""}|${c.if_index}` === selectedKey);
    if (!editing && !picked) {
      toastErr(pushToast, new Error("Escolha a interface na lista."));
      return;
    }
    setSaving(true);
    try {
      const body = {
        carrier_id: carrierId,
        interface_label: interfaceLabel.trim(),
        if_descr: picked?.descr ?? editing?.if_descr ?? "",
        if_name: picked?.alias ?? editing?.if_name ?? "",
        if_index_hint: picked ? Number(picked.if_index) : (editing?.if_index_hint ?? null),
        is_primary_traffic: isPrimary,
        sort_order: editing?.sort_order ?? 0,
      };
      if (editing) {
        await apiFetch(`/api/v1/bgp/devices/${deviceId}/uplinks/${editing.id}`, { method: "PATCH", json: body });
      } else {
        await apiFetch(`/api/v1/bgp/devices/${deviceId}/uplinks`, { method: "POST", json: body });
      }
      toastOk(pushToast, editing ? "Interface atualizada." : "Interface adicionada.");
      onSaved();
      onClose();
    } catch (e) {
      toastErr(pushToast, e, "Falha ao gravar.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !saving && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 560 }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>{editing ? "Editar interface da operadora" : "Ligar interface a uma operadora"}</h3>

        <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 12 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            Operadora
            <div className="row" style={{ gap: 6, marginTop: 4 }}>
              <select className="input" style={{ flex: 1 }} value={carrierId} onChange={(e) => setCarrierId(e.target.value)}>
                <option value="">Selecione…</option>
                {carriers.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn--sm" onClick={onNewCarrier} title="Cadastrar nova operadora">
                <Plus size={12} style={{ marginRight: 3, verticalAlign: -2 }} />
                Nova
              </button>
            </div>
            {carriers.length === 0 && (
              <p style={{ margin: "4px 0 0", color: "var(--warn)", fontSize: 11 }}>
                Nenhuma operadora cadastrada ainda — clique em "Nova" para cadastrar a primeira.
              </p>
            )}
          </label>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            Descrição desta interface
            <input
              className="input"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={interfaceLabel}
              onChange={(e) => setInterfaceLabel(e.target.value)}
              placeholder="Ex.: Uplink principal, Membro Eth-Trunk11"
            />
          </label>
          <div style={{ fontSize: 12, color: "var(--muted)" }}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
              <span>Interface no equipamento (do último relatório BGP)</span>
              <button type="button" className="btn btn--sm" onClick={() => void reportQ.refetch()} disabled={reportQ.isFetching}>
                <RefreshCw size={12} style={{ marginRight: 4, verticalAlign: -2 }} />
                {reportQ.isFetching ? "A atualizar…" : "Recarregar"}
              </button>
            </div>
            <select
              className="input"
              style={{ display: "block", width: "100%", marginTop: 6 }}
              value={selectedKey}
              onChange={(e) => setSelectedKey(e.target.value)}
              disabled={reportQ.isLoading}
            >
              <option value="">{editing ? `(manter: ${editing.if_descr || editing.if_name})` : "Selecione…"}</option>
              {candidates.map((c) => (
                <option key={`${c.descr ?? ""}|${c.if_index}`} value={`${c.descr ?? ""}|${c.if_index}`}>
                  {c.descr || c.if_index} {c.alias ? `— ${c.alias}` : ""} — {c.oper_status === "1" ? "up" : "down"}
                </option>
              ))}
            </select>
            {reportQ.data && candidates.length === 0 ? (
              <p style={{ margin: "4px 0 0", color: "var(--warn)" }}>
                Nenhuma interface coletada ainda — clique em "Atualizar" na tela BGP primeiro.
              </p>
            ) : null}
          </div>
          <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, cursor: "pointer" }}>
            <input type="checkbox" checked={isPrimary} onChange={(e) => setIsPrimary(e.target.checked)} />
            Esta interface representa o tráfego total desta operadora (não marque para membros
            físicos de um Eth-Trunk já contado no agregado — evita somar tráfego em dobro)
          </label>
        </div>

        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 18 }}>
          <button type="button" className="btn" disabled={saving} onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={saving} onClick={() => void save()}>
            {saving ? "A gravar…" : "Guardar"}
          </button>
        </div>
      </div>
    </div>
  );
}

/** Uma linha de interface dentro do grupo da sua operadora (ver carrierGroups abaixo). */
function UplinkRow({ u, onEdit, onDelete }: { u: BgpUplink; onEdit: () => void; onDelete: () => void }) {
  return (
    <tr>
      <td>
        {u.interface_label}
        <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
          {u.if_descr || u.if_name || EM_DASH}
        </div>
      </td>
      <td>
        {u.is_primary_traffic ? <span className="badge badge--ok">Sim</span> : <span className="badge badge--off">Não (membro)</span>}
      </td>
      <td>
        <div className="row" style={{ gap: 4, justifyContent: "flex-end" }}>
          <button type="button" className="btn btn--icon" title="Editar" onClick={onEdit}>
            <Pencil size={13} />
          </button>
          <button type="button" className="btn btn--icon" title="Remover" onClick={onDelete}>
            <Trash2 size={13} />
          </button>
        </div>
      </td>
    </tr>
  );
}

/**
 * Configurações → BGP → Operadoras: cadastro de operadoras (CNPJ, endereço, AS, limite de
 * banda) e ligação de interfaces de cada equipamento BGP a uma operadora já cadastrada (seletor,
 * não mais texto livre) — usado pela tela BGP para o tráfego total só somar as interfaces das
 * operadoras configuradas e para desenhar 1 gráfico por operadora.
 */
export function BgpUplinkCarriersPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [deviceId, setDeviceId] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [carrierFormOpen, setCarrierFormOpen] = useState(false);
  const [editing, setEditing] = useState<BgpUplink | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BgpUplink | null>(null);
  const [deleting, setDeleting] = useState(false);

  const devicesQ = useQuery({
    queryKey: ["bgp-devices"],
    queryFn: () => apiFetch<{ devices: BgpDevice[] }>("/api/v1/bgp/devices"),
  });
  const rows = devicesQ.data?.devices ?? [];
  const selectedId = deviceId ?? rows[0]?.id ?? null;

  const carriersQ = useQuery({
    queryKey: ["bgp-carriers"],
    queryFn: () => apiFetch<{ carriers: BgpCarrier[] }>("/api/v1/bgp/carriers"),
    staleTime: 20_000,
  });
  const carriers = carriersQ.data?.carriers ?? [];

  const uplinksQ = useQuery({
    queryKey: ["bgp-uplinks", selectedId],
    queryFn: () => apiFetch<{ uplinks: BgpUplink[] }>(`/api/v1/bgp/devices/${selectedId}/uplinks`),
    enabled: !!selectedId,
    staleTime: 20_000,
  });
  const uplinks = uplinksQ.data?.uplinks ?? [];

  // Agrupado por operadora — deixa visualmente claro que uma operadora pode ter várias
  // interfaces (todas somadas quando "conta no total"), não só uma linha "canónica".
  const byCarrier = new Map<string, { label: string; group: BgpUplink[] }>();
  for (const u of uplinks) {
    const entry = byCarrier.get(u.carrier_id) ?? { label: u.carrier_label, group: [] };
    entry.group.push(u);
    byCarrier.set(u.carrier_id, entry);
  }
  const carrierGroups = [...byCarrier.entries()].sort((a, b) => a[1].label.localeCompare(b[1].label));

  async function removeUplink() {
    if (!deleteTarget || !selectedId) return;
    setDeleting(true);
    try {
      await apiFetch(`/api/v1/bgp/devices/${selectedId}/uplinks/${deleteTarget.id}`, { method: "DELETE" });
      toastOk(pushToast, "Interface removida.");
      setDeleteTarget(null);
      void qc.invalidateQueries({ queryKey: ["bgp-uplinks", selectedId] });
    } catch (e) {
      toastErr(pushToast, e, "Falha ao remover.");
    } finally {
      setDeleting(false);
    }
  }

  if (devicesQ.isLoading) return <p>A carregar equipamentos BGP…</p>;
  if (devicesQ.isError) return <div className="msg msg--err">{(devicesQ.error as Error).message}</div>;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, marginTop: 16 }}>
      <CarrierRegistrySection carriers={carriers} onChanged={() => void qc.invalidateQueries({ queryKey: ["bgp-carriers"] })} />

      <div className="card" style={{ padding: "12px 16px" }}>
        <h2 style={{ margin: "0 0 6px", fontSize: 16, display: "flex", alignItems: "center", gap: 6 }}>
          Interfaces por equipamento
          <InfoHint label="Sobre interfaces por equipamento">
            <p>
              Diz ao sistema quais interfaces de cada equipamento correspondem a qual operadora
              já cadastrada acima — agrupadas abaixo por operadora. Uma operadora pode ter mais
              do que uma interface "conta no total" (ex.: 2 uplinks independentes, sem
              Eth-Trunk) — todas são somadas no gráfico de tráfego por operadora da tela BGP.
            </p>
          </InfoHint>
        </h2>
        {rows.length === 0 ? (
          <p style={{ fontSize: 13, color: "var(--muted)" }}>Nenhum equipamento com BGP activo ainda.</p>
        ) : (
          <>
            <div className="row" style={{ gap: 8, marginTop: 10, marginBottom: 12, flexWrap: "wrap", alignItems: "center" }}>
              <label style={{ fontSize: 12, fontWeight: 600 }}>Equipamento</label>
              <select className="input" style={{ minWidth: 240 }} value={selectedId ?? ""} onChange={(e) => setDeviceId(e.target.value)}>
                {rows.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.description} {d.ip ? `(${d.ip})` : ""}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="btn btn--ghost"
                onClick={() => {
                  setEditing(null);
                  setFormOpen(true);
                }}
                disabled={!selectedId}
              >
                <Plus size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                Ligar interface
              </button>
            </div>

            {uplinks.length === 0 ? (
              <p style={{ fontSize: 12, color: "var(--muted)" }}>
                Nenhuma interface configurada para este equipamento ainda — o gráfico soma todas as interfaces por
                omissão. Clique em "Ligar interface" para restringir às operadoras reais.
              </p>
            ) : (
              <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
                {carrierGroups.map(([carrierId, { label, group }]) => (
                  <div key={carrierId} className="card" style={{ padding: "10px 12px" }}>
                    <div className="row" style={{ gap: 10, alignItems: "center", marginBottom: 8 }}>
                      <strong style={{ fontSize: 13 }}>{label}</strong>
                    </div>
                    <div className="table-wrap">
                      <table style={{ fontSize: 12 }}>
                        <thead>
                          <tr>
                            <th>Interface</th>
                            <th>Conta no total</th>
                            <th style={{ width: 90 }} />
                          </tr>
                        </thead>
                        <tbody>
                          {group.map((u) => (
                            <UplinkRow key={u.id} u={u} onEdit={() => { setEditing(u); setFormOpen(true); }} onDelete={() => setDeleteTarget(u)} />
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {formOpen && selectedId ? (
        <UplinkFormModal
          deviceId={selectedId}
          editing={editing}
          carriers={carriers}
          onClose={() => setFormOpen(false)}
          onSaved={() => void qc.invalidateQueries({ queryKey: ["bgp-uplinks", selectedId] })}
          onNewCarrier={() => setCarrierFormOpen(true)}
        />
      ) : null}

      {carrierFormOpen ? (
        <CarrierFormModal
          editing={null}
          onClose={() => setCarrierFormOpen(false)}
          onSaved={() => void qc.invalidateQueries({ queryKey: ["bgp-carriers"] })}
        />
      ) : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Remover interface"
        message={deleteTarget ? `Remover "${deleteTarget.carrier_label} — ${deleteTarget.interface_label}"?` : ""}
        confirmLabel="Remover"
        busy={deleting}
        onCancel={() => !deleting && setDeleteTarget(null)}
        onConfirm={() => void removeUplink()}
      />
    </div>
  );
}
