import { useQuery } from "@tanstack/react-query";
import { createPortal } from "react-dom";
import { RefreshCw, X } from "lucide-react";
import { apiFetch } from "../lib/api";
import { formatIntegrationDateTime } from "./integrationDisplay";
import type { HubsoftAttendanceDetail, HubsoftWorkOrderDetail } from "./types";

const DETAIL_FONT = "var(--integration-detail-font-size, 11px)";

export type HubsoftDetailTarget = { kind: "attendance"; protocol: string } | { kind: "work_order"; number: string };

function DetailRow({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value?.trim()) return null;
  return (
    <div className="integration-detail__row">
      <span className="integration-detail__label">{label}</span>
      <span className={`integration-detail__value${mono ? " mono" : ""}`}>{value}</span>
    </div>
  );
}

function MessageThread({ messages }: { messages: { text?: string; at?: string }[] }) {
  if (messages.length === 0) {
    return <p className="integration-detail__empty">Sem mensagens registradas.</p>;
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {messages.map((m, i) => (
        <div key={i} className="card" style={{ padding: "8px 10px", background: "var(--panel2)" }}>
          {m.at ? <div style={{ fontSize: 10, color: "var(--muted)", marginBottom: 3 }}>{m.at}</div> : null}
          <div style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{m.text || "—"}</div>
        </div>
      ))}
    </div>
  );
}

function AttendanceBody({ d }: { d: HubsoftAttendanceDetail }) {
  return (
    <>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">Resumo</h4>
        <div className="integration-detail__rows">
          <DetailRow label="Protocolo" value={d.protocol} mono />
          <DetailRow label="Estado" value={d.status} />
          <DetailRow label="Assunto" value={d.subject} />
          <DetailRow label="Cliente" value={d.client_name ? `${d.client_name}${d.client_code ? ` · ${d.client_code}` : ""}` : undefined} />
          <DetailRow label="Plano / serviço" value={d.plan_name} />
          <DetailRow label="Setor" value={d.sector} />
          <DetailRow label="Responsável" value={d.responsible_user} />
          <DetailRow label="Abertura" value={formatIntegrationDateTime(d.opened_at) || d.opened_at} />
          <DetailRow label="Aberto por" value={d.opened_by_user} />
          <DetailRow label="Fechamento" value={formatIntegrationDateTime(d.closed_at) || d.closed_at} />
          <DetailRow label="Fechado por" value={d.closed_by_user} />
          <DetailRow label="Motivo de fechamento" value={d.closing_reason} />
          <DetailRow label="Descrição de fechamento" value={d.closed_description} />
        </div>
      </section>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">Descrição de abertura</h4>
        <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", margin: 0 }}>{d.description || "—"}</p>
      </section>
      {d.work_orders && d.work_orders.length > 0 ? (
        <section className="integration-detail__section">
          <h4 className="integration-detail__section-title">Ordens de serviço vinculadas ({d.work_orders.length})</h4>
          <div className="integration-detail__rows">
            {d.work_orders.map((wo, i) => (
              <DetailRow
                key={wo.id ?? wo.number ?? i}
                label={`N.º ${wo.number ?? wo.id ?? "—"}`}
                value={[wo.status, wo.plan_name || wo.description].filter(Boolean).join(" — ")}
              />
            ))}
          </div>
        </section>
      ) : null}
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">Mensagens ({d.messages.length})</h4>
        <MessageThread messages={d.messages} />
      </section>
    </>
  );
}

function WorkOrderBody({ d }: { d: HubsoftWorkOrderDetail }) {
  return (
    <>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">Resumo</h4>
        <div className="integration-detail__rows">
          <DetailRow label="N.º O.S." value={d.number} mono />
          <DetailRow label="Tipo" value={d.type} />
          <DetailRow label="Estado" value={d.status} />
          <DetailRow label="Estado de fechamento" value={d.status_closed} />
          <DetailRow label="Cliente" value={d.client_name ? `${d.client_name}${d.client_code ? ` · ${d.client_code}` : ""}` : undefined} />
          <DetailRow label="Plano / serviço" value={d.plan_name} />
          <DetailRow label="Atendimento" value={d.attendance_protocol} mono />
          <DetailRow label="Cadastro" value={formatIntegrationDateTime(d.created_at) || d.created_at} />
          <DetailRow label="Aberta por" value={d.opened_by_user} />
          <DetailRow label="Fechada por" value={d.closed_by_user} />
          <DetailRow label="Agendamento (início)" value={formatIntegrationDateTime(d.scheduled_start) || d.scheduled_start} />
          <DetailRow label="Agendamento (fim)" value={formatIntegrationDateTime(d.scheduled_end) || d.scheduled_end} />
          <DetailRow label="Execução (início)" value={formatIntegrationDateTime(d.executed_start) || d.executed_start} />
          <DetailRow label="Execução (fim)" value={formatIntegrationDateTime(d.executed_end) || d.executed_end} />
        </div>
      </section>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">Descrição de abertura</h4>
        <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", margin: 0 }}>{d.description || "—"}</p>
      </section>
      {d.service_description && d.service_description.trim() !== d.description?.trim() ? (
        <section className="integration-detail__section">
          <h4 className="integration-detail__section-title">Descrição do serviço</h4>
          <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", margin: 0 }}>{d.service_description}</p>
        </section>
      ) : null}
      {d.closed_description ? (
        <section className="integration-detail__section">
          <h4 className="integration-detail__section-title">Descrição de fechamento</h4>
          <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", margin: 0 }}>{d.closed_description}</p>
        </section>
      ) : null}
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">Mensagens ({d.messages.length})</h4>
        <MessageThread messages={d.messages} />
      </section>
    </>
  );
}

/**
 * "Ver mais" nas abas Atendimentos/Ordens de serviço da integração HubSoft — ao contrário do
 * SupportItemDetailModal genérico (que só reexibe os campos já carregados na lista), este busca
 * o registo específico (por protocolo/número) sob demanda, num pedido leve e separado, trazendo
 * campos que a lista paginada não tem (descrição de abertura, responsável, O.S. vinculadas) e a
 * conversa completa (mensagens).
 */
export function HubsoftSupportDetailModal({ target, onClose }: { target: HubsoftDetailTarget; onClose: () => void }) {
  const isAttendance = target.kind === "attendance";
  const key = isAttendance ? target.protocol : target.number;
  const title = isAttendance ? `Atendimento ${key}` : `Ordem de serviço ${key}`;

  const q = useQuery({
    queryKey: ["hubsoft-support-detail", target.kind, key],
    queryFn: () =>
      isAttendance
        ? apiFetch<HubsoftAttendanceDetail>(`/api/v1/integrations/hubsoft/hubsoft/attendance/detail?protocolo=${encodeURIComponent(key)}`)
        : apiFetch<HubsoftWorkOrderDetail>(`/api/v1/integrations/hubsoft/hubsoft/work-orders/detail?numero=${encodeURIComponent(key)}`),
  });

  return createPortal(
    <div className="modal-backdrop modal-backdrop--stack" role="presentation" onMouseDown={onClose}>
      <div
        className="modal integration-detail-modal integration-support-detail-modal integration-support-detail-modal--lg"
        role="dialog"
        aria-labelledby="hubsoft-support-detail-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="integration-detail-modal__head">
          <div style={{ minWidth: 0 }}>
            <h3 id="hubsoft-support-detail-title" className="integration-detail-modal__title">
              {title}
            </h3>
            <p className="integration-detail-modal__subtitle">Detalhe completo e conversa — HubSoft</p>
          </div>
          <button type="button" className="btn" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <div className="integration-support-detail-modal__body">
          <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
            {q.isLoading ? (
              <div className="hubsoft-loading">
                <RefreshCw size={16} className="map-refresh-spin" />
                <span>A carregar detalhe…</span>
              </div>
            ) : q.isError ? (
              <div className="msg msg--err">{(q.error as Error).message}</div>
            ) : !q.data?.ok ? (
              <div className="msg msg--err">{q.data?.message || "Falha ao carregar detalhe."}</div>
            ) : isAttendance ? (
              <AttendanceBody d={q.data as HubsoftAttendanceDetail} />
            ) : (
              <WorkOrderBody d={q.data as HubsoftWorkOrderDetail} />
            )}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}
