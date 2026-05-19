import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { OnuMonthlyReportPanel } from "./OnuMonthlyReportPanel";

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

function statusLabel(st: string | null | undefined): string {
  switch (st) {
    case "completed":
      return "Concluído";
    case "failed":
      return "Falhou";
    case "collecting":
      return "A recolher…";
    case "sending_telegram":
      return "A enviar…";
    default:
      return st ?? "—";
  }
}

function DigestScheduleCard() {
  const qc = useQueryClient();
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
  const [toast, setToast] = useState<{ ok: boolean; text: string } | null>(null);

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
      setToast({ ok: true, text: "Resumo de alertas guardado." });
    },
    onError: (err) => setToast({ ok: false, text: (err as Error).message }),
  });

  const run = useMutation({
    mutationFn: () => apiFetch("/api/v1/settings/automation/alerts-digest/run", { method: "POST", json: {} }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.automationAlertsDigest }),
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
      <p style={{ fontSize: 12, color: "var(--muted)" }}>
        Estado: <strong>{statusLabel(cfg.data?.last_status)}</strong>
        {cfg.data?.last_error ? ` · ${cfg.data.last_error}` : ""}
      </p>
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
          Guardar
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
          Executar agora
        </button>
      </div>
      {toast && <div className={`msg ${toast.ok ? "msg--ok" : "msg--err"}`} style={{ marginTop: 8 }}>{toast.text}</div>}
    </div>
  );
}

function CommercialScheduleCard() {
  const qc = useQueryClient();
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
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.automationCommercial }),
  });

  const run = useMutation({
    mutationFn: () => apiFetch("/api/v1/settings/automation/commercial-report/run", { method: "POST", json: {} }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.automationCommercial }),
  });

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2>Base comercial (mensal)</h2>
      <p style={{ fontSize: 12, color: "var(--muted)" }}>
        Envia o relatório da base comercial sem recolher OLTs. Estado: <strong>{statusLabel(cfg.data?.last_status)}</strong>
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
          Guardar
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
    },
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
        SMTP activo
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 10 }}>
        <SettingsField label="Servidor">
          <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="smtp.empresa.com" />
        </SettingsField>
        <SettingsField label="Porta">
          <input className="input" value={port} onChange={(e) => setPort(e.target.value)} />
        </SettingsField>
        <SettingsField label="Utilizador">
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
          Guardar SMTP
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

export function ScheduledReportsPanel() {
  return (
    <>
      <p style={{ color: "var(--muted)", fontSize: 13 }}>
        Agende envios automáticos por Telegram e/ou e-mail. O relatório ONU mensal continua a recolher OLTs antes de enviar; a base comercial
        abaixo envia apenas os totais já registados.
      </p>
      <OnuMonthlyReportPanel />
      <DigestScheduleCard />
      <CommercialScheduleCard />
      <SmtpPanel />
    </>
  );
}
