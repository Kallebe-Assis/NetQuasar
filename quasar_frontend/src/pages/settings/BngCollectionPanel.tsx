import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Save } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";

type BngMetricDef = {
  enabled?: boolean;
  oid?: string;
  collect_mode?: string;
};

type BngMetricsForm = Record<string, BngMetricDef>;

type CatalogEntry = {
  key: string;
  section: string;
  label: string;
  description: string;
  placeholder: string;
  collect_modes: string[];
  default_mode: string;
  unit?: string;
  recommended?: boolean;
};

type BngCollectionResponse = {
  metrics: BngMetricsForm;
  catalog: CatalogEntry[];
  sections: Record<string, string>;
  collect_mode_labels: Record<string, string>;
};

const SECTION_ORDER = ["system", "health", "subscribers", "pppoe"];

function defaultMetricsForm(catalog: CatalogEntry[]): BngMetricsForm {
  const out: BngMetricsForm = {};
  for (const e of catalog) {
    out[e.key] = {
      enabled: !!e.recommended,
      oid: e.placeholder,
      collect_mode: e.default_mode || "snmp_get",
    };
  }
  return out;
}

function mergeMetricsFromApi(raw: BngMetricsForm | undefined, catalog: CatalogEntry[]): BngMetricsForm {
  const base = defaultMetricsForm(catalog);
  if (!raw) return base;
  for (const e of catalog) {
    const m = raw[e.key];
    if (m) {
      base[e.key] = {
        enabled: m.enabled ?? base[e.key]?.enabled,
        oid: m.oid ?? base[e.key]?.oid ?? e.placeholder,
        collect_mode: m.collect_mode ?? base[e.key]?.collect_mode ?? e.default_mode,
      };
    }
  }
  return base;
}

function countEnabled(metrics: BngMetricsForm, catalog: CatalogEntry[]) {
  let enabled = 0;
  let missingOid = 0;
  let recommendedOn = 0;
  for (const e of catalog) {
    const m = metrics[e.key];
    if (m?.enabled) {
      enabled++;
      if (e.recommended) recommendedOn++;
      if (!m.oid?.trim()) missingOid++;
    }
  }
  return { enabled, missingOid, recommendedOn };
}

export function BngCollectionPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [metrics, setMetrics] = useState<BngMetricsForm>({});
  const [openSections, setOpenSections] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(SECTION_ORDER.map((s) => [s, s === "subscribers" || s === "system"])),
  );

  const config = useQuery({
    queryKey: ["bng-collection"],
    queryFn: () => apiFetch<BngCollectionResponse>("/api/v1/settings/bng-collection"),
  });

  const catalog = config.data?.catalog ?? [];
  const sectionLabels = config.data?.sections ?? {};
  const modeLabels = config.data?.collect_mode_labels ?? {};

  useEffect(() => {
    if (!config.data) return;
    setMetrics(mergeMetricsFromApi(config.data.metrics, config.data.catalog));
  }, [config.data]);

  const stats = useMemo(() => countEnabled(metrics, catalog), [metrics, catalog]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch<{ ok: boolean; message?: string }>("/api/v1/settings/bng-collection", {
        method: "PATCH",
        json: { metrics },
      }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["bng-collection"] });
      toastOk(pushToast, data.message || "Perfil BNG guardado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar."),
  });

  if (config.isLoading) return <p>A carregar perfil BNG…</p>;
  if (config.isError) return <div className="msg msg--err">{(config.error as Error).message}</div>;

  const bySection = SECTION_ORDER.map((section) => ({
    section,
    label: sectionLabels[section] || section,
    fields: catalog.filter((c) => c.section === section),
  })).filter((g) => g.fields.length > 0);

  function toggleSection(section: string) {
    setOpenSections((prev) => ({ ...prev, [section]: !prev[section] }));
  }

  function setMetric(key: string, patch: Partial<BngMetricDef>) {
    setMetrics((prev) => ({
      ...prev,
      [key]: { ...prev[key], ...patch },
    }));
  }

  function applyRecommended() {
    setMetrics((prev) => {
      const next = { ...prev };
      for (const e of catalog) {
        if (e.recommended) {
          next[e.key] = {
            ...next[e.key],
            enabled: true,
            oid: next[e.key]?.oid || e.placeholder,
            collect_mode: next[e.key]?.collect_mode || e.default_mode,
          };
        }
      }
      return next;
    });
  }

  return (
    <div style={{ marginTop: 8 }}>
      <div className="card" style={{ padding: "12px 16px", marginBottom: 16 }}>
        <h2 style={{ margin: "0 0 6px", fontSize: 16 }}>Coleta SNMP — BNG</h2>
        <p style={{ margin: 0, fontSize: 13, color: "var(--muted)", lineHeight: 1.5 }}>
          Perfil para concentradores BNG (Huawei NE8000 e similares). O monitoramento periódico coleta métricas{" "}
          <strong>activas</strong> com OID preenchido. A secção «Sessões PPPoE» é só para consulta manual na página BNG —
          não active walks de sessão no ciclo automático (milhares de linhas SNMP).
        </p>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginTop: 12, fontSize: 12, alignItems: "center" }}>
          <span>
            Métricas activas: <strong>{stats.enabled}</strong>
          </span>
          <span>
            Recomendadas activas: <strong>{stats.recommendedOn}</strong>
          </span>
          {stats.missingOid > 0 && (
            <span style={{ color: "var(--warn, #b45309)" }}>
              Sem OID: <strong>{stats.missingOid}</strong>
            </span>
          )}
          <button type="button" className="btn btn--sm" onClick={applyRecommended}>
            Activar recomendadas
          </button>
        </div>
      </div>

      <div className="msg msg--off" style={{ marginBottom: 12, fontSize: 12 }}>
        <strong>Recomendado no ciclo automático:</strong> sistema (nome, modelo, versão), totais de assinantes (PPPoE, IPv4,
        IPv6, dual-stack) e saúde (CPU). Consulta completa de sessões PPPoE — login, MAC, IP, estados — na página{" "}
        <em>BNG → Sessões PPPoE</em>.
      </div>

      {stats.missingOid > 0 && (
        <div className="msg msg--warn" style={{ marginBottom: 12, fontSize: 12 }}>
          {stats.missingOid} métrica(s) activa(s) sem OID — não serão colectadas até preencher o OID (ajuste o índice da placa
          em CPU/memória/temperatura).
        </div>
      )}

      {bySection.map(({ section, label, fields }) => {
        const open = openSections[section] === true;
        const sectionStats = countEnabled(
          Object.fromEntries(fields.map((f) => [f.key, metrics[f.key] ?? {}])),
          fields,
        );
        const isHeavy = section === "pppoe";
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
              {isHeavy && (
                <span style={{ fontSize: 10, color: "var(--warn, #b45309)", fontWeight: 500 }}>pesado — só manual</span>
              )}
              <span style={{ fontWeight: 400, fontSize: 11, color: "var(--muted)", marginLeft: "auto" }}>
                {sectionStats.enabled} activa(s)
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
                  const modes = field.collect_modes?.length ? field.collect_modes : ["snmp_get", "snmp_walk"];
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
                          {field.recommended ? (
                            <span style={{ fontSize: 10, color: "var(--ok, #16a34a)" }}>recomendado</span>
                          ) : null}
                          {field.unit ? (
                            <span style={{ fontWeight: 400, color: "var(--muted)", fontSize: 11 }}>({field.unit})</span>
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
                                  {modeLabels[mode] || mode}
                                </option>
                              ))}
                            </select>
                          </div>
                          <div className="field" style={{ margin: 0 }}>
                            <label style={{ fontSize: 11 }}>OID</label>
                            <input
                              className="input mono"
                              style={{ fontSize: 12, borderColor: oidMissing ? "var(--warn, #f59e0b)" : undefined }}
                              placeholder={field.placeholder}
                              value={m.oid ?? ""}
                              onChange={(e) => setMetric(field.key, { oid: e.target.value })}
                            />
                          </div>
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

      <div className="row" style={{ marginTop: 16, gap: 8 }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          <Save size={16} style={{ marginRight: 6, verticalAlign: -2 }} />
          Guardar perfil BNG
        </button>
      </div>
    </div>
  );
}
