import { EM_DASH } from "./bgpFormat";
import type { CarStatReport, QosQueueReport } from "./bgpTypes";

type Props = {
  qosQueues: QosQueueReport[];
  carStats: CarStatReport[];
};

/** Aba "QoS" — descarte por classe/fila (HUAWEI-HQOS-MIB e HUAWEI-CBQOS-MIB). "Chave"
 * identifica a fila/classe dentro da interface (direção + camadas do índice composto do
 * equipamento) — não há um nome de fila simples nestas MIBs, por isso fica como identificador
 * técnico ao lado da interface. */
export function BgpQosTab({ qosQueues, carStats }: Props) {
  return (
    <>
      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Descarte por fila (HQoS)</h3>
        {qosQueues.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem dados de filas HQoS (métricas da secção "qos" podem estar desligadas em Configurações → BGP).
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Interface</th>
                  <th>Fila</th>
                  <th>Encaminhados (bytes)</th>
                  <th>Encaminhados (pacotes)</th>
                  <th>Descartados (bytes)</th>
                  <th>Descartados (pacotes)</th>
                </tr>
              </thead>
              <tbody>
                {qosQueues.map((q) => (
                  <tr key={`${q.if_index}-${q.queue_key}`}>
                    <td className="mono">{q.if_name || q.if_index}</td>
                    <td className="mono">{q.queue_key}</td>
                    <td className="mono">{q.forward_bytes ?? EM_DASH}</td>
                    <td className="mono">{q.forward_packets ?? EM_DASH}</td>
                    <td className="mono">{q.drop_bytes ?? EM_DASH}</td>
                    <td className="mono">{q.drop_packets ?? EM_DASH}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 14 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Descarte por classe (CBQoS CAR)</h3>
        {carStats.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem dados de CAR/classe (métricas da secção "qos" podem estar desligadas em Configurações → BGP).
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Interface</th>
                  <th>Classe</th>
                  <th>Conformados (bytes)</th>
                  <th>Excedidos (bytes)</th>
                  <th>Descartados (bytes)</th>
                </tr>
              </thead>
              <tbody>
                {carStats.map((c) => (
                  <tr key={`${c.if_index}-${c.class_key}`}>
                    <td className="mono">{c.if_name || c.if_index}</td>
                    <td className="mono">{c.class_key}</td>
                    <td className="mono">{c.conformed_bytes ?? EM_DASH}</td>
                    <td className="mono">{c.exceeded_bytes ?? EM_DASH}</td>
                    <td className="mono">{c.dropped_bytes ?? EM_DASH}</td>
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
