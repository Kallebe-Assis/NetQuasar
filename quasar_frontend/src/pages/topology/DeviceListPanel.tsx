import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Plus, Search } from "lucide-react";
import { deviceCategoryIcon } from "../../lib/deviceCategoryIcons";
import { MANUAL_EQUIPMENT_KINDS, type ManualEquipmentKind } from "../../lib/topologyManualKinds";
import type { TopologyDevice } from "./types";

export const TOPOLOGY_DRAG_MIME = "application/x-netquasar-topology-device";
export const TOPOLOGY_MANUAL_DRAG_MIME = "application/x-netquasar-topology-manual";

type Props = {
  devices: TopologyDevice[];
  placedIds: Set<string>;
  onAddDevice: (device: TopologyDevice) => void;
  onAddManual: (kind: ManualEquipmentKind) => void;
  collapsed: boolean;
  onToggleCollapsed: () => void;
};

/**
 * Painel lateral (lado direito da tela), retrátil tal como o menu principal (ShellLayout.tsx) —
 * recolhe para uma faixa fina, devolvendo espaço ao canvas. Uma única lista rolável: primeiro os
 * equipamentos cadastrados, depois — no fim da mesma lista, não numa secção à parte — o catálogo
 * fixo de equipamentos avulsos (sem cadastro no sistema, ver lib/topologyManualKinds.ts). Só
 * aparecem rolando até ao fim ou pesquisando (a pesquisa filtra os dois grupos).
 */
export function DeviceListPanel({ devices, placedIds, onAddDevice, onAddManual, collapsed, onToggleCollapsed }: Props) {
  const [q, setQ] = useState("");

  const filtered = useMemo(() => {
    const t = q.trim().toLowerCase();
    if (!t) return devices;
    return devices.filter(
      (d) =>
        d.description.toLowerCase().includes(t) ||
        d.category.toLowerCase().includes(t) ||
        (d.ip ?? "").toLowerCase().includes(t),
    );
  }, [devices, q]);

  const filteredManual = useMemo(() => {
    const t = q.trim().toLowerCase();
    if (!t) return MANUAL_EQUIPMENT_KINDS;
    return MANUAL_EQUIPMENT_KINDS.filter((k) => k.label.toLowerCase().includes(t));
  }, [q]);

  if (collapsed) {
    return (
      <aside className="topo-device-list topo-device-list--collapsed">
        <button type="button" className="btn btn--icon" title="Expandir lista de equipamentos" aria-label="Expandir lista de equipamentos" onClick={onToggleCollapsed}>
          <ChevronLeft size={16} />
        </button>
      </aside>
    );
  }

  return (
    <aside className="topo-device-list">
      <div className="topo-device-list__head">
        <span className="topo-device-list__section-title" style={{ margin: 0 }}>
          Equipamentos
        </span>
        <button type="button" className="btn btn--icon" title="Recolher" aria-label="Recolher lista de equipamentos" onClick={onToggleCollapsed}>
          <ChevronRight size={16} />
        </button>
      </div>
      <div className="topo-device-list__search">
        <Search size={14} />
        <input
          className="input"
          placeholder="Pesquisar equipamento…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 8px" }}>
        Arraste um equipamento para o canvas, ou clique em <Plus size={11} style={{ verticalAlign: -2 }} /> para
        adicioná-lo ao centro.
      </p>
      <div className="topo-device-list__items">
        {filtered.map((d) => {
          const Icon = deviceCategoryIcon(d.category);
          const placed = placedIds.has(d.id);
          return (
            <div
              key={d.id}
              className={`topo-device-list__item${placed ? " topo-device-list__item--placed" : ""}`}
              draggable
              onDragStart={(e) => {
                e.dataTransfer.setData(TOPOLOGY_DRAG_MIME, JSON.stringify(d));
                e.dataTransfer.effectAllowed = "copy";
              }}
              title={placed ? "Já está no canvas — pode arrastar de novo para reposicionar" : "Arraste para o canvas"}
            >
              <Icon size={16} />
              <div className="topo-device-list__item-text">
                <strong>{d.description}</strong>
                <span>
                  {d.category}
                  {d.ip ? ` · ${d.ip}` : ""}
                </span>
              </div>
              <button
                type="button"
                className="btn btn--icon"
                style={{ flexShrink: 0 }}
                title="Adicionar ao centro do canvas"
                onClick={() => onAddDevice(d)}
              >
                <Plus size={13} />
              </button>
            </div>
          );
        })}
        {filtered.length === 0 && filteredManual.length === 0 && (
          <p style={{ fontSize: 12, color: "var(--muted)", padding: 8 }}>Nenhum equipamento encontrado.</p>
        )}
        {filteredManual.length > 0 && (
          <>
            <p className="topo-device-list__section-title topo-device-list__section-title--divider">Equipamentos avulsos</p>
            {filteredManual.map((k) => {
              const Icon = k.icon;
              return (
                <div
                  key={k.id}
                  className="topo-device-list__item"
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData(TOPOLOGY_MANUAL_DRAG_MIME, k.id);
                    e.dataTransfer.effectAllowed = "copy";
                  }}
                  title="Sem cadastro — arraste para o canvas"
                >
                  <Icon size={16} />
                  <div className="topo-device-list__item-text">
                    <strong>{k.label}</strong>
                    <span>Equipamento avulso</span>
                  </div>
                  <button
                    type="button"
                    className="btn btn--icon"
                    style={{ flexShrink: 0 }}
                    title="Adicionar ao centro do canvas"
                    onClick={() => onAddManual(k.id)}
                  >
                    <Plus size={13} />
                  </button>
                </div>
              );
            })}
          </>
        )}
      </div>
    </aside>
  );
}
