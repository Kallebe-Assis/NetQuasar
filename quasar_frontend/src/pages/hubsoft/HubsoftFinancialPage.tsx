import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Users } from "lucide-react";
import { useState } from "react";
import { HubsoftHeader } from "./HubsoftHeader";
import { HubsoftFinancialSummaryView, HubsoftTopDebtorsTable } from "./HubsoftFinancialSummaryView";
import { HubsoftInvoiceListPanel } from "./HubsoftInvoiceListPanel";
import { InfoHint } from "../../components/InfoHint";
import type { HubsoftFinancialSummaryResponse } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { useConsultaToast } from "./hubsoftConsulta";

import { ConsultaLoading } from "./ConsultaLoading";
/**
 * Financeiro da integração HubSoft — dois blocos independentes, cada um com o seu botão Consultar
 * (nada é buscado sozinho):
 *  1. Resumo dos últimos 6 meses: soma uma amostra de clientes (/cliente/financeiro só existe por
 *     cliente, não há "todas as faturas da operadora").
 *  2. Faturas: lista paginada com filtros de período/status/cliente e exportação CSV.
 * A HubSoft só expõe contas A RECEBER — não há "contas a pagar" (despesas a fornecedores).
 */
export function HubsoftFinancialPage() {
  const { runRefetch } = useConsultaToast();

  const q = useQuery({
    queryKey: queryKeys.hubsoftFinancialSummary,
    queryFn: () => apiFetch<HubsoftFinancialSummaryResponse>("/api/v1/integrations/hubsoft/hubsoft/financial-summary"),
    staleTime: Infinity,
    gcTime: 60 * 60 * 1000,
    enabled: false, // só consulta ao clicar em "Consultar"
  });

  const d = q.data;
  const [debtorsOpen, setDebtorsOpen] = useState(false);

  return (
    <div className="integration-consult">
      <HubsoftHeader />

      <div className="card">
        <div className="hubsoft-page-head" style={{ marginBottom: 10 }}>
          <div>
            <h2 style={{ margin: 0, display: "flex", alignItems: "center", gap: 6 }}>
              Resumo financeiro
              <InfoHint label="Sobre o resumo">
                <p>
                  Soma as faturas com vencimento nos últimos 6 meses de uma amostra de clientes (total a receber, vencido, pendente e pago).
                  A HubSoft só expõe contas a receber — não existe “contas a pagar” (despesas a fornecedores) nesta integração.
                </p>
              </InfoHint>
            </h2>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {d?.ok ? `${d.total_invoices.toLocaleString("pt-BR")} fatura(s) com vencimento nos últimos 6 meses.` : "Últimos 6 meses — amostra de clientes."}
            </p>
          </div>
          <div className="row" style={{ gap: 8 }}>
            <button type="button" className="btn btn--sm" disabled={!d?.ok} onClick={() => setDebtorsOpen(true)}>
              <Users size={13} style={{ marginRight: 4, verticalAlign: -2 }} aria-hidden />
              Maiores devedores
            </button>
            <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={() => void runRefetch(q.refetch)}>
              <RefreshCw size={13} className={q.isFetching ? "map-refresh-spin" : undefined} style={{ marginRight: 4, verticalAlign: -2 }} />
              {q.isFetching ? "A consultar…" : "Consultar resumo"}
            </button>
          </div>
        </div>

        {q.isFetching && !d ? (
          <ConsultaLoading text="A coletar amostra da HubSoft — pode demorar até alguns minutos…" />
        ) : q.isError ? (
          <div className="msg msg--err">{(q.error as Error).message}</div>
        ) : d && !d.ok ? (
          <div className="msg msg--err">{d.message || "Falha ao calcular o resumo financeiro."}</div>
        ) : d ? (
          <HubsoftFinancialSummaryView d={d} hideDebtors />
        ) : (
          <div className="hubsoft-empty">Nenhuma consulta feita ainda.</div>
        )}
      </div>

      <HubsoftInvoiceListPanel />

      {debtorsOpen && d?.ok ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setDebtorsOpen(false)}>
          <div
            className="modal modal--wide"
            style={{ maxWidth: 820, width: "100%", maxHeight: "85vh", overflow: "auto" }}
            role="dialog"
            aria-modal="true"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
              <h3 style={{ margin: 0 }}>
                Maiores devedores (amostra) <span className="integration-detail__count">({d.top_debtors.length})</span>
              </h3>
              <button type="button" className="btn btn--sm" onClick={() => setDebtorsOpen(false)}>
                Fechar
              </button>
            </div>
            <HubsoftTopDebtorsTable d={d} />
          </div>
        </div>
      ) : null}
    </div>
  );
}
