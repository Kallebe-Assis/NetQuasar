import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type MonitoringIntervalsPayload = {
  ping_seconds: number;
  telemetry_seconds: number;
  interface_snapshot_seconds: number;
  olt_if_derived_pon_seconds: number;
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

function effectiveMonitoringCycleSeconds(d: MonitoringIntervalsPayload): number {
  if (d.pipeline_cycle_seconds && d.pipeline_cycle_seconds >= 30) {
    return d.pipeline_cycle_seconds;
  }
  return Math.min(
    d.ping_seconds,
    d.telemetry_seconds ?? d.telemetry_minutes * 60,
    d.interface_snapshot_seconds ?? 300,
    d.olt_if_derived_pon_seconds ?? 240,
  );
}

export function MonitoringPingIntervalsCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const q = useQuery({
    queryKey: queryKeys.monIntervals,
    queryFn: () => apiFetch<MonitoringIntervalsPayload>("/api/v1/settings/monitoring-intervals"),
  });
  const [ps, setPs] = useState("");
  const [telS, setTelS] = useState("");
  const [ifaceS, setIfaceS] = useState("");
  const [oltDerivedS, setOltDerivedS] = useState("");
  const [pto, setPto] = useState("");
  const [telTimeout, setTelTimeout] = useState("");
  const [ifaceTimeout, setIfaceTimeout] = useState("");
  const [oltTimeout, setOltTimeout] = useState("");
  const [oltOnuTelnetTimeout, setOltOnuTelnetTimeout] = useState("");
  const [mikrotikTimeout, setMikrotikTimeout] = useState("");
  const [bngTimeout, setBngTimeout] = useState("");
  const [pipelineCycle, setPipelineCycle] = useState("");
  const [icmpSz, setIcmpSz] = useState("");
  const [offTh, setOffTh] = useState("");
  const [uptimeRestart, setUptimeRestart] = useState("");
  const [pingParallel, setPingParallel] = useState(true);
  useEffect(() => {
    if (!q.data) return;
    setPs((v) => (v === "" ? String(q.data.ping_seconds) : v));
    setTelS((v) => (v === "" ? String(q.data.telemetry_seconds ?? q.data.telemetry_minutes * 60) : v));
    setIfaceS((v) => (v === "" ? String(q.data.interface_snapshot_seconds ?? 300) : v));
    setOltDerivedS((v) => (v === "" ? String(q.data.olt_if_derived_pon_seconds ?? 240) : v));
    setPto((v) => (v === "" ? String(q.data.ping_timeout_ms ?? 5500) : v));
    setTelTimeout((v) => (v === "" ? String(q.data.telemetry_timeout_ms ?? 120000) : v));
    setIfaceTimeout((v) => (v === "" ? String(q.data.interface_snapshot_timeout_ms ?? 120000) : v));
    setOltTimeout((v) => (v === "" ? String(q.data.olt_if_derived_pon_timeout_ms ?? 180000) : v));
    setOltOnuTelnetTimeout((v) => (v === "" ? String(q.data.olt_onu_telnet_timeout_ms ?? 600000) : v));
    setMikrotikTimeout((v) => (v === "" ? String(q.data.mikrotik_timeout_ms ?? 120000) : v));
    setBngTimeout((v) => (v === "" ? String(q.data.bng_timeout_ms ?? q.data.telemetry_timeout_ms ?? 120000) : v));
    setPipelineCycle((v) => (v === "" ? String(q.data.pipeline_cycle_seconds ?? 120) : v));
    setIcmpSz((v) => (v === "" ? String(q.data.icmp_payload_bytes ?? 32) : v));
    setOffTh((v) => (v === "" ? String(q.data.offline_ping_fail_threshold ?? 3) : v));
    setUptimeRestart((v) => (v === "" ? String(q.data.uptime_restart_alert_minutes ?? 0) : v));
    setPingParallel(q.data.ping_parallel !== false);
  }, [q.data]);
  const save = useMutation({
    mutationFn: (body: Partial<MonitoringIntervalsPayload>) =>
      apiFetch<MonitoringIntervalsPayload>("/api/v1/settings/monitoring-intervals", { method: "PATCH", json: body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.monIntervals });
      save.reset();
      toastOk(pushToast, "Intervalos de monitoramento guardados.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar intervalos."),
  });

  if (q.isLoading) return <div className="card"><p>A carregar intervalos de sondagem…</p></div>;
  if (q.isError)
    return (
      <div className="card">
        <div className="msg msg--err">{(q.error as Error).message}</div>
      </div>
    );
  if (!q.data) return null;

  const d = q.data;
  const telSec = d.telemetry_seconds ?? d.telemetry_minutes * 60;
  const ifaceSec = d.interface_snapshot_seconds ?? 300;
  const oltSec = d.olt_if_derived_pon_seconds ?? 240;
  const minCycle = effectiveMonitoringCycleSeconds(d);

  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6, marginBottom: 0 }}>
        Intervalos globais e sondagem ICMP
        <InfoHint label="Funcionamento do worker de monitorização">
          <p>
            O <strong>ping (ICMP/TCP)</strong> corre em <strong>paralelo</strong> ao pipeline SNMP/OLT (intervalo «Intervalo entre pings»),
            para que a latência e alertas offline não parem durante colectas longas (ex.: dezenas de OLT).
            Os <strong>totais BNG</strong> (PPPoE online, IPv4/IPv6) também correm em paralelo, no intervalo de <strong>telemetria</strong>,
            com alertas de queda entre coletas conforme os limiares em Alertas.
            Telemetria, interfaces e ONU seguem em sequência no pipeline (~<strong>{minCycle} s</strong> entre ciclos).
          </p>
          <p>
            Valores gravados neste momento: ping (ICMP/TCP) <strong>{d.ping_seconds} s</strong>; tempo máximo da sonda{" "}
            <strong>{d.ping_timeout_ms ?? 5500} ms</strong>; pacote ICMP <strong>{d.icmp_payload_bytes ?? 32} B</strong>; falhas consecutivas até alerta
            offline <strong>{d.offline_ping_fail_threshold ?? 3}</strong>; telemetria <strong>{telSec} s</strong>; interfaces <strong>{ifaceSec} s</strong>;
            OLT/PON IF <strong>{oltSec} s</strong>; alarme de possível reinício (uptime) <strong>{d.uptime_restart_alert_minutes ?? 0}</strong> min (
            <strong>0</strong> = desligado).
          </p>
        </InfoHint>
      </h2>

      <section className="settings-intervals-section" aria-labelledby="mon-intervals-collect-heading">
        <h3 id="mon-intervals-collect-heading">Intervalos de coleta</h3>
        <div className="settings-fields-grid">
          <SettingsField
            label="Ciclo completo pipeline (s)"
            hintLabel="Intervalo entre execuções sequenciais"
            hint={
              <p>
                Tempo mínimo entre <strong>ciclos completos</strong> do pipeline (todos os passos configurados em ordem).
                Configure a ordem dos passos na secção abaixo nesta mesma aba.
              </p>
            }
          >
            <input className="input mono" aria-label="Intervalo pipeline em segundos" value={pipelineCycle} onChange={(e) => setPipelineCycle(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="Intervalo entre pings (s)"
            hintLabel="Intervalo de latência e ping ICMP/TCP"
            hint={
              <p>
                Intervalo do ping em paralelo (não bloqueia telemetria nem coleta OLT). Valores típicos: 30–60 s.
                Com «Ping em paralelo» activo, o passo ping na ordem de monitoramento é ignorado na sequência.
              </p>
            }
          >
            <input className="input mono" aria-label="Intervalo entre pings em segundos" value={ps} onChange={(e) => setPs(e.target.value)} />
          </SettingsField>
          <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 14, gridColumn: "1 / -1" }}>
            <input
              type="checkbox"
              checked={pingParallel}
              onChange={(e) => setPingParallel(e.target.checked)}
              aria-label="Ping em paralelo ao pipeline"
            />
            Ping em paralelo (recomendado — mantém monitoramento de latência durante colectas SNMP/OLT longas)
          </label>
          <SettingsField
            label="Telemetria SNMP (s)"
            hintLabel="Intervalo de telemetria SNMP"
            hint={
              <p>
                De quanto em quanto tempo o worker recolhe <strong>CPU, memória, temperatura e uptime</strong> (SNMP) nos equipamentos com telemetria ativa.
                O mesmo intervalo aplica-se à coleta paralela de <strong>totais BNG</strong> (PPPoE online) quando o passo BNG está activo no pipeline.
                Ex.: 180 s = 3 minutos entre amostras.
              </p>
            }
          >
            <input
              className="input mono"
              aria-label="Intervalo entre ciclos de telemetria em segundos"
              value={telS}
              onChange={(e) => setTelS(e.target.value)}
            />
          </SettingsField>
          <SettingsField
            label="Interfaces SNMP (s)"
            hintLabel="Intervalo de snapshots de interfaces"
            hint={
              <p>
                Intervalo do walk <strong>IF-MIB</strong> e gravação de <code className="mono">interface_snapshots</code> (MikroTik, OLT, etc.). Afecta
                tráfego, estados admin/oper e dados de interface na UI.
              </p>
            }
          >
            <input
              className="input mono"
              aria-label="Intervalo de snapshots de interfaces em segundos"
              value={ifaceS}
              onChange={(e) => setIfaceS(e.target.value)}
            />
          </SettingsField>
          <SettingsField
            label="OLT PON IF (s)"
            hintLabel="Intervalo PON derivada IF-MIB"
            hint={
              <p>
                Colecta de totais <strong>ONU/PON</strong> por derive IF-MIB (ex.: VSOL, fibre MikroTik). Só aplica a OLT compatíveis; fabricantes como
                ZTE/DATACOM usam outro fluxo (refresh OLT / API) e este passo pode ser omitido pelo worker.
              </p>
            }
          >
            <input
              className="input mono"
              aria-label="Intervalo entre colectas ONUs/PON derive IF-MIB em segundos"
              value={oltDerivedS}
              onChange={(e) => setOltDerivedS(e.target.value)}
            />
          </SettingsField>
        </div>
      </section>

      <section className="settings-intervals-section" aria-labelledby="mon-intervals-timeout-heading">
        <h3 id="mon-intervals-timeout-heading">Timeouts por tipo de coleta (ms)</h3>
        <div className="settings-fields-grid">
          <SettingsField
            label="Telemetria SNMP (ms)"
            hintLabel="Timeout da coleta de telemetria"
            hint={
              <p>
                Tempo máximo por ciclo de coleta SNMP de <strong>CPU, memória e uptime</strong>. Intervalo válido: <strong>5000–600000</strong> ms.
              </p>
            }
          >
            <input className="input mono" aria-label="Timeout telemetria em ms" value={telTimeout} onChange={(e) => setTelTimeout(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="Interfaces SNMP (ms)"
            hintLabel="Timeout do snapshot de interfaces"
            hint={
              <p>
                Tempo máximo do walk IF-MIB e gravação de snapshots. Intervalo válido: <strong>5000–600000</strong> ms.
              </p>
            }
          >
            <input className="input mono" aria-label="Timeout interfaces em ms" value={ifaceTimeout} onChange={(e) => setIfaceTimeout(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="OLT PON / ONUs (ms)"
            hintLabel="Timeout coleta ONUs OLT"
            hint={
              <p>
                Tempo máximo da colecta ONUs/PON SNMP nas OLT. Intervalo válido: <strong>5000–600000</strong> ms.
              </p>
            }
          >
            <input className="input mono" aria-label="Timeout OLT PON em ms" value={oltTimeout} onChange={(e) => setOltTimeout(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="OLT telnet ONU/PON (ms)"
            hintLabel="Timeout coleta telnet CLI por ONU"
            hint={
              <p>
                Tempo máximo da fase <strong>telnet</strong> (uma sessão por ciclo) para enriquecer ONUs e PONs/SFP.
                Com 16 PONs ou muitas ONUs, use valores maiores (ex.: <strong>1800000</strong> = 30 min).
                Intervalo válido: <strong>5000–3600000</strong> ms (até 60 min).
              </p>
            }
          >
            <input
              className="input mono"
              aria-label="Timeout telnet ONU OLT em ms"
              value={oltOnuTelnetTimeout}
              onChange={(e) => setOltOnuTelnetTimeout(e.target.value)}
            />
          </SettingsField>
          <SettingsField
            label="MikroTik (ms)"
            hintLabel="Timeout coleta MikroTik"
            hint={
              <p>
                Tempo máximo por equipamento MikroTik no pipeline. Intervalo válido: <strong>5000–600000</strong> ms.
              </p>
            }
          >
            <input className="input mono" aria-label="Timeout MikroTik em ms" value={mikrotikTimeout} onChange={(e) => setMikrotikTimeout(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="BNG (ms)"
            hintLabel="Timeout coleta BNG"
            hint={
              <p>
                Tempo máximo por concentrador BNG (totais PPPoE/IPv4/IPv6 e saúde SNMP). A coleta periódica e os alertas de queda usam amostras em{" "}
                <code className="mono">bng_stats_samples</code>.
                Intervalo válido: <strong>5000–600000</strong> ms.
              </p>
            }
          >
            <input className="input mono" aria-label="Timeout BNG em ms" value={bngTimeout} onChange={(e) => setBngTimeout(e.target.value)} />
          </SettingsField>
        </div>
      </section>

      <section className="settings-intervals-section" aria-labelledby="mon-intervals-icmp-heading">
        <h3 id="mon-intervals-icmp-heading">Sondagem ICMP e equipamento offline</h3>
        <div className="settings-fields-grid">
          <SettingsField
            label="Tempo máximo da sonda (ms)"
            hintLabel="Timeout da sonda de ping"
            hint={
              <p>
                Tempo máximo que cada tentativa de <strong>ping ICMP/TCP</strong> espera resposta antes de contar como falha. Intervalo válido na API:{" "}
                <strong>1000–30000</strong> ms. Valor mais baixo = detecção mais rápida, mas mais falsos positivos em redes lentas.
              </p>
            }
          >
            <input className="input mono" aria-label="Tempo máximo da sonda em milissegundos" value={pto} onChange={(e) => setPto(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="Pacote ICMP (bytes)"
            hintLabel="Tamanho do payload ICMP"
            hint={
              <p>
                Tamanho do payload ICMP enviado em cada ping (predefinição <strong>32</strong> bytes). Alguns equipamentos ou firewalls reagem de forma
                diferente a tamanhos maiores.
              </p>
            }
          >
            <input className="input mono" aria-label="Tamanho do pacote ICMP em bytes" value={icmpSz} onChange={(e) => setIcmpSz(e.target.value)} />
          </SettingsField>
          <SettingsField
            label="Falhas até alertar offline"
            hintLabel="Limiar de falhas consecutivas de ping"
            hint={
              <p>
                Número de ciclos do worker <strong>sem resposta</strong> antes de abrir o alerta «equipamento offline». No ping manual na UI, o sistema pode
                repetir tentativas na mesma acção até atingir este limiar. Ex.: <strong>3</strong> = três falhas seguidas.
              </p>
            }
          >
            <input
              className="input mono"
              aria-label="Número de falhas de ping consecutivas antes de alertar"
              value={offTh}
              onChange={(e) => setOffTh(e.target.value)}
            />
          </SettingsField>
        </div>
      </section>

      <section className="settings-intervals-section" aria-labelledby="mon-intervals-uptime-heading">
        <h3 id="mon-intervals-uptime-heading">Alerta de reinício</h3>
        <div className="settings-fields-grid">
          <SettingsField
            label="Uptime mínimo para alerta (min)"
            hintLabel="Alerta de possível reinício por uptime"
            hint={
              <p>
                Se o <strong>sysUpTime</strong> reportado por SNMP (em minutos) for <strong>inferior</strong> a este valor, o sistema cria um alerta de
                possível reinício do equipamento. Use <strong>0</strong> para desativar este tipo de alerta.
              </p>
            }
          >
            <input
              className="input mono"
              aria-label="Minutos de uptime abaixo dos quais alertar possível reinício"
              value={uptimeRestart}
              onChange={(e) => setUptimeRestart(e.target.value)}
            />
          </SettingsField>
        </div>
      </section>

      <div className="settings-intervals-actions">
        <button
          type="button"
          className="btn btn--primary"
          disabled={save.isPending}
          onClick={() =>
            save.mutate({
              ping_seconds: ps ? Number(ps) : undefined,
              pipeline_cycle_seconds: pipelineCycle ? Number(pipelineCycle) : undefined,
              telemetry_seconds: telS ? Number(telS) : undefined,
              interface_snapshot_seconds: ifaceS ? Number(ifaceS) : undefined,
              olt_if_derived_pon_seconds: oltDerivedS ? Number(oltDerivedS) : undefined,
              ping_timeout_ms: pto ? Number(pto) : undefined,
              telemetry_timeout_ms: telTimeout ? Number(telTimeout) : undefined,
              interface_snapshot_timeout_ms: ifaceTimeout ? Number(ifaceTimeout) : undefined,
              olt_if_derived_pon_timeout_ms: oltTimeout ? Number(oltTimeout) : undefined,
              olt_onu_telnet_timeout_ms: oltOnuTelnetTimeout ? Number(oltOnuTelnetTimeout) : undefined,
              mikrotik_timeout_ms: mikrotikTimeout ? Number(mikrotikTimeout) : undefined,
              bng_timeout_ms: bngTimeout ? Number(bngTimeout) : undefined,
              icmp_payload_bytes: icmpSz !== "" ? Number(icmpSz) : undefined,
              offline_ping_fail_threshold: offTh !== "" ? Number(offTh) : undefined,
              uptime_restart_alert_minutes: uptimeRestart !== "" ? Number(uptimeRestart) : undefined,
              ping_parallel: pingParallel,
            })
          }
        >
          Salvar intervalos / ICMP
        </button>
      </div>
      {save.isError && <div className="msg msg--err">{errorMessageFromUnknown(save.error)}</div>}
    </div>
  );
}
