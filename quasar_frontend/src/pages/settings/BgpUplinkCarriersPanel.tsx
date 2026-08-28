import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Pencil, Plus, RefreshCw, Save, Trash2 } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { EM_DASH } from "../../lib/formatDisplay";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { ConfirmModal } from "../../components/ConfirmModal";
import { InfoHint } from "../../components/InfoHint";

type CarrierLimit = { carrier_label: string; bandwidth_limit_mbps?: number | null };

/** Editor do limite de banda de UMA operadora — input numérico + unidade (Mbps/Gbps),
 * convertido e gravado sempre em Mbps via PUT .../carrier-limits/{label}. Usado no cabeçalho de
 * cada grupo de operadora abaixo; o limite define o teto do eixo Y do gráfico de tráfego por
 * operadora na tela BGP. */
function CarrierLimitEditor({ deviceId, carrierLabel, currentMbps }: { deviceId: string; carrierLabel: string; currentMbps?: number | null }) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [unit, setUnit] = useState<"Mbps" | "Gbps">(currentMbps && currentMbps >= 1000 ? "Gbps" : "Mbps");
  const [value, setValue] = useState<string>(() => {
    if (currentMbps == null) return "";
    return unit === "Gbps" ? String(currentMbps / 1000) : String(currentMbps);
  });

  useEffect(() => {
    if (currentMbps == null) {
      setValue("");
      return;
    }
    setValue(unit === "Gbps" ? String(currentMbps / 1000) : String(currentMbps));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentMbps]);

  const saveMut = useMutation({
    mutationFn: () => {
      const n = value.trim() === "" ? null : Number(value);
      const mbps = n == null ? null : unit === "Gbps" ? n * 1000 : n;
      return apiFetch(`/api/v1/bgp/devices/${deviceId}/carrier-limits/${encodeURIComponent(carrierLabel)}`, {
        method: "PUT",
        json: { bandwidth_limit_mbps: mbps },
      });
    },
    onSuccess: () => {
      toastOk(pushToast, "Limite de banda gravado.");
      void qc.invalidateQueries({ queryKey: ["bgp-carrier-limits", deviceId] });
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao gravar o limite."),
  });

  return (
    <div className="row" style={{ gap: 4, alignItems: "center" }}>
      <span style={{ fontSize: 11, color: "var(--muted)" }}>Limite de banda</span>
      <input
        type="number"
        min={0}
        step="any"
        className="input"
        style={{ width: 90, fontSize: 12, padding: "3px 6px" }}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder="—"
      />
      <select className="input" style={{ fontSize: 12, padding: "3px 6px" }} value={unit} onChange={(e) => setUnit(e.target.value as "Mbps" | "Gbps")}>
        <option value="Mbps">Mbps</option>
        <option value="Gbps">Gbps</option>
      </select>
      <button
        type="button"
        className="btn btn--icon"
        title="Gravar limite"
        disabled={saveMut.isPending}
        onClick={() => saveMut.mutate()}
      >
        <Save size={13} />
      </button>
    </div>
  );
}

type BgpDevice = { id: string; description: string; ip: string };

export type BgpUplink = {
  id: string;
  device_id: string;
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
  onClose,
  onSaved,
}: {
  deviceId: string;
  editing: BgpUplink | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { push: pushToast } = useAppToast();
  const [carrierLabel, setCarrierLabel] = useState(editing?.carrier_label ?? "");
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
    if (!carrierLabel.trim() || !interfaceLabel.trim()) {
      toastErr(pushToast, new Error("Preencha operadora e descrição da interface."));
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
        carrier_label: carrierLabel.trim(),
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
      toastOk(pushToast, editing ? "Operadora atualizada." : "Operadora adicionada.");
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
        <h3 style={{ marginTop: 0 }}>{editing ? "Editar interface da operadora" : "Adicionar interface de operadora"}</h3>

        <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 12 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            Operadora
            <input
              className="input"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={carrierLabel}
              onChange={(e) => setCarrierLabel(e.target.value)}
              placeholder="Ex.: K2, FORTE"
            />
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
 * Configurações → BGP → Operadoras: define quais interfaces de cada equipamento BGP
 * pertencem a qual operadora (fornecedor de link) — usado pela tela BGP para o gráfico de
 * "Tráfego total BGP" só somar as interfaces das operadoras configuradas (não todas as
 * interfaces walked por SNMP, que também incluem uplinks internos/gerência).
 */
export function BgpUplinkCarriersPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [deviceId, setDeviceId] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BgpUplink | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BgpUplink | null>(null);
  const [deleting, setDeleting] = useState(false);

  const devicesQ = useQuery({
    queryKey: ["bgp-devices"],
    queryFn: () => apiFetch<{ devices: BgpDevice[] }>("/api/v1/bgp/devices"),
  });
  const rows = devicesQ.data?.devices ?? [];
  const selectedId = deviceId ?? rows[0]?.id ?? null;

  const uplinksQ = useQuery({
    queryKey: ["bgp-uplinks", selectedId],
    queryFn: () => apiFetch<{ uplinks: BgpUplink[] }>(`/api/v1/bgp/devices/${selectedId}/uplinks`),
    enabled: !!selectedId,
    staleTime: 20_000,
  });
  const uplinks = uplinksQ.data?.uplinks ?? [];

  const limitsQ = useQuery({
    queryKey: ["bgp-carrier-limits", selectedId],
    queryFn: () => apiFetch<{ limits: CarrierLimit[] }>(`/api/v1/bgp/devices/${selectedId}/carrier-limits`),
    enabled: !!selectedId,
    staleTime: 20_000,
  });
  const limitByCarrier = new Map((limitsQ.data?.limits ?? []).map((l) => [l.carrier_label, l.bandwidth_limit_mbps]));

  // Agrupado por operadora — deixa visualmente claro que uma operadora pode ter várias
  // interfaces (todas somadas quando "conta no total"), não só uma linha "canónica".
  const byCarrier = new Map<string, BgpUplink[]>();
  for (const u of uplinks) {
    const list = byCarrier.get(u.carrier_label) ?? [];
    list.push(u);
    byCarrier.set(u.carrier_label, list);
  }
  const carrierGroups = [...byCarrier.entries()].sort((a, b) => a[0].localeCompare(b[0]));

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
    <div className="card" style={{ padding: "12px 16px", marginTop: 16 }}>
      <h2 style={{ margin: "0 0 6px", fontSize: 16, display: "flex", alignItems: "center", gap: 6 }}>
        Operadoras (interfaces)
        <InfoHint label="Sobre operadoras e interfaces">
          <p>
            Diz ao sistema quais interfaces de cada equipamento correspondem a qual operadora
            (fornecedor de link de internet) — agrupadas abaixo por operadora. Uma operadora pode
            ter mais do que uma interface "conta no total" (ex.: 2 uplinks independentes, sem
            Eth-Trunk) — todas são somadas no gráfico de tráfego por operadora da tela BGP. O
            "Limite de banda" de cada operadora define o teto do eixo Y do gráfico dela.
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
              Adicionar interface
            </button>
          </div>

          {uplinks.length === 0 ? (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Nenhuma interface configurada para este equipamento ainda — o gráfico soma todas as interfaces por
              omissão. Clique em "Adicionar interface" para restringir às operadoras reais.
            </p>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
              {carrierGroups.map(([carrierLabel, group]) => (
                <div key={carrierLabel} className="card" style={{ padding: "10px 12px" }}>
                  <div className="row" style={{ gap: 10, alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", marginBottom: 8 }}>
                    <strong style={{ fontSize: 13 }}>{carrierLabel}</strong>
                    {selectedId && <CarrierLimitEditor deviceId={selectedId} carrierLabel={carrierLabel} currentMbps={limitByCarrier.get(carrierLabel)} />}
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

      {formOpen && selectedId ? (
        <UplinkFormModal
          deviceId={selectedId}
          editing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => void qc.invalidateQueries({ queryKey: ["bgp-uplinks", selectedId] })}
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
