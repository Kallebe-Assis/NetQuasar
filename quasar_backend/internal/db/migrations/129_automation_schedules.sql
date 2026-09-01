-- +goose Up
-- Automações personalizadas (Configurações → Automações → "Nova automação"): ao contrário das
-- 5 automações "singleton" já existentes (automation_alerts_digest, automation_commercial_report,
-- etc. — 1 linha fixa cada, id=1), esta tabela permite QUALQUER número de automações, cada uma
-- escolhendo um relatório (do catálogo /reports/system — inclui BGP, HubSoft, alertas, etc. — ou
-- um relatório de frota/combustível) e a sua própria recorrência. Reaproveita o mesmo motor de
-- "due" (scheduleutil.DailyWeeklyDueOnDays/MonthlyDue) e o mesmo bot Telegram "reports" já usados
-- pelas automações singleton — só generaliza o "o quê" e permite múltiplas instâncias.
CREATE TABLE automation_schedules (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    -- 'system' → report_id é um SYSTEM_REPORT_IDS (bgp-overview, hubsoft-overview, active-alerts,
    -- commercial-base, ...). 'fleet' → report_id é um "kind" de relatório de frota (fuelings,
    -- by-vehicle, by-driver, by-station, by-cost-center).
    domain            TEXT NOT NULL DEFAULT 'system',
    report_id         TEXT NOT NULL,
    -- Janela móvel (dias) usada nos relatórios de frota e nos relatórios de sistema com período
    -- (ex.: 30 = "últimos 30 dias" contados a partir do instante da execução agendada).
    period_days       INT NOT NULL DEFAULT 30,
    channel_telegram  BOOLEAN NOT NULL DEFAULT true,
    enabled           BOOLEAN NOT NULL DEFAULT true,
    frequency         TEXT NOT NULL DEFAULT 'daily', -- daily | weekly | custom | monthly
    day_of_week       INT,
    days_of_week      INT[] NOT NULL DEFAULT '{}',
    day_of_month      INT NOT NULL DEFAULT 1,
    time_hhmm         TEXT NOT NULL DEFAULT '08:00',
    timezone          TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    last_run_key      TEXT,
    last_run_at       TIMESTAMPTZ,
    last_run_ok       BOOLEAN,
    last_run_message  TEXT,
    running           BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_automation_schedules_enabled ON automation_schedules (enabled);

-- +goose Down
DROP TABLE IF EXISTS automation_schedules;
