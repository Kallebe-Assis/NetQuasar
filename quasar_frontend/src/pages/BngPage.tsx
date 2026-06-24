import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, RefreshCw, Search } from "lucide-react";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { APP_ROUTES } from "../app/routes";

type BngDevice = {
  id: string;
  description?: string;
  ip?: string;
  brand?: string;
  model?: string;
  category?: string;
};

type BngOverview = {
  device?: BngDevice;
  telemetry_collected_at?: string;
  fields?: Record<string, string | number>;
  latest_stats?: {
    collected_at?: string;
    total_online?: number | null;
    pppoe_online?: number | null;
    ipv4_online?: number | null;
    ipv6_online?: number | null;
    dual_stack_online?: number | null;
  };
};

type StatsSample = {
  collected_at: string;
  total_online?: number | null;
  pppoe_online?: number | null;
  ipv4_online?: number | null;
  ipv6_online?: number | null;
  dual_stack_online?: number | null;
};

type PppoeSession = {
  index?: string;
  login?: string;
  ipv4?: string;
  mac?: string;
  ipv6?: string;
  status?: string;
  auth_state?: string;
  author_state?: string;
  acct_state?: string;
  port_type?: string;
};

const FIELD_LABELS: Record<string, string> = {
  sys_descr: "Descrição",
  sys_name: "Nome",
  sys_uptime: "Uptime",
  hw_model: "Modelo",
  hw_software: "Versão software",
  cpu_usage: "CPU",
  memory_usage: "Memória",
  temperature: "Temperatura",
};

type BngTab = "overview" | "sessions";

function MiniTotalsChart({ samples }: { samples: StatsSample[] }) {
  if (samples.length < 2) {
    return (
      <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>
        Aguardando histórico de coletas periódicas (monitoramento SNMP).
      </p>
    );
  }
  const w = 560;
  const h = 180;
  const pad = 20;
  const series = [
    { key: "pppoe_online" as const, color: "#3b82f6", label: "PPPoE" },
    { key: "ipv4_online" as const, color: "#22c55e", label: "IPv4" },
    { key: "ipv6_online" as const, color: "#a855f7", label: "IPv6" },
    { key: "dual_stack_online" as const, color: "#f59e0b", label: "Dual-stack" },
  ];
  const maxV = Math.max(
    1,
    ...samples.flatMap((s) => series.map((ser) => Number(s[ser.key] ?? 0))),
  );
  const xFor = (i: number) => pad + (i * (w - pad * 2)) / Math.max(1, samples.length - 1);
  const yFor = (v: number) => h - pad - (Math.max(0, v) / maxV) * (h - pad * 2);
  return (
    <div>
      <div className="row" style={{ gap: 12, marginBottom: 8, fontSize: 11, flexWrap: "wrap" }}>
        {series.map((s) => (
          <span key={s.key} style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <span style={{ width: 10, height: 3, background: s.color, display: "inline-block" }} />
            {s.label}
          </span>
        ))}
      </div>
      <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label="Gráfico de totais BNG">
        <line x1={pad} y1={h - pad} x2={w - pad} y2={h - pad} stroke="var(--border)" strokeWidth={1} />
        {series.map((ser) => {
          const path = samples
            .map((p, i) => {
              const v = Number(p[ser.key] ?? 0);
              return `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(v)}`;
            })
            .join(" ");
          return <path key={ser.key} d={path} fill="none" stroke={ser.color} strokeWidth={2} />;
        })}
      </svg>
    </div>
  );
}

export function BngPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [tab, setTab] = useState<BngTab>("overview");
  const [sel, setSel] = useState<string | null>(null);
  const [loginFilter, setLoginFilter] = useState("");
  const [ipv4Filter, setIpv4Filter] = useState("");
  const [macFilter, setMacFilter] = useState("");
  const [authFilter, setAuthFilter] = useState("");
  const [authorFilter, setAuthorFilter] = useState("");

  const devices = useQuery({
    queryKey: ["bng-devices"],
    queryFn: () => apiFetch<{ devices: BngDevice[] }>("/api/v1/bng/devices"),
  });

  const rows = devices.data?.devices ?? [];
  const selectedId = sel ?? rows[0]?.id ?? null;

  const overview = useQuery({
    queryKey: ["bng-overview", selectedId],
    enabled: !!selectedId,
    queryFn: () => apiFetch<BngOverview>(`/api/v1/bng/devices/${selectedId}/overview`),
    refetchInterval: 60_000,
  });

  const history = useQuery({
    queryKey: ["bng-stats-history", selectedId],
    enabled: !!selectedId,
    queryFn: () =>
      apiFetch<{ samples: StatsSample[] }>(`/api/v1/bng/stats/history?device_id=${selectedId}&limit=96`),
    refetchInterval: 60_000,
  });

  const sessions = useQuery({
    queryKey: ["bng-device-sessions", selectedId],
    enabled: !!selectedId && tab === "sessions",
    queryFn: () =>
      apiFetch<{
        sessions: PppoeSession[];
        captured_at?: string;
        note?: string;
        count?: number;
      }>(`/api/v1/bng/devices/${selectedId}/sessions`),
  });

  const collectPeriodic = useMutation({
    mutationFn: () => apiFetch(`/api/v1/bng/devices/${selectedId}/collect`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["bng-overview", selectedId] });
      qc.invalidateQueries({ queryKey: ["bng-stats-history", selectedId] });
      toastOk(pushToast, "Coleta BNG concluída.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha na coleta."),
  });

  const collectSessions = useMutation({
    mutationFn: () => apiFetch(`/api/v1/bng/devices/${selectedId}/sessions/collect`, { method: "POST" }),
    onSuccess: (data: { count?: number }) => {
      qc.invalidateQueries({ queryKey: ["bng-device-sessions", selectedId] });
      toastOk(pushToast, `Consulta completa: ${data.count ?? 0} sessão(ões) PPPoE.`);
    },
    onError: (err) => toastErr(pushToast, err, "Falha na consulta SNMP."),
  });

  const filteredSessions = useMemo(() => {
    const list = sessions.data?.sessions ?? [];
    const login = loginFilter.trim().toLowerCase();
    const ipv4 = ipv4Filter.trim().toLowerCase();
    const mac = macFilter.trim().toLowerCase();
    const auth = authFilter.trim().toLowerCase();
    const author = authorFilter.trim().toLowerCase();
    return list.filter((s) => {
      if (login && !String(s.login ?? "").toLowerCase().includes(login)) return false;
      if (ipv4 && !String(s.ipv4 ?? "").toLowerCase().includes(ipv4)) return false;
      if (mac && !String(s.mac ?? "").toLowerCase().includes(mac)) return false;
      if (auth && !String(s.auth_state ?? "").toLowerCase().includes(auth)) return false;
      if (author && !String(s.author_state ?? "").toLowerCase().includes(author)) return false;
      return true;
    });
  }, [sessions.data?.sessions, loginFilter, ipv4Filter, macFilter, authFilter, authorFilter]);

  const stats = overview.data?.latest_stats;
  const fields = overview.data?.fields ?? {};

  if (devices.isLoading) return <p>A carregar equipamentos BNG…</p>;
  if (devices.isError) return <div className="msg msg--err">{(devices.error as Error).message}</div>;

  return (
    <>
      <div className="page-heading">
        <h1>BNG</h1>
        {tab === "sessions" && <PageCountPill label="Sessões" count={filteredSessions.length} />}
      </div>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Concentradores com switch BNG activo. Totais periódicos via monitoramento SNMP; consulta completa de sessões PPPoE
        sob demanda.{" "}
        <Link to={APP_ROUTES.settings}>Configurações → BNG</Link> para OIDs e métricas recomendadas.
      </p>

      {rows.length === 0 ? (
        <div className="msg msg--warn">
          Nenhum equipamento com BNG activo. Active o switch em{" "}
          <Link to={APP_ROUTES.devices}>Equipamentos</Link>.
        </div>
      ) : (
        <>
          <div className="row" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap", alignItems: "center" }}>
            <label style={{ fontSize: 13 }}>Equipamento</label>
            <select
              className="input"
              style={{ minWidth: 280 }}
              value={selectedId ?? ""}
              onChange={(e) => setSel(e.target.value || null)}
            >
              {rows.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.description || d.ip} {d.ip ? `(${d.ip})` : ""}
                </option>
              ))}
            </select>
            {canMutate && (
              <button
                type="button"
                className="btn"
                disabled={!selectedId || collectPeriodic.isPending}
                onClick={() => collectPeriodic.mutate()}
              >
                <RefreshCw size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                Coletar agora
              </button>
            )}
          </div>

          <div className="tabs" style={{ marginBottom: 16 }}>
            <button type="button" className={tab === "overview" ? "active" : ""} onClick={() => setTab("overview")}>
              Visão geral
            </button>
            <button type="button" className={tab === "sessions" ? "active" : ""} onClick={() => setTab("sessions")}>
              Sessões PPPoE
            </button>
          </div>

          {tab === "overview" && (
            <>
              <div className="row" style={{ gap: 12, marginBottom: 16, flexWrap: "wrap" }}>
                {[
                  { label: "PPPoE online", value: stats?.pppoe_online },
                  { label: "IPv4 online", value: stats?.ipv4_online },
                  { label: "IPv6 online", value: stats?.ipv6_online },
                  { label: "Dual-stack", value: stats?.dual_stack_online },
                  { label: "Total online", value: stats?.total_online },
                ].map((c) => (
                  <div key={c.label} className="card" style={{ padding: "10px 14px", minWidth: 120 }}>
                    <div style={{ fontSize: 11, color: "var(--muted)" }}>{c.label}</div>
                    <strong style={{ fontSize: 20 }}>{c.value ?? "—"}</strong>
                  </div>
                ))}
              </div>

              <div className="card" style={{ padding: 14, marginBottom: 16 }}>
                <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Histórico de totais</h3>
                <MiniTotalsChart samples={history.data?.samples ?? []} />
                {stats?.collected_at && (
                  <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
                    Última amostra: {new Date(stats.collected_at).toLocaleString("pt-BR")}
                  </p>
                )}
              </div>

              <div className="card" style={{ padding: 14 }}>
                <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Sistema e telemetria</h3>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))",
                    gap: 10,
                  }}
                >
                  {Object.entries(FIELD_LABELS).map(([key, label]) => {
                    const val = fields[key];
                    if (val == null || val === "") return null;
                    return (
                      <div key={key} style={{ fontSize: 13 }}>
                        <div style={{ fontSize: 11, color: "var(--muted)" }}>{label}</div>
                        <div className="mono" style={{ wordBreak: "break-word" }}>
                          {String(val)}
                        </div>
                      </div>
                    );
                  })}
                  {overview.data?.device && (
                    <>
                      <div style={{ fontSize: 13 }}>
                        <div style={{ fontSize: 11, color: "var(--muted)" }}>Marca / modelo (cadastro)</div>
                        <div>
                          {[overview.data.device.brand, overview.data.device.model].filter(Boolean).join(" ") || "—"}
                        </div>
                      </div>
                      <div style={{ fontSize: 13 }}>
                        <div style={{ fontSize: 11, color: "var(--muted)" }}>IP de gestão</div>
                        <div className="mono">{overview.data.device.ip || "—"}</div>
                      </div>
                    </>
                  )}
                </div>
                {overview.data?.telemetry_collected_at && (
                  <p style={{ fontSize: 11, color: "var(--muted)", margin: "10px 0 0" }}>
                    Telemetria: {new Date(overview.data.telemetry_collected_at).toLocaleString("pt-BR")}
                  </p>
                )}
              </div>
            </>
          )}

          {tab === "sessions" && (
            <>
              <div className="msg msg--warn" style={{ marginBottom: 12, display: "flex", gap: 8, alignItems: "flex-start" }}>
                <AlertTriangle size={18} style={{ flexShrink: 0, marginTop: 2 }} />
                <div style={{ fontSize: 13, lineHeight: 1.5 }}>
                  <strong>Consulta completa via SNMP walk</strong> — percorre milhares de OIDs (login, IP, MAC, IPv6,
                  estados). Em concentradores com 2 000+ sessões pode demorar <strong>vários minutos</strong> e aumentar
                  carga no BNG. A senha PPPoE <em>não</em> está disponível via SNMP.
                </div>
              </div>

              <div className="row" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
                {canMutate && (
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={!selectedId || collectSessions.isPending}
                    onClick={() => collectSessions.mutate()}
                  >
                    {collectSessions.isPending ? "A consultar SNMP…" : "Consulta completa SNMP"}
                  </button>
                )}
                {sessions.data?.captured_at && (
                  <span style={{ fontSize: 12, color: "var(--muted)", alignSelf: "center" }}>
                    Última consulta: {new Date(sessions.data.captured_at).toLocaleString("pt-BR")} (
                    {sessions.data.count ?? 0} sessões)
                  </span>
                )}
              </div>
              {sessions.data?.note && (
                <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>{sessions.data.note}</p>
              )}

              <div className="card" style={{ padding: 12, marginBottom: 12 }}>
                <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
                  <div className="field" style={{ minWidth: 140 }}>
                    <label style={{ fontSize: 11 }}>Login</label>
                    <input className="input" value={loginFilter} onChange={(e) => setLoginFilter(e.target.value)} placeholder="parcial" />
                  </div>
                  <div className="field" style={{ minWidth: 140 }}>
                    <label style={{ fontSize: 11 }}>IPv4</label>
                    <input className="input mono" value={ipv4Filter} onChange={(e) => setIpv4Filter(e.target.value)} placeholder="100.64 ou 45.235" />
                  </div>
                  <div className="field" style={{ minWidth: 140 }}>
                    <label style={{ fontSize: 11 }}>MAC</label>
                    <input className="input mono" value={macFilter} onChange={(e) => setMacFilter(e.target.value)} placeholder="parcial" />
                  </div>
                  <div className="field" style={{ minWidth: 140 }}>
                    <label style={{ fontSize: 11 }}>Autenticação</label>
                    <input className="input" value={authFilter} onChange={(e) => setAuthFilter(e.target.value)} placeholder="Autenticado…" />
                  </div>
                  <div className="field" style={{ minWidth: 140 }}>
                    <label style={{ fontSize: 11 }}>Autorização</label>
                    <input className="input" value={authorFilter} onChange={(e) => setAuthorFilter(e.target.value)} placeholder="Autorizado…" />
                  </div>
                  <button type="button" className="btn" onClick={() => sessions.refetch()}>
                    <Search size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                    Actualizar lista
                  </button>
                </div>
              </div>

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Status</th>
                      <th>Login</th>
                      <th>IPv4</th>
                      <th>IPv6</th>
                      <th>MAC</th>
                      <th>Autenticação</th>
                      <th>Autorização</th>
                      <th>Accounting</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredSessions.length === 0 ? (
                      <tr>
                        <td colSpan={8} style={{ color: "var(--muted)" }}>
                          {sessions.isLoading
                            ? "A carregar…"
                            : "Nenhuma sessão. Execute a consulta completa SNMP ou aguarde uma consulta anterior."}
                        </td>
                      </tr>
                    ) : (
                      filteredSessions.map((s) => (
                        <tr key={s.index ?? `${s.login}-${s.mac}`}>
                          <td>{s.status || "Up"}</td>
                          <td className="mono">{s.login || "—"}</td>
                          <td className="mono">{s.ipv4 || "—"}</td>
                          <td className="mono" style={{ maxWidth: 180, overflow: "hidden", textOverflow: "ellipsis" }}>
                            {s.ipv6 || "—"}
                          </td>
                          <td className="mono">{s.mac || "—"}</td>
                          <td>{s.auth_state || "—"}</td>
                          <td>{s.author_state || "—"}</td>
                          <td>{s.acct_state || "—"}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </>
      )}
    </>
  );
}
