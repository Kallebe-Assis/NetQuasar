import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  HttpProbeCellSummary,
  isHttpMatrixRowAnyProbeAccessible,
  IcmpSingleOutput,
  SnmpGetOutput,
  SnmpWalkDiscoveriesOutput,
  SnmpWalkRowsOutput,
  SshDialOutput,
  TelnetTestOutput,
  ToolOutputError,
  WalkJobQueuedOutput,
  NetworkToolTextOutput,
} from "../components/ToolsOutputViews";
import { InfoHint } from "../components/InfoHint";
import { apiFetch } from "../lib/api";
import { ToolsPageToastHost, useToolsPageToast } from "./toolsPageToast";

function isRecord(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

function ToolsPanel({ title, description, children, results }: { title: string; description?: ReactNode; children: ReactNode; results?: ReactNode }) {
  return (
    <div className="tools-panel">
      <div className="tools-panel__head">
        <h2>{title}</h2>
        {description ? <p className="tools-panel__desc">{description}</p> : null}
      </div>
      <div className="tools-panel__body">
        {children}
        {results ? <div className="tools-panel__results">{results}</div> : null}
      </div>
    </div>
  );
}

function splitLinesOrComma(s: string): string[] {
  return s
    .split(/[\n,;]+/)
    .map((x) => x.trim())
    .filter(Boolean);
}

type Tab =
  | "host_ping"
  | "http_matrix"
  | "icmp"
  | "snmp"
  | "snmp_bulk"
  | "telnet"
  | "ssh"
  | "snmp_walk"
  | "mikrotik"
  | "tracert"
  | "nmap";

export function ToolsPage() {
  const [tab, setTab] = useState<Tab>("host_ping");
  const { toast, show, dismiss } = useToolsPageToast();

  const [hostPingText, setHostPingText] = useState("example.com\ngoogle.com\ncloudflare.com");
  const [hostPingTimeout, setHostPingTimeout] = useState("4000");
  const hostPingRun = useMutation({
    mutationFn: async () => {
      const hosts = splitLinesOrComma(hostPingText).slice(0, 100);
      const timeout_ms = Math.min(15000, Math.max(500, Number(hostPingTimeout) || 4000));
      type Row = { host: string; ok: boolean; rtt_ms?: number; error?: string; note?: string };
      const rows: Row[] = [];
      for (const host of hosts) {
        try {
          const r = await apiFetch<{ ok?: boolean; rtt_ms?: number; error?: string; note?: string }>("/api/v1/tools/icmp/ping", {
            method: "POST",
            json: { host, timeout_ms },
          });
          rows.push({
            host,
            ok: !!r.ok,
            rtt_ms: typeof r.rtt_ms === "number" ? r.rtt_ms : undefined,
            error: r.error,
            note: r.note,
          });
        } catch (e) {
          rows.push({ host, ok: false, error: e instanceof Error ? e.message : String(e) });
        }
      }
      return { rows, note: "Até 100 nomes por execução; ICMP echo via servidor (resolução DNS + ping)." };
    },
    onMutate: () => {
      show("info", "A executar ping ICMP em lote no servidor…");
    },
    onSuccess: (data) => {
      const n = data.rows.length;
      const ok = data.rows.filter((r) => r.ok).length;
      show("ok", `Ping concluído: ${ok} de ${n} alvo(s) acessível(is), ${n - ok} inacessível(is) ou com erro.`);
    },
    onError: (e) => {
      show("err", e instanceof Error ? e.message : String(e));
    },
  });

  const [mxIps, setMxIps] = useState("1.1.1.1\n8.8.8.8");
  const [mxPorts, setMxPorts] = useState("2265\n80\n8080\n8888\n443\n8443");
  const [mxTo, setMxTo] = useState("300");
  const [mxInsecure, setMxInsecure] = useState(true);
  type HttpMatrixFilter = "all" | "ok" | "blocked";
  const [httpMatrixFilter, setHttpMatrixFilter] = useState<HttpMatrixFilter>("all");
  const httpMatrixRun = useMutation({
    mutationFn: async () => {
      const ips = splitLinesOrComma(mxIps);
      const ports = splitLinesOrComma(mxPorts)
        .map((p) => Number(p))
        .filter((n) => !Number.isNaN(n) && n > 0 && n <= 65535);
      const timeout_ms = Number(mxTo) || 6000;
      type Row = { ip: string; port: number; https: unknown; http: unknown };
      const rows: Row[] = [];
      let n = 0;
      outer: for (const ip of ips) {
        for (const port of ports) {
          if (++n > 64) break outer;
          const urlHttps = `https://${ip}:${port}`;
          const urlHttp = `http://${ip}:${port}`;
          let https: unknown = null;
          let http: unknown = null;
          try {
            https = await apiFetch("/api/v1/tools/http-https-probe", {
              method: "POST",
              json: { url: urlHttps, method: "GET", timeout_ms, follow_redirects: true, insecure_tls: mxInsecure },
            });
          } catch (e) {
            https = { error: e instanceof Error ? e.message : String(e), url: urlHttps };
          }
          try {
            http = await apiFetch("/api/v1/tools/http-https-probe", {
              method: "POST",
              json: { url: urlHttp, method: "GET", timeout_ms, follow_redirects: true, insecure_tls: mxInsecure },
            });
          } catch (e) {
            http = { error: e instanceof Error ? e.message : String(e), url: urlHttp };
          }
          rows.push({ ip, port, https, http });
        }
      }
      return { rows, note: "Máx. 64 combinações IP×porta; cada célula envia um teste HTTP/HTTPS ao servidor." };
    },
    onMutate: () => {
      setHttpMatrixFilter("all");
      show("info", "A executar sondagens HTTP e HTTPS em todas as combinações IP×porta…");
    },
    onSuccess: (data) => {
      const okRows = data.rows.filter((r) => isHttpMatrixRowAnyProbeAccessible(r)).length;
      show("ok", `Matriz concluída: ${data.rows.length} combinação(ões); ${okRows} com HTTP ou HTTPS acessível.`);
    },
    onError: (e) => {
      show("err", e instanceof Error ? e.message : String(e));
    },
  });

  const httpMatrixRowsFiltered = useMemo(() => {
    const rows = httpMatrixRun.data?.rows;
    if (!rows?.length) return [];
    if (httpMatrixFilter === "all") return rows;
    if (httpMatrixFilter === "ok") return rows.filter((r) => isHttpMatrixRowAnyProbeAccessible(r));
    return rows.filter((r) => !isHttpMatrixRowAnyProbeAccessible(r));
  }, [httpMatrixRun.data?.rows, httpMatrixFilter]);

  const [icmpHost, setIcmpHost] = useState("127.0.0.1");
  const [icmpTo, setIcmpTo] = useState("3000");
  const [tracertHost, setTracertHost] = useState("8.8.8.8");
  const [tracertHops, setTracertHops] = useState("30");
  const [tracertTo, setTracertTo] = useState("60000");
  const tracertRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/tracert", {
        method: "POST",
        json: {
          host: tracertHost.trim(),
          max_hops: Number(tracertHops) || 30,
          timeout_ms: Number(tracertTo) || 60000,
        },
      }),
    onMutate: () => show("info", `A executar tracert para ${tracertHost.trim() || "…"}…`),
    onSuccess: (d) => {
      const rec = d as Record<string, unknown>;
      if (rec.ok === true) show("ok", "Tracert concluído.");
      else show("err", typeof rec.error === "string" && rec.error ? rec.error : "Tracert terminou com avisos — veja a saída.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const [nmapHost, setNmapHost] = useState("127.0.0.1");
  const [nmapMode, setNmapMode] = useState("sn");
  const [nmapTo, setNmapTo] = useState("60000");
  const nmapRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/nmap", {
        method: "POST",
        json: {
          host: nmapHost.trim(),
          scan_mode: nmapMode,
          timeout_ms: Number(nmapTo) || 60000,
        },
      }),
    onMutate: () => show("info", `A executar nmap em ${nmapHost.trim() || "…"}…`),
    onSuccess: (d) => {
      const rec = d as Record<string, unknown>;
      if (rec.ok === true) show("ok", "Nmap concluído.");
      else show("err", typeof rec.error === "string" && rec.error ? rec.error : "Nmap terminou com avisos — veja a saída.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const icmpRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/icmp/ping", {
        method: "POST",
        json: { host: icmpHost.trim(), timeout_ms: Number(icmpTo) || 4000 },
      }),
    onMutate: () => show("info", `A enviar ICMP para ${icmpHost.trim() || "…"}…`),
    onSuccess: (d) => {
      const rec = d as Record<string, unknown>;
      if (rec.ok === true) show("ok", "Ping ICMP: destino respondeu.");
      else show("err", typeof rec.error === "string" && rec.error ? rec.error : "Ping ICMP sem resposta ou com erro.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const [snmpHost, setSnmpHost] = useState("127.0.0.1");
  const [snmpPort, setSnmpPort] = useState("161");
  const [snmpComm, setSnmpComm] = useState("public");
  const [snmpOids, setSnmpOids] = useState("1.3.6.1.2.1.1.1.0");
  const [snmpVer, setSnmpVer] = useState("2c");
  const [snmpTo, setSnmpTo] = useState("5000");
  const [snmpRetries, setSnmpRetries] = useState("0");
  const snmpRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/snmp/get", {
        method: "POST",
        json: {
          host: snmpHost.trim(),
          port: Number(snmpPort) || 161,
          community: snmpComm,
          oids: snmpOids
            .split(/[\s,]+/)
            .map((s) => s.trim())
            .filter(Boolean),
          version: snmpVer || "2c",
          timeout_ms: Number(snmpTo) || 5000,
          retries: Number(snmpRetries) || 0,
        },
      }),
    onMutate: () => show("info", "A executar SNMP GET no agente…"),
    onSuccess: (d) => {
      if (isRecord(d) && d.ok === true) show("ok", "SNMP GET concluído com sucesso.");
      else if (isRecord(d) && typeof d.error === "string" && d.error) show("err", `SNMP GET: ${d.error}`);
      else show("ok", "SNMP GET concluído. Verifique o resultado abaixo.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const [bulkHost, setBulkHost] = useState("127.0.0.1");
  const [bulkComm, setBulkComm] = useState("public");
  const [walkHost, setWalkHost] = useState("127.0.0.1");
  const [walkPort, setWalkPort] = useState("161");
  const [walkComm, setWalkComm] = useState("public");
  const [walkVer, setWalkVer] = useState("2c");
  const [walkTo, setWalkTo] = useState("8000");
  const [walkRetries, setWalkRetries] = useState("0");
  const [walkRootOid, setWalkRootOid] = useState("1.3.6.1.2.1");
  const [walkMaxRows, setWalkMaxRows] = useState("8000");
  const [walkSearch, setWalkSearch] = useState("");
  const connDefaults = useQuery({
    queryKey: ["tools-conn-defaults"],
    queryFn: () => apiFetch<{ snmp_community_value?: string }>("/api/v1/settings/connection/defaults"),
  });
  useEffect(() => {
    const v = (connDefaults.data?.snmp_community_value ?? "").trim();
    if (!v) return;
    setSnmpComm((prev) => (prev === "public" || prev.trim() === "" ? v : prev));
    setBulkComm((prev) => (prev === "public" || prev.trim() === "" ? v : prev));
    setWalkComm((prev) => (prev === "public" || prev.trim() === "" ? v : prev));
  }, [connDefaults.data]);

  const [bulkOids, setBulkOids] = useState("1.3.6.1.2.1.1.1.0\n1.3.6.1.2.1.1.3.0");
  const [bulkTo, setBulkTo] = useState("8000");
  const bulkRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/snmp/bulk-get", {
        method: "POST",
        json: {
          host: bulkHost.trim(),
          community: bulkComm,
          oids: bulkOids
            .split(/[\s,\n]+/)
            .map((s) => s.trim())
            .filter(Boolean),
          timeout_ms: Number(bulkTo) || 8000,
        },
      }),
    onMutate: () => show("info", "A executar SNMP bulk-get (vários OIDs)…"),
    onSuccess: (d) => {
      if (isRecord(d) && d.ok === true) show("ok", "SNMP bulk-get concluído com sucesso.");
      else if (isRecord(d) && typeof d.error === "string" && d.error) show("err", `SNMP bulk-get: ${d.error}`);
      else show("ok", "SNMP bulk-get concluído. Verifique o resultado abaixo.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const [tnHost, setTnHost] = useState("127.0.0.1");
  const [tnPort, setTnPort] = useState("23");
  const [tnUser, setTnUser] = useState("");
  const [tnPass, setTnPass] = useState("");
  const [tnTo, setTnTo] = useState("8000");
  const [tnMax, setTnMax] = useState("4096");
  const telnetRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/telnet/test", {
        method: "POST",
        json: {
          host: tnHost.trim(),
          port: tnPort.trim() || "23",
          timeout_ms: Number(tnTo) || 8000,
          user: tnUser.trim() || undefined,
          password: tnPass || undefined,
          max_read_bytes: tnMax ? Number(tnMax) : undefined,
        },
      }),
    onMutate: () => show("info", `A testar Telnet em ${tnHost.trim() || "…"}:${tnPort.trim() || "23"}…`),
    onSuccess: (d) => {
      if (isRecord(d) && d.ok === true) show("ok", "Telnet: ligação e leitura concluídas com sucesso.");
      else if (isRecord(d) && typeof d.error === "string" && d.error) show("err", `Telnet: ${d.error}`);
      else show("err", "Telnet: teste concluído sem sucesso. Veja os detalhes abaixo.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const [sshHost, setSshHost] = useState("127.0.0.1");
  const [sshPort, setSshPort] = useState("22");
  const [sshUser, setSshUser] = useState("nouser");
  const [sshPass, setSshPass] = useState("x");
  const [sshTo, setSshTo] = useState("12000");
  const sshRun = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/tools/ssh/test", {
        method: "POST",
        json: {
          host: sshHost.trim(),
          port: sshPort.trim() || "22",
          user: sshUser.trim(),
          password: sshPass,
          timeout_ms: Number(sshTo) || 12000,
        },
      }),
    onMutate: () => show("info", `A testar handshake SSH em ${sshHost.trim() || "…"}…`),
    onSuccess: (d) => {
      if (isRecord(d) && d.ok === true) show("ok", "SSH: autenticação e dial concluídos com sucesso.");
      else if (isRecord(d) && typeof d.error === "string" && d.error) show("err", `SSH: ${d.error}`);
      else show("err", "SSH: teste falhou. Veja os detalhes abaixo.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const walkRun = useMutation({
    mutationFn: () =>
      apiFetch<{ job_id?: string }>("/api/v1/tools/snmp-walk/run", {
        method: "POST",
        json: {
          host: walkHost.trim(),
          port: Number(walkPort) || 161,
          community: walkComm,
          version: walkVer || "2c",
          timeout_ms: Number(walkTo) || 8000,
          retries: Number(walkRetries) || 0,
          root_oid: walkRootOid.trim() || "1.3.6.1.2.1",
          max_rows: Number(walkMaxRows) || 8000,
        },
      }),
    onMutate: () => show("info", "A enfileirar walk SNMP no servidor…"),
    onSuccess: (data) => {
      const jid = typeof data?.job_id === "string" && data.job_id.trim() ? data.job_id.trim() : "";
      if (jid) {
        setWalkJobId(jid);
        show("ok", `Walk SNMP enfileirado. Identificador do job: ${jid}.`);
      } else {
        show("ok", "Walk SNMP enfileirado. Consulte a resposta abaixo.");
      }
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });
  const [walkJobId, setWalkJobId] = useState("tools-walk-local");
  const walkRows = useMutation({
    mutationFn: () =>
      apiFetch(
        `/api/v1/tools/snmp-walk/jobs/${encodeURIComponent(walkJobId)}/rows?limit=300&offset=0&search=${encodeURIComponent(walkSearch.trim())}`,
      ),
    onMutate: () => show("info", "A carregar linhas do resultado do walk…"),
    onSuccess: (d) => {
      const total = isRecord(d) && typeof d.total === "number" ? d.total : undefined;
      show("ok", total != null ? `Linhas carregadas (${total} no filtro atual).` : "Linhas do walk carregadas.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });
  const walkDisc = useMutation({
    mutationFn: () => apiFetch(`/api/v1/tools/snmp-walk/jobs/${encodeURIComponent(walkJobId)}/discoveries`),
    onMutate: () => show("info", "A carregar descobertas agrupadas do walk…"),
    onSuccess: (d) => {
      const n = isRecord(d) && Array.isArray(d.candidates) ? d.candidates.length : undefined;
      show("ok", n != null ? `Descobertas carregadas (${n} grupo(s)).` : "Descobertas do walk carregadas.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  const [mtHost, setMtHost] = useState("");
  const [mtComm, setMtComm] = useState("public");
  const [mtTo, setMtTo] = useState("8000");
  const [mtJobId, setMtJobId] = useState("");
  useEffect(() => {
    const v = (connDefaults.data?.snmp_community_value ?? "").trim();
    if (!v) return;
    setMtComm((prev) => (prev === "public" || prev.trim() === "" ? v : prev));
  }, [connDefaults.data]);
  const mtWalkIf = useMutation({
    mutationFn: () =>
      apiFetch<{ job_id?: string }>("/api/v1/tools/mikrotik/walk", {
        method: "POST",
        json: {
          host: mtHost.trim(),
          community: mtComm,
          timeout_ms: Math.min(120000, Math.max(1000, Number(mtTo) || 8000)),
          port: 161,
          version: "2c",
          retries: 0,
          max_rows: 8000,
        },
      }),
    onMutate: () => show("info", "A iniciar walk IF-MIB (interfaces) no equipamento…"),
    onSuccess: (data) => {
      const jid = typeof data?.job_id === "string" && data.job_id.trim() ? data.job_id.trim() : "";
      if (jid) setMtJobId(jid);
      show("ok", jid ? `Walk Mikrotik enfileirado. Job: ${jid}.` : "Walk Mikrotik enfileirado.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });
  const mtWalkRows = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/tools/snmp-walk/jobs/${encodeURIComponent(mtJobId)}/rows?limit=400&offset=0&search=`),
    onMutate: () => show("info", "A carregar linhas do walk de interfaces…"),
    onSuccess: (d) => {
      const total = isRecord(d) && typeof d.total === "number" ? d.total : undefined;
      show("ok", total != null ? `Interfaces: ${total} linha(s) carregada(s).` : "Linhas do walk carregadas.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });
  const mtWalkDisc = useMutation({
    mutationFn: () => apiFetch(`/api/v1/tools/snmp-walk/jobs/${encodeURIComponent(mtJobId)}/discoveries`),
    onMutate: () => show("info", "A carregar amostras agrupadas (descobertas)…"),
    onSuccess: (d) => {
      const n = isRecord(d) && Array.isArray(d.candidates) ? d.candidates.length : undefined;
      show("ok", n != null ? `Descobertas carregadas (${n} grupo(s)).` : "Descobertas carregadas.");
    },
    onError: (e) => show("err", e instanceof Error ? e.message : String(e)),
  });

  return (
    <div className="tools-page">
      <ToolsPageToastHost toast={toast} onDismiss={dismiss} />
      <h1 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
        Ferramentas de rede
        <InfoHint label="Sobre as ferramentas de rede">
          <p>
            Diagnóstico executado no <strong>servidor NetQuasar</strong>. Cada ação mostra um aviso no canto superior direito (em curso, sucesso ou erro).
            Walks SNMP correm em segundo plano — guarde o identificador do job para consultar linhas e descobertas.
          </p>
        </InfoHint>
      </h1>

      <div className="tabs" style={{ flexWrap: "wrap", marginBottom: "0.35rem" }}>
        {(
          [
            ["host_ping", "Domínios"],
            ["http_matrix", "HTTP/HTTPS"],
            ["icmp", "ICMP"],
            ["tracert", "Tracert"],
            ["nmap", "Nmap"],
            ["snmp", "SNMP get"],
            ["snmp_bulk", "SNMP bulk"],
            ["telnet", "Telnet"],
            ["ssh", "SSH"],
            ["snmp_walk", "SNMP walk"],
            ["mikrotik", "Mikrotik"],
          ] as const
        ).map(([k, lab]) => (
          <button
            key={k}
            type="button"
            className={tab === k ? "active" : ""}
            onClick={() => {
              setTab(k);
              show("info", `${lab}: preencha os campos e execute a ação. Os resultados aparecem abaixo.`, 4500);
            }}
          >
            {lab}
          </button>
        ))}
      </div>

      {tab === "host_ping" && (
        <ToolsPanel
          title="Ping a hosts ou domínios"
          description="Um nome por linha ou separados por vírgula. O servidor resolve DNS e envia ICMP por alvo (até 100 por execução). Timeout por requisição: 500–15000 ms."
          results={
            <>
              <ToolOutputError err={hostPingRun.error as Error | null} />
              {hostPingRun.data ? (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Host / domínio</th>
                        <th>Estado</th>
                        <th>Latência (ms)</th>
                        <th>Detalhe</th>
                      </tr>
                    </thead>
                    <tbody>
                      {hostPingRun.data.rows.map((r) => (
                        <tr key={r.host}>
                          <td className="mono">{r.host}</td>
                          <td>{r.ok ? <span className="badge badge--ok">Acessível</span> : <span className="badge badge--off">Inacessível</span>}</td>
                          <td className="mono">{r.ok && r.rtt_ms != null ? String(r.rtt_ms) : "—"}</td>
                          <td style={{ fontSize: 11, color: "var(--muted)", maxWidth: 320 }}>
                            {[r.error, r.note].filter(Boolean).join(" · ") || "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  <p style={{ color: "var(--muted)", fontSize: 11, marginTop: 8 }}>{hostPingRun.data.note}</p>
                </div>
              ) : null}
            </>
          }
        >
          <div className="row" style={{ flexWrap: "wrap", gap: 8, marginBottom: 8, alignItems: "center" }}>
            <label className="row" style={{ gap: 6, alignItems: "center" }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Timeout (ms)</span>
              <input
                className="input mono"
                style={{ width: 90 }}
                value={hostPingTimeout}
                onChange={(e) => setHostPingTimeout(e.target.value)}
                title="500–15000 ms por requisição ICMP"
              />
            </label>
          </div>
          <textarea
            className="textarea mono"
            rows={10}
            placeholder={"exemplo.pt\ngoogle.com\n8.8.8.8"}
            value={hostPingText}
            onChange={(e) => setHostPingText(e.target.value)}
          />
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={hostPingRun.isPending} onClick={() => hostPingRun.mutate()}>
              {hostPingRun.isPending ? "A executar ping…" : "Executar ping"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "http_matrix" && (
        <ToolsPanel
          title="HTTP / HTTPS por IP e porta"
          description="Lista de IPs e de portas (máx. 64 combinações). Cada célula testa GET HTTPS e GET HTTP no servidor. Opcional: aceitar certificados TLS inseguros para diagnóstico."
          results={
            <>
              <ToolOutputError err={httpMatrixRun.error as Error | null} />
              {httpMatrixRun.data ? (
                <>
                  <p style={{ color: "var(--muted)", fontSize: 11 }}>{httpMatrixRun.data.note}</p>
                  <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap", alignItems: "center" }}>
                    <span style={{ fontSize: 12, color: "var(--muted)" }}>Mostrar</span>
                    {(
                      [
                        ["all", "Todos", "Filtro: todas as combinações."],
                        ["ok", "Acessíveis", "Filtro: HTTP ou HTTPS OK (pelo menos um)."],
                        ["blocked", "Bloqueados", "Filtro: HTTP e HTTPS falharam (nenhum acessível)."],
                      ] as const
                    ).map(([k, lab, hint]) => (
                      <button
                        key={k}
                        type="button"
                        className={httpMatrixFilter === k ? "btn btn--primary" : "btn"}
                        style={{ fontSize: 12 }}
                        onClick={() => {
                          setHttpMatrixFilter(k);
                          show("info", hint, 4000);
                        }}
                      >
                        {lab}
                      </button>
                    ))}
                    <span style={{ fontSize: 11, color: "var(--muted)" }}>
                      {httpMatrixRowsFiltered.length} de {httpMatrixRun.data.rows.length} combinações IP×porta
                    </span>
                  </div>
                  <div className="table-wrap" style={{ marginTop: 8 }}>
                    {httpMatrixRowsFiltered.length === 0 ? (
                      <p style={{ padding: 12, margin: 0, color: "var(--muted)", fontSize: 13 }}>
                        Nenhuma linha corresponde a este filtro.
                      </p>
                    ) : (
                      <table>
                        <thead>
                          <tr>
                            <th>IP</th>
                            <th>Porta</th>
                            <th>HTTPS</th>
                            <th>HTTP</th>
                          </tr>
                        </thead>
                        <tbody>
                          {httpMatrixRowsFiltered.map((r) => (
                            <tr key={`${r.ip}:${r.port}`}>
                              <td className="mono">{r.ip}</td>
                              <td className="mono">{r.port}</td>
                              <td style={{ fontSize: 11, verticalAlign: "top" }}>
                                <HttpProbeCellSummary probe={r.https} />
                              </td>
                              <td style={{ fontSize: 11, verticalAlign: "top" }}>
                                <HttpProbeCellSummary probe={r.http} />
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
                  </div>
                </>
              ) : null}
            </>
          }
        >
          <div className="tools-http-matrix-inputs">
            <div className="field tools-http-matrix-inputs__col">
              <label>IPs</label>
              <textarea className="textarea mono tools-http-matrix-inputs__textarea" rows={8} value={mxIps} onChange={(e) => setMxIps(e.target.value)} />
            </div>
            <div className="field tools-http-matrix-inputs__col">
              <label>Portas</label>
              <textarea className="textarea mono tools-http-matrix-inputs__textarea" rows={8} value={mxPorts} onChange={(e) => setMxPorts(e.target.value)} />
            </div>
            <div className="field tools-http-matrix-inputs__opts">
              <label>Timeout (ms)</label>
              <input className="input mono" value={mxTo} onChange={(e) => setMxTo(e.target.value)} />
              <label className="row" style={{ gap: 6, marginTop: 10, alignItems: "flex-start" }}>
                <input type="checkbox" checked={mxInsecure} onChange={(e) => setMxInsecure(e.target.checked)} /> TLS inseguro
              </label>
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={httpMatrixRun.isPending} onClick={() => httpMatrixRun.mutate()}>
              {httpMatrixRun.isPending ? "A testar…" : "Executar matriz"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "tracert" && (
        <ToolsPanel
          title="Tracert / traceroute"
          description="Rastreia o caminho até ao destino a partir do servidor NetQuasar (tracert no Windows, traceroute/tracepath no Linux)."
          results={
            <>
              <ToolOutputError err={tracertRun.error as Error | null} />
              {tracertRun.data !== undefined ? <NetworkToolTextOutput data={tracertRun.data} /> : null}
            </>
          }
        >
          <div className="field" style={{ marginBottom: 0 }}>
            <label htmlFor="tools-tracert-host">Host ou IP</label>
            <input id="tools-tracert-host" className="input mono" value={tracertHost} onChange={(e) => setTracertHost(e.target.value)} />
          </div>
          <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: 8 }}>
            <div className="field" style={{ marginBottom: 0, maxWidth: 120 }}>
              <label htmlFor="tools-tracert-hops">Saltos máx.</label>
              <input id="tools-tracert-hops" className="input mono" value={tracertHops} onChange={(e) => setTracertHops(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0, maxWidth: 140 }}>
              <label htmlFor="tools-tracert-to">Timeout total (ms)</label>
              <input id="tools-tracert-to" className="input mono" value={tracertTo} onChange={(e) => setTracertTo(e.target.value)} />
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={tracertRun.isPending || !tracertHost.trim()} onClick={() => tracertRun.mutate()}>
              {tracertRun.isPending ? "A rastrear…" : "Executar tracert"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "nmap" && (
        <ToolsPanel
          title="Nmap (varredura rápida)"
          description="Requer nmap instalado no servidor. Modo «ping» (-sn): descobre se o host está activo; «rápida» (-F): portas mais comuns."
          results={
            <>
              <ToolOutputError err={nmapRun.error as Error | null} />
              {nmapRun.data !== undefined ? <NetworkToolTextOutput data={nmapRun.data} /> : null}
            </>
          }
        >
          <div className="field" style={{ marginBottom: 0 }}>
            <label htmlFor="tools-nmap-host">Host ou IP</label>
            <input id="tools-nmap-host" className="input mono" value={nmapHost} onChange={(e) => setNmapHost(e.target.value)} />
          </div>
          <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: 8, alignItems: "flex-end" }}>
            <div className="field" style={{ marginBottom: 0, minWidth: 140 }}>
              <label htmlFor="tools-nmap-mode">Modo</label>
              <select id="tools-nmap-mode" className="input" value={nmapMode} onChange={(e) => setNmapMode(e.target.value)}>
                <option value="sn">Ping / host up (-sn)</option>
                <option value="quick">Portas comuns (-F)</option>
              </select>
            </div>
            <div className="field" style={{ marginBottom: 0, maxWidth: 140 }}>
              <label htmlFor="tools-nmap-to">Timeout (ms)</label>
              <input id="tools-nmap-to" className="input mono" value={nmapTo} onChange={(e) => setNmapTo(e.target.value)} />
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={nmapRun.isPending || !nmapHost.trim()} onClick={() => nmapRun.mutate()}>
              {nmapRun.isPending ? "A varrer…" : "Executar nmap"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "icmp" && (
        <ToolsPanel
          title="ICMP ping (único alvo)"
          description="Um host ou IP por requisição. Útil para teste rápido com o mesmo motor ICMP usado noutras partes do sistema."
          results={
            <>
              <ToolOutputError err={icmpRun.error as Error | null} />
              {icmpRun.data !== undefined ? <IcmpSingleOutput data={icmpRun.data} /> : null}
            </>
          }
        >
          <div className="field" style={{ marginBottom: 0 }}>
            <label htmlFor="tools-icmp-host">Host ou IP</label>
            <input id="tools-icmp-host" className="input mono" value={icmpHost} onChange={(e) => setIcmpHost(e.target.value)} />
          </div>
          <div className="field" style={{ marginBottom: 0, maxWidth: 140 }}>
            <label htmlFor="tools-icmp-to">Timeout (ms)</label>
            <input id="tools-icmp-to" className="input mono" value={icmpTo} onChange={(e) => setIcmpTo(e.target.value)} />
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={icmpRun.isPending} onClick={() => icmpRun.mutate()}>
              {icmpRun.isPending ? "A enviar…" : "Ping"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "snmp" && (
        <ToolsPanel
          title="SNMP GET"
          description="Leitura de um ou mais OIDs (v1/v2c) no agente indicado. A comunidade pode ser preenchida a partir das definições de rede se existir padrão salvo."
          results={
            <>
              <ToolOutputError err={snmpRun.error as Error | null} />
              {snmpRun.data !== undefined ? <SnmpGetOutput data={snmpRun.data} /> : null}
            </>
          }
        >
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))",
              gap: 12,
              alignItems: "end",
            }}
          >
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-snmp-host">Host / IP</label>
              <input id="tools-snmp-host" className="input mono" value={snmpHost} onChange={(e) => setSnmpHost(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-snmp-port">Porta</label>
              <input id="tools-snmp-port" className="input mono" value={snmpPort} onChange={(e) => setSnmpPort(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-snmp-comm">Comunidade</label>
              <input id="tools-snmp-comm" className="input" value={snmpComm} onChange={(e) => setSnmpComm(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-snmp-ver">Versão</label>
              <input id="tools-snmp-ver" className="input mono" value={snmpVer} onChange={(e) => setSnmpVer(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-snmp-to">Timeout (ms)</label>
              <input id="tools-snmp-to" className="input mono" value={snmpTo} onChange={(e) => setSnmpTo(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-snmp-ret">Repetições</label>
              <input id="tools-snmp-ret" className="input mono" value={snmpRetries} onChange={(e) => setSnmpRetries(e.target.value)} />
            </div>
          </div>
          <div className="field" style={{ marginTop: 12 }}>
            <label htmlFor="tools-snmp-oids">OIDs (espaço ou vírgula)</label>
            <input id="tools-snmp-oids" className="input mono" value={snmpOids} onChange={(e) => setSnmpOids(e.target.value)} />
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={snmpRun.isPending} onClick={() => snmpRun.mutate()}>
              {snmpRun.isPending ? "A pedir…" : "Executar GET"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "snmp_bulk" && (
        <ToolsPanel
          title="SNMP bulk-get (v2c)"
          description="Vários OIDs num única requisição GET-BULK. Indique host, comunidade, timeout e lista de OIDs (um por linha)."
          results={
            <>
              <ToolOutputError err={bulkRun.error as Error | null} />
              {bulkRun.data !== undefined ? <SnmpGetOutput data={bulkRun.data} /> : null}
            </>
          }
        >
          <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "flex-end" }}>
            <div className="field" style={{ marginBottom: 0, flex: "1 1 180px" }}>
              <label htmlFor="tools-bulk-host">Host / IP</label>
              <input id="tools-bulk-host" className="input mono" value={bulkHost} onChange={(e) => setBulkHost(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0, width: 140 }}>
              <label htmlFor="tools-bulk-comm">Comunidade</label>
              <input id="tools-bulk-comm" className="input" value={bulkComm} onChange={(e) => setBulkComm(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0, width: 120 }}>
              <label htmlFor="tools-bulk-to">Timeout (ms)</label>
              <input id="tools-bulk-to" className="input mono" value={bulkTo} onChange={(e) => setBulkTo(e.target.value)} />
            </div>
          </div>
          <div className="field" style={{ marginTop: 12 }}>
            <label htmlFor="tools-bulk-oids">OIDs (um por linha)</label>
            <textarea id="tools-bulk-oids" className="textarea mono" rows={5} value={bulkOids} onChange={(e) => setBulkOids(e.target.value)} />
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={bulkRun.isPending} onClick={() => bulkRun.mutate()}>
              {bulkRun.isPending ? "A pedir…" : "Executar bulk GET"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "telnet" && (
        <ToolsPanel
          title="Telnet"
          description="Teste não interativo: abre TCP, lê banner e opcionalmente envia credenciais em texto claro. Use só em redes de gestão confiáveis."
          results={
            <>
              <ToolOutputError err={telnetRun.error as Error | null} />
              {telnetRun.data !== undefined ? <TelnetTestOutput data={telnetRun.data} /> : null}
            </>
          }
        >
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(200px, 1fr))",
              gap: "12px",
              alignItems: "end",
            }}
          >
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-tn-host">Host ou IP do destino</label>
              <input id="tools-tn-host" className="input mono" value={tnHost} onChange={(e) => setTnHost(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-tn-port">Porta TCP (predefinição 23)</label>
              <input id="tools-tn-port" className="input mono" style={{ maxWidth: 120 }} value={tnPort} onChange={(e) => setTnPort(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-tn-to">Tempo máximo de espera (ms)</label>
              <input id="tools-tn-to" className="input mono" style={{ maxWidth: 120 }} value={tnTo} onChange={(e) => setTnTo(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-tn-max">Máximo de bytes a ler após login</label>
              <input id="tools-tn-max" className="input mono" style={{ maxWidth: 120 }} value={tnMax} onChange={(e) => setTnMax(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-tn-user">Utilizador (opcional, enviado em texto claro)</label>
              <input id="tools-tn-user" className="input" value={tnUser} onChange={(e) => setTnUser(e.target.value)} autoComplete="off" />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-tn-pass">Palavra-passe (opcional)</label>
              <input id="tools-tn-pass" className="input" type="password" value={tnPass} onChange={(e) => setTnPass(e.target.value)} autoComplete="off" />
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={telnetRun.isPending} onClick={() => telnetRun.mutate()}>
              {telnetRun.isPending ? "A testar…" : "Testar ligação"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "ssh" && (
        <ToolsPanel
          title="SSH (teste de dial)"
          description="Verifica se o servidor aceita autenticação por palavra-passe nesta sessão. A chave do host não é verificada — apenas para diagnóstico."
          results={
            <>
              <ToolOutputError err={sshRun.error as Error | null} />
              {sshRun.data !== undefined ? <SshDialOutput data={sshRun.data} /> : null}
            </>
          }
        >
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(200px, 1fr))",
              gap: "12px",
              alignItems: "end",
            }}
          >
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-ssh-host">Host ou IP do servidor SSH</label>
              <input id="tools-ssh-host" className="input mono" value={sshHost} onChange={(e) => setSshHost(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-ssh-port">Porta TCP (predefinição 22)</label>
              <input id="tools-ssh-port" className="input mono" style={{ maxWidth: 120 }} value={sshPort} onChange={(e) => setSshPort(e.target.value)} />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-ssh-user">Nome de utilizador SSH</label>
              <input id="tools-ssh-user" className="input" value={sshUser} onChange={(e) => setSshUser(e.target.value)} autoComplete="off" />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-ssh-pass">Palavra-passe</label>
              <input id="tools-ssh-pass" className="input" type="password" value={sshPass} onChange={(e) => setSshPass(e.target.value)} autoComplete="off" />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label htmlFor="tools-ssh-to">Tempo máximo de espera (ms)</label>
              <input id="tools-ssh-to" className="input mono" style={{ maxWidth: 120 }} value={sshTo} onChange={(e) => setSshTo(e.target.value)} />
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={sshRun.isPending} onClick={() => sshRun.mutate()}>
              {sshRun.isPending ? "A testar…" : "Testar SSH"}
            </button>
          </div>
        </ToolsPanel>
      )}

      {tab === "snmp_walk" && (
        <ToolsPanel
          title="SNMP Walk"
          description="Mesmo preenchimento do SNMP GET, mas para varredura (walk) a partir de um OID raiz. O job corre no servidor — guarde o job_id para consultar linhas e descobertas."
          results={
            <>
              <ToolOutputError err={walkRun.error as Error | null} />
              {walkRun.data !== undefined ? <WalkJobQueuedOutput data={walkRun.data} /> : null}
              {walkRows.isError ? <ToolOutputError err={walkRows.error as Error | null} /> : null}
              {walkRows.data !== undefined ? <SnmpWalkRowsOutput data={walkRows.data} /> : null}
              {walkDisc.isError ? <ToolOutputError err={walkDisc.error as Error | null} /> : null}
              {walkDisc.data !== undefined ? <SnmpWalkDiscoveriesOutput data={walkDisc.data} /> : null}
            </>
          }
        >
          <div className="field">
            <label>Host / porta / comunidade / versão</label>
            <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
              <input className="input" value={walkHost} onChange={(e) => setWalkHost(e.target.value)} placeholder="host ou IP" />
              <input className="input" style={{ width: 72 }} value={walkPort} onChange={(e) => setWalkPort(e.target.value)} placeholder="161" />
              <input className="input" style={{ width: 120 }} value={walkComm} onChange={(e) => setWalkComm(e.target.value)} placeholder="community" />
              <input className="input" style={{ width: 56 }} value={walkVer} onChange={(e) => setWalkVer(e.target.value)} placeholder="2c" />
            </div>
          </div>
          <div className="field">
            <label>OID raiz / timeout / retries / máximo de linhas</label>
            <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
              <input className="input mono" style={{ minWidth: 260 }} value={walkRootOid} onChange={(e) => setWalkRootOid(e.target.value)} placeholder="1.3.6.1.2.1" />
              <input className="input" style={{ width: 100 }} value={walkTo} onChange={(e) => setWalkTo(e.target.value)} placeholder="timeout_ms" />
              <input className="input" style={{ width: 80 }} value={walkRetries} onChange={(e) => setWalkRetries(e.target.value)} placeholder="retries" />
              <input className="input" style={{ width: 110 }} value={walkMaxRows} onChange={(e) => setWalkMaxRows(e.target.value)} placeholder="max_rows" />
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={walkRun.isPending} onClick={() => walkRun.mutate()}>
              {walkRun.isPending ? "A iniciar…" : "Iniciar walk"}
            </button>
          </div>
          <div className="field" style={{ marginTop: 8 }}>
            <label>Job e consulta</label>
            <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "center" }}>
              <input className="input mono" style={{ width: 260 }} value={walkJobId} onChange={(e) => setWalkJobId(e.target.value)} placeholder="job_id" />
              <input
                className="input"
                style={{ minWidth: 220 }}
                value={walkSearch}
                onChange={(e) => setWalkSearch(e.target.value)}
                placeholder="filtro por OID, tipo ou valor"
              />
              <button type="button" className="btn" disabled={walkRows.isPending} onClick={() => walkRows.mutate()}>
                {walkRows.isPending ? "A carregar…" : "Ver linhas"}
              </button>
              <button type="button" className="btn" disabled={walkDisc.isPending} onClick={() => walkDisc.mutate()}>
                {walkDisc.isPending ? "A carregar…" : "Ver descobertas"}
              </button>
            </div>
          </div>
        </ToolsPanel>
      )}

      {tab === "mikrotik" && (
        <ToolsPanel
          title="Mikrotik — walk de interfaces (IF-MIB)"
          description={
            <>
              Varredura SNMP a partir de <span className="mono">1.3.6.1.2.1.2.2.1</span> (ifTable). O job corre no servidor; use os botões abaixo para ver linhas e
              amostras agrupadas (útil em RouterOS e outros agentes).
            </>
          }
          results={
            <>
              <ToolOutputError err={mtWalkIf.error as Error | null} />
              {mtWalkIf.data !== undefined ? <WalkJobQueuedOutput data={mtWalkIf.data} /> : null}
              {mtWalkRows.isError ? <ToolOutputError err={mtWalkRows.error as Error | null} /> : null}
              {mtWalkRows.data !== undefined ? <SnmpWalkRowsOutput data={mtWalkRows.data} /> : null}
              {mtWalkDisc.isError ? <ToolOutputError err={mtWalkDisc.error as Error | null} /> : null}
              {mtWalkDisc.data !== undefined ? <SnmpWalkDiscoveriesOutput data={mtWalkDisc.data} /> : null}
            </>
          }
        >
          <div className="field">
            <label>IP ou hostname · comunidade SNMP · timeout (ms)</label>
            <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
              <input className="input mono" style={{ minWidth: 160 }} value={mtHost} onChange={(e) => setMtHost(e.target.value)} placeholder="192.168.88.1" />
              <input className="input" style={{ width: 120 }} value={mtComm} onChange={(e) => setMtComm(e.target.value)} placeholder="community" />
              <input className="input" style={{ width: 100 }} value={mtTo} onChange={(e) => setMtTo(e.target.value)} placeholder="timeout_ms" />
            </div>
          </div>
          <div className="tools-panel__actions">
            <button type="button" className="btn btn--primary" disabled={mtWalkIf.isPending || !mtHost.trim()} onClick={() => mtWalkIf.mutate()}>
              {mtWalkIf.isPending ? "A iniciar…" : "Descobrir interfaces (walk)"}
            </button>
          </div>
          <div className="field" style={{ marginTop: 8 }}>
            <label>Job e consulta</label>
            <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "center" }}>
              <input className="input mono" style={{ minWidth: 280 }} value={mtJobId} onChange={(e) => setMtJobId(e.target.value)} placeholder="job_id (preenchido após o walk)" />
              <button type="button" className="btn" disabled={mtWalkRows.isPending || !mtJobId.trim()} onClick={() => mtWalkRows.mutate()}>
                {mtWalkRows.isPending ? "A carregar…" : "Ver linhas"}
              </button>
              <button type="button" className="btn" disabled={mtWalkDisc.isPending || !mtJobId.trim()} onClick={() => mtWalkDisc.mutate()}>
                {mtWalkDisc.isPending ? "A carregar…" : "Ver descobertas"}
              </button>
            </div>
          </div>
        </ToolsPanel>
      )}
    </div>
  );
}
