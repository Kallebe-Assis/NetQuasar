import { useMemo } from "react";
import { EM_DASH, formatDbm } from "../lib/formatDisplay";
import { formatBytes as formatDataBytes } from "../lib/formatBytes";
import {
  buildMikrotikMetricCards,
  sectionLabel,
  stripPPPoEName,
  type MikrotikCatalogEntry,
  type MikrotikMetricCard,
  type MikrotikMetricConfig,
} from "../lib/mikrotikMetricsDisplay";

type Props = {
  metrics?: Record<string, unknown>;
  catalog: MikrotikCatalogEntry[];
  config: Record<string, MikrotikMetricConfig>;
  sectionLabels: Record<string, string>;
  deviceLabel: string;
  deviceModel?: string | null;
  deviceIp?: string | null;
  collectedAt?: string;
  formatCollectedAt: (iso?: string) => string;
  ifaceUp: number;
  ifaceDown: number;
  ifaceTotal: number;
};

function MetricScalar({ card }: { card: Extract<MikrotikMetricCard, { kind: "scalar" }> }) {
  return (
    <div className="mk-metric-scalar">
      <span className="mk-metric-label">{card.label}</span>
      <span className={`mk-metric-value mono ${card.ok ? "" : "mk-metric-value--err"}`}>
        {card.ok ? card.value : card.error || "Falhou"}
      </span>
    </div>
  );
}

function MetricOptical({ card }: { card: Extract<MikrotikMetricCard, { kind: "optical" }> }) {
  if (card.ports.length === 0) {
    return <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Nenhuma porta SFP detectada.</p>;
  }
  return (
    <div className="table-wrap" style={{ maxHeight: 280, overflow: "auto" }}>
      <table style={{ fontSize: 11, width: "100%" }}>
        <thead>
          <tr>
            <th>Porta</th>
            <th className="mono">RX</th>
            <th className="mono">TX</th>
            <th className="mono">Temp</th>
            <th className="mono">V</th>
            <th className="mono">Bias</th>
          </tr>
        </thead>
        <tbody>
          {card.ports.map((p, i) => (
            <tr key={`${p.index ?? i}-${p.name ?? ""}`}>
              <td>{String(p.name ?? `idx ${p.index ?? "?"}`)}</td>
              <td className="mono">{formatDbm(p.rx_dbm as number | undefined)}</td>
              <td className="mono">{formatDbm(p.tx_dbm as number | undefined)}</td>
              <td className="mono">
                {p.temperature_c != null ? `${Number(p.temperature_c).toFixed(1)} °C` : EM_DASH}
              </td>
              <td className="mono">
                {p.supply_voltage_v != null ? `${Number(p.supply_voltage_v).toFixed(3)} V` : EM_DASH}
              </td>
              <td className="mono">
                {p.bias_current_ma != null ? `${Number(p.bias_current_ma).toFixed(0)} mA` : EM_DASH}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function MetricPPPoE({ card }: { card: Extract<MikrotikMetricCard, { kind: "pppoe" }> }) {
  if (card.sessions.length === 0) {
    return (
      <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>
        Nenhuma sessão PPPoE activa no IF-MIB (clientes offline não aparecem aqui).
      </p>
    );
  }
  return (
    <>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
        <strong>{card.sessions.length}</strong> sessão(ões) ligada(s) — filtro ifDescr «pppoe».
      </p>
      <div className="table-wrap" style={{ maxHeight: 320, overflow: "auto" }}>
        <table style={{ fontSize: 11, width: "100%" }}>
          <thead>
            <tr>
              <th>Idx</th>
              <th>Cliente</th>
              <th>Status</th>
              <th className="mono">RX acum.</th>
              <th className="mono">TX acum.</th>
            </tr>
          </thead>
          <tbody>
            {card.sessions.map((s) => (
              <tr key={String(s.if_index)}>
                <td className="mono">{String(s.if_index)}</td>
                <td style={{ wordBreak: "break-word" }}>{stripPPPoEName(String(s.name ?? ""))}</td>
                <td>
                  <span
                    className={
                      String(s.oper_status_label) === "up"
                        ? "badge badge--ok"
                        : String(s.oper_status_label) === "down"
                          ? "badge badge--err"
                          : "badge badge--off"
                    }
                    style={{ fontSize: 9 }}
                  >
                    {String(s.oper_status_label ?? s.oper_status ?? "—").toUpperCase()}
                  </span>
                </td>
                <td className="mono">{formatDataBytes(Number(s.in_octets ?? 0))}</td>
                <td className="mono">{formatDataBytes(Number(s.out_octets ?? 0))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function MetricInterfaceStatus({ card }: { card: Extract<MikrotikMetricCard, { kind: "interface_status" }> }) {
  const show = card.rows.slice(0, 40);
  const more = card.rows.length - show.length;
  return (
    <>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
        {card.rows.length} interface(s)
      </p>
      <div className="table-wrap" style={{ maxHeight: 240, overflow: "auto" }}>
        <table style={{ fontSize: 11, width: "100%" }}>
          <thead>
            <tr>
              <th>Idx</th>
              <th>Oper</th>
              <th>Admin</th>
            </tr>
          </thead>
          <tbody>
            {show.map((r) => (
              <tr key={String(r.if_index)}>
                <td className="mono">{String(r.if_index)}</td>
                <td>{String(r.oper_status_label ?? r.oper_status ?? "—")}</td>
                <td>{String(r.admin_status_label ?? r.admin_status ?? "—")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {more > 0 && (
        <p style={{ fontSize: 10, color: "var(--muted)", margin: "6px 0 0" }}>+ {more} não mostradas</p>
      )}
    </>
  );
}

function MetricCardBody({ card }: { card: MikrotikMetricCard }) {
  switch (card.kind) {
    case "scalar":
      return <MetricScalar card={card} />;
    case "optical":
      return <MetricOptical card={card} />;
    case "pppoe":
      return <MetricPPPoE card={card} />;
    case "interface_status":
      return <MetricInterfaceStatus card={card} />;
    case "count":
      return (
        <div className="mk-metric-scalar">
          <span className="mk-metric-label">{card.label}</span>
          <span className="mk-metric-value mono">
            {card.count.toLocaleString("pt-BR")}
            {card.note ? <span style={{ fontSize: 10, color: "var(--muted)", marginLeft: 6 }}>{card.note}</span> : null}
          </span>
        </div>
      );
    default:
      return null;
  }
}

export function MikrotikMetricsOverview(props: Props) {
  const cards = useMemo(
    () =>
      buildMikrotikMetricCards({
        metrics: props.metrics,
        catalog: props.catalog,
        config: props.config,
      }),
    [props.metrics, props.catalog, props.config],
  );

  const bySection = useMemo(() => {
    const map = new Map<string, MikrotikMetricCard[]>();
    for (const c of cards) {
      const list = map.get(c.section) ?? [];
      list.push(c);
      map.set(c.section, list);
    }
    return map;
  }, [cards]);

  if (cards.length === 0) {
    return (
      <div className="msg" style={{ fontSize: 13 }}>
        Nenhuma métrica activa com dados na última telemetria. Active campos em{" "}
        <strong>Configurações → MikroTik</strong> e execute <strong>Atualizar telemetria</strong>.
      </div>
    );
  }

  return (
    <>
      <style>{`
        .mk-overview-grid {
          display: grid;
          grid-template-columns: repeat(2, minmax(0, 1fr));
          gap: 12px;
          margin-bottom: 12px;
        }
        @media (max-width: 960px) {
          .mk-overview-grid { grid-template-columns: 1fr; }
        }
        .mk-panel {
          background: var(--surface-2, rgba(0,0,0,0.04));
          border: 1px solid var(--border);
          border-radius: 8px;
          padding: 12px 14px;
          min-width: 0;
        }
        .mk-panel h3 {
          margin: 0 0 10px;
          font-size: 13px;
          font-weight: 600;
          color: var(--text);
        }
        .mk-metric-scalar {
          display: flex;
          justify-content: space-between;
          align-items: baseline;
          gap: 8px;
          padding: 6px 0;
          border-bottom: 1px solid var(--border);
          font-size: 12px;
        }
        .mk-metric-scalar:last-child { border-bottom: none; }
        .mk-metric-label { color: var(--muted); flex: 1; min-width: 0; }
        .mk-metric-value { font-weight: 600; text-align: right; }
        .mk-metric-value--err { color: var(--err, #dc2626); font-weight: 500; }
        .mk-device-strip {
          display: grid;
          grid-template-columns: repeat(2, minmax(0, 1fr));
          gap: 8px;
          margin-bottom: 12px;
        }
        .mk-device-kpi {
          padding: 10px 12px;
          border-radius: 8px;
          background: var(--surface-2, rgba(0,0,0,0.04));
          border: 1px solid var(--border);
        }
        .mk-device-kpi strong { display: block; font-size: 11px; color: var(--muted); margin-bottom: 4px; }
        .mk-device-kpi span { font-size: 15px; font-weight: 600; }
      `}</style>

      <div className="mk-device-strip">
        <div className="mk-device-kpi">
          <strong>Equipamento</strong>
          <span>{props.deviceLabel}</span>
        </div>
        <div className="mk-device-kpi">
          <strong>Modelo / IP</strong>
          <span className="mono" style={{ fontSize: 13 }}>
            {[props.deviceModel, props.deviceIp].filter(Boolean).join(" · ") || EM_DASH}
          </span>
        </div>
        <div className="mk-device-kpi">
          <strong>Interfaces (snapshot)</strong>
          <span>
            <span style={{ color: "var(--ok)" }}>{props.ifaceUp} UP</span>
            {" · "}
            <span style={{ color: "var(--err)" }}>{props.ifaceDown} DOWN</span>
            {" · "}
            {props.ifaceTotal} total
          </span>
        </div>
        <div className="mk-device-kpi">
          <strong>Últ. telemetria</strong>
          <span className="mono" style={{ fontSize: 12 }}>
            {props.formatCollectedAt(props.collectedAt)}
          </span>
        </div>
      </div>

      <div className="mk-overview-grid">
        {Array.from(bySection.entries()).map(([section, sectionCards]) => (
          <div key={section} className="mk-panel" style={section === "ppp" || section === "optical" ? { gridColumn: "1 / -1" } : undefined}>
            <h3>{sectionLabel(section, props.sectionLabels)}</h3>
            {sectionCards.map((card) => (
              <div key={card.key} style={{ marginBottom: sectionCards.length > 1 ? 14 : 0 }}>
                {sectionCards.length > 1 && card.kind !== "scalar" ? (
                  <p style={{ fontSize: 12, fontWeight: 600, margin: "0 0 6px" }}>{card.label}</p>
                ) : null}
                <MetricCardBody card={card} />
              </div>
            ))}
          </div>
        ))}
      </div>
    </>
  );
}
