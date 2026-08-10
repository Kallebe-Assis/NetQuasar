import L from "leaflet";
import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject, type ReactNode } from "react";
import { CircleMarker, MapContainer, Marker, Polyline, Popup, TileLayer, useMap, useMapEvents } from "react-leaflet";
import "leaflet/dist/leaflet.css";
import { infrastructurePinIcon, isInfraMapKind, INFRA_MAP_KIND_LABELS, type InfraMapKind, type MapIconStyles, DEFAULT_MAP_ICON_STYLES } from "../lib/mapInfrastructureIcons";
import { buildPinSvg, pinLayout } from "../lib/mapPinStyles";

export type MapPointKind = "equipment" | "connection" | InfraMapKind;

export type MapLatLng = { lat: number; lng: number };

export type MapPlaceMode = "place" | "cable" | "reposition" | "edit-cable" | null;

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
  /** Splitter da CTO (ex.: 1x8), para popup. */
  splitter?: string | null;
  /** Trajeto do cabo (vários pontos). */
  path?: MapLatLng[] | null;
};

/** Agrupado (grelha por tipo), Desagrupado (marcadores individuais + empilhamento), Online/Offline (pins verde / vermelho / cinza). */
export type MapDisplayMode = "cluster" | "scatter" | "status";

export type MapColors = {
  equipment: string;
  connection: string;
  cto?: string;
  splice_box?: string;
};

export const DEFAULT_MAP_COLORS: MapColors = {
  equipment: "#3388ff",
  connection: "#3b82f6",
  cto: "#0D0663",
  splice_box: "#d97706",
};

const STACK_MERGE_M = 22;
const SPIDER_RADIUS_M = 34;
const FIT_PADDING: [number, number] = [48, 48];
const FIT_MAX_ZOOM = 16;
const SINGLE_POINT_ZOOM = 14;
const CLUSTER_EXPAND_MAX_ZOOM = 17;

/** Raio do spider cresce com o número de elementos empilhados. */
function spiderRadiusForCount(count: number): number {
  return SPIDER_RADIUS_M + Math.max(0, count - 3) * 8;
}

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
      timer = window.setTimeout(emit, 450);
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
  const isCto = p.mapKind === "cto";
  const isInfra = !!(p.mapKind && isInfraMapKind(p.mapKind));
  const splitter = (p.splitter ?? "").trim();
  const kindLabel = isCto
    ? splitter
      ? `CTO · ${splitter}`
      : "CTO"
    : isInfra
      ? INFRA_MAP_KIND_LABELS[p.mapKind as InfraMapKind]
      : p.category === p.status
        ? p.category
        : `${p.category} · ${p.status}`;
  return (
    <>
      <strong>{p.description}</strong>
      <div style={{ fontSize: 12, marginTop: 2 }}>
        <div>{kindLabel}</div>
        {displayMode === "status" ? <div style={{ color: "var(--muted)" }}>Vista online/offline</div> : null}
        {p.ip ? <div className="mono">{p.ip}</div> : null}
        <div className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
          {p.lat.toFixed(5)}, {p.lng.toFixed(5)}
        </div>
        {!isInfra && p.status ? (
          <div style={{ marginTop: 2 }}>
            <span className={`badge ${p.status === "online" ? "badge--ok" : p.status === "offline" ? "badge--err" : "badge--off"}`}>
              {p.status}
            </span>
          </div>
        ) : null}
      </div>
    </>
  );
}

function stackMemberMeta(p: MapPoint): string {
  const parts: string[] = [];
  if (p.mapKind && isInfraMapKind(p.mapKind)) {
    parts.push(INFRA_MAP_KIND_LABELS[p.mapKind as InfraMapKind]);
    if (p.mapKind === "cto" && (p.splitter ?? "").trim()) parts.push(String(p.splitter).trim());
  } else {
    if (p.category) parts.push(p.category);
    if (p.status && p.status !== p.category) parts.push(p.status);
  }
  if (p.ip) parts.push(p.ip);
  return parts.filter(Boolean).join(" · ");
}

/** Lista seleccionável quando vários elementos partilham as mesmas coordenadas. */
function StackMembersPopup({
  members,
  onSelectDevice,
  hint = "Clique num «Detalhe» ou no pin para separar no mapa.",
}: {
  members: MapPoint[];
  onSelectDevice?: (id: string) => void;
  hint?: string;
}) {
  return (
    <div className="map-stack-popup" onMouseDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()}>
      <strong>
        {members.length} no mesmo local
      </strong>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 8px" }}>{hint}</p>
      <ul className="map-stack-popup__list">
        {members.map((m) => (
          <li key={m.id} className="map-stack-popup__item">
            <div className="map-stack-popup__body">
              <div className="map-stack-popup__name">{m.description}</div>
              <div className="map-stack-popup__meta">{stackMemberMeta(m)}</div>
            </div>
            {onSelectDevice ? (
              <button
                type="button"
                className="btn btn--primary"
                style={{ padding: "4px 8px", fontSize: 11, flexShrink: 0 }}
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  onSelectDevice(m.id);
                }}
              >
                Detalhe
              </button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

function IconInfo() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 16v-4" />
      <path d="M12 8h.01" />
    </svg>
  );
}

function IconSplitter() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M22 12A10 10 0 1 1 12 2" />
      <path d="M22 2 12 12" />
      <path d="M16 2h6v6" />
    </svg>
  );
}

function IconEdit() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M10 3H8" />
      <path d="m15.007 5.008 3.987 3.986" />
      <path d="M20 15v4" />
      <path d="M21.174 6.813a2.82 2.82 0 0 0-3.986-3.987L3.842 16.175a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" />
      <path d="M22 17h-4" />
      <path d="M4 5v4" />
      <path d="M6 7H2" />
      <path d="M9 2v2" />
    </svg>
  );
}

function IconLocate() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <line x1="2" x2="5" y1="12" y2="12" />
      <line x1="19" x2="22" y1="12" y2="12" />
      <line x1="12" x2="12" y1="2" y2="5" />
      <line x1="12" x2="12" y1="19" y2="22" />
      <circle cx="12" cy="12" r="7" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function IconFibers() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M17 19a1 1 0 0 1-1-1v-2a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a1 1 0 0 1-1 1z" />
      <path d="M17 21v-2" />
      <path d="M19 14V6.5a1 1 0 0 0-7 0v11a1 1 0 0 1-7 0V10" />
      <path d="M21 21v-2" />
      <path d="M3 5V3" />
      <path d="M4 10a2 2 0 0 1-2-2V6a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2a2 2 0 0 1-2 2z" />
      <path d="M7 5V3" />
    </svg>
  );
}

function IconEmenda() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5" />
      <path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09" />
      <path d="M9 12a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.4 22.4 0 0 1-4 2z" />
      <path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 .05 5 .05" />
    </svg>
  );
}

function MapPopupIconBtn({
  title,
  onClick,
  children,
  primary,
}: {
  title: string;
  onClick: () => void;
  children: ReactNode;
  primary?: boolean;
}) {
  return (
    <button
      type="button"
      className={`map-popup-actions__btn${primary ? " map-popup-actions__btn--primary" : ""}`}
      title={title}
      aria-label={title}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onClick();
      }}
    >
      {children}
    </button>
  );
}

function MapPointPopupActions({
  p,
  onSelectDevice,
  onOpenSplitter,
  onOpenCableFibers,
  onOpenSplice,
  onEditPosition,
  onCopyCoords,
}: {
  p: MapPoint;
  onSelectDevice?: (id: string) => void;
  onOpenSplitter?: (id: string) => void;
  onOpenCableFibers?: (id: string) => void;
  onOpenSplice?: (id: string) => void;
  onEditPosition?: (id: string) => void;
  onCopyCoords?: (lat: number, lng: number) => void;
}) {
  const kind = p.mapKind;
  const isInfra = !!(kind && isInfraMapKind(kind));
  const isCto = kind === "cto";
  const isCable = kind === "cable";
  const isSplice = kind === "splice_box";
  const showSplitter = !!(onOpenSplitter && isCto);
  const showCableFibers = !!(onOpenCableFibers && isCable);
  const showSplice = !!(onOpenSplice && isSplice);
  const showEdit = !!(onEditPosition && isCto);
  const showCoords = !!(onCopyCoords && isInfra && Number.isFinite(p.lat) && Number.isFinite(p.lng));

  if (isInfra) {
    if (!onSelectDevice && !showSplitter && !showCableFibers && !showSplice && !showEdit && !showCoords) return null;
    return (
      <div className="map-popup-actions" role="group" aria-label="Acções do elemento">
        {onSelectDevice ? (
          <MapPopupIconBtn title="Detalhes" onClick={() => onSelectDevice(p.id)}>
            <IconInfo />
          </MapPopupIconBtn>
        ) : null}
        {showSplitter ? (
          <MapPopupIconBtn title="Splitter" primary onClick={() => onOpenSplitter!(p.id)}>
            <IconSplitter />
          </MapPopupIconBtn>
        ) : null}
        {showCableFibers ? (
          <MapPopupIconBtn title="Fibras" primary onClick={() => onOpenCableFibers!(p.id)}>
            <IconFibers />
          </MapPopupIconBtn>
        ) : null}
        {showSplice ? (
          <MapPopupIconBtn title="Emenda" primary onClick={() => onOpenSplice!(p.id)}>
            <IconEmenda />
          </MapPopupIconBtn>
        ) : null}
        {showEdit ? (
          <MapPopupIconBtn title="Editar" onClick={() => onEditPosition!(p.id)}>
            <IconEdit />
          </MapPopupIconBtn>
        ) : null}
        {showCoords ? (
          <MapPopupIconBtn title="Coordenadas" onClick={() => onCopyCoords!(p.lat, p.lng)}>
            <IconLocate />
          </MapPopupIconBtn>
        ) : null}
      </div>
    );
  }

  if (!onSelectDevice) return null;
  return (
    <div className="map-popup-actions" role="group" aria-label="Acções">
      <MapPopupIconBtn title="Detalhes" onClick={() => onSelectDevice(p.id)}>
        <IconInfo />
      </MapPopupIconBtn>
    </div>
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

function equipmentPinIcon(color: string, styleId = "pin"): L.DivIcon {
  const key = `eq:v2:${styleId}:${color}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const layout = pinLayout("equipment", styleId);
  const html = buildPinSvg("equipment", styleId, color, layout.size);
  const icon = L.divIcon({
    className: "map-equip-pin-wrap",
    html: `<div style="filter:drop-shadow(0 1px 2px rgba(0,0,0,.28));line-height:0">${html}</div>`,
    iconSize: layout.iconSize,
    iconAnchor: layout.iconAnchor,
    popupAnchor: layout.popupAnchor,
  });
  iconCache.set(key, icon);
  return icon;
}

/** Marcador de login/conexão. */
function connectionPinIcon(color: string, styleId = "user"): L.DivIcon {
  const key = `conn:v2:${styleId}:${color}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const layout = pinLayout("connection", styleId);
  const html = buildPinSvg("connection", styleId, color, layout.size);
  const icon = L.divIcon({
    className: "map-conn-pin-wrap",
    html: `<div style="filter:drop-shadow(0 1px 2px rgba(0,0,0,.28));line-height:0">${html}</div>`,
    iconSize: layout.iconSize,
    iconAnchor: layout.iconAnchor,
    popupAnchor: layout.popupAnchor,
  });
  iconCache.set(key, icon);
  return icon;
}

function clusterBadgeIcon(count: number, color: string, kind: string, isConnection: boolean, iconStyles: MapIconStyles): L.DivIcon {
  const styleId = isConnection ? iconStyles.connection : iconStyles.equipment;
  const key = `badge:v2:${count}:${color}:${kind}:${isConnection}:${styleId}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const badge = count > 1 ? `<span style="position:absolute;top:-6px;right:-8px;min-width:18px;height:18px;padding:0 5px;border-radius:9px;background:#0f172a;color:#fff;font:700 11px/18px system-ui,sans-serif;text-align:center;box-shadow:0 1px 4px rgba(0,0,0,.35)">${count > 999 ? "999+" : count}</span>` : "";
  const role = isConnection ? "connection" : "equipment";
  const layout = pinLayout(role, styleId);
  const inner = buildPinSvg(role, styleId, color, layout.size);
  const html = `<div style="position:relative;display:inline-block;line-height:0;filter:drop-shadow(0 1px 2px rgba(0,0,0,.28))">${inner}${badge}</div>`;
  const icon = L.divIcon({
    className: "map-cluster-badge-wrap",
    html,
    iconSize: layout.iconSize,
    iconAnchor: layout.iconAnchor,
    popupAnchor: layout.popupAnchor,
  });
  iconCache.set(key, icon);
  return icon;
}

function clusterBadgeInfraIcon(count: number, kind: InfraMapKind, color?: string | null, styleId?: string | null): L.DivIcon {
  const key = `infra-badge:v2:${count}:${kind}:${color ?? ""}:${styleId ?? ""}`;
  const cached = iconCache.get(key);
  if (cached) return cached;
  const badge =
    count > 1
      ? `<span style="position:absolute;top:-6px;right:-8px;min-width:18px;height:18px;padding:0 5px;border-radius:9px;background:#0f172a;color:#fff;font:700 11px/18px system-ui,sans-serif;text-align:center;box-shadow:0 1px 4px rgba(0,0,0,.35)">${count > 999 ? "999+" : count}</span>`
      : "";
  const innerPin = infrastructurePinIcon(kind, color, null, styleId).options.html ?? "";
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


function highlightSet(highlightedId?: string | string[] | null): Set<string> {
  if (!highlightedId) return new Set();
  if (Array.isArray(highlightedId)) return new Set(highlightedId.filter(Boolean));
  return new Set([highlightedId]);
}

function infraStyleFor(kind: InfraMapKind, styles: MapIconStyles): string | null {
  if (kind === "cto") return styles.cto;
  if (kind === "splice_box") return styles.splice_box;
  return null;
}

function markerIconOpts(
  displayMode: MapDisplayMode,
  p: MapPoint,
  colors: MapColors,
  iconStyles: MapIconStyles,
  highlightedId?: string | string[] | null,
): { icon: L.Icon | L.DivIcon } {
  const highlighted = highlightSet(highlightedId).has(p.id);
  let icon: L.DivIcon;
  if (isInfrastructurePoint(p) && p.mapKind && isInfraMapKind(p.mapKind)) {
    icon = infrastructurePinIcon(p.mapKind, p.markerColor, p.mapKind === "cto" ? p.mapLabel : null, infraStyleFor(p.mapKind, iconStyles));
  } else if (isConnectionPoint(p)) {
    icon = connectionPinIcon(colors.connection, iconStyles.connection);
  } else if (displayMode !== "status") {
    icon = equipmentPinIcon(colors.equipment, iconStyles.equipment);
  } else {
    icon = statusPinIcon(p.status);
  }
  return { icon: withMapPinHighlight(icon, highlighted, highlightAccent(p, colors)) };
}

function markerIconOptsGroup(
  displayMode: MapDisplayMode,
  members: MapPoint[],
  colors: MapColors,
  iconStyles: MapIconStyles,
  clusterKind?: string,
  highlightedId?: string | string[] | null,
): { icon: L.Icon | L.DivIcon } {
  const isConn = members.length > 0 && members.every(isConnectionPoint);
  const infraKind = members.length > 0 && members.every((m) => m.mapKind === members[0].mapKind && isInfrastructurePoint(m))
    ? members[0].mapKind
    : null;
  if (members.length > 1 && clusterKind) {
    if (infraKind && isInfraMapKind(infraKind)) {
      const color = members[0].markerColor;
      return { icon: clusterBadgeInfraIcon(members.length, infraKind, color, infraStyleFor(infraKind, iconStyles)) };
    }
    const color = isConn ? colors.connection : colors.equipment;
    return { icon: clusterBadgeIcon(members.length, color, clusterKind, isConn, iconStyles) };
  }
  const single = members[0];
  const highlighted = members.length === 1 && highlightSet(highlightedId).has(single.id);
  if (infraKind && isInfraMapKind(infraKind)) {
    const label = infraKind === "cto" && members.length === 1 ? members[0].mapLabel : null;
    const icon = infrastructurePinIcon(infraKind, members[0].markerColor, label, infraStyleFor(infraKind, iconStyles));
    return { icon: withMapPinHighlight(icon, highlighted, highlightAccent(single, colors)) };
  }
  if (isConn) {
    const icon = connectionPinIcon(colors.connection, iconStyles.connection);
    return { icon: withMapPinHighlight(icon, highlighted, colors.connection) };
  }
  if (displayMode !== "status") {
    const icon = equipmentPinIcon(colors.equipment, iconStyles.equipment);
    return { icon: withMapPinHighlight(icon, highlighted, colors.equipment) };
  }
  const icon = statusPinIcon(dominantStatus(members));
  return { icon: withMapPinHighlight(icon, highlighted, colors.equipment) };
}

function mapMarkerProps(
  p: MapPoint,
  displayMode: MapDisplayMode,
  colors: MapColors,
  iconStyles: MapIconStyles,
  highlightedId?: string | string[] | null,
) {
  const highlighted = highlightSet(highlightedId).has(p.id);
  return {
    ...markerIconOpts(displayMode, p, colors, iconStyles, highlightedId),
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
  onOpenSplitter,
  onOpenCableFibers,
  onOpenSplice,
  onEditPosition,
  onCopyCoords,
  displayMode,
  colors,
  iconStyles,
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
  onOpenSplitter?: (id: string) => void;
  onOpenCableFibers?: (id: string) => void;
  onOpenSplice?: (id: string) => void;
  onEditPosition?: (id: string) => void;
  onCopyCoords?: (lat: number, lng: number) => void;
  displayMode: MapDisplayMode;
  colors: MapColors;
  iconStyles: MapIconStyles;
  highlightedId?: string | string[] | null;
}) {
  const map = useMap();

  if (c.members.length === 1) {
    const p = c.members[0];
    return (
      <Marker position={[p.lat, p.lng]} {...mapMarkerProps(p, displayMode, colors, iconStyles, highlightedId)}>
        <Popup>
          {devicePopupBody(p, displayMode)}
          <MapPointPopupActions p={p} onSelectDevice={onSelectDevice} onOpenSplitter={onOpenSplitter} onOpenCableFibers={onOpenCableFibers} onOpenSplice={onOpenSplice} onEditPosition={onEditPosition} onCopyCoords={onCopyCoords} />
        </Popup>
      </Marker>
    );
  }

  if (!expanded.has(c.key)) {
    return (
      <Marker
        position={[c.lat, c.lng]}
        {...markerIconOptsGroup(displayMode, c.members, colors, iconStyles, c.kind, highlightedId)}
        zIndexOffset={c.members.some((m) => highlightSet(highlightedId).has(m.id)) ? 1200 : 0}
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
          <ul style={{ margin: "6px 0 0", paddingLeft: 0, listStyle: "none", fontSize: 12, maxHeight: 240, overflow: "auto" }}>
            {c.members.map((m) => (
              <li key={m.id} className="map-stack-popup__item" style={{ paddingLeft: 0 }}>
                <div className="map-stack-popup__body">
                  <div className="map-stack-popup__name">{m.description}</div>
                  <div className="map-stack-popup__meta">{stackMemberMeta(m)}</div>
                </div>
                {onSelectDevice && (
                  <button type="button" className="btn btn--primary" style={{ padding: "4px 8px", fontSize: 11 }} onClick={() => onSelectDevice(m.id)}>
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
            <Marker key={p.id} position={[p.lat, p.lng]} {...mapMarkerProps(p, displayMode, colors, iconStyles, highlightedId)}>
              <Popup>
                {devicePopupBody(p, displayMode)}
                <MapPointPopupActions p={p} onSelectDevice={onSelectDevice} onOpenSplitter={onOpenSplitter} onOpenCableFibers={onOpenCableFibers} onOpenSplice={onOpenSplice} onEditPosition={onEditPosition} onCopyCoords={onCopyCoords} />
              </Popup>
            </Marker>
          );
        }
        if (isSpider && spider) {
          const radius = spiderRadiusForCount(spider.members.length);
          return spider.members.map((m, i) => {
            const [plat, plng] = ringOffsetLatLng(spider.center[0], spider.center[1], i, spider.members.length, radius * spider.phase);
            return (
              <Marker key={`${sk}-${m.id}`} position={[plat, plng]} {...mapMarkerProps(m, displayMode, colors, iconStyles, highlightedId)}>
                <Popup>
                  {devicePopupBody(m, displayMode)}
                  <MapPointPopupActions p={m} onSelectDevice={onSelectDevice} onOpenSplitter={onOpenSplitter} onOpenCableFibers={onOpenCableFibers} onOpenSplice={onOpenSplice} onEditPosition={onEditPosition} onCopyCoords={onCopyCoords} />
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
            {...markerIconOptsGroup(displayMode, grp, colors, iconStyles, pointClusterKind(grp[0]), highlightedId)}
            zIndexOffset={grp.some((m) => highlightSet(highlightedId).has(m.id)) ? 1200 : 0}
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
              <StackMembersPopup members={grp} onSelectDevice={onSelectDevice} />
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
  onOpenSplitter,
  onOpenCableFibers,
  onOpenSplice,
  onEditPosition,
  onCopyCoords,
  spider,
  setSpider,
  spiderRef,
  runSpiderOpen,
  stopSpiderAnim,
  colors,
  iconStyles,
  highlightedId,
}: {
  points: MapPoint[];
  displayMode: MapDisplayMode;
  onSelectDevice?: (id: string) => void;
  onOpenSplitter?: (id: string) => void;
  onOpenCableFibers?: (id: string) => void;
  onOpenSplice?: (id: string) => void;
  onEditPosition?: (id: string) => void;
  onCopyCoords?: (lat: number, lng: number) => void;
  spider: SpiderState;
  setSpider: (s: SpiderState) => void;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  colors: MapColors;
  iconStyles: MapIconStyles;
  highlightedId?: string | string[] | null;
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
          onOpenSplitter={onOpenSplitter}
          onOpenCableFibers={onOpenCableFibers}
          onOpenSplice={onOpenSplice}
          onEditPosition={onEditPosition}
          onCopyCoords={onCopyCoords}
          displayMode={displayMode}
          colors={colors}
          iconStyles={iconStyles}
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
  iconStyles,
  highlightedId,
  onSelectDevice,
}: {
  sk: string;
  grp: MapPoint[];
  displayMode: MapDisplayMode;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  setSpider: (s: SpiderState) => void;
  colors: MapColors;
  iconStyles: MapIconStyles;
  highlightedId?: string | string[] | null;
  onSelectDevice?: (id: string) => void;
}) {
  const map = useMap();
  const [clat, clng] = centroid(grp);
  return (
    <Marker
      position={[clat, clng]}
      {...markerIconOptsGroup(displayMode, grp, colors, iconStyles, pointClusterKind(grp[0]), highlightedId)}
      zIndexOffset={grp.some((m) => highlightSet(highlightedId).has(m.id)) ? 1200 : 0}
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
        <StackMembersPopup members={grp} onSelectDevice={onSelectDevice} hint="Clique num «Detalhe» ou no pin para aproximar e separar." />
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
  onOpenSplitter,
  onOpenCableFibers,
  onOpenSplice,
  onEditPosition,
  onCopyCoords,
  colors,
  iconStyles,
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
  onOpenSplitter?: (id: string) => void;
  onOpenCableFibers?: (id: string) => void;
  onOpenSplice?: (id: string) => void;
  onEditPosition?: (id: string) => void;
  onCopyCoords?: (lat: number, lng: number) => void;
  colors: MapColors;
  iconStyles: MapIconStyles;
  keyPrefix: string;
  highlightedId?: string | string[] | null;
}) {
  return (
    <>
      {stacks.map((grp, idx) => {
        const sk = `${keyPrefix}-${idx}-${grp.map((g) => g.id).join(",")}`;
        const isSpider = spider?.key === sk;

        if (grp.length === 1) {
          const p = grp[0];
          return (
            <Marker key={p.id} position={[p.lat, p.lng]} {...mapMarkerProps(p, displayMode, colors, iconStyles, highlightedId)}>
              <Popup>
                {devicePopupBody(p, displayMode)}
                <MapPointPopupActions p={p} onSelectDevice={onSelectDevice} onOpenSplitter={onOpenSplitter} onOpenCableFibers={onOpenCableFibers} onOpenSplice={onOpenSplice} onEditPosition={onEditPosition} onCopyCoords={onCopyCoords} />
              </Popup>
            </Marker>
          );
        }

        if (isSpider && spider) {
          const radius = spiderRadiusForCount(spider.members.length);
          return spider.members.map((m, i) => {
            const [plat, plng] = ringOffsetLatLng(spider.center[0], spider.center[1], i, spider.members.length, radius * spider.phase);
            return (
              <Marker key={`${sk}-${m.id}`} position={[plat, plng]} {...mapMarkerProps(m, displayMode, colors, iconStyles, highlightedId)}>
                <Popup>
                  {devicePopupBody(m, displayMode)}
                  <MapPointPopupActions p={m} onSelectDevice={onSelectDevice} onOpenSplitter={onOpenSplitter} onOpenCableFibers={onOpenCableFibers} onOpenSplice={onOpenSplice} onEditPosition={onEditPosition} onCopyCoords={onCopyCoords} />
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
            iconStyles={iconStyles}
            highlightedId={highlightedId}
            onSelectDevice={onSelectDevice}
          />
        );
      })}
    </>
  );
}

function MapPlaceClickLayer({
  enabled,
  onMapClick,
}: {
  enabled: boolean;
  onMapClick?: (lat: number, lng: number) => void;
}) {
  useMapEvents({
    click(e) {
      if (!enabled || !onMapClick) return;
      onMapClick(e.latlng.lat, e.latlng.lng);
    },
  });
  return null;
}

function CablePathsLayer({
  points,
  editingCableId,
}: {
  points: MapPoint[];
  editingCableId?: string | null;
}) {
  const cables = useMemo(
    () => points.filter((p) => p.mapKind === "cable" && Array.isArray(p.path) && p.path.length >= 2),
    [points],
  );
  return (
    <>
      {cables.map((c) => {
        if (editingCableId && c.id === editingCableId) return null;
        return (
          <Polyline
            key={`cable-path-${c.id}`}
            positions={c.path!.map((pt) => [pt.lat, pt.lng] as [number, number])}
            pathOptions={{ color: "#0f766e", weight: 4, opacity: 0.85 }}
          />
        );
      })}
    </>
  );
}

function DraftCableLayer({
  draftPath,
  editable = false,
  onVertexMove,
}: {
  draftPath: MapLatLng[];
  editable?: boolean;
  onVertexMove?: (index: number, lat: number, lng: number) => void;
}) {
  const vertexIcon = useMemo(() => {
    const html =
      '<div style="width:14px;height:14px;border-radius:50%;background:#2563eb;border:2px solid #fff;box-shadow:0 1px 4px rgba(0,0,0,.35)"></div>';
    return L.divIcon({
      className: "map-cable-vertex",
      html,
      iconSize: [14, 14],
      iconAnchor: [7, 7],
    });
  }, []);

  if (draftPath.length === 0) return null;
  const positions = draftPath.map((p) => [p.lat, p.lng] as [number, number]);
  return (
    <>
      {draftPath.length >= 2 ? (
        <Polyline
          positions={positions}
          pathOptions={{
            color: editable ? "#2563eb" : "#ea580c",
            weight: 4,
            dashArray: editable ? undefined : "8 6",
            opacity: 0.95,
          }}
        />
      ) : null}
      {draftPath.map((p, i) =>
        editable && onVertexMove ? (
          <Marker
            key={`draft-edit-${i}`}
            position={[p.lat, p.lng]}
            icon={vertexIcon}
            draggable
            zIndexOffset={1800}
            eventHandlers={{
              dragend: (e) => {
                const ll = e.target.getLatLng();
                onVertexMove(i, ll.lat, ll.lng);
              },
            }}
          />
        ) : (
          <CircleMarker
            key={`draft-${i}`}
            center={[p.lat, p.lng]}
            radius={i === 0 || i === draftPath.length - 1 ? 7 : 5}
            pathOptions={{
              color: "#c2410c",
              fillColor: i === 0 ? "#16a34a" : i === draftPath.length - 1 ? "#ea580c" : "#fdba74",
              fillOpacity: 1,
              weight: 2,
            }}
          />
        ),
      )}
    </>
  );
}

function RepositionGhostMarker({
  position,
  onDragEnd,
}: {
  position: MapLatLng | null;
  onDragEnd?: (lat: number, lng: number) => void;
}) {
  const icon = useMemo(() => {
    const html =
      '<div style="width:22px;height:22px;border-radius:50%;background:#ea580c;border:3px solid #fff;box-shadow:0 2px 8px rgba(0,0,0,.35)"></div>';
    return L.divIcon({
      className: "map-reposition-ghost",
      html,
      iconSize: [22, 22],
      iconAnchor: [11, 11],
    });
  }, []);

  if (!position) return null;
  return (
    <Marker
      position={[position.lat, position.lng]}
      icon={icon}
      draggable
      zIndexOffset={1900}
      eventHandlers={{
        dragend: (e) => {
          const ll = e.target.getLatLng();
          onDragEnd?.(ll.lat, ll.lng);
        },
      }}
    >
      <Popup>Arraste para reposicionar ou clique no mapa.</Popup>
    </Marker>
  );
}

function userLocationIcon(): L.DivIcon {
  const key = "user-gps:v1";
  const cached = iconCache.get(key);
  if (cached) return cached;
  const html = `<div class="map-user-loc" aria-hidden="true"><span class="map-user-loc__pulse"></span><span class="map-user-loc__dot"></span></div>`;
  const icon = L.divIcon({
    className: "map-user-loc-wrap",
    html,
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -14],
  });
  iconCache.set(key, icon);
  return icon;
}

function UserLocationMarker({ location }: { location: MapLatLng | null | undefined }) {
  if (!location || !Number.isFinite(location.lat) || !Number.isFinite(location.lng)) return null;
  return (
    <Marker position={[location.lat, location.lng]} icon={userLocationIcon()} zIndexOffset={2000}>
      <Popup>
        <strong>A sua posição</strong>
        <div className="mono" style={{ fontSize: 12 }}>
          {location.lat.toFixed(6)}, {location.lng.toFixed(6)}
        </div>
      </Popup>
    </Marker>
  );
}

function locationSearchIcon(): L.DivIcon {
  const key = "loc-search-pin";
  const cached = iconCache.get(key);
  if (cached) return cached;
  const html = `<div style="width:18px;height:18px;border-radius:50%;background:#2563eb;border:3px solid #fff;box-shadow:0 1px 6px rgba(0,0,0,.35)"></div>`;
  const icon = L.divIcon({
    className: "map-locate-pin-wrap",
    html,
    iconSize: [18, 18],
    iconAnchor: [9, 9],
  });
  iconCache.set(key, icon);
  return icon;
}

function LocationSearchMarker({ pin }: { pin: { lat: number; lng: number; label?: string } | null | undefined }) {
  if (!pin || !Number.isFinite(pin.lat) || !Number.isFinite(pin.lng)) return null;
  return (
    <Marker position={[pin.lat, pin.lng]} icon={locationSearchIcon()} zIndexOffset={2100}>
      <Popup>
        <strong>{pin.label?.trim() || "Localização"}</strong>
        <div className="mono" style={{ fontSize: 12 }}>
          {pin.lat.toFixed(6)}, {pin.lng.toFixed(6)}
        </div>
      </Popup>
    </Marker>
  );
}

export function EquipmentMap({
  points,
  displayMode,
  onSelectDevice,
  onOpenSplitter,
  onOpenCableFibers,
  onOpenSplice,
  onEditPosition,
  onCopyCoords,
  flyTo,
  flyKey,
  fitBoundsVersion,
  onBoundsChange,
  mapColors,
  mapIconStyles,
  connectionClusterForced = false,
  mapHeight = 480,
  highlightedId = null,
  userLocation = null,
  locationPin = null,
  placeMode = null,
  draftPath = [],
  onMapClick,
  onDraftVertexMove,
  repositionPreview = null,
  editingCableMapId = null,
  mapEditMode = false,
}: {
  points: MapPoint[];
  displayMode: MapDisplayMode;
  onSelectDevice?: (id: string) => void;
  /** Abre o modal de splitter da CTO (popup do mapa). */
  onOpenSplitter?: (id: string) => void;
  /** Abre o modal de fibras do cabo (popup do mapa). */
  onOpenCableFibers?: (id: string) => void;
  /** Abre o modal de emenda (caixa de emenda). */
  onOpenSplice?: (id: string) => void;
  /** Abre o painel em modo edição de posição (CTO). */
  onEditPosition?: (id: string) => void;
  /** Copia coordenadas do ponto. */
  onCopyCoords?: (lat: number, lng: number) => void;
  flyTo: { lat: number; lng: number; zoom?: number } | null;
  flyKey: number;
  fitBoundsVersion: number;
  onBoundsChange?: (b: MapBounds) => void;
  mapColors?: MapColors;
  mapIconStyles?: MapIconStyles;
  /** Mantém conexões agrupadas mesmo em vista desagrupada (desempenho com milhares de logins). */
  connectionClusterForced?: boolean;
  mapHeight?: number | string;
  /** Pin seleccionado e/ou CTOs próximas (destaque visual). */
  highlightedId?: string | string[] | null;
  /** Posição GPS do técnico (marcador em tempo real). */
  userLocation?: MapLatLng | null;
  /** Resultado de pesquisa de endereço / coordenadas / URL do Maps. */
  locationPin?: { lat: number; lng: number; label?: string } | null;
  /** Modo de adicionar / editar no mapa (cursor + clique). */
  placeMode?: MapPlaceMode;
  /** Trajeto em construção ou edição do cabo. */
  draftPath?: MapLatLng[];
  onMapClick?: (lat: number, lng: number) => void;
  /** Arrastar vértice do trajeto em edição. */
  onDraftVertexMove?: (index: number, lat: number, lng: number) => void;
  /** Pré-visualização ao reposicionar um ponto. */
  repositionPreview?: MapLatLng | null;
  /** Map id do cabo cujo path está a ser editado (oculta o polyline original). */
  editingCableMapId?: string | null;
  /** Destaque visual do modo edição. */
  mapEditMode?: boolean;
}) {
  const colors = mapColors ?? DEFAULT_MAP_COLORS;
  const iconStyles = mapIconStyles ?? DEFAULT_MAP_ICON_STYLES;
  const valid = useMemo(() => points.filter((p) => Number.isFinite(p.lat) && Number.isFinite(p.lng)), [points]);
  const equipValid = useMemo(() => valid.filter((p) => !isConnectionPoint(p)), [valid]);
  const connValid = useMemo(() => valid.filter(isConnectionPoint), [valid]);
  const center: [number, number] = valid.length ? [valid[0].lat, valid[0].lng] : [-14.235, -51.9253];
  const placing =
    placeMode === "place" || placeMode === "cable" || placeMode === "reposition" || placeMode === "edit-cable";
  const selectHandler = placing ? undefined : onSelectDevice;
  const splitterHandler = placing ? undefined : onOpenSplitter;
  const cableFibersHandler = placing ? undefined : onOpenCableFibers;
  const spliceHandler = placing ? undefined : onOpenSplice;
  const editHandler = placing || !mapEditMode ? undefined : onEditPosition;
  const copyCoordsHandler = placing ? undefined : onCopyCoords;

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

  return (
    <div
      className={[
        placing
          ? placeMode === "cable" || placeMode === "edit-cable"
            ? "map-place-mode map-place-mode--cable"
            : placeMode === "reposition"
              ? "map-place-mode map-place-mode--reposition"
              : "map-place-mode map-place-mode--place"
          : undefined,
        mapEditMode ? "map-edit-mode" : undefined,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <MapContainer
        center={center}
        zoom={valid.length ? 6 : 5}
        style={{ height: mapHeight, width: "100%", minHeight: 420, borderRadius: "var(--radius)", border: "1px solid var(--border)" }}
        scrollWheelZoom
      >
        <MapInvalidateSize />
        {valid.length > 0 ? <FitBounds pointsRef={fitPointsRef} version={fitBoundsVersion} /> : null}
        <MapBoundsReporter onBoundsChange={onBoundsChange} />
        <MapFlyTo target={flyTo} flyKey={flyKey} />
        <CloseSpiderOnMapClick active={!placing && !!spider && spider.phase >= 0.995} onClose={() => setSpider(null)} />
        <MapPlaceClickLayer enabled={placing} onMapClick={onMapClick} />
        <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a>' url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />

        <CablePathsLayer points={valid} editingCableId={editingCableMapId} />
        <DraftCableLayer
          draftPath={draftPath}
          editable={placeMode === "edit-cable"}
          onVertexMove={onDraftVertexMove}
        />
        <RepositionGhostMarker
          position={placeMode === "reposition" ? repositionPreview : null}
          onDragEnd={onMapClick}
        />
        <UserLocationMarker location={userLocation} />
        <LocationSearchMarker pin={locationPin} />

        {displayMode === "cluster" && (
          <ClusterMarkersByView
            points={valid}
            displayMode={displayMode}
            onSelectDevice={selectHandler}
            onOpenSplitter={splitterHandler}
            onOpenCableFibers={cableFibersHandler} onOpenSplice={spliceHandler}
            onEditPosition={editHandler}
            onCopyCoords={copyCoordsHandler}
            spider={spider}
            setSpider={setSpider}
            spiderRef={spiderRef}
            runSpiderOpen={runSpiderOpen}
            stopSpiderAnim={stopSpiderAnim}
            colors={colors}
            iconStyles={iconStyles}
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
              onSelectDevice={selectHandler}
              onOpenSplitter={splitterHandler}
              onOpenCableFibers={cableFibersHandler} onOpenSplice={spliceHandler}
            onEditPosition={editHandler}
            onCopyCoords={copyCoordsHandler}
              colors={colors}
              iconStyles={iconStyles}
              keyPrefix="eq"
              highlightedId={highlightedId}
            />
            {connectionClusterForced && connValid.length > 0 ? (
              <ClusterMarkersByView
                points={connValid}
                displayMode={displayMode}
                onSelectDevice={selectHandler}
                onOpenSplitter={splitterHandler}
                onOpenCableFibers={cableFibersHandler} onOpenSplice={spliceHandler}
            onEditPosition={editHandler}
            onCopyCoords={copyCoordsHandler}
                spider={spider}
                setSpider={setSpider}
                spiderRef={spiderRef}
                runSpiderOpen={runSpiderOpen}
                stopSpiderAnim={stopSpiderAnim}
                colors={colors}
                iconStyles={iconStyles}
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
                onSelectDevice={selectHandler}
                onOpenSplitter={splitterHandler}
                onOpenCableFibers={cableFibersHandler} onOpenSplice={spliceHandler}
            onEditPosition={editHandler}
            onCopyCoords={copyCoordsHandler}
                colors={colors}
                iconStyles={iconStyles}
                keyPrefix="conn"
                highlightedId={highlightedId}
              />
            )}
          </>
        )}
      </MapContainer>
    </div>
  );
}