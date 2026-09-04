import { useMemo, useState } from "react";
import { Plus, Search } from "lucide-react";
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
};

/**
 * Painel lateral (lado direito da tela) com 2 secções arrastáveis para o canvas:
 * equipamentos cadastrados (busca por descrição/categoria/IP) e equipamentos avulsos — um
 * catálogo fixo de elementos de rede sem cadastro no sistema (ver lib/topologyManualKinds.ts).
 */
export function DeviceListPanel({ devices, placedIds, onAddDevice, onAddManual }: Props) {
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

  return (
    <aside className="topo-device-list">
      <p className="topo-device-list__section-title">Equipamentos avulsos</p>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 6px" }}>
        Sem cadastro no sistema — descrição e IP ficam no próprio diagrama. Arraste para o canvas, ou clique em{" "}
        <Plus size={11} style={{ verticalAlign: -2 }} /> para adicionar ao centro.
      </p>
      <div className="topo-device-list__items topo-device-list__items--manual">
        {MANUAL_EQUIPMENT_KINDS.map((k) => {
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
              title="Arraste para o canvas"
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
      </div>

      <p className="topo-device-list__section-title" style={{ marginTop: 10 }}>
        Equipamentos cadastrados
      </p>
      <div className="topo-device-list__search">
        <Search size={14} />
        <input
          className="input"
          placeholder="Pesquisar equipamento…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>
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
        {filtered.length === 0 && (
          <p style={{ fontSize: 12, color: "var(--muted)", padding: 8 }}>Nenhum equipamento encontrado.</p>
        )}
      </div>
    </aside>
  );
}
