/**
 * Modal de análise e limpeza de dados históricos na base PostgreSQL.
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Database, Loader2, Trash2 } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { formatBytes } from "../../lib/formatBytes";
import { toastErr, toastOk } from "../../lib/operationToast";
import { useAppToast } from "../../lib/appToast";

type DbCleanupOverviewItem = {
  table: string;
  label: string;
  date_column: string;
  category: string;
  exists?: boolean;
  total_count?: number;
  size_bytes?: number;
  oldest_at?: string;
  newest_at?: string;
  error?: string;
};

type DbCleanupOverviewResponse = {
  database_size_bytes: number;
  total_rows: number;
  items: DbCleanupOverviewItem[];
  scanned_at: string;
};

type DbCleanupScanItem = {
  table: string;
  label: string;
  date_column: string;
  category: string;
  exists?: boolean;
  count: number;
  oldest_eligible_at?: string;
};

type DbCleanupScanResponse = {
  older_than_days: number;
  cutoff_at: string;
  items: DbCleanupScanItem[];
  total_rows: number;
};

const DAY_PRESETS = [7, 30, 60, 90, 180, 365] as const;
const SCAN_MIN_DAYS = 1;
const EXECUTE_MIN_DAYS = 7;

const CATEGORY_LABELS: Record<string, string> = {
  monitoramento: "Monitoramento",
  bng: "BNG",
  olt: "OLT",
  sistema: "Sistema",
  alertas: "Alertas",
};

function formatDt(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("pt-PT", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

function daysBetweenOldest(oldest?: string): number | null {
  if (!oldest) return null;
  const d = new Date(oldest);
  if (Number.isNaN(d.getTime())) return null;
  return Math.floor((Date.now() - d.getTime()) / (24 * 60 * 60 * 1000));
}

export function DatabaseCleanupModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [olderThanDays, setOlderThanDays] = useState("30");
  const [selectedTables, setSelectedTables] = useState<Set<string>>(new Set());
  const [scanResult, setScanResult] = useState<DbCleanupScanResponse | null>(null);
  const [confirmStep, setConfirmStep] = useState<0 | 1 | 2>(0);
  const [modalTab, setModalTab] = useState<"overview" | "purge">("overview");

  const overview = useQuery({
    queryKey: ["db-cleanup-overview"],
    queryFn: () => apiFetch<DbCleanupOverviewResponse>("/api/v1/settings/database/cleanup/overview"),
    enabled: open,
    staleTime: 0,
  });

  useEffect(() => {
    if (!open) return;
    setScanResult(null);
    setConfirmStep(0);
    setModalTab("overview");
  }, [open]);

  useEffect(() => {
    if (!overview.data?.items.length) return;
    setSelectedTables(new Set(overview.data.items.filter((it) => it.exists !== false).map((it) => it.table)));
  }, [overview.data]);

  const oldestGlobal = useMemo(() => {
    let min: Date | null = null;
    for (const it of overview.data?.items ?? []) {
      if (!it.oldest_at) continue;
      const d = new Date(it.oldest_at);
      if (!Number.isNaN(d.getTime()) && (!min || d < min)) min = d;
    }
    return min;
  }, [overview.data]);

  const scan = useMutation({
    mutationFn: () => {
      const d = Math.max(SCAN_MIN_DAYS, parseInt(olderThanDays, 10) || 30);
      return apiFetch<DbCleanupScanResponse>("/api/v1/settings/database/cleanup/scan", {
        method: "POST",
        json: {
          older_than_days: d,
          tables: Array.from(selectedTables),
        },
      });
    },
    onSuccess: (data) => {
      setScanResult(data);
      setConfirmStep(0);
      setModalTab("purge");
      toastOk(pushToast, `Análise: ${data.total_rows.toLocaleString("pt-PT")} registo(s) elegíveis.`);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao analisar dados antigos."),
  });

  const execute = useMutation({
    mutationFn: () => {
      const d = scanResult?.older_than_days ?? Math.max(EXECUTE_MIN_DAYS, parseInt(olderThanDays, 10) || 30);
      return apiFetch<{ ok?: boolean; deleted_total?: number; message?: string }>("/api/v1/settings/database/cleanup/execute", {
        method: "POST",
        json: {
          older_than_days: d,
          tables: Array.from(selectedTables),
          confirm: true,
        },
      });
    },
    onSuccess: (data) => {
      setConfirmStep(0);
      const n = data.deleted_total ?? 0;
      toastOk(pushToast, data.message ?? `${n.toLocaleString("pt-PT")} registo(s) apagado(s).`);
      qc.invalidateQueries({ queryKey: ["db-cleanup-overview"] });
      scan.mutate();
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao apagar dados antigos."),
  });

  const toggleTable = (table: string) => {
    setSelectedTables((prev) => {
      const next = new Set(prev);
      if (next.has(table)) next.delete(table);
      else next.add(table);
      return next;
    });
  };

  const toggleAll = (checked: boolean) => {
    if (!overview.data) return;
    if (checked) {
      setSelectedTables(new Set(overview.data.items.filter((it) => it.exists !== false).map((it) => it.table)));
    } else {
      setSelectedTables(new Set());
    }
  };

  if (!open) return null;

  const parsedDays = parseInt(olderThanDays, 10) || 30;
  const canExecute = parsedDays >= EXECUTE_MIN_DAYS && selectedTables.size > 0;

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !execute.isPending && onClose()}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 920, width: "min(96vw, 920px)", maxHeight: "92vh", overflow: "auto", position: "relative" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 12, marginBottom: 12 }}>
          <div>
            <h2 style={{ margin: "0 0 4px", fontSize: 18, display: "flex", alignItems: "center", gap: 8 }}>
              <Database size={20} aria-hidden />
              Análise e limpeza de dados
            </h2>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: 0, lineHeight: 1.45 }}>
              Telemetria, históricos, snapshots BNG/OLT, eventos, jobs e alertas encerrados. Configurações e auditoria de operações não são
              apagados.
            </p>
          </div>
          <button type="button" className="btn" onClick={onClose} disabled={execute.isPending} aria-label="Fechar">
            Fechar
          </button>
        </div>

        {overview.isLoading && (
          <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "24px 0", color: "var(--muted)" }}>
            <Loader2 size={18} className="map-refresh-spin" aria-hidden />
            A carregar estatísticas da base de dados…
          </div>
        )}

        {overview.isError && (
          <div className="msg msg--err" style={{ marginBottom: 12 }}>
            {(overview.error as Error).message}
          </div>
        )}

        {overview.data && (
          <>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))",
                gap: 10,
                marginBottom: 16,
              }}
            >
              <div className="card" style={{ padding: 10, margin: 0 }}>
                <div style={{ fontSize: 11, color: "var(--muted)" }}>Tamanho da base</div>
                <div style={{ fontSize: 16, fontWeight: 600 }}>{formatBytes(overview.data.database_size_bytes)}</div>
              </div>
              <div className="card" style={{ padding: 10, margin: 0 }}>
                <div style={{ fontSize: 11, color: "var(--muted)" }}>Registos históricos</div>
                <div style={{ fontSize: 16, fontWeight: 600 }}>{overview.data.total_rows.toLocaleString("pt-PT")}</div>
              </div>
              <div className="card" style={{ padding: 10, margin: 0 }}>
                <div style={{ fontSize: 11, color: "var(--muted)" }}>Dado mais antigo</div>
                <div style={{ fontSize: 13, fontWeight: 600 }}>{oldestGlobal ? formatDt(oldestGlobal.toISOString()) : "—"}</div>
                {oldestGlobal && (
                  <div style={{ fontSize: 10, color: "var(--muted)" }}>
                    há ~{Math.floor((Date.now() - oldestGlobal.getTime()) / (24 * 60 * 60 * 1000))} dias
                  </div>
                )}
              </div>
            </div>

            <div className="row" style={{ gap: 6, marginBottom: 12, flexWrap: "wrap" }}>
              <button type="button" className={`btn ${modalTab === "overview" ? "btn--primary" : ""}`} onClick={() => setModalTab("overview")}>
                Visão geral
              </button>
              <button type="button" className={`btn ${modalTab === "purge" ? "btn--primary" : ""}`} onClick={() => setModalTab("purge")}>
                Análise e eliminação
              </button>
            </div>

            {modalTab === "overview" && (
              <div className="table-wrap" style={{ maxHeight: 360, overflow: "auto", marginBottom: 12 }}>
                <table style={{ fontSize: 12 }}>
                  <thead>
                    <tr>
                      <th>Tabela</th>
                      <th>Categoria</th>
                      <th style={{ textAlign: "right" }}>Total</th>
                      <th>Mais antigo</th>
                      <th>Mais recente</th>
                      <th style={{ textAlign: "right" }}>Tamanho</th>
                    </tr>
                  </thead>
                  <tbody>
                    {overview.data.items.map((it) => {
                      const age = daysBetweenOldest(it.oldest_at);
                      return (
                        <tr key={it.table} style={it.exists === false ? { opacity: 0.5 } : undefined}>
                          <td>
                            {it.label}
                            <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                              {it.table}
                            </div>
                            {it.error && <div style={{ fontSize: 10, color: "var(--danger, #dc2626)" }}>{it.error}</div>}
                          </td>
                          <td>{CATEGORY_LABELS[it.category] ?? it.category}</td>
                          <td style={{ textAlign: "right" }}>{(it.total_count ?? 0).toLocaleString("pt-PT")}</td>
                          <td>
                            {formatDt(it.oldest_at)}
                            {age != null && it.total_count ? (
                              <div style={{ fontSize: 10, color: "var(--muted)" }}>{age} dias</div>
                            ) : null}
                          </td>
                          <td>{formatDt(it.newest_at)}</td>
                          <td style={{ textAlign: "right" }}>{it.size_bytes != null ? formatBytes(it.size_bytes) : "—"}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}

            {modalTab === "purge" && (
              <>
                <div
                  style={{
                    padding: "10px 12px",
                    borderRadius: 8,
                    background: "rgba(59,130,246,0.08)",
                    border: "1px solid rgba(59,130,246,0.2)",
                    fontSize: 12,
                    marginBottom: 12,
                    lineHeight: 1.45,
                  }}
                >
                  {oldestGlobal ? (
                    <>
                      O registo mais antigo na base tem <strong>~{Math.floor((Date.now() - oldestGlobal.getTime()) / (24 * 60 * 60 * 1000))} dias</strong>.
                      Use um período <strong>menor ou igual</strong> a essa idade para encontrar dados elegíveis (ex.: se o mais antigo tem 25 dias, analise com 20 ou 30 dias).
                    </>
                  ) : (
                    <>Não há dados históricos nas tabelas monitorizadas, ou as tabelas estão vazias.</>
                  )}
                </div>

                <div className="row stack-mobile" style={{ gap: 8, alignItems: "flex-end", flexWrap: "wrap", marginBottom: 12 }}>
                  <div className="field" style={{ margin: 0, minWidth: 140 }}>
                    <label htmlFor="db-cleanup-days-modal">Mais antigos que (dias)</label>
                    <input
                      id="db-cleanup-days-modal"
                      className="input"
                      type="number"
                      min={SCAN_MIN_DAYS}
                      value={olderThanDays}
                      onChange={(e) => setOlderThanDays(e.target.value)}
                      style={{ maxWidth: 120 }}
                    />
                  </div>
                  <div className="row" style={{ gap: 4, flexWrap: "wrap" }}>
                    {DAY_PRESETS.map((d) => (
                      <button key={d} type="button" className="btn" style={{ padding: "4px 10px", fontSize: 12 }} onClick={() => setOlderThanDays(String(d))}>
                        {d}d
                      </button>
                    ))}
                  </div>
                  <button type="button" className="btn btn--primary" disabled={scan.isPending || selectedTables.size === 0} onClick={() => scan.mutate()}>
                    {scan.isPending ? "A analisar…" : "Analisar seleccionados"}
                  </button>
                </div>

                <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
                  Análise: mínimo {SCAN_MIN_DAYS} dia · Eliminação: mínimo {EXECUTE_MIN_DAYS} dias · {selectedTables.size} tabela(s) seleccionada(s)
                </p>

                <div className="table-wrap" style={{ maxHeight: 220, overflow: "auto", marginBottom: 12 }}>
                  <table style={{ fontSize: 12 }}>
                    <thead>
                      <tr>
                        <th style={{ width: 36 }}>
                          <input
                            type="checkbox"
                            checked={selectedTables.size === overview.data.items.filter((it) => it.exists !== false).length}
                            onChange={(e) => toggleAll(e.target.checked)}
                            aria-label="Seleccionar todas"
                          />
                        </th>
                        <th>Tabela</th>
                        <th style={{ textAlign: "right" }}>Total actual</th>
                        <th>Mais antigo</th>
                        <th style={{ textAlign: "right" }}>Elegíveis</th>
                      </tr>
                    </thead>
                    <tbody>
                      {overview.data.items.map((it) => {
                        const scanItem = scanResult?.items.find((s) => s.table === it.table);
                        return (
                          <tr key={it.table}>
                            <td>
                              <input
                                type="checkbox"
                                checked={selectedTables.has(it.table)}
                                disabled={it.exists === false}
                                onChange={() => toggleTable(it.table)}
                                aria-label={`Seleccionar ${it.label}`}
                              />
                            </td>
                            <td>
                              {it.label}
                              <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                                {it.table}
                              </div>
                            </td>
                            <td style={{ textAlign: "right" }}>{(it.total_count ?? 0).toLocaleString("pt-PT")}</td>
                            <td>{formatDt(it.oldest_at)}</td>
                            <td style={{ textAlign: "right", fontWeight: scanItem && scanItem.count > 0 ? 600 : undefined }}>
                              {scanItem ? scanItem.count.toLocaleString("pt-PT") : "—"}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>

                {scanResult && (
                  <div style={{ marginBottom: 12 }}>
                    <p style={{ fontSize: 13, margin: "0 0 8px" }}>
                      <strong>{scanResult.total_rows.toLocaleString("pt-PT")}</strong> registo(s) anteriores a{" "}
                      <span className="mono">{formatDt(scanResult.cutoff_at)}</span> ({scanResult.older_than_days} dias).
                    </p>
                    {scanResult.total_rows > 0 ? (
                      <button
                        type="button"
                        className="btn btn--danger"
                        disabled={execute.isPending || !canExecute}
                        onClick={() => setConfirmStep(1)}
                        title={!canExecute ? `Mínimo ${EXECUTE_MIN_DAYS} dias para eliminar` : undefined}
                      >
                        <Trash2 size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                        Apagar {scanResult.total_rows.toLocaleString("pt-PT")} registo(s)
                      </button>
                    ) : (
                      <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>
                        Nenhum registo elegível com este período. Tente um valor menor (ex.: 7 ou 14 dias) ou consulte a coluna «Mais antigo».
                      </p>
                    )}
                    {!canExecute && parsedDays < EXECUTE_MIN_DAYS && scanResult.total_rows > 0 && (
                      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0" }}>
                        Para eliminar, o período deve ser pelo menos {EXECUTE_MIN_DAYS} dias.
                      </p>
                    )}
                  </div>
                )}
              </>
            )}
          </>
        )}

        {confirmStep > 0 && scanResult && (
          <div
            style={{
              position: "absolute",
              inset: 0,
              background: "rgba(0,0,0,0.55)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              padding: 16,
              borderRadius: "inherit",
            }}
            onMouseDown={() => !execute.isPending && setConfirmStep(0)}
          >
            <div className="card" style={{ maxWidth: 440, width: "100%", margin: 0 }} onMouseDown={(e) => e.stopPropagation()}>
              <h3 style={{ margin: "0 0 8px", fontSize: 16 }}>{confirmStep === 1 ? "Confirmar limpeza" : "Confirmação final"}</h3>
              {confirmStep === 1 ? (
                <>
                  <p style={{ fontSize: 13, lineHeight: 1.5, margin: "0 0 12px" }}>
                    Serão apagados permanentemente <strong>{scanResult.total_rows.toLocaleString("pt-PT")}</strong> registo(s) com mais de{" "}
                    {scanResult.older_than_days} dias em <strong>{selectedTables.size}</strong> tabela(s). Esta acção não pode ser desfeita.
                  </p>
                  <div className="row" style={{ gap: 8, justifyContent: "flex-end", flexWrap: "wrap" }}>
                    <button type="button" className="btn" disabled={execute.isPending} onClick={() => setConfirmStep(0)}>
                      Cancelar
                    </button>
                    <button type="button" className="btn btn--danger" onClick={() => setConfirmStep(2)}>
                      Sim, quero apagar
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <p style={{ fontSize: 13, lineHeight: 1.5, margin: "0 0 12px" }}>
                    Tem <strong>absoluta certeza</strong>? {scanResult.total_rows.toLocaleString("pt-PT")} linhas serão removidas. A operação fica
                    registada na auditoria.
                  </p>
                  <div className="row" style={{ gap: 8, justifyContent: "flex-end", flexWrap: "wrap" }}>
                    <button type="button" className="btn" disabled={execute.isPending} onClick={() => setConfirmStep(1)}>
                      Voltar
                    </button>
                    <button type="button" className="btn btn--danger" disabled={execute.isPending} onClick={() => execute.mutate()}>
                      {execute.isPending ? "A apagar…" : "Apagar definitivamente"}
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export function DatabaseCleanupButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <div style={{ marginTop: 28, paddingTop: 20, borderTop: "1px solid var(--border)" }}>
        <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>Manutenção de dados históricos</h3>
        <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.45 }}>
          Analise o volume e a antiguidade dos dados na base PostgreSQL e elimine registos antigos com confirmação em duas etapas. A auditoria
          regista quem executou cada operação.
        </p>
        <button type="button" className="btn btn--primary" onClick={() => setOpen(true)}>
          <Database size={16} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
          Abrir análise e limpeza…
        </button>
      </div>
      <DatabaseCleanupModal open={open} onClose={() => setOpen(false)} />
    </>
  );
}
