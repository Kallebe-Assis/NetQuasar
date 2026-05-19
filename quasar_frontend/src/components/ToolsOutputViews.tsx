import { useState, type CSSProperties, type ReactNode } from "react";

function isRecord(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

function str(v: unknown): string {
  if (v === null || v === undefined) return "";
  return String(v);
}

const preBox: CSSProperties = {
  margin: 0,
  marginTop: 8,
  padding: 10,
  maxHeight: 220,
  overflow: "auto",
  fontSize: 11,
  lineHeight: 1.45,
  background: "var(--panel2)",
  borderRadius: "var(--radius)",
  border: "1px solid var(--border)",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
};

const panelStyle: CSSProperties = {
  marginTop: 12,
  padding: 12,
  borderRadius: "var(--radius)",
  border: "1px solid var(--border)",
  background: "var(--panel)",
};

export function ToolOutputError({ err }: { err: Error | null }) {
  if (!err) return null;
  return (
    <div className="msg msg--err" style={{ marginTop: 12 }}>
      {err.message}
    </div>
  );
}

function CollapsibleRawJson({ data }: { data: unknown }) {
  const [open, setOpen] = useState(false);
  if (data === undefined || data === null) return null;
  return (
    <div style={{ marginTop: 10 }}>
      <button type="button" className="btn" style={{ fontSize: 12, padding: "4px 10px" }} onClick={() => setOpen((o) => !o)}>
        {open ? "Ocultar JSON bruto" : "Ver JSON bruto"}
      </button>
      {open ? <pre className="mono" style={{ ...preBox, maxHeight: 360 }}>{JSON.stringify(data, null, 2)}</pre> : null}
    </div>
  );
}

function kvRow(k: string, v: ReactNode) {
  return (
    <div className="row" style={{ justifyContent: "space-between", gap: 12, alignItems: "baseline", flexWrap: "wrap" }}>
      <span style={{ color: "var(--muted)", fontSize: 12 }}>{k}</span>
      <span style={{ fontSize: 13, textAlign: "right" }}>{v}</span>
    </div>
  );
}

/** Erros de timeout na matriz HTTP: só o estado «Erro» — sem texto longo nem link. */
function isHttpProbeTimeoutStyleError(raw: string): boolean {
  const low = raw.toLowerCase();
  if (low.includes("context deadline exceeded")) return true;
  if (low.includes("i/o timeout")) return true;
  if (low.includes("timeout") && !low.includes("x509")) return true;
  return false;
}

/** Traduz mensagens técnicas do cliente HTTP em texto legível (sem prefixos tipo Get "http://…"). */
export function friendlyHttpProbeError(raw: string): string {
  let s = raw.trim();
  s = s.replace(/^(?:Get|Post|Put|Head|Patch|Delete)\s+"[^"]*":\s*/i, "").trim();
  s = s.replace(/^dial\s+tcp[^:]*:\s*/i, "").trim();
  const low = s.toLowerCase();
  if (low.includes("context deadline exceeded")) return "Tempo de espera esgotado (o destino não respondeu a tempo).";
  if (low.includes("i/o timeout") || (low.includes("timeout") && !low.includes("x509"))) return "Tempo de espera esgotado.";
  if (low.includes("connection refused")) return "Conexão recusada (porta fechada ou sem serviço).";
  if (low.includes("connection reset") || low.includes("reset by peer")) return "Conexão terminada pelo destino.";
  if (low.includes("no route to host")) return "Sem rota até ao destino.";
  if (low.includes("network is unreachable")) return "Rede inacessível.";
  if (low.includes("host is down")) return "Host indisponível.";
  if (low.includes("tls") && low.includes("handshake")) return "Falha no handshake TLS.";
  if (low.includes("certificate") || low.includes("x509")) return "Problema com o certificado TLS.";
  if (low.includes("eof") || low.includes("unexpected eof")) return "Conexão fechada de forma inesperada.";
  if (low.includes("no such host") || low.includes("name or service not known")) return "Nome de host não encontrado (DNS).";
  if (s.length > 160) return `${s.slice(0, 157)}…`;
  return s || "Pedido falhou.";
}

/** Probe considerado acessível: resposta OK sem campo de erro. */
export function isHttpProbeAccessible(probe: unknown): boolean {
  if (!isRecord(probe)) return false;
  if (typeof probe.error === "string" && probe.error.trim()) return false;
  return probe.ok === true;
}

/** Filtro «Acessíveis»: pelo menos um dos protocolos respondeu OK. */
export function isHttpMatrixRowAnyProbeAccessible(row: { https: unknown; http: unknown }): boolean {
  return isHttpProbeAccessible(row.https) || isHttpProbeAccessible(row.http);
}

function sanitizeHttpHref(raw: string): string | null {
  const t = raw.trim();
  if (!/^https?:\/\//i.test(t)) return null;
  try {
    const u = new URL(t);
    if (u.protocol !== "http:" && u.protocol !== "https:") return null;
    return u.href;
  } catch {
    return null;
  }
}

/** Primeiro URL http(s) encontrado numa mensagem técnica (ex. Get "http://…"). */
function extractHttpUrlFromMessage(raw: string): string | null {
  const m = raw.match(/https?:\/\/[^\s"'<>]+/i);
  return m ? sanitizeHttpHref(m[0]) : null;
}

function HttpProbeUrlLink({ urlSource }: { urlSource: string | null | undefined }) {
  if (typeof urlSource !== "string" || !urlSource.trim()) return null;
  const href = sanitizeHttpHref(urlSource) ?? extractHttpUrlFromMessage(urlSource);
  if (!href) return null;
  const display = href.length > 80 ? `${href.slice(0, 78)}…` : href;
  return (
    <div style={{ marginTop: 6 }}>
      <a href={href} target="_blank" rel="noopener noreferrer" className="mono" style={{ fontSize: 11, wordBreak: "break-all" }} title={href}>
        {display}
      </a>
    </div>
  );
}

/** Resumo de uma célula HTTP/HTTPS (probe). */
export function HttpProbeCellSummary({ probe }: { probe: unknown }) {
  if (probe === null || probe === undefined) return <span style={{ color: "var(--muted)" }}>—</span>;
  if (!isRecord(probe)) {
    return <span className="mono" style={{ fontSize: 11 }}>{JSON.stringify(probe)}</span>;
  }
  if (typeof probe.error === "string" && probe.error) {
    const errRaw = probe.error;
    const hideDetail = isHttpProbeTimeoutStyleError(errRaw);
    const detail = hideDetail ? "" : friendlyHttpProbeError(errRaw);
    return (
      <div style={{ fontSize: 12 }}>
        <span className="badge badge--err">Erro</span>
        {detail ? (
          <div style={{ marginTop: 6, color: "var(--muted)", fontSize: 11, lineHeight: 1.4 }}>{detail}</div>
        ) : null}
      </div>
    );
  }
  const ok = probe.ok === true;
  const status = probe.status;
  const lat = probe.latency_ms;
  return (
    <div style={{ fontSize: 12 }}>
      <span className={ok ? "badge badge--ok" : "badge badge--err"}>{ok ? "OK" : "Falhou"}</span>
      {typeof status === "number" ? (
        <span style={{ marginLeft: 6, fontFamily: "var(--mono)", fontSize: 11 }}>
          HTTP {status}
        </span>
      ) : null}
      {typeof lat === "number" ? (
        <span style={{ marginLeft: 6, color: "var(--muted)", fontSize: 11 }}>
          {lat} ms
        </span>
      ) : null}
      <HttpProbeUrlLink urlSource={typeof probe.url === "string" ? probe.url : undefined} />
    </div>
  );
}

export function IcmpSingleOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return (
      <>
        <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>
        <CollapsibleRawJson data={data} />
      </>
    );
  }
  const ok = data.ok === true;
  const rtt = typeof data.rtt_ms === "number" ? data.rtt_ms : undefined;
  return (
    <div style={panelStyle}>
      <div className="row" style={{ alignItems: "center", gap: 10, marginBottom: 10 }}>
        <span className={ok ? "badge" : "badge badge--off"}>{ok ? "Acessível" : "Inacessível / erro"}</span>
        {rtt != null ? <span style={{ fontFamily: "var(--mono)", fontSize: 14 }}>{rtt} ms</span> : null}
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {kvRow("Perda de pacotes", <span className="mono">{str(data.packet_loss)}</span>)}
        {kvRow("Enviados / recebidos", (
          <span className="mono">
            {str(data.packets_sent)} / {str(data.packets_recv)}
          </span>
        ))}
        {kvRow("Modo privilegiado", <span>{data.privileged_mode === true ? "sim" : data.privileged_mode === false ? "não" : "—"}</span>)}
        {typeof data.error === "string" && data.error ? kvRow("Erro", <span style={{ color: "var(--err)" }}>{data.error}</span>) : null}
        {typeof data.note === "string" && data.note ? kvRow("Nota", <span style={{ fontSize: 12 }}>{data.note}</span>) : null}
      </div>
      <CollapsibleRawJson data={data} />
    </div>
  );
}

export function SnmpGetOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return (
      <>
        <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>
        <CollapsibleRawJson data={data} />
      </>
    );
  }
  const ok = data.ok === true;
  const vars = Array.isArray(data.vars) ? data.vars : [];
  return (
    <div style={panelStyle}>
      <div className="row" style={{ alignItems: "center", gap: 10, marginBottom: 10 }}>
        <span className={ok ? "badge" : "badge badge--off"}>{ok ? "SNMP OK" : "Falha"}</span>
        {typeof data.error === "string" && data.error ? <span style={{ fontSize: 12, color: "var(--err)" }}>{data.error}</span> : null}
      </div>
      {typeof data.note === "string" && data.note ? <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>{data.note}</p> : null}
      {vars.length > 0 ? (
        <div className="table-wrap" style={{ marginTop: 8 }}>
          <table>
            <thead>
              <tr>
                <th>OID</th>
                <th>Tipo</th>
                <th>Valor</th>
              </tr>
            </thead>
            <tbody>
              {vars.map((row, i) => {
                if (!isRecord(row)) return null;
                return (
                  <tr key={`${str(row.oid)}-${i}`}>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {str(row.oid)}
                    </td>
                    <td style={{ fontSize: 11 }}>{str(row.type)}</td>
                    <td className="mono" style={{ fontSize: 11, maxWidth: 420, wordBreak: "break-all" }}>
                      {str(row.value)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      <CollapsibleRawJson data={data} />
    </div>
  );
}

export function TelnetTestOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return (
      <>
        <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>
        <CollapsibleRawJson data={data} />
      </>
    );
  }
  const ok = data.ok === true;
  return (
    <div style={panelStyle}>
      <div className="row" style={{ alignItems: "center", gap: 10, marginBottom: 8 }}>
        <span className={ok ? "badge" : "badge badge--off"}>{ok ? "Sessão OK" : "Falhou"}</span>
        {typeof data.latency_ms === "number" ? <span className="mono" style={{ fontSize: 13 }}>{data.latency_ms} ms</span> : null}
      </div>
      {typeof data.note === "string" && data.note ? <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>{data.note}</p> : null}
      {typeof data.error === "string" && data.error ? <div className="msg msg--err" style={{ marginTop: 8 }}>{data.error}</div> : null}
      {typeof data.banner === "string" && data.banner ? (
        <div style={{ marginTop: 10 }}>
          <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 4 }}>Banner</div>
          <pre className="mono" style={preBox}>
            {data.banner}
          </pre>
        </div>
      ) : null}
      {typeof data.after_login_snippet === "string" && data.after_login_snippet ? (
        <div style={{ marginTop: 10 }}>
          <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 4 }}>Após login (excerto)</div>
          <pre className="mono" style={preBox}>
            {data.after_login_snippet}
          </pre>
        </div>
      ) : null}
      <CollapsibleRawJson data={data} />
    </div>
  );
}

export function SshDialOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return (
      <>
        <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>
        <CollapsibleRawJson data={data} />
      </>
    );
  }
  const ok = data.ok === true;
  return (
    <div style={panelStyle}>
      <div className="row" style={{ alignItems: "center", gap: 10, marginBottom: 8 }}>
        <span className={ok ? "badge" : "badge badge--off"}>{ok ? "SSH OK" : "Falhou"}</span>
        {typeof data.latency_ms === "number" ? <span className="mono" style={{ fontSize: 13 }}>{data.latency_ms} ms</span> : null}
      </div>
      {typeof data.remote_addr === "string" && data.remote_addr ? kvRow("Remoto", <span className="mono">{data.remote_addr}</span>) : null}
      {typeof data.note === "string" && data.note ? <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 8 }}>{data.note}</p> : null}
      {typeof data.error === "string" && data.error ? <div className="msg msg--err" style={{ marginTop: 8 }}>{data.error}</div> : null}
      <CollapsibleRawJson data={data} />
    </div>
  );
}

/** Resposta 202 ao iniciar walk (fila). */
export function WalkJobQueuedOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>;
  }
  const jid = data.job_id;
  return (
    <div style={panelStyle}>
      <div className="row" style={{ alignItems: "center", gap: 8, marginBottom: 8 }}>
        <span className="badge">Em fila</span>
        {typeof data.status === "string" ? <span style={{ fontSize: 12, color: "var(--muted)" }}>{data.status}</span> : null}
      </div>
      {jid != null ? (
        <div style={{ fontSize: 12, marginBottom: 6 }}>
          <span style={{ color: "var(--muted)" }}>Job ID</span>{" "}
          <code className="mono" style={{ fontSize: 11 }}>
            {str(jid)}
          </code>
        </div>
      ) : null}
      {typeof data.host === "string" && data.host ? kvRow("Host", <span className="mono">{data.host}</span>) : null}
      {typeof data.root_oid === "string" && data.root_oid ? kvRow("OID raiz", <span className="mono" style={{ fontSize: 11 }}>{data.root_oid}</span>) : null}
      {typeof data.note === "string" && data.note ? <p style={{ fontSize: 12, color: "var(--muted)", marginBottom: 0 }}>{data.note}</p> : null}
      <CollapsibleRawJson data={data} />
    </div>
  );
}

type SnmpVarRow = { oid?: string; type?: string; value?: string };

export function SnmpWalkRowsOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>;
  }
  const rows = (Array.isArray(data.rows) ? data.rows : []) as unknown[];
  const parsed: SnmpVarRow[] = rows.map((r) => (isRecord(r) ? { oid: str(r.oid), type: str(r.type), value: str(r.value) } : {}));

  return (
    <div style={panelStyle}>
      <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "center", marginBottom: 10 }}>
        {typeof data.status === "string" ? <span className="badge">{data.status}</span> : null}
        {data.job_id != null ? (
          <span className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
            job {str(data.job_id)}
          </span>
        ) : null}
        {typeof data.total === "number" ? (
          <span style={{ fontSize: 12, color: "var(--muted)" }}>
            {data.total} linha(s) (offset {str(data.offset)}, limite {str(data.limit)})
          </span>
        ) : null}
      </div>
      {typeof data.host === "string" && data.host ? <div style={{ fontSize: 12, marginBottom: 6 }}>Host: <span className="mono">{data.host}</span></div> : null}
      {typeof data.scope === "string" && data.scope ? (
        <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 8 }}>Âmbito: {data.scope}</div>
      ) : null}
      {parsed.length === 0 ? <p style={{ color: "var(--muted)", fontSize: 13 }}>Sem linhas nesta página (job ainda a correr ou sem resultados).</p> : null}
      {parsed.length > 0 ? (
        <div className="table-wrap" style={{ maxHeight: 380 }}>
          <table>
            <thead>
              <tr>
                <th>OID</th>
                <th>Tipo</th>
                <th>Valor</th>
              </tr>
            </thead>
            <tbody>
              {parsed.map((r, i) => (
                <tr key={`${r.oid}-${i}`}>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {r.oid}
                  </td>
                  <td style={{ fontSize: 11 }}>{r.type}</td>
                  <td className="mono" style={{ fontSize: 11, maxWidth: 480, wordBreak: "break-all" }}>
                    {r.value}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <CollapsibleRawJson data={data} />
    </div>
  );
}

export function SnmpWalkDiscoveriesOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) {
    return <pre className="mono" style={{ ...preBox, marginTop: 12 }}>{JSON.stringify(data, null, 2)}</pre>;
  }
  const candidates = Array.isArray(data.candidates) ? data.candidates : [];
  return (
    <div style={panelStyle}>
      <div className="row" style={{ gap: 8, marginBottom: 10, flexWrap: "wrap" }}>
        {typeof data.status === "string" ? <span className="badge">{data.status}</span> : null}
        {data.job_id != null ? <span className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>job {str(data.job_id)}</span> : null}
      </div>
      {candidates.length === 0 ? <p style={{ color: "var(--muted)", fontSize: 13 }}>Sem amostras agrupadas ainda.</p> : null}
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        {candidates.map((c, idx) => {
          if (!isRecord(c)) return null;
          const kind = str(c.kind);
          const root = str(c.root_oid);
          const sample = Array.isArray(c.sample) ? c.sample : [];
          return (
            <div key={`${kind}-${idx}`} style={{ border: "1px solid var(--border)", borderRadius: 6, padding: 10, background: "var(--panel2)" }}>
              <div style={{ fontWeight: 600, fontSize: 13 }}>{kind || "Grupo"}</div>
              {root ? <div className="mono" style={{ fontSize: 10, color: "var(--muted)", marginTop: 4 }}>{root}</div> : null}
              {sample.length > 0 ? (
                <div className="table-wrap" style={{ marginTop: 8, maxHeight: 200 }}>
                  <table>
                    <thead>
                      <tr>
                        <th>OID</th>
                        <th>Tipo</th>
                        <th>Valor</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sample.map((row, i) => {
                        if (!isRecord(row)) return null;
                        return (
                          <tr key={i}>
                            <td className="mono" style={{ fontSize: 10 }}>
                              {str(row.oid)}
                            </td>
                            <td style={{ fontSize: 10 }}>{str(row.type)}</td>
                            <td className="mono" style={{ fontSize: 10, maxWidth: 320, wordBreak: "break-all" }}>
                              {str(row.value)}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
      <CollapsibleRawJson data={data} />
    </div>
  );
}

export function NetworkToolTextOutput({ data }: { data: unknown }) {
  if (!isRecord(data)) return null;
  const cmd = str(data.command);
  const output = str(data.output);
  const err = str(data.error);
  const ok = data.ok === true;
  const hops = Array.isArray(data.hops) ? data.hops : null;
  return (
    <div style={panelStyle}>
      <div className="row" style={{ justifyContent: "space-between", flexWrap: "wrap", gap: 8 }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>Resultado</span>
        {ok ? <span className="badge badge--ok">Concluído</span> : <span className="badge badge--off">Com avisos</span>}
      </div>
      {cmd ? (
        <p style={{ margin: "8px 0 0", fontSize: 11, color: "var(--muted)" }}>
          Comando: <span className="mono">{cmd}</span>
        </p>
      ) : null}
      {err ? (
        <div className="msg msg--err" style={{ marginTop: 8, fontSize: 12 }}>
          {err}
        </div>
      ) : null}
      {hops && hops.length > 0 ? (
        <div className="table-wrap" style={{ marginTop: 10 }}>
          <table>
            <thead>
              <tr>
                <th>Salto</th>
                <th>Detalhe</th>
              </tr>
            </thead>
            <tbody>
              {hops.map((h, i) => {
                if (!isRecord(h)) return null;
                return (
                  <tr key={i}>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {str(h.hop) || "—"}
                    </td>
                    <td className="mono" style={{ fontSize: 10, wordBreak: "break-all" }}>
                      {str(h.detail) || str(h.raw)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      {output ? <pre className="mono" style={preBox}>{output}</pre> : !err ? <p style={{ marginTop: 8, color: "var(--muted)", fontSize: 12 }}>Sem saída.</p> : null}
      <CollapsibleRawJson data={data} />
    </div>
  );
}
