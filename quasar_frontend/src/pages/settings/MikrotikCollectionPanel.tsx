import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useState,
} from "react";
import { ChevronDown, ChevronRight, Save } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { apiFetch } from "../../lib/api";
import { OltMetricsOidTable, type MetricsOidFieldMeta } from "./OltMetricsOidTable";

type MikrotikMetricDef = {
  enabled?: boolean;
  oid?: string;
  collect_mode?: string;
  value_divisor?: number;
};

type MikrotikMetricsForm = Record<string, MikrotikMetricDef>;

type CatalogEntry = {
  key: string;
  section: string;
  label: string;
  description: string;
  placeholder: string;
  collect_modes: string[];
  default_mode: string;
  walk_target?: string;
  unit?: string;
  default_divisor?: number;
  show_divisor?: boolean;
  optical_column?: number;
};

type MikrotikCollectionStep = {
  id?: string;
  method: string;
  enabled?: boolean;
  oid?: string;
  store_as?: string;
};

type MikrotikCollectionResponse = {
  metrics: MikrotikMetricsForm;
  collection_steps?: MikrotikCollectionStep[];
  catalog: CatalogEntry[];
  sections: Record<string, string>;
  collect_mode_labels: Record<string, string>;
};

export const MIKROTIK_SNMP_SECTION_ORDER = [
  "system",
  "health",
  "interfaces",
  "optical",
  "wireless",
  "ppp",
  "users",
  "dhcp",
  "ip",
] as const;

const SECTION_ORDER = MIKROTIK_SNMP_SECTION_ORDER;

const COLLECT_METHODS = [
  { value: "snmp_get", label: "SNMP GET (escalar)" },
  { value: "snmp_walk", label: "SNMP WALK (tabela)" },
  { value: "if_mib_table", label: "IF-MIB (walk tabela)" },
  { value: "if_mib_status", label: "IF-MIB status (parse)" },
  { value: "if_mib_pppoe", label: "PPPoE activo (IF-MIB)" },
  { value: "optical_sfp_table", label: "Tabela SFP parseada" },
  { value: "optical_sfp_column", label: "Coluna SFP (derivada)" },
];

function defaultMetricsForm(catalog: CatalogEntry[]): MikrotikMetricsForm {
  const out: MikrotikMetricsForm = {};
  for (const e of catalog) {
    out[e.key] = {
      enabled: false,
      oid: e.placeholder,
      collect_mode: e.default_mode || "snmp_get",
      value_divisor: e.default_divisor && e.default_divisor > 1 ? e.default_divisor : undefined,
    };
  }
  return out;
}

function mergeMetricsFromApi(raw: MikrotikMetricsForm | undefined, catalog: CatalogEntry[]): MikrotikMetricsForm {
  const base = defaultMetricsForm(catalog);
  if (!raw) return base;
  for (const e of catalog) {
    const m = raw[e.key];
    if (m) {
      base[e.key] = {
        enabled: m.enabled ?? base[e.key]?.enabled,
        oid: m.oid ?? base[e.key]?.oid ?? e.placeholder,
        collect_mode: m.collect_mode ?? base[e.key]?.collect_mode ?? e.default_mode,
        value_divisor:
          m.value_divisor ??
          (e.default_divisor && e.default_divisor > 1 ? e.default_divisor : base[e.key]?.value_divisor),
      };
    }
  }
  return base;
}

function countEnabled(metrics: MikrotikMetricsForm, catalog: CatalogEntry[]) {
  let enabled = 0;
  let missingOid = 0;
  for (const e of catalog) {
    const m = metrics[e.key];
    if (m?.enabled) {
      enabled++;
      if (!m.oid?.trim()) missingOid++;
    }
  }
  return { enabled, missingOid };
}

export type MikrotikCollectionHandle = {
  save: () => void;
  isPending: boolean;
  reloadFromServer: () => void;
};

type Props = {
  embedded?: boolean;
  variant?: "page" | "modal";
  activeSection?: string;
  apiPath?: string;
  queryKey?: string;
  saveSuccessMessage?: string;
  loadingLabel?: string;
  onSaved?: () => void;
  onPendingChange?: (pending: boolean) => void;
};

function collectModeTypeLabel(mode: string): string {
  const m = mode.toLowerCase();
  if (m.includes("walk") || m.includes("table") || m.includes("column") || m.includes("pppoe")) return "Walk";
  if (m.includes("get") || m.includes("status")) return "GET";
  return "SNMP";
}

function MetricFieldsGrid({
  fields,
  metrics,
  modeLabels,
  onSetMetric,
  title,
  description,
  entity,
  expandedKey,
  onToggleExpand,
  idPrefix = "mk-metric",
}: {
  fields: CatalogEntry[];
  metrics: MikrotikMetricsForm;
  modeLabels: Record<string, string>;
  onSetMetric: (key: string, patch: Partial<MikrotikMetricDef>) => void;
  title: string;
  description: string;
  entity: string;
  expandedKey: string | null;
  onToggleExpand: (key: string) => void;
  idPrefix?: string;
}) {
  const oidFields: MetricsOidFieldMeta[] = fields.map((f) => ({
    key: f.key,
    label: f.label,
    shortDesc: f.description.slice(0, 80) + (f.description.length > 80 ? "…" : ""),
    hint: f.description,
    placeholder: f.placeholder,
    entity,
    unit: f.unit || "—",
    typeLabel: collectModeTypeLabel(f.default_mode || "snmp_get"),
    expandable: true,
    supportsDivisor: Boolean(f.show_divisor || (f.default_divisor && f.default_divisor > 1)),
  }));
  const tableMetrics: Record<string, { enabled?: boolean; oid?: string }> = {};
  for (const f of fields) {
    tableMetrics[f.key] = { enabled: metrics[f.key]?.enabled, oid: metrics[f.key]?.oid };
  }
  return (
    <OltMetricsOidTable
      title={title}
      description={description}
      fields={oidFields}
      metrics={tableMetrics}
      expandedKey={expandedKey}
      onToggleExpand={onToggleExpand}
      onToggleEnabled={(key, enabled) => onSetMetric(key, { enabled })}
      onOidChange={(key, oid) => onSetMetric(key, { oid })}
      idPrefix={idPrefix}
      defaultEnabled={false}
      renderExpanded={(field) => {
        const cat = fields.find((f) => f.key === field.key);
        if (!cat) return null;
        const m = metrics[field.key] ?? {};
        if (m.enabled !== true) {
          return (
            <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>
              Active a métrica para configurar tipo de coleta e divisor.
            </p>
          );
        }
        const modes = cat.collect_modes?.length ? cat.collect_modes : ["snmp_get", "snmp_walk"];
        return (
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div className="field" style={{ margin: 0, maxWidth: 360 }}>
              <label style={{ fontSize: 11 }}>Tipo de coleta</label>
              <select
                className="input"
                style={{ fontSize: 12, padding: "4px 8px" }}
                value={m.collect_mode ?? cat.default_mode}
                onChange={(e) => onSetMetric(field.key, { collect_mode: e.target.value })}
              >
                {modes.map((mode) => (
                  <option key={mode} value={mode}>
                    {modeLabels[mode] || COLLECT_METHODS.find((x) => x.value === mode)?.label || mode}
                  </option>
                ))}
              </select>
            </div>
            {(cat.show_divisor || cat.default_divisor) && (
              <div className="field" style={{ margin: 0, maxWidth: 200 }}>
                <label style={{ fontSize: 11 }}>
                  Divisor de saída
                  {cat.default_divisor && cat.default_divisor > 1 ? (
                    <span style={{ color: "var(--muted)", fontWeight: 400 }}> (padrão: {cat.default_divisor})</span>
                  ) : null}
                </label>
                <input
                  className="input"
                  type="number"
                  min={1}
                  step={1}
                  style={{ fontSize: 12 }}
                  placeholder={cat.default_divisor ? String(cat.default_divisor) : "1"}
                  value={m.value_divisor ?? ""}
                  onChange={(e) => {
                    const v = e.target.value.trim();
                    onSetMetric(field.key, {
                      value_divisor: v === "" ? undefined : Math.max(1, parseInt(v, 10) || 1),
                    });
                  }}
                />
              </div>
            )}
            {(cat.walk_target === "interfaces" || cat.section === "optical") && (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
                {cat.section === "optical"
                  ? cat.key === "optical_table"
                    ? "Use «Tabela SFP parseada» para um walk único com RX/TX/temp/voltagem por porta."
                    : "«Coluna SFP (derivada)» usa a tabela completa; «SNMP WALK» coleta só esta coluna."
                  : cat.key === "if_oper_status" || cat.key === "if_admin_status"
                    ? "«IF-MIB status (parse)» devolve up/down por ifIndex; «SNMP WALK» devolve vars brutos."
                    : "Destino: snapshot de interfaces (ciclo IF-MIB)"}
              </p>
            )}
          </div>
        );
      }}
    />
  );
}

function AdvancedStepsEditor({
  steps,
  setSteps,
}: {
  steps: MikrotikCollectionStep[];
  setSteps: (steps: MikrotikCollectionStep[]) => void;
}) {
  return (
    <>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
        Walks ou GETs adicionais além do catálogo. Cada passo activo exige OID.
      </p>
      {steps.map((step, idx) => (
        <div key={step.id || idx} className="card" style={{ marginBottom: 8, padding: 10 }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr auto", gap: 8, alignItems: "end" }}>
            <div className="field" style={{ margin: 0 }}>
              <label style={{ fontSize: 11 }}>Método</label>
              <select
                className="input"
                value={step.method}
                onChange={(e) => {
                  const next = [...steps];
                  next[idx] = { ...step, method: e.target.value };
                  setSteps(next);
                }}
              >
                <option value="snmp_walk">SNMP WALK</option>
                <option value="snmp_get">SNMP GET</option>
              </select>
            </div>
            <div className="field" style={{ margin: 0 }}>
              <label style={{ fontSize: 11 }}>OID</label>
              <input
                className="input"
                style={{ fontFamily: "monospace", fontSize: 12 }}
                value={step.oid ?? ""}
                onChange={(e) => {
                  const next = [...steps];
                  next[idx] = { ...step, oid: e.target.value };
                  setSteps(next);
                }}
              />
            </div>
            <button
              type="button"
              className="btn btn--ghost"
              onClick={() => setSteps(steps.filter((_, i) => i !== idx))}
            >
              Remover
            </button>
          </div>
        </div>
      ))}
      <button
        type="button"
        className="btn btn--ghost"
        style={{ fontSize: 12 }}
        onClick={() =>
          setSteps([...steps, { id: `step_${Date.now()}`, method: "snmp_walk", enabled: true, oid: "" }])
        }
      >
        + Adicionar passo
      </button>
    </>
  );
}

export const MikrotikCollectionPanel = forwardRef<MikrotikCollectionHandle, Props>(function MikrotikCollectionPanel(
  {
    embedded,
    variant = "page",
    activeSection,
    apiPath = "/api/v1/settings/mikrotik-collection",
    queryKey = "mikrotik-collection",
    saveSuccessMessage = "Perfil MikroTik guardado.",
    loadingLabel = "A carregar perfil MikroTik…",
    onSaved,
    onPendingChange,
  },
  ref,
) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [metrics, setMetrics] = useState<MikrotikMetricsForm>({});
  const [steps, setSteps] = useState<MikrotikCollectionStep[]>([]);
  const [openSections, setOpenSections] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(SECTION_ORDER.map((s) => [s, false])),
  );
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [expandedMetricKey, setExpandedMetricKey] = useState<string | null>(null);
  const isModal = variant === "modal";

  const config = useQuery({
    queryKey: [queryKey],
    queryFn: () => apiFetch<MikrotikCollectionResponse>(apiPath),
  });

  const catalog = config.data?.catalog ?? [];
  const sectionLabels = config.data?.sections ?? {};
  const modeLabels = config.data?.collect_mode_labels ?? {};

  useEffect(() => {
    if (!config.data) return;
    setMetrics(mergeMetricsFromApi(config.data.metrics, config.data.catalog));
    setSteps(Array.isArray(config.data.collection_steps) ? config.data.collection_steps : []);
  }, [config.data]);

  const stats = useMemo(() => countEnabled(metrics, catalog), [metrics, catalog]);

  const bySection = useMemo(
    () =>
      SECTION_ORDER.map((section) => ({
        section,
        label: sectionLabels[section] || section,
        fields: catalog.filter((c) => c.section === section),
      })).filter((g) => g.fields.length > 0),
    [catalog, sectionLabels],
  );

  const patch = useMutation({
    mutationFn: () =>
      apiFetch<{ ok: boolean; message?: string }>(apiPath, {
        method: "PATCH",
        json: { metrics, collection_steps: steps },
      }),
    onMutate: () => onPendingChange?.(true),
    onSettled: () => onPendingChange?.(false),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      toastOk(pushToast, data.message || saveSuccessMessage);
      onSaved?.();
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar."),
  });

  function reloadFromServer() {
    if (!config.data) return;
    setMetrics(mergeMetricsFromApi(config.data.metrics, config.data.catalog));
    setSteps(Array.isArray(config.data.collection_steps) ? config.data.collection_steps : []);
  }

  useImperativeHandle(
    ref,
    () => ({
      save: () => patch.mutate(),
      isPending: patch.isPending,
      reloadFromServer,
    }),
    [patch.isPending, metrics, steps, config.data],
  );

  function toggleSection(section: string) {
    setOpenSections((prev) => ({ ...prev, [section]: !prev[section] }));
  }

  function setMetric(key: string, patchMetric: Partial<MikrotikMetricDef>) {
    setMetrics((prev) => ({
      ...prev,
      [key]: { ...prev[key], ...patchMetric },
    }));
  }

  if (config.isLoading) return <p>{loadingLabel}</p>;
  if (config.isError) return <div className="msg msg--err">{(config.error as Error).message}</div>;

  if (isModal) {
    const section = activeSection || "geral";
    if (section === "geral") {
      return (
        <div className="olt-profile-modal__section">
          <h3 className="olt-profile-modal__section-title">Geral</h3>
          <p style={{ margin: "0 0 12px", fontSize: 13, color: "var(--muted)", lineHeight: 1.5 }}>
            Configure o que o monitoramento deve coletar em equipamentos MikroTik/RouterOS. Apenas métricas{" "}
            <strong>activas</strong> com <strong>OID preenchido</strong> entram na coleta.
          </p>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, fontSize: 12 }}>
            <span>
              Métricas activas: <strong>{stats.enabled}</strong>
            </span>
            {stats.missingOid > 0 && (
              <span style={{ color: "var(--warn, #b45309)" }}>
                Sem OID: <strong>{stats.missingOid}</strong>
              </span>
            )}
          </div>
          {stats.enabled === 0 && (
            <div className="msg msg--off" style={{ marginTop: 12, fontSize: 12 }}>
              Nenhuma métrica activa. Active pelo menos um campo nas secções à esquerda.
            </div>
          )}
          {stats.missingOid > 0 && (
            <div className="msg msg--warn" style={{ marginTop: 12, fontSize: 12 }}>
              {stats.missingOid} métrica(s) activa(s) sem OID — o sistema não tentará colectá-las até preencher o OID.
            </div>
          )}
        </div>
      );
    }
    if (section === "advanced") {
      return (
        <div className="olt-profile-modal__section">
          <h3 className="olt-profile-modal__section-title">Avançado</h3>
          <AdvancedStepsEditor steps={steps} setSteps={setSteps} />
        </div>
      );
    }
    const group = bySection.find((g) => g.section === section);
    if (!group) return null;
    return (
      <div className="olt-profile-modal__section">
        <MetricFieldsGrid
          title={group.label}
          description={`Métricas SNMP — ${group.label}. Active o switch e preencha o OID.`}
          entity={group.label}
          fields={group.fields}
          metrics={metrics}
          modeLabels={modeLabels}
          onSetMetric={setMetric}
          expandedKey={expandedMetricKey}
          onToggleExpand={(key) => setExpandedMetricKey((cur) => (cur === key ? null : key))}
          idPrefix={`mk-${queryKey}`}
        />
      </div>
    );
  }

  return (
    <div style={{ marginTop: embedded ? 0 : 8 }}>
      <div className="card" style={{ padding: "12px 16px", marginBottom: 16 }}>
        <h2 style={{ margin: "0 0 6px", fontSize: 16 }}>Coleta SNMP — MikroTik</h2>
        <p style={{ margin: 0, fontSize: 13, color: "var(--muted)", lineHeight: 1.5 }}>
          Configure o que o monitoramento deve coletar em equipamentos MikroTik/RouterOS. Apenas métricas{" "}
          <strong>activas</strong> com <strong>OID preenchido</strong> entram na coleta. Campos activos sem OID são
          ignorados e reportados como dados em falta.
        </p>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginTop: 12, fontSize: 12 }}>
          <span>
            Métricas activas: <strong>{stats.enabled}</strong>
          </span>
          {stats.missingOid > 0 && (
            <span style={{ color: "var(--warn, #b45309)" }}>
              Sem OID (não colectadas): <strong>{stats.missingOid}</strong>
            </span>
          )}
        </div>
      </div>

      {stats.enabled === 0 && (
        <div className="msg msg--off" style={{ marginBottom: 12, fontSize: 12 }}>
          Nenhuma métrica activa. Active pelo menos um campo abaixo para o monitoramento recolher dados.
        </div>
      )}

      {stats.missingOid > 0 && (
        <div className="msg msg--warn" style={{ marginBottom: 12, fontSize: 12 }}>
          {stats.missingOid} métrica(s) activa(s) sem OID — o sistema não tentará colectá-las até preencher o OID.
        </div>
      )}

      {bySection.map(({ section, label, fields }) => {
        const open = openSections[section] === true;
        const sectionStats = countEnabled(
          Object.fromEntries(fields.map((f) => [f.key, metrics[f.key] ?? {}])),
          fields,
        );
        return (
          <div key={section} className="card" style={{ marginBottom: 10, padding: 0, overflow: "hidden" }}>
            <button
              type="button"
              onClick={() => toggleSection(section)}
              style={{
                width: "100%",
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "10px 14px",
                background: "var(--surface-2, rgba(0,0,0,0.03))",
                border: "none",
                cursor: "pointer",
                textAlign: "left",
                fontWeight: 600,
                fontSize: 14,
              }}
            >
              {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
              {label}
              <span style={{ fontWeight: 400, fontSize: 11, color: "var(--muted)", marginLeft: "auto" }}>
                {sectionStats.enabled} activa(s)
                {sectionStats.missingOid > 0 ? ` · ${sectionStats.missingOid} sem OID` : ""}
              </span>
            </button>
            {open && (
              <div style={{ padding: 12 }}>
                <MetricFieldsGrid
                  title={label}
                  description={`Métricas SNMP — ${label}.`}
                  entity={label}
                  fields={fields}
                  metrics={metrics}
                  modeLabels={modeLabels}
                  onSetMetric={setMetric}
                  expandedKey={expandedMetricKey}
                  onToggleExpand={(key) => setExpandedMetricKey((cur) => (cur === key ? null : key))}
                  idPrefix={`mk-page-${queryKey}`}
                />
              </div>
            )}
          </div>
        );
      })}

      <details open={showAdvanced} onToggle={(e) => setShowAdvanced((e.target as HTMLDetailsElement).open)}>
        <summary style={{ cursor: "pointer", fontWeight: 600, fontSize: 13, marginBottom: 8 }}>
          Passos SNMP extra (avançado)
        </summary>
        <AdvancedStepsEditor steps={steps} setSteps={setSteps} />
      </details>

      <div style={{ marginTop: 16, display: "flex", gap: 8, alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          <Save size={16} style={{ marginRight: 6, verticalAlign: "middle" }} />
          {patch.isPending ? "A guardar…" : "Guardar perfil MikroTik"}
        </button>
      </div>
    </div>
  );
});
