import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  ChevronDown,
  ChevronUp,
  Clock3,
  Gauge,
  GripVertical,
  Info,
  Moon,
  Network,
  Plus,
  Radio,
  Router,
  Server,
  Thermometer,
  Trash2,
  Wifi,
  Zap,
} from "lucide-react";
import { useEffect, useMemo, useState, type Dispatch, type ReactNode, type SetStateAction } from "react";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { MonitoringAdvancedFields, type MonitoringIntervalsPayload } from "./MonitoringIntervalsCard";

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

type MonitoringConfigPayload = Omit<MonitoringIntervalsPayload, "pipeline_steps"> & {
  pipeline_steps: PipelineStep[];
};

type MonState = {
  is_running?: boolean;
  monitoring_mode?: string;
  last_pipeline_cycle_at?: string | null;
  last_cycle_at?: string | null;
  last_started_at?: string | null;
};

type SubTab = "overview" | "pipeline" | "frequencies" | "equipment" | "advanced" | "night";
type ModeChoice = "full" | "partial" | "simple_ping";

const STEP_KINDS = [
  { value: "ping", label: "Ping (Latência)", desc: "Verificação de disponibilidade via ICMP/TCP" },
  { value: "telemetry", label: "SNMP — Telemetria", desc: "Temperatura, uptime, CPU, memória e demais métricas" },
  { value: "bng", label: "BNG — Totais", desc: "PPPoE, IPv4, IPv6 e dual-stack" },
  { value: "mikrotik", label: "Interfaces — MikroTik", desc: "Tráfego, erros, status UP/DOWN e óptica SFP" },
  { value: "switch", label: "Interfaces — Switch", desc: "Tráfego, erros, status UP/DOWN e óptica SFP" },
  { value: "interfaces_olt", label: "Interfaces — OLT", desc: "IF-MIB das OLTs" },
  { value: "interfaces_mikrotik", label: "Interfaces SNMP (MikroTik)", desc: "Walk IF-MIB dedicado" },
  { value: "interfaces_switch", label: "Interfaces SNMP (Switch)", desc: "Walk IF-MIB dedicado" },
  { value: "olt_onu", label: "ONUs / PON — OLT", desc: "Status ONU, status e TX das PONs" },
];

const CATEGORIES = ["olt", "bng", "mikrotik", "router", "switch", "radio", "servidor", "outro"];

const TELEMETRY_FIELDS = [
  { value: "cpu", label: "CPU" },
  { value: "memory", label: "Memória" },
  { value: "uptime", label: "Uptime" },
  { value: "temperature", label: "Temperatura" },
];

const SUB_TABS: { id: SubTab; label: string }[] = [
  { id: "overview", label: "Visão Geral" },
  { id: "pipeline", label: "Ordem de Monitoramento" },
  { id: "frequencies", label: "Frequências e Cronograma" },
  { id: "equipment", label: "Tipos de Equipamentos" },
  { id: "advanced", label: "Avançado" },
  { id: "night", label: "Coleta Noturna" },
];

const DEFAULT_INTERVALS: MonitoringIntervalsPayload = {
  ping_seconds: 30,
  telemetry_seconds: 120,
  interface_snapshot_seconds: 150,
  olt_if_derived_pon_seconds: 180,
  olt_pon_status_seconds: 180,
  olt_onu_counts_seconds: 300,
  olt_full_collect_seconds: 0,
  olt_full_collect_schedule: "03:00",
  pipeline_cycle_seconds: 120,
  telemetry_minutes: 2,
  ping_timeout_ms: 5500,
  telemetry_timeout_ms: 120000,
  interface_snapshot_timeout_ms: 120000,
  olt_if_derived_pon_timeout_ms: 180000,
  olt_onu_telnet_timeout_ms: 600000,
  mikrotik_timeout_ms: 120000,
  bng_timeout_ms: 120000,
  icmp_payload_bytes: 32,
  offline_ping_fail_threshold: 3,
  uptime_restart_alert_minutes: 0,
  ping_parallel: true,
};

function defaultPipelineSteps(): PipelineStep[] {
  return [
    { id: "ping-all", kind: "ping", enabled: true, scope: { target: "all" } },
    { id: "telemetry-all", kind: "telemetry", enabled: true, scope: { target: "all" }, options: { telemetry_fields: [] } },
    {
      id: "bng-monitoring",
      kind: "bng",
      enabled: true,
      scope: { target: "category", category: "bng" },
      options: { bng_mode: "monitoring" },
    },
    {
      id: "mikrotik-if",
      kind: "mikrotik",
      enabled: true,
      scope: { target: "category", category: "mikrotik" },
      options: { mikrotik_mode: "full" },
    },
    {
      id: "switch-if",
      kind: "switch",
      enabled: true,
      scope: { target: "category", category: "switch" },
      options: { mikrotik_mode: "full" },
    },
    { id: "olt-if", kind: "interfaces_olt", enabled: true, scope: { target: "category", category: "olt" } },
    {
      id: "olt-baseline",
      kind: "olt_onu",
      enabled: true,
      scope: { target: "category", category: "olt" },
      options: { olt_onu_mode: "baseline" },
    },
    {
      id: "olt-onu-full",
      kind: "olt_onu",
      enabled: false,
      scope: { target: "category", category: "olt" },
      options: { olt_onu_mode: "full" },
    },
  ];
}

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
    base.options = { bng_mode: "monitoring" };
  }
  if (kind === "olt_onu") {
    base.scope = { target: "category", category: "olt" };
    base.options = { olt_onu_mode: "baseline" };
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

function stepMeta(kind: string) {
  return STEP_KINDS.find((k) => k.value === kind) ?? { value: kind, label: kind, desc: "" };
}

function stepIcon(kind: string, mode?: string): { node: ReactNode; tone: string } {
  if (kind === "ping") return { node: <Wifi size={18} />, tone: "green" };
  if (kind === "telemetry") return { node: <Thermometer size={18} />, tone: "blue" };
  if (kind === "bng") return { node: <Server size={18} />, tone: "indigo" };
  if (kind === "mikrotik" || kind === "interfaces_mikrotik") return { node: <Router size={18} />, tone: "purple" };
  if (kind === "switch" || kind === "interfaces_switch") return { node: <Network size={18} />, tone: "violet" };
  if (kind === "interfaces_olt") return { node: <Radio size={18} />, tone: "teal" };
  if (kind === "olt_onu") {
    if (mode === "pon_status" || mode === "baseline") return { node: <Zap size={18} />, tone: "teal" };
    return { node: <Activity size={18} />, tone: "navy" };
  }
  return { node: <Gauge size={18} />, tone: "blue" };
}

function formatDurationSec(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return "—";
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const r = sec % 60;
  if (r === 0) return `${m} min`;
  return `${m} min ${r}s`;
}

function formatClock(iso?: string | null): { time: string; date: string } {
  if (!iso) return { time: "—", date: "" };
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return { time: "—", date: "" };
  return {
    time: d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
    date: d.toLocaleDateString("pt-BR", { day: "2-digit", month: "short", year: "numeric" }),
  };
}

function nextRunInfo(lastIso: string | null | undefined, cycleSec: number) {
  if (!lastIso || !cycleSec) return { time: "—", date: "", countdown: "" };
  const last = new Date(lastIso).getTime();
  if (Number.isNaN(last)) return { time: "—", date: "", countdown: "" };
  const next = new Date(last + cycleSec * 1000);
  const diff = Math.max(0, Math.round((next.getTime() - Date.now()) / 1000));
  const mm = String(Math.floor(diff / 60)).padStart(2, "0");
  const ss = String(diff % 60).padStart(2, "0");
  return {
    ...formatClock(next.toISOString()),
    countdown: diff > 0 ? `Em ${mm}:${ss}` : "Agora",
  };
}

function frequencyForStep(step: PipelineStep, intervals: MonitoringIntervalsPayload): number {
  switch (step.kind) {
    case "ping":
      return intervals.ping_seconds;
    case "telemetry":
    case "bng":
      return intervals.telemetry_seconds ?? intervals.telemetry_minutes * 60;
    case "mikrotik":
    case "switch":
    case "interfaces_olt":
    case "interfaces_mikrotik":
    case "interfaces_switch":
      return intervals.interface_snapshot_seconds ?? 300;
    case "olt_onu": {
      const mode = step.options?.olt_onu_mode ?? "full";
      if (mode === "pon_status") return intervals.olt_pon_status_seconds ?? 60;
      if (mode === "baseline" || mode === "onu_counts" || mode === "status_only" || mode === "status_rx") {
        return intervals.olt_onu_counts_seconds ?? 180;
      }
      return intervals.olt_full_collect_seconds ?? 0;
    }
    default:
      return intervals.pipeline_cycle_seconds ?? 120;
  }
}

function modeFromRuntime(mode?: string, steps?: PipelineStep[]): ModeChoice {
  const m = String(mode ?? "").toLowerCase();
  if (m === "simple_ping") return "simple_ping";
  if (steps && steps.some((s) => !s.enabled)) return "partial";
  return "full";
}

export function MonitoringSettingsPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [subTab, setSubTab] = useState<SubTab>("overview");
  const [addKind, setAddKind] = useState("ping");
  const [modeChoice, setModeChoice] = useState<ModeChoice>("full");
  const [tick, setTick] = useState(0);

  const [steps, setSteps] = useState<PipelineStep[]>([]);
  const [draft, setDraft] = useState<MonitoringIntervalsPayload>(DEFAULT_INTERVALS);
  const [equip, setEquip] = useState({
    switch: true,
    mikrotik: true,
    olt: true,
    onu: true,
    generic: true,
  });

  const intervalsQ = useQuery({
    queryKey: queryKeys.monIntervals,
    queryFn: () => apiFetch<MonitoringConfigPayload>("/api/v1/settings/monitoring-intervals"),
  });
  const stateQ = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<MonState>("/api/v1/monitoring/state"),
    refetchInterval: 5000,
  });

  useEffect(() => {
    const id = window.setInterval(() => setTick((t) => t + 1), 1000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    if (!intervalsQ.data) return;
    const d = intervalsQ.data;
    setSteps(d.pipeline_steps?.length ? d.pipeline_steps : defaultPipelineSteps());
    setDraft({
      ...DEFAULT_INTERVALS,
      ...d,
      telemetry_seconds: d.telemetry_seconds ?? d.telemetry_minutes * 60,
      ping_parallel: d.ping_parallel !== false,
    });
    setEquip({
      switch: (d.pipeline_steps ?? []).some((s) => s.enabled && (s.kind === "switch" || s.kind === "interfaces_switch")),
      mikrotik: (d.pipeline_steps ?? []).some((s) => s.enabled && (s.kind === "mikrotik" || s.kind === "interfaces_mikrotik")),
      olt: (d.pipeline_steps ?? []).some((s) => s.enabled && (s.kind === "interfaces_olt" || s.kind === "olt_onu")),
      onu: (d.pipeline_steps ?? []).some(
        (s) => s.enabled && s.kind === "olt_onu" && (s.options?.olt_onu_mode ?? "full") !== "pon_status",
      ),
      generic: (d.pipeline_steps ?? []).some((s) => s.enabled && (s.kind === "telemetry" || s.kind === "ping")),
    });
  }, [intervalsQ.data]);

  useEffect(() => {
    if (!stateQ.data) return;
    setModeChoice(modeFromRuntime(stateQ.data.monitoring_mode, intervalsQ.data?.pipeline_steps));
  }, [stateQ.data, intervalsQ.data?.pipeline_steps]);

  const save = useMutation({
    mutationFn: async () => {
      await apiFetch("/api/v1/settings/monitoring-intervals", {
        method: "PATCH",
        json: {
          ...draft,
          pipeline_steps: steps,
          pipeline_cycle_seconds: draft.pipeline_cycle_seconds,
        },
      });
      if (stateQ.data?.is_running) {
        const apiMode = modeChoice === "simple_ping" ? "simple_ping" : "full";
        if (apiMode !== stateQ.data.monitoring_mode) {
          await apiFetch("/api/v1/monitoring/start", { method: "POST", json: { mode: apiMode } });
        }
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.monIntervals });
      void qc.invalidateQueries({ queryKey: queryKeys.monState });
      toastOk(pushToast, "Configurações de monitoramento guardadas.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar configurações."),
  });

  const move = (idx: number, dir: -1 | 1) => {
    const j = idx + dir;
    if (j < 0 || j >= steps.length) return;
    const next = [...steps];
    [next[idx], next[j]] = [next[j], next[idx]];
    setSteps(next);
  };

  const updateStep = (idx: number, patch: Partial<PipelineStep>) => {
    setSteps((prev) =>
      prev.map((s, i) =>
        i === idx
          ? {
              ...s,
              ...patch,
              scope: { ...s.scope, ...patch.scope },
              options: { ...s.options, ...patch.options },
            }
          : s,
      ),
    );
  };

  const applyEquipToggle = (key: keyof typeof equip, on: boolean) => {
    setEquip((e) => ({ ...e, [key]: on }));
    setSteps((prev) =>
      prev.map((s) => {
        const kind = s.kind;
        if (key === "mikrotik" && (kind === "mikrotik" || kind === "interfaces_mikrotik")) return { ...s, enabled: on };
        if (key === "switch" && (kind === "switch" || kind === "interfaces_switch")) return { ...s, enabled: on };
        if (key === "olt" && (kind === "interfaces_olt" || kind === "olt_onu")) return { ...s, enabled: on };
        if (key === "onu" && kind === "olt_onu") return { ...s, enabled: on };
        if (key === "generic" && (kind === "telemetry" || kind === "ping")) return { ...s, enabled: on };
        return s;
      }),
    );
  };

  const restoreDefaults = () => {
    setSteps(defaultPipelineSteps());
    setDraft({ ...DEFAULT_INTERVALS });
    setModeChoice("full");
    setEquip({ switch: true, mikrotik: true, olt: true, onu: true, generic: true });
    toastOk(pushToast, "Padrões restaurados neste formulário. Clique em Salvar para aplicar.");
  };

  const lastPipeline = stateQ.data?.last_pipeline_cycle_at ?? stateQ.data?.last_cycle_at;
  const cycleSec = draft.pipeline_cycle_seconds ?? 120;
  const lastFmt = formatClock(lastPipeline);
  const nextFmt = useMemo(() => nextRunInfo(lastPipeline, cycleSec), [lastPipeline, cycleSec, tick]);
  const running = !!stateQ.data?.is_running;
  const modeLabel =
    modeChoice === "simple_ping" ? "Ping Apenas" : modeChoice === "partial" ? "Parcial" : "Completo";

  if (intervalsQ.isLoading) {
    return (
      <div className="card mon-cfg">
        <p>A carregar configurações de monitoramento…</p>
      </div>
    );
  }
  if (intervalsQ.isError) {
    return (
      <div className="card mon-cfg">
        <div className="msg msg--err">{errorMessageFromUnknown(intervalsQ.error)}</div>
      </div>
    );
  }

  return (
    <div className="mon-cfg">
      <header className="mon-cfg__header">
        <div className="mon-cfg__header-text">
          <h2 className="mon-cfg__title">Configurações de Monitoramento</h2>
          <p className="mon-cfg__subtitle">
            Defina a ordem de execução, frequência e tipos de coleta do pipeline de monitoramento.
          </p>
        </div>
        <div className="mon-cfg__header-actions">
          <button type="button" className="btn btn--ghost" onClick={restoreDefaults}>
            Restaurar Padrões
          </button>
          <button type="button" className="btn btn--primary" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "A guardar…" : "Salvar Configurações"}
          </button>
        </div>
      </header>

      <nav className="mon-cfg__tabs" aria-label="Secções de monitoramento">
        {SUB_TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`mon-cfg__tab${subTab === t.id ? " is-active" : ""}`}
            onClick={() => setSubTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </nav>

      {(subTab === "overview" || subTab === "pipeline" || subTab === "frequencies" || subTab === "equipment") && (
        <div className="mon-cfg__stats" aria-live="polite">
          <StatCard label="Status do Monitoramento">
            <span className={`mon-cfg__status-pill${running ? " is-on" : ""}`}>
              <span className="mon-cfg__status-dot" />
              {running ? "Ativo" : "Parado"}
            </span>
          </StatCard>
          <StatCard label="Modo de Monitoramento">
            <strong>{modeLabel}</strong>
          </StatCard>
          <StatCard label="Equipamentos Alvo">
            <strong>Todos</strong>
          </StatCard>
          <StatCard label="Última Execução (Pipeline)">
            <div className="mon-cfg__stat-time">
              <Clock3 size={16} aria-hidden />
              <div>
                <strong>{lastFmt.time}</strong>
                {lastFmt.date ? <span>{lastFmt.date}</span> : null}
              </div>
            </div>
          </StatCard>
          <StatCard label="Próxima Execução">
            <div className="mon-cfg__stat-time">
              <Clock3 size={16} aria-hidden />
              <div>
                <strong>{nextFmt.time}</strong>
                {nextFmt.countdown ? <span className="mon-cfg__countdown">{nextFmt.countdown}</span> : null}
              </div>
            </div>
          </StatCard>
        </div>
      )}

      {subTab === "overview" && (
        <>
          <div className="mon-cfg__grid">
            <section className="mon-cfg__card">
              <div className="mon-cfg__card-head">
                <h3>Ordem de Monitoramento (Pipeline)</h3>
                <button type="button" className="btn btn--ghost btn--sm" onClick={() => setSubTab("pipeline")}>
                  Editar
                </button>
              </div>
              <ol className="mon-cfg__pipeline">
                {steps.map((step, idx) => {
                  const meta = stepMeta(step.kind);
                  const icon = stepIcon(step.kind, step.options?.olt_onu_mode);
                  const freq = frequencyForStep(step, draft);
                  return (
                    <li key={step.id} className={`mon-cfg__pipe-item${step.enabled ? "" : " is-off"}`}>
                      <span className="mon-cfg__grip" aria-hidden>
                        <GripVertical size={16} />
                      </span>
                      <span className="mon-cfg__pipe-num">{idx + 1}</span>
                      <span className={`mon-cfg__pipe-icon mon-cfg__pipe-icon--${icon.tone}`}>{icon.node}</span>
                      <div className="mon-cfg__pipe-body">
                        <div className="mon-cfg__pipe-title">{meta.label}</div>
                        <div className="mon-cfg__pipe-desc">{meta.desc}</div>
                      </div>
                      <span className={`mon-cfg__badge${step.enabled ? " is-on" : ""}`}>
                        {step.enabled ? "Ativo" : "Off"}
                      </span>
                      <span className="mon-cfg__freq">{formatDurationSec(freq)}</span>
                    </li>
                  );
                })}
              </ol>
              <button
                type="button"
                className="mon-cfg__add-link"
                onClick={() => {
                  setSubTab("pipeline");
                  setSteps((p) => [...p, newStep(addKind)]);
                }}
              >
                <Plus size={16} /> Adicionar etapa ao pipeline
              </button>
            </section>

            <div className="mon-cfg__side">
              <section className="mon-cfg__card">
                <div className="mon-cfg__card-head">
                  <h3>Frequências e Cronograma</h3>
                </div>
                <div className="mon-cfg__freq-list">
                  <FreqRow
                    label="Ping (Latência)"
                    seconds={draft.ping_seconds}
                    onChange={(n) => setDraft((d) => ({ ...d, ping_seconds: n }))}
                  />
                  <FreqRow
                    label="SNMP — Telemetria"
                    seconds={draft.telemetry_seconds ?? draft.telemetry_minutes * 60}
                    onChange={(n) => setDraft((d) => ({ ...d, telemetry_seconds: n }))}
                  />
                  <FreqRow
                    label="Interfaces (MikroTik/Switch)"
                    seconds={draft.interface_snapshot_seconds ?? 300}
                    onChange={(n) => setDraft((d) => ({ ...d, interface_snapshot_seconds: n }))}
                  />
                  <FreqRow
                    label="PON — OLT (status/TX)"
                    seconds={draft.olt_pon_status_seconds ?? 60}
                    onChange={(n) => setDraft((d) => ({ ...d, olt_pon_status_seconds: n }))}
                  />
                  <FreqRow
                    label="ONUs — OLT (status)"
                    seconds={draft.olt_onu_counts_seconds ?? 180}
                    onChange={(n) => setDraft((d) => ({ ...d, olt_onu_counts_seconds: n }))}
                  />
                  <FreqRow
                    label="Ciclo completo do pipeline"
                    seconds={draft.pipeline_cycle_seconds ?? 120}
                    onChange={(n) => setDraft((d) => ({ ...d, pipeline_cycle_seconds: n }))}
                  />
                </div>
              </section>

              <section className="mon-cfg__card">
                <div className="mon-cfg__card-head">
                  <h3>Tipos de Equipamentos Alvo</h3>
                </div>
                <div className="mon-cfg__equip-list">
                  <EquipToggle
                    label="Roteadores / Switches (SNMP)"
                    checked={equip.switch}
                    onChange={(v) => applyEquipToggle("switch", v)}
                  />
                  <EquipToggle label="MikroTik" checked={equip.mikrotik} onChange={(v) => applyEquipToggle("mikrotik", v)} />
                  <EquipToggle label="OLT" checked={equip.olt} onChange={(v) => applyEquipToggle("olt", v)} />
                  <EquipToggle label="ONUs" checked={equip.onu} onChange={(v) => applyEquipToggle("onu", v)} />
                  <EquipToggle
                    label="Demais Equipamentos (Genéricos)"
                    checked={equip.generic}
                    onChange={(v) => applyEquipToggle("generic", v)}
                  />
                </div>
              </section>
            </div>
          </div>

          <section className="mon-cfg__card mon-cfg__modes">
            <div className="mon-cfg__card-head">
              <h3>Modo de Monitoramento</h3>
            </div>
            <div className="mon-cfg__mode-grid">
              <ModeCard
                title="Completo"
                desc="Executa todos os módulos activos do pipeline na ordem definida."
                selected={modeChoice === "full"}
                badge="Recomendado"
                onSelect={() => setModeChoice("full")}
              />
              <ModeCard
                title="Parcial"
                desc="Executa apenas os módulos activos que seleccionar no pipeline."
                selected={modeChoice === "partial"}
                onSelect={() => setModeChoice("partial")}
              />
              <ModeCard
                title="Ping Apenas"
                desc="Apenas verificação de disponibilidade (ICMP/TCP), sem SNMP/OLT."
                selected={modeChoice === "simple_ping"}
                onSelect={() => setModeChoice("simple_ping")}
              />
            </div>
            <div className="mon-cfg__info-banner">
              <Info size={16} aria-hidden />
              <p>
                {modeChoice === "simple_ping"
                  ? "No modo Ping Apenas, o worker ignora telemetria, interfaces e OLT."
                  : modeChoice === "partial"
                    ? "No modo Parcial, só os passos marcados como activos no pipeline são executados."
                    : "No modo Completo, todos os módulos activos no pipeline serão executados na ordem definida."}
              </p>
            </div>
          </section>

          <div className="mon-cfg__footer-links">
            <FooterLink icon={<Info size={16} />} label="Informações Importantes" onClick={() => setSubTab("advanced")} />
            <FooterLink icon={<Gauge size={16} />} label="Timeouts" onClick={() => setSubTab("advanced")} />
            <FooterLink icon={<Network size={16} />} label="Impacto na Rede" onClick={() => setSubTab("advanced")} />
            <FooterLink icon={<Moon size={16} />} label="Coleta Noturna" onClick={() => setSubTab("night")} />
            <FooterLink icon={<Clock3 size={16} />} label="Histórico" onClick={() => setSubTab("frequencies")} />
          </div>
        </>
      )}

      {subTab === "pipeline" && (
        <section className="mon-cfg__card">
          <div className="mon-cfg__card-head">
            <h3>Ordem de Monitoramento</h3>
            <p className="mon-cfg__card-hint">
              Reordene as etapas, active/desactive módulos e ajuste alvo ou modo de coleta. Cada etapa corre em sequência no ciclo.
            </p>
          </div>
          <PipelineEditor
            steps={steps}
            draft={draft}
            addKind={addKind}
            setAddKind={setAddKind}
            move={move}
            updateStep={updateStep}
            setSteps={setSteps}
          />
        </section>
      )}

      {subTab === "frequencies" && (
        <section className="mon-cfg__card">
          <div className="mon-cfg__card-head">
            <h3>Frequências e Cronograma</h3>
          </div>
          <div className="mon-cfg__freq-list mon-cfg__freq-list--wide">
            <FreqRow label="Ciclo completo do pipeline" seconds={draft.pipeline_cycle_seconds ?? 120} onChange={(n) => setDraft((d) => ({ ...d, pipeline_cycle_seconds: n }))} />
            <FreqRow label="Ping (Latência)" seconds={draft.ping_seconds} onChange={(n) => setDraft((d) => ({ ...d, ping_seconds: n }))} />
            <FreqRow label="Telemetria SNMP / BNG" seconds={draft.telemetry_seconds ?? 120} onChange={(n) => setDraft((d) => ({ ...d, telemetry_seconds: n }))} />
            <FreqRow label="Interfaces SNMP" seconds={draft.interface_snapshot_seconds ?? 300} onChange={(n) => setDraft((d) => ({ ...d, interface_snapshot_seconds: n }))} />
            <FreqRow label="OLT PON IF" seconds={draft.olt_if_derived_pon_seconds ?? 240} onChange={(n) => setDraft((d) => ({ ...d, olt_if_derived_pon_seconds: n }))} />
            <FreqRow label="OLT status PON" seconds={draft.olt_pon_status_seconds ?? 60} onChange={(n) => setDraft((d) => ({ ...d, olt_pon_status_seconds: n }))} />
            <FreqRow label="OLT contagens / status ONU" seconds={draft.olt_onu_counts_seconds ?? 180} onChange={(n) => setDraft((d) => ({ ...d, olt_onu_counts_seconds: n }))} />
            <FreqRow label="OLT coleta completa (0 = só agenda/manual)" seconds={draft.olt_full_collect_seconds ?? 0} onChange={(n) => setDraft((d) => ({ ...d, olt_full_collect_seconds: n }))} allowZero />
          </div>
          <label className="mon-cfg__check">
            <input
              type="checkbox"
              checked={draft.ping_parallel !== false}
              onChange={(e) => setDraft((d) => ({ ...d, ping_parallel: e.target.checked }))}
            />
            Ping em paralelo ao pipeline (recomendado)
          </label>
        </section>
      )}

      {subTab === "equipment" && (
        <section className="mon-cfg__card">
          <div className="mon-cfg__card-head">
            <h3>Tipos de Equipamentos Alvo</h3>
            <p className="mon-cfg__card-hint">Desactivar um tipo desliga as etapas correspondentes no pipeline.</p>
          </div>
          <div className="mon-cfg__equip-list mon-cfg__equip-list--wide">
            <EquipToggle label="Roteadores / Switches (SNMP)" checked={equip.switch} onChange={(v) => applyEquipToggle("switch", v)} />
            <EquipToggle label="MikroTik" checked={equip.mikrotik} onChange={(v) => applyEquipToggle("mikrotik", v)} />
            <EquipToggle label="OLT" checked={equip.olt} onChange={(v) => applyEquipToggle("olt", v)} />
            <EquipToggle label="ONUs" checked={equip.onu} onChange={(v) => applyEquipToggle("onu", v)} />
            <EquipToggle label="Demais Equipamentos (Genéricos)" checked={equip.generic} onChange={(v) => applyEquipToggle("generic", v)} />
          </div>
        </section>
      )}

      {subTab === "advanced" && (
        <section className="mon-cfg__card">
          <div className="mon-cfg__card-head">
            <h3>Avançado — Timeouts e ICMP</h3>
          </div>
          <MonitoringAdvancedFields draft={draft} setDraft={setDraft} />
        </section>
      )}

      {subTab === "night" && (
        <section className="mon-cfg__card">
          <div className="mon-cfg__card-head">
            <h3>Coleta Noturna</h3>
            <p className="mon-cfg__card-hint">
              Agenda diária para a coleta completa OLT (serial, RX, modelo, telnet). Use intervalo 0 nas frequências para
              depender só deste horário.
            </p>
          </div>
          <label className="mon-cfg__night-field">
            <span>Horário da coleta completa (HH:MM)</span>
            <input
              className="input mono"
              placeholder="03:00"
              value={draft.olt_full_collect_schedule ?? ""}
              onChange={(e) => setDraft((d) => ({ ...d, olt_full_collect_schedule: e.target.value }))}
            />
          </label>
          <FreqRow
            label="Intervalo automático da coleta completa (0 = desligado)"
            seconds={draft.olt_full_collect_seconds ?? 0}
            onChange={(n) => setDraft((d) => ({ ...d, olt_full_collect_seconds: n }))}
            allowZero
          />
        </section>
      )}

      {save.isError && <div className="msg msg--err">{errorMessageFromUnknown(save.error)}</div>}
    </div>
  );
}

function StatCard({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="mon-cfg__stat">
      <span className="mon-cfg__stat-label">{label}</span>
      <div className="mon-cfg__stat-value">{children}</div>
    </div>
  );
}

function FreqRow({
  label,
  seconds,
  onChange,
  allowZero,
}: {
  label: string;
  seconds: number;
  onChange: (n: number) => void;
  allowZero?: boolean;
}) {
  const useMinutes = seconds >= 60 && seconds % 60 === 0;
  const unit = useMinutes ? "minutes" : "seconds";
  const display = unit === "minutes" ? Math.round(seconds / 60) : seconds;

  return (
    <div className="mon-cfg__freq-row">
      <div className="mon-cfg__freq-label">
        <span>{label}</span>
        <small>A cada</small>
      </div>
      <div className="mon-cfg__freq-controls">
        <input
          className="input mono"
          type="number"
          min={allowZero ? 0 : 1}
          value={Number.isFinite(display) ? display : 0}
          onChange={(e) => {
            const n = Number(e.target.value);
            if (!Number.isFinite(n)) return;
            onChange(unit === "minutes" ? n * 60 : n);
          }}
          aria-label={label}
        />
        <select
          className="input"
          value={unit}
          onChange={(e) => {
            const next = e.target.value as "seconds" | "minutes";
            if (next === "minutes") {
              const mins = Math.max(allowZero ? 0 : 1, Math.round(seconds / 60) || 1);
              onChange(mins * 60);
            } else {
              onChange(Math.max(allowZero ? 0 : 1, seconds || 30));
            }
          }}
        >
          <option value="seconds">segundos</option>
          <option value="minutes">minutos</option>
        </select>
      </div>
    </div>
  );
}

function EquipToggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="mon-cfg__equip">
      <span>{label}</span>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        className={`mon-cfg__switch${checked ? " is-on" : ""}`}
        onClick={() => onChange(!checked)}
      >
        <span className="mon-cfg__switch-thumb" />
      </button>
    </label>
  );
}

function ModeCard({
  title,
  desc,
  selected,
  badge,
  onSelect,
}: {
  title: string;
  desc: string;
  selected: boolean;
  badge?: string;
  onSelect: () => void;
}) {
  return (
    <button type="button" className={`mon-cfg__mode${selected ? " is-selected" : ""}`} onClick={onSelect}>
      <span className="mon-cfg__mode-radio" aria-hidden />
      <span className="mon-cfg__mode-body">
        <span className="mon-cfg__mode-title">
          {title}
          {badge ? <em>{badge}</em> : null}
        </span>
        <span className="mon-cfg__mode-desc">{desc}</span>
      </span>
    </button>
  );
}

function FooterLink({ icon, label, onClick }: { icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button type="button" className="mon-cfg__footer-card" onClick={onClick}>
      {icon}
      <span>{label}</span>
    </button>
  );
}

function PipelineEditor({
  steps,
  draft,
  addKind,
  setAddKind,
  move,
  updateStep,
  setSteps,
}: {
  steps: PipelineStep[];
  draft: MonitoringIntervalsPayload;
  addKind: string;
  setAddKind: (v: string) => void;
  move: (idx: number, dir: -1 | 1) => void;
  updateStep: (idx: number, patch: Partial<PipelineStep>) => void;
  setSteps: Dispatch<SetStateAction<PipelineStep[]>>;
}) {
  const [expanded, setExpanded] = useState<string | null>(null);

  return (
    <>
      <ol className="mon-cfg__pipeline mon-cfg__pipeline--edit">
        {steps.map((step, idx) => {
          const meta = stepMeta(step.kind);
          const icon = stepIcon(step.kind, step.options?.olt_onu_mode);
          const freq = frequencyForStep(step, draft);
          const open = expanded === step.id;
          return (
            <li key={step.id} className={`mon-cfg__pipe-edit${step.enabled ? "" : " is-off"}${open ? " is-open" : ""}`}>
              <div className="mon-cfg__pipe-item">
                <span className="mon-cfg__grip" aria-hidden>
                  <GripVertical size={16} />
                </span>
                <span className="mon-cfg__pipe-num">{idx + 1}</span>
                <span className={`mon-cfg__pipe-icon mon-cfg__pipe-icon--${icon.tone}`}>{icon.node}</span>
                <div className="mon-cfg__pipe-body">
                  <div className="mon-cfg__pipe-title">{meta.label}</div>
                  <div className="mon-cfg__pipe-desc">{meta.desc}</div>
                </div>
                <button
                  type="button"
                  className={`mon-cfg__badge mon-cfg__badge-btn${step.enabled ? " is-on" : ""}`}
                  onClick={() => updateStep(idx, { enabled: !step.enabled })}
                  aria-pressed={step.enabled}
                >
                  {step.enabled ? "Ativo" : "Off"}
                </button>
                <span className="mon-cfg__freq">{formatDurationSec(freq)}</span>
                <div className="mon-cfg__pipe-actions">
                  <button type="button" className="btn btn--ghost btn--sm" disabled={idx === 0} onClick={() => move(idx, -1)} aria-label="Subir">
                    <ChevronUp size={14} />
                  </button>
                  <button
                    type="button"
                    className="btn btn--ghost btn--sm"
                    disabled={idx === steps.length - 1}
                    onClick={() => move(idx, 1)}
                    aria-label="Descer"
                  >
                    <ChevronDown size={14} />
                  </button>
                  <button
                    type="button"
                    className={`btn btn--ghost btn--sm${open ? " is-active" : ""}`}
                    onClick={() => setExpanded(open ? null : step.id)}
                    aria-expanded={open}
                    aria-label={open ? "Ocultar opções" : "Mostrar opções"}
                  >
                    {open ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                  </button>
                  <button
                    type="button"
                    className="btn btn--ghost btn--sm"
                    onClick={() => {
                      setExpanded((cur) => (cur === step.id ? null : cur));
                      setSteps((p) => p.filter((_, i) => i !== idx));
                    }}
                    aria-label="Remover"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>

              {open ? (
                <div className="mon-cfg__pipe-options">
                  <label className="mon-cfg__opt">
                    <span>Tipo</span>
                    <select className="input" value={step.kind} onChange={(e) => updateStep(idx, { kind: e.target.value })}>
                      {STEP_KINDS.map((k) => (
                        <option key={k.value} value={k.value}>
                          {k.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="mon-cfg__opt">
                    <span>Alvo</span>
                    <select
                      className="input"
                      value={step.scope.target}
                      onChange={(e) =>
                        updateStep(idx, { scope: { ...step.scope, target: e.target.value as PipelineStepScope["target"] } })
                      }
                    >
                      <option value="all">Todos</option>
                      <option value="category">Categoria</option>
                      <option value="devices">Equipamentos</option>
                    </select>
                  </label>
                  {step.scope.target === "category" && (
                    <label className="mon-cfg__opt">
                      <span>Categoria</span>
                      <select
                        className="input"
                        value={step.scope.category ?? ""}
                        onChange={(e) => updateStep(idx, { scope: { ...step.scope, category: e.target.value } })}
                      >
                        {CATEGORIES.map((c) => (
                          <option key={c} value={c}>
                            {c}
                          </option>
                        ))}
                      </select>
                    </label>
                  )}
                  {step.kind === "olt_onu" && (
                    <label className="mon-cfg__opt mon-cfg__opt--wide">
                      <span>Modo ONU</span>
                      <select
                        className="input"
                        value={step.options?.olt_onu_mode ?? "full"}
                        onChange={(e) => updateStep(idx, { options: { ...step.options, olt_onu_mode: e.target.value } })}
                      >
                        <option value="baseline">Linha-base: status ONU + status/TX PON</option>
                        <option value="pon_status">Status PON (up/down)</option>
                        <option value="onu_counts">Contagens ONU online/offline</option>
                        <option value="status_only">Status PON + ONU</option>
                        <option value="status_rx">Status + RX</option>
                        <option value="full">Completo (SNMP + telnet)</option>
                      </select>
                    </label>
                  )}
                  {(step.kind === "mikrotik" || step.kind === "switch") && (
                    <label className="mon-cfg__opt">
                      <span>{step.kind === "switch" ? "Modo Switch" : "Modo MikroTik"}</span>
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
                    <label className="mon-cfg__opt mon-cfg__opt--wide">
                      <span>Modo BNG</span>
                      <select
                        className="input"
                        value={step.options?.bng_mode ?? "totals"}
                        onChange={(e) => updateStep(idx, { options: { ...step.options, bng_mode: e.target.value } })}
                      >
                        <option value="monitoring">Linha-base: sistema + saúde + totais</option>
                        <option value="totals">Totais de logins</option>
                        <option value="health">Saúde + sistema</option>
                        <option value="system">Só sistema</option>
                        <option value="full">Perfil completo</option>
                      </select>
                    </label>
                  )}
                  {step.scope.target === "devices" && (
                    <label className="mon-cfg__opt mon-cfg__opt--wide">
                      <span>IDs (vírgula)</span>
                      <input
                        className="input mono"
                        value={(step.scope.device_ids ?? []).join(", ")}
                        onChange={(e) =>
                          updateStep(idx, {
                            scope: {
                              ...step.scope,
                              device_ids: e.target.value
                                .split(",")
                                .map((x) => x.trim())
                                .filter(Boolean),
                            },
                          })
                        }
                      />
                    </label>
                  )}
                  {step.kind === "telemetry" && (
                    <div className="mon-cfg__opt mon-cfg__opt--wide mon-cfg__opt-metrics">
                      <span>Métricas</span>
                      <div className="mon-cfg__metric-chips">
                        {TELEMETRY_FIELDS.map((f) => {
                          const sel = step.options?.telemetry_fields ?? [];
                          const on = sel.includes(f.value);
                          return (
                            <label key={f.value} className={`mon-cfg__chip${on ? " is-on" : ""}`}>
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
                        <span className="mon-cfg__chip-hint">vazio = completo</span>
                      </div>
                    </div>
                  )}
                </div>
              ) : null}
            </li>
          );
        })}
      </ol>

      <div className="mon-cfg__pipeline-add">
        <select className="input" value={addKind} onChange={(e) => setAddKind(e.target.value)} aria-label="Tipo da nova etapa">
          {STEP_KINDS.map((k) => (
            <option key={k.value} value={k.value}>
              {k.label}
            </option>
          ))}
        </select>
        <button type="button" className="mon-cfg__add-link" onClick={() => setSteps((p) => [...p, newStep(addKind)])}>
          <Plus size={16} /> Adicionar etapa ao pipeline
        </button>
      </div>
    </>
  );
}
