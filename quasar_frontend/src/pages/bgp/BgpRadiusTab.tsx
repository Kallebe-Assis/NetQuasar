import { EM_DASH } from "./bgpFormat";
import type { RadiusServerReport } from "./bgpTypes";

type Props = {
  radiusServers: RadiusServerReport[];
};

/** Aba "RADIUS" — saúde por servidor RADIUS (HUAWEI-BRAS-RADIUS-MIB). Aviso: esta MIB é
 * tipicamente do lado BNG/AAA do chassi — numa virtual-system dedicada só a BGP pode não estar
 * visível (particionamento de VS), o que é esperado e não indica um problema de coleta. Se
 * vier vazio aqui, confirme no equipamento BNG. */
export function BgpRadiusTab({ radiusServers }: Props) {
  if (radiusServers.length === 0) {
    return (
      <div className="msg msg--warn">
        Sem dados de RADIUS coletados. Isto é esperado se este equipamento for uma virtual-system dedicada só a BGP — a
        função RADIUS/AAA costuma estar noutra VS (ex.: "BNG"), com agente SNMP próprio. Verifique também se as métricas da
        secção "radius" estão activas em Configurações → BGP.
      </div>
    );
  }
  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Saúde do RADIUS por servidor</h3>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Servidor</th>
              <th>Auth: pedidos</th>
              <th>Auth: aceites</th>
              <th>Auth: rejeitados</th>
              <th>Auth: timeouts</th>
              <th>Auth: sem resposta</th>
              <th>Acct: pedidos</th>
              <th>Acct: respostas</th>
              <th>Acct: timeouts</th>
              <th>Acct: sem resposta</th>
            </tr>
          </thead>
          <tbody>
            {radiusServers.map((r) => (
              <tr key={r.server_ip}>
                <td className="mono">{r.server_ip}</td>
                <td className="mono">{r.authen_requests ?? EM_DASH}</td>
                <td className="mono">{r.authen_accepts ?? EM_DASH}</td>
                <td className="mono">{r.authen_rejects ?? EM_DASH}</td>
                <td className="mono">{r.authen_timeouts ?? EM_DASH}</td>
                <td className="mono">{r.authen_server_not_reply ?? EM_DASH}</td>
                <td className="mono">{r.acct_requests ?? EM_DASH}</td>
                <td className="mono">{r.acct_responses ?? EM_DASH}</td>
                <td className="mono">{r.acct_timeouts ?? EM_DASH}</td>
                <td className="mono">{r.acct_server_not_reply ?? EM_DASH}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
