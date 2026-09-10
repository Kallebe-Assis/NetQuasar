import { useMemo } from "react";

/** Campos do GET /api/v1/monitoring/state que este resumo usa. */
export type MonitorEngineState = {
  is_running?: boolean;
  monitoring_mode?: string;
  runtime_updated_at?: string | null;
  last_cycle_at?: string | null;
  last_pipeline_cycle_at?: string | null;
  last_telemetry_cycle_at?: string | null;
  last_cycle_ok_count?: number | null;
  last_cycle_fail_count?: number | null;
  current_activity?: string | null;
  last_activity?: string | null;
  last_activity_finished_at?: string | null;
  last_internet_check_ok?: boolean | null;
  last_internet_check_at?: string | null;
  last_internet_check_detail?: { error_detail?: string; error_code?: string } | Record<string, unknown> | null;
};

type Props = {
  state?: MonitorEngineState;
  /** Intervalos (GET /api/v1/settings/monitoring-intervals) — usados para estimar o próximo ciclo. */
  intervals?: { pipeline_cycle_seconds?: number; ping_seconds?: number } | undefined;
  /** Muda a cada tick do relógio da página para o "próximo ciclo em ~Xs" ir andando. */
  agoTick?: number;
  modeLabel: string;
};

type Tone = "ok" | "warn" | "err";

const TONE_COLOR: Record<Tone, string> = {
  ok: "#16a34a",
  warn: "#d29922",
  err: "#f85149",
};

function fmtDur(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return `${h}h${String(m % 60).padStart(2, "0")}`;
}

function fmtAgo(iso: string | null | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const diff = Date.now() - t;
  if (diff < 0) return "agora";
  if (diff < 60_000) return `há ${Math.round(diff / 1000)}s`;
  if (diff < 3_600_000) return `há ${Math.round(diff / 60_000)} min`;
  if (diff < 86_400_000) return `há ${Math.round(diff / 3_600_000)}h`;
  return new Date(iso).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
}

/**
 * Bloco-resumo do estado do motor de monitoramento: semáforo (verde/âmbar/vermelho),
 * próxima ação prevista e último erro — em vez de linhas de texto soltas.
 */
export function MonitorEngineSummary({ state, intervals, agoTick, modeLabel }: Props) {
  void agoTick; // só força re-render; os cálculos abaixo usam Date.now()

  const view = useMemo(() => {
    const running = !!state?.is_running;
    const now = Date.now();
    const lastCycleIso =
      state?.last_cycle_at ?? state?.last_pipeline_cycle_at ?? state?.last_telemetry_cycle_at ?? null;
    const cycleSecs = Math.max(
      30,
      intervals?.pipeline_cycle_seconds ?? intervals?.ping_seconds ?? 60,
    );
    const sinceCycleMs = lastCycleIso ? now - new Date(lastCycleIso).getTime() : null;
    const stale = running && sinceCycleMs != null && sinceCycleMs > Math.max(cycleSecs * 3, 300) * 1000;
    const failCount = Number(state?.last_cycle_fail_count ?? 0);
    const okCount = Number(state?.last_cycle_ok_count ?? 0);

    let tone: Tone;
    let title: string;
    if (!running) {
      tone = "err";
      title = "Motor parado";
    } else if (stale || failCount > 0) {
      tone = "warn";
      title = stale ? "Motor ativo — sem ciclos recentes" : "Motor ativo — com falhas no último ciclo";
    } else {
      tone = "ok";
      title = "Motor ativo";
    }

    let nextAction: string;
    if (!running) {
      nextAction = 'Clique em "Iniciar monitoramento" para ligar o motor.';
    } else if (state?.current_activity && String(state.current_activity).trim()) {
      nextAction = `Em execução agora: ${String(state.current_activity).trim()}`;
    } else if (lastCycleIso) {
      const nextInMs = cycleSecs * 1000 - (sinceCycleMs ?? 0);
      nextAction =
        nextInMs > 1000
          ? `Próximo ciclo do pipeline em ~${fmtDur(nextInMs)}`
          : "Próximo ciclo do pipeline a arrancar…";
    } else {
      nextAction = "A aguardar o primeiro ciclo do pipeline…";
    }

    // Só conta como "erro atual" se o motor está ligado ou o último ciclo foi recente — senão é
    // um número antigo (motor parado há dias) e mostrá-lo assusta sem motivo.
    const cycleIsRecent = sinceCycleMs != null && sinceCycleMs < 3_600_000;
    let lastError: string | null = null;
    if (failCount > 0 && (running || cycleIsRecent)) {
      lastError = `Último ciclo: ${failCount} equipamento(s) com falha${okCount ? ` · ${okCount} ok` : ""}.`;
    } else if (state?.last_internet_check_ok === false) {
      const d = (state?.last_internet_check_detail ?? {}) as { error_detail?: string; error_code?: string };
      const extra = d.error_detail || d.error_code || "";
      lastError = `Internet inacessível na última verificação${extra ? ` — ${extra}` : ""} (${fmtAgo(state?.last_internet_check_at)}).`;
    }

    return { tone, title, nextAction, lastError, lastCycleIso, running };
  }, [state, intervals, agoTick]);

  const color = TONE_COLOR[view.tone];

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "auto 1fr",
        gap: "10px 14px",
        alignItems: "start",
        padding: "12px 14px",
        borderRadius: 10,
        border: "1px solid var(--border)",
        borderLeft: `4px solid ${color}`,
        background: "color-mix(in srgb, var(--panel2) 55%, transparent)",
      }}
    >
      <span
        aria-hidden
        style={{
          width: 14,
          height: 14,
          borderRadius: "50%",
          background: color,
          marginTop: 3,
          boxShadow: `0 0 0 4px color-mix(in srgb, ${color} 22%, transparent)`,
        }}
      />
      <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
        <div style={{ display: "flex", alignItems: "baseline", gap: 8, flexWrap: "wrap" }}>
          <strong style={{ fontSize: 14, color: "var(--text)" }}>{view.title}</strong>
          <span
            style={{
              fontSize: 11,
              padding: "1px 7px",
              borderRadius: 999,
              border: "1px solid var(--border)",
              color: "var(--muted)",
            }}
          >
            {modeLabel}
          </span>
        </div>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>
          Último ciclo {fmtAgo(view.lastCycleIso)} · estado no servidor {fmtAgo(state?.runtime_updated_at)}
        </span>
      </div>

      <span style={{ fontSize: 11, color: "var(--muted)", gridColumn: "1 / -1", borderTop: "1px dashed var(--border)", paddingTop: 8 }}>
        Próxima ação
      </span>
      <div style={{ gridColumn: "2", marginTop: -6 }}>
        <span style={{ fontSize: 13, color: "var(--text)" }}>{view.nextAction}</span>
      </div>

      <span style={{ fontSize: 11, color: "var(--muted)", gridColumn: "1 / -1" }}>Último erro</span>
      <div style={{ gridColumn: "2", marginTop: -6 }}>
        <span style={{ fontSize: 13, color: view.lastError ? TONE_COLOR.err : "var(--muted)" }}>
          {view.lastError ?? "Sem erros recentes."}
        </span>
      </div>
    </div>
  );
}
