import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import { AttendanceTabContent } from "../../integrations/HubsoftClientResults";
import { SupportItemDetailModal, type SupportDetailTarget } from "../../integrations/SupportItemDetailModal";
import type { RecentActivityResponse } from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";

/**
 * Atendimentos recentes de TODOS os clientes — não há endpoint na HubSoft que liste isso
 * directamente (todo endpoint de cliente exige busca+termo_busca, confirmado em produção;
 * o "irmão" sem /cliente/ também não existe: "Método/endpoint não disponível"). O NetQuasar
 * varre uma amostra de clientes e consulta o atendimento de cada um (ver
 * internal/integrationhubsoft.BuildRecentActivity) — por isso demora e fica em cache
 * (queryKeys.hubsoftRecentActivity, partilhada com a aba Ordens de serviço) até pedir para
 * atualizar.
 */
export function HubsoftAttendancePage() {
  const qc = useQueryClient();
  const [detailTarget, setDetailTarget] = useState<SupportDetailTarget | null>(null);

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
            <h2 style={{ margin: 0 }}>Atendimentos recentes</h2>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {d ? `Amostra de ${d.sample_clients.toLocaleString("pt-BR")} clientes consultados — mostrando os 20 atendimentos mais recentes.` : "Varre uma amostra de clientes e mostra os atendimentos mais recentes encontrados."}
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
          <div className="msg msg--err">{d?.message || "Falha ao coletar atendimentos."}</div>
        ) : (
          <AttendanceTabContent loading={false} ok message={d.message} items={d.attendance} onShowDetail={setDetailTarget} />
        )}
      </div>
      {detailTarget ? <SupportItemDetailModal target={detailTarget} onClose={() => setDetailTarget(null)} /> : null}
    </div>
  );
}
