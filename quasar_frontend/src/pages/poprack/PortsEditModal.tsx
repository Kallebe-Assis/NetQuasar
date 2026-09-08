import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { RACK_PORT_TYPE_LABELS, type RackPort, type RackPortType } from "./types";

type Props = {
  open: boolean;
  nodeLabel: string;
  ports: RackPort[];
  onSave: (ports: RackPort[]) => void;
  onClose: () => void;
};

/** Modal de edição de portas de uma caixa do diagrama 2D do POP — quantidade (adicionar/remover
 * linhas), e por porta: descrição e tipo (SFP/SFP+/Ethernet), tudo opcional. Draft local, só
 * grava no nó (via onSave) ao clicar "Guardar" — fechar/cancelar não perde nada no diagrama. */
export function PortsEditModal({ open, nodeLabel, ports, onSave, onClose }: Props) {
  const [draft, setDraft] = useState<RackPort[]>(ports);

  useEffect(() => {
    if (open) setDraft(ports);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  if (!open) return null;

  function patchPort(index: number, patch: Partial<RackPort>) {
    setDraft((d) => d.map((p) => (p.index === index ? { ...p, ...patch } : p)));
  }
  function removePort(index: number) {
    setDraft((d) => d.filter((p) => p.index !== index));
  }
  function addPort() {
    const nextIndex = draft.length > 0 ? Math.max(...draft.map((p) => p.index)) + 1 : 1;
    setDraft((d) => [...d, { index: nextIndex, label: String(nextIndex) }]);
  }
  function save() {
    onSave(draft);
    onClose();
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div className="modal modal--wide" role="dialog" aria-modal="true" style={{ maxWidth: 640 }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>Portas — {nodeLabel}</h3>
        <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 12px" }}>
          Descrição e tipo são opcionais. Adicione ou remova linhas para mudar a quantidade de portas.
        </p>
        <div className="table-wrap" style={{ maxHeight: 380, overflow: "auto" }}>
          <table style={{ fontSize: 12 }}>
            <thead>
              <tr>
                <th style={{ width: 70 }}>Nº / rótulo</th>
                <th>Descrição</th>
                <th style={{ width: 160 }}>Tipo</th>
                <th style={{ width: 36 }} />
              </tr>
            </thead>
            <tbody>
              {draft.map((p) => (
                <tr key={p.index}>
                  <td>
                    <input
                      className="input mono"
                      style={{ width: 60 }}
                      value={p.label}
                      onChange={(e) => patchPort(p.index, { label: e.target.value })}
                    />
                  </td>
                  <td>
                    <input
                      className="input"
                      style={{ width: "100%" }}
                      value={p.description ?? ""}
                      placeholder="Opcional"
                      onChange={(e) => patchPort(p.index, { description: e.target.value })}
                    />
                  </td>
                  <td>
                    <select
                      className="select"
                      value={p.portType ?? ""}
                      onChange={(e) => patchPort(p.index, { portType: (e.target.value || null) as RackPortType | null })}
                    >
                      <option value="">—</option>
                      {(Object.entries(RACK_PORT_TYPE_LABELS) as Array<[RackPortType, string]>).map(([v, label]) => (
                        <option key={v} value={v}>
                          {label}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <button type="button" className="btn btn--icon" title="Remover porta" aria-label="Remover porta" onClick={() => removePort(p.index)}>
                      <Trash2 size={13} />
                    </button>
                  </td>
                </tr>
              ))}
              {draft.length === 0 ? (
                <tr>
                  <td colSpan={4} style={{ color: "var(--muted)", textAlign: "center", padding: 12 }}>
                    Sem portas.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
        <button type="button" className="btn btn--sm" style={{ marginTop: 10 }} onClick={addPort}>
          <Plus size={13} style={{ verticalAlign: -2 }} /> Adicionar porta
        </button>
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" onClick={save}>
            Guardar
          </button>
        </div>
      </div>
    </div>
  );
}
