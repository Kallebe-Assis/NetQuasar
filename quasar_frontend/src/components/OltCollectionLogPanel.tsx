type StepRow = {
  id?: string;
  method?: string;
  status?: string;
  elapsed_ms?: number;
  error?: string;
  detail?: Record<string, unknown>;
};

type CollectionLog = {
  scope?: string;
  elapsed_ms?: number;
  mode?: string;
  profile_error?: string;
  profile_exec?: string;
  refresh_timeout?: boolean;
  refresh_cancelled?: string;
  steps?: StepRow[];
  vsol_steps?: unknown;
  vsol?: Record<string, unknown>;
  zte?: Record<string, unknown>;
  has_snmp_debug?: boolean;
};

function fmtMs(ms: unknown): string {
  const n = Number(ms);
  if (!Number.isFinite(n)) return "—";
  if (n < 1000) return `${n} ms`;
  return `${(n / 1000).toFixed(1)} s`;
}

function fmtVal(v: unknown): string {
  if (v == null || v === "") return "—";
  if (typeof v === "boolean") return v ? "sim" : "não";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

const METHOD_LABELS: Record<string, string> = {
  if_mib_refresh: "Actualizar interfaces (SNMP walk completo)",
  if_mib_snapshot: "Ler interfaces (rápido ou walk)",
  onu_snmp_walk: "Contagem simples via snmpwalk (legado)",
  onu_metrics_collect: "Coleta SNMP por métricas (serial, estado, RX…)",
  vsol_onu_collect: "VSOL gOnuAuthList (legado)",
  snmp_walk: "SNMP walk",
  snmp_get: "SNMP get",
  telnet: "Telnet CLI",
  datacom_build_pons: "Datacom agregar PONs",
  if_mib_merge_pons: "IF-MIB fundir PONs",
  stabilize_pons: "Estabilizar PONs",
};

type Props = {
  log: CollectionLog | null | undefined;
  loading?: boolean;
  lastError?: string | null;
};

export function OltCollectionLogPanel({ log, loading, lastError }: Props) {
  const steps = Array.isArray(log?.steps) ? log!.steps! : [];
  const vsol = log?.vsol ?? {};

  return (
    <div
      className="card"
      style={{
        marginTop: 12,
        padding: 12,
        border: "1px solid var(--border)",
        background: "var(--surface-2, rgba(0,0,0,0.03))",
      }}
    >
      <h2 style={{ margin: 0, fontSize: 15 }}>Log da coleta</h2>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0" }}>
        Passos executados no último refresh. Use para validar se as ONUs foram lidas (refs, snmpwalk, erros).
      </p>

      {loading && (
        <p className="msg msg--off" style={{ marginTop: 10, fontSize: 12 }}>
          A coletar… isto pode demorar 1–3 minutos (VSOL: snmpwalk na tabela de ONUs).
        </p>
      )}

      {lastError && (
        <div className="msg msg--err" style={{ marginTop: 10, fontSize: 12 }}>
          {lastError}
        </div>
      )}

      {!loading && !log && !lastError && (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 10 }}>
          Ainda sem log. Clique em «Atualizar ONUs» para executar a coleta.
        </p>
      )}

      {log && (
        <>
          <div className="row" style={{ flexWrap: "wrap", gap: 12, marginTop: 10, fontSize: 12 }}>
            <span>
              <strong>Âmbito:</strong> {log.scope === "onu" ? "rápido (ONUs)" : log.scope ?? "completo"}
            </span>
            <span>
              <strong>Duração total:</strong> {fmtMs(log.elapsed_ms)}
            </span>
            {log.has_snmp_debug ? (
              <span className="badge badge--ok">Debug SNMP gravado</span>
            ) : null}
          </div>

          {(log.profile_error || log.profile_exec || log.refresh_timeout) && (
            <div className="msg msg--err" style={{ marginTop: 8, fontSize: 12 }}>
              {log.refresh_timeout
                ? "Coleta interrompida por tempo limite — dados parciais podem ter sido gravados. Actualize de novo ou use «Só interfaces» antes de «Atualizar ONUs»."
                : null}
              {[log.profile_error, log.profile_exec, log.refresh_cancelled].filter(Boolean).join(" · ")}
            </div>
          )}

          <div className="grid-cards" style={{ marginTop: 10, gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))" }}>
            <div className="stat">
              <div className="stat__k">Refs IF-MIB</div>
              <div className="stat__v">{fmtVal(vsol.refs)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">ONUs parseadas</div>
              <div className="stat__v">{fmtVal(vsol.onus_parsed)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">Vars SNMP</div>
              <div className="stat__v">{fmtVal(vsol.snmp_vars)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">Online 4.1.8 OK</div>
              <div className="stat__v">{fmtVal(vsol.online_complete)}</div>
            </div>
          </div>

          {vsol.note ? (
            <p className="msg msg--off" style={{ fontSize: 11, marginTop: 8 }}>
              Nota VSOL: {fmtVal(vsol.note)}
            </p>
          ) : null}

          {steps.length > 0 ? (
            <div className="table-wrap" style={{ marginTop: 10, maxHeight: 220, overflow: "auto" }}>
              <table className="table table--compact" style={{ fontSize: 11, width: "100%" }}>
                <thead>
                  <tr>
                    <th>Passo</th>
                    <th>Método</th>
                    <th>Estado</th>
                    <th className="mono">Tempo</th>
                    <th>Detalhe</th>
                  </tr>
                </thead>
                <tbody>
                  {steps.map((st, i) => (
                    <tr key={`${st.id}-${i}`}>
                      <td className="mono">{st.id ?? `#${i + 1}`}</td>
                      <td>{METHOD_LABELS[st.method ?? ""] ?? st.method ?? "—"}</td>
                      <td>
                        <span className={st.status === "ok" ? "badge badge--ok" : "badge badge--err"}>
                          {st.status ?? "—"}
                        </span>
                        {st.error ? (
                          <div style={{ fontSize: 10, color: "var(--danger)", marginTop: 2 }}>{st.error}</div>
                        ) : null}
                      </td>
                      <td className="mono">{fmtMs(st.elapsed_ms)}</td>
                      <td style={{ fontSize: 10, maxWidth: 280, wordBreak: "break-word" }}>
                        {st.detail ? fmtVal(st.detail) : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 8 }}>
              Sem passos registados no snapshot (perfil vazio ou refresh antigo).
            </p>
          )}

          {log.vsol_steps ? (
            <details style={{ marginTop: 8 }}>
              <summary style={{ fontSize: 11, cursor: "pointer" }}>Passos SNMP internos (vsol_collect_steps)</summary>
              <pre
                className="mono"
                style={{ fontSize: 10, overflow: "auto", maxHeight: 120, marginTop: 6, padding: 8, borderRadius: 6 }}
              >
                {JSON.stringify(log.vsol_steps, null, 2)}
              </pre>
            </details>
          ) : null}
        </>
      )}
    </div>
  );
}
