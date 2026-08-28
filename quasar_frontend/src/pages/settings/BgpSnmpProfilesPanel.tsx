import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Copy, Plus, Save, Star, Trash2 } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { apiFetch } from "../../lib/api";
import { OltMetricsOidTable, type MetricsOidFieldMeta } from "./OltMetricsOidTable";

type BgpMetricDef = {
  enabled?: boolean;
  oid?: string;
  collect_mode?: string;
};

type BgpMetricsForm = Record<string, BgpMetricDef>;

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

export type BgpSnmpProfile = {
  id: string;
  name: string;
  metrics: BgpMetricsForm;
  is_default?: boolean;
  updated_at?: string;
};

type ProfilesResponse = {
  profiles: BgpSnmpProfile[];
  catalog: CatalogEntry[];
  sections: Record<string, string>;
};

const SECTION_ORDER = [
  "saude",
  "interfaces",
  "trafego",
  "peers",
  "optica",
  "chassi",
  "vs",
  "bfd",
  "etrunk",
  "qos",
  "cpu_nucleos",
  "radius",
  "lldp",
] as const;

const MODE_LABELS: Record<string, string> = {
  snmp_get: "SNMP GET",
  snmp_walk: "SNMP WALK",
};

function defaultMetricsForm(catalog: CatalogEntry[]): BgpMetricsForm {
  const out: BgpMetricsForm = {};
  for (const e of catalog) {
    out[e.key] = { enabled: !!e.recommended, oid: e.placeholder, collect_mode: e.default_mode || "snmp_get" };
  }
  return out;
}

function mergeMetricsFromApi(raw: BgpMetricsForm | undefined, catalog: CatalogEntry[]): BgpMetricsForm {
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

function countEnabled(metrics: BgpMetricsForm, catalog: CatalogEntry[]) {
  let enabled = 0;
  for (const e of catalog) {
    if (metrics[e.key]?.enabled) enabled++;
  }
  return enabled;
}

function collectModeTypeLabel(mode: string): string {
  const m = mode.toLowerCase();
  if (m.includes("walk")) return "Walk";
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
    expandable: (f.collect_modes?.length ?? 0) > 1,
  }));
}

function BgpMetricsOidSection({
  title,
  description,
  fields,
  entity,
  metrics,
  onSetMetric,
  expandedKey,
  onToggleExpand,
}: {
  title: string;
  description: string;
  fields: CatalogEntry[];
  entity: string;
  metrics: BgpMetricsForm;
  onSetMetric: (key: string, patch: Partial<BgpMetricDef>) => void;
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
      idPrefix="bgp-metric"
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
                  {MODE_LABELS[mode] || mode}
                </option>
              ))}
            </select>
          </div>
        );
      }}
    />
  );
}

export function BgpSnmpProfilesPanel() {
  const apiBase = "/api/v1/settings/bgp-snmp-profiles";
  const queryKey = "bgp-snmp-profiles";
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [profileId, setProfileId] = useState("");
  const [profileName, setProfileName] = useState("");
  const [metrics, setMetrics] = useState<BgpMetricsForm>({});
  const [openSections, setOpenSections] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(SECTION_ORDER.map((s) => [s, true])),
  );
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [copyOpen, setCopyOpen] = useState(false);
  const [copyName, setCopyName] = useState("");
  const [expandedMetricKey, setExpandedMetricKey] = useState<string | null>(null);

  const list = useQuery({
    queryKey: [queryKey],
    queryFn: () => apiFetch<ProfilesResponse>(apiBase),
  });

  const catalog = list.data?.catalog ?? [];
  const sectionLabels = list.data?.sections ?? {};
  const profiles = list.data?.profiles ?? [];

  const selected = useMemo(() => profiles.find((p) => p.id === profileId) ?? null, [profiles, profileId]);

  useEffect(() => {
    if (profiles.length === 0) {
      setProfileId("");
      return;
    }
    if (!profileId || !profiles.some((p) => p.id === profileId)) {
      const def = profiles.find((p) => p.is_default) ?? profiles[0];
      setProfileId(def.id);
    }
  }, [profiles, profileId]);

  useEffect(() => {
    if (!selected) return;
    setProfileName(selected.name);
    setMetrics(mergeMetricsFromApi(selected.metrics, catalog));
  }, [selected, catalog]);

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

  const save = useMutation({
    mutationFn: () => {
      if (!selected) throw new Error("perfil não seleccionado");
      return apiFetch<BgpSnmpProfile>(`${apiBase}/${selected.id}`, {
        method: "PATCH",
        json: { name: profileName.trim(), metrics },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      toastOk(pushToast, "Perfil SNMP de BGP guardado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao guardar."),
  });

  const create = useMutation({
    mutationFn: (name: string) =>
      apiFetch<BgpSnmpProfile>(apiBase, {
        method: "POST",
        json: { name, metrics: defaultMetricsForm(catalog) },
      }),
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      setProfileId(p.id);
      setCreateOpen(false);
      setCreateName("");
      toastOk(pushToast, `Perfil «${p.name}» criado.`);
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao criar perfil."),
  });

  const copyProfile = useMutation({
    mutationFn: (name: string) => apiFetch<BgpSnmpProfile>(apiBase, { method: "POST", json: { name, metrics } }),
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      setProfileId(p.id);
      setCopyOpen(false);
      setCopyName("");
      toastOk(pushToast, `Cópia criada: «${p.name}».`);
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao copiar."),
  });

  const remove = useMutation({
    mutationFn: (id: string) => apiFetch(`${apiBase}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      toastOk(pushToast, "Perfil removido.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao remover."),
  });

  const setDefault = useMutation({
    mutationFn: (id: string) => apiFetch<BgpSnmpProfile>(`${apiBase}/${id}`, { method: "PATCH", json: { is_default: true } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      toastOk(pushToast, "Perfil definido como padrão (usado pela coleta periódica).");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao definir padrão."),
  });

  function setMetric(key: string, patch: Partial<BgpMetricDef>) {
    setMetrics((prev) => ({ ...prev, [key]: { ...prev[key], ...patch } }));
  }

  function toggleSection(section: string) {
    setOpenSections((prev) => ({ ...prev, [section]: !prev[section] }));
  }

  if (list.isLoading) return <p>A carregar perfis SNMP de BGP…</p>;
  if (list.isError) return <div className="msg msg--err">{(list.error as Error).message}</div>;

  return (
    <div>
      <div className="card" style={{ padding: "12px 16px", marginBottom: 16 }}>
        <h2 style={{ margin: "0 0 6px", fontSize: 16 }}>Coleta SNMP — BGP</h2>
        <p style={{ margin: 0, fontSize: 13, color: "var(--muted)", lineHeight: 1.5 }}>
          Perfis de OIDs SNMP para saúde, interfaces, tráfego e peers BGP. O perfil marcado como{" "}
          <strong>padrão</strong> é o usado pela coleta periódica (equipamentos com BGP activo na aba
          Monitoramento). Crie perfis extra se algum fabricante precisar de OIDs diferentes.
        </p>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 12, alignItems: "center" }}>
          <label style={{ fontSize: 12, fontWeight: 600 }}>Perfil</label>
          <select className="input" style={{ minWidth: 200 }} value={profileId} onChange={(e) => setProfileId(e.target.value)}>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
                {p.is_default ? " (padrão)" : ""}
              </option>
            ))}
          </select>
          <button type="button" className="btn btn--ghost" onClick={() => setCreateOpen(true)}>
            <Plus size={14} style={{ marginRight: 4, verticalAlign: "middle" }} />
            Novo
          </button>
          <button
            type="button"
            className="btn btn--ghost"
            disabled={!selected}
            onClick={() => {
              setCopyName(`${selected?.name ?? "Perfil"} (cópia)`);
              setCopyOpen(true);
            }}
          >
            <Copy size={14} style={{ marginRight: 4, verticalAlign: "middle" }} />
            Copiar
          </button>
          <button
            type="button"
            className="btn btn--ghost"
            disabled={!selected || selected.is_default || setDefault.isPending}
            onClick={() => selected && setDefault.mutate(selected.id)}
            title="Usar este perfil na coleta periódica"
          >
            <Star size={14} style={{ marginRight: 4, verticalAlign: "middle" }} />
            Definir padrão
          </button>
          <button
            type="button"
            className="btn btn--ghost"
            disabled={!selected || selected.is_default || remove.isPending}
            onClick={() => {
              if (!selected || !window.confirm(`Apagar perfil «${selected.name}»?`)) return;
              remove.mutate(selected.id);
            }}
          >
            <Trash2 size={14} style={{ marginRight: 4, verticalAlign: "middle" }} />
            Apagar
          </button>
        </div>
        {selected && (
          <div className="field" style={{ marginTop: 10, maxWidth: 360 }}>
            <label style={{ fontSize: 11 }}>Nome do perfil</label>
            <input
              className="input"
              value={profileName}
              disabled={selected.is_default}
              onChange={(e) => setProfileName(e.target.value)}
            />
            {selected.is_default && (
              <p style={{ fontSize: 10, color: "var(--muted)", margin: "4px 0 0" }}>
                O nome do perfil padrão não pode ser alterado.
              </p>
            )}
          </div>
        )}
        <div style={{ marginTop: 8, fontSize: 12 }}>
          Métricas activas neste perfil: <strong>{stats}</strong>
        </div>
      </div>

      {createOpen && (
        <div className="card" style={{ marginBottom: 12, padding: 12 }}>
          <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>Novo perfil SNMP de BGP</h3>
          <div className="field" style={{ margin: 0, maxWidth: 320 }}>
            <label style={{ fontSize: 11 }}>Nome</label>
            <input className="input" value={createName} onChange={(e) => setCreateName(e.target.value)} />
          </div>
          <div style={{ marginTop: 8, display: "flex", gap: 8 }}>
            <button
              type="button"
              className="btn btn--primary"
              disabled={!createName.trim() || create.isPending}
              onClick={() => create.mutate(createName.trim())}
            >
              Criar
            </button>
            <button type="button" className="btn btn--ghost" onClick={() => setCreateOpen(false)}>
              Cancelar
            </button>
          </div>
        </div>
      )}

      {copyOpen && (
        <div className="card" style={{ marginBottom: 12, padding: 12 }}>
          <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>Copiar perfil</h3>
          <div className="field" style={{ margin: 0, maxWidth: 320 }}>
            <label style={{ fontSize: 11 }}>Nome da cópia</label>
            <input className="input" value={copyName} onChange={(e) => setCopyName(e.target.value)} />
          </div>
          <div style={{ marginTop: 8, display: "flex", gap: 8 }}>
            <button
              type="button"
              className="btn btn--primary"
              disabled={!copyName.trim() || copyProfile.isPending}
              onClick={() => copyProfile.mutate(copyName.trim())}
            >
              Copiar
            </button>
            <button type="button" className="btn btn--ghost" onClick={() => setCopyOpen(false)}>
              Cancelar
            </button>
          </div>
        </div>
      )}

      {bySection.map(({ section, label, fields }) => {
        const open = openSections[section] === true;
        const sectionEnabled = countEnabled(
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
                {sectionEnabled} activa(s)
              </span>
            </button>
            {open && (
              <div style={{ padding: 12 }}>
                <BgpMetricsOidSection
                  title={label}
                  description={`Métricas SNMP — ${label}. Active o switch e preencha o OID.`}
                  fields={fields}
                  entity={section === "peers" ? "Peer" : "BGP"}
                  metrics={metrics}
                  onSetMetric={setMetric}
                  expandedKey={expandedMetricKey}
                  onToggleExpand={(key) => setExpandedMetricKey((cur) => (cur === key ? null : key))}
                />
              </div>
            )}
          </div>
        );
      })}

      <div style={{ marginTop: 16, display: "flex", gap: 8, alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={!profileId || save.isPending} onClick={() => save.mutate()}>
          <Save size={16} style={{ marginRight: 6, verticalAlign: "middle" }} />
          {save.isPending ? "A guardar…" : "Guardar perfil BGP"}
        </button>
      </div>
    </div>
  );
}
