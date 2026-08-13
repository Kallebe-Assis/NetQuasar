import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { FileUp, LocateFixed, Pencil, Search } from "lucide-react";
import { EquipmentMap, DEFAULT_MAP_COLORS, expandMapBounds, quantizeMapBounds, sameMapBounds, type MapBounds, type MapDisplayMode, type MapLatLng, type MapPlaceMode, type MapPoint } from "../components/EquipmentMap";
import { MapDetailModal } from "../components/MapDetailModal";
import { MapFilterButton, MapFilterModal } from "../components/MapFilterModal";
import { MapInfraSidePanel, parseInfraMapId } from "../components/MapInfraSidePanel";
import { MapPlaceElementModal, type MapPlaceSession, type PlaceableKind } from "../components/MapPlaceElementModal";
import { MapProjectKmlImport } from "../components/MapProjectKmlImport";
import { MapSettingsButton, MapSettingsModal } from "../components/MapSettingsModal";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { CTO_MAP_PIN_COLOR, DEFAULT_MAP_ICON_STYLES, INFRA_MAP_KIND_LABELS, isInfraMapKind, type InfraMapKind, type MapIconStyles } from "../lib/mapInfrastructureIcons";
import { fiberSpecByName } from "../lib/fiberSplitter";
import { formatDistanceMeters } from "../lib/nearestCtoMatch";
import { apiFetch } from "../lib/api";
import { can, isAdminUser } from "../lib/auth";
import { queryKeys } from "../lib/queryKeys";
import { fetchUiAppearance, mapColorsFromAppearance, mapIconsFromAppearance, type MapAppearanceColors } from "../lib/uiAppearance";
import { looksLikeHTTPURL, parseLatLngPair, shouldLocateQuery, type MapLocateHit } from "../lib/mapLocationQuery";

class MapSectionErrorBoundary extends React.Component<Readonly<{ children: React.ReactNode }>, { err: Error | null }> {
  state = { err: null as Error | null };

  static getDerivedStateFromError(err: Error) {
    return { err };
  }

  render() {
    if (this.state.err) {
      return (
        <div className="msg msg--err" style={{ marginTop: 12, padding: 12, color: "var(--text)" }}>
          <strong>Erro ao mostrar o mapa</strong>
          <p style={{ margin: "8px 0 0", fontSize: 12 }}>{this.state.err.message}</p>
          <p style={{ margin: "8px 0 0", fontSize: 11, color: "var(--muted)" }}>Recarregue a página ou abra a consola do navegador (F12) para mais detalhes.</p>
          <button type="button" className="btn" style={{ marginTop: 10 }} onClick={() => this.setState({ err: null })}>
            Tentar novamente
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

type Point = {
  id: string;
  description: string;
  category: string;
  lat: number;
  lng: number;
  ip?: string | null;
  pop_id?: string | null;
  operational_mode?: string;
  status: string;
  last_check_at?: string | null;
  coord_source?: string;
  point_type?: "equipment" | "connection" | InfraMapKind;
  login?: string;
  mapKind?: InfraMapKind | "equipment" | "connection";
  markerColor?: string | null;
  display_number?: number;
  mapLabel?: string;
  splitter?: string | null;
  fiber_color?: string | null;
  path?: MapLatLng[] | null;
};

type ConnectionPoint = {
  id: string;
  client_name: string;
  login: string;
  connection_kind: string;
  lat: number;
  lng: number;
  address?: string;
  neighborhood?: string;
};

type InfrastructurePoint = {
  id: string;
  description: string;
  display_number: number;
  lat: number;
  lng: number;
  point_type: InfraMapKind;
  id_prefix?: string;
  color?: string;
  splitter?: string | null;
  fiber_color?: string | null;
  path?: MapLatLng[] | null;
};

type NearestCtoApi = {
  id: string;
  map_id: string;
  description: string;
  display_number: number;
  lat: number;
  lng: number;
  distance_m: number;
  splitter?: string | null;
  fiber_color?: string | null;
};

type PointDetail = Point & {
  network_status?: string;
  brand?: string;
  model?: string;
  mac?: string;
  serial_number?: string;
  software_version?: string;
  hardware_version?: string;
  ping_enabled?: boolean;
  telemetry_enabled?: boolean;
  locality_id?: string | null;
  updated_at?: string | null;
  last_check_at?: string | null;
};

/** Alinhado com a lista de categorias em equipamentos. */

const MAP_LIST_PAGE_SIZE = 100;

type MapSearchResult = {
  id: string;
  label: string;
  kind: string;
  category?: string;
  project_name?: string | null;
  lat: number;
  lng: number;
  map_id: string;
};

function parseMapSearchInput(raw: string): { q: string; type: string } {
  const s = raw.trim();
  const rules: [RegExp, string][] = [
    [/^cto\s*:\s*/i, "cto"],
    [/^poste\s*:\s*/i, "pole"],
    [/^pop\s*:\s*/i, "pop"],
    [/^login\s*:\s*/i, "login"],
    [/^logins\s*:\s*/i, "login"],
    [/^equip(?:amento)?s?\s*:\s*/i, "equipment"],
    [/^infra\s*:\s*/i, "infra"],
  ];
  for (const [re, type] of rules) {
    if (re.test(s)) return { q: s.replace(re, "").trim(), type };
  }
  return { q: s, type: "" };
}

function searchKindLabel(kind: string): string {
  switch (kind) {
    case "equipment":
      return "Equipamento";
    case "login":
      return "Login";
    case "cto":
      return "CTO";
    case "pole":
      return "Poste";
    case "splice_box":
      return "Emenda";
    case "cable":
      return "Cabo";
    case "project":
      return "Projeto";
    case "pop":
      return "POP";
    default:
      return kind;
  }
}

function IconRefresh() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
      <path d="M3 3v5h5" />
      <path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16" />
      <path d="M16 16h5v5" />
    </svg>
  );
}

export function MapPage() {
  const [view, setView] = useState<"lista" | "mapa">("mapa");
  const userPickedTab = useRef(false);
  const [popId, setPopId] = useState("");
  const [category, setCategory] = useState("");
  const [selId, setSelId] = useState<string | null>(null);
  const [displayMode, setDisplayMode] = useState<MapDisplayMode>("cluster");
  const [ctoColorByFeed, setCtoColorByFeed] = useState(false);
  const [fitBoundsVersion, setFitBoundsVersion] = useState(0);
  const [flyTo, setFlyTo] = useState<{ lat: number; lng: number; zoom?: number } | null>(null);
  const [flyKey, setFlyKey] = useState(0);
  const [mapToast, setMapToast] = useState<{ ok: boolean; text: string } | null>(null);
  const [showConnections, setShowConnections] = useState(false);
  const [showCtos, setShowCtos] = useState(true);
  const [showCables, setShowCables] = useState(true);
  const [showSpliceBoxes, setShowSpliceBoxes] = useState(false);
  const [showPoles, setShowPoles] = useState(false);
  const [showProjects, setShowProjects] = useState(false);
  const [showPops, setShowPops] = useState(true);
  const [showEquipment, setShowEquipment] = useState(true);
  const [projectFilterId, setProjectFilterId] = useState("");
  const [filterModalOpen, setFilterModalOpen] = useState(false);
  const [settingsModalOpen, setSettingsModalOpen] = useState(false);
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [infraPanelOpen, setInfraPanelOpen] = useState(false);
  const [autoOpenSplitter, setAutoOpenSplitter] = useState(false);
  const [autoOpenCableFibers, setAutoOpenCableFibers] = useState(false);
  const [autoOpenSplice, setAutoOpenSplice] = useState(false);
  const [autoOpenEdit, setAutoOpenEdit] = useState(false);
  const [searchInput, setSearchInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchTypeFilter, setSearchTypeFilter] = useState("");
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const [locationPin, setLocationPin] = useState<{ lat: number; lng: number; label?: string } | null>(null);
  const [localityFlyId, setLocalityFlyId] = useState("");
  const [localityFlyNote, setLocalityFlyNote] = useState<string | null>(null);
  const [localityFlyPending, setLocalityFlyPending] = useState(false);
  const [detailFallback, setDetailFallback] = useState<Point | null>(null);
  const searchWrapRef = useRef<HTMLDivElement>(null);
  const [mapBounds, setMapBounds] = useState<MapBounds | null>(null);
  const [listPage, setListPage] = useState(0);
  const [mapPrefsDraft, setMapPrefsDraft] = useState<MapAppearanceColors>(() => ({
    equipment: DEFAULT_MAP_COLORS.equipment,
    connection: DEFAULT_MAP_COLORS.connection,
    cto: DEFAULT_MAP_COLORS.cto ?? CTO_MAP_PIN_COLOR,
    splice_box: DEFAULT_MAP_COLORS.splice_box ?? "#d97706",
  }));
  const [mapIconsDraft, setMapIconsDraft] = useState<MapIconStyles>(() => ({ ...DEFAULT_MAP_ICON_STYLES }));
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [addKind, setAddKind] = useState<PlaceableKind | null>(null);
  const [draftPath, setDraftPath] = useState<MapLatLng[]>([]);
  const [placeSession, setPlaceSession] = useState<MapPlaceSession | null>(null);
  const addMenuRef = useRef<HTMLDivElement>(null);
  const canEditMap = isAdminUser() || can("connections.manage") || can("map.manage");
  const [mapEditMode, setMapEditMode] = useState(false);
  const [kmlImportOpen, setKmlImportOpen] = useState(false);
  const [hiddenMapIds, setHiddenMapIds] = useState<Set<string>>(() => new Set());
  const [repositionTarget, setRepositionTarget] = useState<{
    mapId: string;
    kind: InfraMapKind;
    entityId: string;
  } | null>(null);
  const [repositionPreview, setRepositionPreview] = useState<MapLatLng | null>(null);
  const [editingCable, setEditingCable] = useState<{ mapId: string; entityId: string } | null>(null);
  const [geoTracking, setGeoTracking] = useState(false);
  const [userLocation, setUserLocation] = useState<MapLatLng | null>(null);
  const [geoError, setGeoError] = useState<string | null>(null);
  const [nearestCtos, setNearestCtos] = useState<NearestCtoApi[]>([]);
  const geoWatchRef = useRef<number | null>(null);
  const geoFirstFixRef = useRef(false);
  const nearestQueryRef = useRef<{ lat: number; lng: number; at: number } | null>(null);
  const onMapBoundsChange = useCallback((b: MapBounds) => {
    const next = quantizeMapBounds(b);
    setMapBounds((prev) => (prev && sameMapBounds(prev, next) ? prev : next));
  }, []);
  const qc = useQueryClient();

  const placeMode: MapPlaceMode = editingCable
    ? "edit-cable"
    : repositionTarget
      ? "reposition"
      : addKind === "cable"
        ? "cable"
        : addKind
          ? "place"
          : null;

  const uiAppearance = useQuery({
    queryKey: queryKeys.uiAppearance,
    queryFn: fetchUiAppearance,
  });

  useEffect(() => {
    setMapPrefsDraft(mapColorsFromAppearance(uiAppearance.data));
    setMapIconsDraft(mapIconsFromAppearance(uiAppearance.data));
  }, [
    uiAppearance.data?.map_equipment_color,
    uiAppearance.data?.map_connection_color,
    uiAppearance.data?.map_cto_color,
    uiAppearance.data?.map_splice_color,
    uiAppearance.data?.map_equipment_icon,
    uiAppearance.data?.map_connection_icon,
    uiAppearance.data?.map_cto_icon,
    uiAppearance.data?.map_splice_icon,
  ]);

  const saveMapPrefs = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/ui-appearance", {
        method: "PATCH",
        json: {
          map_equipment_color: mapPrefsDraft.equipment,
          map_connection_color: mapPrefsDraft.connection,
          map_cto_color: mapPrefsDraft.cto,
          map_splice_color: mapPrefsDraft.splice_box,
          map_equipment_icon: mapIconsDraft.equipment,
          map_connection_icon: mapIconsDraft.connection,
          map_cto_icon: mapIconsDraft.cto,
          map_splice_icon: mapIconsDraft.splice_box,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.uiAppearance });
      setSettingsModalOpen(false);
      setMapToast({ ok: true, text: "Preferências do mapa guardadas." });
    },
    onError: (e) => setMapToast({ ok: false, text: e instanceof Error ? e.message : "Falha ao guardar preferências." }),
  });

  const mapColors = useMemo(
    () => ({
      equipment: mapPrefsDraft.equipment,
      connection: mapPrefsDraft.connection,
      cto: mapPrefsDraft.cto,
      splice_box: mapPrefsDraft.splice_box,
    }),
    [mapPrefsDraft],
  );

  const mapIconStyles = mapIconsDraft;

  const pops = useQuery({ queryKey: ["pops"], queryFn: () => apiFetch<{ pops: { id: string; description: string }[] }>("/api/v1/pops") });

  const localities = useQuery({
    queryKey: ["commercial-localities-map"],
    queryFn: () => apiFetch<{ localities: { id: string; name: string }[] }>("/api/v1/commercial/localities"),
  });

  useEffect(() => {
    const t = window.setTimeout(() => {
      const parsed = parseMapSearchInput(searchInput);
      setDebouncedSearch(parsed.q);
      setSearchTypeFilter(parsed.type);
    }, 280);
    return () => window.clearTimeout(t);
  }, [searchInput]);

  const locateQueryEnabled = shouldLocateQuery(searchInput) && debouncedSearch.length >= 2;
  const entitySearchEnabled =
    debouncedSearch.length >= 2 &&
    !looksLikeHTTPURL(searchInput) &&
    !parseLatLngPair(searchInput);

  const mapSearch = useQuery({
    queryKey: ["map-search", debouncedSearch, searchTypeFilter],
    enabled: entitySearchEnabled,
    queryFn: () => {
      const params = new URLSearchParams({ q: debouncedSearch });
      if (searchTypeFilter) params.set("type", searchTypeFilter);
      return apiFetch<{ results: MapSearchResult[] }>(`/api/v1/map/search?${params.toString()}`);
    },
  });

  const mapLocate = useQuery({
    queryKey: ["map-locate", searchInput.trim()],
    enabled: locateQueryEnabled,
    queryFn: () =>
      apiFetch<{ results: MapLocateHit[]; note?: string }>(
        `/api/v1/map/locate?q=${encodeURIComponent(searchInput.trim())}`,
      ),
  });

  useEffect(() => {
    if (!searchOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (searchWrapRef.current && !searchWrapRef.current.contains(e.target as Node)) {
        setSearchOpen(false);
        setMobileSearchOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [searchOpen]);

  const queryBounds = useMemo(() => (mapBounds ? expandMapBounds(mapBounds, 0.28) : null), [mapBounds]);

  const connPts = useQuery({
    queryKey: [
      "map-connection-points",
      queryBounds?.minLat,
      queryBounds?.maxLat,
      queryBounds?.minLng,
      queryBounds?.maxLng,
      queryBounds?.zoom,
    ],
    queryFn: () => {
      const params = new URLSearchParams();
      if (queryBounds) {
        params.set("min_lat", String(queryBounds.minLat));
        params.set("max_lat", String(queryBounds.maxLat));
        params.set("min_lng", String(queryBounds.minLng));
        params.set("max_lng", String(queryBounds.maxLng));
        if (queryBounds.zoom != null) params.set("zoom", String(queryBounds.zoom));
      }
      const qs = params.toString();
      return apiFetch<{ points: ConnectionPoint[]; total?: number; truncated?: boolean; limit?: number }>(
        `/api/v1/map/connection-points${qs ? `?${qs}` : ""}`,
      );
    },
    enabled: showConnections && queryBounds != null,
    placeholderData: keepPreviousData,
    staleTime: 20_000,
    refetchOnWindowFocus: false,
  });

  const infraKinds = useMemo(() => {
    const kinds: string[] = [];
    if (showCtos) kinds.push("ctos");
    if (showCables) kinds.push("cables");
    if (showSpliceBoxes) kinds.push("splice_boxes");
    if (showPoles) kinds.push("poles");
    if (showProjects) kinds.push("projects");
    if (showPops) kinds.push("pops");
    return kinds;
  }, [showCtos, showCables, showSpliceBoxes, showPoles, showProjects, showPops]);

  const showInfrastructure = infraKinds.length > 0;

  const projectsList = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: () =>
      apiFetch<{ projects: { id: string; display_number: number; description: string; status?: string }[] }>(
        "/api/v1/commercial/network/projects",
      ),
  });
  const mapProjects = useMemo(
    () => (projectsList.data?.projects ?? []).filter((p) => p.status !== "inativo"),
    [projectsList.data],
  );

  const infraPts = useQuery({
    queryKey: [
      "map-infrastructure-points",
      infraKinds.join(","),
      projectFilterId,
      queryBounds?.minLat,
      queryBounds?.maxLat,
      queryBounds?.minLng,
      queryBounds?.maxLng,
      queryBounds?.zoom,
    ],
    queryFn: () => {
      const params = new URLSearchParams();
      params.set("kinds", infraKinds.join(","));
      if (projectFilterId.trim()) params.set("project_id", projectFilterId.trim());
      if (queryBounds) {
        params.set("min_lat", String(queryBounds.minLat));
        params.set("max_lat", String(queryBounds.maxLat));
        params.set("min_lng", String(queryBounds.minLng));
        params.set("max_lng", String(queryBounds.maxLng));
        if (queryBounds.zoom != null) params.set("zoom", String(queryBounds.zoom));
      }
      const qs = params.toString();
      return apiFetch<{ points: InfrastructurePoint[]; total?: number; truncated?: boolean; limit?: number }>(
        `/api/v1/map/infrastructure-points${qs ? `?${qs}` : ""}`,
      );
    },
    enabled: showInfrastructure && queryBounds != null,
    placeholderData: keepPreviousData,
    staleTime: 20_000,
    refetchOnWindowFocus: false,
  });

  const pts = useQuery({
    queryKey: ["map-points", popId, category],
    queryFn: () => {
      const params = new URLSearchParams();
      const pPop = popId.trim();
      const pCat = category.trim();
      if (pPop) params.set("pop_id", pPop);
      if (pCat) params.set("category", pCat);
      const qs = params.toString();
      return apiFetch<{ points: Point[] }>(`/api/v1/map/equipment-points${qs ? `?${qs}` : ""}`);
    },
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });

  const equipPoints = useMemo(() => (Array.isArray(pts.data?.points) ? pts.data.points : []), [pts.data?.points]);

  const infraCacheRef = useRef<Map<string, InfrastructurePoint>>(new Map());
  const infraCacheScopeRef = useRef("");
  const [infraStablePoints, setInfraStablePoints] = useState<InfrastructurePoint[]>([]);

  useEffect(() => {
    const scope = `${infraKinds.join(",")}|${projectFilterId}`;
    if (infraCacheScopeRef.current !== scope) {
      infraCacheRef.current.clear();
      infraCacheScopeRef.current = scope;
    }
    const incoming = infraPts.data?.points;
    if (!Array.isArray(incoming) || !queryBounds) return;
    const cache = infraCacheRef.current;
    for (const p of incoming) {
      cache.set(`${p.point_type}:${p.id}`, p);
    }
    const keep = expandMapBounds(queryBounds, 0.6);
    const next: InfrastructurePoint[] = [];
    for (const [k, p] of cache) {
      const lat = Number(p.lat);
      const lng = Number(p.lng);
      if (
        !Number.isFinite(lat) ||
        !Number.isFinite(lng) ||
        lat < keep.minLat ||
        lat > keep.maxLat ||
        lng < keep.minLng ||
        lng > keep.maxLng
      ) {
        cache.delete(k);
        continue;
      }
      next.push(p);
    }
    setInfraStablePoints(next);
  }, [infraPts.data?.points, queryBounds, infraKinds, projectFilterId]);

  const displayedPoints = useMemo(() => {
    const projectScoped = projectFilterId.trim() !== "";
    // Com projeto seleccionado: só infraestrutura desse projeto (sem equipamentos/logins).
    const equip = showEquipment && !projectScoped ? equipPoints : [];
    const connRaw = showConnections && !projectScoped && Array.isArray(connPts.data?.points) ? connPts.data.points : [];
    const conn: Point[] = connRaw.map((c) => ({
      id: `conn-${c.id}`,
      description: `${c.client_name} (${c.login})`,
      category: c.connection_kind === "dhcp" ? "Conexão DHCP" : "Conexão PPPoE",
      lat: Number(c.lat),
      lng: Number(c.lng),
      status: "connection",
      point_type: "connection" as const,
      mapKind: "connection" as const,
      login: c.login,
    }));
    const infraRaw = showInfrastructure ? infraStablePoints : [];
    const infraIds = new Set<string>();
    const infra: Point[] = infraRaw
      .filter((p) => isInfraMapKind(p.point_type))
      .map((p) => {
        infraIds.add(`${p.point_type}:${p.id}`);
        const splitterLabel = p.point_type === "cto" && p.splitter ? String(p.splitter).trim() : "";
        const ctoColor = ctoColorByFeed ? fiberSpecByName(p.fiber_color).hex : mapPrefsDraft.cto;
        const spliceColor = mapPrefsDraft.splice_box;
        return {
          id: `infra-${p.point_type}-${p.id}`,
          description:
            p.point_type === "cto" || p.point_type === "pop"
              ? p.description
              : p.id_prefix
                ? `${p.id_prefix} ${p.display_number} — ${p.description}`
                : p.description,
          category: p.point_type === "cto" ? "CTO" : INFRA_MAP_KIND_LABELS[p.point_type],
          lat: Number(p.lat),
          lng: Number(p.lng),
          status: p.point_type === "cto" ? splitterLabel || "—" : "infra",
          point_type: p.point_type,
          mapKind: p.point_type,
          markerColor:
            p.point_type === "cto" ? ctoColor : p.point_type === "splice_box" ? spliceColor : p.color ?? null,
          display_number: p.display_number,
          mapLabel: p.point_type === "cto" ? p.description : undefined,
          splitter: p.splitter ?? null,
          fiber_color: p.fiber_color ?? null,
          path: Array.isArray(p.path) ? p.path : null,
        };
      });
    // Garante que as CTOs próximas do GPS aparecem mesmo fora do viewport actual.
    if (showCtos) {
      for (const c of nearestCtos) {
        const key = `cto:${c.id}`;
        if (infraIds.has(key)) continue;
        infraIds.add(key);
        const ctoColor = ctoColorByFeed ? fiberSpecByName(c.fiber_color).hex : mapPrefsDraft.cto;
        infra.push({
          id: c.map_id,
          description: c.description,
          category: "CTO",
          lat: Number(c.lat),
          lng: Number(c.lng),
          status: (c.splitter ?? "").trim() || "—",
          point_type: "cto",
          mapKind: "cto",
          markerColor: ctoColor,
          display_number: c.display_number,
          mapLabel: c.description,
          splitter: c.splitter ?? null,
          fiber_color: c.fiber_color ?? null,
          path: null,
        });
      }
    }
    return [...equip, ...conn, ...infra].filter((p) => !hiddenMapIds.has(p.id));
  }, [
    equipPoints,
    connPts.data?.points,
    infraStablePoints,
    showConnections,
    showEquipment,
    showInfrastructure,
    showCtos,
    nearestCtos,
    projectFilterId,
    ctoColorByFeed,
    mapPrefsDraft.cto,
    mapPrefsDraft.splice_box,
    hiddenMapIds,
  ]);

  const connTotal = connPts.data?.total;
  const connTruncated = !!connPts.data?.truncated;
  const connLimit = connPts.data?.limit;

  const connectionClusterForced = useMemo(() => {
    if (!showConnections || displayMode === "cluster") return false;
    const zoom = mapBounds?.zoom ?? 6;
    const connCount = connPts.data?.points?.length ?? 0;
    if (zoom < 13) return true;
    if (connCount > 500 || (connTotal ?? 0) > 800) return true;
    return false;
  }, [showConnections, displayMode, mapBounds?.zoom, connPts.data?.points?.length, connTotal]);

  const selPoint = useMemo(
    () => displayedPoints.find((p) => p.id === selId) ?? (detailFallback?.id === selId ? detailFallback : null),
    [displayedPoints, selId, detailFallback],
  );
  const isConnPoint = !!selId?.startsWith("conn-");
  const isInfraPoint = !!selId?.startsWith("infra-");

  const detail = useQuery({
    queryKey: ["map-point-detail", selId],
    enabled: !!selId && !isConnPoint && !isInfraPoint,
    queryFn: () => apiFetch<PointDetail>(`/api/v1/map/equipment-points/${selId!}`),
  });

  const mapPoints: MapPoint[] = useMemo(() => {
    const nearestLabel = new Map(nearestCtos.map((c, i) => [c.map_id, `#${i + 1} · ${formatDistanceMeters(c.distance_m)}`]));
    const zoom = mapBounds?.zoom ?? 0;
    // Labels de CTO são caras no DOM — só com zoom alto ou nas próximas/seleccionadas.
    const showCtoLabels = zoom >= 15;
    return displayedPoints
      .filter((p) => !(repositionTarget && p.id === repositionTarget.mapId))
      .map((p) => {
      const nearest = nearestLabel.get(p.id);
      const selected = selId === p.id;
      let mapLabel = p.mapLabel;
      if (p.mapKind === "cto") {
        if (nearest) mapLabel = nearest;
        else if (!showCtoLabels && !selected) mapLabel = undefined;
      }
      return {
        id: p.id,
        description: p.description,
        lat: Number(p.lat),
        lng: Number(p.lng),
        ip: p.ip,
        category: p.category,
        status: p.status,
        mapKind: p.mapKind,
        markerColor: p.markerColor,
        mapLabel,
        splitter: p.splitter ?? null,
        path: p.path ?? null,
      };
    });
  }, [displayedPoints, selId, nearestCtos, mapBounds?.zoom, repositionTarget]);

  const mapHighlightIds = useMemo(() => {
    const ids = nearestCtos.map((c) => c.map_id);
    if (selId && !ids.includes(selId)) ids.unshift(selId);
    else if (selId) {
      // keep selId first for emphasis order isn't needed
    }
    return ids.length > 0 ? ids : selId;
  }, [nearestCtos, selId]);

  const stopGeoTracking = useCallback(() => {
    if (geoWatchRef.current != null && typeof navigator !== "undefined" && navigator.geolocation) {
      navigator.geolocation.clearWatch(geoWatchRef.current);
    }
    geoWatchRef.current = null;
    setGeoTracking(false);
  }, []);

  const fetchNearestCtos = useCallback(
    async (lat: number, lng: number) => {
      const params = new URLSearchParams({
        lat: String(lat),
        lng: String(lng),
        limit: "3",
      });
      if (projectFilterId.trim()) params.set("project_id", projectFilterId.trim());
      try {
        const r = await apiFetch<{ ctos?: NearestCtoApi[] }>(`/api/v1/map/nearest-ctos?${params}`);
        setNearestCtos(Array.isArray(r.ctos) ? r.ctos : []);
      } catch (e) {
        setMapToast({ ok: false, text: e instanceof Error ? e.message : "Falha ao calcular CTOs próximas." });
      }
    },
    [projectFilterId],
  );

  const startGeoTracking = useCallback(() => {
    if (typeof navigator === "undefined" || !navigator.geolocation) {
      setGeoError("Geolocalização não é suportada neste navegador.");
      setMapToast({ ok: false, text: "Geolocalização não suportada neste dispositivo." });
      return;
    }
    if (!window.isSecureContext) {
      setGeoError("GPS exige HTTPS (ou localhost).");
      setMapToast({ ok: false, text: "Para usar o GPS no telemóvel, aceda via HTTPS." });
      return;
    }
    stopGeoTracking();
    setGeoError(null);
    setGeoTracking(true);
    setShowCtos(true);
    setShowEquipment(false);
    setShowConnections(false);
    geoFirstFixRef.current = false;
    nearestQueryRef.current = null;
    userPickedTab.current = true;
    setView("mapa");
    setMapToast({ ok: true, text: "A pedir permissão de localização…" });

    geoWatchRef.current = navigator.geolocation.watchPosition(
      (pos) => {
        const lat = pos.coords.latitude;
        const lng = pos.coords.longitude;
        setUserLocation({ lat, lng });
        setGeoError(null);
        if (!geoFirstFixRef.current) {
          geoFirstFixRef.current = true;
          setFlyTo({ lat, lng, zoom: 17 });
          setFlyKey((k) => k + 1);
          setMapToast({ ok: true, text: "Posição obtida. A calcular CTOs próximas…" });
        }
        const prev = nearestQueryRef.current;
        const now = Date.now();
        const movedFar =
          !prev ||
          Math.abs(prev.lat - lat) > 0.00018 ||
          Math.abs(prev.lng - lng) > 0.00018 ||
          now - prev.at > 8_000;
        if (!movedFar) return;
        nearestQueryRef.current = { lat, lng, at: now };
        void fetchNearestCtos(lat, lng);
      },
      (err) => {
        const msg =
          err.code === err.PERMISSION_DENIED
            ? "Permissão de localização negada. Active o GPS nas definições do browser."
            : err.code === err.POSITION_UNAVAILABLE
              ? "Posição indisponível. Verifique o GPS do dispositivo."
              : "Tempo esgotado ao obter a localização.";
        setGeoError(msg);
        setMapToast({ ok: false, text: msg });
        stopGeoTracking();
      },
      { enableHighAccuracy: true, maximumAge: 3_000, timeout: 25_000 },
    );
  }, [fetchNearestCtos, stopGeoTracking]);

  useEffect(() => {
    return () => {
      if (geoWatchRef.current != null && typeof navigator !== "undefined" && navigator.geolocation) {
        navigator.geolocation.clearWatch(geoWatchRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (!geoTracking || !userLocation) return;
    void fetchNearestCtos(userLocation.lat, userLocation.lng);
  }, [projectFilterId]); // eslint-disable-line react-hooks/exhaustive-deps -- só reconsulta ao mudar projeto

  const cancelAddMode = useCallback(() => {
    setAddKind(null);
    setDraftPath([]);
    setAddMenuOpen(false);
    setRepositionTarget(null);
    setRepositionPreview(null);
    setEditingCable(null);
  }, []);

  const toggleMapEditMode = useCallback(() => {
    setMapEditMode((prev) => {
      const next = !prev;
      if (!next) {
        setRepositionTarget(null);
        setRepositionPreview(null);
        setEditingCable(null);
        setDraftPath([]);
        setAddKind(null);
      } else {
        setAddKind(null);
        setAddMenuOpen(false);
        setPlaceSession(null);
      }
      return next;
    });
  }, []);

  const patchInfraPosition = useCallback(
    async (kind: InfraMapKind, entityId: string, lat: number, lng: number, path?: MapLatLng[]) => {
      const endpoint =
        kind === "cto"
          ? `/api/v1/commercial/network/ctos/${entityId}`
          : kind === "cable"
            ? `/api/v1/commercial/network/cables/${entityId}`
            : kind === "splice_box"
              ? `/api/v1/commercial/network/splice-boxes/${entityId}`
              : kind === "pole"
                ? `/api/v1/commercial/network/poles/${entityId}`
                : kind === "project"
                  ? `/api/v1/commercial/network/projects/${entityId}`
                  : kind === "pop"
                    ? `/api/v1/pops/${entityId}`
                    : null;
      if (!endpoint) throw new Error("Tipo não suportado.");
      const json: Record<string, unknown> = { latitude: lat, longitude: lng };
      if (kind === "cable" && path && path.length >= 2) {
        json.path = path.map((p) => ({ lat: p.lat, lng: p.lng }));
      }
      await apiFetch(endpoint, { method: "PATCH", json });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      if (kind === "cto") await qc.invalidateQueries({ queryKey: ["map-cto-detail", entityId] });
      if (kind === "cable") await qc.invalidateQueries({ queryKey: ["map-cable-detail", entityId] });
    },
    [qc],
  );

  const startReposition = useCallback(
    (mapId: string, kind: InfraMapKind, entityId: string) => {
      if (!mapEditMode) {
        setMapToast({ ok: false, text: "Active o Modo edição para reposicionar elementos." });
        return;
      }
      setAddKind(null);
      setPlaceSession(null);
      if (kind === "cable") {
        const pt = displayedPoints.find((p) => p.id === mapId);
        const path =
          pt?.path && pt.path.length >= 2
            ? pt.path.map((p) => ({ lat: p.lat, lng: p.lng }))
            : pt
              ? [
                  { lat: Number(pt.lat), lng: Number(pt.lng) },
                  { lat: Number(pt.lat) + 0.0001, lng: Number(pt.lng) + 0.0001 },
                ]
              : [];
        setEditingCable({ mapId, entityId });
        setDraftPath(path);
        setRepositionTarget(null);
        setRepositionPreview(null);
        setMapToast({ ok: true, text: "Arraste os pontos do cabo ou clique no mapa para adicionar. Guarde quando terminar." });
        return;
      }
      const pt = displayedPoints.find((p) => p.id === mapId);
      setEditingCable(null);
      setDraftPath([]);
      setRepositionTarget({ mapId, kind, entityId });
      setRepositionPreview(pt ? { lat: Number(pt.lat), lng: Number(pt.lng) } : null);
      setMapToast({ ok: true, text: "Clique no mapa ou arraste o marcador para a nova posição." });
    },
    [mapEditMode, displayedPoints],
  );

  const commitReposition = useCallback(
    async (lat: number, lng: number) => {
      if (!repositionTarget) return;
      try {
        await patchInfraPosition(repositionTarget.kind, repositionTarget.entityId, lat, lng);
        setRepositionPreview({ lat, lng });
        setRepositionTarget(null);
        setRepositionPreview(null);
        setMapToast({ ok: true, text: "Posição actualizada." });
        setFlyTo({ lat, lng, zoom: 17 });
        setFlyKey((k) => k + 1);
        if (selId === repositionTarget.mapId) {
          setDetailFallback({
            id: repositionTarget.mapId,
            description: selPoint?.description ?? "Elemento",
            category: selPoint?.category ?? "",
            lat,
            lng,
            status: selPoint?.status ?? "infra",
            mapKind: repositionTarget.kind,
          });
        }
      } catch (e) {
        setMapToast({ ok: false, text: e instanceof Error ? e.message : "Falha ao reposicionar." });
      }
    },
    [repositionTarget, patchInfraPosition, selId, selPoint],
  );

  const saveEditedCablePath = useCallback(async () => {
    if (!editingCable) return;
    if (draftPath.length < 2) {
      setMapToast({ ok: false, text: "O cabo precisa de pelo menos 2 pontos." });
      return;
    }
    try {
      await patchInfraPosition("cable", editingCable.entityId, draftPath[0].lat, draftPath[0].lng, draftPath);
      setEditingCable(null);
      setDraftPath([]);
      setMapToast({ ok: true, text: "Trajeto do cabo actualizado." });
    } catch (e) {
      setMapToast({ ok: false, text: e instanceof Error ? e.message : "Falha ao guardar o cabo." });
    }
  }, [editingCable, draftPath, patchInfraPosition]);

  const enableLayerForKind = useCallback((kind: PlaceableKind) => {
    if (kind === "cto") setShowCtos(true);
    else if (kind === "cable") setShowCables(true);
    else if (kind === "splice_box") setShowSpliceBoxes(true);
    else if (kind === "pole") setShowPoles(true);
    else if (kind === "project") setShowProjects(true);
    else if (kind === "pop") setShowPops(true);
  }, []);

  const startAddKind = useCallback((kind: PlaceableKind) => {
    setMapEditMode(false);
    setRepositionTarget(null);
    setRepositionPreview(null);
    setEditingCable(null);
    setAddKind(kind);
    setDraftPath([]);
    setAddMenuOpen(false);
    setPlaceSession(null);
    enableLayerForKind(kind);
    userPickedTab.current = true;
    setView("mapa");
  }, [enableLayerForKind]);

  const handleMapPlaceClick = useCallback(
    (lat: number, lng: number) => {
      if (repositionTarget) {
        void commitReposition(lat, lng);
        return;
      }
      if (editingCable) {
        setDraftPath((prev) => [...prev, { lat, lng }]);
        return;
      }
      if (!addKind) return;
      if (addKind === "cable") {
        setDraftPath((prev) => [...prev, { lat, lng }]);
        return;
      }
      setPlaceSession({ mode: "create", kind: addKind, lat, lng });
      setAddKind(null);
    },
    [addKind, repositionTarget, editingCable, commitReposition],
  );

  const saveCablePath = useCallback(() => {
    if (draftPath.length < 2) {
      setMapToast({ ok: false, text: "O cabo precisa de pelo menos 2 pontos no trajeto." });
      return;
    }
    setShowCables(true);
    setPlaceSession({
      mode: "create-cable-path",
      path: draftPath.map((p) => ({ lat: p.lat, lng: p.lng })),
    });
    setAddKind(null);
    setDraftPath([]);
  }, [draftPath]);

  const openPointDetail = useCallback(
    (id: string, fly = true) => {
      setDetailFallback(null);
      setSelId(id);
      const infra = parseInfraMapId(id);
      if (infra) {
        setInfraPanelOpen(true);
        setDetailModalOpen(false);
      } else {
        setInfraPanelOpen(false);
        setDetailModalOpen(true);
      }
      userPickedTab.current = true;
      setView("mapa");
      const p = displayedPoints.find((x) => x.id === id);
      if (p && fly) {
        setFlyTo({ lat: Number(p.lat), lng: Number(p.lng), zoom: 17 });
        setFlyKey((k) => k + 1);
      }
    },
    [displayedPoints],
  );

  const selectSearchResult = useCallback(
    (row: MapSearchResult) => {
      setSearchInput("");
      setSearchOpen(false);
      setMobileSearchOpen(false);
      setDebouncedSearch("");
      setLocationPin(null);
      const existing = displayedPoints.find((p) => p.id === row.map_id);
      if (!existing) {
        const isLogin = row.kind === "login";
        const isInfra = row.kind !== "login" && row.kind !== "equipment";
        setDetailFallback({
          id: row.map_id,
          description: row.label,
          category: row.category ?? searchKindLabel(row.kind),
          lat: row.lat,
          lng: row.lng,
          status: isLogin ? "connection" : isInfra ? "infra" : "online",
          login: isLogin ? row.label.replace(/^.*\(([^)]+)\)\s*$/, "$1") : undefined,
          mapKind: isLogin ? "connection" : isInfra ? (row.kind as InfraMapKind) : "equipment",
        });
      } else {
        setDetailFallback(null);
      }
      setSelId(row.map_id);
      userPickedTab.current = true;
      setView("mapa");
      if (row.kind === "login") {
        setShowConnections(true);
        setInfraPanelOpen(false);
        setDetailModalOpen(true);
      } else if (row.kind === "equipment") {
        setShowEquipment(true);
        setInfraPanelOpen(false);
        setDetailModalOpen(true);
      } else {
        if (row.kind === "cto") setShowCtos(true);
        else if (row.kind === "cable") setShowCables(true);
        else if (row.kind === "splice_box") setShowSpliceBoxes(true);
        else if (row.kind === "pole") setShowPoles(true);
        else if (row.kind === "project") setShowProjects(true);
        else if (row.kind === "pop") setShowPops(true);
        setInfraPanelOpen(true);
        setDetailModalOpen(false);
      }
      setFlyTo({ lat: row.lat, lng: row.lng, zoom: 17 });
      setFlyKey((k) => k + 1);
    },
    [displayedPoints],
  );

  const selectLocateResult = useCallback((hit: MapLocateHit) => {
    setSearchInput("");
    setSearchOpen(false);
    setMobileSearchOpen(false);
    setDebouncedSearch("");
    setSelId(null);
    setDetailModalOpen(false);
    setInfraPanelOpen(false);
    setLocationPin({
      lat: hit.lat,
      lng: hit.lng,
      label: hit.display || hit.label || "Localização",
    });
    userPickedTab.current = true;
    setView("mapa");
    setFlyTo({ lat: hit.lat, lng: hit.lng, zoom: 17 });
    setFlyKey((k) => k + 1);
    setMapToast({ ok: true, text: `Localizado: ${hit.display || hit.label}` });
  }, []);

  const flyToLocality = useCallback(async () => {
    if (!localityFlyId) return;
    setLocalityFlyPending(true);
    setLocalityFlyNote(null);
    try {
      const r = await apiFetch<{ found?: boolean; lat?: number; lng?: number; name?: string; note?: string }>(
        `/api/v1/map/locality-center?locality_id=${encodeURIComponent(localityFlyId)}`,
      );
      if (!r.found || r.lat == null || r.lng == null) {
        setLocalityFlyNote(r.note ?? "Sem coordenadas para esta localidade.");
        return;
      }
      userPickedTab.current = true;
      setView("mapa");
      setFlyTo({ lat: r.lat, lng: r.lng, zoom: 13 });
      setFlyKey((k) => k + 1);
      setFilterModalOpen(false);
      setMapToast({ ok: true, text: `Mapa centrado em ${r.name ?? "localidade"}.` });
    } catch (e) {
      setLocalityFlyNote(e instanceof Error ? e.message : "Falha ao localizar localidade.");
    } finally {
      setLocalityFlyPending(false);
    }
  }, [localityFlyId]);

  const projectFitDoneRef = useRef<string>("");

  useEffect(() => {
    const pid = projectFilterId.trim();
    if (!pid) {
      projectFitDoneRef.current = "";
      return;
    }
    setShowCtos(true);
    setShowCables(true);
    setShowSpliceBoxes(true);
    setShowPoles(true);
    setShowProjects(true);
    userPickedTab.current = true;
    setView("mapa");

    let cancelled = false;
    void (async () => {
      try {
        const r = await apiFetch<{ found?: boolean; lat?: number; lng?: number; description?: string; note?: string }>(
          `/api/v1/map/project-center?project_id=${encodeURIComponent(pid)}`,
        );
        if (cancelled) return;
        if (!r.found || r.lat == null || r.lng == null) {
          setMapToast({ ok: false, text: r.note ?? "Projeto sem coordenadas no mapa." });
          return;
        }
        setFlyTo({ lat: r.lat, lng: r.lng, zoom: 14 });
        setFlyKey((k) => k + 1);
        setMapToast({
          ok: true,
          text: `A mostrar só o projeto${r.description ? `: ${r.description}` : ""}.`,
        });
      } catch (e) {
        if (!cancelled) {
          setMapToast({ ok: false, text: e instanceof Error ? e.message : "Falha ao localizar o projeto." });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectFilterId]);

  useEffect(() => {
    const pid = projectFilterId.trim();
    if (!pid || infraPts.isFetching || !infraPts.data) return;
    if (projectFitDoneRef.current === pid) return;
    const n = Array.isArray(infraPts.data.points) ? infraPts.data.points.length : 0;
    if (n === 0) return;
    projectFitDoneRef.current = pid;
    setFitBoundsVersion((v) => v + 1);
  }, [projectFilterId, infraPts.isFetching, infraPts.data]);

  useEffect(() => {
    const pid = projectFilterId.trim();
    if (!pid || !projectsList.data?.projects) return;
    const p = projectsList.data.projects.find((x) => x.id === pid);
    if (p?.status === "inativo") setProjectFilterId("");
  }, [projectFilterId, projectsList.data]);

  const filterActiveCount = useMemo(() => {
    let n = 0;
    if (popId) n++;
    if (category) n++;
    if (projectFilterId) n++;
    if (!showEquipment || showConnections || !showCtos || !showCables || !showPops || showSpliceBoxes || showPoles || showProjects) n++;
    if (displayMode !== "cluster") n++;
    return n;
  }, [
    popId,
    category,
    projectFilterId,
    showEquipment,
    showConnections,
    showCtos,
    showCables,
    showSpliceBoxes,
    showPoles,
    showProjects,
    showPops,
    displayMode,
  ]);

  const listPageCount = Math.max(1, Math.ceil(displayedPoints.length / MAP_LIST_PAGE_SIZE));
  const safeListPage = Math.min(listPage, listPageCount - 1);
  const listPageRows = useMemo(
    () => displayedPoints.slice(safeListPage * MAP_LIST_PAGE_SIZE, safeListPage * MAP_LIST_PAGE_SIZE + MAP_LIST_PAGE_SIZE),
    [displayedPoints, safeListPage],
  );

  const popsOptions = useMemo(() => {
    const raw = pops.data?.pops;
    return Array.isArray(raw) ? raw : [];
  }, [pops.data?.pops]);


  useEffect(() => {
    setFitBoundsVersion((v) => v + 1);
    setListPage(0);
  }, [popId, category, projectFilterId, showConnections, showEquipment, showCtos, showCables, showSpliceBoxes, showPoles, showProjects, showPops]);

  useEffect(() => {
    setListPage(0);
  }, [displayedPoints.length]);

  useEffect(() => {
    if (!mapToast) return;
    const t = window.setTimeout(() => setMapToast(null), 10_000);
    return () => window.clearTimeout(t);
  }, [mapToast]);

  useEffect(() => {
    if (!addMenuOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (addMenuRef.current && !addMenuRef.current.contains(e.target as Node)) {
        setAddMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [addMenuOpen]);

  useEffect(() => {
    if (!addKind && !repositionTarget && !editingCable) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") cancelAddMode();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [addKind, repositionTarget, editingCable, cancelAddMode]);

  /** Só o total da API — não usar `filteredPoints`: filtro POP/categoria vazio não deve trocar para «Lista» e esconder o mapa. */
  useEffect(() => {
    if (userPickedTab.current || (pts.isPending && !pts.data)) return;
    const total = Array.isArray(pts.data?.points) ? pts.data.points.length : 0;
    setView((cur) => {
      const next = total > 0 ? "mapa" : "lista";
      return cur === next ? cur : next;
    });
  }, [pts.isPending, pts.data, pts.data?.points?.length]);

  if (pts.isPending && !pts.data) {
    return (
      <div style={{ color: "var(--text)", padding: "1rem 0" }}>
        <p>Carregando pontos…</p>
      </div>
    );
  }
  if (pts.isError && !pts.data) {
    return (
      <div className="msg msg--err" style={{ color: "var(--text)" }}>
        {(pts.error as Error).message}
      </div>
    );
  }

  return (
    <div className="map-page" style={{ color: "var(--text)", minHeight: "50vh" }}>
      {mapToast ? (
        <div className={`page-toast ${mapToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status">
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setMapToast(null)}>
            ×
          </button>
          {mapToast.text}
        </div>
      ) : null}
      <div className="page-heading">
        <h1>
          Mapa
          <InfoHint label="Como usar o mapa">
            <p>
              Equipamentos com coordenadas e infraestrutura (CTOs, cabos, etc.) apenas na área visível, com limite por
              zoom para manter o mapa fluido. Use o botão <strong>+</strong> no mapa para adicionar elementos.
            </p>
            <p>
              Na pesquisa pode usar nome de equipamento/CTO, endereço, coordenadas (<span className="mono">-21.40, -42.19</span>)
              ou link do Google Maps.
            </p>
            <p>Seleccione uma CTO para abrir o painel lateral (localização e edição). Filtros no ícone de filtro.</p>
          </InfoHint>
        </h1>
        <span className="map-page__points-pill">
          <PageCountPill label="Pontos visíveis" count={displayedPoints.length} />
        </span>
        {showInfrastructure && infraPts.data?.truncated ? (
          <span className="map-page__infra-zoom-hint" style={{ fontSize: 11, color: "var(--muted)" }}>
            Infra limitada neste zoom
            {infraPts.data.limit != null ? ` (máx. ${infraPts.data.limit})` : ""} — aproxime o mapa para ver mais CTOs
          </span>
        ) : null}
        {showConnections && connTotal != null ? (
          <span className="map-page__conn-hint" style={{ fontSize: 11, color: "var(--muted)" }}>
            Conexões na área: {connPts.data?.points?.length ?? 0}
            {connTotal > 0 ? ` / ${connTotal} com coordenadas` : ""}
            {connLimit != null && connLimit > 0 ? ` (limite ${connLimit} neste zoom)` : ""}
            {connTruncated ? " — aproxime o mapa para ver mais" : ""}
            {connectionClusterForced && displayMode !== "cluster" ? " · logins agrupados por desempenho" : ""}
          </span>
        ) : null}
      </div>

      <div className="card map-toolbar" style={{ marginBottom: 12 }}>
        <div className="row map-toolbar__row" style={{ flexWrap: "wrap", gap: 10, alignItems: "flex-end" }}>
          <button
            type="button"
            className={`btn btn--icon btn--icon-menu map-toolbar__search-toggle${mobileSearchOpen || searchOpen ? " btn--primary" : ""}`}
            title="Pesquisar no mapa"
            aria-label="Pesquisar no mapa"
            aria-expanded={mobileSearchOpen || searchOpen}
            onClick={() => {
              setMobileSearchOpen((o) => !o);
              setSearchOpen(true);
            }}
          >
            <Search size={18} aria-hidden />
          </button>
          <div
            ref={searchWrapRef}
            className={`map-toolbar__search${mobileSearchOpen ? " map-toolbar__search--open" : ""}`}
            style={{ position: "relative", flex: "2 1 280px", minWidth: 220 }}
          >
            <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span className="map-toolbar__search-label" style={{ fontSize: 12, color: "var(--muted)" }}>
                Pesquisar no mapa
              </span>
              <input
                className="input"
                type="search"
                placeholder="Endereço, coords, link Maps, CTO, equipamento…"
                value={searchInput}
                onChange={(e) => {
                  setSearchInput(e.target.value);
                  setSearchOpen(true);
                  setMobileSearchOpen(true);
                }}
                onFocus={() => {
                  setSearchOpen(true);
                  setMobileSearchOpen(true);
                }}
                autoComplete="off"
              />
            </label>
            {searchOpen && (debouncedSearch.length >= 2 || searchInput.trim().length >= 2 || looksLikeHTTPURL(searchInput)) ? (
              <div
                className="card map-toolbar__search-dd"
                style={{
                  position: "absolute",
                  top: "100%",
                  left: 0,
                  right: 0,
                  zIndex: 40,
                  marginTop: 4,
                  maxHeight: 320,
                  overflow: "auto",
                  padding: 0,
                  boxShadow: "0 8px 24px rgba(0,0,0,.18)",
                }}
              >
                <div style={{ padding: "8px 10px", borderBottom: "1px solid var(--border)", fontSize: 11, color: "var(--muted)" }}>
                  Prefixo: <span className="mono">cto:</span> <span className="mono">login:</span> · coords · link Maps · endereço
                </div>
                {(mapLocate.isFetching || mapSearch.isFetching) && (
                  <p style={{ padding: 10, fontSize: 12, margin: 0 }}>A pesquisar…</p>
                )}
                {(mapLocate.data?.results?.length ?? 0) > 0 ? (
                  <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
                    {mapLocate.data!.results.map((hit, idx) => (
                      <li key={`loc-${idx}-${hit.lat}-${hit.lng}`}>
                        <button
                          type="button"
                          className="btn"
                          style={{
                            width: "100%",
                            textAlign: "left",
                            borderRadius: 0,
                            border: "none",
                            borderBottom: "1px solid var(--border)",
                            padding: "8px 10px",
                            fontSize: 12,
                            background: "transparent",
                          }}
                          onClick={() => selectLocateResult(hit)}
                        >
                          <span style={{ fontWeight: 600 }}>
                            {hit.source === "coords" ? "Coordenadas" : hit.source === "maps_url" ? "Link do mapa" : "Endereço"}
                          </span>
                          <span style={{ display: "block", color: "var(--muted)", marginTop: 2 }}>
                            {hit.label}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : null}
                {mapLocate.data?.note ? (
                  <p style={{ padding: 10, fontSize: 12, margin: 0, color: "var(--muted)" }}>{mapLocate.data.note}</p>
                ) : null}
                {entitySearchEnabled ? (
                  (mapSearch.data?.results?.length ?? 0) === 0 && !mapSearch.isFetching && (mapLocate.data?.results?.length ?? 0) === 0 ? (
                    <p style={{ padding: 10, fontSize: 12, margin: 0, color: "var(--muted)" }}>Nenhum resultado.</p>
                  ) : (
                    <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
                      {(mapSearch.data?.results ?? []).map((row) => (
                        <li key={`${row.kind}-${row.id}`}>
                          <button
                            type="button"
                            className="btn"
                            style={{
                              width: "100%",
                              textAlign: "left",
                              borderRadius: 0,
                              border: "none",
                              borderBottom: "1px solid var(--border)",
                              padding: "8px 10px",
                              fontSize: 12,
                              background: "transparent",
                            }}
                            onClick={() => selectSearchResult(row)}
                          >
                            <span style={{ fontWeight: 600 }}>
                              {row.project_name?.trim() ? `${row.project_name.trim()} — ${row.label}` : row.label}
                            </span>
                            <span style={{ display: "block", color: "var(--muted)", marginTop: 2 }}>
                              {searchKindLabel(row.kind)}
                              {row.category ? ` · ${row.category}` : ""}
                            </span>
                          </button>
                        </li>
                      ))}
                    </ul>
                  )
                ) : null}
                {!entitySearchEnabled &&
                !mapLocate.isFetching &&
                (mapLocate.data?.results?.length ?? 0) === 0 &&
                !mapLocate.data?.note ? (
                  <p style={{ padding: 10, fontSize: 12, margin: 0, color: "var(--muted)" }}>Nenhum resultado.</p>
                ) : null}
              </div>
            ) : null}
          </div>
          <MapFilterButton activeCount={filterActiveCount} onClick={() => setFilterModalOpen(true)} />
          <button
            type="button"
            className={`btn btn--icon btn--icon-menu${geoTracking ? " btn--primary" : ""}`}
            title={geoTracking ? "Parar localização GPS" : "Usar minha localização (CTOs próximas)"}
            aria-label={geoTracking ? "Parar localização GPS" : "Usar minha localização"}
            aria-pressed={geoTracking}
            onClick={() => {
              if (geoTracking) {
                stopGeoTracking();
                setNearestCtos([]);
                setMapToast({ ok: true, text: "Localização GPS desligada." });
              } else {
                startGeoTracking();
              }
            }}
          >
            <LocateFixed size={18} strokeWidth={2} aria-hidden />
          </button>
          <MapSettingsButton onClick={() => setSettingsModalOpen(true)} />
          <button
            type="button"
            className="btn btn--icon btn--icon-menu"
            title="Recarregar coordenadas e ajustar o mapa"
            aria-label="Recarregar coordenadas e ajustar o mapa"
            disabled={pts.isFetching}
            onClick={async () => {
              try {
                const r = await pts.refetch();
                if (r.error) {
                  setMapToast({ ok: false, text: (r.error as Error).message || "Erro ao actualizar o mapa." });
                } else {
                  setMapToast({ ok: true, text: "Mapa actualizado com os filtros actuais." });
                }
              } catch (e) {
                setMapToast({ ok: false, text: e instanceof Error ? e.message : "Erro ao actualizar o mapa." });
              } finally {
                setFitBoundsVersion((v) => v + 1);
              }
            }}
          >
            <span className={pts.isFetching ? "map-refresh-spin" : undefined} style={{ display: "inline-flex" }}>
              <IconRefresh />
            </span>
          </button>
        </div>
      </div>

      <MapFilterModal
        open={filterModalOpen}
        onClose={() => setFilterModalOpen(false)}
        displayMode={displayMode}
        onDisplayMode={setDisplayMode}
        popId={popId}
        onPopId={setPopId}
        popsOptions={popsOptions}
        popsPending={pops.isPending}
        popsError={pops.isError}
        category={category}
        onCategory={setCategory}
        projectId={projectFilterId}
        onProjectId={setProjectFilterId}
        projectsOptions={mapProjects}
        showEquipment={showEquipment}
        onShowEquipment={setShowEquipment}
        showCtos={showCtos}
        onShowCtos={setShowCtos}
        showCables={showCables}
        onShowCables={setShowCables}
        showConnections={showConnections}
        onShowConnections={setShowConnections}
        showSpliceBoxes={showSpliceBoxes}
        onShowSpliceBoxes={setShowSpliceBoxes}
        showPoles={showPoles}
        onShowPoles={setShowPoles}
        showProjects={showProjects}
        onShowProjects={setShowProjects}
        showPops={showPops}
        onShowPops={setShowPops}
        ctoColorByFeed={ctoColorByFeed}
        onCtoColorByFeed={setCtoColorByFeed}
        localities={localities.data?.localities ?? []}
        localityFlyId={localityFlyId}
        onLocalityFlyId={setLocalityFlyId}
        onFlyToLocality={() => void flyToLocality()}
        localityFlyPending={localityFlyPending}
        localityFlyNote={localityFlyNote}
      />

      <MapSettingsModal
        open={settingsModalOpen}
        onClose={() => {
          setMapPrefsDraft(mapColorsFromAppearance(uiAppearance.data));
          setMapIconsDraft(mapIconsFromAppearance(uiAppearance.data));
          setSettingsModalOpen(false);
        }}
        colors={mapPrefsDraft}
        onColorsChange={setMapPrefsDraft}
        icons={mapIconsDraft}
        onIconsChange={setMapIconsDraft}
        onSave={() => saveMapPrefs.mutate()}
        savePending={saveMapPrefs.isPending}
      />

      <MapDetailModal
        open={detailModalOpen && !isInfraPoint}
        onClose={() => setDetailModalOpen(false)}
        selId={selId}
        selPoint={selPoint}
        isConnPoint={isConnPoint}
        isInfraPoint={false}
        detailLoading={detail.isLoading}
        detailError={detail.error as Error | null}
        detail={detail.data ?? null}
      />

      <div className="tabs">
        <button
          type="button"
          className={view === "lista" ? "active" : ""}
          onClick={() => {
            userPickedTab.current = true;
            setView("lista");
          }}
        >
          Lista
        </button>
        <button
          type="button"
          className={view === "mapa" ? "active" : ""}
          onClick={() => {
            userPickedTab.current = true;
            setView("mapa");
          }}
        >
          Mapa OSM
        </button>
      </div>

      <div style={{ marginTop: 12 }}>
        {view === "lista" ? (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Descrição</th>
                    <th>Categoria</th>
                    <th>Lat</th>
                    <th>Lng</th>
                    <th>Status</th>
                    <th>OSM</th>
                  </tr>
                </thead>
                <tbody>
                  {listPageRows.map((p) => (
                    <tr
                      key={p.id}
                      className={selId === p.id ? "row-interactive row-interactive--selected" : "row-interactive"}
                      onClick={() => openPointDetail(p.id)}
                    >
                      <td>{p.description}</td>
                      <td>{p.category}</td>
                      <td className="mono">{p.lat}</td>
                      <td className="mono">{p.lng}</td>
                      <td>
                        <span className={`badge ${p.status === "online" ? "badge--ok" : p.status === "offline" ? "badge--err" : "badge--off"}`}>{p.status}</span>
                      </td>
                      <td>
                        <a
                          href={`https://www.openstreetmap.org/?mlat=${p.lat}&mlon=${p.lng}#map=15/${p.lat}/${p.lng}`}
                          target="_blank"
                          rel="noreferrer"
                          onClick={(e) => e.stopPropagation()}
                        >
                          abrir
                        </a>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {displayedPoints.length > MAP_LIST_PAGE_SIZE ? (
                <div className="row conn-table-pager" style={{ marginTop: 10, justifyContent: "space-between", flexWrap: "wrap", gap: 8 }}>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>
                    {safeListPage * MAP_LIST_PAGE_SIZE + 1}–{Math.min(displayedPoints.length, (safeListPage + 1) * MAP_LIST_PAGE_SIZE)} de{" "}
                    {displayedPoints.length}
                  </span>
                  <div className="row" style={{ gap: 6 }}>
                    <button type="button" className="btn" disabled={safeListPage <= 0} onClick={() => setListPage((p) => Math.max(0, p - 1))}>
                      Anterior
                    </button>
                    <button
                      type="button"
                      className="btn"
                      disabled={safeListPage >= listPageCount - 1}
                      onClick={() => setListPage((p) => Math.min(listPageCount - 1, p + 1))}
                    >
                      Seguinte
                    </button>
                  </div>
                </div>
              ) : null}
            </div>
          ) : (
            <MapSectionErrorBoundary>
              <div className={`map-workspace${infraPanelOpen && isInfraPoint ? " map-workspace--with-panel" : ""}`}>
                <div className="map-workspace__map">
                  {canEditMap && !addKind && !editingCable && !repositionTarget ? (
                    <div className="map-edit-toggle">
                      <button
                        type="button"
                        className={`map-edit-toggle__btn${mapEditMode ? " map-edit-toggle__btn--active" : ""}`}
                        aria-pressed={mapEditMode}
                        title={mapEditMode ? "Sair do modo edição" : "Entrar no modo edição"}
                        onClick={toggleMapEditMode}
                      >
                        <Pencil size={15} strokeWidth={2.25} />
                        {mapEditMode ? "Modo edição" : "Editar"}
                      </button>
                      <button
                        type="button"
                        className="map-edit-toggle__btn"
                        title="Importar KML/KMZ ou substituir um projeto existente"
                        onClick={() => setKmlImportOpen(true)}
                      >
                        <FileUp size={15} strokeWidth={2.25} />
                        Importar
                      </button>
                    </div>
                  ) : null}
                  {canEditMap && !mapEditMode && !addKind && !editingCable && !repositionTarget ? (
                    <div className="map-add" ref={addMenuRef}>
                      <button
                        type="button"
                        className={`map-add__btn${addKind ? " map-add__btn--active" : ""}`}
                        aria-label="Adicionar no mapa"
                        aria-expanded={addMenuOpen}
                        onClick={() => setAddMenuOpen((o) => !o)}
                      >
                        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" aria-hidden>
                          <path d="M12 5v14M5 12h14" strokeLinecap="round" />
                        </svg>
                      </button>
                      {addMenuOpen ? (
                        <div className="map-add__menu" role="menu">
                          {(
                            [
                              ["cto", "CTO"],
                              ["cable", "Cabo"],
                              ["splice_box", "Caixa de emenda"],
                              ["pole", "Poste"],
                              ["pop", "POP"],
                              ["project", "Projeto"],
                            ] as const
                          ).map(([id, label]) => (
                            <button key={id} type="button" role="menuitem" className="map-add__item" onClick={() => startAddKind(id)}>
                              {label}
                            </button>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  ) : null}

                  {editingCable ? (
                    <div className="map-cable-toolbar" role="status">
                      <span>
                        Editar trajeto do cabo ({draftPath.length} ponto{draftPath.length === 1 ? "" : "s"}). Arraste vértices ou
                        clique para adicionar.
                      </span>
                      <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                        <button
                          type="button"
                          className="btn btn--sm"
                          disabled={draftPath.length === 0}
                          onClick={() => setDraftPath((p) => p.slice(0, -1))}
                        >
                          Desfazer
                        </button>
                        <button type="button" className="btn btn--sm" onClick={cancelAddMode}>
                          Cancelar
                        </button>
                        <button
                          type="button"
                          className="btn btn--sm btn--primary"
                          disabled={draftPath.length < 2}
                          onClick={() => void saveEditedCablePath()}
                        >
                          Guardar trajeto
                        </button>
                      </div>
                    </div>
                  ) : repositionTarget ? (
                    <div className="map-place-hint" role="status">
                      <span>
                        Reposicionar: clique no mapa ou arraste o marcador. Esc cancela.
                      </span>
                      <button type="button" className="btn btn--sm" onClick={cancelAddMode}>
                        Cancelar
                      </button>
                    </div>
                  ) : addKind === "cable" ? (
                    <div className="map-cable-toolbar" role="status">
                      <span>
                        Desenhe o trajeto ({draftPath.length} ponto{draftPath.length === 1 ? "" : "s"}). Clique no mapa para
                        adicionar.
                      </span>
                      <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                        <button
                          type="button"
                          className="btn btn--sm"
                          disabled={draftPath.length === 0}
                          onClick={() => setDraftPath((p) => p.slice(0, -1))}
                        >
                          Desfazer
                        </button>
                        <button type="button" className="btn btn--sm" onClick={cancelAddMode}>
                          Cancelar
                        </button>
                        <button
                          type="button"
                          className="btn btn--sm btn--primary"
                          disabled={draftPath.length < 2}
                          onClick={() => saveCablePath()}
                        >
                          Salvar trajeto
                        </button>
                      </div>
                    </div>
                  ) : addKind ? (
                    <div className="map-place-hint" role="status">
                      <span>
                        Clique no mapa para posicionar: <strong>{INFRA_MAP_KIND_LABELS[addKind] ?? addKind}</strong>
                      </span>
                      <button type="button" className="btn btn--sm" onClick={cancelAddMode}>
                        Cancelar
                      </button>
                    </div>
                  ) : mapEditMode ? (
                    <div className="map-place-hint" role="status">
                      <span>
                        Modo edição activo — seleccione um elemento para alterar a descrição, reposicionar, ocultar ou excluir.
                      </span>
                      <button type="button" className="btn btn--sm" onClick={toggleMapEditMode}>
                        Sair
                      </button>
                    </div>
                  ) : null}

                  {locationPin ? (
                    <div className="map-hidden-chip map-locate-chip">
                      <span>{locationPin.label?.trim() || "Ponto da pesquisa"}</span>
                      <button
                        type="button"
                        className="btn btn--sm"
                        onClick={() => {
                          setLocationPin(null);
                          setMapToast({ ok: true, text: "Marcador da pesquisa removido." });
                        }}
                      >
                        Remover
                      </button>
                    </div>
                  ) : null}

                  {hiddenMapIds.size > 0 ? (
                    <div className="map-hidden-chip" style={locationPin ? { top: 108 } : undefined}>
                      <span>{hiddenMapIds.size} oculto{hiddenMapIds.size === 1 ? "" : "s"}</span>
                      <button type="button" className="btn btn--sm" onClick={() => setHiddenMapIds(new Set())}>
                        Mostrar todos
                      </button>
                    </div>
                  ) : null}

                  <EquipmentMap
                    points={mapPoints}
                    displayMode={displayMode}
                    mapHeight="min(72vh, 720px)"
                    mapColors={mapColors}
                    mapIconStyles={mapIconStyles}
                    connectionClusterForced={connectionClusterForced}
                    highlightedId={mapHighlightIds}
                    userLocation={userLocation}
                    locationPin={locationPin}
                    onClearLocationPin={() => {
                      setLocationPin(null);
                      setMapToast({ ok: true, text: "Marcador da pesquisa removido." });
                    }}
                    placeMode={placeMode}
                    draftPath={draftPath}
                    mapEditMode={mapEditMode}
                    editingCableMapId={editingCable?.mapId ?? null}
                    repositionPreview={repositionPreview}
                    onDraftVertexMove={(index, lat, lng) => {
                      setDraftPath((prev) => prev.map((p, i) => (i === index ? { lat, lng } : p)));
                    }}
                    onMapClick={handleMapPlaceClick}
                    onSelectDevice={(id) => {
                      if (id) openPointDetail(id);
                    }}
                    onOpenSplitter={(id) => {
                      if (!id) return;
                      setAutoOpenSplitter(true);
                      openPointDetail(id);
                    }}
                    onOpenCableFibers={(id) => {
                      if (!id) return;
                      setAutoOpenCableFibers(true);
                      openPointDetail(id);
                    }}
                    onOpenSplice={(id) => {
                      if (!id) return;
                      setAutoOpenSplice(true);
                      openPointDetail(id);
                    }}
                    onEditPosition={
                      canEditMap && mapEditMode
                        ? (id) => {
                            if (!id) return;
                            setAutoOpenEdit(true);
                            openPointDetail(id, false);
                          }
                        : undefined
                    }
                    onCopyCoords={(lat, lng) => {
                      const text = `${lat.toFixed(6)}, ${lng.toFixed(6)}`;
                      void navigator.clipboard.writeText(text).then(
                        () => setMapToast({ ok: true, text: `Coordenadas copiadas: ${text}` }),
                        () => setMapToast({ ok: false, text: "Não foi possível copiar as coordenadas." }),
                      );
                    }}
                    flyTo={flyTo}
                    flyKey={flyKey}
                    fitBoundsVersion={fitBoundsVersion}
                    onBoundsChange={onMapBoundsChange}
                  />
                  {geoTracking || nearestCtos.length > 0 || geoError ? (
                    <div className="map-nearest-panel" role="region" aria-label="CTOs próximas">
                      <div className="map-nearest-panel__title">
                        <span>{geoTracking ? "CTOs mais próximas" : "CTOs próximas"}</span>
                        {geoTracking ? (
                          <button type="button" className="btn btn--sm" onClick={() => { stopGeoTracking(); setNearestCtos([]); }}>
                            Parar
                          </button>
                        ) : null}
                      </div>
                      {geoError ? <p className="msg msg--err" style={{ margin: "0 0 8px", fontSize: 12 }}>{geoError}</p> : null}
                      {geoTracking && !userLocation && !geoError ? (
                        <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>A obter GPS…</p>
                      ) : null}
                      {nearestCtos.length > 0 ? (
                        <ul className="map-nearest-panel__list">
                          {nearestCtos.map((c, i) => (
                            <li key={c.id}>
                              <button
                                type="button"
                                className={`map-nearest-panel__item${selId === c.map_id ? " map-nearest-panel__item--active" : ""}`}
                                onClick={() => openPointDetail(c.map_id)}
                              >
                                <span className="map-nearest-panel__rank">{i + 1}</span>
                                <span className="map-nearest-panel__meta">
                                  <span className="map-nearest-panel__name">
                                    {c.description || `CTO ${c.display_number}`}
                                  </span>
                                  <span className="map-nearest-panel__dist">
                                    {formatDistanceMeters(c.distance_m)}
                                    {c.splitter ? ` · ${c.splitter}` : ""}
                                  </span>
                                </span>
                              </button>
                            </li>
                          ))}
                        </ul>
                      ) : geoTracking && userLocation ? (
                        <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>Nenhuma CTO com coordenadas encontrada.</p>
                      ) : null}
                    </div>
                  ) : null}
                </div>
                {infraPanelOpen && isInfraPoint ? (
                  <MapInfraSidePanel
                    open
                    mapId={selId}
                    mapEditMode={mapEditMode}
                    autoOpenSplitter={autoOpenSplitter}
                    onSplitterAutoOpened={() => setAutoOpenSplitter(false)}
                    autoOpenCableFibers={autoOpenCableFibers}
                    onCableFibersAutoOpened={() => setAutoOpenCableFibers(false)}
                    autoOpenSplice={autoOpenSplice}
                    onSpliceAutoOpened={() => setAutoOpenSplice(false)}
                    autoOpenEdit={autoOpenEdit}
                    onEditAutoOpened={() => setAutoOpenEdit(false)}
                    fallback={
                      selPoint
                        ? {
                            description: selPoint.description,
                            lat: Number(selPoint.lat),
                            lng: Number(selPoint.lng),
                            category: selPoint.category,
                            splitter: selPoint.splitter,
                          }
                        : null
                    }
                    onClose={() => {
                      setInfraPanelOpen(false);
                      setAutoOpenSplitter(false);
                      setAutoOpenCableFibers(false);
                      setAutoOpenSplice(false);
                      setAutoOpenEdit(false);
                      setSelId(null);
                    }}
                    onHideFromMap={(id) => {
                      setHiddenMapIds((prev) => new Set(prev).add(id));
                      setInfraPanelOpen(false);
                      setSelId(null);
                      setMapToast({ ok: true, text: "Elemento oculto neste mapa (sessão actual)." });
                    }}
                    onStartReposition={(mapId, kind, entityId) => startReposition(mapId, kind, entityId)}
                    onDeleted={() => {
                      setMapToast({ ok: true, text: "Elemento excluído." });
                      setSelId(null);
                      setInfraPanelOpen(false);
                    }}
                    onSaved={(next) => {
                      setMapToast({ ok: true, text: "Elemento actualizado." });
                      setFlyTo({ lat: next.lat, lng: next.lng, zoom: 17 });
                      setFlyKey((k) => k + 1);
                      if (selId) {
                        setDetailFallback({
                          id: selId,
                          description: next.description,
                          category: selPoint?.category ?? "CTO",
                          lat: next.lat,
                          lng: next.lng,
                          status: selPoint?.status ?? "—",
                          mapKind: selPoint?.mapKind ?? "cto",
                        });
                      }
                    }}
                  />
                ) : null}
              </div>
            </MapSectionErrorBoundary>
          )}
      </div>

      <MapPlaceElementModal
        session={placeSession}
        onClose={() => setPlaceSession(null)}
        onSaved={(info) => {
          setPlaceSession(null);
          const mapId = `infra-${info.kind}-${info.id}`;
          setMapToast({
            ok: true,
            text: `${INFRA_MAP_KIND_LABELS[info.kind] ?? info.kind} guardado.`,
          });
          setFlyTo({ lat: info.lat, lng: info.lng, zoom: 17 });
          setFlyKey((k) => k + 1);
          openPointDetail(mapId, false);
        }}
      />
      {canEditMap ? (
        <MapProjectKmlImport
          open={kmlImportOpen}
          defaultProjectId={projectFilterId}
          projects={mapProjects}
          onClose={() => setKmlImportOpen(false)}
          onImported={(info) => {
            const n =
              (info.imported.ctos ?? 0) +
              (info.imported.splice_boxes ?? 0) +
              (info.imported.poles ?? 0) +
              (info.imported.cables ?? 0) +
              (info.imported.pops ?? 0);
            setMapToast({
              ok: true,
              text: info.replaced
                ? `Projeto substituído (${n} elemento(s) importados).`
                : `Projeto criado (${n} elemento(s) importados).`,
            });
            if (info.projectId) {
              setProjectFilterId(info.projectId);
              setShowCtos(true);
              setShowCables(true);
              setShowSpliceBoxes(true);
              setShowPoles(true);
              setShowPops(true);
            }
          }}
        />
      ) : null}
    </div>
  );
}
