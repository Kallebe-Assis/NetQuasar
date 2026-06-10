import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Save } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";

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

const SECTION_ORDER = ["system", "health", "interfaces", "optical", "wireless", "ppp", "users", "dhcp", "ip"];

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
        value_divisor: m.value_divisor ?? (e.default_divisor && e.default_divisor > 1 ? e.default_divisor : base[e.key]?.value_divisor),
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

export function MikrotikCollectionPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [metrics, setMetrics] = useState<MikrotikMetricsForm>({});
  const [steps, setSteps] = useState<MikrotikCollectionStep[]>([]);
  const [openSections, setOpenSections] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(SECTION_ORDER.map((s) => [s, false])),
  );
  const [showAdvanced, setShowAdvanced] = useState(false);

  const config = useQuery({
    queryKey: ["mikrotik-collection"],
    queryFn: () => apiFetch<MikrotikCollectionResponse>("/api/v1/settings/mikrotik-collection"),
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

  const patch = useMutation({
    mutationFn: () =>
      apiFetch<{ ok: boolean; message?: string }>("/api/v1/settings/mikrotik-collection", {
        method: "PATCH",
        json: { metrics, collection_steps: steps },
      }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["mikrotik-collection"] });
      toastOk(pushToast, data.message || "Perfil MikroTik guardado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar."),
  });

  if (config.isLoading) return <p>A carregar perfil MikroTik…</p>;
  if (config.isError) return <div className="msg msg--err">{(config.error as Error).message}</div>;

  const bySection = SECTION_ORDER.map((section) => ({
    section,
    label: sectionLabels[section] || section,
    fields: catalog.filter((c) => c.section === section),
  })).filter((g) => g.fields.length > 0);

  function toggleSection(section: string) {
    setOpenSections((prev) => ({ ...prev, [section]: !prev[section] }));
  }

  function setMetric(key: string, patch: Partial<MikrotikMetricDef>) {
    setMetrics((prev) => ({
      ...prev,
      [key]: { ...prev[key], ...patch },
    }));
  }

  return (
    <div style={{ marginTop: 8 }}>

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
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "repeat(auto-fill, minmax(360px, 1fr))",
                  gap: 8,
                  padding: 12,
                }}
              >
                {fields.map((field) => {
                  const m = metrics[field.key] ?? {};
                  const enabled = m.enabled === true;
                  const oidMissing = enabled && !m.oid?.trim();
                  const modes = field.collect_modes?.length
                    ? field.collect_modes
                    : ["snmp_get", "snmp_walk"];
                  return (
                    <div
                      key={field.key}
                      className="card"
                      style={{
                        margin: 0,
                        padding: "10px 12px",
                        background: "var(--surface-2, rgba(0,0,0,0.04))",
                        borderColor: oidMissing ? "var(--warn, #f59e0b)" : undefined,
                      }}
                    >
                      <div className="row" style={{ alignItems: "center", gap: 6, marginBottom: 8 }}>
                        <label
                          style={{
                            fontWeight: 600,
                            margin: 0,
                            fontSize: 13,
                            display: "flex",
                            alignItems: "center",
                            gap: 6,
                            flex: 1,
                          }}
                        >
                          <input
                            type="checkbox"
                            checked={enabled}
                            onChange={(e) => setMetric(field.key, { enabled: e.target.checked })}
                          />
                          {field.label}
                          {field.unit ? (
                            <span style={{ fontWeight: 400, color: "var(--muted)", fontSize: 11 }}>
                              ({field.unit})
                            </span>
                          ) : null}
                        </label>
                        <InfoHint label={field.label}>{field.description}</InfoHint>
                      </div>
                      {enabled && (
                        <>
                          <div className="field" style={{ margin: "0 0 6px" }}>
                            <label style={{ fontSize: 11 }}>Tipo de coleta</label>
                            <select
                              className="input"
                              style={{ fontSize: 12, padding: "4px 8px" }}
                              value={m.collect_mode ?? field.default_mode}
                              onChange={(e) => setMetric(field.key, { collect_mode: e.target.value })}
                            >
                              {modes.map((mode) => (
                                <option key={mode} value={mode}>
                                  {modeLabels[mode] || COLLECT_METHODS.find((x) => x.value === mode)?.label || mode}
                                </option>
                              ))}
                            </select>
                          </div>
                          <div className="field" style={{ margin: "0 0 6px" }}>
                            <label style={{ fontSize: 11 }}>
                              OID {m.collect_mode === "snmp_get" ? "(GET)" : "(walk raiz)"}
                            </label>
                            <input
                              className="input"
                              style={{
                                fontSize: 12,
                                fontFamily: "monospace",
                                borderColor: oidMissing ? "var(--warn, #f59e0b)" : undefined,
                              }}
                              placeholder={field.placeholder}
                              value={m.oid ?? ""}
                              onChange={(e) => setMetric(field.key, { oid: e.target.value })}
                            />
                            {oidMissing && (
                              <p style={{ fontSize: 10, color: "var(--warn, #b45309)", margin: "4px 0 0" }}>
                                OID em falta — coleta desactivada para este campo
                              </p>
                            )}
                          </div>
                          {(field.show_divisor || field.default_divisor) && (
                            <div className="field" style={{ margin: "6px 0 0" }}>
                              <label style={{ fontSize: 11 }}>
                                Divisor de saída
                                {field.default_divisor && field.default_divisor > 1 ? (
                                  <span style={{ color: "var(--muted)", fontWeight: 400 }}> (padrão: {field.default_divisor})</span>
                                ) : null}
                              </label>
                              <input
                                className="input"
                                type="number"
                                min={1}
                                step={1}
                                style={{ fontSize: 12, maxWidth: 120 }}
                                placeholder={field.default_divisor ? String(field.default_divisor) : "1"}
                                value={m.value_divisor ?? ""}
                                onChange={(e) => {
                                  const v = e.target.value.trim();
                                  setMetric(field.key, {
                                    value_divisor: v === "" ? undefined : Math.max(1, parseInt(v, 10) || 1),
                                  });
                                }}
                              />
                              <p style={{ fontSize: 10, color: "var(--muted)", margin: "4px 0 0" }}>
                                Ex.: SNMP 237 com divisor 10 → 23,7 · RX -7637 com divisor 1000 → -7,637 dBm
                              </p>
                            </div>
                          )}
                          {(field.walk_target === "interfaces" || field.section === "optical") && (
                            <p style={{ fontSize: 10, color: "var(--muted)", margin: "6px 0 0" }}>
                              {field.section === "optical"
                                ? field.key === "optical_table"
                                  ? "Use «Tabela SFP parseada» para um walk único com RX/TX/temp/voltagem por porta."
                                  : "«Coluna SFP (derivada)» usa a tabela completa; «SNMP WALK» coleta só esta coluna."
                                : field.key === "if_oper_status" || field.key === "if_admin_status"
                                  ? "«IF-MIB status (parse)» devolve up/down por ifIndex; «SNMP WALK» devolve vars brutos."
                                  : "Destino: snapshot de interfaces (ciclo IF-MIB)"}
                            </p>
                          )}
                        </>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}

      <details open={showAdvanced} onToggle={(e) => setShowAdvanced((e.target as HTMLDetailsElement).open)}>
        <summary style={{ cursor: "pointer", fontWeight: 600, fontSize: 13, marginBottom: 8 }}>
          Passos SNMP extra (avançado)
        </summary>
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
            setSteps([
              ...steps,
              { id: `step_${Date.now()}`, method: "snmp_walk", enabled: true, oid: "" },
            ])
          }
        >
          + Adicionar passo
        </button>
      </details>

      <div style={{ marginTop: 16, display: "flex", gap: 8, alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          <Save size={16} style={{ marginRight: 6, verticalAlign: "middle" }} />
          {patch.isPending ? "A guardar…" : "Guardar perfil MikroTik"}
        </button>
      </div>
    </div>
  );
}
