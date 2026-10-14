import { ExportMenu } from "../../components/ExportMenu";
import { EM_DASH, formatDuration, peerStateBadge } from "./bgpFormat";
import type { BFDSessionReport, PeerReport } from "./bgpTypes";

type Props = {
  peers: PeerReport[];
  bfdSessions: BFDSessionReport[];
};

const BFD_STATE_BADGE: Record<string, string> = { up: "badge--ok", down: "badge--err", init: "badge--warn", "admin down": "badge--off" };

/** Aba "Peers" — tabela de peers BGP já existente na Visão Geral, agora com as 3 colunas de
 * prefixos (recebidos/activos/anunciados — HUAWEI-BGP-VPN-MIB hwBgpPeerRouteTable, contador
 * pré-agregado pelo próprio equipamento) + sessões BFD associadas (HUAWEI-BFD-MIB), que também
 * protegem sessões de peer BGP contra queda lenta por timer. */
export function BgpPeersTab({ peers, bfdSessions }: Props) {
  return (
    <>
      {peers.length > 0 ? (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", margin: "0 0 10px" }}>
            <h3 style={{ margin: 0, fontSize: 15 }}>Peers BGP</h3>
            <ExportMenu
              filename="bgp-peers"
              title="Peers BGP"
              rows={peers}
              columns={[
                { header: "IP do peer", value: (p) => p.peer_ip },
                { header: "AS remoto", value: (p) => p.remote_as },
                { header: "Estado", value: (p) => p.state_label },
                { header: "Tempo estabelecido", value: (p) => formatDuration(p.established_seconds) },
                { header: "Updates in", value: (p) => p.in_updates },
                { header: "Updates out", value: (p) => p.out_updates },
                { header: "Prefixos recebidos", value: (p) => p.prefixes_received },
                { header: "Prefixos ativos", value: (p) => p.prefixes_active },
                { header: "Prefixos anunciados", value: (p) => p.prefixes_advertised },
              ]}
            />
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>IP do peer</th>
                  <th>AS remoto</th>
                  <th>Estado</th>
                  <th>Tempo estabelecido</th>
                  <th>Updates in</th>
                  <th>Updates out</th>
                  <th>Prefixos recebidos</th>
                  <th>Prefixos activos</th>
                  <th>Prefixos anunciados</th>
                </tr>
              </thead>
              <tbody>
                {peers.map((p) => (
                  <tr key={p.peer_ip}>
                    <td className="mono">{p.peer_ip}</td>
                    <td className="mono">{p.remote_as || EM_DASH}</td>
                    <td>{peerStateBadge(p.state_label)}</td>
                    <td>{formatDuration(p.established_seconds)}</td>
                    <td>{p.in_updates ?? EM_DASH}</td>
                    <td>{p.out_updates ?? EM_DASH}</td>
                    <td className="mono">{p.prefixes_received ?? EM_DASH}</td>
                    <td className="mono">{p.prefixes_active ?? EM_DASH}</td>
                    <td className="mono">{p.prefixes_advertised ?? EM_DASH}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="msg msg--warn">Sem peers BGP coletados.</div>
      )}

      <div className="card" style={{ padding: 14 }}>
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", margin: "0 0 4px" }}>
          <h3 style={{ margin: 0, fontSize: 15 }}>Sessões BFD</h3>
          <ExportMenu
            filename="bgp-sessoes-bfd"
            title="Sessões BFD"
            rows={bfdSessions}
            columns={[
              { header: "Peer", value: (s) => s.peer_addr },
              { header: "Interface", value: (s) => s.bind_if_name },
              { header: "Estado", value: (s) => s.state_label },
              { header: "Diagnóstico", value: (s) => s.diag },
              { header: "VRF", value: (s) => s.vpn_name },
              { header: "Motivo da queda", value: (s) => s.down_reason },
            ]}
          />
        </div>
        <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--muted)" }}>
          Detecção rápida de falha (HUAWEI-BFD-MIB) — cada sessão traz o motivo real da queda, quando aplicável.
        </p>
        {bfdSessions.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem sessões BFD activas neste perfil (métricas da secção "bfd" podem estar desligadas em Configurações → BGP).
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Peer</th>
                  <th>Interface</th>
                  <th>Estado</th>
                  <th>Diagnóstico</th>
                  <th>VRF</th>
                  <th>Motivo da queda</th>
                </tr>
              </thead>
              <tbody>
                {bfdSessions.map((s) => (
                  <tr key={s.sess_index}>
                    <td className="mono">{s.peer_addr || EM_DASH}</td>
                    <td className="mono">{s.bind_if_name || EM_DASH}</td>
                    <td>
                      <span className={`badge ${BFD_STATE_BADGE[s.state_label ?? ""] ?? "badge--off"}`}>
                        {s.state_label ?? EM_DASH}
                      </span>
                    </td>
                    <td>{s.diag ?? EM_DASH}</td>
                    <td>{s.vpn_name || EM_DASH}</td>
                    <td>{s.down_reason || EM_DASH}</td>
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
