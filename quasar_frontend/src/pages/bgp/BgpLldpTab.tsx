import { EM_DASH } from "./bgpFormat";
import type { LLDPNeighborReport } from "./bgpTypes";

type Props = {
  lldpNeighbors: LLDPNeighborReport[];
};

/** Aba "LLDP" — vizinhos LLDP por porta local (LLDP-MIB padrão), puramente informativo.
 * NÃO alimenta a tela Topologia nem oferece nenhuma acção de "adicionar à topologia" — a
 * topologia é desenhada manualmente pelo utilizador em Mapa → Topologia. */
export function BgpLldpTab({ lldpNeighbors }: Props) {
  if (lldpNeighbors.length === 0) {
    return (
      <div className="msg msg--warn">
        Sem vizinhos LLDP coletados. Active as métricas da secção "lldp" em Configurações → BGP se quiser estes dados.
      </div>
    );
  }
  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Vizinhos LLDP</h3>
      <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--muted)" }}>
        Apenas informativo — não alimenta a tela Topologia (Mapa → Topologia é desenhada manualmente).
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Porta local</th>
              <th>Chassi remoto</th>
              <th>Porta remota</th>
              <th>Descrição da porta</th>
              <th>Sistema remoto</th>
            </tr>
          </thead>
          <tbody>
            {lldpNeighbors.map((n) => (
              <tr key={`${n.local_port_num}-${n.rem_key}`}>
                <td className="mono">{n.local_if_name || n.local_port_num}</td>
                <td className="mono">{n.chassis_id || EM_DASH}</td>
                <td className="mono">{n.port_id || EM_DASH}</td>
                <td>{n.port_desc || EM_DASH}</td>
                <td>{n.sys_name || n.sys_desc || EM_DASH}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
