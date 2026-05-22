import { createPortal } from "react-dom";
import { X } from "lucide-react";
import type { AttendanceItem, WorkOrderItem } from "./types";
import {
  formatSupportFieldValue,
  orderedRawEntries,
  supportFieldLabel,
  type SupportDetailKind,
} from "./ixcSupportFields";

const DETAIL_FONT = "var(--integration-detail-font-size, 11px)";

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="integration-detail__row">
      <span className="integration-detail__label">{label}</span>
      <span className={`integration-detail__value${mono ? " mono" : ""}`}>{value}</span>
    </div>
  );
}

function SummaryAttendance({ item }: { item: AttendanceItem }) {
  const rows: { label: string; value?: string; mono?: boolean }[] = [
    { label: "Protocolo", value: item.protocol, mono: true },
    { label: "Estado", value: item.status },
    { label: "Assunto", value: item.subject },
    { label: "Descrição", value: item.description },
    { label: "Abertura", value: item.opened_at, mono: true },
    { label: "Fechamento", value: item.closed_at, mono: true },
  ];
  const visible = rows.filter((r) => r.value?.trim());
  if (visible.length === 0) return null;
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">Resumo</h4>
      <div className="integration-detail__rows">
        {visible.map((r) => (
          <DetailRow key={r.label} label={r.label} value={r.value!} mono={r.mono} />
        ))}
      </div>
    </section>
  );
}

function SummaryWorkOrder({ item }: { item: WorkOrderItem }) {
  const rows: { label: string; value?: string; mono?: boolean }[] = [
    { label: "N.º O.S.", value: item.number ?? item.id, mono: true },
    { label: "Estado", value: item.status_label ?? item.status },
    { label: "Plano / serviço", value: item.plan_name },
    { label: "Descrição", value: item.description },
    { label: "Atendimento", value: item.attendance_protocol, mono: true },
    { label: "Cadastro", value: item.created_at, mono: true },
    { label: "Agendamento", value: item.scheduled_at, mono: true },
    { label: "Valor", value: item.value },
  ];
  const visible = rows.filter((r) => r.value?.trim());
  if (visible.length === 0) return null;
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">Resumo</h4>
      <div className="integration-detail__rows">
        {visible.map((r) => (
          <DetailRow key={r.label} label={r.label} value={r.value!} mono={r.mono} />
        ))}
      </div>
    </section>
  );
}

function RawFieldsBody({ raw, kind }: { raw?: Record<string, unknown>; kind: SupportDetailKind }) {
  const entries = orderedRawEntries(raw, kind);
  if (entries.length === 0) {
    return <p className="integration-detail__empty">Sem campos adicionais no registro.</p>;
  }
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">Dados completos (IXC)</h4>
      <div className="integration-detail__rows">
        {entries.map(([key, value]) => {
          const text = formatSupportFieldValue(key, value);
          if (!text) return null;
          const multiline = text.length > 120 || text.includes("\n");
          return (
            <div
              key={key}
              className={`integration-detail__row${multiline ? " integration-detail__row--stack" : ""}`}
            >
              <span className="integration-detail__label">{supportFieldLabel(kind, key)}</span>
              <span
                className={`integration-detail__value${multiline ? " integration-detail__value--block mono" : ""}`}
              >
                {text}
              </span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

export type SupportDetailTarget =
  | { kind: "attendance"; item: AttendanceItem }
  | { kind: "work_order"; item: WorkOrderItem };

export function supportDetailTitle(target: SupportDetailTarget): string {
  if (target.kind === "attendance") {
    const p = target.item.protocol?.trim();
    const id = target.item.id?.trim();
    if (p) return `Atendimento ${p}`;
    if (id) return `Atendimento #${id}`;
    return "Atendimento";
  }
  const n = target.item.number?.trim() ?? target.item.id?.trim();
  return n ? `Ordem de serviço ${n}` : "Ordem de serviço";
}

export function SupportItemDetailModal({
  target,
  onClose,
}: {
  target: SupportDetailTarget;
  onClose: () => void;
}) {
  const title = supportDetailTitle(target);
  const raw = target.item.raw;

  return createPortal(
    <div className="modal-backdrop modal-backdrop--stack" role="presentation" onMouseDown={onClose}>
      <div
        className="modal integration-detail-modal integration-support-detail-modal"
        role="dialog"
        aria-labelledby="support-item-detail-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="integration-detail-modal__head">
          <div style={{ minWidth: 0 }}>
            <h3 id="support-item-detail-title" className="integration-detail-modal__title">
              {title}
            </h3>
            <p className="integration-detail-modal__subtitle">Todos os campos retornados pela integração</p>
          </div>
          <button type="button" className="btn" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <div className="integration-support-detail-modal__body">
          <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
            {target.kind === "attendance" ? (
              <SummaryAttendance item={target.item} />
            ) : (
              <SummaryWorkOrder item={target.item} />
            )}
            <RawFieldsBody raw={raw} kind={target.kind} />
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}
