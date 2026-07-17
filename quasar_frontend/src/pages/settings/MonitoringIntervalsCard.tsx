import type { Dispatch, SetStateAction } from "react";
import { SettingsField } from "../../components/SettingsField";

export type MonitoringIntervalsPayload = {
  ping_seconds: number;
  telemetry_seconds: number;
  interface_snapshot_seconds: number;
  olt_if_derived_pon_seconds: number;
  olt_pon_status_seconds?: number;
  olt_onu_counts_seconds?: number;
  olt_full_collect_seconds?: number;
  olt_full_collect_schedule?: string;
  pipeline_cycle_seconds?: number;
  telemetry_minutes: number;
  ping_timeout_ms: number;
  telemetry_timeout_ms?: number;
  interface_snapshot_timeout_ms?: number;
  olt_if_derived_pon_timeout_ms?: number;
  olt_onu_telnet_timeout_ms?: number;
  mikrotik_timeout_ms?: number;
  bng_timeout_ms?: number;
  icmp_payload_bytes?: number;
  offline_ping_fail_threshold?: number;
  uptime_restart_alert_minutes?: number;
  ping_parallel?: boolean;
  pipeline_steps?: unknown[];
};

/** Campos avançados (timeouts / ICMP) usados pelo painel unificado de monitoramento. */
export function MonitoringAdvancedFields({
  draft,
  setDraft,
}: {
  draft: MonitoringIntervalsPayload;
  setDraft: Dispatch<SetStateAction<MonitoringIntervalsPayload>>;
}) {
  const set = <K extends keyof MonitoringIntervalsPayload>(key: K, value: MonitoringIntervalsPayload[K]) => {
    setDraft((d) => ({ ...d, [key]: value }));
  };

  return (
    <>
      <section className="settings-intervals-section" aria-labelledby="mon-adv-timeout-heading">
        <h3 id="mon-adv-timeout-heading">Timeouts por tipo de coleta (ms)</h3>
        <div className="settings-fields-grid">
          <SettingsField label="Telemetria SNMP (ms)" hintLabel="Timeout telemetria" hint={<p>Tempo máximo por ciclo de telemetria SNMP. Intervalo: 5000–600000 ms.</p>}>
            <input
              className="input mono"
              value={String(draft.telemetry_timeout_ms ?? 120000)}
              onChange={(e) => set("telemetry_timeout_ms", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="Interfaces SNMP (ms)" hintLabel="Timeout interfaces" hint={<p>Tempo máximo do walk IF-MIB. Intervalo: 5000–600000 ms.</p>}>
            <input
              className="input mono"
              value={String(draft.interface_snapshot_timeout_ms ?? 120000)}
              onChange={(e) => set("interface_snapshot_timeout_ms", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="OLT PON / ONUs (ms)" hintLabel="Timeout OLT" hint={<p>Tempo máximo da coleta ONU/PON SNMP. Intervalo: 5000–600000 ms.</p>}>
            <input
              className="input mono"
              value={String(draft.olt_if_derived_pon_timeout_ms ?? 180000)}
              onChange={(e) => set("olt_if_derived_pon_timeout_ms", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="OLT telnet ONU/PON (ms)" hintLabel="Timeout telnet" hint={<p>Tempo máximo da fase telnet. Intervalo: 5000–3600000 ms.</p>}>
            <input
              className="input mono"
              value={String(draft.olt_onu_telnet_timeout_ms ?? 600000)}
              onChange={(e) => set("olt_onu_telnet_timeout_ms", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="MikroTik (ms)" hintLabel="Timeout MikroTik" hint={<p>Tempo máximo por equipamento MikroTik. Intervalo: 5000–600000 ms.</p>}>
            <input
              className="input mono"
              value={String(draft.mikrotik_timeout_ms ?? 120000)}
              onChange={(e) => set("mikrotik_timeout_ms", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="BNG (ms)" hintLabel="Timeout BNG" hint={<p>Tempo máximo por concentrador BNG. Intervalo: 5000–600000 ms.</p>}>
            <input
              className="input mono"
              value={String(draft.bng_timeout_ms ?? 120000)}
              onChange={(e) => set("bng_timeout_ms", Number(e.target.value))}
            />
          </SettingsField>
        </div>
      </section>

      <section className="settings-intervals-section" aria-labelledby="mon-adv-icmp-heading">
        <h3 id="mon-adv-icmp-heading">Sondagem ICMP e equipamento offline</h3>
        <div className="settings-fields-grid">
          <SettingsField label="Tempo máximo da sonda (ms)" hintLabel="Timeout ping" hint={<p>Timeout ICMP/TCP por tentativa. Intervalo: 1000–30000 ms.</p>}>
            <input className="input mono" value={String(draft.ping_timeout_ms)} onChange={(e) => set("ping_timeout_ms", Number(e.target.value))} />
          </SettingsField>
          <SettingsField label="Pacote ICMP (bytes)" hintLabel="Payload ICMP" hint={<p>Tamanho do payload ICMP (predefinição 32).</p>}>
            <input
              className="input mono"
              value={String(draft.icmp_payload_bytes ?? 32)}
              onChange={(e) => set("icmp_payload_bytes", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="Falhas até alertar offline" hintLabel="Limiar offline" hint={<p>Falhas consecutivas antes do alerta offline.</p>}>
            <input
              className="input mono"
              value={String(draft.offline_ping_fail_threshold ?? 3)}
              onChange={(e) => set("offline_ping_fail_threshold", Number(e.target.value))}
            />
          </SettingsField>
          <SettingsField label="Uptime mínimo para alerta (min)" hintLabel="Reinício" hint={<p>0 = desligado. Alerta se sysUpTime for inferior a este valor.</p>}>
            <input
              className="input mono"
              value={String(draft.uptime_restart_alert_minutes ?? 0)}
              onChange={(e) => set("uptime_restart_alert_minutes", Number(e.target.value))}
            />
          </SettingsField>
        </div>
      </section>
    </>
  );
}

/** @deprecated Prefer MonitoringSettingsPanel — mantido para imports legados. */
export function MonitoringPingIntervalsCard() {
  return null;
}
