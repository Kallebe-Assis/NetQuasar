import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Mail, RefreshCw } from "lucide-react";
import type { HubsoftInvoiceListResponse, HubsoftInvoiceRow } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { ConfirmModal } from "../../components/ConfirmModal";

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
  const [appliedBusca, setAppliedBusca] = useState("");
  const [page, setPage] = useState(0);
  const perPage = 25;

  const params = useMemo(() => {
    const p = new URLSearchParams();
    p.set("page", String(page));
    p.set("per_page", String(perPage));
    if (from) p.set("data_inicio", from);
    if (to) p.set("data_fim", to);
    if (status === "aberto") p.set("apenas_em_aberto", "sim");
    if (status === "quitado") p.set("apenas_quitado", "sim");
    if (appliedBusca.trim()) p.set("busca", appliedBusca.trim());
    return p.toString();
  }, [page, from, to, status, appliedBusca]);

  const q = useQuery({
    queryKey: ["hubsoft-financial-list", params],
    placeholderData: keepPreviousData,
    queryFn: () => apiFetch<HubsoftInvoiceListResponse>(`/api/v1/integrations/hubsoft/hubsoft/financial/list?${params}`),
  });

  const d = q.data;
  const invoices = d?.invoices ?? [];

  function applyPreset(days: number) {
    setFrom(todayISO(-days));
    setTo(todayISO());
    setPage(0);
  }

  const { push: pushToast } = useAppToast();
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

  return (
    <section className="integration-detail__section" style={{ marginTop: 16 }}>
      <h4 className="integration-detail__section-title">Faturas</h4>

      <div className="row" style={{ gap: 6, flexWrap: "wrap", alignItems: "flex-end", marginBottom: 10 }}>
        <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
          Vencimento de
          <input
            type="date"
            className="input"
            style={{ fontSize: 12 }}
            value={from}
            onChange={(e) => {
              setFrom(e.target.value);
              setPage(0);
            }}
          />
        </label>
        <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
          até
          <input
            type="date"
            className="input"
            style={{ fontSize: 12 }}
            value={to}
            onChange={(e) => {
              setTo(e.target.value);
              setPage(0);
            }}
          />
        </label>
        <button type="button" className="btn btn--sm" onClick={() => applyPreset(30)}>
          30 dias
        </button>
        <button type="button" className="btn btn--sm" onClick={() => applyPreset(90)}>
          90 dias
        </button>
        <button type="button" className="btn btn--sm" onClick={() => applyPreset(180)}>
          180 dias
        </button>
        <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
          Estado
          <select
            className="input"
            style={{ fontSize: 12 }}
            value={status}
            onChange={(e) => {
              setStatus(e.target.value as "" | "aberto" | "quitado");
              setPage(0);
            }}
          >
            <option value="">Todas</option>
            <option value="aberto">Em aberto</option>
            <option value="quitado">Quitadas</option>
          </select>
        </label>
        <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
          Cliente
          <input
            className="input"
            style={{ fontSize: 12 }}
            value={busca}
            onChange={(e) => setBusca(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                setAppliedBusca(busca);
                setPage(0);
              }
            }}
            placeholder="Nome ou código"
          />
        </label>
        <button
          type="button"
          className="btn btn--sm"
          onClick={() => {
            setAppliedBusca(busca);
            setPage(0);
          }}
        >
          Buscar
        </button>
        <button type="button" className="btn btn--sm" disabled={q.isFetching} onClick={() => void q.refetch()} style={{ marginLeft: "auto" }}>
          <RefreshCw size={12} className={q.isFetching ? "map-refresh-spin" : undefined} style={{ marginRight: 4, verticalAlign: -2 }} />
          Atualizar
        </button>
      </div>

      {q.isLoading ? (
        <div className="hubsoft-loading">
          <RefreshCw size={16} className="map-refresh-spin" />
          <span>A carregar faturas…</span>
        </div>
      ) : q.isError ? (
        <div className="msg msg--err">{(q.error as Error).message}</div>
      ) : !d?.ok ? (
        <div className="msg msg--err">{d?.message || "Falha ao consultar faturas."}</div>
      ) : invoices.length === 0 ? (
        <div className="msg">{d.message || "Nenhuma fatura encontrada."}</div>
      ) : (
        <>
          <div className="table-wrap integration-support-table">
            <table className="integration-support-table__grid">
              <thead>
                <tr>
                  <th>Cliente</th>
                  <th>Vencimento</th>
                  <th>Pagamento</th>
                  <th>Valor</th>
                  <th>Estado</th>
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

          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginTop: 8, fontSize: 11, color: "var(--muted)" }}>
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
    </section>
  );
}
