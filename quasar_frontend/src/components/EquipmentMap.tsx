import L from "leaflet";
import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { MapContainer, Marker, Popup, TileLayer, useMap } from "react-leaflet";
import "leaflet/dist/leaflet.css";
import { infrastructurePinIcon, isInfraMapKind, type InfraMapKind } from "../lib/mapInfrastructureIcons";

export type MapPointKind = "equipment" | "connection" | InfraMapKind;

export type MapPoint = {
  id: string;
  description: string;
  lat: number;
  lng: number;
  ip?: string | null;
  category: string;
  status: string;
  /** Tipo de ícone no mapa (CTO, poste, equipamento, etc.). */
  mapKind?: MapPointKind;
  /** Cor opcional (ex.: cor do projeto). */
  markerColor?: string | null;
  /** Etiqueta curta no pin (ex.: descrição da CTO). */
  mapLabel?: string | null;
};

/** Agrupado (grelha por tipo), Desagrupado (marcadores individuais + empilhamento), Online/Offline (pins verde / vermelho / cinza). */
export type MapDisplayMode = "cluster" | "scatter" | "status";

export type MapColors = {
  equipment: string;
  connection: string;
};

export const DEFAULT_MAP_COLORS: MapColors = {
  equipment: "#3388ff",
  connection: "#3b82f6",
};

const STACK_MERGE_M = 22;
const SPIDER_RADIUS_M = 28;
const FIT_PADDING: [number, number] = [48, 48];
const FIT_MAX_ZOOM = 16;
const SINGLE_POINT_ZOOM = 14;
const CLUSTER_EXPAND_MAX_ZOOM = 17;

function haversineM(aLat: number, aLng: number, bLat: number, bLng: number): number {
  const R = 6371000;
  const toR = (d: number) => (d * Math.PI) / 180;
  const dLat = toR(bLat - aLat);
  const dLng = toR(bLng - aLng);
  const s1 = Math.sin(dLat / 2);
  const s2 = Math.sin(dLng / 2);
  const h = s1 * s1 + Math.cos(toR(aLat)) * Math.cos(toR(bLat)) * s2 * s2;
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(h)));
}

function ringOffsetLatLng(centerLat: number, centerLng: number, index: number, total: number, radiusM: number): [number, number] {
  if (total <= 1 || radiusM <= 0) return [centerLat, centerLng];
  const angle = (2 * Math.PI * index) / total - Math.PI / 2;
  const dy = radiusM * Math.cos(angle);
  const dx = radiusM * Math.sin(angle);
  const R = 6378137;
  const dLat = (dy / R) * (180 / Math.PI);
  const dLng = (dx / (R * Math.cos((centerLat * Math.PI) / 180))) * (180 / Math.PI);
  return [centerLat + dLat, centerLng + dLng];
}

function easeOutCubic(t: number): number {
  return 1 - (1 - t) ** 3;
}

function mergeProximityStacks(points: MapPoint[], maxM: number): MapPoint[][] {
  let clusters: MapPoint[][] = points.map((p) => [p]);
  let changed = true;
  while (changed) {
    changed = false;
    outer: for (let i = 0; i < clusters.length; i++) {
      for (let j = i + 1; j < clusters.length; j++) {
        let minD = Infinity;
        for (const a of clusters[i]) {
          for (const b of clusters[j]) {
            const d = haversineM(a.lat, a.lng, b.lat, b.lng);
            if (d < minD) minD = d;
          }
        }
        if (minD <= maxM) {
          clusters[i] = clusters[i].concat(clusters[j]);
          clusters.splice(j, 1);
          changed = true;
          break outer;
        }
      }
    }
  }
  return clusters;
}

/** Mais zoom ⇒ mais casas decimais na grelha ⇒ células menores (mais «desagrupado» na vista). */
function gridDecimalsForZoom(z: number): number {
  if (!Number.isFinite(z)) return 2;
  if (z <= 5) return 1;
  if (z <= 8) return 2;
  if (z <= 11) return 3;
  if (z <= 14) return 4;
  return 5;
}

/** Identificador leve para invalidar clusters — evita sort de milhares de IDs. */
function pointsFingerprint(points: MapPoint[]): string {
  let h = points.length;
  for (let i = 0; i < points.length; i++) {
    const id = points[i].id;
    for (let j = 0; j < id.length; j++) h = (h * 33 + id.charCodeAt(j)) | 0;
  }
  return `${points.length}:${h}`;
}

function pointClusterKind(p: MapPoint): string {
  if (p.status === "connection" || p.mapKind === "connection") return "connection";
  if (p.mapKind && isInfraMapKind(p.mapKind)) return p.mapKind;
  return p.category || "equipamento";
}

function clusterKindLabel(kind: string, count: number): string {
  if (kind === "connection") return count === 1 ? "Conexão" : "Conexões";
  if (kind === "cto") return count === 1 ? "CTO" : "CTOs";
  if (kind === "splice_box") return count === 1 ? "Caixa de emenda" : "Caixas de emenda";
  if (kind === "cable") return count === 1 ? "Cabo" : "Cabos";
  if (kind === "pole") return count === 1 ? "Poste" : "Postes";
  if (kind === "project") return count === 1 ? "Projeto" : "Projetos";
  return count === 1 ? kind : `${kind} (${count})`;
}

function gridClusters(points: MapPoint[], decimals: number): { key: string; kind: string; members: MapPoint[]; lat: number; lng: number }[] {
  const f = 10 ** decimals;
  const m = new Map<string, MapPoint[]>();
  const kinds = new Map<string, string>();
  for (const p of points) {
    const gx = Math.round(p.lat * f) / f;
    const gy = Math.round(p.lng * f) / f;
    const kind = pointClusterKind(p);
    const key = `${gx.toFixed(decimals)},${gy.toFixed(decimals)}|${kind}`;
    const arr = m.get(key);
    if (arr) arr.push(p);
    else m.set(key, [p]);
    kinds.set(key, kind);
  }
  const out: { key: string; kind: string; members: MapPoint[]; lat: number; lng: number }[] = [];
  for (const [key, members] of m) {
    const lat = members.reduce((s, x) => s + x.lat, 0) / members.length;
    const lng = members.reduce((s, x) => s + x.lng, 0) / members.length;
    out.push({ key, kind: kinds.get(key) ?? "equipamento", members, lat, lng });
  }
  return out;
}

function centroid(pts: MapPoint[]): [number, number] {
  const lat = pts.reduce((s, p) => s + p.lat, 0) / pts.length;
  const lng = pts.reduce((s, p) => s + p.lng, 0) / pts.length;
  return [lat, lng];
}

function MapInvalidateSize() {
  const map = useMap();
  useEffect(() => {
    const run = () => {
      map.invalidateSize();
    };
    run();
    const t = window.setTimeout(run, 120);
    const onResize = () => run();
    window.addEventListener("resize", onResize);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("resize", onResize);
    };
  }, [map]);
  return null;
}

/** Só reage a `version` e lê sempre a lista actual via ref — evita fitBounds a cada render (que anulava o «voar» para o equipamento). */
function FitBounds({ pointsRef, version }: { pointsRef: MutableRefObject<{ lat: number; lng: number }[]>; version: number }) {
  const map = useMap();
  useEffect(() => {
    const pts = pointsRef.current;
    if (!pts.length) return;
    if (pts.length === 1) {
      const p = pts[0];
      map.setView([p.lat, p.lng], SINGLE_POINT_ZOOM, { animate: false });
      return;
    }
    const b = L.latLngBounds(pts.map((p) => [p.lat, p.lng] as [number, number]));
    if (b.isValid()) {
      map.fitBounds(b, { padding: FIT_PADDING, maxZoom: FIT_MAX_ZOOM, animate: false });
    }
  }, [map, version]);
  return null;
}

export type MapBounds = { minLat: number; maxLat: number; minLng: number; maxLng: number; zoom?: number };

function MapBoundsReporter({ onBoundsChange }: { onBoundsChange?: (b: MapBounds) => void }) {
  const map = useMap();
  useEffect(() => {
    if (!onBoundsChange) return;
    let timer: number | null = null;
    const emit = () => {
      const b = map.getBounds();
      onBoundsChange({
        minLat: b.getSouth(),
        maxLat: b.getNorth(),
        minLng: b.getWest(),
        maxLng: b.getEast(),
        zoom: map.getZoom(),
      });
    };
    const schedule = () => {
      if (timer != null) window.clearTimeout(timer);
      timer = window.setTimeout(emit, 280);
    };
    map.whenReady(emit);
    map.on("moveend", schedule);
    map.on("zoomend", schedule);
    return () => {
      if (timer != null) window.clearTimeout(timer);
      map.off("moveend", schedule);
      map.off("zoomend", schedule);
    };
  }, [map, onBoundsChange]);
  return null;
}

function MapFlyTo({ target, flyKey }: { target: { lat: number; lng: number; zoom?: number } | null; flyKey: number }) {
  const map = useMap();
  useEffect(() => {
    if (!target || flyKey <= 0) return;
    map.setView([target.lat, target.lng], target.zoom ?? 16, { animate: true });
  }, [map, target, flyKey]);
  return null;
}

function CloseSpiderOnMapClick({ active, onClose }: { active: boolean; onClose: () => void }) {
  const map = useMap();
  useEffect(() => {
    if (!active) return;
    const h = () => onClose();
    map.on("click", h);
    return () => {
      map.off("click", h);
    };
  }, [map, active, onClose]);
  return null;
}

function devicePopupBody(p: MapPoint, displayMode: MapDisplayMode) {
  return (
    <>
      <strong>{p.description}</strong>
      <div style={{ fontSize: 12 }}>
        {p.category} · {p.status}
        {displayMode === "status" ? <span style={{ color: "var(--muted)" }}> · Vista online/offline</span> : null}
        {p.ip ? <div className="mono">{p.ip}</div> : null}
      </div>
    </>
  );
}

function dominantStatus(members: MapPoint[]): "online" | "offline" | "unknown" {
  const on = members.filter((m) => m.status === "online").length;
  const off = members.filter((m) => m.status === "offline").length;
  if (on >= off && on > 0) return "online";
  if (off > 0) return "offline";
  return "unknown";
}

const iconCache = new Map<string, L.DivIcon>();

function equipmentPinIcon(color: string): L.DivIcon {
  const key = `eq:${color}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const fill = color;
  const stroke = "rgba(0,0,0,0.32)";
  const html = `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="38" viewBox="0 0 30 38" aria-hidden="true"><path fill="${fill}" stroke="${stroke}" stroke-width="1" d="M15 2C8.4 2 3 7.3 3 13.8c0 6.2 9.8 16.5 11.6 18.4L15 36l0.4-3.8C17.2 30.3 27 20 27 13.8 27 7.3 21.6 2 15 2z"/><circle cx="15" cy="14" r="4.2" fill="#fff" opacity="0.95"/></svg>`;
  const icon = L.divIcon({
    className: "map-equip-pin-wrap",
    html,
    iconSize: [30, 38],
    iconAnchor: [15, 36],
    popupAnchor: [0, -34],
  });
  iconCache.set(key, icon);
  return icon;
}

/** Marcador de login/conexão — círculo com ícone de utilizador. */
function connectionPinIcon(color: string): L.DivIcon {
  const key = `conn:${color}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const fill = color;
  const stroke = "rgba(0,0,0,0.28)";
  const html = `<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 28 28" aria-hidden="true"><circle cx="14" cy="14" r="12" fill="${fill}" stroke="${stroke}" stroke-width="1.2"/><circle cx="14" cy="11" r="4" fill="#fff" opacity="0.95"/><path d="M7 22c0-3.9 3.1-7 7-7s7 3.1 7 7" fill="#fff" opacity="0.95"/></svg>`;
  const icon = L.divIcon({
    className: "map-conn-pin-wrap",
    html,
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -14],
  });
  iconCache.set(key, icon);
  return icon;
}

function clusterBadgeIcon(count: number, color: string, kind: string, isConnection: boolean): L.DivIcon {
  const key = `badge:${count}:${color}:${kind}:${isConnection}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const badge = count > 1 ? `<span style="position:absolute;top:-6px;right:-8px;min-width:18px;height:18px;padding:0 5px;border-radius:9px;background:#0f172a;color:#fff;font:700 11px/18px system-ui,sans-serif;text-align:center;box-shadow:0 1px 4px rgba(0,0,0,.35)">${count > 999 ? "999+" : count}</span>` : "";
  const stroke = "rgba(0,0,0,0.32)";
  const inner = isConnection
    ? `<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 28 28"><circle cx="14" cy="14" r="12" fill="${color}" stroke="${stroke}" stroke-width="1.2"/><circle cx="14" cy="11" r="4" fill="#fff" opacity="0.95"/><path d="M7 22c0-3.9 3.1-7 7-7s7 3.1 7 7" fill="#fff" opacity="0.95"/></svg>`
    : `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="38" viewBox="0 0 30 38"><path fill="${color}" stroke="${stroke}" stroke-width="1" d="M15 2C8.4 2 3 7.3 3 13.8c0 6.2 9.8 16.5 11.6 18.4L15 36l0.4-3.8C17.2 30.3 27 20 27 13.8 27 7.3 21.6 2 15 2z"/><circle cx="15" cy="14" r="4.2" fill="#fff" opacity="0.95"/></svg>`;
  const html = `<div style="position:relative;display:inline-block;line-height:0">${inner}${badge}</div>`;
  const icon = L.divIcon({
    className: "map-cluster-badge-wrap",
    html,
    iconSize: isConnection ? [28, 28] : [30, 38],
    iconAnchor: isConnection ? [14, 14] : [15, 36],
    popupAnchor: isConnection ? [0, -14] : [0, -34],
  });
  iconCache.set(key, icon);
  return icon;
}

function clusterBadgeInfraIcon(count: number, kind: InfraMapKind, color?: string | null): L.DivIcon {
  const key = `infra-badge:${count}:${kind}:${color ?? ""}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const badge =
    count > 1
      ? `<span style="position:absolute;top:-6px;right:-8px;min-width:18px;height:18px;padding:0 5px;border-radius:9px;background:#0f172a;color:#fff;font:700 11px/18px system-ui,sans-serif;text-align:center;box-shadow:0 1px 4px rgba(0,0,0,.35)">${count > 999 ? "999+" : count}</span>`
      : "";
  const innerPin = infrastructurePinIcon(kind, color).options.html ?? "";
  const html = `<div style="position:relative;display:inline-block;line-height:0">${innerPin}${badge}</div>`;
  const icon = L.divIcon({
    className: "map-cluster-badge-wrap",
    html,
    iconSize: [26, 26],
    iconAnchor: [13, 13],
    popupAnchor: [0, -13],
  });
  iconCache.set(key, icon);
  return icon;
}

/** Pin em forma de gota (SVG), verde / vermelho / cinza — modo «estado». */
function statusPinIcon(status: string): L.DivIcon {
  const st = status === "online" ? "online" : status === "offline" ? "offline" : "unknown";
  const fill = st === "online" ? "#22c55e" : st === "offline" ? "#ef4444" : "#94a3b8";
  const stroke = "rgba(0,0,0,0.32)";
  const html = `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="38" viewBox="0 0 30 38" aria-hidden="true"><path fill="${fill}" stroke="${stroke}" stroke-width="1" d="M15 2C8.4 2 3 7.3 3 13.8c0 6.2 9.8 16.5 11.6 18.4L15 36l0.4-3.8C17.2 30.3 27 20 27 13.8 27 7.3 21.6 2 15 2z"/><circle cx="15" cy="14" r="4.2" fill="#fff" opacity="0.95"/></svg>`;
  return L.divIcon({
    className: "map-status-pin-wrap",
    html,
    iconSize: [30, 38],
    iconAnchor: [15, 36],
    popupAnchor: [0, -34],
  });
}

/**
 * Nunca passar `icon={undefined}` ao Marker: o Leaflet faz `options.icon = undefined` e apaga o Icon.Default
 * (origem do erro «Cannot read properties of undefined (reading 'createIcon')» com react-leaflet).
 */
function isConnectionPoint(p: MapPoint): boolean {
  return p.status === "connection" || p.mapKind === "connection";
}

function isInfrastructurePoint(p: MapPoint): boolean {
  return !!p.mapKind && isInfraMapKind(p.mapKind);
}

function highlightAccent(p: MapPoint, colors: MapColors): string {
  if (p.markerColor?.trim()) return p.markerColor.trim();
  if (isInfrastructurePoint(p) && p.mapKind && isInfraMapKind(p.mapKind)) {
    return p.mapKind === "cto" ? "#7c3aed" : "#2563eb";
  }
  if (isConnectionPoint(p)) return colors.connection;
  return colors.equipment;
}

function withMapPinHighlight(icon: L.DivIcon, highlighted: boolean, accent: string): L.DivIcon {
  if (!highlighted) return icon;
  const inner = icon.options.html ?? "";
  const html = `<div class="map-pin-highlight-inner" style="--map-highlight:${accent}">${inner}</div>`;
  return L.divIcon({
    className: `${icon.options.className ?? ""} map-pin-highlight-wrap`.trim(),
    html,
    iconSize: icon.options.iconSize,
    iconAnchor: icon.options.iconAnchor,
    popupAnchor: icon.options.popupAnchor,
  });
}

function markerIconOpts(
  displayMode: MapDisplayMode,
  p: MapPoint,
  colors: MapColors,
  highlightedId?: string | null,
): { icon: L.Icon | L.DivIcon } {
  const highlighted = !!highlightedId && p.id === highlightedId;
  let icon: L.DivIcon;
  if (isInfrastructurePoint(p) && p.mapKind && isInfraMapKind(p.mapKind)) {
    icon = infrastructurePinIcon(p.mapKind, p.markerColor, p.mapKind === "cto" ? p.mapLabel : null);
  } else if (isConnectionPoint(p)) {
    icon = connectionPinIcon(colors.connection);
  } else if (displayMode !== "status") {
    icon = equipmentPinIcon(colors.equipment);
  } else {
    icon = statusPinIcon(p.status);
  }
  return { icon: withMapPinHighlight(icon, highlighted, highlightAccent(p, colors)) };
}

function markerIconOptsGroup(
  displayMode: MapDisplayMode,
  members: MapPoint[],
  colors: MapColors,
  clusterKind?: string,
  highlightedId?: string | null,
): { icon: L.Icon | L.DivIcon } {
  const isConn = members.length > 0 && members.every(isConnectionPoint);
  const infraKind = members.length > 0 && members.every((m) => m.mapKind === members[0].mapKind && isInfrastructurePoint(m))
    ? members[0].mapKind
    : null;
  if (members.length > 1 && clusterKind) {
    if (infraKind && isInfraMapKind(infraKind)) {
      const color = members[0].markerColor;
      return { icon: clusterBadgeInfraIcon(members.length, infraKind, color) };
    }
    const color = isConn ? colors.connection : colors.equipment;
    return { icon: clusterBadgeIcon(members.length, color, clusterKind, isConn) };
  }
  const single = members[0];
  const highlighted = !!highlightedId && members.length === 1 && single.id === highlightedId;
  if (infraKind && isInfraMapKind(infraKind)) {
    const label = infraKind === "cto" && members.length === 1 ? members[0].mapLabel : null;
    const icon = infrastructurePinIcon(infraKind, members[0].markerColor, label);
    return { icon: withMapPinHighlight(icon, highlighted, highlightAccent(single, colors)) };
  }
  if (isConn) {
    const icon = connectionPinIcon(colors.connection);
    return { icon: withMapPinHighlight(icon, highlighted, colors.connection) };
  }
  if (displayMode !== "status") {
    const icon = equipmentPinIcon(colors.equipment);
    return { icon: withMapPinHighlight(icon, highlighted, colors.equipment) };
  }
  const icon = statusPinIcon(dominantStatus(members));
  return { icon: withMapPinHighlight(icon, highlighted, colors.equipment) };
}

function mapMarkerProps(
  p: MapPoint,
  displayMode: MapDisplayMode,
  colors: MapColors,
  highlightedId?: string | null,
) {
  const highlighted = !!highlightedId && p.id === highlightedId;
  return {
    ...markerIconOpts(displayMode, p, colors, highlightedId),
    zIndexOffset: highlighted ? 1200 : 0,
  };
}

type SpiderState = { key: string; members: MapPoint[]; center: [number, number]; phase: number } | null;

type ClusterCell = { key: string; kind: string; members: MapPoint[]; lat: number; lng: number };

function ClusterCellMarkers({
  c,
  expanded,
  onExpandCluster,
  spider,
  setSpider,
  spiderRef,
  runSpiderOpen,
  stopSpiderAnim,
  onSelectDevice,
  displayMode,
  colors,
  highlightedId,
}: {
  c: ClusterCell;
  expanded: Set<string>;
  onExpandCluster: (key: string) => void;
  spider: SpiderState;
  setSpider: (s: SpiderState) => void;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  onSelectDevice?: (id: string) => void;
  displayMode: MapDisplayMode;
  colors: MapColors;
  highlightedId?: string | null;
}) {
  const map = useMap();

  if (c.members.length === 1) {
    const p = c.members[0];
    return (
      <Marker position={[p.lat, p.lng]} {...mapMarkerProps(p, displayMode, colors, highlightedId)}>
        <Popup>
          {devicePopupBody(p, displayMode)}
          {onSelectDevice && (
            <button type="button" className="btn" style={{ marginTop: 6 }} onClick={() => onSelectDevice(p.id)}>
              Ver detalhe
            </button>
          )}
        </Popup>
      </Marker>
    );
  }

  if (!expanded.has(c.key)) {
    return (
      <Marker
        position={[c.lat, c.lng]}
        {...markerIconOptsGroup(displayMode, c.members, colors, c.kind, highlightedId)}
        zIndexOffset={c.members.some((m) => m.id === highlightedId) ? 1200 : 0}
        eventHandlers={{
          click: (e) => {
            L.DomEvent.stopPropagation(e);
            const b = L.latLngBounds(c.members.map((m) => [m.lat, m.lng] as [number, number]));
            if (b.isValid()) {
              map.fitBounds(b, { padding: FIT_PADDING, maxZoom: CLUSTER_EXPAND_MAX_ZOOM, animate: true });
            }
            onExpandCluster(c.key);
          },
        }}
      >
        <Popup>
          <strong>{c.members.length} {clusterKindLabel(c.kind, c.members.length).toLowerCase()}</strong>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 6px" }}>Aproxime o mapa ou clique no pin para separar os pontos.</p>
          <ul style={{ margin: "6px 0 0", paddingLeft: 18, fontSize: 12, maxHeight: 200, overflow: "auto" }}>
            {c.members.map((m) => (
              <li key={m.id}>
                {m.description}{" "}
                {onSelectDevice && (
                  <button type="button" className="btn" style={{ marginLeft: 4, padding: "2px 6px", fontSize: 11 }} onClick={() => onSelectDevice(m.id)}>
                    Detalhe
                  </button>
                )}
              </li>
            ))}
          </ul>
        </Popup>
      </Marker>
    );
  }

  const stacks = mergeProximityStacks(c.members, STACK_MERGE_M);
  return (
    <>
      {stacks.map((grp, idx) => {
        const sk = `c-${c.key}-sub-${idx}`;
        const isSpider = spider?.key === sk;
        if (grp.length === 1) {
          const p = grp[0];
          return (
            <Marker key={p.id} position={[p.lat, p.lng]} {...mapMarkerProps(p, displayMode, colors, highlightedId)}>
              <Popup>
                {devicePopupBody(p, displayMode)}
                {onSelectDevice && (
                  <button type="button" className="btn" style={{ marginTop: 6 }} onClick={() => onSelectDevice(p.id)}>
                    Ver detalhe
                  </button>
                )}
              </Popup>
            </Marker>
          );
        }
        if (isSpider && spider) {
          return spider.members.map((m, i) => {
            const [plat, plng] = ringOffsetLatLng(spider.center[0], spider.center[1], i, spider.members.length, SPIDER_RADIUS_M * spider.phase);
            return (
              <Marker key={`${sk}-${m.id}`} position={[plat, plng]} {...mapMarkerProps(m, displayMode, colors, highlightedId)}>
                <Popup>
                  {devicePopupBody(m, displayMode)}
                  {onSelectDevice && (
                    <button type="button" className="btn" style={{ marginTop: 6 }} onClick={() => onSelectDevice(m.id)}>
                      Ver detalhe
                    </button>
                  )}
                </Popup>
              </Marker>
            );
          });
        }
        const [clat, clng] = centroid(grp);
        return (
          <Marker
            key={sk}
            position={[clat, clng]}
            {...markerIconOptsGroup(displayMode, grp, colors, pointClusterKind(grp[0]), highlightedId)}
            zIndexOffset={grp.some((m) => m.id === highlightedId) ? 1200 : 0}
            eventHandlers={{
              click: (e) => {
                L.DomEvent.stopPropagation(e);
                const cur = spiderRef.current;
                if (cur?.key === sk && cur.phase >= 0.995) {
                  stopSpiderAnim();
                  setSpider(null);
                  return;
                }
                runSpiderOpen(sk, grp, [clat, clng]);
              },
            }}
          >
            <Popup>
              <strong>{grp.length} no mesmo local</strong>
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0" }}>Clique no pin para expandir.</p>
              <ul style={{ margin: 0, paddingLeft: 18, fontSize: 12 }}>
                {grp.map((m) => (
                  <li key={m.id}>{m.description}</li>
                ))}
              </ul>
            </Popup>
          </Marker>
        );
      })}
    </>
  );
}

/** Modo agrupado: grelha de agregação depende do nível de zoom actual. */
function ClusterMarkersByView({
  points,
  displayMode,
  onSelectDevice,
  spider,
  setSpider,
  spiderRef,
  runSpiderOpen,
  stopSpiderAnim,
  colors,
  highlightedId,
}: {
  points: MapPoint[];
  displayMode: MapDisplayMode;
  onSelectDevice?: (id: string) => void;
  spider: SpiderState;
  setSpider: (s: SpiderState) => void;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  colors: MapColors;
  highlightedId?: string | null;
}) {
  const map = useMap();
  const [zoom, setZoom] = useState(() => map.getZoom());
  const decimals = gridDecimalsForZoom(zoom);
  const pointsFp = useMemo(() => pointsFingerprint(points), [points]);

  useEffect(() => {
    const bump = () => setZoom(map.getZoom());
    bump();
    map.whenReady(bump);
    map.on("zoomend", bump);
    return () => {
      map.off("zoomend", bump);
    };
  }, [map]);

  const [expandedClusterKeys, setExpandedClusterKeys] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    setExpandedClusterKeys(new Set());
    setSpider(null);
    stopSpiderAnim();
  }, [pointsFp, decimals, stopSpiderAnim, setSpider]);

  const clustersGrid = useMemo(() => gridClusters(points, decimals), [points, decimals]);

  const expandCluster = useCallback((key: string) => {
    setExpandedClusterKeys((prev) => new Set(prev).add(key));
  }, []);

  return (
    <>
      {clustersGrid.map((c) => (
        <ClusterCellMarkers
          key={`${c.key}-${decimals}`}
          c={c}
          expanded={expandedClusterKeys}
          onExpandCluster={expandCluster}
          spider={spider}
          setSpider={setSpider}
          spiderRef={spiderRef}
          runSpiderOpen={runSpiderOpen}
          stopSpiderAnim={stopSpiderAnim}
          onSelectDevice={onSelectDevice}
          displayMode={displayMode}
          colors={colors}
          highlightedId={highlightedId}
        />
      ))}
    </>
  );
}

function ScatterStackMarker({
  sk,
  grp,
  displayMode,
  spiderRef,
  runSpiderOpen,
  stopSpiderAnim,
  setSpider,
  colors,
  highlightedId,
}: {
  sk: string;
  grp: MapPoint[];
  displayMode: MapDisplayMode;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  setSpider: (s: SpiderState) => void;
  colors: MapColors;
  highlightedId?: string | null;
}) {
  const map = useMap();
  const [clat, clng] = centroid(grp);
  return (
    <Marker
      position={[clat, clng]}
      {...markerIconOptsGroup(displayMode, grp, colors, pointClusterKind(grp[0]), highlightedId)}
      zIndexOffset={grp.some((m) => m.id === highlightedId) ? 1200 : 0}
      eventHandlers={{
        click: (e) => {
          L.DomEvent.stopPropagation(e);
          const cur = spiderRef.current;
          if (cur?.key === sk && cur.phase >= 0.995) {
            stopSpiderAnim();
            setSpider(null);
            return;
          }
          const b = L.latLngBounds(grp.map((m) => [m.lat, m.lng] as [number, number]));
          if (b.isValid()) {
            map.fitBounds(b, { padding: FIT_PADDING, maxZoom: CLUSTER_EXPAND_MAX_ZOOM, animate: true });
          }
          runSpiderOpen(sk, grp, [clat, clng]);
        },
      }}
    >
      <Popup>
        <strong>{grp.length} no mesmo local</strong>
        <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0" }}>Clique no pin para aproximar e expandir.</p>
        <ul style={{ margin: 0, paddingLeft: 18, fontSize: 12 }}>
          {grp.map((m) => (
            <li key={m.id}>{m.description}</li>
          ))}
        </ul>
      </Popup>
    </Marker>
  );
}

function ScatterMarkersLayer({
  stacks,
  displayMode,
  spider,
  setSpider,
  spiderRef,
  runSpiderOpen,
  stopSpiderAnim,
  onSelectDevice,
  colors,
  keyPrefix,
  highlightedId,
}: {
  stacks: MapPoint[][];
  displayMode: MapDisplayMode;
  spider: SpiderState;
  setSpider: (s: SpiderState) => void;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  onSelectDevice?: (id: string) => void;
  colors: MapColors;
  keyPrefix: string;
  highlightedId?: string | null;
}) {
  return (
    <>
      {stacks.map((grp, idx) => {
        const sk = `${keyPrefix}-${idx}-${grp.map((g) => g.id).join(",")}`;
        const isSpider = spider?.key === sk;

        if (grp.length === 1) {
          const p = grp[0];
          return (
            <Marker key={p.id} position={[p.lat, p.lng]} {...mapMarkerProps(p, displayMode, colors, highlightedId)}>
              <Popup>
                {devicePopupBody(p, displayMode)}
                {onSelectDevice && (
                  <button type="button" className="btn" style={{ marginTop: 6 }} onClick={() => onSelectDevice(p.id)}>
                    Ver detalhe
                  </button>
                )}
              </Popup>
            </Marker>
          );
        }

        if (isSpider && spider) {
          return spider.members.map((m, i) => {
            const [plat, plng] = ringOffsetLatLng(spider.center[0], spider.center[1], i, spider.members.length, SPIDER_RADIUS_M * spider.phase);
            return (
              <Marker key={`${sk}-${m.id}`} position={[plat, plng]} {...mapMarkerProps(m, displayMode, colors, highlightedId)}>
                <Popup>
                  {devicePopupBody(m, displayMode)}
                  {onSelectDevice && (
                    <button type="button" className="btn" style={{ marginTop: 6 }} onClick={() => onSelectDevice(m.id)}>
                      Ver detalhe
                    </button>
                  )}
                </Popup>
              </Marker>
            );
          });
        }

        return (
          <ScatterStackMarker
            key={sk}
            sk={sk}
            grp={grp}
            displayMode={displayMode}
            spiderRef={spiderRef}
            runSpiderOpen={runSpiderOpen}
            stopSpiderAnim={stopSpiderAnim}
            setSpider={setSpider}
            colors={colors}
            highlightedId={highlightedId}
          />
        );
      })}
    </>
  );
}

export function EquipmentMap({
  points,
  displayMode,
  onSelectDevice,
  flyTo,
  flyKey,
  fitBoundsVersion,
  onBoundsChange,
  mapColors,
  connectionClusterForced = false,
  mapHeight = 480,
  highlightedId = null,
}: {
  points: MapPoint[];
  displayMode: MapDisplayMode;
  onSelectDevice?: (id: string) => void;
  flyTo: { lat: number; lng: number; zoom?: number } | null;
  flyKey: number;
  fitBoundsVersion: number;
  onBoundsChange?: (b: MapBounds) => void;
  mapColors?: MapColors;
  /** Mantém conexões agrupadas mesmo em vista desagrupada (desempenho com milhares de logins). */
  connectionClusterForced?: boolean;
  mapHeight?: number | string;
  /** Pin seleccionado (ex.: resultado da pesquisa) com destaque visual. */
  highlightedId?: string | null;
}) {
  const colors = mapColors ?? DEFAULT_MAP_COLORS;
  const valid = useMemo(() => points.filter((p) => Number.isFinite(p.lat) && Number.isFinite(p.lng)), [points]);
  const equipValid = useMemo(() => valid.filter((p) => !isConnectionPoint(p)), [valid]);
  const connValid = useMemo(() => valid.filter(isConnectionPoint), [valid]);
  const center: [number, number] = valid.length ? [valid[0].lat, valid[0].lng] : [-14.235, -51.9253];

  const [spider, setSpider] = useState<SpiderState>(null);
  const spiderRef = useRef(spider);
  spiderRef.current = spider;
  const rafRef = useRef<number | null>(null);

  const pointsFp = useMemo(() => pointsFingerprint(valid), [valid]);

  useEffect(() => {
    setSpider(null);
  }, [pointsFp, displayMode]);

  const stopSpiderAnim = useCallback(() => {
    if (rafRef.current != null) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
  }, []);

  const runSpiderOpen = useCallback(
    (key: string, members: MapPoint[], center: [number, number]) => {
      stopSpiderAnim();
      setSpider({ key, members, center, phase: 0 });
      const t0 = performance.now();
      const dur = 520;
      const tick = (now: number) => {
        const t = Math.min(1, (now - t0) / dur);
        const ph = t < 1 ? easeOutCubic(t) : 1;
        setSpider({ key, members, center, phase: ph });
        if (t < 1) rafRef.current = requestAnimationFrame(tick);
        else rafRef.current = null;
      };
      rafRef.current = requestAnimationFrame(tick);
    },
    [stopSpiderAnim],
  );

  useEffect(() => {
    return () => stopSpiderAnim();
  }, [stopSpiderAnim]);

  const stacksScatterEquip = useMemo(
    () => (displayMode === "cluster" ? [] : mergeProximityStacks(equipValid, STACK_MERGE_M)),
    [equipValid, displayMode],
  );
  const stacksScatterConn = useMemo(
    () => (displayMode === "cluster" || connectionClusterForced ? [] : mergeProximityStacks(connValid, STACK_MERGE_M)),
    [connValid, displayMode, connectionClusterForced],
  );

  const fitPointsRef = useRef<{ lat: number; lng: number }[]>([]);
  fitPointsRef.current = valid.map((p) => ({ lat: p.lat, lng: p.lng }));

  if (valid.length === 0) {
    return <p style={{ color: "var(--muted)" }}>Sem coordenadas para mostrar no mapa.</p>;
  }

  return (
    <MapContainer
      center={center}
      zoom={6}
      style={{ height: mapHeight, width: "100%", minHeight: 420, borderRadius: "var(--radius)", border: "1px solid var(--border)" }}
      scrollWheelZoom
    >
      <MapInvalidateSize />
      <FitBounds pointsRef={fitPointsRef} version={fitBoundsVersion} />
      <MapBoundsReporter onBoundsChange={onBoundsChange} />
      <MapFlyTo target={flyTo} flyKey={flyKey} />
      <CloseSpiderOnMapClick active={!!spider && spider.phase >= 0.995} onClose={() => setSpider(null)} />
      <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a>' url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />

      {displayMode === "cluster" && (
        <ClusterMarkersByView
          points={valid}
          displayMode={displayMode}
          onSelectDevice={onSelectDevice}
          spider={spider}
          setSpider={setSpider}
          spiderRef={spiderRef}
          runSpiderOpen={runSpiderOpen}
          stopSpiderAnim={stopSpiderAnim}
          colors={colors}
          highlightedId={highlightedId}
        />
      )}

      {(displayMode === "scatter" || displayMode === "status") && (
        <>
          <ScatterMarkersLayer
            stacks={stacksScatterEquip}
            displayMode={displayMode}
            spider={spider}
            setSpider={setSpider}
            spiderRef={spiderRef}
            runSpiderOpen={runSpiderOpen}
            stopSpiderAnim={stopSpiderAnim}
            onSelectDevice={onSelectDevice}
            colors={colors}
            keyPrefix="eq"
            highlightedId={highlightedId}
          />
          {connectionClusterForced && connValid.length > 0 ? (
            <ClusterMarkersByView
              points={connValid}
              displayMode={displayMode}
              onSelectDevice={onSelectDevice}
              spider={spider}
              setSpider={setSpider}
              spiderRef={spiderRef}
              runSpiderOpen={runSpiderOpen}
              stopSpiderAnim={stopSpiderAnim}
              colors={colors}
              highlightedId={highlightedId}
            />
          ) : (
            <ScatterMarkersLayer
              stacks={stacksScatterConn}
              displayMode={displayMode}
              spider={spider}
              setSpider={setSpider}
              spiderRef={spiderRef}
              runSpiderOpen={runSpiderOpen}
              stopSpiderAnim={stopSpiderAnim}
              onSelectDevice={onSelectDevice}
              colors={colors}
              keyPrefix="conn"
              highlightedId={highlightedId}
            />
          )}
        </>
      )}
    </MapContainer>
  );
}