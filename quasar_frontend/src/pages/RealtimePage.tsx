import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ToolOutputError } from "../components/ToolsOutputViews";
import { apiFetch } from "../lib/api";
import { apiUrl, getStoredApiKey } from "../lib/auth";

type DeviceRow = {
  id: string;
  description: string;
  ip?: string | null;
  network_status?: string | null;
};

type RealtimeSample = {
  device_id: string;
  ok?: boolean;
  latency_ms?: number;
  method?: string;
  checked_at?: string | null;
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

  const idsCsv = useMemo(() => picked.join(","), [picked]);
  const [liveSamples, setLiveSamples] = useState<RealtimeSample[] | null>(null);
  const rt = useQuery({
    queryKey: ["realtime-ping", idsCsv],
    queryFn: () => apiFetch<{ samples: RealtimeSample[]; note?: string }>(`/api/v1/realtime/ping?device_ids=${encodeURIComponent(idsCsv)}`),
    enabled: picked.length > 0 && picked.length <= 3,
    refetchInterval: false,
  });

  useEffect(() => {
    setLiveSamples(rt.data?.samples ?? null);
  }, [rt.data?.samples]);

  useEffect(() => {
    if (picked.length === 0 || picked.length > 3) return;
    const base = apiUrl("/api/v1/realtime/ws");
    const token = getStoredApiKey();
    const wsUrl = base.replace(/^http/i, "ws") + (token ? `?api_key=${encodeURIComponent(token)}` : "");
    const ws = new WebSocket(wsUrl);
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(String(ev.data ?? "{}")) as { type?: string; data?: { samples?: RealtimeSample[] } };
        if (msg.type === "realtime.ping.samples" && Array.isArray(msg.data?.samples)) {
          const allow = new Set(picked);
          setLiveSamples(msg.data.samples.filter((s) => allow.has(String(s.device_id))));
        }
      } catch {
        // noop
      }
    };
    return () => ws.close();
  }, [picked]);

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
        Monitorização rápida do estado de ping em cache (até 3 equipamentos). Apenas equipamentos com estado de rede{" "}
        <strong>Normal</strong> podem ser selecionados. Atualização automática a cada 4 segundos enquanto houver seleção.
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
              <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
                Selecionados: <strong>{picked.length}</strong> de 3 máximo.
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
                    {normalDevices.map((d) => (
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

      {picked.length > 0 && picked.length <= 3 && (
        <div className="card" style={{ marginTop: 12 }}>
          <h2>Última leitura</h2>
          {rt.isLoading && <p>A obter dados…</p>}
          <ToolOutputError err={rt.error as Error | null} />
          {rt.data?.note ? <p style={{ color: "var(--muted)", fontSize: 12 }}>{rt.data.note}</p> : null}
          {(liveSamples ?? rt.data?.samples) && !rt.isError ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 12, marginTop: 8 }}>
              {(liveSamples ?? rt.data?.samples ?? []).map((s) => {
                const dev = deviceById.get(s.device_id);
                const ok = s.ok === true;
                const lat = typeof s.latency_ms === "number" ? s.latency_ms : null;
                return (
                  <div
                    key={s.device_id}
                    style={{
                      border: "1px solid var(--border)",
                      borderRadius: "var(--radius)",
                      padding: "12px 14px",
                      background: "var(--panel2)",
                    }}
                  >
                    <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
                      <div>
                        <div style={{ fontWeight: 600, fontSize: 15 }}>{dev?.description ?? "Equipamento"}</div>
                        <div className="mono" style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }}>
                          {s.device_id}
                        </div>
                      </div>
                      <div className="row" style={{ gap: 10, alignItems: "center" }}>
                        <span className={ok ? "badge" : "badge badge--off"}>{ok ? "Ping OK" : "Ping falhou"}</span>
                        {lat != null ? (
                          <span className="mono" style={{ fontSize: 15 }}>
                            {lat} ms
                          </span>
                        ) : (
                          <span style={{ color: "var(--muted)", fontSize: 13 }}>—</span>
                        )}
                      </div>
                    </div>
                    <div style={{ marginTop: 10, fontSize: 12, color: "var(--muted)", display: "flex", flexWrap: "wrap", gap: 12 }}>
                      <span>
                        Método: <span className="mono">{s.method ?? "—"}</span>
                      </span>
                      <span>
                        Registo:{" "}
                        <span className="mono" style={{ fontSize: 11 }}>
                          {s.checked_at ?? "—"}
                        </span>
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          ) : null}
        </div>
      )}
    </>
  );
}
