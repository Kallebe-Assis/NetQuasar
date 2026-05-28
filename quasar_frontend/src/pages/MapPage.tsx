import { keepPreviousData, useQuery } from "@tanstack/react-query";
import React, { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { EquipmentMap, type MapDisplayMode, type MapPoint } from "../components/EquipmentMap";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";

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
const MAP_DEVICE_CATEGORIES = ["Concentrador", "Energia", "Mikrotik", "OLT", "Rádio", "Servidor", "Máquina Virtual", "Outros"] as const;

function fmtIso(s: string | null | undefined): string {
  if (!s?.trim()) return "—";
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString("pt-PT");
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
  const [fitBoundsVersion, setFitBoundsVersion] = useState(0);
  const [flyTo, setFlyTo] = useState<{ lat: number; lng: number; zoom?: number } | null>(null);
  const [flyKey, setFlyKey] = useState(0);
  const [mapDeviceSelect, setMapDeviceSelect] = useState("");
  const [mapToast, setMapToast] = useState<{ ok: boolean; text: string } | null>(null);

  const pops = useQuery({ queryKey: ["pops"], queryFn: () => apiFetch<{ pops: { id: string; description: string }[] }>("/api/v1/pops") });

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

  const detail = useQuery({
    queryKey: ["map-point-detail", selId],
    enabled: !!selId,
    queryFn: () => apiFetch<PointDetail>(`/api/v1/map/equipment-points/${selId}`),
  });

  const displayedPoints = useMemo(() => {
    const raw = pts.data?.points;
    return Array.isArray(raw) ? raw : [];
  }, [pts.data?.points]);

  const displayedIdSet = useMemo(() => new Set(displayedPoints.map((p) => p.id)), [displayedPoints]);

  useEffect(() => {
    if (!mapDeviceSelect) return;
    if (!displayedIdSet.has(mapDeviceSelect)) {
      setMapDeviceSelect("");
      setFlyTo(null);
    }
  }, [mapDeviceSelect, displayedIdSet]);

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
      })),
    [displayedPoints],
  );

  const sortedEquipOptions = useMemo(() => {
    const arr = [...displayedPoints];
    arr.sort((a, b) => String(a.description ?? "").localeCompare(String(b.description ?? ""), "pt"));
    return arr;
  }, [displayedPoints]);

  const popsOptions = useMemo(() => {
    const raw = pops.data?.pops;
    return Array.isArray(raw) ? raw : [];
  }, [pops.data?.pops]);


  useEffect(() => {
    setFitBoundsVersion((v) => v + 1);
  }, [popId, category]);

  useEffect(() => {
    if (!mapToast) return;
    const t = window.setTimeout(() => setMapToast(null), 10_000);
    return () => window.clearTimeout(t);
  }, [mapToast]);

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
              Equipamentos com coordenadas. POP e categoria pedem os pontos ao servidor; <strong>Vistas</strong> escolhe agrupamento fixo,
              desagrupado ou cores de estado. Em <strong>Agrupado</strong>, a densidade dos grupos segue o zoom (aproximar separa os pins); clique num
              grupo para expandir.
            </p>
            <p>Seleccione um equipamento na lista, no mapa ou no filtro «Equipamento».</p>
          </InfoHint>
        </h1>
        <PageCountPill label="Pontos visíveis" count={displayedPoints.length} />
      </div>

      <div className="card" style={{ marginBottom: 12 }}>
        <div className="row" style={{ flexWrap: "wrap", gap: 10, alignItems: "flex-end" }}>
          <label style={{ display: "flex", flexDirection: "column", alignItems: "stretch", gap: 6, flex: "0 1 220px", minWidth: 180 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Vistas</span>
            <select className="select" style={{ width: "100%", minWidth: 180 }} value={displayMode} onChange={(e) => setDisplayMode(e.target.value as MapDisplayMode)}>
              <option value="cluster">Agrupado (padrão)</option>
              <option value="scatter">Desagrupado</option>
              <option value="status">Online / Offline</option>
            </select>
          </label>
          <label className="row" style={{ gap: 8, alignItems: "center", flex: "1 1 200px", minWidth: 180 }}>
            <span style={{ fontSize: 12, color: "var(--muted)", whiteSpace: "nowrap" }}>POP</span>
            <select
              className="select"
              style={{ width: "100%", minWidth: 160 }}
              value={popId}
              onChange={(e) => setPopId(e.target.value)}
              disabled={pops.isPending}
              title={pops.isError ? "Não foi possível carregar a lista de POPs" : undefined}
            >
              <option value="">Todos os POPs</option>
              {popsOptions.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.description}
                </option>
              ))}
            </select>
            {pops.isError ? (
              <span className="msg msg--err" style={{ fontSize: 11, maxWidth: 280 }}>
                POPs: {(pops.error as Error).message}
              </span>
            ) : null}
          </label>
          <label className="row" style={{ gap: 8, alignItems: "center", flex: "1 1 180px", minWidth: 160 }}>
            <span style={{ fontSize: 12, color: "var(--muted)", whiteSpace: "nowrap" }}>Categoria</span>
            <select className="select" style={{ width: "100%", minWidth: 140 }} value={category} onChange={(e) => setCategory(e.target.value)}>
              <option value="">Todas</option>
              {MAP_DEVICE_CATEGORIES.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
          <label className="row" style={{ gap: 8, alignItems: "center", flex: "2 1 240px", minWidth: 200 }}>
            <span style={{ fontSize: 12, color: "var(--muted)", whiteSpace: "nowrap" }}>Equipamento</span>
            <select
              className="select"
              style={{ width: "100%", minWidth: 200 }}
              value={mapDeviceSelect}
              onChange={(e) => {
                const id = e.target.value;
                setMapDeviceSelect(id);
                if (!id) {
                  setFlyTo(null);
                  setSelId(null);
                  return;
                }
                const p = displayedPoints.find((x) => x.id === id);
                if (p) {
                  setSelId(id);
                  setFlyTo({ lat: Number(p.lat), lng: Number(p.lng), zoom: 17 });
                  setFlyKey((k) => k + 1);
                }
              }}
            >
              <option value="">— Localizar no mapa —</option>
              {sortedEquipOptions.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.description}
                </option>
              ))}
            </select>
          </label>
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

      <div style={{ display: "grid", gridTemplateColumns: "1fr minmax(280px, 360px)", gap: 16, alignItems: "start", marginTop: 12 }}>
        <div>
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
                  {displayedPoints.map((p) => (
                    <tr
                      key={p.id}
                      className={selId === p.id ? "row-interactive row-interactive--selected" : "row-interactive"}
                      onClick={() => {
                        setSelId(p.id);
                        setMapDeviceSelect(p.id);
                      }}
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
            </div>
          ) : (
            <MapSectionErrorBoundary>
              <div style={{ position: "relative", zIndex: 0, isolation: "isolate" }}>
                <EquipmentMap
                  points={mapPoints}
                  displayMode={displayMode}
                  onSelectDevice={(id) => {
                    setSelId(id);
                    setMapDeviceSelect(id || "");
                  }}
                  flyTo={flyTo}
                  flyKey={flyKey}
                  fitBoundsVersion={fitBoundsVersion}
                />
              </div>
            </MapSectionErrorBoundary>
          )}
        </div>

        <div className="card">
          <h2 style={{ marginTop: 0 }}>Detalhe</h2>
          {selId && detail.isLoading && <p>A carregar…</p>}
          {selId && detail.isError && <div className="msg msg--err">{(detail.error as Error).message}</div>}
          {selId && detail.data && (
            <div style={{ fontSize: 13 }}>
              <p style={{ marginTop: 0 }}>
                <strong>{detail.data.description}</strong>
              </p>
              <table style={{ width: "100%", fontSize: 12, borderCollapse: "collapse" }}>
                <tbody>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0", verticalAlign: "top" }}>Categoria</td>
                    <td className="mono" style={{ padding: "4px 0" }}>
                      {detail.data.category}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Marca / Modelo</td>
                    <td style={{ padding: "4px 0" }}>
                      {[detail.data.brand, detail.data.model].filter(Boolean).join(" · ") || "—"}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>IP</td>
                    <td className="mono" style={{ padding: "4px 0" }}>
                      {detail.data.ip ?? "—"}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Rede</td>
                    <td style={{ padding: "4px 0" }}>{detail.data.network_status ?? "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Modo operação</td>
                    <td style={{ padding: "4px 0" }}>{detail.data.operational_mode ?? "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Ping / Telemetria</td>
                    <td style={{ padding: "4px 0" }}>
                      {detail.data.ping_enabled ? "sim" : "não"} / {detail.data.telemetry_enabled ? "sim" : "não"}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>MAC</td>
                    <td className="mono" style={{ padding: "4px 0" }}>
                      {detail.data.mac || "—"}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>N.º série</td>
                    <td className="mono" style={{ padding: "4px 0" }}>
                      {detail.data.serial_number || "—"}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Firmware</td>
                    <td style={{ padding: "4px 0", fontSize: 11 }}>{detail.data.software_version || "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Hardware</td>
                    <td style={{ padding: "4px 0", fontSize: 11 }}>{detail.data.hardware_version || "—"}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Coordenadas</td>
                    <td className="mono" style={{ padding: "4px 0" }}>
                      {detail.data.lat}, {detail.data.lng}
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Estado (probe)</td>
                    <td style={{ padding: "4px 0" }}>
                      <span className={`badge ${detail.data.status === "online" ? "badge--ok" : detail.data.status === "offline" ? "badge--err" : "badge--off"}`}>
                        {detail.data.status}
                      </span>
                    </td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Última verificação</td>
                    <td style={{ padding: "4px 0", fontSize: 11 }}>{fmtIso(detail.data.last_check_at)}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Actualizado (equip.)</td>
                    <td style={{ padding: "4px 0", fontSize: 11 }}>{fmtIso(detail.data.updated_at)}</td>
                  </tr>
                  <tr>
                    <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>ID</td>
                    <td className="mono" style={{ padding: "4px 0", fontSize: 10 }}>
                      {String(detail.data.id)}
                    </td>
                  </tr>
                </tbody>
              </table>
              <div className="row" style={{ marginTop: 12, flexWrap: "wrap", gap: 8 }}>
                <Link to={`/devices?focus=${encodeURIComponent(String(detail.data.id))}`} className="btn btn--primary">
                  Editar equipamento
                </Link>
                <Link to="/devices" className="btn">
                  Lista de equipamentos
                </Link>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
