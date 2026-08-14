export const TZ_DEFAULT = "America/Sao_Paulo";

export const WEEKDAY_LABELS = ["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"] as const;

export type AutomationJobType =
  | "database_backup"
  | "onu_monthly_report"
  | "bng_stats_report"
  | "alerts_digest"
  | "commercial_report";

export type RecurrenceKind = "daily" | "weekly" | "monthly" | "custom";

export type AutomationJobDef = {
  id: AutomationJobType;
  label: string;
  description: string;
  category: string;
  recurrences: RecurrenceKind[];
  patchPath: string;
  runPath: string;
};

export const AUTOMATION_JOBS: AutomationJobDef[] = [
  {
    id: "database_backup",
    label: "Backup PostgreSQL",
    description: "Dump completo da base enviado para o bucket Backblaze B2.",
    category: "Sistema",
    recurrences: ["daily", "weekly", "custom"],
    patchPath: "/api/v1/settings/automation/database-backup",
    runPath: "/api/v1/settings/automation/database-backup/run",
  },
  {
    id: "alerts_digest",
    label: "Resumo de alertas",
    description: "Contagem de alertas abertos e incidentes por Telegram e/ou e-mail.",
    category: "Relatórios",
    recurrences: ["daily", "weekly", "custom"],
    patchPath: "/api/v1/settings/automation/alerts-digest",
    runPath: "/api/v1/settings/automation/alerts-digest/run",
  },
  {
    id: "bng_stats_report",
    label: "Totais BNG",
    description: "Totais de logins PPPoE, IPv4, IPv6 e dual-stack.",
    category: "Relatórios",
    recurrences: ["daily", "weekly", "custom"],
    patchPath: "/api/v1/settings/automation/bng-stats-report",
    runPath: "/api/v1/settings/automation/bng-stats-report/run",
  },
  {
    id: "onu_monthly_report",
    label: "Relatório ONU",
    description: "Recolhe snapshots das OLTs e envia o resumo mensal de ONUs.",
    category: "Relatórios",
    recurrences: ["monthly"],
    patchPath: "/api/v1/settings/automation/onu-monthly-report",
    runPath: "/api/v1/settings/automation/onu-monthly-report/run",
  },
  {
    id: "commercial_report",
    label: "Base comercial",
    description: "Relatório mensal da base comercial, sem recolher OLTs.",
    category: "Relatórios",
    recurrences: ["monthly"],
    patchPath: "/api/v1/settings/automation/commercial-report",
    runPath: "/api/v1/settings/automation/commercial-report/run",
  },
];

export function automationJobDef(id: string): AutomationJobDef | undefined {
  return AUTOMATION_JOBS.find((j) => j.id === id);
}

export type RecurrenceDraft = {
  kind: RecurrenceKind;
  time: string;
  timezone: string;
  weekdays: number[];
  dayOfMonth: number;
};

export function emptyRecurrence(kind: RecurrenceKind = "daily"): RecurrenceDraft {
  return {
    kind,
    time: "08:00",
    timezone: TZ_DEFAULT,
    weekdays: kind === "weekly" ? [1] : kind === "custom" ? [1, 2, 3, 4, 5] : [],
    dayOfMonth: 1,
  };
}

export function recurrenceLabel(kind: RecurrenceKind): string {
  switch (kind) {
    case "daily":
      return "Diária";
    case "weekly":
      return "Semanal";
    case "monthly":
      return "Mensal";
    case "custom":
      return "Personalizada";
  }
}

export function formatRecurrence(draft: RecurrenceDraft): string {
  const hhmm = (draft.time || "08:00").slice(0, 5);
  if (draft.kind === "daily") return `Todos os dias às ${hhmm}`;
  if (draft.kind === "monthly") return `Todo dia ${draft.dayOfMonth} às ${hhmm}`;
  const days = (draft.weekdays.length ? draft.weekdays : [1])
    .slice()
    .sort((a, b) => a - b)
    .map((d) => WEEKDAY_LABELS[d] ?? d);
  if (draft.kind === "weekly" && days.length <= 1) return `Toda ${days[0] ?? "Seg"} às ${hhmm}`;
  return `${days.join(", ")} às ${hhmm}`;
}

export function draftFromJob(job: {
  frequency?: string | null;
  day_of_week?: number | null;
  day_of_month?: number | null;
  time_hhmm?: string | null;
  timezone?: string | null;
  days_of_week?: number[] | null;
}): RecurrenceDraft {
  const time = (job.time_hhmm ?? "08:00").slice(0, 5);
  const timezone = job.timezone?.trim() || TZ_DEFAULT;
  const customDays = (job.days_of_week ?? []).filter((d) => d >= 0 && d <= 6);
  if (customDays.length > 1 || job.frequency === "custom") {
    return { kind: "custom", time, timezone, weekdays: customDays.length ? customDays : [job.day_of_week ?? 1], dayOfMonth: 1 };
  }
  if (job.frequency === "monthly" || (job.day_of_month != null && job.frequency !== "daily" && job.frequency !== "weekly")) {
    return { kind: "monthly", time, timezone, weekdays: [], dayOfMonth: job.day_of_month || 1 };
  }
  if (job.frequency === "weekly") {
    return {
      kind: "weekly",
      time,
      timezone,
      weekdays: customDays.length ? customDays : [job.day_of_week ?? 1],
      dayOfMonth: 1,
    };
  }
  return { kind: "daily", time, timezone, weekdays: [], dayOfMonth: 1 };
}

/** Payload PATCH compatível com os endpoints singleton actuais. */
export function recurrenceToPatch(jobId: AutomationJobType, draft: RecurrenceDraft, enabled = true): Record<string, unknown> {
  const time_hhmm = (draft.time || "08:00").slice(0, 5);
  const timezone = draft.timezone.trim() || TZ_DEFAULT;
  const weekdays = [...new Set(draft.weekdays.filter((d) => d >= 0 && d <= 6))].sort((a, b) => a - b);

  if (jobId === "onu_monthly_report" || jobId === "commercial_report") {
    return {
      enabled,
      day_of_month: draft.kind === "monthly" ? draft.dayOfMonth || 1 : draft.dayOfMonth || 1,
      time_hhmm,
      timezone,
    };
  }

  if (draft.kind === "daily" || (draft.kind === "custom" && weekdays.length >= 7)) {
    return { enabled, frequency: "daily", time_hhmm, timezone, days_of_week: [] };
  }
  const dow = weekdays[0] ?? 1;
  return {
    enabled,
    frequency: weekdays.length > 1 ? "custom" : "weekly",
    day_of_week: dow,
    days_of_week: weekdays.length ? weekdays : [dow],
    time_hhmm,
    timezone,
  };
}

export function daysFromSchedule(cfg: {
  frequency?: string | null;
  day_of_week?: number | null;
  days_of_week?: number[] | null;
}): number[] {
  const custom = (cfg.days_of_week ?? []).filter((d) => d >= 0 && d <= 6);
  if (custom.length) return [...new Set(custom)].sort((a, b) => a - b);
  if (cfg.frequency === "weekly" && cfg.day_of_week != null && cfg.day_of_week >= 0 && cfg.day_of_week <= 6) {
    return [cfg.day_of_week];
  }
  return [1];
}

export function scheduleFromDays(freq: string, days: number[]) {
  const weekdays = [...new Set(days.filter((d) => d >= 0 && d <= 6))].sort((a, b) => a - b);
  if (freq === "daily" || weekdays.length >= 7) {
    return { frequency: "daily", day_of_week: null as number | null, days_of_week: [] as number[] };
  }
  return {
    frequency: weekdays.length === 1 ? "weekly" : "custom",
    day_of_week: weekdays[0] ?? 1,
    days_of_week: weekdays,
  };
}
