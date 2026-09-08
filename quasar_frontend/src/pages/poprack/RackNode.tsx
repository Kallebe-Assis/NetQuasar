import { memo } from "react";
import { Handle, NodeToolbar, Position, type NodeProps } from "@xyflow/react";
import { Pencil, Trash2 } from "lucide-react";
import { RACK_KIND_LABELS, RACK_PORT_TYPE_ICONS, type RackNode as RackNodeT, type RackNodeData } from "./types";

/** Rectângulo de rack — cabeçalho (tipo + nome editável) e uma célula por porta, cada célula com
 * o seu próprio Handle (ligação de fibra porta-a-porta, não nó-a-nó como a Topologia geral).
 * Handle com `style={{position:"static"}}` sai do posicionamento absoluto por lado que o React
 * Flow usa por omissão (Position.Top/Bottom/…) e passa a fluir normalmente dentro do grid de
 * portas — é o mesmo truque de "um handle por posição custom" quando não são só 4 lados fixos. */
function RackNodeInner({ id, data, selected }: NodeProps<RackNodeT & { data: RackNodeData }>) {
  return (
    <div className={`rack-node rack-node--${data.kind}${selected ? " rack-node--selected" : ""}`}>
      <NodeToolbar isVisible={selected} position={Position.Top} style={{ zIndex: 1000 }}>
        <button
          type="button"
          className="btn btn--icon topo-node-toolbar__btn"
          title="Editar portas (quantidade, descrição, tipo)"
          onClick={() => data.onEditPorts?.(id)}
        >
          <Pencil size={13} />
        </button>
        <button
          type="button"
          className="btn btn--icon topo-node-toolbar__btn"
          title="Remover esta caixa do diagrama"
          onClick={() => data.onRequestRemove?.(id)}
        >
          <Trash2 size={13} />
        </button>
      </NodeToolbar>
      <div className="rack-node__header">
        <span className="rack-node__kind">{RACK_KIND_LABELS[data.kind]}</span>
        <input
          className="rack-node__label-input"
          value={data.label}
          onChange={(e) => data.onPatch?.(id, { label: e.target.value })}
          onPointerDown={(e) => e.stopPropagation()}
          placeholder="Nome"
        />
      </div>
      <div className="rack-node__ports">
        {data.ports.map((p) => {
          const TypeIcon = p.portType ? RACK_PORT_TYPE_ICONS[p.portType] : null;
          const title = [`Porta ${p.label}`, p.description, p.portType].filter(Boolean).join(" — ");
          return (
            <div key={p.index} className="rack-node__port" title={title}>
              <Handle
                id={`port-${p.index}`}
                type="source"
                position={Position.Bottom}
                isConnectableStart
                isConnectableEnd
                className="rack-node__port-handle"
                style={{ position: "static", transform: "none" }}
              />
              {TypeIcon ? <TypeIcon size={9} className="rack-node__port-type-icon" /> : null}
              <span className="rack-node__port-num">{p.label}</span>
            </div>
          );
        })}
        {data.ports.length === 0 ? <span className="rack-node__no-ports">Sem portas</span> : null}
      </div>
    </div>
  );
}

export const RackNode = memo(RackNodeInner);
