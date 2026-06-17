import { useMutation, useQuery } from "@tanstack/react-query";
import { createPortal } from "react-dom";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { FileDown, FileText, RefreshCw, Search, Send, X } from "lucide-react";
import { PageCountPill } from "../components/PageCountPill";
import { isAdminUser } from "../lib/auth";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import {
  downloadSystemReportCsv,
  fetchSystemReport,
  fetchSystemReportCatalog,
  sendSystemReportTelegram,
  summaryEntries,
  type SystemReportId,
  type SystemReportPayload,
} from "../lib/systemReports";

function ReportChart({ payload }: { payload: SystemReportPayload }) {
  const pts = payload.chart?.points ?? [];
  if (pts.length === 0) return null;
  const data = pts.map((p) => ({
    label: String(p.t ?? ""),
    Total: Number(p.total ?? 0),
    Online: Number(p.online ?? 0),
    Offline: Number(p.offline ?? 0),
  }));
  return (
    <div className="card" style={{ padding: 12, marginTop: 12 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>{payload.chart?.label ?? "Gráfico"}</h3>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 10 }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 10 }} width={44} allowDecimals={false} />
          <Tooltip />
          <Legend />
          <Line type="monotone" dataKey="Total" stroke="#58a6ff" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="Online" stroke="#3fb950" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="Offline" stroke="#f85149" strokeWidth={2} dot={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function ReportPreviewBody({ payload }: { payload: SystemReportPayload }) {
  const summary = summaryEntries(payload.summary);
  const cols = payload.columns ?? [];
  const rows = payload.rows ?? [];
  return (
    <div className="system-report-print">
      <header style={{ marginBottom: 16 }}>
        <h2 style={{ margin: "0 0 4px" }}>{payload.title}</h2>
        {payload.description && (
          <p style={{ margin: "0 0 8px", color: "var(--muted)", fontSize: 13 }}>{payload.description}</p>
        )}
        <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>
          Gerado: {new Date(payload.generated_at).toLocaleString("pt-PT")}
        </p>
      </header>

      {summary.length > 0 && (
        <section style={{ marginBottom: 16 }}>
          <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>Resumo</h3>
          <div className="table-wrap">
            <table>
              <tbody>
                {summary.map(([k, v]) => (
                  <tr key={k}>
                    <th style={{ textAlign: "left", width: "40%" }}>{k}</th>
                    <td>{v}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {payload.report_id === "olt-overview" && <ReportChart payload={payload} />}

      {rows.length > 0 && (
        <section>
          <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>
            Detalhes <span style={{ color: "var(--muted)", fontWeight: 400 }}>({rows.length} linhas)</span>
          </h3>
          <div className="table-wrap" style={{ maxHeight: 420, overflow: "auto" }}>
            <table>
              <thead>
                <tr>
                  {cols.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.slice(0, 500).map((row, i) => (
                  <tr key={i}>
                    {row.map((cell, j) => (
                      <td key={j} style={{ maxWidth: 280, overflow: "hidden", textOverflow: "ellipsis" }} title={cell}>
                        {cell || "—"}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {rows.length > 500 && (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>Pré-visualização limitada a 500 linhas. Exporte CSV para o conjunto completo.</p>
          )}
        </section>
      )}
    </div>
  );
}

function ReportCard({
  id,
  title,
  description,
  onOpen,
  onPrint,
}: {
  id: SystemReportId;
  title: string;
  description: string;
  onOpen: (id: SystemReportId) => void;
  onPrint: (id: SystemReportId) => void;
}) {
  const admin = isAdminUser();
  const { push: pushToast } = useAppToast();

  const csvMut = useMutation({
    mutationFn: () => downloadSystemReportCsv(id),
    onSuccess: () => pushToast({ tone: "ok", text: "CSV descarregado." }),
    onError: (e) => toastErr(pushToast, e, "Falha ao exportar CSV"),
  });

  const tgMut = useMutation({
    mutationFn: () => sendSystemReportTelegram(id),
    onSuccess: () => toastOk(pushToast, "Relatório enviado ao Telegram."),
    onError: (e) => toastErr(pushToast, e, "Falha ao enviar Telegram"),
  });

  return (
    <div className="card" style={{ padding: 16, display: "flex", flexDirection: "column", gap: 10 }}>
      <div>
        <h3 style={{ margin: "0 0 6px", fontSize: 15 }}>{title}</h3>
        <p style={{ margin: 0, fontSize: 13, color: "var(--muted)", lineHeight: 1.45 }}>{description}</p>
      </div>
      <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: "auto" }}>
        <button type="button" className="btn btn--primary" onClick={() => onOpen(id)}>
          Ver relatório
        </button>
        <button type="button" className="btn" disabled={csvMut.isPending} onClick={() => csvMut.mutate()}>
          <FileDown size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
          CSV
        </button>
        <button type="button" className="btn" onClick={() => onPrint(id)}>
          <FileText size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
          PDF
        </button>
        {admin && (
          <button type="button" className="btn" disabled={tgMut.isPending} onClick={() => tgMut.mutate()}>
            <Send size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
            Telegram
          </button>
        )}
      </div>
    </div>
  );
}

export function ReportsPage() {
  const [previewId, setPreviewId] = useState<SystemReportId | null>(null);
  const [printOnLoad, setPrintOnLoad] = useState(false);
  const [search, setSearch] = useState("");
  const printRef = useRef<HTMLDivElement>(null);
  const admin = isAdminUser();
  const { push: pushToast } = useAppToast();

  const catalog = useQuery({
    queryKey: ["system-reports-catalog"],
    queryFn: fetchSystemReportCatalog,
  });

  const preview = useQuery({
    queryKey: ["system-report", previewId],
    queryFn: () => fetchSystemReport(previewId!),
    enabled: previewId != null,
  });

  const openReport = useCallback((id: SystemReportId, print = false) => {
    setPrintOnLoad(print);
    setPreviewId(id);
  }, []);

  const handlePrint = useCallback(() => {
    document.body.classList.add("print-system-report");
    window.print();
    window.setTimeout(() => document.body.classList.remove("print-system-report"), 800);
  }, []);

  const tgMut = useMutation({
    mutationFn: (id: SystemReportId) => sendSystemReportTelegram(id),
    onSuccess: () => toastOk(pushToast, "Relatório enviado ao Telegram."),
    onError: (e) => toastErr(pushToast, e, "Falha ao enviar Telegram"),
  });

  const csvMut = useMutation({
    mutationFn: (id: SystemReportId) => downloadSystemReportCsv(id),
    onSuccess: () => pushToast({ tone: "ok", text: "CSV descarregado." }),
    onError: (e) => toastErr(pushToast, e, "Falha ao exportar CSV"),
  });

  const items = useMemo(() => {
    const all = catalog.data?.reports ?? [];
    const q = search.trim().toLowerCase();
    if (!q) return all;
    return all.filter(
      (r) => r.title.toLowerCase().includes(q) || r.description.toLowerCase().includes(q) || r.id.toLowerCase().includes(q),
    );
  }, [catalog.data?.reports, search]);

  if (catalog.isLoading) return <p>Carregando relatórios…</p>;
  if (catalog.isError) return <div className="msg msg--err">{(catalog.error as Error).message}</div>;

  return (
    <>
      <div className="page-heading">
        <h1>Relatórios</h1>
        <PageCountPill label="Relatórios" count={items.length} />
      </div>
      <p style={{ color: "var(--muted)", marginTop: 0, maxWidth: 720 }}>
        Relatórios operacionais com exportação CSV, impressão em PDF e envio em texto simples ao canal Telegram de relatórios
        {admin ? "" : " (Telegram requer perfil administrador)"}.
      </p>
      <div className="row" style={{ marginBottom: 16, gap: 10, flexWrap: "wrap", alignItems: "center" }}>
        <label className="row" style={{ gap: 8, alignItems: "center", flex: "1 1 280px", minWidth: 220 }}>
          <Search size={16} style={{ color: "var(--muted)", flexShrink: 0 }} aria-hidden />
          <input
            className="input"
            type="search"
            placeholder="Pesquisar relatório…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{ width: "100%" }}
          />
        </label>
        <button type="button" className="btn" onClick={() => catalog.refetch()}>
          <RefreshCw size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
          Actualizar lista
        </button>
      </div>

      {items.length === 0 ? (
        <p style={{ color: "var(--muted)" }}>Nenhum relatório corresponde à pesquisa.</p>
      ) : (
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))",
          gap: 14,
        }}
      >
        {items.map((r) => (
          <ReportCard
            key={r.id}
            id={r.id as SystemReportId}
            title={r.title}
            description={r.description}
            onOpen={(id) => openReport(id, false)}
            onPrint={(id) => openReport(id, true)}
          />
        ))}
      </div>
      )}

      {previewId != null &&
        createPortal(
          <div className="modal-backdrop" role="presentation" onClick={() => setPreviewId(null)}>
            <div
              className="modal card system-report-modal"
              role="dialog"
              aria-modal="true"
              style={{ width: "min(960px, 96vw)", maxHeight: "92vh", overflow: "auto", padding: 20 }}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 12 }}>
                <h2 style={{ margin: 0, fontSize: 18 }}>Pré-visualização</h2>
                <button type="button" className="btn no-print" aria-label="Fechar" onClick={() => setPreviewId(null)}>
                  <X size={16} />
                </button>
              </div>

              {preview.isLoading && <p>A carregar dados…</p>}
              {preview.isError && <div className="msg msg--err">{(preview.error as Error).message}</div>}
              {preview.data && (
                <>
                  <div ref={printRef}>
                    <ReportPreviewBody payload={preview.data} />
                  </div>
                  <div className="row no-print" style={{ gap: 8, marginTop: 16, flexWrap: "wrap" }}>
                    <button type="button" className="btn btn--primary" onClick={handlePrint}>
                      <FileText size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                      PDF / Imprimir
                    </button>
                    <button
                      type="button"
                      className="btn"
                      disabled={csvMut.isPending}
                      onClick={() => csvMut.mutate(previewId)}
                    >
                      <FileDown size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                      CSV
                    </button>
                    {admin && (
                      <button
                        type="button"
                        className="btn"
                        disabled={tgMut.isPending}
                        onClick={() => tgMut.mutate(previewId)}
                      >
                        <Send size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                        Telegram
                      </button>
                    )}
                  </div>
                </>
              )}
            </div>
          </div>,
          document.body,
        )}

      {preview.data && printOnLoad && !preview.isLoading && (
        <AutoPrint onReady={handlePrint} onDone={() => setPrintOnLoad(false)} />
      )}
    </>
  );
}

function AutoPrint({ onReady, onDone }: { onReady: () => void; onDone: () => void }) {
  useEffect(() => {
    const t = window.setTimeout(() => {
      onReady();
      onDone();
    }, 500);
    return () => window.clearTimeout(t);
  }, [onReady, onDone]);
  return null;
}
