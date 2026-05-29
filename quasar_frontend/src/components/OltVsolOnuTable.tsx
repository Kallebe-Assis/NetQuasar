import { useMemo } from "react";
import { EM_DASH, formatNullable, formatSnmpMetricCell } from "../lib/formatDisplay";

export type VsOnuRow = {
  pon?: number;
  onu?: number;
  pon_compact?: string;
  if_index?: number;
  if_name?: string;
  if_descr?: string;
  oper_status?: number;
  oper_status_label?: string;
  profile_name?: string;
  phase_sta?: string;
  online?: boolean;
  onu_online_sta?: number;
  status_source?: string;
  offline_rx_dbm?: number;
  rx_dbm?: number;
  temp?: string;
  voltage?: string;
  bias?: string;
  tx_pwr?: string;
  rx_pwr?: string;
  vendor?: string;
  version?: string;
  serial?: string;
  model?: string;
  vlan?: string;
};

function cell(v: unknown): string {
  if (typeof v === "number" && Number.isFinite(v)) return String(v);
  return formatNullable(v);
}

function metricCell(v: unknown): string {
  if (v == null || (typeof v === "string" && v.trim() === "")) return EM_DASH;
  return formatSnmpMetricCell(v);
}

function displayOnuRx(u: VsOnuRow): string {
  const dbm = u.rx_dbm;
  if (typeof dbm === "number" && Number.isFinite(dbm) && dbm <= 0) {
    return dbm.toFixed(2);
  }
  return metricCell(u.rx_pwr);
}

function rowHasText(u: VsOnuRow, key: keyof VsOnuRow): boolean {
  const v = u[key];
  return v != null && String(v).trim() !== "";
}

function onlineBadge(u: VsOnuRow, rxStatusMode?: boolean) {
  const useRx =
    rxStatusMode ||
    u.status_source === "rx_threshold" ||
    (typeof u.online === "boolean" && (u.rx_dbm != null || rowHasText(u, "rx_pwr")));

  if (useRx && typeof u.online === "boolean") {
    const th = u.offline_rx_dbm;
    const rx =
      typeof u.rx_dbm === "number" && Number.isFinite(u.rx_dbm)
        ? u.rx_dbm
        : Number.parseFloat(String(u.rx_pwr ?? "").replace(",", "."));
    const title =
      th != null && Number.isFinite(rx)
        ? `RX ${rx.toFixed(2)} dBm — ${u.online ? "online" : "offline"} (limiar ${th} dBm: ${u.online ? "≥" : "<"})`
        : u.online
          ? "Online (potência RX)"
          : "Offline (potência RX)";
    return u.online ? (
      <span className="badge badge--ok" title={title}>
        Online
      </span>
    ) : (
      <span className="badge badge--err" title={title}>
        Offline
      </span>
    );
  }

  const label = String(u.oper_status_label ?? "").trim().toLowerCase();
  const sta = u.oper_status ?? u.onu_online_sta;
  if (label === "up" || u.online === true || sta === 1) {
    return (
      <span className="badge badge--ok" title={sta != null ? `ifOperStatus ${sta}` : undefined}>
        {label === "up" ? `up (${sta ?? 1})` : "Online"}
      </span>
    );
  }
  if (label === "down" || u.online === false || sta === 2) {
    return (
      <span className="badge badge--err" title={sta != null ? `ifOperStatus ${sta}` : undefined}>
        {label === "down" ? `down (${sta ?? 2})` : "Offline"}
      </span>
    );
  }
  if (label !== "") {
    return (
      <span className="badge badge--off" title={sta != null ? `ifOperStatus ${sta}` : undefined}>
        {label}
        {sta != null ? ` (${sta})` : ""}
      </span>
    );
  }
  if (sta === 3) {
    return <span className="badge badge--ok">Online</span>;
  }
  if (sta === 4 || (sta != null && sta !== 1 && sta !== 3 && sta >= 0)) {
    return <span className="badge badge--err">Offline</span>;
  }
  return <span className="badge badge--off">{EM_DASH}</span>;
}

type ColKey = "interface" | "status" | "phase" | "rx" | "tx" | "voltage" | "temp" | "model" | "serial" | "vlan";

function metricEnabled(enabled: string[] | undefined, key: string): boolean {
  if (enabled === undefined) {
    return true;
  }
  if (enabled.length === 0) {
    return false;
  }
  return enabled.includes(key);
}

function buildVisibleColumns(
  enabled: string[] | undefined,
  rows: VsOnuRow[],
  rxStatusMode?: boolean,
): Set<ColKey> {
  const show = new Set<ColKey>();
  const any = (pred: (u: VsOnuRow) => boolean) => rows.some(pred);

  if (
    rxStatusMode ||
    metricEnabled(enabled, "status") ||
    metricEnabled(enabled, "rx_power") ||
    any((u) => typeof u.online === "boolean")
  ) {
    show.add("status");
  }
  if (metricEnabled(enabled, "rx_power") || any((u) => rowHasText(u, "rx_pwr") || u.rx_dbm != null)) {
    show.add("rx");
  }
  if (metricEnabled(enabled, "tx_power") || any((u) => rowHasText(u, "tx_pwr"))) {
    show.add("tx");
  }
  if (metricEnabled(enabled, "temperature") || any((u) => rowHasText(u, "temp"))) {
    show.add("temp");
  }
  if (metricEnabled(enabled, "model") || any((u) => rowHasText(u, "model"))) {
    show.add("model");
  }
  if (metricEnabled(enabled, "serial") || any((u) => rowHasText(u, "serial"))) {
    show.add("serial");
  }
  if (metricEnabled(enabled, "vlan") || any((u) => rowHasText(u, "vlan"))) {
    show.add("vlan");
  }
  if (any((u) => rowHasText(u, "if_name") || rowHasText(u, "if_descr"))) {
    show.add("interface");
  }
  if (any((u) => rowHasText(u, "phase_sta"))) {
    show.add("phase");
  }
  if (any((u) => rowHasText(u, "voltage"))) {
    show.add("voltage");
  }
  return show;
}

type Props = {
  rows: VsOnuRow[];
  note?: string;
  onuRefs?: number;
  enabledMetrics?: string[];
  rxStatusMode?: boolean;
  offlineRxDbm?: number;
};

export function OltVsolOnuTable({ rows, note, onuRefs, enabledMetrics, rxStatusMode, offlineRxDbm }: Props) {
  const visible = useMemo(
    () => buildVisibleColumns(enabledMetrics, rows, rxStatusMode),
    [enabledMetrics, rows, rxStatusMode],
  );

  const displayRows = useMemo(() => {
    if (offlineRxDbm == null || !Number.isFinite(offlineRxDbm)) {
      return rows;
    }
    return rows.map((u) => ({
      ...u,
      offline_rx_dbm: u.offline_rx_dbm ?? offlineRxDbm,
    }));
  }, [rows, offlineRxDbm]);

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
              {visible.has("interface") && <th>Interface</th>}
              {visible.has("status") && <th>Status</th>}
              {visible.has("phase") && <th>Fase</th>}
              {visible.has("rx") && <th className="mono">RX ONU</th>}
              {visible.has("tx") && <th className="mono">TX ONU</th>}
              {visible.has("voltage") && <th className="mono">Voltagem</th>}
              {visible.has("temp") && <th className="mono">Temp.</th>}
              {visible.has("model") && <th>Modelo da ONU</th>}
              {visible.has("serial") && <th>Serial</th>}
              {visible.has("vlan") && <th className="mono">VLAN</th>}
            </tr>
          </thead>
          <tbody>
            {displayRows.map((u, i) => (
              <tr key={`${u.pon}-${u.onu}-${i}`}>
                <td className="mono">{cell(u.pon_compact ?? u.pon)}</td>
                <td className="mono">{cell(u.onu)}</td>
                {visible.has("interface") && (
                  <td className="mono" style={{ maxWidth: 160, wordBreak: "break-all" }} title={u.if_descr}>
                    {cell(u.if_name ?? u.if_descr)}
                  </td>
                )}
                {visible.has("status") && <td>{onlineBadge(u, rxStatusMode)}</td>}
                {visible.has("phase") && <td>{cell(u.phase_sta)}</td>}
                {visible.has("rx") && (
                  <td className="mono">{displayOnuRx(u)}</td>
                )}
                {visible.has("tx") && <td className="mono">{metricCell(u.tx_pwr)}</td>}
                {visible.has("voltage") && <td className="mono">{metricCell(u.voltage)}</td>}
                {visible.has("temp") && <td className="mono">{metricCell(u.temp)}</td>}
                {visible.has("model") && <td>{cell(u.model)}</td>}
                {visible.has("serial") && (
                  <td className="mono" style={{ maxWidth: 140, wordBreak: "break-all" }}>
                    {cell(u.serial)}
                  </td>
                )}
                {visible.has("vlan") && <td className="mono">{cell(u.vlan)}</td>}
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
      {onuRefs != null && onuRefs > 0 ? (
        <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }}>Referências IF-MIB: {onuRefs}</p>
      ) : null}
    </>
  );
}
