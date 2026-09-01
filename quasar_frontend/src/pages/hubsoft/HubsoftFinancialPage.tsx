import { useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import { HubsoftFinancialSummaryView } from "./HubsoftFinancialSummaryView";
import { HubsoftInvoiceListPanel } from "./HubsoftInvoiceListPanel";
import type { HubsoftFinancialSummaryResponse } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";

/**
 * Resumo financeiro agregado — total a receber (pendente + vencido) e pago, somado numa
 * amostra de clientes (mesma técnica do Dashboard: /cliente/financeiro só existe por
 * cliente, não há "todas as faturas da operadora"). A API da HubSoft, sendo o faturamento do
 * próprio provedor aos seus clientes, só expõe contas A RECEBER — não há "contas a pagar"
 * (despesas a fornecedores) nesta integração.
 */
export function HubsoftFinancialPage() {
  const qc = useQueryClient();

  const q = useQuery({
    queryKey: queryKeys.hubsoftFinancialSummary,
    queryFn: () => apiFetch<HubsoftFinancialSummaryResponse>("/api/v1/integrations/hubsoft/hubsoft/financial-summary"),
    staleTime: Infinity,
    gcTime: 60 * 60 * 1000,
  });

  const d = q.data;

  return (
    <div className="integration-consult">
      <HubsoftHeader />
      <div className="card">
        <div className="hubsoft-page-head">
          <div>
            <h2 style={{ margin: 0 }}>Financeiro</h2>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {d
                ? `${d.total_invoices.toLocaleString("pt-BR")} fatura(s) com vencimento nos últimos 6 meses. A HubSoft só expõe contas a receber (faturas de clientes) nesta integração — não existe "contas a pagar" (despesas a fornecedores) aqui.`
                : "Soma as faturas dos últimos 6 meses — total a receber, vencido, pendente e pago."}
            </p>
          </div>
          <button
            type="button"
            className="btn btn--sm"
            disabled={q.isFetching}
            onClick={() => void qc.refetchQueries({ queryKey: queryKeys.hubsoftFinancialSummary })}
          >
            <RefreshCw size={13} className={q.isFetching ? "map-refresh-spin" : undefined} /> Atualizar
          </button>
        </div>

        {q.isLoading ? (
          <div className="hubsoft-loading">
            <RefreshCw size={18} className="map-refresh-spin" />
            <span>A coletar amostra da HubSoft — isto pode demorar até alguns minutos…</span>
          </div>
        ) : q.isError ? (
          <div className="msg msg--err">{(q.error as Error).message}</div>
        ) : !d?.ok ? (
          <div className="msg msg--err">{d?.message || "Falha ao calcular o resumo financeiro."}</div>
        ) : (
          <HubsoftFinancialSummaryView d={d} />
        )}

        <HubsoftInvoiceListPanel />
      </div>
    </div>
  );
}
