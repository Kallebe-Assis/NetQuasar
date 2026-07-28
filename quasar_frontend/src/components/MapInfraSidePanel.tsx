import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
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
  type NetworkSpliceBox,
} from "../lib/networkInfrastructure";
import { APP_ROUTES } from "../app/routes";
import type { InfraMapKind } from "../lib/mapInfrastructureIcons";
import { INFRA_MAP_KIND_LABELS } from "../lib/mapInfrastructureIcons";

export function parseInfraMapId(mapId: string): { kind: InfraMapKind; id: string } | null {
  if (!mapId.startsWith("infra-")) return null;
  const rest = mapId.slice("infra-".length);
  const kinds: InfraMapKind[] = ["splice_box", "project", "cable", "pole", "cto"];
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
}: Props) {
  const parsed = mapId ? parseInfraMapId(mapId) : null;
  const canEdit = isAdminUser() || can("connections.manage") || can("map.manage");
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [splitterOpen, setSplitterOpen] = useState(false);
  const [cableFibersOpen, setCableFibersOpen] = useState(false);
  const [spliceOpen, setSpliceOpen] = useState(false);
  const [form, setForm] = useState({
    description: "",
    latitude: "",
    longitude: "",
    splitter: "",
    transmitter: "",
    fiber_color: "",
    notes: "",
    needs_maintenance: false,
  });
  const [err, setErr] = useState<string | null>(null);

  const ctoQ = useQuery({
    queryKey: ["map-cto-detail", parsed?.id],
    enabled: open && parsed?.kind === "cto" && !!parsed.id,
    queryFn: () => apiFetch<NetworkCto>(`/api/v1/commercial/network/ctos/${parsed!.id}`),
  });

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

  useEffect(() => {
    setEditing(false);
    setSplitterOpen(false);
    setCableFibersOpen(false);
    setSpliceOpen(false);
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
    if (!open || !autoOpenEdit || parsed?.kind !== "cto" || !canEdit) return;
    setEditing(true);
    onEditAutoOpened?.();
  }, [open, autoOpenEdit, mapId, parsed?.kind, canEdit, onEditAutoOpened]);

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
      fiber_color: c.fiber_color?.trim() ? c.fiber_color : "Desconhecido",
      notes: c.notes ?? "",
      needs_maintenance: !!c.needs_maintenance,
    });
  }, [ctoQ.data]);

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
      onSaved?.(next);
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar."),
  });

  if (!open || !parsed) return null;

  const title =
    parsed.kind === "cto"
      ? ctoQ.data?.description ?? fallback?.description ?? "CTO"
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

  return (
    <aside className="map-infra-panel" aria-label="Painel de infraestrutura">
      <div className="map-infra-panel__head">
        <div>
          <div className="map-infra-panel__kind">CTO</div>
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
                Editar no mapa
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
          <label className="map-infra-panel__field">
            <span>Transmissor</span>
            <input className="input" value={form.transmitter} onChange={(e) => setForm({ ...form, transmitter: e.target.value })} />
          </label>
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
          <dl className="map-infra-panel__dl">
            <div>
              <dt>Localização</dt>
              <dd className="mono">
                {fmtCoord(fallback?.lat)}, {fmtCoord(fallback?.lng)}
              </dd>
            </div>
          </dl>
          <div className="map-infra-panel__actions">
            <Link className="btn" to={APP_ROUTES.connections}>
              Abrir em Conexões
            </Link>
          </div>
        </div>
      ) : null}
    </aside>
  );
}
