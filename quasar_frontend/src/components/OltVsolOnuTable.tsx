import { EM_DASH, formatNullable } from "../lib/formatDisplay";

export type VsOnuRow = {
  pon?: number;
  onu?: number;
  profile_name?: string;
  phase_sta?: string;
  online?: boolean;
  onu_online_sta?: number;
  temp?: string;
  voltage?: string;
  bias?: string;
  tx_pwr?: string;
  rx_pwr?: string;
  vendor?: string;
  version?: string;
  serial?: string;
  model?: string;
};

function cell(v: unknown): string {
  if (typeof v === "number" && Number.isFinite(v)) return String(v);
  return formatNullable(v);
}

function onlineBadge(u: VsOnuRow) {
  if (u.online === true) {
    return <span className="badge badge--ok">Online</span>;
  }
  if (u.online === false) {
    return <span className="badge badge--err">Offline</span>;
  }
  const sta = u.onu_online_sta;
  if (sta === 1 || sta === 3) {
    return <span className="badge badge--ok">Online</span>;
  }
  if (sta === 4 || (sta != null && sta !== 1 && sta !== 3 && sta >= 0)) {
    return <span className="badge badge--err">Offline</span>;
  }
  return <span className="badge badge--off">{EM_DASH}</span>;
}

type Props = {
  rows: VsOnuRow[];
  note?: string;
  onuRefs?: number;
};

export function OltVsolOnuTable({ rows, note, onuRefs }: Props) {
  if (rows.length === 0) {
    const hint =
      note && note.includes("MIB SNMP")
        ? note
        : note && note.trim() !== ""
          ? note
          : "Nenhuma ONU encontrada. Verifique os OIDs em Configurações → Perfis OLT e se a OLT responde SNMP (IP e community).";
    return <p style={{ fontSize: 12, color: "var(--muted)" }}>{hint}</p>;
  }
  return (
    <>
      <div className="table-wrap" style={{ maxHeight: 420, overflow: "auto" }}>
        <table className="table table--compact" style={{ width: "100%", fontSize: 12 }}>
          <thead>
            <tr>
              <th>PON</th>
              <th>ONU</th>
              <th>Estado</th>
              <th>Fase</th>
              <th className="mono">RX ONU</th>
              <th className="mono">TX ONU</th>
              <th className="mono">Voltagem</th>
              <th className="mono">Temp.</th>
              <th>Modelo</th>
              <th>Serial</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((u, i) => (
              <tr key={`${u.pon}-${u.onu}-${i}`}>
                <td className="mono">{cell(u.pon)}</td>
                <td className="mono">{cell(u.onu)}</td>
                <td>{onlineBadge(u)}</td>
                <td>{cell(u.phase_sta)}</td>
                <td className="mono">{cell(u.rx_pwr)}</td>
                <td className="mono">{cell(u.tx_pwr)}</td>
                <td className="mono">{cell(u.voltage)}</td>
                <td className="mono">{cell(u.temp)}</td>
                <td>{cell(u.model)}</td>
                <td className="mono" style={{ maxWidth: 140, wordBreak: "break-all" }}>
                  {cell(u.serial)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {note ? (
        <p className="msg msg--off" style={{ fontSize: 11, marginTop: 8 }}>
          {note}
        </p>
      ) : null}
    </>
  );
}
