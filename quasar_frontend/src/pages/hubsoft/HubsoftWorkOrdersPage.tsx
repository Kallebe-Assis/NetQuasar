import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import { WorkOrdersTabContent } from "../../integrations/HubsoftClientResults";
import type { SupportDetailTarget } from "../../integrations/SupportItemDetailModal";
import { HubsoftSupportDetailModal, type HubsoftDetailTarget } from "../../integrations/HubsoftSupportDetailModal";
import type { RecentActivityResponse } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";

/**
 * Ordens de serviço recentes de TODOS os clientes — mesma amostra/cache que a aba
 * Atendimentos (ver HubsoftAttendancePage e internal/integrationhubsoft.BuildRecentActivity);
 * usa a mesma queryKey, então quem já abriu uma das duas abas não paga o custo da varredura
 * de novo ao abrir a outra.
 */
export function HubsoftWorkOrdersPage() {
  const qc = useQueryClient();
  const [detailTarget, setDetailTarget] = useState<HubsoftDetailTarget | null>(null);

  // Ver comentário equivalente em HubsoftAttendancePage.tsx.
  function handleShowDetail(t: SupportDetailTarget) {
    if (t.kind === "work_order" && t.item.number) {
      setDetailTarget({ kind: "work_order", number: t.item.number });
    }
  }

  const q = useQuery({
    queryKey: queryKeys.hubsoftRecentActivity,
    queryFn: () => apiFetch<RecentActivityResponse>("/api/v1/integrations/hubsoft/hubsoft/recent-activity"),
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
            <h2 style={{ margin: 0 }}>Ordens de serviço recentes</h2>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {d
                ? `${d.total_work_orders_found.toLocaleString("pt-BR")} ordem(ns) de serviço nos últimos 30 dias — mostrando as 20 mais recentes.`
                : "Consulta todas as ordens de serviço dos últimos 30 dias e mostra as mais recentes."}
            </p>
          </div>
          <button
            type="button"
            className="btn btn--sm"
            disabled={q.isFetching}
            onClick={() => void qc.refetchQueries({ queryKey: queryKeys.hubsoftRecentActivity })}
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
          <div className="msg msg--err">{d?.message || "Falha ao coletar ordens de serviço."}</div>
        ) : (
          <WorkOrdersTabContent loading={false} ok message={d.message} items={d.work_orders} onShowDetail={handleShowDetail} />
        )}
      </div>
      {detailTarget ? <HubsoftSupportDetailModal target={detailTarget} onClose={() => setDetailTarget(null)} /> : null}
    </div>
  );
}
