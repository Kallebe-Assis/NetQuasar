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
import { formatBngDateTime } from "../lib/bngDisplay";
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

function bngChartLabel(iso: string) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function BngDeviceChart({ device, points, title }: { device?: string; points: NonNullable<SystemReportPayload["chart"]>["points"]; title?: string }) {
  const data = useMemo(
    () =>
      (points ?? []).map((p) => {
        const iso = String(p.collected_at ?? p.t ?? "");
        return {
          iso,
          label: bngChartLabel(iso),
          device: p.device,
          Total: p.total ?? null,
          PPPoE: p.pppoe ?? null,
          IPv4: p.ipv4 ?? null,
          IPv6: p.ipv6 ?? null,
          "Dual-stack": p.dual_stack ?? null,
        };
      }),
    [points],
  );

  const yDomain = useMemo(() => {
    const numeric: number[] = [];
    for (const row of data) {
      for (const v of [row.Total, row.PPPoE, row.IPv4, row.IPv6, row["Dual-stack"]]) {
        if (v != null && Number.isFinite(v)) numeric.push(v);
      }
    }
    if (numeric.length === 0) return [0, 1] as [number, number];
    const min = Math.min(...numeric);
    const max = Math.max(...numeric);
    const span = Math.max(1, max - min);
    const pad = Math.max(1, Math.round(span * 0.08));
    return [min - pad, max + pad] as [number, number];
  }, [data]);

  if (data.length === 0) return null;

  return (
    <div className="card" style={{ padding: 12, marginTop: 12 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>
        {title ?? "Gráfico BNG"}
        {device ? <span style={{ fontWeight: 400, color: "var(--muted)" }}> — {device}</span> : null}
      </h3>
      <ResponsiveContainer width="100%" height={260}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 9 }} interval="preserveStartEnd" minTickGap={28} />
          <YAxis tick={{ fontSize: 10 }} width={48} allowDecimals={false} domain={yDomain} />
          <Tooltip
            labelFormatter={(_, items) => {
              const iso = items?.[0]?.payload?.iso;
              return iso ? new Date(String(iso)).toLocaleString("pt-BR") : "";
            }}
          />
          <Legend />
          <Line type="monotone" dataKey="Total" stroke="#64748b" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="PPPoE" stroke="#3b82f6" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="IPv4" stroke="#22c55e" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="IPv6" stroke="#a855f7" strokeWidth={2} dot={false} connectNulls={false} />
          <Line type="monotone" dataKey="Dual-stack" stroke="#f59e0b" strokeWidth={2} dot={false} connectNulls={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function BngReportAverages({ payload }: { payload: SystemReportPayload }) {
  const windows = payload.averages?.windows ?? [];
  if (windows.length === 0) return null;
  return (
    <section style={{ marginBottom: 16 }}>
      <h3 style={{ fontSize: 14, margin: "0 0 8px" }}>Médias de logins (BNG)</h3>
      <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>
        Média aritmética das coletas SNMP por janela — incluída no envio Telegram quando há dados suficientes.
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Período</th>
              <th>Amostras</th>
              <th>Total</th>
              <th>PPPoE</th>
              <th>IPv4</th>
              <th>IPv6</th>
              <th>Dual-stack</th>
            </tr>
          </thead>
          <tbody>
            {windows.map((w) => (
              <tr key={w.days}>
                <td>{w.label}</td>
                <td>{w.samples.toLocaleString("pt-PT")}</td>
                <td>{w.total != null ? w.total.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.pppoe != null ? w.pppoe.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.ipv4 != null ? w.ipv4.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.ipv6 != null ? w.ipv6.toLocaleString("pt-PT") : "—"}</td>
                <td>{w.dual_stack != null ? w.dual_stack.toLocaleString("pt-PT") : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function BngReportChart({ payload }: { payload: SystemReportPayload }) {
  const pts = payload.chart?.points ?? [];
  const byDevice = useMemo(() => {
    const map = new Map<string, typeof pts>();
    for (const p of pts) {
      const key = p.device?.trim() || "BNG";
      const list = map.get(key) ?? [];
      list.push(p);
      map.set(key, list);
    }
    return map;
  }, [pts]);

  if (pts.length === 0) return null;

  if (byDevice.size <= 1) {
    const only = [...byDevice.values()][0] ?? pts;
    return <BngDeviceChart points={only} title={payload.chart?.label} />;
  }

  return (
    <>
      {[...byDevice.entries()].map(([device, devicePts]) => (
        <BngDeviceChart key={device} device={device} points={devicePts} title={payload.chart?.label} />
      ))}
    </>
  );
}

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
      {payload.report_id === "bng-subscribers" && <BngReportAverages payload={payload} />}
      {payload.report_id === "bng-subscribers" && <BngReportChart payload={payload} />}

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
                    {row.map((cell, j) => {
                      const col = cols[j] ?? "";
                      const display =
                        col === "Última coleta" && cell ? formatBngDateTime(cell) : cell || "—";
                      return (
                        <td key={j} style={{ maxWidth: 280, overflow: "hidden", textOverflow: "ellipsis" }} title={display}>
                          {display}
                        </td>
                      );
                    })}
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
