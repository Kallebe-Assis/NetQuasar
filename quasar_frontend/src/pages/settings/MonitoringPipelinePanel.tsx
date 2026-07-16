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
  bng_mode?: string;
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
  { value: "telemetry", label: "Telemetria SNMP (CPU, memória, uptime)" },
  { value: "bng", label: "BNG (logins / saúde SNMP)" },
  { value: "mikrotik", label: "MikroTik (interfaces/métricas)" },
  { value: "switch", label: "Switch (interfaces/métricas)" },
  { value: "interfaces_olt", label: "Interfaces SNMP (OLT)" },
  { value: "interfaces_mikrotik", label: "Interfaces SNMP (MikroTik)" },
  { value: "interfaces_switch", label: "Interfaces SNMP (Switch)" },
  { value: "olt_onu", label: "Coleta ONUs (OLT SNMP/telnet)" },
];

const CATEGORIES = ["olt", "bng", "mikrotik", "router", "switch", "radio", "servidor", "outro"];

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
  if (kind === "bng") {
    base.scope = { target: "category", category: "bng" };
    base.options = { bng_mode: "totals" };
  }
  if (kind === "olt_onu") {
    base.scope = { target: "category", category: "olt" };
    base.options = { olt_onu_mode: "full" };
  }
  if (kind === "mikrotik") {
    base.scope = { target: "category", category: "mikrotik" };
    base.options = { mikrotik_mode: "full" };
  }
  if (kind === "switch") {
    base.scope = { target: "category", category: "switch" };
    base.options = { mikrotik_mode: "full" };
  }
  if (kind === "interfaces_olt") base.scope = { target: "category", category: "olt" };
  if (kind === "interfaces_mikrotik") base.scope = { target: "category", category: "mikrotik" };
  if (kind === "interfaces_switch") base.scope = { target: "category", category: "switch" };
  return base;
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
            Telemetria, BNG, interfaces e coleta ONU correm <strong>em sequência</strong> (cada etapa só inicia após a anterior).
            A coleta OLT tem <strong>3 cadências</strong>: status PON (frequente), contagens ONU (intermédia) e completa
            (manual, intervalo ou horário agendado em Intervalos). Cada passo <code>olt_onu</code> só corre quando o seu
            intervalo/agenda estiver vencido.
            Equipamentos com coleta BNG activa são recolhidos no passo <strong>BNG</strong>, não na telemetria genérica.
            O passo <strong>Ping</strong> na lista abaixo serve para definir o alvo (todos/categoria); com «Ping em paralelo» activo
            (Configurações → Intervalos), o ping <strong>não entra na fila</strong> — corre à parte, no intervalo «Intervalo entre pings».
          </p>
        </InfoHint>
      </h2>

      <div className="pipeline-cycle-field" style={{ marginBottom: 10 }}>
        <label className="pipeline-step-field pipeline-step-field--cycle">
          <span className="pipeline-step-field__label">Intervalo entre ciclos (s)</span>
          <input className="input mono" value={cycleSec} onChange={(e) => setCycleSec(e.target.value)} aria-label="Intervalo pipeline" />
        </label>
      </div>

      <ol className="pipeline-steps-list">
        {steps.map((step, idx) => (
          <li key={step.id} className="pipeline-step-card">
            <div className="pipeline-step-card__row">
              <span className="pipeline-step-index" aria-hidden>{idx + 1}.</span>
              <label className="pipeline-step-field pipeline-step-field--kind">
                <span className="pipeline-step-field__label">Tipo</span>
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
              </label>
              <label className="pipeline-step-check">
                <input type="checkbox" checked={step.enabled} onChange={(e) => updateStep(idx, { enabled: e.target.checked })} />
                Activo
              </label>
              <label className="pipeline-step-field pipeline-step-field--target">
                <span className="pipeline-step-field__label">Alvo</span>
                <select
                  className="input"
                  value={step.scope.target}
                  onChange={(e) => updateStep(idx, { scope: { ...step.scope, target: e.target.value as PipelineStepScope["target"] } })}
                >
                  <option value="all">Todos</option>
                  <option value="category">Categoria</option>
                  <option value="devices">Equipamentos</option>
                </select>
              </label>
              {step.scope.target === "category" && (
                <label className="pipeline-step-field pipeline-step-field--category">
                  <span className="pipeline-step-field__label">Categoria</span>
                  <select className="input" value={step.scope.category ?? ""} onChange={(e) => updateStep(idx, { scope: { ...step.scope, category: e.target.value } })}>
                    {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
                  </select>
                </label>
              )}
              {step.kind === "olt_onu" && (
                <label className="pipeline-step-field pipeline-step-field--mode">
                  <span className="pipeline-step-field__label">Modo ONU</span>
                  <select
                    className="input"
                    value={step.options?.olt_onu_mode ?? "full"}
                    onChange={(e) => updateStep(idx, { options: { ...step.options, olt_onu_mode: e.target.value } })}
                  >
                    <option value="pon_status">Status PON (up/down)</option>
                    <option value="onu_counts">Contagens ONU online/offline</option>
                    <option value="status_only">Status PON + ONU</option>
                    <option value="status_rx">Status + RX</option>
                    <option value="full">Completo (SNMP + telnet)</option>
                  </select>
                </label>
              )}
              {(step.kind === "mikrotik" || step.kind === "switch") && (
                <label className="pipeline-step-field pipeline-step-field--mode">
                  <span className="pipeline-step-field__label">
                    {step.kind === "switch" ? "Modo Switch" : "Modo MikroTik"}
                  </span>
                  <select
                    className="input"
                    value={step.options?.mikrotik_mode ?? "full"}
                    onChange={(e) => updateStep(idx, { options: { ...step.options, mikrotik_mode: e.target.value } })}
                  >
                    <option value="full">Completo</option>
                    <option value="pppoe">Só PPPoE</option>
                    <option value="interfaces">Só interfaces</option>
                    <option value="interface_traffic">Tráfego/interface</option>
                  </select>
                </label>
              )}
              {step.kind === "bng" && (
                <label className="pipeline-step-field pipeline-step-field--mode">
                  <span className="pipeline-step-field__label">Modo BNG</span>
                  <select
                    className="input"
                    value={step.options?.bng_mode ?? "totals"}
                    onChange={(e) => updateStep(idx, { options: { ...step.options, bng_mode: e.target.value } })}
                  >
                    <option value="totals">Totais de logins</option>
                    <option value="health">Saúde + sistema</option>
                    <option value="system">Só sistema</option>
                    <option value="full">Perfil completo</option>
                  </select>
                </label>
              )}
              <div className="pipeline-step-actions">
                <button type="button" className="btn btn--ghost btn--sm" disabled={idx === 0} onClick={() => move(idx, -1)} aria-label="Subir"><ChevronUp size={14} /></button>
                <button type="button" className="btn btn--ghost btn--sm" disabled={idx === steps.length - 1} onClick={() => move(idx, 1)} aria-label="Descer"><ChevronDown size={14} /></button>
                <button type="button" className="btn btn--ghost btn--sm" onClick={() => setSteps((p) => p.filter((_, i) => i !== idx))} aria-label="Remover"><Trash2 size={14} /></button>
              </div>
            </div>

            {step.scope.target === "devices" && (
              <label className="pipeline-step-field pipeline-step-field--devices">
                <span className="pipeline-step-field__label">IDs (vírgula)</span>
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
              <div className="pipeline-step-metrics">
                <span className="pipeline-step-field__label" style={{ marginRight: 2 }}>Métricas</span>
                {TELEMETRY_FIELDS.map((f) => {
                  const sel = step.options?.telemetry_fields ?? [];
                  const on = sel.includes(f.value);
                  return (
                    <label key={f.value} className="pipeline-step-metric-check">
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
                <span className="pipeline-step-metrics-hint">vazio = completo</span>
              </div>
            )}
          </li>
        ))}
      </ol>

      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 12, alignItems: "center" }}>
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
