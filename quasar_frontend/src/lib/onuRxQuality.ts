import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./api";

/** Faixas de qualidade da potência RX da ONU — configuráveis em Configurações → OLT (ver
 * OltVendorsPanel.tsx), guardadas em monitoring_settings (GET/PATCH /api/v1/settings/monitoring).
 * Usadas tanto no medidor do relatório telnet de uma ONU (OltOnuTelnetReportModal.tsx) quanto na
 * coluna RX da tabela principal de ONUs (OltVsolOnuTable.tsx) — um só lugar para configurar, dois
 * lugares que já mostravam a potência RX passam a usar a mesma classificação. */
export type OnuRxQuality = "bom" | "aceitavel" | "ruim";

export type OnuRxThresholds = {
  onu_rx_good_dbm: number;
  onu_rx_bad_dbm: number;
};

/** Espelha os defaults da coluna no banco (migração 141) — usado só como fallback enquanto a
 * consulta ainda não voltou (evita a UI "piscar" sem cor antes do primeiro fetch). */
export const DEFAULT_ONU_RX_THRESHOLDS: OnuRxThresholds = { onu_rx_good_dbm: -23, onu_rx_bad_dbm: -27 };

export function classifyOnuRx(dbm: number, t: OnuRxThresholds = DEFAULT_ONU_RX_THRESHOLDS): OnuRxQuality {
  if (dbm <= t.onu_rx_bad_dbm) return "ruim";
  if (dbm <= t.onu_rx_good_dbm) return "aceitavel";
  return "bom";
}

export const ONU_RX_QUALITY_COLOR: Record<OnuRxQuality, string> = {
  ruim: "var(--err)",
  aceitavel: "var(--warn)",
  bom: "var(--ok)",
};

export const ONU_RX_QUALITY_LABEL: Record<OnuRxQuality, string> = {
  ruim: "Ruim",
  aceitavel: "Aceitável",
  bom: "Bom",
};

/** Mesma query key usada pela tela de Monitoramento (MonitoringPage.tsx, "mon-settings") — cache
 * partilhada, e um PATCH em qualquer um dos dois sítios invalida os outros. */
export function useOnuRxThresholds() {
  const q = useQuery({
    queryKey: ["mon-settings"],
    queryFn: () =>
      apiFetch<{
        vps_latency_offset_ms: number;
        internet_check_targets: unknown;
        internet_check_timeout_ms: number;
        onu_rx_good_dbm: number;
        onu_rx_bad_dbm: number;
      }>("/api/v1/settings/monitoring"),
  });
  const thresholds: OnuRxThresholds = {
    onu_rx_good_dbm: q.data?.onu_rx_good_dbm ?? DEFAULT_ONU_RX_THRESHOLDS.onu_rx_good_dbm,
    onu_rx_bad_dbm: q.data?.onu_rx_bad_dbm ?? DEFAULT_ONU_RX_THRESHOLDS.onu_rx_bad_dbm,
  };
  return { ...q, thresholds };
}
