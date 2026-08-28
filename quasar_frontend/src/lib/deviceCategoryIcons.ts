import { Box, Cable, Cpu, HelpCircle, Network, RadioTower, Router, Server, Zap, type LucideIcon } from "lucide-react";

/**
 * Ícone por categoria de equipamento (mesmas 9 categorias de DevicesPage.tsx `CATEGORIES`) —
 * usado na tela Topologia (menu Mapa) para o ícone de cada nó de equipamento. Não existe
 * nenhum mapeamento categoria→ícone no resto do sistema (os ícones da barra lateral
 * "Equipamentos" em ShellLayout.tsx são por rota de navegação, não por este campo `category`).
 */
export const DEVICE_CATEGORY_ICONS: Record<string, LucideIcon> = {
  Concentrador: Router,
  Energia: Zap,
  Mikrotik: Cpu,
  Switch: Network,
  OLT: Cable,
  Rádio: RadioTower,
  Servidor: Server,
  "Máquina Virtual": Box,
  Outros: HelpCircle,
};

export function deviceCategoryIcon(category: string | undefined | null): LucideIcon {
  const key = String(category ?? "").trim();
  return DEVICE_CATEGORY_ICONS[key] ?? HelpCircle;
}
