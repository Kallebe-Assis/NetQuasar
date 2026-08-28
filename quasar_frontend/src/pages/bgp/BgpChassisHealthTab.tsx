import { EM_DASH, severityBadge } from "./bgpFormat";
import type { BoardAlarmReport, FanReport, PowerSupplyReport, TemperatureReport, VoltageReport } from "./bgpTypes";

type Props = {
  boardAlarms: BoardAlarmReport[];
  fans: FanReport[];
  powerSupplies: PowerSupplyReport[];
  temperatures: TemperatureReport[];
  voltages: VoltageReport[];
};

const ALARM_COLOR: Record<string, string> = {
  normal: "#3fb950",
  aviso: "var(--warn)",
  grave: "var(--err)",
  desconhecido: "var(--muted)",
};

/** Aba "Saúde do Chassi" — "semáforo" de hardware por placa (hwEntityAlarmLight, BITS
 * decodificado em report_hardware.go) + ventoinhas/fontes/temperatura/tensão por sensor, cada
 * um com o estado normal/aviso/grave/fatal já reportado pelo próprio equipamento. */
export function BgpChassisHealthTab({ boardAlarms, fans, powerSupplies, temperatures, voltages }: Props) {
  return (
    <>
      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Luz de alarme por placa</h3>
        <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--muted)" }}>
          Semáforo de hardware por placa (hwEntityAlarmLight) — verde/amarelo/vermelho.
        </p>
        {boardAlarms.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem dados de alarme por placa (métrica "entity_alarm_light" pode estar desligada em Configurações → BGP).
          </div>
        ) : (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 10 }}>
            {boardAlarms.map((b) => (
              <div
                key={b.physical_index}
                className="card"
                style={{ padding: "10px 14px", minWidth: 160, display: "flex", alignItems: "center", gap: 10 }}
              >
                <span
                  style={{
                    display: "inline-block",
                    width: 12,
                    height: 12,
                    borderRadius: "50%",
                    background: ALARM_COLOR[b.severity] ?? "var(--muted)",
                    flexShrink: 0,
                  }}
                />
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600 }}>{b.name || b.physical_index}</div>
                  <div style={{ fontSize: 11, color: "var(--muted)" }}>{b.severity}</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Ventoinhas</h3>
        {fans.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>Sem dados de ventoinhas (secção "chassi" desligada em Configurações → BGP).</div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Slot</th>
                  <th>Nº série</th>
                  <th>Presente</th>
                  <th>Velocidade</th>
                  <th>Estado</th>
                </tr>
              </thead>
              <tbody>
                {fans.map((f) => (
                  <tr key={`${f.slot}-${f.sn}`}>
                    <td className="mono">{f.slot}</td>
                    <td className="mono">{f.sn}</td>
                    <td className="mono">{f.present === "1" ? "Sim" : f.present === "2" ? "Não" : EM_DASH}</td>
                    <td className="mono">{f.speed ?? EM_DASH}</td>
                    <td>{severityBadge(f.state_label)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Fontes de alimentação</h3>
        {powerSupplies.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>Sem dados de fontes (secção "chassi" desligada em Configurações → BGP).</div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Slot</th>
                  <th>Nº série</th>
                  <th>Presente</th>
                  <th>Corrente</th>
                  <th>Tensão</th>
                  <th>Estado</th>
                </tr>
              </thead>
              <tbody>
                {powerSupplies.map((p) => (
                  <tr key={`${p.slot}-${p.sn}`}>
                    <td className="mono">{p.slot}</td>
                    <td className="mono">{p.sn}</td>
                    <td className="mono">{p.present === "1" ? "Sim" : p.present === "2" ? "Não" : EM_DASH}</td>
                    <td className="mono">{p.current ?? EM_DASH}</td>
                    <td className="mono">{p.voltage ?? EM_DASH}</td>
                    <td>{severityBadge(p.state_label)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Temperatura por sensor</h3>
        {temperatures.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>Sem dados de temperatura (secção "chassi" desligada em Configurações → BGP).</div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Chassi</th>
                  <th>Slot</th>
                  <th>I2C</th>
                  <th>Valor</th>
                  <th>Estado</th>
                </tr>
              </thead>
              <tbody>
                {temperatures.map((t) => (
                  <tr key={`${t.slot_raw}-${t.i2c}`}>
                    <td className="mono">{t.chassis ?? EM_DASH}</td>
                    <td className="mono">{t.slot ?? EM_DASH}</td>
                    <td className="mono">{t.i2c}</td>
                    <td className="mono">{t.value ?? EM_DASH}</td>
                    <td>{severityBadge(t.status_label)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 14 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Tensão por sensor</h3>
        {voltages.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>Sem dados de tensão (secção "chassi" desligada em Configurações → BGP).</div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Chassi</th>
                  <th>Slot</th>
                  <th>I2C</th>
                  <th>Valor</th>
                  <th>Estado</th>
                </tr>
              </thead>
              <tbody>
                {voltages.map((v) => (
                  <tr key={`${v.slot_raw}-${v.i2c}`}>
                    <td className="mono">{v.chassis ?? EM_DASH}</td>
                    <td className="mono">{v.slot ?? EM_DASH}</td>
                    <td className="mono">{v.i2c}</td>
                    <td className="mono">{v.value ?? EM_DASH}</td>
                    <td>{severityBadge(v.status_label)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  );
}
