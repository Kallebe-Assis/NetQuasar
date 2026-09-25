import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Download, Mail, RefreshCw } from "lucide-react";
import type { HubsoftInvoiceListResponse, HubsoftInvoiceRow } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConsultaToast } from "./hubsoftConsulta";

import { ConsultaLoading } from "./ConsultaLoading";
function fmtCurrencyStr(v?: string): string {
  const n = Number(v);
  if (!v || !Number.isFinite(n)) return v || "—";
  return n.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

/** "2026-08-27" → "27/08/2026" — split de string em vez de Date() de propósito: a HubSoft devolve
 * data pura (sem hora) nestes campos, e Date() interpretaria como meia-noite UTC, podendo
 * mostrar o dia errado dependendo do fuso do navegador. */
function fmtDateStr(v?: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(v ?? "");
  if (!m) return v || "—";
  return `${m[3]}/${m[2]}/${m[1]}`;
}

const STATUS_LABEL: Record<string, string> = { paid: "Pago", overdue: "Vencido", pending: "Pendente" };
const STATUS_CLASS: Record<string, string> = { paid: "badge badge--ok", overdue: "badge badge--err", pending: "badge" };

const EXPORT_PER_PAGE = 100;
const EXPORT_MAX_PAGES = 50; // teto de 5000 faturas por exportação

function csvCell(v: string): string {
  return /[",\n;]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

function downloadInvoicesCsv(rows: HubsoftInvoiceRow[]) {
  const headers = ["Cliente", "Código", "Plano", "Cidade", "Telefone", "Vencimento", "Pagamento", "Valor", "Valor pago", "Status"];
  const lines = [headers.join(",")];
  for (const inv of rows) {
    const cols = [
      inv.client_name ?? "",
      inv.client_code ?? "",
      inv.plan ?? "",
      inv.city ?? "",
      inv.phone ?? "",
      fmtDateStr(inv.due_date) === "—" ? "" : fmtDateStr(inv.due_date),
      fmtDateStr(inv.payment_date) === "—" ? "" : fmtDateStr(inv.payment_date),
      inv.value ?? "",
      inv.value_paid ?? "",
      (inv.status && STATUS_LABEL[inv.status]) || "",
    ];
    lines.push(cols.map(csvCell).join(","));
  }
  const blob = new Blob([`﻿${lines.join("\r\n")}`], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `faturas-hubsoft-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

function todayISO(offsetDays = 0): string {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return d.toISOString().slice(0, 10);
}

/**
 * Lista paginada de faturas — ao contrário do resumo (HubsoftFinancialSummaryView, que soma uma
 * janela fixa de 6 meses), aqui o usuário filtra o período/estado e pagina pelos resultados; cada
 * página é 1 pedido HTTP direto à HubSoft (ver internal/integrationhubsoft.ListInvoices), não
 * uma varredura — continua rápido mesmo com muitas faturas no período.
 */
export function HubsoftInvoiceListPanel() {
  const [from, setFrom] = useState(() => todayISO(-30));
  const [to, setTo] = useState(() => todayISO());
  const [status, setStatus] = useState<"" | "aberto" | "quitado">("");
  const [busca, setBusca] = useState("");
  // Só o botão "Consultar" (ou Enter no campo Cliente) busca na HubSoft: os campos acima são rascunho.
  const [applied, setApplied] = useState<{ from: string; to: string; status: "" | "aberto" | "quitado"; busca: string } | null>(null);
  const [page, setPage] = useState(0);
  const perPage = 25;
  const qc = useQueryClient();
  const { notify, missing } = useConsultaToast();

  const buildParams = (a: { from: string; to: string; status: "" | "aberto" | "quitado"; busca: string }, pg: number) => {
    const p = new URLSearchParams();
    p.set("page", String(pg));
    p.set("per_page", String(perPage));
    if (a.from) p.set("data_inicio", a.from);
    if (a.to) p.set("data_fim", a.to);
    if (a.status === "aberto") p.set("apenas_em_aberto", "sim");
    if (a.status === "quitado") p.set("apenas_quitado", "sim");
    if (a.busca.trim()) p.set("busca", a.busca.trim());
    return p.toString();
  };
  const fetchList = (qs: string) => apiFetch<HubsoftInvoiceListResponse>(`/api/v1/integrations/hubsoft/hubsoft/financial/list?${qs}`);
  const params = useMemo(() => (applied ? buildParams(applied, page) : ""), [applied, page]); // eslint-disable-line react-hooks/exhaustive-deps

  const q = useQuery({
    queryKey: ["hubsoft-financial-list", params],
    placeholderData: keepPreviousData,
    queryFn: () => fetchList(params),
    enabled: applied !== null,
    staleTime: 5 * 60_000, // o clique em Consultar já buscou; evita repetir ao aplicar o filtro
  });

  async function consult() {
    if (!from || !to) {
      missing("informe o período (vencimento de / até).");
      return;
    }
    if (from > to) {
      missing("o período está invertido (a data inicial é maior que a final).");
      return;
    }
    const next = { from, to, status, busca };
    try {
      const r = await qc.fetchQuery({ queryKey: ["hubsoft-financial-list", buildParams(next, 0)], queryFn: () => fetchList(buildParams(next, 0)), staleTime: 0 });
      if (!notify(r, null)) return;
    } catch (e) {
      notify(null, e);
      return;
    }
    setPage(0);
    setApplied(next);
  }

  const d = q.data;
  const invoices = d?.invoices ?? [];

  function applyPreset(days: number) {
    setFrom(todayISO(-days));
    setTo(todayISO());
  }

  const { push: pushToast } = useAppToast();
  const [exportProgress, setExportProgress] = useState<string | null>(null);

  /** Exporta TODAS as páginas do período/filtros aplicados (não só a página visível). */
  async function exportCsv() {
    const base = new URLSearchParams(params);
    base.set("per_page", String(EXPORT_PER_PAGE));
    const all: HubsoftInvoiceRow[] = [];
    try {
      let totalPages = 1;
      for (let p = 0; p < Math.min(totalPages, EXPORT_MAX_PAGES); p++) {
        base.set("page", String(p));
        setExportProgress(`A exportar… página ${p + 1}${totalPages > 1 ? ` de ${Math.min(totalPages, EXPORT_MAX_PAGES)}` : ""}`);
        const res = await apiFetch<HubsoftInvoiceListResponse>(`/api/v1/integrations/hubsoft/hubsoft/financial/list?${base.toString()}`);
        if (!res.ok) throw new Error(res.message || "Falha ao consultar faturas.");
        all.push(...(res.invoices ?? []));
        totalPages = Math.max(res.total_pages, 1);
      }
      if (all.length === 0) {
        toastErr(pushToast, new Error("Nenhuma fatura para exportar."));
        return;
      }
      downloadInvoicesCsv(all);
      toastOk(pushToast, `${all.length.toLocaleString("pt-BR")} fatura(s) exportada(s).`);
    } catch (e) {
      toastErr(pushToast, e, "Falha ao exportar faturas.");
    } finally {
      setExportProgress(null);
    }
  }

  const [resendTarget, setResendTarget] = useState<HubsoftInvoiceRow | null>(null);
  const resendM = useMutation({
    mutationFn: (inv: HubsoftInvoiceRow) =>
      apiFetch(`/api/v1/integrations/hubsoft/hubsoft/financial/invoice/${encodeURIComponent(inv.id ?? "")}/resend`, { method: "POST", json: {} }),
    onSuccess: () => {
      toastOk(pushToast, "Fatura reenviada por e-mail.");
      setResendTarget(null);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao reenviar a fatura."),
  });

  const fieldStyle = { fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3, minWidth: 0 } as const;

  return (
    <div className="card">
      <div className="hubsoft-page-head" style={{ marginBottom: 10 }}>
        <div>
          <h2 style={{ margin: 0 }}>Faturas</h2>
          <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>Filtre por vencimento, status ou cliente.</p>
        </div>
        <div className="row" style={{ gap: 8 }}>
          <button
            type="button"
            className="btn btn--sm"
            disabled={exportProgress != null || invoices.length === 0}
            onClick={() => void exportCsv()}
            title="Exporta todas as páginas do período/filtros aplicados"
          >
            <Download size={13} style={{ marginRight: 4, verticalAlign: -2 }} />
            {exportProgress ?? "Exportar CSV"}
          </button>
          <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={() => void consult()}>
            <RefreshCw size={13} className={q.isFetching ? "map-refresh-spin" : undefined} style={{ marginRight: 4, verticalAlign: -2 }} />
            {q.isFetching ? "A consultar…" : "Consultar faturas"}
          </button>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))", gap: 10, alignItems: "end", marginBottom: 14 }}>
        <label style={fieldStyle}>
          Vencimento de
          <input type="date" className="input" value={from} onChange={(e) => setFrom(e.target.value)} />
        </label>
        <label style={fieldStyle}>
          Até
          <input type="date" className="input" value={to} onChange={(e) => setTo(e.target.value)} />
        </label>
        <div style={fieldStyle}>
          Atalho de período
          <div className="row" style={{ gap: 0 }}>
            {[30, 90, 180].map((days) => (
              <button key={days} type="button" className="btn btn--sm" onClick={() => applyPreset(days)}>
                {days} dias
              </button>
            ))}
          </div>
        </div>
        <label style={fieldStyle}>
          Status
          <select className="input" value={status} onChange={(e) => setStatus(e.target.value as "" | "aberto" | "quitado")}>
            <option value="">Todas</option>
            <option value="aberto">Em aberto</option>
            <option value="quitado">Quitadas</option>
          </select>
        </label>
        <label style={{ ...fieldStyle, gridColumn: "span 2" }}>
          Cliente
          <input
            className="input"
            value={busca}
            onChange={(e) => setBusca(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void consult();
            }}
            placeholder="Nome ou código"
          />
        </label>
      </div>

      {!applied ? (
        <div className="hubsoft-empty">Nenhuma consulta feita ainda.</div>
      ) : q.isLoading ? (
        <ConsultaLoading text="A carregar faturas…" />
      ) : q.isError ? (
        <div className="msg msg--err">{(q.error as Error).message}</div>
      ) : !d?.ok ? (
        <div className="msg msg--err">{d?.message || "Falha ao consultar faturas."}</div>
      ) : invoices.length === 0 ? (
        <div className="msg">{d.message || "Nenhuma fatura encontrada."}</div>
      ) : (
        <>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, fontSize: 11, color: "var(--muted)" }}>
            <span>
              {d.total_registros.toLocaleString("pt-BR")} fatura(s) · página {d.page + 1} de {Math.max(d.total_pages, 1)}
            </span>
            <div className="row" style={{ gap: 4 }}>
              <button type="button" className="btn btn--sm" disabled={page === 0 || q.isFetching} onClick={() => setPage((p) => Math.max(0, p - 1))}>
                <ChevronLeft size={12} />
              </button>
              <button
                type="button"
                className="btn btn--sm"
                disabled={page + 1 >= d.total_pages || q.isFetching}
                onClick={() => setPage((p) => p + 1)}
              >
                <ChevronRight size={12} />
              </button>
            </div>
          </div>

          <div className="table-wrap integration-support-table">
            <table className="integration-support-table__grid">
              <thead>
                <tr>
                  <th>Cliente</th>
                  <th>Plano</th>
                  <th>Cidade</th>
                  <th>Telefone</th>
                  <th>Vencimento</th>
                  <th>Pagamento</th>
                  <th>Valor</th>
                  <th>Status</th>
                  <th style={{ width: 40 }} />
                </tr>
              </thead>
              <tbody>
                {invoices.map((inv, i) => (
                  <tr key={inv.id ?? i}>
                    <td className="integration-support-table__cell">
                      {inv.client_name || "—"}
                      {inv.client_code ? <span className="mono integration-support-table__meta"> · {inv.client_code}</span> : null}
                    </td>
                    <td className="integration-support-table__cell">{inv.plan || "—"}</td>
                    <td className="integration-support-table__cell">{inv.city || "—"}</td>
                    <td className="mono integration-support-table__cell">{inv.phone || "—"}</td>
                    <td className="mono integration-support-table__cell">{fmtDateStr(inv.due_date)}</td>
                    <td className="mono integration-support-table__cell">{fmtDateStr(inv.payment_date)}</td>
                    <td className="mono integration-support-table__cell">{fmtCurrencyStr(inv.value)}</td>
                    <td className="integration-support-table__cell">
                      <span className={(inv.status && STATUS_CLASS[inv.status]) || "badge"}>
                        {(inv.status && STATUS_LABEL[inv.status]) || "—"}
                      </span>
                    </td>
                    <td className="integration-support-table__cell">
                      {inv.id ? (
                        <button type="button" className="btn btn--icon" title="Reenviar fatura por e-mail" onClick={() => setResendTarget(inv)}>
                          <Mail size={13} />
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      <ConfirmModal
        open={!!resendTarget}
        title="Reenviar fatura por e-mail"
        message={
          resendTarget
            ? `Reenviar a fatura de ${resendTarget.client_name || "este cliente"} (vencimento ${fmtDateStr(resendTarget.due_date)}) para o(s) e-mail(s) já cadastrado(s) na HubSoft?`
            : ""
        }
        confirmLabel="Reenviar"
        busy={resendM.isPending}
        onCancel={() => !resendM.isPending && setResendTarget(null)}
        onConfirm={() => resendTarget && resendM.mutate(resendTarget)}
      />
    </div>
  );
}
