import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { ConfirmModal } from "./ConfirmModal";
import { can, isAdminUser } from "../lib/auth";
import { apiFetch } from "../lib/api";
import { CtoSplitterModal } from "./CtoSplitterModal";
import { CableFibersModal } from "./CableFibersModal";
import { SpliceBoxModal } from "./SpliceBoxModal";
import {
  buildDefaultSplicePairs,
  buildDefaultSplitterPorts,
  formatFeedFiberColor,
  parseSplitterOutputs,
  type SplicePair,
  type SplitterPort,
} from "../lib/fiberSplitter";
import {
  FIBER_COLORS,
  fmtCoord,
  formatSplitterDisplay,
  normalizeSplitterInput,
  parseCoordInput,
  type NetworkCable,
  type NetworkCto,
  type NetworkPole,
  type NetworkSpliceBox,
} from "../lib/networkInfrastructure";
import { APP_ROUTES } from "../app/routes";
import { OltInterfaceSelects } from "./OltInterfaceSelects";
import { formatOltPonLabel, type OltPonCatalog } from "../lib/oltPonInterfaces";
import { queryKeys } from "../lib/queryKeys";
import type { InfraMapKind } from "../lib/mapInfrastructureIcons";
import { INFRA_MAP_KIND_LABELS } from "../lib/mapInfrastructureIcons";

export function parseInfraMapId(mapId: string): { kind: InfraMapKind; id: string } | null {
  if (!mapId.startsWith("infra-")) return null;
  const rest = mapId.slice("infra-".length);
  const kinds: InfraMapKind[] = ["splice_box", "project", "cable", "pole", "cto", "pop"];
  for (const kind of kinds) {
    if (rest.startsWith(`${kind}-`)) {
      return { kind, id: rest.slice(kind.length + 1) };
    }
  }
  return null;
}

function normalizePortsFromApi(
  raw: NetworkCto["splitter_ports"] | NetworkCable["fiber_ports"],
  countHint?: number | null,
): SplitterPort[] | null {
  if (!raw || !Array.isArray(raw) || raw.length === 0) return null;
  const n = countHint && countHint > 0 ? countHint : raw.length;
  return buildDefaultSplitterPorts(
    n,
    raw.map((p) => ({
      port: Number(p.port) || 0,
      color: String(p.color ?? ""),
      color_hex: String(p.color_hex ?? "#64748b"),
      label: String(p.label ?? ""),
      status: (p.status as SplitterPort["status"]) || "livre",
      note: String(p.note ?? ""),
      destination: String(p.destination ?? ""),
    })),
  );
}

type Props = {
  open: boolean;
  mapId: string | null;
  fallback?: { description: string; lat: number; lng: number; category: string; splitter?: string | null } | null;
  onClose: () => void;
  onSaved?: (next: { lat: number; lng: number; description: string }) => void;
  /** Abre o modal de splitter assim que a CTO estiver seleccionada (ex.: botão do popup do mapa). */
  autoOpenSplitter?: boolean;
  onSplitterAutoOpened?: () => void;
  /** Abre o modal de fibras do cabo (popup do mapa). */
  autoOpenCableFibers?: boolean;
  onCableFibersAutoOpened?: () => void;
  /** Abre o modal de emenda (caixa de emenda). */
  autoOpenSplice?: boolean;
  onSpliceAutoOpened?: () => void;
  /** Abre directamente o formulário de edição (posição, etc.). */
  autoOpenEdit?: boolean;
  onEditAutoOpened?: () => void;
  /** Só em modo edição do mapa: excluir / ocultar / reposicionar. */
  mapEditMode?: boolean;
  onHideFromMap?: (mapId: string) => void;
  onStartReposition?: (mapId: string, kind: InfraMapKind, entityId: string) => void;
  onDeleted?: (mapId: string) => void;
};

export function MapInfraSidePanel({
  open,
  mapId,
  fallback,
  onClose,
  onSaved,
  autoOpenSplitter = false,
  onSplitterAutoOpened,
  autoOpenCableFibers = false,
  onCableFibersAutoOpened,
  autoOpenSplice = false,
  onSpliceAutoOpened,
  autoOpenEdit = false,
  onEditAutoOpened,
  mapEditMode = false,
  onHideFromMap,
  onStartReposition,
  onDeleted,
}: Props) {
  const parsed = mapId ? parseInfraMapId(mapId) : null;
  const canEdit = isAdminUser() || can("connections.manage") || can("map.manage");
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [splitterOpen, setSplitterOpen] = useState(false);
  const [cableFibersOpen, setCableFibersOpen] = useState(false);
  const [spliceOpen, setSpliceOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [form, setForm] = useState({
    description: "",
    latitude: "",
    longitude: "",
    splitter: "",
    transmitter: "",
    olt_device_id: "",
    pon: "",
    fiber_color: "",
    notes: "",
    needs_maintenance: false,
  });
  const [err, setErr] = useState<string | null>(null);
  const [descDraft, setDescDraft] = useState("");

  const ctoQ = useQuery({
    queryKey: ["map-cto-detail", parsed?.id],
    enabled: open && parsed?.kind === "cto" && !!parsed.id,
    queryFn: () => apiFetch<NetworkCto>(`/api/v1/commercial/network/ctos/${parsed!.id}`),
  });

  const oltsQ = useQuery({
    queryKey: queryKeys.oltDevices,
    queryFn: () => apiFetch<{ olts: OltPonCatalog[] }>("/api/v1/olt/devices"),
    enabled: open && parsed?.kind === "cto",
    staleTime: 5 * 60 * 1000,
  });
  const olts = oltsQ.data?.olts ?? [];

  const cableQ = useQuery({
    queryKey: ["map-cable-detail", parsed?.id],
    enabled: open && parsed?.kind === "cable" && !!parsed.id,
    queryFn: () => apiFetch<NetworkCable>(`/api/v1/commercial/network/cables/${parsed!.id}`),
  });

  const spliceQ = useQuery({
    queryKey: ["map-splice-detail", parsed?.id],
    enabled: open && parsed?.kind === "splice_box" && !!parsed.id,
    queryFn: () => apiFetch<NetworkSpliceBox>(`/api/v1/commercial/network/splice-boxes/${parsed!.id}`),
  });

  const poleQ = useQuery({
    queryKey: ["map-pole-detail", parsed?.id],
    enabled: open && parsed?.kind === "pole" && !!parsed.id,
    queryFn: () => apiFetch<NetworkPole>(`/api/v1/commercial/network/poles/${parsed!.id}`),
  });

  const popQ = useQuery({
    queryKey: ["map-pop-detail", parsed?.id],
    enabled: open && parsed?.kind === "pop" && !!parsed.id,
    queryFn: () =>
      apiFetch<{
        id: string;
        description: string;
        address?: string | null;
        latitude?: number | null;
        longitude?: number | null;
        locality_name?: string | null;
        device_count?: number;
      }>(`/api/v1/pops/${parsed!.id}`),
  });

  useEffect(() => {
    setEditing(false);
    setSplitterOpen(false);
    setCableFibersOpen(false);
    setSpliceOpen(false);
    setConfirmDelete(false);
    setErr(null);
  }, [mapId]);

  useEffect(() => {
    if (!open || !autoOpenSplitter || parsed?.kind !== "cto") return;
    setSplitterOpen(true);
    onSplitterAutoOpened?.();
  }, [open, autoOpenSplitter, mapId, parsed?.kind, onSplitterAutoOpened]);

  useEffect(() => {
    if (!open || !autoOpenCableFibers || parsed?.kind !== "cable") return;
    setCableFibersOpen(true);
    onCableFibersAutoOpened?.();
  }, [open, autoOpenCableFibers, mapId, parsed?.kind, onCableFibersAutoOpened]);

  useEffect(() => {
    if (!open || !autoOpenSplice || parsed?.kind !== "splice_box") return;
    setSpliceOpen(true);
    onSpliceAutoOpened?.();
  }, [open, autoOpenSplice, mapId, parsed?.kind, onSpliceAutoOpened]);

  useEffect(() => {
    if (!open || !autoOpenEdit || !canEdit) return;
    if (parsed?.kind === "cto" && !mapEditMode) setEditing(true);
    onEditAutoOpened?.();
  }, [open, autoOpenEdit, mapId, parsed?.kind, canEdit, mapEditMode, onEditAutoOpened]);

  const splitterPorts = useMemo(() => {
    const n = parseSplitterOutputs(ctoQ.data?.splitter) ?? undefined;
    return normalizePortsFromApi(ctoQ.data?.splitter_ports, n ?? null);
  }, [ctoQ.data?.splitter_ports, ctoQ.data?.splitter]);

  const cablePorts = useMemo(
    () => normalizePortsFromApi(cableQ.data?.fiber_ports, cableQ.data?.fiber_count ?? null),
    [cableQ.data?.fiber_ports, cableQ.data?.fiber_count],
  );

  const splicePorts = useMemo(() => {
    const n = parseSplitterOutputs(spliceQ.data?.splitter) ?? spliceQ.data?.fiber_count ?? undefined;
    return normalizePortsFromApi(spliceQ.data?.splitter_ports, n ?? null);
  }, [spliceQ.data?.splitter_ports, spliceQ.data?.splitter, spliceQ.data?.fiber_count]);

  const splicePairs = useMemo((): SplicePair[] | null => {
    const raw = spliceQ.data?.splice_pairs;
    if (!raw || !Array.isArray(raw) || raw.length === 0) return null;
    const n = spliceQ.data?.fiber_count && spliceQ.data.fiber_count > 0 ? spliceQ.data.fiber_count : raw.length;
    return buildDefaultSplicePairs(
      n,
      raw.map((p) => ({
        port: Number(p.port) || 0,
        left_color: String(p.left_color ?? ""),
        left_color_hex: String(p.left_color_hex ?? "#64748b"),
        right_color: String(p.right_color ?? ""),
        right_color_hex: String(p.right_color_hex ?? "#64748b"),
        status: (p.status as SplicePair["status"]) || "livre",
        note: String(p.note ?? ""),
        destination: String(p.destination ?? ""),
      })),
    );
  }, [spliceQ.data?.splice_pairs, spliceQ.data?.fiber_count]);

  useEffect(() => {
    const c = ctoQ.data;
    if (!c) return;
    setForm({
      description: c.description ?? "",
      latitude: c.latitude != null ? String(c.latitude) : "",
      longitude: c.longitude != null ? String(c.longitude) : "",
      splitter: c.splitter ? normalizeSplitterInput(c.splitter) ?? c.splitter : "",
      transmitter: c.transmitter ?? "",
      olt_device_id: c.olt_device_id ?? "",
      pon: c.pon != null && Number(c.pon) > 0 ? String(c.pon) : "",
      fiber_color: c.fiber_color?.trim() ? c.fiber_color : "Desconhecido",
      notes: c.notes ?? "",
      needs_maintenance: !!c.needs_maintenance,
    });
  }, [ctoQ.data]);

  useEffect(() => {
    const next =
      parsed?.kind === "cto"
        ? (ctoQ.data?.description ?? fallback?.description ?? "")
        : parsed?.kind === "cable"
          ? (cableQ.data?.description ?? fallback?.description ?? "")
          : parsed?.kind === "splice_box"
            ? (spliceQ.data?.description ?? fallback?.description ?? "")
            : parsed?.kind === "pole"
              ? (poleQ.data?.description ?? fallback?.description ?? "")
              : parsed?.kind === "pop"
                ? (popQ.data?.description ?? fallback?.description ?? "")
                : (fallback?.description ?? "");
    setDescDraft(next);
  }, [
    parsed?.kind,
    ctoQ.data?.description,
    cableQ.data?.description,
    spliceQ.data?.description,
    poleQ.data?.description,
    popQ.data?.description,
    fallback?.description,
  ]);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (!parsed || parsed.kind !== "cto") throw new Error("Só CTOs podem ser editadas aqui.");
      const description = form.description.trim();
      if (!description) throw new Error("Descrição obrigatória.");
      const lat = parseCoordInput(form.latitude);
      const lng = parseCoordInput(form.longitude);
      if (lat == null || lng == null) throw new Error("Latitude e longitude obrigatórias.");
      await apiFetch(`/api/v1/commercial/network/ctos/${parsed.id}`, {
        method: "PATCH",
        json: {
          description,
          latitude: lat,
          longitude: lng,
          splitter: normalizeSplitterInput(form.splitter) || null,
          transmitter: form.transmitter.trim() || null,
          olt_device_id: form.olt_device_id.trim() || null,
          pon: (() => {
            const n = Number(form.pon.trim());
            return Number.isFinite(n) && n > 0 ? n : null;
          })(),
          fiber_color: form.fiber_color.trim() || null,
          notes: form.notes.trim() || null,
          needs_maintenance: form.needs_maintenance,
        },
      });
      return { lat, lng, description };
    },
    onSuccess: async (next) => {
      setEditing(false);
      setErr(null);
      await qc.invalidateQueries({ queryKey: ["map-cto-detail", parsed?.id] });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      await qc.invalidateQueries({ queryKey: queryKeys.networkCtos });
      onSaved?.(next);
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar."),
  });

  const deleteMut = useMutation({
    mutationFn: async () => {
      if (!parsed) throw new Error("Elemento inválido.");
      const path =
        parsed.kind === "cto"
          ? `/api/v1/commercial/network/ctos/${parsed.id}`
          : parsed.kind === "cable"
            ? `/api/v1/commercial/network/cables/${parsed.id}`
            : parsed.kind === "splice_box"
              ? `/api/v1/commercial/network/splice-boxes/${parsed.id}`
              : parsed.kind === "pole"
                ? `/api/v1/commercial/network/poles/${parsed.id}`
                : parsed.kind === "pop"
                  ? `/api/v1/pops/${parsed.id}`
                  : null;
      if (!path) throw new Error("Este tipo não pode ser excluído aqui.");
      await apiFetch(path, { method: "DELETE" });
    },
    onSuccess: async () => {
      setConfirmDelete(false);
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      await qc.invalidateQueries({ queryKey: queryKeys.pops });
      if (mapId) onDeleted?.(mapId);
      onClose();
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao excluir."),
  });

  const saveDescMut = useMutation({
    mutationFn: async () => {
      if (!parsed) throw new Error("Elemento inválido.");
      const description = descDraft.trim();
      if (!description) throw new Error("Descrição obrigatória.");
      const path =
        parsed.kind === "cto"
          ? `/api/v1/commercial/network/ctos/${parsed.id}`
          : parsed.kind === "cable"
            ? `/api/v1/commercial/network/cables/${parsed.id}`
            : parsed.kind === "splice_box"
              ? `/api/v1/commercial/network/splice-boxes/${parsed.id}`
              : parsed.kind === "pole"
                ? `/api/v1/commercial/network/poles/${parsed.id}`
                : parsed.kind === "pop"
                  ? `/api/v1/pops/${parsed.id}`
                  : null;
      if (!path) throw new Error("Este tipo não pode ser editado aqui.");
      await apiFetch(path, { method: "PATCH", json: { description } });
      return description;
    },
    onSuccess: async (description) => {
      setErr(null);
      await qc.invalidateQueries({ queryKey: ["map-cto-detail", parsed?.id] });
      await qc.invalidateQueries({ queryKey: ["map-cable-detail", parsed?.id] });
      await qc.invalidateQueries({ queryKey: ["map-splice-detail", parsed?.id] });
      await qc.invalidateQueries({ queryKey: ["map-pole-detail", parsed?.id] });
      await qc.invalidateQueries({ queryKey: ["map-pop-detail", parsed?.id] });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      await qc.invalidateQueries({ queryKey: queryKeys.networkCtos });
      await qc.invalidateQueries({ queryKey: queryKeys.pops });
      const lat = Number(ctoQ.data?.latitude ?? cableQ.data?.latitude ?? spliceQ.data?.latitude ?? poleQ.data?.latitude ?? popQ.data?.latitude ?? fallback?.lat ?? 0);
      const lng = Number(ctoQ.data?.longitude ?? cableQ.data?.longitude ?? spliceQ.data?.longitude ?? poleQ.data?.longitude ?? popQ.data?.longitude ?? fallback?.lng ?? 0);
      onSaved?.({ lat, lng, description });
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar a descrição."),
  });

  if (!open || !parsed) return null;

  const editableKinds: InfraMapKind[] = ["cto", "cable", "splice_box", "pole", "pop"];
  const canMapEditActions = mapEditMode && canEdit && editableKinds.includes(parsed.kind);

  const title =
    parsed.kind === "cto"
      ? ctoQ.data?.description ?? fallback?.description ?? "CTO"
      : parsed.kind === "cable"
        ? cableQ.data?.description ?? fallback?.description ?? INFRA_MAP_KIND_LABELS[parsed.kind]
        : parsed.kind === "splice_box"
          ? spliceQ.data?.description ?? fallback?.description ?? INFRA_MAP_KIND_LABELS[parsed.kind]
          : parsed.kind === "pole"
            ? poleQ.data?.description ?? fallback?.description ?? INFRA_MAP_KIND_LABELS[parsed.kind]
            : parsed.kind === "pop"
              ? popQ.data?.description ?? fallback?.description ?? INFRA_MAP_KIND_LABELS[parsed.kind]
              : fallback?.description ?? INFRA_MAP_KIND_LABELS[parsed.kind];

  const lat = ctoQ.data?.latitude ?? fallback?.lat;
  const lng = ctoQ.data?.longitude ?? fallback?.lng;

  const splitterDisp = formatSplitterDisplay(ctoQ.data?.splitter ?? fallback?.splitter ?? null);
  const splitterLabel = splitterDisp !== "—" ? splitterDisp : "—";
  const feedFiberLabel = formatFeedFiberColor(ctoQ.data?.fiber_color);
  const panelTitle = `CTO - ${splitterLabel}`;
  const panelSubtitle =
    parsed.kind === "cto"
      ? ctoQ.data?.description ?? fallback?.description ?? null
      : null;

  function renderMapEditActions() {
    if (!canMapEditActions || !mapId || !parsed) return null;
    return (
      <div className="map-infra-panel__edit-actions">
        <p className="map-infra-panel__muted" style={{ margin: "0 0 6px" }}>
          Modo edição
        </p>
        {err ? <div className="msg msg--err">{err}</div> : null}
        <label className="map-infra-panel__field">
          <span>Descrição</span>
          <input
            className="input"
            value={descDraft}
            onChange={(e) => setDescDraft(e.target.value)}
            disabled={saveDescMut.isPending}
          />
        </label>
        <div className="map-infra-panel__actions" style={{ marginBottom: 8 }}>
          <button
            type="button"
            className="btn btn--primary"
            disabled={saveDescMut.isPending || !descDraft.trim()}
            onClick={() => saveDescMut.mutate()}
          >
            {saveDescMut.isPending ? "A guardar…" : "Guardar descrição"}
          </button>
        </div>
        <div className="map-infra-panel__actions">
          <button
            type="button"
            className="btn"
            onClick={() => onStartReposition?.(mapId, parsed.kind, parsed.id)}
          >
            {parsed.kind === "cable" ? "Reposicionar trajeto" : "Reposicionar no mapa"}
          </button>
          <button type="button" className="btn" onClick={() => onHideFromMap?.(mapId)}>
            Ocultar no mapa
          </button>
          <button type="button" className="btn btn--danger" onClick={() => setConfirmDelete(true)}>
            Excluir
          </button>
        </div>
      </div>
    );
  }

  return (
    <aside className="map-infra-panel" aria-label="Painel de infraestrutura">
      <div className="map-infra-panel__head">
        <div>
          <div className="map-infra-panel__kind">{INFRA_MAP_KIND_LABELS[parsed.kind]}</div>
          <h2 className="map-infra-panel__title">{parsed.kind === "cto" ? panelTitle : title}</h2>
          {parsed.kind === "cto" && panelSubtitle && panelSubtitle !== panelTitle ? (
            <p className="map-infra-panel__muted" style={{ marginTop: 2 }}>
              {panelSubtitle}
            </p>
          ) : null}
        </div>
        <button type="button" className="btn btn--icon" aria-label="Fechar painel" onClick={onClose}>
          ×
        </button>
      </div>

      {parsed.kind === "cto" && ctoQ.isLoading ? (
        <p className="map-infra-panel__muted">A carregar CTO…</p>
      ) : null}
      {parsed.kind === "cto" && ctoQ.isError ? (
        <div className="msg msg--err">{(ctoQ.error as Error).message}</div>
      ) : null}

      {parsed.kind === "cto" && !editing ? (
        <div className="map-infra-panel__body">
          {renderMapEditActions()}
          <dl className="map-infra-panel__dl">
            <div>
              <dt>Nº</dt>
              <dd className="mono">{ctoQ.data?.display_number ?? "—"}</dd>
            </div>
            <div>
              <dt>Localização</dt>
              <dd className="mono">
                {fmtCoord(lat)}, {fmtCoord(lng)}
              </dd>
            </div>
            <div>
              <dt>Splitter</dt>
              <dd>{formatSplitterDisplay(ctoQ.data?.splitter)}</dd>
            </div>
            <div>
              <dt>Transmissor</dt>
              <dd>{ctoQ.data?.transmitter || "—"}</dd>
            </div>
            <div>
              <dt>Interface</dt>
              <dd>
                {ctoQ.data?.pon
                  ? formatOltPonLabel(Number(ctoQ.data.pon), String(ctoQ.data.pon_description ?? "").trim(), "")
                  : "—"}
              </dd>
            </div>
            <div>
              <dt>VLAN</dt>
              <dd>{ctoQ.data?.vlan != null && ctoQ.data.vlan !== "" ? String(ctoQ.data.vlan) : "—"}</dd>
            </div>
            <div>
              <dt>Cor da fibra</dt>
              <dd>{feedFiberLabel}</dd>
            </div>
            <div>
              <dt>Projeto</dt>
              <dd>{ctoQ.data?.project_label || "—"}</dd>
            </div>
            <div>
              <dt>Localidade</dt>
              <dd>{ctoQ.data?.locality_name || "—"}</dd>
            </div>
            <div>
              <dt>Manutenção</dt>
              <dd>{ctoQ.data?.needs_maintenance ? "Sim" : "Não"}</dd>
            </div>
            {ctoQ.data?.notes ? (
              <div>
                <dt>Notas</dt>
                <dd>{ctoQ.data.notes}</dd>
              </div>
            ) : null}
          </dl>
          <div className="map-infra-panel__actions">
            <button type="button" className="btn btn--primary" onClick={() => setSplitterOpen(true)}>
              Visualizar splitter
            </button>
            {canEdit ? (
              <button type="button" className="btn" onClick={() => setEditing(true)}>
                Editar dados
              </button>
            ) : null}
            <Link className="btn" to={`${APP_ROUTES.connections}?tab=cto`}>
              Abrir em Conexões
            </Link>
            {lat != null && lng != null ? (
              <a
                className="btn"
                href={`https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=18/${lat}/${lng}`}
                target="_blank"
                rel="noreferrer"
              >
                OSM
              </a>
            ) : null}
          </div>
        </div>
      ) : null}

      {parsed.kind === "cto" && parsed.id ? (
        <CtoSplitterModal
          open={splitterOpen}
          ctoId={parsed.id}
          ctoName={title}
          splitter={ctoQ.data?.splitter}
          feedFiberColor={ctoQ.data?.fiber_color}
          ports={splitterPorts}
          canEdit={canEdit}
          onClose={() => setSplitterOpen(false)}
        />
      ) : null}

      {parsed.kind === "cto" && editing ? (
        <form
          className="map-infra-panel__body"
          onSubmit={(e) => {
            e.preventDefault();
            saveMut.mutate();
          }}
        >
          <label className="map-infra-panel__field">
            <span>Descrição</span>
            <input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </label>
          <div className="map-infra-panel__grid2">
            <label className="map-infra-panel__field">
              <span>Latitude</span>
              <input className="input mono" value={form.latitude} onChange={(e) => setForm({ ...form, latitude: e.target.value })} />
            </label>
            <label className="map-infra-panel__field">
              <span>Longitude</span>
              <input className="input mono" value={form.longitude} onChange={(e) => setForm({ ...form, longitude: e.target.value })} />
            </label>
          </div>
          <label className="map-infra-panel__field">
            <span>Splitter</span>
            <input className="input" value={form.splitter} onChange={(e) => setForm({ ...form, splitter: e.target.value })} placeholder="1x8" />
          </label>
          <OltInterfaceSelects
            olts={olts}
            oltDeviceId={form.olt_device_id}
            transmitter={form.transmitter}
            pon={form.pon}
            disabled={saveMut.isPending}
            fieldClassName="map-infra-panel__field"
            labelClassName=""
            onChange={(next) => setForm({ ...form, ...next })}
          />
          <label className="map-infra-panel__field">
            <span>Cor da fibra</span>
            <select className="select" value={form.fiber_color} onChange={(e) => setForm({ ...form, fiber_color: e.target.value })}>
              <option value="">—</option>
              {FIBER_COLORS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
          <label className="map-infra-panel__field">
            <span>Notas</span>
            <textarea className="input" rows={3} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
          </label>
          <label className="conn-switch">
            <input
              type="checkbox"
              checked={form.needs_maintenance}
              onChange={(e) => setForm({ ...form, needs_maintenance: e.target.checked })}
            />
            Necessita manutenção
          </label>
          {err ? <div className="msg msg--err">{err}</div> : null}
          <div className="map-infra-panel__actions">
            <button type="submit" className="btn btn--primary" disabled={saveMut.isPending}>
              {saveMut.isPending ? "A guardar…" : "Guardar"}
            </button>
            <button type="button" className="btn" disabled={saveMut.isPending} onClick={() => setEditing(false)}>
              Cancelar
            </button>
          </div>
        </form>
      ) : null}

      {parsed.kind === "cable" ? (
        <div className="map-infra-panel__body">
          {renderMapEditActions()}
          {cableQ.isLoading ? <p className="map-infra-panel__muted">A carregar cabo…</p> : null}
          {cableQ.isError ? <div className="msg msg--err">Não foi possível carregar o cabo.</div> : null}
          <dl className="map-infra-panel__dl">
            <div>
              <dt>Descrição</dt>
              <dd>{cableQ.data?.description || fallback?.description || "—"}</dd>
            </div>
            <div>
              <dt>Tipo</dt>
              <dd>{cableQ.data?.cable_type || fallback?.category || "Cabo"}</dd>
            </div>
            <div>
              <dt>Fibras</dt>
              <dd>{cableQ.data?.fiber_count != null ? `${cableQ.data.fiber_count} fibras` : "—"}</dd>
            </div>
            <div>
              <dt>Status</dt>
              <dd>{cableQ.data?.status || "—"}</dd>
            </div>
            <div>
              <dt>Projeto</dt>
              <dd>{cableQ.data?.project_label || "—"}</dd>
            </div>
            <div>
              <dt>Localização</dt>
              <dd className="mono">
                {fmtCoord(cableQ.data?.latitude ?? fallback?.lat)}, {fmtCoord(cableQ.data?.longitude ?? fallback?.lng)}
              </dd>
            </div>
          </dl>
          <div className="map-infra-panel__actions">
            <button type="button" className="btn btn--primary" onClick={() => setCableFibersOpen(true)} disabled={!cableQ.data}>
              Fibras
            </button>
            <Link className="btn" to={`${APP_ROUTES.connections}?tab=cables`}>
              Abrir em Conexões
            </Link>
          </div>
        </div>
      ) : null}

      {parsed.kind === "cable" && parsed.id ? (
        <CableFibersModal
          open={cableFibersOpen}
          cableId={parsed.id}
          cableName={cableQ.data?.description || fallback?.description || "Cabo"}
          fiberCount={cableQ.data?.fiber_count}
          ports={cablePorts}
          canEdit={canEdit}
          onClose={() => setCableFibersOpen(false)}
        />
      ) : null}

      {parsed.kind === "splice_box" ? (
        <div className="map-infra-panel__body">
          {renderMapEditActions()}
          {spliceQ.isLoading ? <p className="map-infra-panel__muted">A carregar caixa…</p> : null}
          {spliceQ.isError ? <div className="msg msg--err">Não foi possível carregar a caixa de emenda.</div> : null}
          <dl className="map-infra-panel__dl">
            <div>
              <dt>Descrição</dt>
              <dd>{spliceQ.data?.description || fallback?.description || "—"}</dd>
            </div>
            <div>
              <dt>Modelo</dt>
              <dd>{spliceQ.data?.box_model === "distribuicao" ? "Distribuição" : "Emenda"}</dd>
            </div>
            <div>
              <dt>Fibras</dt>
              <dd>
                {spliceQ.data?.box_model === "distribuicao"
                  ? formatSplitterDisplay(spliceQ.data?.splitter) || "—"
                  : spliceQ.data?.fiber_count != null
                    ? `${spliceQ.data.fiber_count} fibras`
                    : "—"}
              </dd>
            </div>
            <div>
              <dt>Projeto</dt>
              <dd>{spliceQ.data?.project_label || "—"}</dd>
            </div>
            <div>
              <dt>Localização</dt>
              <dd className="mono">
                {fmtCoord(spliceQ.data?.latitude ?? fallback?.lat)}, {fmtCoord(spliceQ.data?.longitude ?? fallback?.lng)}
              </dd>
            </div>
          </dl>
          <div className="map-infra-panel__actions">
            <button type="button" className="btn btn--primary" onClick={() => setSpliceOpen(true)} disabled={!spliceQ.data}>
              Interior
            </button>
            <Link className="btn" to={`${APP_ROUTES.connections}?tab=splice`}>
              Abrir em Conexões
            </Link>
          </div>
        </div>
      ) : null}

      {parsed.kind === "splice_box" && parsed.id ? (
        <SpliceBoxModal
          open={spliceOpen}
          spliceId={parsed.id}
          spliceName={spliceQ.data?.description || fallback?.description || "Caixa de emenda"}
          boxModel={spliceQ.data?.box_model}
          fiberCount={spliceQ.data?.fiber_count}
          splitter={spliceQ.data?.splitter}
          feedFiberColor={spliceQ.data?.fiber_color}
          ports={splicePorts}
          pairs={splicePairs}
          canEdit={canEdit}
          onClose={() => setSpliceOpen(false)}
        />
      ) : null}

      {parsed.kind !== "cto" && parsed.kind !== "cable" && parsed.kind !== "splice_box" ? (
        <div className="map-infra-panel__body">
          {renderMapEditActions()}
          {parsed.kind === "pole" && poleQ.isLoading ? <p className="map-infra-panel__muted">A carregar poste…</p> : null}
          {parsed.kind === "pop" && popQ.isLoading ? <p className="map-infra-panel__muted">A carregar POP…</p> : null}
          <dl className="map-infra-panel__dl">
            <div>
              <dt>Descrição</dt>
              <dd>
                {parsed.kind === "pop"
                  ? popQ.data?.description || fallback?.description || "—"
                  : poleQ.data?.description || fallback?.description || "—"}
              </dd>
            </div>
            {parsed.kind === "pole" ? (
              <div>
                <dt>Tipo</dt>
                <dd>{poleQ.data?.pole_type || fallback?.category || "Poste"}</dd>
              </div>
            ) : null}
            {parsed.kind === "pop" ? (
              <>
                <div>
                  <dt>Localidade</dt>
                  <dd>{popQ.data?.locality_name || "—"}</dd>
                </div>
                <div>
                  <dt>Equipamentos</dt>
                  <dd>{popQ.data?.device_count ?? 0}</dd>
                </div>
                {popQ.data?.address ? (
                  <div>
                    <dt>Endereço</dt>
                    <dd>{popQ.data.address}</dd>
                  </div>
                ) : null}
              </>
            ) : null}
            <div>
              <dt>Localização</dt>
              <dd className="mono">
                {fmtCoord(
                  parsed.kind === "pop"
                    ? (popQ.data?.latitude ?? fallback?.lat)
                    : (poleQ.data?.latitude ?? fallback?.lat),
                )}
                ,{" "}
                {fmtCoord(
                  parsed.kind === "pop"
                    ? (popQ.data?.longitude ?? fallback?.lng)
                    : (poleQ.data?.longitude ?? fallback?.lng),
                )}
              </dd>
            </div>
          </dl>
          <div className="map-infra-panel__actions">
            <Link className="btn" to={parsed.kind === "pop" ? APP_ROUTES.pops : APP_ROUTES.connections}>
              {parsed.kind === "pop" ? "Abrir em POPs" : "Abrir em Conexões"}
            </Link>
          </div>
        </div>
      ) : null}

      {confirmDelete ? (
        <ConfirmModal
          open
          title="Excluir elemento"
          message={`Tem a certeza que deseja excluir permanentemente este ${INFRA_MAP_KIND_LABELS[parsed.kind] ?? "elemento"}?`}
          confirmLabel="Excluir"
          danger
          busy={deleteMut.isPending}
          onCancel={() => setConfirmDelete(false)}
          onConfirm={() => deleteMut.mutate()}
        />
      ) : null}
    </aside>
  );
}
