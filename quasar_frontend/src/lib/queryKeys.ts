import type { QueryClient } from "@tanstack/react-query";

/**
 * Chaves React Query partilhadas — usar estas constantes em vez de strings soltas
 * para invalidações e cache alinhados entre páginas.
 */
export const queryKeys = {
  uiAppearance: ["ui-appearance"] as const,
  monState: ["mon-state"] as const,
  monStateGlobal: ["mon-state-global-indicator"] as const,
  monIntervals: ["mon-intervals"] as const,
  automationOnu: ["automation-onu"] as const,
  automationOnuRuns: ["automation-onu-runs"] as const,
  automationAlertsDigest: ["automation-alerts-digest"] as const,
  automationCommercial: ["automation-commercial"] as const,
  automationHistory: ["automation-history"] as const,
  smtpSettings: ["smtp-settings"] as const,
  alertsIncidents: ["alerts-incidents"] as const,
  alertsActive: ["alerts-active"] as const,
  alertsIgnored: ["alerts-ignored"] as const,
  alertsHist: ["alerts-hist"] as const,
  alertsResolvedWindow: ["alerts-resolved-window"] as const,
  alertsPingUnreachable: ["alerts-ping-unreachable"] as const,
  monitoringActiveEquipment: ["monitoring-active-equipment"] as const,
  oltDevices: ["olt-devices"] as const,
  clientConnections: ["client-connections"] as const,
  commercialLocalities: ["commercial-loc"] as const,
  mapConnectionPoints: ["map-connection-points"] as const,
  networkProjects: ["network-projects"] as const,
  networkCtos: ["network-ctos"] as const,
  networkSpliceBoxes: ["network-splice-boxes"] as const,
  networkCables: ["network-cables"] as const,
  networkPoles: ["network-poles"] as const,
  settingsAlertThresholdRules: ["settings-alert-threshold-rules"] as const,
  alertRules: ["alert-rules"] as const,
  integrations: ["integrations"] as const,
  integrationDetail: (slug: string) => ["integration", slug] as const,
  integrationLogs: (slug: string, requestId?: string) => ["integration-logs", slug, requestId ?? ""] as const,
} as const;

/** Invalida listas de alertas usadas em Alertas, OLT e Configurações. */
export function invalidateAlertListQueries(qc: QueryClient): Promise<void> {
  return Promise.all([
    qc.invalidateQueries({ queryKey: queryKeys.alertsActive }),
    qc.invalidateQueries({ queryKey: queryKeys.alertsHist }),
    qc.invalidateQueries({ queryKey: queryKeys.alertsResolvedWindow }),
    qc.invalidateQueries({ queryKey: queryKeys.alertsIncidents }),
    qc.invalidateQueries({ queryKey: queryKeys.alertsIgnored }),
  ]).then(() => undefined);
}
