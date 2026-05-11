import L from "leaflet";
import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { MapContainer, Marker, Popup, TileLayer, useMap } from "react-leaflet";
import markerIcon2x from "leaflet/dist/images/marker-icon-2x.png";
import markerIcon from "leaflet/dist/images/marker-icon.png";
import markerShadow from "leaflet/dist/images/marker-shadow.png";
import "leaflet/dist/leaflet.css";

export type MapPoint = {
  id: string;
  description: string;
  lat: number;
  lng: number;
  ip?: string | null;
  category: string;
  status: string;
};

/** Agrupado (grelha), Desagrupado (marcadores individuais + empilhamento), Online/Offline (pins verde / vermelho / cinza). */
export type MapDisplayMode = "cluster" | "scatter" | "status";

const STACK_MERGE_M = 22;
const SPIDER_RADIUS_M = 28;
const FIT_PADDING: [number, number] = [48, 48];
const FIT_MAX_ZOOM = 16;
const SINGLE_POINT_ZOOM = 14;
const CLUSTER_EXPAND_MAX_ZOOM = 17;

function fixLeafletIcons() {
  const def = L.Icon.Default.prototype as unknown as { _getIconUrl?: unknown };
  delete def._getIconUrl;
  L.Icon.Default.mergeOptions({
    iconUrl: markerIcon,
    iconRetinaUrl: markerIcon2x,
    shadowUrl: markerShadow,
  });
}

/** Antes do primeiro paint dos Markers (o useEffect corre demasiado tarde). */
fixLeafletIcons();

/** Uma instância partilhada com URLs explícitos (evita tipos frágeis de `Icon.Default` no strict TS). */
let sharedDefaultMarkerIcon: L.Icon | null = null;
function defaultMarkerIcon(): L.Icon {
  if (!sharedDefaultMarkerIcon) {
    sharedDefaultMarkerIcon = L.icon({
      iconUrl: markerIcon,
      iconRetinaUrl: markerIcon2x,
      shadowUrl: markerShadow,
      iconSize: [25, 41],
      iconAnchor: [12, 41],
      popupAnchor: [1, -34],
      shadowSize: [41, 41],
    });
  }
  return sharedDefaultMarkerIcon;
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

function gridClusters(points: MapPoint[], decimals: number): { key: string; members: MapPoint[]; lat: number; lng: number }[] {
  const f = 10 ** decimals;
  const m = new Map<string, MapPoint[]>();
  for (const p of points) {
    const gx = Math.round(p.lat * f) / f;
    const gy = Math.round(p.lng * f) / f;
    const key = `${gx.toFixed(decimals)},${gy.toFixed(decimals)}`;
    const arr = m.get(key);
    if (arr) arr.push(p);
    else m.set(key, [p]);
  }
  const out: { key: string; members: MapPoint[]; lat: number; lng: number }[] = [];
  for (const [key, members] of m) {
    const lat = members.reduce((s, x) => s + x.lat, 0) / members.length;
    const lng = members.reduce((s, x) => s + x.lng, 0) / members.length;
    out.push({ key, members, lat, lng });
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
function markerIconOpts(displayMode: MapDisplayMode, status: string): { icon: L.Icon | L.DivIcon } {
  if (displayMode !== "status") return { icon: defaultMarkerIcon() };
  return { icon: statusPinIcon(status) };
}

function markerIconOptsGroup(displayMode: MapDisplayMode, members: MapPoint[]): { icon: L.Icon | L.DivIcon } {
  if (displayMode !== "status") return { icon: defaultMarkerIcon() };
  return { icon: statusPinIcon(dominantStatus(members)) };
}

type SpiderState = { key: string; members: MapPoint[]; center: [number, number]; phase: number } | null;

type ClusterCell = { key: string; members: MapPoint[]; lat: number; lng: number };

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
}) {
  const map = useMap();

  if (c.members.length === 1) {
    const p = c.members[0];
    return (
      <Marker position={[p.lat, p.lng]} {...markerIconOpts(displayMode, p.status)}>
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
        {...markerIconOptsGroup(displayMode, c.members)}
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
          <strong>{c.members.length} equipamento(s)</strong>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 6px" }}>Clique no pin para aproximar e ver marcadores separados.</p>
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
            <Marker key={p.id} position={[p.lat, p.lng]} {...markerIconOpts(displayMode, p.status)}>
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
              <Marker key={`${sk}-${m.id}`} position={[plat, plng]} {...markerIconOpts(displayMode, m.status)}>
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
            {...markerIconOptsGroup(displayMode, grp)}
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
}: {
  points: MapPoint[];
  displayMode: MapDisplayMode;
  onSelectDevice?: (id: string) => void;
  spider: SpiderState;
  setSpider: (s: SpiderState) => void;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
}) {
  const map = useMap();
  const [zoom, setZoom] = useState(() => map.getZoom());
  const decimals = gridDecimalsForZoom(zoom);
  const pointsFingerprint = useMemo(() => points.map((p) => p.id).sort().join("|"), [points]);

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
  }, [pointsFingerprint, decimals, stopSpiderAnim, setSpider]);

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
}: {
  sk: string;
  grp: MapPoint[];
  displayMode: MapDisplayMode;
  spiderRef: MutableRefObject<SpiderState>;
  runSpiderOpen: (key: string, members: MapPoint[], center: [number, number]) => void;
  stopSpiderAnim: () => void;
  setSpider: (s: SpiderState) => void;
}) {
  const map = useMap();
  const [clat, clng] = centroid(grp);
  return (
    <Marker
      position={[clat, clng]}
      {...markerIconOptsGroup(displayMode, grp)}
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

export function EquipmentMap({
  points,
  displayMode,
  onSelectDevice,
  flyTo,
  flyKey,
  fitBoundsVersion,
}: {
  points: MapPoint[];
  displayMode: MapDisplayMode;
  onSelectDevice?: (id: string) => void;
  /** Quando definido, centra o mapa neste ponto (ex.: filtro por equipamento). */
  flyTo: { lat: number; lng: number; zoom?: number } | null;
  /** Incrementar para repetir o mesmo alvo (ex.: re-seleccionar equipamento). */
  flyKey: number;
  /** Incrementar para voltar a ajustar o zoom aos pontos visíveis (refetch ou mudança de filtros). */
  fitBoundsVersion: number;
}) {
  const valid = useMemo(() => points.filter((p) => Number.isFinite(p.lat) && Number.isFinite(p.lng)), [points]);
  const center: [number, number] = valid.length ? [valid[0].lat, valid[0].lng] : [-14.235, -51.9253];

  const [spider, setSpider] = useState<SpiderState>(null);
  const spiderRef = useRef(spider);
  spiderRef.current = spider;
  const rafRef = useRef<number | null>(null);

  const pointsFingerprint = useMemo(() => valid.map((p) => p.id).sort().join("|"), [valid]);

  useEffect(() => {
    setSpider(null);
  }, [pointsFingerprint, displayMode]);

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

  const stacksScatter = useMemo(() => mergeProximityStacks(valid, STACK_MERGE_M), [valid]);

  const fitPointsRef = useRef<{ lat: number; lng: number }[]>([]);
  fitPointsRef.current = valid.map((p) => ({ lat: p.lat, lng: p.lng }));

  if (valid.length === 0) {
    return <p style={{ color: "var(--muted)" }}>Sem coordenadas para mostrar no mapa.</p>;
  }

  return (
    <MapContainer
      center={center}
      zoom={6}
      style={{ height: 480, width: "100%", minHeight: 420, borderRadius: "var(--radius)", border: "1px solid var(--border)" }}
      scrollWheelZoom
    >
      <MapInvalidateSize />
      <FitBounds pointsRef={fitPointsRef} version={fitBoundsVersion} />
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
        />
      )}

      {(displayMode === "scatter" || displayMode === "status") &&
        stacksScatter.map((grp, idx) => {
          const sk = `s-${idx}-${grp.map((g) => g.id).join(",")}`;
          const isSpider = spider?.key === sk;

          if (grp.length === 1) {
            const p = grp[0];
            return (
              <Marker key={p.id} position={[p.lat, p.lng]} {...markerIconOpts(displayMode, p.status)}>
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
                <Marker key={`${sk}-${m.id}`} position={[plat, plng]} {...markerIconOpts(displayMode, m.status)}>
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
            />
          );
        })}
    </MapContainer>
  );
}