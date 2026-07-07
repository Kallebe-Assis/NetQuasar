import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Copy, Plus, Save, Trash2 } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";

type TelnetMetricDef = {
  enabled?: boolean;
  command?: string;
};

type TelnetMetricsForm = Record<string, TelnetMetricDef>;

type CatalogEntry = {
  key: string;
  section: string;
  label: string;
  description: string;
  default_command: string;
  parser: string;
  scope?: string;
  fields?: string;
};

type TelnetProfile = {
  id: string;
  name: string;
  metrics: TelnetMetricsForm;
  pre_commands?: string[];
  is_default?: boolean;
  updated_at?: string;
};

type ProfilesResponse = {
  profiles: TelnetProfile[];
  catalog: CatalogEntry[];
  sections: Record<string, string>;
};

const SECTION_ORDER = ["system", "health", "interfaces", "optical", "wireless"];

function defaultMetricsForm(catalog: CatalogEntry[]): TelnetMetricsForm {
  const out: TelnetMetricsForm = {};
  for (const e of catalog) {
    out[e.key] = { enabled: false, command: e.default_command };
  }
  return out;
}

function mergeMetricsFromApi(raw: TelnetMetricsForm | undefined, catalog: CatalogEntry[]): TelnetMetricsForm {
  const base = defaultMetricsForm(catalog);
  if (!raw) return base;
  for (const e of catalog) {
    const m = raw[e.key];
    if (m) {
      base[e.key] = {
        enabled: m.enabled ?? base[e.key]?.enabled,
        command: m.command?.trim() || e.default_command,
      };
    }
  }
  return base;
}

function countEnabled(metrics: TelnetMetricsForm, catalog: CatalogEntry[]) {
  let enabled = 0;
  for (const e of catalog) {
    if (metrics[e.key]?.enabled) enabled++;
  }
  return enabled;
}

export function MikrotikTelnetProfilesPanel({
  apiBase = "/api/v1/settings/mikrotik-telnet-profiles",
  queryKey = "mikrotik-telnet-profiles",
}: {
  apiBase?: string;
  queryKey?: string;
} = {}) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [profileId, setProfileId] = useState("");
  const [profileName, setProfileName] = useState("");
  const [metrics, setMetrics] = useState<TelnetMetricsForm>({});
  const [preCommandsText, setPreCommandsText] = useState("");
  const [openSections, setOpenSections] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(SECTION_ORDER.map((s) => [s, true])),
  );
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [copyOpen, setCopyOpen] = useState(false);
  const [copyName, setCopyName] = useState("");

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
    setPreCommandsText((selected.pre_commands ?? []).join("\n"));
  }, [selected, catalog]);

  const connDefaults = useQuery({
    queryKey: ["settings-conn-def"],
    queryFn: () =>
      apiFetch<{ telnet_user: string | null; telnet_password_configured: boolean }>(
        "/api/v1/settings/connection/defaults",
      ),
    staleTime: 60_000,
  });

  const stats = useMemo(() => countEnabled(metrics, catalog), [metrics, catalog]);

  const save = useMutation({
    mutationFn: () => {
      if (!profileId) throw new Error("perfil não seleccionado");
      const pre = preCommandsText
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      return apiFetch<TelnetProfile>(`${apiBase}/${profileId}`, {
        method: "PATCH",
        json: { name: profileName.trim(), metrics, pre_commands: pre },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      toastOk(pushToast, "Perfil telnet guardado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao guardar."),
  });

  const create = useMutation({
    mutationFn: (name: string) =>
      apiFetch<TelnetProfile>(apiBase, {
        method: "POST",
        json: { name, metrics: mergeMetricsFromApi(undefined, catalog), pre_commands: [] },
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

  const remove = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`${apiBase}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      toastOk(pushToast, "Perfil removido.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao remover."),
  });

  const copyProfile = useMutation({
    mutationFn: async ({ name }: { name: string }) => {
      if (!selected) throw new Error("nenhum perfil seleccionado");
      const pre = preCommandsText
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      return apiFetch<TelnetProfile>(apiBase, {
        method: "POST",
        json: { name, metrics, pre_commands: pre },
      });
    },
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: [queryKey] });
      setProfileId(p.id);
      setCopyOpen(false);
      setCopyName("");
      toastOk(pushToast, `Cópia criada: «${p.name}».`);
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao copiar."),
  });

  if (list.isLoading) return <p>A carregar perfis telnet…</p>;
  if (list.isError) return <div className="msg msg--err">{(list.error as Error).message}</div>;

  const bySection = SECTION_ORDER.map((section) => ({
    section,
    label: sectionLabels[section] || section,
    fields: catalog.filter((c) => c.section === section),
  })).filter((g) => g.fields.length > 0);

  const telnetReady =
    !!connDefaults.data?.telnet_user?.trim() && connDefaults.data?.telnet_password_configured === true;

  function setMetric(key: string, patch: Partial<TelnetMetricDef>) {
    setMetrics((prev) => ({ ...prev, [key]: { ...prev[key], ...patch } }));
  }

  function toggleSection(section: string) {
    setOpenSections((prev) => ({ ...prev, [section]: !prev[section] }));
  }

  return (
    <div>
      <div className="card" style={{ padding: "12px 16px", marginBottom: 16 }}>
        <h2 style={{ margin: "0 0 6px", fontSize: 16 }}>Coleta Telnet — MikroTik</h2>
        <p style={{ margin: 0, fontSize: 13, color: "var(--muted)", lineHeight: 1.5 }}>
          Perfis nomeados com comandos RouterOS para métricas que não vêm bem via SNMP: interfaces (MTU, status,
          banda), SFP (RX/TX), saúde (temperatura/voltagem), wireless (SSID, canal, protocolo) e uptime. Cada
          equipamento MikroTik pode usar um perfil diferente na página MikroTik; sem perfil atribuído usa o{" "}
          <strong>padrão</strong>.
        </p>
        {!telnetReady && (
          <div className="msg msg--warn" style={{ marginTop: 10, fontSize: 12 }}>
            Credenciais telnet não configuradas — defina em <strong>Rede e SNMP</strong> antes de coletar.
          </div>
        )}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 12, alignItems: "center" }}>
          <label style={{ fontSize: 12, fontWeight: 600 }}>Perfil</label>
          <select
            className="input"
            style={{ minWidth: 200 }}
            value={profileId}
            onChange={(e) => setProfileId(e.target.value)}
          >
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
          <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>Novo perfil telnet</h3>
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
              onClick={() => copyProfile.mutate({ name: copyName.trim() })}
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
                  return (
                    <div
                      key={field.key}
                      className="card"
                      style={{ margin: 0, padding: "10px 12px", background: "var(--surface-2, rgba(0,0,0,0.04))" }}
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
                        </label>
                        <InfoHint label={field.label}>{field.description}</InfoHint>
                      </div>
                      {enabled && (
                        <>
                          <div className="field" style={{ margin: 0 }}>
                            <label style={{ fontSize: 11 }}>Comando RouterOS</label>
                            <input
                              className="input"
                              style={{ fontSize: 12, fontFamily: "monospace" }}
                              placeholder={field.default_command}
                              value={m.command ?? field.default_command}
                              onChange={(e) => setMetric(field.key, { command: e.target.value })}
                            />
                            {field.fields ? (
                              <p style={{ fontSize: 10, color: "var(--muted)", margin: "4px 0 0" }}>
                                Campos: <span className="mono">{field.fields}</span>
                              </p>
                            ) : null}
                            {field.scope ? (
                              <p style={{ fontSize: 10, color: "var(--muted)", margin: "4px 0 0" }}>
                                Executado por interface — use <span className="mono">{"{interface}"}</span> no comando.
                              </p>
                            ) : null}
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

      <details style={{ marginTop: 8 }}>
        <summary style={{ cursor: "pointer", fontWeight: 600, fontSize: 13, marginBottom: 8 }}>
          Comandos pré-sessão (avançado)
        </summary>
        <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
          Executados após login e antes de cada comando de métrica (um por linha). Ex.: desactivar paging ou entrar em
          modo específico.
        </p>
        <textarea
          className="input"
          rows={4}
          style={{ fontFamily: "monospace", fontSize: 12, width: "100%" }}
          value={preCommandsText}
          onChange={(e) => setPreCommandsText(e.target.value)}
          placeholder="/system resource print without-paging"
        />
      </details>

      <div style={{ marginTop: 16, display: "flex", gap: 8, alignItems: "center" }}>
        <button
          type="button"
          className="btn btn--primary"
          disabled={!profileId || save.isPending}
          onClick={() => save.mutate()}
        >
          <Save size={16} style={{ marginRight: 6, verticalAlign: "middle" }} />
          {save.isPending ? "A guardar…" : "Guardar perfil telnet"}
        </button>
      </div>
    </div>
  );
}
