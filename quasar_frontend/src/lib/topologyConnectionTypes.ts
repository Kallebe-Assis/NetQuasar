/**
 * Catálogo fixo de tipos de conexão da tela Topologia (menu Mapa) — cor, traço e ícone por
 * tipo. Não existe nenhum conceito de "conexão entre 2 equipamentos" no resto do sistema
 * (confirmado por pesquisa exaustiva nas migrações), então este catálogo é próprio da tela.
 */
export type TopologyConnectionType = "fibra" | "transporte" | "radio" | "utp" | "vpn" | "outro";

export type ConnectionTypeMeta = {
  id: TopologyConnectionType;
  label: string;
  color: string;
  dash?: string; // strokeDasharray do SVG; undefined = traço contínuo
};

export const TOPOLOGY_CONNECTION_TYPES: ConnectionTypeMeta[] = [
  { id: "fibra", label: "Fibra óptica", color: "#eab308" },
  { id: "transporte", label: "Transporte", color: "#f97316" },
  { id: "radio", label: "Rádio / Wireless", color: "#a855f7", dash: "6 4" },
  { id: "utp", label: "Cabo UTP", color: "#3b82f6" },
  { id: "vpn", label: "VPN", color: "#22c55e", dash: "2 4" },
  { id: "outro", label: "Outro", color: "#94a3b8", dash: "1 3" },
];

const BY_ID: Record<string, ConnectionTypeMeta> = Object.fromEntries(
  TOPOLOGY_CONNECTION_TYPES.map((t) => [t.id, t]),
);

export function connectionTypeMeta(type: string | undefined | null): ConnectionTypeMeta {
  return BY_ID[String(type ?? "")] ?? TOPOLOGY_CONNECTION_TYPES[TOPOLOGY_CONNECTION_TYPES.length - 1];
}

export const DEFAULT_CONNECTION_TYPE: TopologyConnectionType = "fibra";
export const DEFAULT_EDGE_ICON_SIZE = 18;
export const MIN_EDGE_ICON_SIZE = 10;
export const MAX_EDGE_ICON_SIZE = 40;

export const DEFAULT_NODE_SIZE = 64;
export const MIN_NODE_SIZE = 32;
export const MAX_NODE_SIZE = 160;

export const DEFAULT_GROUP_SIZE = { width: 320, height: 240 };
export const MIN_GROUP_SIZE = { width: 120, height: 100 };
