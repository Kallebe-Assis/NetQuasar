import type { QueryClient } from "@tanstack/react-query";
import { apiFetch } from "./api";
import { errorMessageFromUnknown } from "./apiErrors";
import { invalidateDashboardAfterCollect } from "./dashboardCache";
import { queryKeys } from "./queryKeys";

type ToastApi = {
  push: (input: {
    tone: "ok" | "err" | "info";
    text: string;
    loading?: boolean;
    autoMs?: number;
  }) => string;
  dismiss: (id: string) => void;
};

export async function collectDeviceTelemetry(
  deviceId: string,
  deviceLabel: string,
  toast: ToastApi,
  qc?: QueryClient,
): Promise<void> {
  const label = deviceLabel.trim() || deviceId.slice(0, 8);
  const loadingId = toast.push({
    tone: "info",
    text: `Coletando telemetria SNMP de ${label}…`,
    loading: true,
    autoMs: 0,
  });
  try {
    await apiFetch(`/api/v1/telemetry/devices/${deviceId}/collect`, { method: "POST", json: {} });
    toast.dismiss(loadingId);
    toast.push({ tone: "ok", text: `Telemetria de ${label} concluída.` });
    void qc?.invalidateQueries({ queryKey: queryKeys.alertsPingUnreachable });
    void qc?.invalidateQueries({ queryKey: ["mikrotik-tel", deviceId] });
    if (qc) {
      void qc.invalidateQueries({ queryKey: queryKeys.monitoringActiveEquipment });
      invalidateDashboardAfterCollect(qc);
    }
  } catch (e) {
    toast.dismiss(loadingId);
    toast.push({ tone: "err", text: errorMessageFromUnknown(e) || `Falha na telemetria de ${label}.` });
    throw e;
  }
}
