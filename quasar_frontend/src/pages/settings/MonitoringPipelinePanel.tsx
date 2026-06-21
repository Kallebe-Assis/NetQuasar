import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { MonitoringPingIntervalsCard } from "./MonitoringIntervalsCard";

export type PipelineStepScope = {
  target: "all" | "category" | "devices";
  category?: string;
  device_ids?: string[];
};

export type PipelineStepOptions = {
  telemetry_fields?: string[];
  olt_onu_mode?: string;
  mikrotik_mode?: string;
};

export type PipelineStep = {
  id: string;
  kind: string;
  enabled: boolean;
  scope: PipelineStepScope;
  options?: PipelineStepOptions;
};

type MonitoringConfigPayload = {
  pipeline_steps: PipelineStep[];
  pipeline_cycle_seconds?: number;
};

const STEP_KINDS = [
  { value: "ping", label: "Ping (ICMP/TCP)" },
  { value: "telemetry", label: "Telemetria SNMP" },
  { value: "olt_onu", label: "Coleta ONUs (OLT)" },
  { value: "mikrotik", label: "MikroTik (interfaces/métricas)" },
  { value: "interfaces_olt", label: "Interfaces SNMP (OLT)" },
  { value: "interfaces_mikrotik", label: "Interfaces SNMP (MikroTik)" },
];

const CATEGORIES = ["olt", "mikrotik", "router", "switch", "radio", "outro"];

const TELEMETRY_FIELDS = [
  { value: "cpu", label: "CPU" },
  { value: "memory", label: "Memória" },
  { value: "uptime", label: "Uptime" },
  { value: "temperature", label: "Temperatura" },
];

function newStep(kind: string): PipelineStep {
  const base: PipelineStep = {
    id: `${kind}-${Date.now()}`,
    kind,
    enabled: true,
    scope: { target: "all" },
    options: {},
  };
  if (kind === "telemetry") base.options = { telemetry_fields: [] };
  if (kind === "olt_onu") {
    base.scope = { target: "category", category: "olt" };
    base.options = { olt_onu_mode: "full" };
  }
  if (kind === "mikrotik") {
    base.scope = { target: "category", category: "mikrotik" };
    base.options = { mikrotik_mode: "full" };
  }
  if (kind === "interfaces_olt") base.scope = { target: "category", category: "olt" };
  if (kind === "interfaces_mikrotik") base.scope = { target: "category", category: "mikrotik" };
  return base;
}

function kindLabel(kind: string): string {
  return STEP_KINDS.find((k) => k.value === kind)?.label ?? kind;
}

export function MonitoringSettingsPanel() {
  return (
    <>
      <MonitoringPingIntervalsCard />
      <MonitoringPipelineCard />
    </>
  );
}

function MonitoringPipelineCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const q = useQuery({
    queryKey: queryKeys.monIntervals,
    queryFn: () => apiFetch<MonitoringConfigPayload>("/api/v1/settings/monitoring-intervals"),
  });
  const [steps, setSteps] = useState<PipelineStep[]>([]);
  const [cycleSec, setCycleSec] = useState("");
  const [addKind, setAddKind] = useState("ping");

  useEffect(() => {
    if (!q.data?.pipeline_steps) return;
    setSteps(q.data.pipeline_steps);
    setCycleSec((v) => (v === "" ? String(q.data.pipeline_cycle_seconds ?? 120) : v));
  }, [q.data]);

  const save = useMutation({
    mutationFn: (body: Partial<MonitoringConfigPayload>) =>
      apiFetch("/api/v1/settings/monitoring-intervals", { method: "PATCH", json: body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.monIntervals });
      toastOk(pushToast, "Ordem de monitoramento guardada.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar pipeline."),
  });

  const move = (idx: number, dir: -1 | 1) => {
    const j = idx + dir;
    if (j < 0 || j >= steps.length) return;
    const next = [...steps];
    [next[idx], next[j]] = [next[j], next[idx]];
    setSteps(next);
  };

  const updateStep = (idx: number, patch: Partial<PipelineStep>) => {
    setSteps((prev) => prev.map((s, i) => (i === idx ? { ...s, ...patch, scope: { ...s.scope, ...patch.scope }, options: { ...s.options, ...patch.options } } : s)));
  };

  if (q.isLoading) return <div className="card"><p>A carregar pipeline…</p></div>;

  return (
    <div className="card">
      <h2 style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        Ordem de monitoramento
        <InfoHint label="Pipeline sequencial">
          <p>
            Telemetria, interfaces e coleta ONU correm <strong>em sequência</strong> (cada etapa só inicia após a anterior).
            O passo <strong>Ping</strong> na lista abaixo serve para definir o alvo (todos/categoria); com «Ping em paralelo» activo
            (Configurações → Intervalos), o ping <strong>não entra na fila</strong> — corre à parte, no intervalo «Intervalo entre pings».
          </p>
        </InfoHint>
      </h2>

      <div className="settings-fields-grid" style={{ marginBottom: 16, maxWidth: 320 }}>
        <label>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Intervalo entre ciclos completos (s)</span>
          <input className="input mono" value={cycleSec} onChange={(e) => setCycleSec(e.target.value)} aria-label="Intervalo pipeline" />
        </label>
      </div>

      <ol style={{ listStyle: "none", padding: 0, margin: 0, display: "flex", flexDirection: "column", gap: 12 }}>
        {steps.map((step, idx) => (
          <li key={step.id} className="card" style={{ padding: 12, margin: 0, background: "var(--surface-2, rgba(0,0,0,.04))" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 10 }}>
              <strong>{idx + 1}.</strong>
              <select
                className="input"
                value={step.kind}
                onChange={(e) => updateStep(idx, { kind: e.target.value })}
                aria-label={`Tipo passo ${idx + 1}`}
              >
                {STEP_KINDS.map((k) => (
                  <option key={k.value} value={k.value}>{k.label}</option>
                ))}
              </select>
              <label style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13 }}>
                <input type="checkbox" checked={step.enabled} onChange={(e) => updateStep(idx, { enabled: e.target.checked })} />
                Activo
              </label>
              <div style={{ marginLeft: "auto", display: "flex", gap: 4 }}>
                <button type="button" className="btn btn--ghost btn--sm" disabled={idx === 0} onClick={() => move(idx, -1)} aria-label="Subir"><ChevronUp size={16} /></button>
                <button type="button" className="btn btn--ghost btn--sm" disabled={idx === steps.length - 1} onClick={() => move(idx, 1)} aria-label="Descer"><ChevronDown size={16} /></button>
                <button type="button" className="btn btn--ghost btn--sm" onClick={() => setSteps((p) => p.filter((_, i) => i !== idx))} aria-label="Remover"><Trash2 size={16} /></button>
              </div>
            </div>

            <div className="settings-fields-grid">
              <label>
                <span style={{ fontSize: 12, color: "var(--muted)" }}>Alvo</span>
                <select
                  className="input"
                  value={step.scope.target}
                  onChange={(e) => updateStep(idx, { scope: { ...step.scope, target: e.target.value as PipelineStepScope["target"] } })}
                >
                  <option value="all">Todos os equipamentos</option>
                  <option value="category">Categoria específica</option>
                  <option value="devices">Equipamentos (IDs UUID)</option>
                </select>
              </label>
              {step.scope.target === "category" && (
                <label>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Categoria</span>
                  <select className="input" value={step.scope.category ?? ""} onChange={(e) => updateStep(idx, { scope: { ...step.scope, category: e.target.value } })}>
                    {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
                  </select>
                </label>
              )}
              {step.scope.target === "devices" && (
                <label style={{ gridColumn: "1 / -1" }}>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>IDs (separados por vírgula)</span>
                  <input
                    className="input mono"
                    value={(step.scope.device_ids ?? []).join(", ")}
                    onChange={(e) =>
                      updateStep(idx, {
                        scope: {
                          ...step.scope,
                          device_ids: e.target.value.split(",").map((x) => x.trim()).filter(Boolean),
                        },
                      })
                    }
                  />
                </label>
              )}

              {step.kind === "telemetry" && (
                <label style={{ gridColumn: "1 / -1" }}>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Métricas (vazio = completo)</span>
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 4 }}>
                    {TELEMETRY_FIELDS.map((f) => {
                      const sel = step.options?.telemetry_fields ?? [];
                      const on = sel.includes(f.value);
                      return (
                        <label key={f.value} style={{ fontSize: 13, display: "flex", gap: 4 }}>
                          <input
                            type="checkbox"
                            checked={on}
                            onChange={(e) => {
                              const next = e.target.checked ? [...sel, f.value] : sel.filter((x) => x !== f.value);
                              updateStep(idx, { options: { ...step.options, telemetry_fields: next } });
                            }}
                          />
                          {f.label}
                        </label>
                      );
                    })}
                  </div>
                </label>
              )}

              {step.kind === "olt_onu" && (
                <label>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Modo coleta ONU</span>
                  <select
                    className="input"
                    value={step.options?.olt_onu_mode ?? "full"}
                    onChange={(e) => updateStep(idx, { options: { ...step.options, olt_onu_mode: e.target.value } })}
                  >
                    <option value="full">Completo</option>
                    <option value="status_only">Só status UP/DOWN</option>
                    <option value="status_rx">Status + RX</option>
                  </select>
                </label>
              )}

              {step.kind === "mikrotik" && (
                <label>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Modo MikroTik</span>
                  <select
                    className="input"
                    value={step.options?.mikrotik_mode ?? "full"}
                    onChange={(e) => updateStep(idx, { options: { ...step.options, mikrotik_mode: e.target.value } })}
                  >
                    <option value="full">Completo</option>
                    <option value="pppoe">Só PPPoE</option>
                    <option value="interfaces">Só interfaces</option>
                    <option value="interface_traffic">Consumo por interface</option>
                  </select>
                </label>
              )}
            </div>
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>{kindLabel(step.kind)}</p>
          </li>
        ))}
      </ol>

      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 16, alignItems: "center" }}>
        <select className="input" value={addKind} onChange={(e) => setAddKind(e.target.value)} style={{ maxWidth: 280 }}>
          {STEP_KINDS.map((k) => <option key={k.value} value={k.value}>{k.label}</option>)}
        </select>
        <button type="button" className="btn btn--ghost" onClick={() => setSteps((p) => [...p, newStep(addKind)])}>
          <Plus size={16} style={{ marginRight: 4 }} /> Adicionar etapa
        </button>
        <button
          type="button"
          className="btn btn--primary"
          disabled={save.isPending || steps.length === 0}
          onClick={() =>
            save.mutate({
              pipeline_steps: steps,
              pipeline_cycle_seconds: cycleSec ? Number(cycleSec) : undefined,
            })
          }
        >
          Salvar ordem de monitoramento
        </button>
      </div>
      {save.isError && <div className="msg msg--err">{errorMessageFromUnknown(save.error)}</div>}
    </div>
  );
}
