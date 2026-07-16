import { Activity, ChevronDown, ChevronUp } from "lucide-react";
import { Fragment, type ReactNode } from "react";
import { InfoHint } from "../../components/InfoHint";

/** Meta de linha na tabela de métricas/OIDs (OLT, BNG, MikroTik, Switch). */
export type MetricsOidFieldMeta = {
  key: string;
  label: string;
  shortDesc: string;
  hint: string;
  placeholder: string;
  entity: string;
  unit: string;
  typeLabel: string;
  /** Mostra botão de expandir (opções avançadas). */
  expandable?: boolean;
  /** @deprecated use expandable — compat OLT */
  hasStatusValues?: boolean;
  hasStatusMode?: boolean;
  supportsDivisor?: boolean;
};

/** Alias legado usado pelo Perfil OLT. */
export type OltMetricFieldMeta = MetricsOidFieldMeta & {
  entity: "ONU" | "PON" | string;
  typeLabel: "Gauge" | "Integer" | "String" | "Status" | string;
};

type MetricRow = {
  enabled?: boolean;
  oid?: string;
};

type Props = {
  title: string;
  description: string;
  fields: MetricsOidFieldMeta[];
  metrics: Record<string, MetricRow | undefined>;
  expandedKey: string | null;
  onToggleExpand: (key: string) => void;
  onToggleEnabled: (key: string, enabled: boolean) => void;
  onOidChange: (key: string, oid: string) => void;
  renderExpanded?: (field: MetricsOidFieldMeta) => ReactNode;
  /** Cabeçalho da coluna de valor (default: OID). */
  oidColumnLabel?: string;
  /** Cabeçalho da coluna entidade (default: Entidade). */
  entityColumnLabel?: string;
  /** Prefixo único para ids dos switches (evitar colisão com vários painéis). */
  idPrefix?: string;
  /** Se true, métrica activa por omissão quando `enabled` está undefined (OLT). Default false = precisa enabled===true. */
  defaultEnabled?: boolean;
};

function typeBadgeClass(t: string): string {
  switch (t) {
    case "Gauge":
      return "olt-metrics-badge olt-metrics-badge--gauge";
    case "Integer":
      return "olt-metrics-badge olt-metrics-badge--int";
    case "Status":
      return "olt-metrics-badge olt-metrics-badge--status";
    case "Walk":
    case "GET":
      return "olt-metrics-badge olt-metrics-badge--gauge";
    default:
      return "olt-metrics-badge olt-metrics-badge--str";
  }
}

function fieldExpandable(field: MetricsOidFieldMeta, hasRenderer: boolean): boolean {
  if (!hasRenderer) return false;
  if (field.expandable) return true;
  return Boolean(field.hasStatusValues || field.hasStatusMode || field.supportsDivisor);
}

/** Tabela de OIDs/métricas no estilo do painel de perfil OLT. */
export function OltMetricsOidTable({
  title,
  description,
  fields,
  metrics,
  expandedKey,
  onToggleExpand,
  onToggleEnabled,
  onOidChange,
  renderExpanded,
  oidColumnLabel = "OID",
  entityColumnLabel = "Entidade",
  idPrefix = "olt-metric",
  defaultEnabled = true,
}: Props) {
  return (
    <div className="olt-metrics-panel">
      <div className="olt-metrics-panel__head">
        <div>
          <h3
            className="olt-profile-modal__section-title"
            style={{ margin: 0, display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            {title}
            <InfoHint label={title} className="info-hint--subtle">
              {description}
            </InfoHint>
          </h3>
          <p className="olt-metrics-panel__desc">{description}</p>
        </div>
      </div>

      <div className="table-wrap olt-metrics-table-wrap">
        <table className="olt-metrics-table">
          <thead>
            <tr>
              <th>Métrica</th>
              <th>{entityColumnLabel}</th>
              <th>{oidColumnLabel}</th>
              <th>Unidade</th>
              <th>Tipo</th>
              <th style={{ width: 88 }}>Ativo</th>
              <th style={{ width: 56 }} />
            </tr>
          </thead>
          <tbody>
            {fields.map((field) => {
              const m = metrics[field.key] ?? {};
              const enabled = defaultEnabled ? m.enabled !== false : m.enabled === true;
              const expanded = expandedKey === field.key;
              const canExpand = fieldExpandable(field, Boolean(renderExpanded));
              return (
                <Fragment key={field.key}>
                  <tr className={enabled ? undefined : "olt-metrics-table__row--off"}>
                    <td>
                      <div className="olt-metrics-table__metric">
                        <span className="olt-metrics-table__icon" aria-hidden>
                          <Activity size={14} strokeWidth={2} />
                        </span>
                        <div>
                          <div className="olt-metrics-table__name">
                            <span>{field.label}</span>
                            <InfoHint label={`Sobre ${field.label}`} className="info-hint--subtle">
                              {field.hint}
                            </InfoHint>
                          </div>
                          <div className="olt-metrics-table__short">{field.shortDesc}</div>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span className="olt-metrics-entity">{field.entity}</span>
                    </td>
                    <td>
                      <div className="olt-metrics-table__oid-cell">
                        <input
                          className="input mono olt-metrics-table__oid"
                          placeholder={field.placeholder || `${oidColumnLabel}…`}
                          value={m.oid ?? ""}
                          disabled={!enabled}
                          onChange={(e) => onOidChange(field.key, e.target.value)}
                          aria-label={`${oidColumnLabel} de ${field.label}`}
                        />
                        <InfoHint label={`${oidColumnLabel} — ${field.label}`} className="info-hint--subtle">
                          {field.hint}
                        </InfoHint>
                      </div>
                    </td>
                    <td className="olt-metrics-table__unit">{field.unit || "—"}</td>
                    <td>
                      <span className={typeBadgeClass(field.typeLabel)}>{field.typeLabel}</span>
                    </td>
                    <td>
                      <label className="toggle olt-metrics-table__switch" htmlFor={`${idPrefix}-on-${field.key}`}>
                        <span className="toggle__track">
                          <input
                            id={`${idPrefix}-on-${field.key}`}
                            type="checkbox"
                            role="switch"
                            className="toggle__input"
                            checked={enabled}
                            onChange={(e) => onToggleEnabled(field.key, e.target.checked)}
                            aria-label={`${enabled ? "Desativar" : "Ativar"} ${field.label}`}
                          />
                          <span className="toggle__thumb" aria-hidden />
                        </span>
                      </label>
                    </td>
                    <td>
                      {canExpand ? (
                        <button
                          type="button"
                          className="btn btn--sm"
                          style={{ padding: "4px 8px" }}
                          title={expanded ? "Fechar opções" : "Opções avançadas"}
                          onClick={() => onToggleExpand(field.key)}
                        >
                          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                        </button>
                      ) : null}
                    </td>
                  </tr>
                  {expanded && canExpand && renderExpanded ? (
                    <tr className="olt-metrics-table__expand">
                      <td colSpan={7}>
                        <div className="olt-metrics-table__expand-body">{renderExpanded(field)}</div>
                      </td>
                    </tr>
                  ) : null}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
