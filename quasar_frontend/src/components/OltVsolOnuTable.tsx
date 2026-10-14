import { useMemo } from "react";
import { ActionMenu } from "./ActionMenu";
import { ExportMenu } from "./ExportMenu";
import { EM_DASH, formatNullable, formatSnmpMetricCell } from "../lib/formatDisplay";
import { SortableTh, useTableSort, type SortValue } from "../lib/tableSort";
import { classifyOnuRx, ONU_RX_QUALITY_COLOR, ONU_RX_QUALITY_LABEL, useOnuRxThresholds } from "../lib/onuRxQuality";

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
  telnet_report_at?: string;
  data_source_telnet?: boolean;
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

/** dBm numérico da RX da ONU, preferindo o campo já parseado (rx_dbm) e caindo para o texto
 * (rx_pwr) — mesma prioridade de displayOnuRx acima, só que devolvendo o número em vez do texto
 * formatado, para poder classificar bom/aceitável/ruim (ver lib/onuRxQuality.ts). */
function onuRxDbmValue(u: VsOnuRow): number | null {
  if (typeof u.rx_dbm === "number" && Number.isFinite(u.rx_dbm) && u.rx_dbm <= 0) return u.rx_dbm;
  const n = Number.parseFloat(String(u.rx_pwr ?? "").replace(",", "."));
  return Number.isFinite(n) && n <= 0 ? n : null;
}

function rowHasText(u: VsOnuRow, key: keyof VsOnuRow): boolean {
  const v = u[key];
  return v != null && String(v).trim() !== "";
}

function telnetCliBadge(u: { data_source_telnet?: boolean; telnet_report_at?: string }) {
  if (!u.data_source_telnet) return null;
  return (
    <span
      className="badge badge--ok"
      style={{ marginLeft: 4, fontSize: 9, verticalAlign: "middle" }}
      title={u.telnet_report_at ? `Telnet ${u.telnet_report_at}` : "Dados via telnet CLI"}
    >
      CLI
    </span>
  );
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

/** Mesmo critério do selo de status, em texto (para exportação). */
function onlineText(u: VsOnuRow, rxStatusMode?: boolean): string {
  const useRx =
    rxStatusMode ||
    u.status_source === "rx_threshold" ||
    (typeof u.online === "boolean" && (u.rx_dbm != null || rowHasText(u, "rx_pwr")));
  if (useRx && typeof u.online === "boolean") return u.online ? "Online" : "Offline";
  const label = String(u.oper_status_label ?? "").trim().toLowerCase();
  const sta = u.oper_status ?? u.onu_online_sta;
  if (label === "up" || u.online === true || sta === 1) return "Online";
  if (label === "down" || u.online === false || sta === 2) return "Offline";
  if (label !== "") return label;
  if (sta === 3) return "Online";
  if (sta === 4 || (sta != null && sta !== 1 && sta !== 3 && sta >= 0)) return "Offline";
  return "";
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
  hasVlanResolver?: boolean,
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
  if (metricEnabled(enabled, "vlan") || any((u) => rowHasText(u, "vlan")) || hasVlanResolver) {
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
  /** Item "Histórico" no menu de 3 pontinhos de cada linha — ausente esconde a coluna de ações
   * inteira (ex.: quando o chamador ainda não sabe o device_id). */
  onShowHistory?: (pon: number, onu: number, label: string) => void;
  /** Resolve a VLAN de uma ONU pela PON dela + config do equipamento (pon_vlans no cadastro da
   * OLT). Quando dado, a coluna VLAN aparece mesmo que as linhas não tragam vlan própria. */
  vlanForPon?: (pon?: number) => string;
};

export function OltVsolOnuTable({ rows, note, onuRefs, enabledMetrics, rxStatusMode, offlineRxDbm, onShowHistory, vlanForPon }: Props) {
  const { thresholds: rxThresholds } = useOnuRxThresholds();
  const visible = useMemo(
    () => buildVisibleColumns(enabledMetrics, rows, rxStatusMode, !!vlanForPon),
    [enabledMetrics, rows, rxStatusMode, vlanForPon],
  );

  const { sort, toggle, sortRows } = useTableSort<ColKey | "pon" | "onu">();
  const baseRows = useMemo(() => {
    if (offlineRxDbm == null || !Number.isFinite(offlineRxDbm)) {
      return rows;
    }
    return rows.map((u) => ({
      ...u,
      offline_rx_dbm: u.offline_rx_dbm ?? offlineRxDbm,
    }));
  }, [rows, offlineRxDbm]);
  const displayRows = useMemo(
    () =>
      sortRows(baseRows, (u, key): SortValue => {
        const num = (s: unknown): number | null => {
          const n = Number.parseFloat(String(s ?? "").replace(",", "."));
          return Number.isFinite(n) ? n : null;
        };
        switch (key) {
          case "pon":
            return u.pon ?? null;
          case "onu":
            return u.onu ?? null;
          case "interface":
            return u.if_name ?? u.if_descr ?? null;
          case "status":
            return onlineText(u, rxStatusMode);
          case "phase":
            return u.phase_sta ?? null;
          case "rx":
            return onuRxDbmValue(u);
          case "tx":
            return num(u.tx_pwr);
          case "voltage":
            return num(u.voltage);
          case "temp":
            return num(u.temp);
          case "model":
            return u.model ?? null;
          case "serial":
            return u.serial ?? null;
          case "vlan":
            return String(u.vlan ?? "").trim() || vlanForPon?.(u.pon)?.trim() || null;
          default:
            return null;
        }
      }),
    [baseRows, sortRows, rxStatusMode, vlanForPon],
  );

  if (rows.length === 0) {
    const hint =
      note && note.includes("MIB SNMP")
        ? note
        : note && note.trim() !== ""
          ? note
          : "Nenhuma ONU encontrada. Verifique os OIDs em Configurações → OLT e se a OLT responde SNMP (IP e community).";
    return <p style={{ fontSize: 12, color: "var(--muted)" }}>{hint}</p>;
  }

  return (
    <>
      <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 6 }}>
        <ExportMenu
          filename="olt-onus"
          title="ONUs — status e telemetria"
          rows={displayRows}
          columns={[
            { header: "PON", value: (u) => cell(u.pon_compact ?? u.pon) },
            { header: "ONU", value: (u) => cell(u.onu) },
            ...(visible.has("interface") ? [{ header: "Interface", value: (u: VsOnuRow) => cell(u.if_name ?? u.if_descr) }] : []),
            ...(visible.has("status") ? [{ header: "Status", value: (u: VsOnuRow) => onlineText(u, rxStatusMode) }] : []),
            ...(visible.has("phase") ? [{ header: "Fase", value: (u: VsOnuRow) => cell(u.phase_sta) }] : []),
            ...(visible.has("rx") ? [{ header: "RX ONU", value: (u: VsOnuRow) => displayOnuRx(u) }] : []),
            ...(visible.has("tx") ? [{ header: "TX ONU", value: (u: VsOnuRow) => metricCell(u.tx_pwr) }] : []),
            ...(visible.has("voltage") ? [{ header: "Voltagem", value: (u: VsOnuRow) => metricCell(u.voltage) }] : []),
            ...(visible.has("temp") ? [{ header: "Temperatura", value: (u: VsOnuRow) => metricCell(u.temp) }] : []),
            ...(visible.has("model") ? [{ header: "Modelo da ONU", value: (u: VsOnuRow) => cell(u.model) }] : []),
            ...(visible.has("serial") ? [{ header: "Serial", value: (u: VsOnuRow) => cell(u.serial) }] : []),
            ...(visible.has("vlan") ? [{ header: "VLAN", value: (u: VsOnuRow) => String(u.vlan ?? "").trim() || vlanForPon?.(u.pon)?.trim() || "" }] : []),
          ]}
        />
      </div>
      <div className="table-wrap" style={{ maxHeight: 420, overflow: "auto" }}>
        <table className="table table--compact" style={{ width: "100%", fontSize: 12 }}>
          <thead>
            <tr>
              <SortableTh col="pon" sort={sort} onSort={toggle}>PON</SortableTh>
              <SortableTh col="onu" sort={sort} onSort={toggle}>ONU</SortableTh>
              {visible.has("interface") && <SortableTh col="interface" sort={sort} onSort={toggle}>Interface</SortableTh>}
              {visible.has("status") && <SortableTh col="status" sort={sort} onSort={toggle}>Status</SortableTh>}
              {visible.has("phase") && <SortableTh col="phase" sort={sort} onSort={toggle}>Fase</SortableTh>}
              {visible.has("rx") && <SortableTh col="rx" sort={sort} onSort={toggle} className="mono">RX ONU</SortableTh>}
              {visible.has("tx") && <SortableTh col="tx" sort={sort} onSort={toggle} className="mono">TX ONU</SortableTh>}
              {visible.has("voltage") && <SortableTh col="voltage" sort={sort} onSort={toggle} className="mono">Voltagem</SortableTh>}
              {visible.has("temp") && <SortableTh col="temp" sort={sort} onSort={toggle} className="mono">Temp.</SortableTh>}
              {visible.has("model") && <SortableTh col="model" sort={sort} onSort={toggle}>Modelo da ONU</SortableTh>}
              {visible.has("serial") && <SortableTh col="serial" sort={sort} onSort={toggle}>Serial</SortableTh>}
              {visible.has("vlan") && <SortableTh col="vlan" sort={sort} onSort={toggle} className="mono">VLAN</SortableTh>}
              {onShowHistory && <th aria-label="Ações" />}
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
                {visible.has("rx") &&
                  (() => {
                    const rxDbm = onuRxDbmValue(u);
                    const rxQuality = rxDbm != null ? classifyOnuRx(rxDbm, rxThresholds) : null;
                    return (
                      <td className="mono" style={rxQuality ? { color: ONU_RX_QUALITY_COLOR[rxQuality] } : undefined} title={rxQuality ? ONU_RX_QUALITY_LABEL[rxQuality] : undefined}>
                        {displayOnuRx(u)}
                        {telnetCliBadge(u)}
                      </td>
                    );
                  })()}
                {visible.has("tx") && (
                  <td className="mono">
                    {metricCell(u.tx_pwr)}
                    {telnetCliBadge(u)}
                  </td>
                )}
                {visible.has("voltage") && (
                  <td className="mono">
                    {metricCell(u.voltage)}
                    {telnetCliBadge(u)}
                  </td>
                )}
                {visible.has("temp") && (
                  <td className="mono">
                    {metricCell(u.temp)}
                    {telnetCliBadge(u)}
                  </td>
                )}
                {visible.has("model") && (
                  <td>
                    {cell(u.model)}
                    {u.data_source_telnet ? (
                      <span className="badge badge--ok" style={{ marginLeft: 6, fontSize: 9 }} title={u.telnet_report_at ? `Telnet ${u.telnet_report_at}` : "Dados telnet"}>
                        CLI
                      </span>
                    ) : null}
                  </td>
                )}
                {visible.has("serial") && (
                  <td className="mono" style={{ maxWidth: 140, wordBreak: "break-all" }}>
                    {cell(u.serial)}
                  </td>
                )}
                {visible.has("vlan") && (
                  <td className="mono">
                    {(() => {
                      const own = String(u.vlan ?? "").trim();
                      if (own) return own;
                      const byPon = vlanForPon?.(u.pon)?.trim();
                      return byPon ? (
                        <span title={`VLAN da PON ${u.pon} (cadastro do equipamento)`} style={{ color: "var(--muted)" }}>
                          {byPon}
                        </span>
                      ) : (
                        EM_DASH
                      );
                    })()}
                  </td>
                )}
                {onShowHistory && (
                  <td style={{ textAlign: "right" }}>
                    {u.pon != null && u.onu != null ? (
                      <ActionMenu
                        align="end"
                        title="Ações da ONU"
                        items={[
                          {
                            id: "history",
                            label: "Histórico",
                            onClick: () =>
                              onShowHistory(u.pon as number, u.onu as number, `PON ${cell(u.pon_compact ?? u.pon)} · ONU ${u.onu}`),
                          },
                        ]}
                      />
                    ) : null}
                  </td>
                )}
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
