import { createPortal } from "react-dom";

type ConfirmModalProps = {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  secondaryLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  onSecondary?: () => void;
};

export function ConfirmModal({
  open,
  title,
  message,
  confirmLabel = "Confirmar",
  cancelLabel = "Cancelar",
  secondaryLabel,
  danger,
  busy,
  onCancel,
  onConfirm,
  onSecondary,
}: ConfirmModalProps) {
  if (!open) return null;
  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onCancel}>
      <div className="modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <h3>{title}</h3>
        <p style={{ color: "var(--muted)", fontSize: 12 }}>{message}</p>
        <div className="row" style={{ justifyContent: "flex-end", marginTop: 10, flexWrap: "wrap", gap: 8 }}>
          <button type="button" className="btn" disabled={busy} onClick={onCancel}>
            {cancelLabel}
          </button>
          {secondaryLabel && onSecondary ? (
            <button type="button" className="btn" disabled={busy} onClick={onSecondary}>
              {secondaryLabel}
            </button>
          ) : null}
          <button type="button" className={`btn ${danger ? "btn--danger" : "btn--primary"}`} disabled={busy} onClick={onConfirm}>
            {busy ? "…" : confirmLabel}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

