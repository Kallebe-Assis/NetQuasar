import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Pencil } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { apiFetch } from "../../lib/api";
import {
  bngExtraRowsFromBlock,
  buildBngOverridesFromRows,
  type CategoryOverrides,
  type ExtraOidRow,
} from "../../lib/oidExtrasConfig";
import { OidExtrasEditor } from "./OidExtrasEditor";
import { OltMetricsOidTable, type MetricsOidFieldMeta } from "./OltMetricsOidTable";

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
  options?: BngCollectionOptions;
  catalog: CatalogEntry[];
  sections: Record<string, string>;
  collect_mode_labels: Record<string, string>;
};

type BngCollectionOptions = {
  pppoe_login_strip_suffix?: string;
  uplink_interfaces?: string[];
};

const SECTION_ORDER = ["system", "health", "subscribers", "pppoe"];

type EditSection = "geral" | (typeof SECTION_ORDER)[number] | "extras";

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

function collectModeTypeLabel(mode: string): string {
  const m = mode.toLowerCase();
  if (m.includes("walk") || m.includes("access")) return "Walk";
  if (m.includes("get")) return "GET";
  return "SNMP";
}

function catalogToOidFields(fields: CatalogEntry[], entity: string): MetricsOidFieldMeta[] {
  return fields.map((f) => ({
    key: f.key,
    label: f.label,
    shortDesc: f.recommended ? "Recomendado" : f.description.slice(0, 80) + (f.description.length > 80 ? "…" : ""),
    hint: f.description,
    placeholder: f.placeholder,
    entity,
    unit: f.unit || "—",
    typeLabel: collectModeTypeLabel(f.default_mode || "snmp_get"),
    expandable: (f.collect_modes?.length ?? 0) > 1 || Boolean(f.collect_modes?.length),
  }));
}

function BngMetricsOidSection({
  title,
  description,
  fields,
  entity,
  metrics,
  modeLabels,
  onSetMetric,
  expandedKey,
  onToggleExpand,
}: {
  title: string;
  description: string;
  fields: CatalogEntry[];
  entity: string;
  metrics: BngMetricsForm;
  modeLabels: Record<string, string>;
  onSetMetric: (key: string, patch: Partial<BngMetricDef>) => void;
  expandedKey: string | null;
  onToggleExpand: (key: string) => void;
}) {
  const oidFields = catalogToOidFields(fields, entity);
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
      idPrefix="bng-metric"
      defaultEnabled={false}
      renderExpanded={(field) => {
        const cat = fields.find((f) => f.key === field.key);
        if (!cat) return null;
        const m = metrics[field.key] ?? {};
        if (m.enabled !== true) {
          return <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>Active a métrica para configurar o tipo de coleta.</p>;
        }
        const modes = cat.collect_modes?.length ? cat.collect_modes : ["snmp_get", "snmp_walk"];
        return (
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
                  {modeLabels[mode] || mode}
                </option>
              ))}
            </select>
          </div>
        );
      }}
    />
  );
}

export function BngCollectionPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [editing, setEditing] = useState(false);
  const [editSection, setEditSection] = useState<EditSection>("geral");
  const [expandedMetricKey, setExpandedMetricKey] = useState<string | null>(null);
  const [metrics, setMetrics] = useState<BngMetricsForm>({});
  const [options, setOptions] = useState<BngCollectionOptions>({});
  const [extraRows, setExtraRows] = useState<ExtraOidRow[]>([]);
  const [extrasBaseline, setExtrasBaseline] = useState<CategoryOverrides | undefined>();

  const config = useQuery({
    queryKey: ["bng-collection"],
    queryFn: () => apiFetch<BngCollectionResponse>("/api/v1/settings/bng-collection"),
  });

  const connDefaults = useQuery({
    queryKey: ["settings-conn-def"],
    queryFn: () =>
      apiFetch<{ snmp_oid_overrides?: { bng?: CategoryOverrides } }>("/api/v1/settings/connection/defaults"),
  });

  const catalog = config.data?.catalog ?? [];
  const sectionLabels = config.data?.sections ?? {};
  const modeLabels = config.data?.collect_mode_labels ?? {};

  useEffect(() => {
    if (!config.data) return;
    setMetrics(mergeMetricsFromApi(config.data.metrics, config.data.catalog));
    setOptions(config.data.options ?? {});
  }, [config.data]);

  useEffect(() => {
    if (!connDefaults.data) return;
    const overrides = connDefaults.data.snmp_oid_overrides;
    const bngBlock =
      overrides && typeof overrides === "object" && !Array.isArray(overrides)
        ? (overrides as { bng?: CategoryOverrides }).bng
        : undefined;
    setExtrasBaseline(bngBlock);
    setExtraRows(bngExtraRowsFromBlock(bngBlock));
  }, [connDefaults.data]);

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

  const navSections = useMemo(
    () => [
      { id: "geral" as const, label: "Geral" },
      ...bySection.map((g) => ({ id: g.section as EditSection, label: g.label })),
      { id: "extras" as const, label: "OIDs extra" },
    ],
    [bySection],
  );

  const patch = useMutation({
    mutationFn: async () => {
      await apiFetch<{ ok: boolean; message?: string }>("/api/v1/settings/bng-collection", {
        method: "PATCH",
        json: { metrics, options },
      });
      const bngOverrides = buildBngOverridesFromRows(extraRows, extrasBaseline);
      const prev =
        connDefaults.data?.snmp_oid_overrides && typeof connDefaults.data.snmp_oid_overrides === "object"
          ? (connDefaults.data.snmp_oid_overrides as Record<string, unknown>)
          : {};
      await apiFetch("/api/v1/settings/connection/defaults", {
        method: "PATCH",
        json: {
          snmp_oid_overrides: { ...prev, bng: bngOverrides },
        },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["bng-collection"] });
      qc.invalidateQueries({ queryKey: ["settings-conn-def"] });
      toastOk(pushToast, "Perfil BNG e OIDs extra guardados.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar."),
  });

  function openEdit() {
    if (config.data) {
      setMetrics(mergeMetricsFromApi(config.data.metrics, config.data.catalog));
      setOptions(config.data.options ?? {});
    }
    if (connDefaults.data) {
      const overrides = connDefaults.data.snmp_oid_overrides;
      const bngBlock =
        overrides && typeof overrides === "object" && !Array.isArray(overrides)
          ? (overrides as { bng?: CategoryOverrides }).bng
          : undefined;
      setExtrasBaseline(bngBlock);
      setExtraRows(bngExtraRowsFromBlock(bngBlock));
    }
    setEditSection("geral");
    setEditing(true);
  }

  function closeEdit() {
    if (config.data) {
      setMetrics(mergeMetricsFromApi(config.data.metrics, config.data.catalog));
      setOptions(config.data.options ?? {});
    }
    if (connDefaults.data) {
      const overrides = connDefaults.data.snmp_oid_overrides;
      const bngBlock =
        overrides && typeof overrides === "object" && !Array.isArray(overrides)
          ? (overrides as { bng?: CategoryOverrides }).bng
          : undefined;
      setExtrasBaseline(bngBlock);
      setExtraRows(bngExtraRowsFromBlock(bngBlock));
    }
    setEditing(false);
  }

  function setMetric(key: string, patchMetric: Partial<BngMetricDef>) {
    setMetrics((prev) => ({
      ...prev,
      [key]: { ...prev[key], ...patchMetric },
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

  if (config.isLoading || connDefaults.isLoading) return <p>A carregar perfil BNG…</p>;
  if (config.isError) return <div className="msg msg--err">{(config.error as Error).message}</div>;
  if (connDefaults.isError) return <div className="msg msg--err">{(connDefaults.error as Error).message}</div>;

  const statusLabel =
    stats.enabled > 0
      ? `${stats.enabled} métrica${stats.enabled === 1 ? "" : "s"} activa${stats.enabled === 1 ? "" : "s"}`
      : "Inactivo";

  return (
    <>
      <div className="olt-profiles-layout">
        <div className="card">
          <div>
            <h2 style={{ margin: 0 }}>Perfis BNG</h2>
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 4, marginBottom: 0 }}>
              Coleta SNMP para concentradores BNG (Huawei NE8000 e similares). Intervalo em Configurações →
              Monitoramento.
            </p>
          </div>

          <div className="table-wrap" style={{ marginTop: 12 }}>
            <table className="olt-profiles-table">
              <thead>
                <tr>
                  <th>Descrição</th>
                  <th>Marca/Tipo</th>
                  <th>Modelo</th>
                  <th>Status</th>
                  <th style={{ width: 110 }}>Ações</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>Coleta SNMP global</td>
                  <td>BNG</td>
                  <td className="mono">Huawei NE8000+</td>
                  <td>
                    <span className={stats.enabled > 0 ? "badge badge--ok" : "badge"}>{statusLabel}</span>
                  </td>
                  <td>
                    <div className="olt-profiles-table__actions">
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Editar"
                        aria-label="Editar coleta SNMP BNG"
                        onClick={openEdit}
                      >
                        <Pencil size={14} aria-hidden />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {editing && (
        <div
          className="modal-backdrop olt-profile-modal-backdrop"
          role="presentation"
          onClick={closeEdit}
        >
          <div
            className="olt-profile-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="bng-profile-edit-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="olt-profile-modal__header">
              <div>
                <h2 id="bng-profile-edit-title" style={{ margin: 0 }}>
                  Editar perfil BNG
                </h2>
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "4px 0 0" }}>
                  Coleta SNMP global · Huawei NE8000+
                </p>
              </div>
              <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
                <button type="button" className="btn" onClick={closeEdit}>
                  Cancelar
                </button>
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={patch.isPending}
                  onClick={() => patch.mutate()}
                >
                  {patch.isPending ? "A guardar…" : "Guardar"}
                </button>
              </div>
            </div>

            <div className="olt-profile-modal__body" style={{ gridTemplateColumns: "200px minmax(0, 1fr)" }}>
              <nav className="olt-profile-modal__nav" aria-label="Secções do perfil">
                <div className="olt-profile-modal__nav-list">
                  {navSections.map((sec) => (
                    <button
                      key={sec.id}
                      type="button"
                      className={
                        "olt-profile-modal__nav-btn" +
                        (editSection === sec.id ? " olt-profile-modal__nav-btn--active" : "")
                      }
                      onClick={() => setEditSection(sec.id)}
                    >
                      {sec.label}
                    </button>
                  ))}
                </div>
              </nav>

              <div className="olt-profile-modal__main">
                {editSection === "geral" && (
                  <div className="olt-profile-modal__section">
                    <h3 className="olt-profile-modal__section-title">Geral</h3>
                    <p style={{ margin: "0 0 12px", fontSize: 13, color: "var(--muted)", lineHeight: 1.5 }}>
                      O monitoramento periódico coleta métricas <strong>activas</strong> com OID preenchido. Totais de
                      logins entram no ciclo de telemetria — o equipamento precisa de telemetria e BNG activos. A
                      secção «Sessões PPPoE» é só para consulta manual (não active walks no ciclo automático).
                    </p>
                    <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16, fontSize: 12, alignItems: "center" }}>
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

                    <div className="field" style={{ maxWidth: 360, margin: "0 0 16px" }}>
                      <label style={{ fontSize: 11 }}>Sufixo a ignorar (logins PPPoE)</label>
                      <input
                        className="input mono"
                        placeholder="@g2.com.br"
                        value={options.pppoe_login_strip_suffix ?? ""}
                        onChange={(e) =>
                          setOptions((prev) => ({ ...prev, pppoe_login_strip_suffix: e.target.value }))
                        }
                      />
                      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0", lineHeight: 1.4 }}>
                        Sufixo RADIUS removido na exibição e na pesquisa (ex.: <span className="mono">@g2.com.br</span>).
                      </p>
                    </div>

                    <div className="field" style={{ maxWidth: 480, margin: 0 }}>
                      <label style={{ fontSize: 11 }}>Interfaces de uplink (Links BGP)</label>
                      <textarea
                        className="input mono"
                        rows={3}
                        placeholder={"WAN-BGP\nVLAN-254-BGP"}
                        value={(options.uplink_interfaces ?? []).join("\n")}
                        onChange={(e) => {
                          const parts = e.target.value
                            .split(/[\n,;]+/)
                            .map((s) => s.trim())
                            .filter(Boolean);
                          setOptions((prev) => ({
                            ...prev,
                            uplink_interfaces: parts.length > 0 ? parts : undefined,
                          }));
                        }}
                      />
                      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0", lineHeight: 1.4 }}>
                        Um nome por linha. Se vazio, detecta interfaces com «BGP» ou «WAN-BGP» no nome.
                      </p>
                    </div>

                    {stats.missingOid > 0 && (
                      <div className="msg msg--warn" style={{ marginTop: 12, fontSize: 12 }}>
                        {stats.missingOid} métrica(s) activa(s) sem OID — não serão colectadas até preencher o OID.
                      </div>
                    )}
                  </div>
                )}

                {bySection.map(({ section, label, fields }) =>
                  editSection === section ? (
                    <div key={section} className="olt-profile-modal__section">
                      {section === "pppoe" && (
                        <div className="msg msg--off" style={{ marginBottom: 12, fontSize: 12 }}>
                          Secção pesada — só consulta manual na página BNG → Sessões PPPoE. Não active no ciclo automático.
                        </div>
                      )}
                      <BngMetricsOidSection
                        title={label}
                        description={`Métricas SNMP — ${label}. Active o switch e preencha o OID.`}
                        fields={fields}
                        entity={section === "pppoe" ? "PPPoE" : section === "subscribers" ? "Totais" : "BNG"}
                        metrics={metrics}
                        modeLabels={modeLabels}
                        onSetMetric={setMetric}
                        expandedKey={expandedMetricKey}
                        onToggleExpand={(key) => setExpandedMetricKey((cur) => (cur === key ? null : key))}
                      />
                    </div>
                  ) : null,
                )}

                {editSection === "extras" && (
                  <div className="olt-profile-modal__section">
                    <h3 className="olt-profile-modal__section-title">OIDs extra</h3>
                    <OidExtrasEditor title="OIDs extra (telemetria BNG)" rows={extraRows} onChange={setExtraRows} />
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
