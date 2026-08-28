import { useMemo, useState } from "react";
import { Plus, Search } from "lucide-react";
import { deviceCategoryIcon } from "../../lib/deviceCategoryIcons";
import type { TopologyDevice } from "./types";

export const TOPOLOGY_DRAG_MIME = "application/x-netquasar-topology-device";

type Props = {
  devices: TopologyDevice[];
  placedIds: Set<string>;
  onAddDevice: (device: TopologyDevice) => void;
};

/**
 * Lista vertical de equipamentos (lado direito da tela) — descrição/categoria/IP, arrastável
 * para o canvas. Equipamentos já colocados no canvas ficam esmaecidos com uma marca.
 */
export function DeviceListPanel({ devices, placedIds, onAddDevice }: Props) {
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
        {filtered.length === 0 && (
          <p style={{ fontSize: 12, color: "var(--muted)", padding: 8 }}>Nenhum equipamento encontrado.</p>
        )}
      </div>
    </aside>
  );
}
