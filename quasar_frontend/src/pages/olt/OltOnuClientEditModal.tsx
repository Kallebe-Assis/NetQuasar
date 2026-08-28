import { useEffect, useState } from "react";
import { createPortal } from "react-dom";

type Props = {
  open: boolean;
  serial: string;
  currentName: string;
  busy?: boolean;
  onCancel: () => void;
  onSave: (name: string) => void;
  onRemove: () => void;
};

/**
 * Edição do vínculo cliente ↔ ONU de uma única linha (aba Pesquisa de ONUs) — mesmo padrão
 * visual do ConfirmModal, mas com um campo de texto para o nome do cliente. "Salvar" faz
 * upsert via POST /onu-client-links/import (1 linha); "Remover" chama
 * DELETE /onu-client-links/{serial}.
 */
export function OltOnuClientEditModal({ open, serial, currentName, busy, onCancel, onSave, onRemove }: Props) {
  const [name, setName] = useState(currentName);

  useEffect(() => {
    if (open) setName(currentName);
  }, [open, currentName]);

  if (!open) return null;
  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onCancel}>
      <div className="modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <h3>Editar cliente da ONU</h3>
        <p style={{ color: "var(--muted)", fontSize: 12, marginBottom: 10 }}>
          Serial: <span className="mono">{serial}</span>
        </p>
        <div className="field">
          <label>Nome do cliente</label>
          <input
            className="input"
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Ex.: João da Silva"
          />
        </div>
        <div className="row" style={{ justifyContent: "flex-end", marginTop: 12, flexWrap: "wrap", gap: 8 }}>
          <button type="button" className="btn" disabled={busy} onClick={onCancel}>
            Cancelar
          </button>
          {currentName.trim() ? (
            <button type="button" className="btn btn--danger" disabled={busy} onClick={onRemove}>
              {busy ? "…" : "Remover"}
            </button>
          ) : null}
          <button
            type="button"
            className="btn btn--primary"
            disabled={busy || !name.trim()}
            onClick={() => onSave(name.trim())}
          >
            {busy ? "…" : "Salvar"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
