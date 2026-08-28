import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Plus, Trash2, Pencil, RefreshCw } from "lucide-react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { apiFetch } from "../lib/api";
import { formatBitrate } from "../lib/formatBitrate";
import { EM_DASH } from "../lib/formatDisplay";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { ConfirmModal } from "./ConfirmModal";
import { InfoHint } from "./InfoHint";

type BngUplink = {
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

type UplinkPoint = { t: string; in_bps: number; out_bps: number };
type UplinkResult = BngUplink & { points: UplinkPoint[]; current?: UplinkPoint | null };
type UplinksHistoryResponse = {
  device_id: string;
  since: string;
  until: string;
  bucket: "minute" | "hour" | "day";
  uplinks: UplinkResult[];
  carrier_totals: Record<string, { in_bps: number; out_bps: number }>;
};

type IfCandidate = {
  if_index: number;
  descr: string;
  if_name: string;
  if_alias: string;
  display_name: string;
  oper_status: string;
  in_bps?: number;
  out_bps?: number;
};

const DAY_OPTIONS = [1, 3, 7, 30] as const;
const CARRIER_COLORS = ["#58a6ff", "#3fb950", "#d29922", "#a371f7", "#f85149", "#79c0ff"];

function fmtAxisTime(iso: string, bucket: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return iso;
  if (bucket === "day") return d.toLocaleDateString("pt-PT", { day: "2-digit", month: "short" });
  return d.toLocaleString("pt-PT", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function UplinkFormModal({
  deviceId,
  editing,
  onClose,
  onSaved,
}: {
  deviceId: string;
  editing: BngUplink | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { push: pushToast } = useAppToast();
  const [carrierLabel, setCarrierLabel] = useState(editing?.carrier_label ?? "");
  const [interfaceLabel, setInterfaceLabel] = useState(editing?.interface_label ?? "");
  const [selectedKey, setSelectedKey] = useState<string>(
    editing ? `${editing.if_descr}|${editing.if_name}|${editing.if_index_hint ?? ""}` : "",
  );
  const [isPrimary, setIsPrimary] = useState(editing?.is_primary_traffic ?? true);
  const [saving, setSaving] = useState(false);

  const candidatesQ = useQuery({
    queryKey: ["bng-uplink-candidates", deviceId],
    queryFn: () =>
      apiFetch<{ interface_table: IfCandidate[] }>(`/api/v1/interfaces/devices/${deviceId}`),
    staleTime: 15_000,
  });
  const candidates = candidatesQ.data?.interface_table ?? [];

  async function refreshCandidates() {
    try {
      await apiFetch(`/api/v1/interfaces/devices/${deviceId}/refresh`, { method: "POST" });
      await candidatesQ.refetch();
      toastOk(pushToast, "Lista de interfaces atualizada.");
    } catch (e) {
      toastErr(pushToast, e, "Falha ao atualizar interfaces.");
    }
  }

  async function save() {
    if (!carrierLabel.trim() || !interfaceLabel.trim()) {
      toastErr(pushToast, new Error("Preencha operadora e descrição da interface."));
      return;
    }
    const picked = candidates.find((c) => `${c.descr}|${c.if_name}|${c.if_index}` === selectedKey);
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
        if_name: picked?.if_name ?? editing?.if_name ?? "",
        if_index_hint: picked?.if_index ?? editing?.if_index_hint ?? null,
        is_primary_traffic: isPrimary,
        sort_order: editing?.sort_order ?? 0,
      };
      if (editing) {
        await apiFetch(`/api/v1/bng/devices/${deviceId}/uplinks/${editing.id}`, { method: "PATCH", json: body });
      } else {
        await apiFetch(`/api/v1/bng/devices/${deviceId}/uplinks`, { method: "POST", json: body });
      }
      toastOk(pushToast, editing ? "Uplink atualizado." : "Uplink adicionado.");
      onSaved();
      onClose();
    } catch (e) {
      toastErr(pushToast, e, "Falha ao gravar uplink.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !saving && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 560 }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>{editing ? "Editar uplink" : "Adicionar uplink de operadora"}</h3>

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
              placeholder="Ex.: Uplink principal, Membro Eth-Trunk10"
            />
          </label>
          <div style={{ fontSize: 12, color: "var(--muted)" }}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
              <span>Interface no equipamento</span>
              <button type="button" className="btn btn--sm" onClick={() => void refreshCandidates()} disabled={candidatesQ.isFetching}>
                <RefreshCw size={12} style={{ marginRight: 4, verticalAlign: -2 }} />
                {candidatesQ.isFetching ? "A atualizar…" : "Atualizar lista"}
              </button>
            </div>
            <select
              className="input"
              style={{ display: "block", width: "100%", marginTop: 6 }}
              value={selectedKey}
              onChange={(e) => setSelectedKey(e.target.value)}
              disabled={candidatesQ.isLoading}
            >
              <option value="">
                {editing ? `(manter: ${editing.if_descr || editing.if_name})` : "Selecione…"}
              </option>
              {candidates.map((c) => (
                <option key={`${c.descr}|${c.if_name}|${c.if_index}`} value={`${c.descr}|${c.if_name}|${c.if_index}`}>
                  {c.display_name || c.descr || c.if_name} — {c.oper_status}
                  {c.in_bps != null ? ` — ${formatBitrate(c.in_bps)} in / ${formatBitrate(c.out_bps)} out` : ""}
                </option>
              ))}
            </select>
            {candidatesQ.data && candidates.length === 0 ? (
              <p style={{ margin: "4px 0 0", color: "var(--warn)" }}>
                Nenhuma interface coletada ainda — clique em "Atualizar lista".
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

export function BngUplinksPanel({ deviceId }: { deviceId: string }) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [days, setDays] = useState<(typeof DAY_OPTIONS)[number]>(1);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BngUplink | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BngUplink | null>(null);
  const [deleting, setDeleting] = useState(false);

  const uplinksQ = useQuery({
    queryKey: ["bng-uplinks", deviceId],
    queryFn: () => apiFetch<{ uplinks: BngUplink[] }>(`/api/v1/bng/devices/${deviceId}/uplinks`),
    staleTime: 20_000,
  });
  const hasUplinks = (uplinksQ.data?.uplinks?.length ?? 0) > 0;

  const historyQ = useQuery({
    queryKey: ["bng-uplinks-history", deviceId, days],
    queryFn: () => apiFetch<UplinksHistoryResponse>(`/api/v1/bng/devices/${deviceId}/uplinks/history?days=${days}`),
    enabled: hasUplinks,
    staleTime: 30_000,
    refetchInterval: 60_000,
  });

  const chartRows = useMemo(() => {
    const results = historyQ.data?.uplinks ?? [];
    const bucket = historyQ.data?.bucket ?? "hour";
    const byTime = new Map<string, Record<string, number | string>>();
    for (const u of results) {
      for (const p of u.points) {
        const row = byTime.get(p.t) ?? { t: p.t, label: fmtAxisTime(p.t, bucket) };
        row[`${u.carrier_label} IN`] = p.in_bps;
        row[`${u.carrier_label} OUT`] = p.out_bps;
        byTime.set(p.t, row);
      }
    }
    return Array.from(byTime.values()).sort((a, b) => String(a.t).localeCompare(String(b.t)));
  }, [historyQ.data]);

  const carrierKeys = useMemo(() => {
    const set = new Set<string>();
    for (const u of historyQ.data?.uplinks ?? []) set.add(u.carrier_label);
    return Array.from(set);
  }, [historyQ.data]);

  async function deleteUplink() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await apiFetch(`/api/v1/bng/devices/${deviceId}/uplinks/${deleteTarget.id}`, { method: "DELETE" });
      toastOk(pushToast, "Uplink removido.");
      setDeleteTarget(null);
      void qc.invalidateQueries({ queryKey: ["bng-uplinks", deviceId] });
      void qc.invalidateQueries({ queryKey: ["bng-uplinks-history", deviceId] });
    } catch (e) {
      toastErr(pushToast, e, "Falha ao remover uplink.");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div style={{ marginBottom: 20 }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, flexWrap: "wrap", gap: 8 }}>
        <h3 style={{ fontSize: 14, margin: 0, color: "var(--muted)", fontWeight: 600, display: "flex", alignItems: "center", gap: 6 }}>
          Uplinks de operadora (internet)
          <InfoHint label="Sobre os uplinks de operadora">
            <p>
              Tráfego calculado a partir do histórico de interfaces (IF-MIB — ifHCInOctets/ifHCOutOctets) já coletado
              periodicamente. Marque como "tráfego total" só a interface que representa o total real da operadora
              (o agregado Eth-Trunk, quando existir) — interfaces físicas membro de um trunk ficam desmarcadas para
              não somar tráfego em dobro.
            </p>
          </InfoHint>
        </h3>
        <button
          type="button"
          className="btn btn--sm"
          onClick={() => {
            setEditing(null);
            setFormOpen(true);
          }}
        >
          <Plus size={13} style={{ marginRight: 4, verticalAlign: -2 }} />
          Adicionar uplink
        </button>
      </div>

      {!hasUplinks ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>
          Nenhum uplink configurado ainda. Clique em "Adicionar uplink" e escolha a interface (ex.: a que liga à
          operadora K2 ou o Eth-Trunk da FORTE) para começar a acompanhar o tráfego aqui.
        </p>
      ) : (
        <>
          <div className="row" style={{ gap: 12, marginBottom: 10, flexWrap: "wrap" }}>
            {Object.entries(historyQ.data?.carrier_totals ?? {}).map(([carrier, tot]) => (
              <div className="stat" key={carrier} style={{ minWidth: 170 }}>
                <div className="stat__k">{carrier} — agora</div>
                <div className="stat__v" style={{ fontSize: 15 }}>
                  ↓ {formatBitrate(tot.in_bps)} · ↑ {formatBitrate(tot.out_bps)}
                </div>
              </div>
            ))}
          </div>

          <div className="row" style={{ gap: 8, marginBottom: 10, flexWrap: "wrap", alignItems: "center" }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Período:</span>
            {DAY_OPTIONS.map((d) => (
              <button key={d} type="button" className={`btn btn--sm${days === d ? " btn--primary" : ""}`} onClick={() => setDays(d)}>
                {d === 1 ? "24 h" : `${d} dias`}
              </button>
            ))}
          </div>

          {chartRows.length === 0 ? (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Ainda sem pontos suficientes no período — o histórico de tráfego cresce a cada coleta de interfaces do BNG.
            </p>
          ) : (
            <div className="card" style={{ padding: 12, marginBottom: 14 }}>
              <ResponsiveContainer width="100%" height={260}>
                <LineChart data={chartRows} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis dataKey="label" tick={{ fontSize: 10 }} interval="preserveStartEnd" />
                  <YAxis tick={{ fontSize: 10 }} width={60} tickFormatter={(v) => formatBitrate(Number(v))} />
                  <Tooltip
                    formatter={(v: number, name: string) => [formatBitrate(v), name]}
                    contentStyle={{ background: "var(--panel2)", border: "1px solid var(--border)", fontSize: 12, color: "var(--text)" }}
                    itemStyle={{ color: "var(--text)" }}
                    labelStyle={{ color: "var(--text)" }}
                  />
                  <Legend />
                  {carrierKeys.map((c, i) => (
                    <Line
                      key={`${c} IN`}
                      type="monotone"
                      dataKey={`${c} IN`}
                      name={`${c} — download`}
                      stroke={CARRIER_COLORS[i * 2 % CARRIER_COLORS.length]}
                      strokeWidth={2}
                      dot={false}
                    />
                  ))}
                  {carrierKeys.map((c, i) => (
                    <Line
                      key={`${c} OUT`}
                      type="monotone"
                      dataKey={`${c} OUT`}
                      name={`${c} — upload`}
                      stroke={CARRIER_COLORS[(i * 2 + 1) % CARRIER_COLORS.length]}
                      strokeWidth={2}
                      strokeDasharray="4 3"
                      dot={false}
                    />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            </div>
          )}

          <div className="table-wrap">
            <table style={{ fontSize: 11 }}>
              <thead>
                <tr>
                  <th>Operadora</th>
                  <th>Interface</th>
                  <th>Conta no total</th>
                  <th className="mono">Agora (in/out)</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {(uplinksQ.data?.uplinks ?? []).map((u) => {
                  const cur = historyQ.data?.uplinks.find((r) => r.id === u.id)?.current;
                  return (
                    <tr key={u.id}>
                      <td>{u.carrier_label}</td>
                      <td>
                        {u.interface_label}
                        <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                          {u.if_descr || u.if_name || EM_DASH}
                        </div>
                      </td>
                      <td>{u.is_primary_traffic ? <span className="badge badge--ok">Sim</span> : <span className="badge badge--off">Não (membro)</span>}</td>
                      <td className="mono">{cur ? `${formatBitrate(cur.in_bps)} / ${formatBitrate(cur.out_bps)}` : EM_DASH}</td>
                      <td>
                        <div className="row" style={{ gap: 4, justifyContent: "flex-end" }}>
                          <button
                            type="button"
                            className="btn btn--icon btn--sm"
                            title="Editar"
                            onClick={() => {
                              setEditing(u);
                              setFormOpen(true);
                            }}
                          >
                            <Pencil size={13} />
                          </button>
                          <button
                            type="button"
                            className="btn btn--icon btn--sm"
                            title="Remover"
                            onClick={() => setDeleteTarget(u)}
                          >
                            <Trash2 size={13} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {formOpen ? (
        <UplinkFormModal
          deviceId={deviceId}
          editing={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => {
            void qc.invalidateQueries({ queryKey: ["bng-uplinks", deviceId] });
            void qc.invalidateQueries({ queryKey: ["bng-uplinks-history", deviceId] });
          }}
        />
      ) : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Remover uplink"
        message={deleteTarget ? `Remover "${deleteTarget.carrier_label} — ${deleteTarget.interface_label}"?` : ""}
        confirmLabel="Remover"
        cancelLabel="Cancelar"
        busy={deleting}
        onCancel={() => !deleting && setDeleteTarget(null)}
        onConfirm={() => void deleteUplink()}
      />
    </div>
  );
}
