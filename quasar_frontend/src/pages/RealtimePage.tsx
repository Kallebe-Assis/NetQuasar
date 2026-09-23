import { useMemo, useState, useEffect, useRef } from "react";
import { Filter, Search } from "lucide-react";
import { LatencyLiveChart } from "../components/LatencyLiveChart";
import { useContinuousIcmpPing } from "../hooks/useContinuousIcmpPing";
import { apiFetch } from "../lib/api";
import { useQuery } from "@tanstack/react-query";

type DeviceRow = {
  id: string;
  description: string;
  ip?: string | null;
  network_status?: string | null;
  category?: string | null;
};

function isNormalNetworkStatus(ns: string | null | undefined): boolean {
  return String(ns ?? "").trim().toLowerCase() === "normal";
}

export function RealtimePage() {
  const devices = useQuery({
    queryKey: ["devices-rt"],
    queryFn: () => apiFetch<{ devices: DeviceRow[] }>("/api/v1/devices"),
  });

  const normalDevices = useMemo(
    () => (devices.data?.devices ?? []).filter((d) => isNormalNetworkStatus(d.network_status)),
    [devices.data?.devices],
  );

  const [search, setSearch] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  const [categoryFilter, setCategoryFilter] = useState<string>("");
  const filterRef = useRef<HTMLDivElement>(null);

  const categories = useMemo(() => {
    const set = new Set<string>();
    for (const d of normalDevices) {
      const c = String(d.category ?? "").trim();
      if (c) set.add(c);
    }
    return Array.from(set).sort();
  }, [normalDevices]);

  useEffect(() => {
    if (!filterOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (filterRef.current && !filterRef.current.contains(e.target as Node)) setFilterOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [filterOpen]);

  const visibleDevices = useMemo(() => {
    const q = search.trim().toLowerCase();
    return normalDevices.filter((d) => {
      if (categoryFilter && String(d.category ?? "").trim() !== categoryFilter) return false;
      if (!q) return true;
      return d.description.toLowerCase().includes(q) || (d.ip ?? "").toLowerCase().includes(q);
    });
  }, [normalDevices, search, categoryFilter]);

  const deviceById = useMemo(() => {
    const m = new Map<string, DeviceRow>();
    for (const d of normalDevices) m.set(d.id, d);
    return m;
  }, [normalDevices]);

  const [picked, setPicked] = useState<string[]>([]);

  useEffect(() => {
    const allowed = new Set(normalDevices.map((d) => d.id));
    setPicked((prev) => prev.filter((id) => allowed.has(id)));
  }, [normalDevices]);

  const targets = useMemo(
    () =>
      picked
        .map((id) => {
          const d = deviceById.get(id);
          const host = String(d?.ip ?? "").trim();
          if (!d || !host) return null;
          return { id, host, label: d.description };
        })
        .filter((t): t is { id: string; host: string; label: string } => t != null),
    [picked, deviceById],
  );

  const ping = useContinuousIcmpPing({
    targets,
    enabled: targets.length > 0 && targets.length <= 3,
    minIntervalMs: 1000,
    timeoutMs: 3000,
    maxPoints: 120,
  });

  function toggle(id: string) {
    setPicked((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id);
      if (prev.length >= 3) return [...prev.slice(1), id];
      return [...prev, id];
    });
  }

  return (
    <>
      <h1>Tempo real</h1>
      <p style={{ color: "var(--muted)", fontSize: 13, marginTop: 0, maxWidth: 720 }}>
        Ping ICMP contínuo (intervalo mínimo de 1 s entre ciclos) para até 3 equipamentos com estado de rede{" "}
        <strong>Normal</strong>. O gráfico de latência é construído à medida que as amostras chegam.
      </p>

      {devices.isLoading && <p>A carregar equipamentos…</p>}
      {devices.isError && <div className="msg msg--err">{(devices.error as Error).message}</div>}
      {devices.data && (
        <div className="card">
          <h2>Equipamentos (rede Normal)</h2>
          {normalDevices.length === 0 ? (
            <p style={{ color: "var(--muted)", fontSize: 13 }}>
              Nenhum equipamento com estado de rede Normal. Ajuste os equipamentos em{" "}
              <strong>Equipamentos</strong> para utilizar esta vista.
            </p>
          ) : (
            <>
              <div className="row" style={{ gap: 8, marginBottom: 10, flexWrap: "wrap", alignItems: "center" }}>
                <div style={{ position: "relative", flex: 1, minWidth: 200, maxWidth: 360 }}>
                  <Search size={14} style={{ position: "absolute", left: 10, top: "50%", transform: "translateY(-50%)", color: "var(--muted)" }} aria-hidden />
                  <input
                    className="input"
                    style={{ paddingLeft: 30, width: "100%" }}
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Pesquisar por descrição ou IP…"
                    aria-label="Pesquisar equipamentos"
                  />
                </div>
                <div ref={filterRef} style={{ position: "relative" }}>
                  <button
                    type="button"
                    className={`btn${categoryFilter ? " btn--primary" : ""}`}
                    onClick={() => setFilterOpen((v) => !v)}
                    aria-expanded={filterOpen}
                  >
                    <Filter size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                    Filtro{categoryFilter ? `: ${categoryFilter}` : ""}
                  </button>
                  {filterOpen ? (
                    <div
                      className="card"
                      style={{ position: "absolute", top: "100%", right: 0, marginTop: 4, minWidth: 200, zIndex: 30, padding: 10 }}
                    >
                      <p style={{ margin: "0 0 6px", fontSize: 11, fontWeight: 600, color: "var(--muted)" }}>Categoria</p>
                      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                        <label className="row" style={{ gap: 6, fontSize: 12, cursor: "pointer" }}>
                          <input type="radio" checked={categoryFilter === ""} onChange={() => setCategoryFilter("")} />
                          Todas
                        </label>
                        {categories.map((c) => (
                          <label key={c} className="row" style={{ gap: 6, fontSize: 12, cursor: "pointer" }}>
                            <input type="radio" checked={categoryFilter === c} onChange={() => setCategoryFilter(c)} />
                            {c}
                          </label>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
              <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
                Selecionados: <strong>{picked.length}</strong> de 3 máximo
                {ping.running ? " · ping contínuo activo" : ""}.
              </p>
              <div className="table-wrap" style={{ maxHeight: 320, overflow: "auto" }}>
                <table>
                  <thead>
                    <tr>
                      <th style={{ width: 40 }} />
                      <th>Descrição</th>
                      <th>IP</th>
                      <th>Estado rede</th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleDevices.length === 0 ? (
                      <tr>
                        <td colSpan={4} style={{ color: "var(--muted)", textAlign: "center", padding: 12 }}>
                          Nenhum equipamento corresponde à pesquisa/filtro.
                        </td>
                      </tr>
                    ) : null}
                    {visibleDevices.map((d) => (
                      <tr key={d.id}>
                        <td>
                          <input type="checkbox" checked={picked.includes(d.id)} onChange={() => toggle(d.id)} />
                        </td>
                        <td>{d.description}</td>
                        <td className="mono">{d.ip ?? "—"}</td>
                        <td>
                          <span className="badge">{d.network_status ?? "Normal"}</span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}

      {normalDevices.length > 0 && picked.length === 0 ? (
        <p style={{ color: "var(--muted)", marginTop: 12 }}>Escolha pelo menos um equipamento (máximo 3).</p>
      ) : null}

      {picked.length > 0 && targets.length === 0 ? (
        <p style={{ color: "var(--muted)", marginTop: 12 }}>Os equipamentos seleccionados não têm IP configurado.</p>
      ) : null}

      {targets.length > 0 && targets.length <= 3 && (
        <div className="card" style={{ marginTop: 12 }}>
          <h2>Latência em tempo real</h2>
          {ping.lastError ? (
            <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
              Último aviso: {ping.lastError}
            </p>
          ) : null}
          <div style={{ display: "flex", flexDirection: "column", gap: 16, marginTop: 8 }}>
            {targets.map((t) => {
              const latest = ping.latest[t.id];
              const ok = latest?.ok === true;
              const lat = latest?.ms ?? null;
              const points = ping.series[t.id] ?? [];
              return (
                <div
                  key={t.id}
                  style={{
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius)",
                    padding: "12px 14px",
                    background: "var(--panel2)",
                  }}
                >
                  <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
                    <div>
                      <div style={{ fontWeight: 600, fontSize: 15 }}>{t.label}</div>
                      <div className="mono" style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }}>
                        {t.host}
                      </div>
                    </div>
                    <div className="row" style={{ gap: 10, alignItems: "center" }}>
                      {latest ? (
                        <>
                          <span className={ok ? "badge" : "badge badge--off"}>{ok ? "Ping OK" : "Ping falhou"}</span>
                          {lat != null ? (
                            <span className="mono" style={{ fontSize: 15 }}>
                              {lat} ms
                            </span>
                          ) : (
                            <span style={{ color: "var(--muted)", fontSize: 13 }}>—</span>
                          )}
                        </>
                      ) : (
                        <span style={{ color: "var(--muted)", fontSize: 12 }}>A iniciar…</span>
                      )}
                    </div>
                  </div>
                  <div style={{ marginTop: 12 }}>
                    <LatencyLiveChart points={points} ariaLabel={`Latência de ${t.label}`} />
                  </div>
                  <div style={{ marginTop: 8, fontSize: 11, color: "var(--muted)" }}>
                    {points.length} amostra(s) · intervalo ≥ 1 s
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </>
  );
}
