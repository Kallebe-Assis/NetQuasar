import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Mail, Plus, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import {
  automationJobDef,
  draftFromJob,
  formatRecurrence,
} from "../../lib/automationJobs";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import { AddAutomationModal } from "./AddAutomationModal";
import { AutomationHistoryModal } from "./AutomationHistoryModal";
import { AutomationsHistoryTable, AutomationsLogDetail, type AutomationHistoryRow } from "./AutomationsHistoryTable";
import { OnuMonthlyReportPanel } from "./OnuMonthlyReportPanel";
import { DatabaseBackupAutomationCard } from "./DatabaseBackupAutomationCard";

const TZ_DEFAULT = "America/Sao_Paulo";

type ScheduleCfg = {
  enabled: boolean;
  frequency?: string;
  day_of_week?: number | null;
  day_of_month?: number | null;
  time_hhmm: string;
  timezone: string;
  channel_telegram: boolean;
  channel_email: boolean;
  email_to?: string | null;
  last_status?: string | null;
  last_error?: string | null;
  running: boolean;
};

type SmtpCfg = {
  enabled: boolean;
  host?: string | null;
  port: number;
  username?: string | null;
  password_configured: boolean;
  from_address?: string | null;
  use_tls: boolean;
};

function DigestScheduleCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.automationAlertsDigest,
    queryFn: () => apiFetch<ScheduleCfg>("/api/v1/settings/automation/alerts-digest"),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  });
  const [enabled, setEnabled] = useState(false);
  const [freq, setFreq] = useState("daily");
  const [dow, setDow] = useState("1");
  const [timeVal, setTimeVal] = useState("07:30");
  const [tz, setTz] = useState(TZ_DEFAULT);
  const [tg, setTg] = useState(true);
  const [em, setEm] = useState(false);
  const [emailTo, setEmailTo] = useState("");

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setFreq(cfg.data.frequency ?? "daily");
    setDow(cfg.data.day_of_week != null ? String(cfg.data.day_of_week) : "1");
    setTimeVal((cfg.data.time_hhmm ?? "07:30").slice(0, 5));
    setTz(cfg.data.timezone?.trim() || TZ_DEFAULT);
    setTg(cfg.data.channel_telegram);
    setEm(cfg.data.channel_email);
    setEmailTo(cfg.data.email_to ?? "");
  }, [cfg.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/automation/alerts-digest", {
        method: "PATCH",
        json: {
          enabled,
          frequency: freq,
          day_of_week: Number(dow),
          time_hhmm: timeVal,
          timezone: tz,
          channel_telegram: tg,
          channel_email: em,
          email_to: emailTo.trim() || null,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationAlertsDigest });
      toastOk(pushToast, "Resumo de alertas salvo.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar resumo de alertas."),
  });

  const run = useMutation({
    mutationFn: () => apiFetch("/api/v1/settings/automation/alerts-digest/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationAlertsDigest });
      void qc.invalidateQueries({ queryKey: queryKeys.automationHistory });
    },
  });

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2 style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        Resumo de alertas
        <InfoHint label="Resumo de alertas agendado">
          <p>Envia contagem de alertas abertos, resolvidos em 24 h e incidentes correlacionados via Telegram (relatórios) e/ou e-mail SMTP.</p>
        </InfoHint>
      </h2>
      {cfg.data?.running ? (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>Execução em curso…</p>
      ) : null}
      <label className="row" style={{ gap: 8, marginTop: 10 }}>
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={busy} />
        Agendamento ativo
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 10 }}>
        <SettingsField label="Frequência">
          <select className="input" value={freq} onChange={(e) => setFreq(e.target.value)} disabled={busy || !enabled}>
            <option value="daily">Diário</option>
            <option value="weekly">Semanal</option>
          </select>
        </SettingsField>
        {freq === "weekly" && (
          <SettingsField label="Dia da semana">
            <select className="input" value={dow} onChange={(e) => setDow(e.target.value)} disabled={busy || !enabled}>
              {["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"].map((l, i) => (
                <option key={l} value={String(i)}>
                  {l}
                </option>
              ))}
            </select>
          </SettingsField>
        )}
        <SettingsField label="Hora">
          <input className="input" type="time" value={timeVal} onChange={(e) => setTimeVal(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
        <SettingsField label="Fuso horário">
          <input className="input mono" value={tz} onChange={(e) => setTz(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
      </div>
      <div className="row" style={{ gap: 12, marginTop: 10, flexWrap: "wrap" }}>
        <label className="row" style={{ gap: 6 }}>
          <input type="checkbox" checked={tg} onChange={(e) => setTg(e.target.checked)} disabled={busy} />
          Telegram (relatórios)
        </label>
        <label className="row" style={{ gap: 6 }}>
          <input type="checkbox" checked={em} onChange={(e) => setEm(e.target.checked)} disabled={busy} />
          E-mail
        </label>
      </div>
      {em && (
        <div style={{ marginTop: 8 }}>
        <SettingsField label="Destinatários e-mail">
          <input
            className="input"
            value={emailTo}
            onChange={(e) => setEmailTo(e.target.value)}
            placeholder="noc@empresa.com, gestao@empresa.com"
            disabled={busy}
          />
        </SettingsField>
        </div>
      )}
      <div className="row" style={{ marginTop: 12, gap: 8 }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending || busy} onClick={() => patch.mutate()}>
          Salvar
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
          Executar agora
        </button>
      </div>
    </div>
  );
}

function BngStatsScheduleCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.automationBngStats,
    queryFn: () => apiFetch<ScheduleCfg>("/api/v1/settings/automation/bng-stats-report"),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  });
  const [enabled, setEnabled] = useState(false);
  const [freq, setFreq] = useState("daily");
  const [dow, setDow] = useState("1");
  const [timeVal, setTimeVal] = useState("08:00");
  const [tz, setTz] = useState(TZ_DEFAULT);
  const [tg, setTg] = useState(true);
  const [em, setEm] = useState(false);
  const [emailTo, setEmailTo] = useState("");

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setFreq(cfg.data.frequency ?? "daily");
    setDow(cfg.data.day_of_week != null ? String(cfg.data.day_of_week) : "1");
    setTimeVal((cfg.data.time_hhmm ?? "08:00").slice(0, 5));
    setTz(cfg.data.timezone?.trim() || TZ_DEFAULT);
    setTg(cfg.data.channel_telegram);
    setEm(cfg.data.channel_email);
    setEmailTo(cfg.data.email_to ?? "");
  }, [cfg.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/automation/bng-stats-report", {
        method: "PATCH",
        json: {
          enabled,
          frequency: freq,
          day_of_week: Number(dow),
          time_hhmm: timeVal,
          timezone: tz,
          channel_telegram: tg,
          channel_email: em,
          email_to: emailTo.trim() || null,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationBngStats });
      toastOk(pushToast, "Automação BNG salva.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar automação BNG."),
  });

  const run = useMutation({
    mutationFn: () => apiFetch("/api/v1/settings/automation/bng-stats-report/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationBngStats });
      void qc.invalidateQueries({ queryKey: queryKeys.automationHistory });
    },
  });

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2 style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        Totais BNG
        <InfoHint label="Relatório agendado de totais BNG">
          <p>
            Envia o relatório <strong>BNG — totais de logins</strong> (PPPoE, IPv4, IPv6, dual-stack) via Telegram e/ou e-mail.
            Usa amostras já recolhidas pelo monitoramento SNMP — não executa consulta completa de sessões.
          </p>
        </InfoHint>
      </h2>
      {cfg.data?.running ? (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>Execução em curso…</p>
      ) : null}
      <label className="row" style={{ gap: 8, marginTop: 10 }}>
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={busy} />
        Agendamento ativo
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 10 }}>
        <SettingsField label="Frequência">
          <select className="input" value={freq} onChange={(e) => setFreq(e.target.value)} disabled={busy || !enabled}>
            <option value="daily">Diário</option>
            <option value="weekly">Semanal</option>
          </select>
        </SettingsField>
        {freq === "weekly" && (
          <SettingsField label="Dia da semana">
            <select className="input" value={dow} onChange={(e) => setDow(e.target.value)} disabled={busy || !enabled}>
              {["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"].map((l, i) => (
                <option key={l} value={String(i)}>
                  {l}
                </option>
              ))}
            </select>
          </SettingsField>
        )}
        <SettingsField label="Hora">
          <input className="input" type="time" value={timeVal} onChange={(e) => setTimeVal(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
        <SettingsField label="Fuso horário">
          <input className="input mono" value={tz} onChange={(e) => setTz(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
      </div>
      <div className="row" style={{ gap: 12, marginTop: 10, flexWrap: "wrap" }}>
        <label className="row" style={{ gap: 6 }}>
          <input type="checkbox" checked={tg} onChange={(e) => setTg(e.target.checked)} disabled={busy} />
          Telegram (relatórios)
        </label>
        <label className="row" style={{ gap: 6 }}>
          <input type="checkbox" checked={em} onChange={(e) => setEm(e.target.checked)} disabled={busy} />
          E-mail
        </label>
      </div>
      {em && (
        <div style={{ marginTop: 8 }}>
          <SettingsField label="Destinatários e-mail">
            <input
              className="input"
              value={emailTo}
              onChange={(e) => setEmailTo(e.target.value)}
              placeholder="noc@empresa.com"
              disabled={busy}
            />
          </SettingsField>
        </div>
      )}
      <div className="row" style={{ marginTop: 12, gap: 8 }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending || busy} onClick={() => patch.mutate()}>
          Salvar
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
          Executar agora
        </button>
      </div>
    </div>
  );
}

function CommercialScheduleCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.automationCommercial,
    queryFn: () => apiFetch<ScheduleCfg>("/api/v1/settings/automation/commercial-report"),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  });
  const [enabled, setEnabled] = useState(false);
  const [dom, setDom] = useState("1");
  const [timeVal, setTimeVal] = useState("09:00");
  const [tz, setTz] = useState(TZ_DEFAULT);
  const [tg, setTg] = useState(true);
  const [em, setEm] = useState(false);
  const [emailTo, setEmailTo] = useState("");

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setDom(cfg.data.day_of_month != null ? String(cfg.data.day_of_month) : "1");
    setTimeVal((cfg.data.time_hhmm ?? "09:00").slice(0, 5));
    setTz(cfg.data.timezone?.trim() || TZ_DEFAULT);
    setTg(cfg.data.channel_telegram);
    setEm(cfg.data.channel_email);
    setEmailTo(cfg.data.email_to ?? "");
  }, [cfg.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/automation/commercial-report", {
        method: "PATCH",
        json: {
          enabled,
          day_of_month: Number(dom) || 1,
          time_hhmm: timeVal,
          timezone: tz,
          channel_telegram: tg,
          channel_email: em,
          email_to: emailTo.trim() || null,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationCommercial });
      toastOk(pushToast, "Relatório comercial agendado salvo.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar agendamento comercial."),
  });

  const run = useMutation({
    mutationFn: () => apiFetch("/api/v1/settings/automation/commercial-report/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationCommercial });
      void qc.invalidateQueries({ queryKey: queryKeys.automationHistory });
    },
  });

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2>Base comercial (mensal)</h2>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Envia o relatório da base comercial sem recolher OLTs. Resultados em <strong>Histórico</strong>.
        {cfg.data?.running ? " · execução em curso…" : ""}
      </p>
      <label className="row" style={{ gap: 8, marginTop: 10 }}>
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={busy} />
        Agendamento ativo
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 10 }}>
        <SettingsField label="Dia do mês">
          <select className="input" value={dom} onChange={(e) => setDom(e.target.value)} disabled={busy || !enabled}>
            {Array.from({ length: 31 }, (_, i) => i + 1).map((d) => (
              <option key={d} value={String(d)}>
                {d}
              </option>
            ))}
          </select>
        </SettingsField>
        <SettingsField label="Hora">
          <input className="input" type="time" value={timeVal} onChange={(e) => setTimeVal(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
        <SettingsField label="Fuso horário">
          <input className="input mono" value={tz} onChange={(e) => setTz(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
      </div>
      <div className="row" style={{ gap: 12, marginTop: 10, flexWrap: "wrap" }}>
        <label className="row" style={{ gap: 6 }}>
          <input type="checkbox" checked={tg} onChange={(e) => setTg(e.target.checked)} disabled={busy} />
          Telegram
        </label>
        <label className="row" style={{ gap: 6 }}>
          <input type="checkbox" checked={em} onChange={(e) => setEm(e.target.checked)} disabled={busy} />
          E-mail
        </label>
      </div>
      {em && (
        <div style={{ marginTop: 8 }}>
          <SettingsField label="Destinatários">
            <input className="input" value={emailTo} onChange={(e) => setEmailTo(e.target.value)} disabled={busy} />
          </SettingsField>
        </div>
      )}
      <div className="row" style={{ marginTop: 12, gap: 8 }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending || busy} onClick={() => patch.mutate()}>
          Salvar
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
          Executar agora
        </button>
      </div>
    </div>
  );
}

function SmtpPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.smtpSettings,
    queryFn: () => apiFetch<SmtpCfg>("/api/v1/settings/notifications/smtp"),
  });
  const [enabled, setEnabled] = useState(false);
  const [host, setHost] = useState("");
  const [port, setPort] = useState("587");
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [from, setFrom] = useState("");
  const [tls, setTls] = useState(true);
  const [testTo, setTestTo] = useState("");

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setHost(cfg.data.host ?? "");
    setPort(String(cfg.data.port || 587));
    setUser(cfg.data.username ?? "");
    setFrom(cfg.data.from_address ?? "");
    setTls(cfg.data.use_tls);
  }, [cfg.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/notifications/smtp", {
        method: "PATCH",
        json: {
          enabled,
          host: host.trim() || null,
          port: Number(port) || 587,
          username: user.trim() || null,
          password: pass || undefined,
          from_address: from.trim() || null,
          use_tls: tls,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.smtpSettings });
      setPass("");
      toastOk(pushToast, "SMTP salvo com sucesso.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar SMTP."),
  });

  const test = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/notifications/smtp/test", {
        method: "POST",
        json: { to: testTo.trim() },
      }),
  });

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2>E-mail (SMTP)</h2>
      <p style={{ fontSize: 12, color: "var(--muted)" }}>
        Necessário para relatórios por e-mail. Palavra-passe: {cfg.data?.password_configured ? "configurada" : "não definida"}.
      </p>
      <label className="row" style={{ gap: 8, marginTop: 10 }}>
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        SMTP ativo
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 10 }}>
        <SettingsField label="Servidor">
          <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="smtp.empresa.com" />
        </SettingsField>
        <SettingsField label="Porta">
          <input className="input" value={port} onChange={(e) => setPort(e.target.value)} />
        </SettingsField>
        <SettingsField label="Usuário">
          <input className="input" value={user} onChange={(e) => setUser(e.target.value)} />
        </SettingsField>
        <SettingsField label="Palavra-passe">
          <input className="input" type="password" value={pass} onChange={(e) => setPass(e.target.value)} placeholder="(deixar vazio para manter)" />
        </SettingsField>
        <SettingsField label="Remetente">
          <input className="input" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="noc@empresa.com" />
        </SettingsField>
      </div>
      <label className="row" style={{ gap: 8, marginTop: 10 }}>
        <input type="checkbox" checked={tls} onChange={(e) => setTls(e.target.checked)} />
        TLS
      </label>
      <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap" }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          Salvar SMTP
        </button>
        <input className="input" style={{ maxWidth: 280 }} value={testTo} onChange={(e) => setTestTo(e.target.value)} placeholder="e-mail para teste" />
        <button type="button" className="btn" disabled={test.isPending} onClick={() => test.mutate()}>
          Testar envio
        </button>
      </div>
      {test.isError && <div className="msg msg--err" style={{ marginTop: 8 }}>{(test.error as Error).message}</div>}
      {test.isSuccess && <div className="msg msg--ok" style={{ marginTop: 8 }}>E-mail de teste enviado.</div>}
    </div>
  );
}

type AutomationJobOverview = {
  job_type: string;
  label: string;
  category: string;
  description: string;
  enabled: boolean;
  running: boolean;
  last_status?: string | null;
  last_error?: string | null;
  last_run_at?: string | null;
  runs_24h?: number;
  fail_24h?: number;
  frequency?: string | null;
  day_of_week?: number | null;
  day_of_month?: number | null;
  time_hhmm?: string | null;
  timezone?: string | null;
  days_of_week?: number[] | null;
};

type AutomationKpis = {
  total: number;
  enabled: number;
  disabled: number;
  running: number;
  executed_today: number;
  success_rate_30d?: number | null;
  runs_30d?: number;
  failures_30d?: number;
  last_failure_at?: string | null;
};

type DetailTab = "geral" | "historico" | "logs";

function formatWhen(iso?: string | null) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
  } catch {
    return iso;
  }
}

function jobScheduleText(j: AutomationJobOverview): string {
  const def = automationJobDef(j.job_type);
  if (def?.recurrences.length === 1 && def.recurrences[0] === "monthly") {
    return formatRecurrence(draftFromJob({ ...j, frequency: "monthly", day_of_month: j.day_of_month ?? 1 }));
  }
  return formatRecurrence(draftFromJob(j));
}

export function ScheduledReportsPanel() {
  const [historyOpen, setHistoryOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [smtpOpen, setSmtpOpen] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [detailTab, setDetailTab] = useState<DetailTab>("geral");
  const [logRow, setLogRow] = useState<AutomationHistoryRow | null>(null);

  const overview = useQuery({
    queryKey: queryKeys.automationOverview,
    queryFn: () =>
      apiFetch<{ jobs: AutomationJobOverview[]; kpis: AutomationKpis }>("/api/v1/settings/automation"),
    refetchInterval: 8000,
  });

  const jobs = overview.data?.jobs ?? [];
  const kpis = overview.data?.kpis;
  const registered = useMemo(() => jobs.filter((j) => j.enabled), [jobs]);
  const selectedJob = registered.find((j) => j.job_type === selected) ?? null;

  useEffect(() => {
    if (selected && !registered.some((j) => j.job_type === selected)) {
      setSelected(registered[0]?.job_type ?? null);
    }
  }, [registered, selected]);

  useEffect(() => {
    setLogRow(null);
    setDetailTab("geral");
  }, [selected]);

  return (
    <div className="automations-workspace">
      <div className="automations-workspace__head">
        <div>
          <h2 style={{ margin: 0 }}>Automações</h2>
          <p style={{ color: "var(--muted)", fontSize: 13, margin: "6px 0 0" }}>
            Cadastre o que deve correr sozinho: tipo, dias e hora. O resto (canais, teste, histórico) fica em cada cartão.
          </p>
        </div>
        <div className="automations-workspace__actions">
          <button
            type="button"
            className="btn btn--icon btn--icon-menu"
            title="Actualizar"
            aria-label="Actualizar"
            disabled={overview.isFetching}
            onClick={() => void overview.refetch()}
          >
            <RefreshCw size={18} aria-hidden />
          </button>
          <button
            type="button"
            className="btn btn--icon btn--icon-menu"
            title="Histórico global"
            aria-label="Histórico global"
            onClick={() => setHistoryOpen(true)}
          >
            <History size={18} aria-hidden />
          </button>
          <button
            type="button"
            className={`btn btn--icon btn--icon-menu${smtpOpen ? " btn--primary" : ""}`}
            title="E-mail (SMTP)"
            aria-label="E-mail SMTP"
            onClick={() => {
              setSmtpOpen((v) => !v);
              setSelected(null);
            }}
          >
            <Mail size={18} aria-hidden />
          </button>
          <button
            type="button"
            className="btn btn--icon btn--icon-menu btn--primary"
            title="Adicionar nova automação"
            aria-label="Adicionar nova automação"
            onClick={() => setAddOpen(true)}
          >
            <Plus size={18} aria-hidden />
          </button>
        </div>
      </div>

      <div className="automations-kpis">
        <div className="automations-kpi card">
          <div className="automations-kpi__label">Cadastradas</div>
          <div className="automations-kpi__value">{registered.length}</div>
          <div className="automations-kpi__meta">{kpis ? `${kpis.enabled} activas` : "…"}</div>
        </div>
        <div className="automations-kpi card">
          <div className="automations-kpi__label">Executadas hoje</div>
          <div className="automations-kpi__value">{kpis?.executed_today ?? "—"}</div>
          <div className="automations-kpi__meta">{kpis?.running ? `${kpis.running} em curso` : "Nenhuma em curso"}</div>
        </div>
        <div className="automations-kpi card">
          <div className="automations-kpi__label">Sucesso (30d)</div>
          <div className="automations-kpi__value">
            {kpis?.success_rate_30d != null ? `${kpis.success_rate_30d.toFixed(1)}%` : "—"}
          </div>
          <div className="automations-kpi__meta">{kpis?.runs_30d != null ? `${kpis.runs_30d} execuções` : "…"}</div>
        </div>
        <div className="automations-kpi card">
          <div className="automations-kpi__label">Falhas (30d)</div>
          <div className="automations-kpi__value">{kpis?.failures_30d ?? "—"}</div>
          <div className="automations-kpi__meta">Última: {formatWhen(kpis?.last_failure_at)}</div>
        </div>
      </div>

      {smtpOpen ? (
        <div className="card automations-smtp-card">
          <SmtpPanel />
        </div>
      ) : null}

      <div className="automations-register card">
        <div className="automations-register__title">Cadastros</div>
        {overview.isLoading ? (
          <p style={{ fontSize: 13, color: "var(--muted)" }}>A carregar…</p>
        ) : registered.length === 0 ? (
          <div className="automations-empty">
            <p>Nenhuma automação cadastrada.</p>
            <button type="button" className="btn btn--primary" onClick={() => setAddOpen(true)}>
              Adicionar a primeira
            </button>
          </div>
        ) : (
          <ul className="automations-cards">
            {registered.map((j) => (
              <li key={j.job_type}>
                <button
                  type="button"
                  className={`automations-card${selected === j.job_type ? " is-active" : ""}`}
                  onClick={() => {
                    setSmtpOpen(false);
                    setSelected(j.job_type);
                  }}
                >
                  <span className="automations-card__top">
                    <span className="automations-card__name">{j.label}</span>
                    <span className={`automations-pill ${j.running ? "automations-pill--run" : "automations-pill--on"}`}>
                      {j.running ? "A correr" : "Activa"}
                    </span>
                  </span>
                  <span className="automations-card__when">{jobScheduleText(j)}</span>
                  <span className="automations-card__meta">
                    {j.category} · última: {formatWhen(j.last_run_at)}
                    {j.last_status ? ` · ${j.last_status}` : ""}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {selectedJob ? (
        <div className="automations-detail card">
          <div className="automations-detail__head">
            <div>
              <h3 style={{ margin: 0 }}>{selectedJob.label}</h3>
              <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--muted)" }}>{selectedJob.description}</p>
            </div>
            <span className="automations-pill automations-pill--on">Activa</span>
          </div>
          <div className="automations-detail__tabs" role="tablist">
            {(
              [
                ["geral", "Geral"],
                ["historico", "Histórico"],
                ["logs", "Logs"],
              ] as const
            ).map(([id, lab]) => (
              <button
                key={id}
                type="button"
                role="tab"
                aria-selected={detailTab === id}
                className={detailTab === id ? "is-active" : ""}
                onClick={() => setDetailTab(id)}
              >
                {lab}
              </button>
            ))}
          </div>
          <div className="automations-detail__body">
            {detailTab === "geral" ? (
              <div className="automations-detail__form">
                {selected === "database_backup" && <DatabaseBackupAutomationCard />}
                {selected === "onu_monthly_report" && <OnuMonthlyReportPanel />}
                {selected === "bng_stats_report" && <BngStatsScheduleCard />}
                {selected === "alerts_digest" && <DigestScheduleCard />}
                {selected === "commercial_report" && <CommercialScheduleCard />}
              </div>
            ) : null}
            {detailTab === "historico" ? (
              <AutomationsHistoryTable
                jobType={selected ?? ""}
                showJobColumn={false}
                selectedId={logRow?.id}
                onSelect={(row) => {
                  setLogRow(row);
                  setDetailTab("logs");
                }}
              />
            ) : null}
            {detailTab === "logs" ? (
              <div>
                <AutomationsHistoryTable
                  jobType={selected ?? ""}
                  showJobColumn={false}
                  compact
                  limit={50}
                  selectedId={logRow?.id}
                  onSelect={setLogRow}
                />
                <div style={{ marginTop: 12 }}>
                  <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>Detalhe da execução</h4>
                  <AutomationsLogDetail row={logRow} />
                </div>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      <AddAutomationModal
        open={addOpen}
        takenIds={registered.map((j) => j.job_type)}
        onClose={() => setAddOpen(false)}
        onCreated={(id) => {
          setAddOpen(false);
          setSmtpOpen(false);
          setSelected(id);
        }}
      />
      <AutomationHistoryModal open={historyOpen} onClose={() => setHistoryOpen(false)} />
    </div>
  );
}
