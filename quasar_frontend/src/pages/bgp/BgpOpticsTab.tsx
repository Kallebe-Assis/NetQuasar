import { EM_DASH } from "./bgpFormat";
import type { OpticsReport } from "./bgpTypes";

type Props = {
  optics: OpticsReport[];
};

/** Aba "Óptica" — diagnóstico óptico completo por porta (HUAWEI-ENTITY-EXTENT-MIB
 * hwOpticalModuleInfoTable): potência Rx/Tx, corrente do laser (bias), temperatura e tensão do
 * transceiver, rotulado com o nome da porta (entPhysicalName). Desligado por omissão no
 * catálogo (pode ter dezenas de portas) — ligue em Configurações → BGP → secção "Diagnóstico
 * óptico por porta" se quiser estes dados no ciclo periódico. */
export function BgpOpticsTab({ optics }: Props) {
  if (optics.length === 0) {
    return (
      <div className="msg msg--warn">
        Sem dados ópticos coletados. Active as métricas da secção "Diagnóstico óptico por porta" em Configurações → BGP
        (desligadas por omissão, pois podem ter dezenas de portas por ciclo de coleta).
      </div>
    );
  }
  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Diagnóstico óptico por porta</h3>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Porta</th>
              <th>Potência Rx</th>
              <th>Potência Tx</th>
              <th>Corrente do laser</th>
              <th>Temperatura</th>
              <th>Tensão</th>
            </tr>
          </thead>
          <tbody>
            {optics.map((o) => (
              <tr key={o.physical_index}>
                <td className="mono">{o.port_label || o.physical_index}</td>
                <td className="mono">{o.rx_power ?? EM_DASH}</td>
                <td className="mono">{o.tx_power ?? EM_DASH}</td>
                <td className="mono">{o.bias_current ?? EM_DASH}</td>
                <td className="mono">{o.temperature ?? EM_DASH}</td>
                <td className="mono">{o.voltage ?? EM_DASH}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
