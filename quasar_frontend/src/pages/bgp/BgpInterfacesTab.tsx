import { EM_DASH, ifaceStatusBadge, num } from "./bgpFormat";
import { formatBitrate } from "../../lib/formatBitrate";
import type { ETrunkMemberReport, ETrunkReport, InterfaceReport } from "./bgpTypes";

type Props = {
  interfaces: InterfaceReport[];
  etrunks: ETrunkReport[];
  etrunkMembers: ETrunkMemberReport[];
};

const ETRUNK_STATUS_BADGE: Record<string, string> = { master: "badge--ok", backup: "badge--warn", initialize: "badge--off" };

/** Aba "Interfaces & LAG" — tabela de interfaces já existente + E-Trunk (LAG entre
 * equipamentos, HUAWEI-E-TRUNK-MIB). O IF-MIB só sabe "up/down"; aqui aparece quem é
 * master/backup e o motivo real (BFD caiu, peer caiu, todos os membros caíram, etc.),
 * reportado pelo próprio equipamento. */
export function BgpInterfacesTab({ interfaces, etrunks, etrunkMembers }: Props) {
  const membersByParent = new Map<string, ETrunkMemberReport[]>();
  for (const m of etrunkMembers) {
    const list = membersByParent.get(m.parent_id) ?? [];
    list.push(m);
    membersByParent.set(m.parent_id, list);
  }

  return (
    <>
      {interfaces.length > 0 ? (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Interfaces</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Interface</th>
                  <th>Descrição</th>
                  <th>Estado</th>
                  <th>Download</th>
                  <th>Upload</th>
                </tr>
              </thead>
              <tbody>
                {interfaces.map((i) => (
                  <tr key={i.if_index}>
                    <td className="mono">{i.descr || i.if_index}</td>
                    <td>{i.alias || EM_DASH}</td>
                    <td>{ifaceStatusBadge(i.oper_status)}</td>
                    <td className="mono">{i.in_bit_rate ? formatBitrate(num(i.in_bit_rate)) : EM_DASH}</td>
                    <td className="mono">{i.out_bit_rate ? formatBitrate(num(i.out_bit_rate)) : EM_DASH}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="msg msg--warn">Sem interfaces coletadas.</div>
      )}

      <div className="card" style={{ padding: 14 }}>
        <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>E-Trunk (LAG entre equipamentos)</h3>
        <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--muted)" }}>
          Estado master/backup e o motivo real reportado pelo equipamento — não apenas "up/down".
        </p>
        {etrunks.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem E-Trunks configurados neste equipamento (ou métricas da secção "etrunk" desligadas em Configurações → BGP).
          </div>
        ) : (
          etrunks.map((e) => (
            <div key={e.etrunk_id} style={{ marginBottom: 14 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
                <strong style={{ fontSize: 13 }}>E-Trunk {e.etrunk_id}</strong>
                <span className={`badge ${ETRUNK_STATUS_BADGE[e.status_label ?? ""] ?? "badge--off"}`}>
                  {e.status_label ?? EM_DASH}
                </span>
                {e.status_reason && (
                  <span style={{ fontSize: 11, color: "var(--muted)" }}>Motivo: {e.status_reason}</span>
                )}
              </div>
              {(membersByParent.get(e.etrunk_id) ?? []).length > 0 && (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Membro</th>
                        <th>Estado</th>
                        <th>Motivo</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(membersByParent.get(e.etrunk_id) ?? []).map((m) => (
                        <tr key={`${m.parent_id}-${m.member_id}`}>
                          <td className="mono">{m.member_id}</td>
                          <td>
                            <span className={`badge ${ETRUNK_STATUS_BADGE[m.status_label ?? ""] ?? "badge--off"}`}>
                              {m.status_label ?? EM_DASH}
                            </span>
                          </td>
                          <td>{m.status_reason || EM_DASH}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </>
  );
}
