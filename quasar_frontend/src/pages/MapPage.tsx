import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { EquipmentMap, DEFAULT_MAP_COLORS, type MapBounds, type MapDisplayMode, type MapLatLng, type MapPlaceMode, type MapPoint } from "../components/EquipmentMap";
import { MapDetailModal } from "../components/MapDetailModal";
import { MapFilterButton, MapFilterModal } from "../components/MapFilterModal";
import { MapInfraSidePanel, parseInfraMapId } from "../components/MapInfraSidePanel";
import { MapPlaceElementModal, type MapPlaceSession, type PlaceableKind } from "../components/MapPlaceElementModal";
import { MapSettingsButton, MapSettingsModal } from "../components/MapSettingsModal";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { CTO_MAP_PIN_COLOR, DEFAULT_MAP_ICON_STYLES, INFRA_MAP_KIND_LABELS, isInfraMapKind, type InfraMapKind, type MapIconStyles } from "../lib/mapInfrastructureIcons";
import { fiberSpecByName } from "../lib/fiberSplitter";
import { apiFetch } from "../lib/api";
import { can, isAdminUser } from "../lib/auth";
import { type MonitoringStateSync, monitoringPollMs, useMonitoringLiveSync } from "../lib/monitoringLiveSync";
import { queryKeys } from "../lib/queryKeys";
import { fetchUiAppearance, mapColorsFromAppearance, mapIconsFromAppearance, type MapAppearanceColors } from "../lib/uiAppearance";

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
  lat: number;
  lng: number;
  map_id: string;
};

function parseMapSearchInput(raw: string): { q: string; type: string } {
  const s = raw.trim();
  const rules: [RegExp, string][] = [
    [/^cto\s*:\s*/i, "cto"],
    [/^poste\s*:\s*/i, "pole"],
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
  const onMapBoundsChange = useCallback((b: MapBounds) => setMapBounds(b), []);
  const qc = useQueryClient();

  const placeMode: MapPlaceMode = addKind === "cable" ? "cable" : addKind ? "place" : null;

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

  const mapSearch = useQuery({
    queryKey: ["map-search", debouncedSearch, searchTypeFilter],
    enabled: debouncedSearch.length >= 2,
    queryFn: () => {
      const params = new URLSearchParams({ q: debouncedSearch });
      if (searchTypeFilter) params.set("type", searchTypeFilter);
      return apiFetch<{ results: MapSearchResult[] }>(`/api/v1/map/search?${params.toString()}`);
    },
  });

  useEffect(() => {
    if (!searchOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (searchWrapRef.current && !searchWrapRef.current.contains(e.target as Node)) setSearchOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [searchOpen]);

  const monState = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<MonitoringStateSync>("/api/v1/monitoring/state"),
    refetchInterval: (q) => monitoringPollMs(5000, q.state.data?.is_running),
  });
  useMonitoringLiveSync(monState.data, { map: true });

  const connPts = useQuery({
    queryKey: [
      "map-connection-points",
      mapBounds?.minLat,
      mapBounds?.maxLat,
      mapBounds?.minLng,
      mapBounds?.maxLng,
      mapBounds?.zoom,
    ],
    queryFn: () => {
      const params = new URLSearchParams();
      if (mapBounds) {
        params.set("min_lat", String(mapBounds.minLat));
        params.set("max_lat", String(mapBounds.maxLat));
        params.set("min_lng", String(mapBounds.minLng));
        params.set("max_lng", String(mapBounds.maxLng));
        if (mapBounds.zoom != null) params.set("zoom", String(mapBounds.zoom));
      }
      const qs = params.toString();
      return apiFetch<{ points: ConnectionPoint[]; total?: number; truncated?: boolean; limit?: number }>(
        `/api/v1/map/connection-points${qs ? `?${qs}` : ""}`,
      );
    },
    enabled: showConnections && mapBounds != null,
    placeholderData: keepPreviousData,
  });

  const infraKinds = useMemo(() => {
    const kinds: string[] = [];
    if (showCtos) kinds.push("ctos");
    if (showCables) kinds.push("cables");
    if (showSpliceBoxes) kinds.push("splice_boxes");
    if (showPoles) kinds.push("poles");
    if (showProjects) kinds.push("projects");
    return kinds;
  }, [showCtos, showCables, showSpliceBoxes, showPoles, showProjects]);

  const showInfrastructure = infraKinds.length > 0;

  const projectsList = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: () => apiFetch<{ projects: { id: string; display_number: number; description: string }[] }>("/api/v1/commercial/network/projects"),
  });

  const infraPts = useQuery({
    queryKey: [
      "map-infrastructure-points",
      infraKinds.join(","),
      projectFilterId,
      projectFilterId ? "all" : mapBounds?.minLat,
      projectFilterId ? "all" : mapBounds?.maxLat,
      projectFilterId ? "all" : mapBounds?.minLng,
      projectFilterId ? "all" : mapBounds?.maxLng,
      projectFilterId ? "all" : mapBounds?.zoom,
    ],
    queryFn: () => {
      const params = new URLSearchParams();
      params.set("kinds", infraKinds.join(","));
      if (projectFilterId.trim()) params.set("project_id", projectFilterId.trim());
      // Sem filtro de projeto: limitar ao viewport. Com projeto: API ignora bbox.
      if (!projectFilterId.trim() && mapBounds) {
        params.set("min_lat", String(mapBounds.minLat));
        params.set("max_lat", String(mapBounds.maxLat));
        params.set("min_lng", String(mapBounds.minLng));
        params.set("max_lng", String(mapBounds.maxLng));
        if (mapBounds.zoom != null) params.set("zoom", String(mapBounds.zoom));
      }
      const qs = params.toString();
      return apiFetch<{ points: InfrastructurePoint[]; total?: number; truncated?: boolean }>(
        `/api/v1/map/infrastructure-points${qs ? `?${qs}` : ""}`,
      );
    },
    enabled: showInfrastructure && (projectFilterId.trim() !== "" || mapBounds != null),
    placeholderData: keepPreviousData,
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
  });

  const equipPoints = useMemo(() => (Array.isArray(pts.data?.points) ? pts.data.points : []), [pts.data?.points]);

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
    const infraRaw = showInfrastructure && Array.isArray(infraPts.data?.points) ? infraPts.data.points : [];
    const infra: Point[] = infraRaw
      .filter((p) => isInfraMapKind(p.point_type))
      .map((p) => {
        const splitterLabel = p.point_type === "cto" && p.splitter ? String(p.splitter).trim() : "";
        const ctoColor = ctoColorByFeed ? fiberSpecByName(p.fiber_color).hex : mapPrefsDraft.cto;
        const spliceColor = mapPrefsDraft.splice_box;
        return {
          id: `infra-${p.point_type}-${p.id}`,
          description: p.point_type === "cto" ? p.description : p.id_prefix ? `${p.id_prefix} ${p.display_number} — ${p.description}` : p.description,
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
    return [...equip, ...conn, ...infra];
  }, [
    equipPoints,
    connPts.data?.points,
    infraPts.data?.points,
    showConnections,
    showEquipment,
    showInfrastructure,
    projectFilterId,
    ctoColorByFeed,
    mapPrefsDraft.cto,
    mapPrefsDraft.splice_box,
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

  const mapPoints: MapPoint[] = useMemo(
    () =>
      displayedPoints.map((p) => ({
        id: p.id,
        description: p.description,
        lat: Number(p.lat),
        lng: Number(p.lng),
        ip: p.ip,
        category: p.category,
        status: p.status,
        mapKind: p.mapKind,
        markerColor: p.markerColor,
        mapLabel: p.mapLabel,
        splitter: p.splitter ?? null,
        path: p.path ?? null,
      })),
    [displayedPoints],
  );

  const cancelAddMode = useCallback(() => {
    setAddKind(null);
    setDraftPath([]);
    setAddMenuOpen(false);
  }, []);

  const enableLayerForKind = useCallback((kind: PlaceableKind) => {
    if (kind === "cto") setShowCtos(true);
    else if (kind === "cable") setShowCables(true);
    else if (kind === "splice_box") setShowSpliceBoxes(true);
    else if (kind === "pole") setShowPoles(true);
    else if (kind === "project") setShowProjects(true);
  }, []);

  const startAddKind = useCallback((kind: PlaceableKind) => {
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
      if (!addKind) return;
      if (addKind === "cable") {
        setDraftPath((prev) => [...prev, { lat, lng }]);
        return;
      }
      setPlaceSession({ mode: "create", kind: addKind, lat, lng });
      setAddKind(null);
    },
    [addKind],
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
      setDebouncedSearch("");
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
        setInfraPanelOpen(true);
        setDetailModalOpen(false);
      }
      setFlyTo({ lat: row.lat, lng: row.lng, zoom: 17 });
      setFlyKey((k) => k + 1);
    },
    [displayedPoints],
  );

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

  const filterActiveCount = useMemo(() => {
    let n = 0;
    if (popId) n++;
    if (category) n++;
    if (projectFilterId) n++;
    if (!showEquipment || showConnections || !showCtos || !showCables || showSpliceBoxes || showPoles || showProjects) n++;
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
  }, [popId, category, projectFilterId, showConnections, showEquipment, showCtos, showCables, showSpliceBoxes, showPoles, showProjects]);

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
    if (!addKind) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") cancelAddMode();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [addKind, cancelAddMode]);

  /** Só o total da API — não usar `filteredPoints`: filtro POP/categoria vazio não deve trocar para «Lista» e esconder o mapa. */
  useEffect(() => {
    if (userPickedTab.current || (pts.isPending && !pts.data)) return;
    const ptsArr = pts.data?.points;
    const total = Array.isArray(ptsArr) ? ptsArr.length : 0;
    if (total > 0) setView("mapa");
    else setView("lista");
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
              Equipamentos com coordenadas, CTOs e cabos da área visível (viewport), e opcionalmente logins ou outra
              infraestrutura. Use o botão <strong>+</strong> no mapa para adicionar elementos clicando na posição.
            </p>
            <p>Seleccione uma CTO para abrir o painel lateral (localização e edição). Filtros no ícone de filtro.</p>
          </InfoHint>
        </h1>
        <PageCountPill label="Pontos visíveis" count={displayedPoints.length} />
        {showConnections && connTotal != null ? (
          <span style={{ fontSize: 11, color: "var(--muted)" }}>
            Conexões na área: {connPts.data?.points?.length ?? 0}
            {connTotal > 0 ? ` / ${connTotal} com coordenadas` : ""}
            {connLimit != null && connLimit > 0 ? ` (limite ${connLimit} neste zoom)` : ""}
            {connTruncated ? " — aproxime o mapa para ver mais" : ""}
            {connectionClusterForced && displayMode !== "cluster" ? " · logins agrupados por desempenho" : ""}
          </span>
        ) : null}
      </div>

      <div className="card" style={{ marginBottom: 12 }}>
        <div className="row" style={{ flexWrap: "wrap", gap: 10, alignItems: "flex-end" }}>
          <div ref={searchWrapRef} style={{ position: "relative", flex: "2 1 280px", minWidth: 220 }}>
            <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Pesquisar no mapa</span>
              <input
                className="input"
                type="search"
                placeholder="Equipamento, login, CTO, poste… (ex.: cto:centro, login:joao)"
                value={searchInput}
                onChange={(e) => {
                  setSearchInput(e.target.value);
                  setSearchOpen(true);
                }}
                onFocus={() => setSearchOpen(true)}
                autoComplete="off"
              />
            </label>
            {searchOpen && (debouncedSearch.length >= 2 || searchInput.trim().length >= 2) ? (
              <div
                className="card"
                style={{
                  position: "absolute",
                  top: "100%",
                  left: 0,
                  right: 0,
                  zIndex: 40,
                  marginTop: 4,
                  maxHeight: 280,
                  overflow: "auto",
                  padding: 0,
                  boxShadow: "0 8px 24px rgba(0,0,0,.18)",
                }}
              >
                <div style={{ padding: "8px 10px", borderBottom: "1px solid var(--border)", fontSize: 11, color: "var(--muted)" }}>
                  Prefixos: <span className="mono">cto:</span> <span className="mono">poste:</span> <span className="mono">login:</span> <span className="mono">equip:</span>
                </div>
                {mapSearch.isFetching ? (
                  <p style={{ padding: 10, fontSize: 12, margin: 0 }}>A pesquisar…</p>
                ) : (mapSearch.data?.results?.length ?? 0) === 0 ? (
                  <p style={{ padding: 10, fontSize: 12, margin: 0, color: "var(--muted)" }}>Nenhum resultado.</p>
                ) : (
                  <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
                    {mapSearch.data!.results.map((row) => (
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
                          <span style={{ fontWeight: 600 }}>{row.label}</span>
                          <span style={{ color: "var(--muted)", marginLeft: 8 }}>{searchKindLabel(row.kind)}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ) : null}
          </div>
          <MapFilterButton activeCount={filterActiveCount} onClick={() => setFilterModalOpen(true)} />
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
        projectsOptions={projectsList.data?.projects ?? []}
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
                  {canEditMap ? (
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

                  {addKind === "cable" ? (
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
                  ) : null}

                  <EquipmentMap
                    points={mapPoints}
                    displayMode={displayMode}
                    mapHeight="min(72vh, 720px)"
                    mapColors={mapColors}
                    mapIconStyles={mapIconStyles}
                    connectionClusterForced={connectionClusterForced}
                    highlightedId={selId}
                    placeMode={placeMode}
                    draftPath={draftPath}
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
                      canEditMap
                        ? (id) => {
                            if (!id) return;
                            setAutoOpenEdit(true);
                            openPointDetail(id);
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
                </div>
                {infraPanelOpen && isInfraPoint ? (
                  <MapInfraSidePanel
                    open
                    mapId={selId}
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
                    onSaved={(next) => {
                      setMapToast({ ok: true, text: "CTO actualizada." });
                      setFlyTo({ lat: next.lat, lng: next.lng, zoom: 17 });
                      setFlyKey((k) => k + 1);
                      if (selId) {
                        setDetailFallback({
                          id: selId,
                          description: next.description,
                          category: "CTO",
                          lat: next.lat,
                          lng: next.lng,
                          status: "infra",
                          mapKind: "cto",
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
    </div>
  );
}
