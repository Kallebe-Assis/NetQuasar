import { ArrowRightLeft, Cable, Network, RadioTower, Router, Server, type LucideIcon } from "lucide-react";

/**
 * Catálogo fixo de "equipamentos avulsos" da tela Topologia (menu Mapa → Topologia) — nós que
 * representam um elemento de rede (switch, roteador, rádio, conversor de mídia, ONU, OLT) sem
 * estar ligado a nenhum equipamento cadastrado no sistema (internal/api/handlers_devices.go).
 * Servem para desenhar pontos da rede que ainda não têm — ou nunca vão ter — cadastro próprio
 * (ex.: uma ONU de cliente, um conversor de mídia numa caixa de emenda). Descrição e IP são
 * sempre opcionais.
 */
export type ManualEquipmentKind = "switch" | "roteador" | "radio" | "conversor" | "onu" | "olt";

export type ManualKindMeta = { id: ManualEquipmentKind; label: string; icon: LucideIcon };

export const MANUAL_EQUIPMENT_KINDS: ManualKindMeta[] = [
  { id: "switch", label: "Switch", icon: Network },
  { id: "roteador", label: "Roteador", icon: Router },
  { id: "radio", label: "Rádio", icon: RadioTower },
  { id: "conversor", label: "Conversor de mídia", icon: ArrowRightLeft },
  { id: "onu", label: "ONU", icon: Cable },
  { id: "olt", label: "OLT", icon: Server },
];

const BY_ID: Record<string, ManualKindMeta> = Object.fromEntries(MANUAL_EQUIPMENT_KINDS.map((k) => [k.id, k]));

export function manualKindMeta(kind: string | undefined | null): ManualKindMeta {
  return BY_ID[String(kind ?? "")] ?? MANUAL_EQUIPMENT_KINDS[0];
}

export const DEFAULT_MANUAL_KIND: ManualEquipmentKind = "switch";
