import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type OnuAutomationCfg = {
  enabled: boolean;
  mode: string;
  day_of_month: number | null;
  time_hhmm: string;
  timezone: string;
  running: boolean;
};

const TZ_DEFAULT = "America/Sao_Paulo";

export function OnuMonthlyReportPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.automationOnu,
    queryFn: () => apiFetch<OnuAutomationCfg>("/api/v1/settings/automation/onu-monthly-report"),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  });

  const [enabled, setEnabled] = useState(false);
  const [dom, setDom] = useState("1");
  const [timeVal, setTimeVal] = useState("08:00");
  const [tz, setTz] = useState(TZ_DEFAULT);

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setDom(cfg.data.day_of_month != null ? String(cfg.data.day_of_month) : "1");
    const th = (cfg.data.time_hhmm ?? "08:00").trim();
    setTimeVal(th.length >= 5 ? th.slice(0, 5) : "08:00");
    setTz(cfg.data.timezone?.trim() || TZ_DEFAULT);
  }, [cfg.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/automation/onu-monthly-report", {
        method: "PATCH",
        json: {
          enabled,
          day_of_month: Number(dom) || 1,
          time_hhmm: timeVal,
          timezone: tz.trim() || TZ_DEFAULT,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationOnu });
      toastOk(pushToast, "Agendamento ONU salvo.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar agendamento ONU."),
  });

  const run = useMutation({
    mutationFn: () => apiFetch<{ status: string }>("/api/v1/settings/automation/onu-monthly-report/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationOnu });
      void qc.invalidateQueries({ queryKey: queryKeys.automationHistory });
      toastOk(pushToast, "Execução iniciada — consulte o Histórico.");
    },
    onError: (err) => toastErr(pushToast, err, "Não foi possível iniciar execução ONU."),
  });

  if (cfg.isLoading) return <p>A carregar…</p>;
  if (cfg.isError) return <div className="msg msg--err">{(cfg.error as Error).message}</div>;

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <div className="card">
      <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
        Relatório ONU mensal
        <InfoHint label="Relatório automático ONU">
          <p>
            No dia e hora definidos (fuso abaixo), o sistema recolhe snapshots das OLTs e envia um resumo ao canal Telegram de{" "}
            <strong>Relatórios</strong>. A verificação corre a cada 30 segundos; se o dia agendado passar sem execução, o relatório é gerado no
            dia seguinte (desde que ainda não tenha sido feito no mês). O resultado de cada execução aparece em <strong>Histórico</strong>.
          </p>
        </InfoHint>
      </h2>
      {cfg.data?.running ? (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>Execução em curso…</p>
      ) : null}

      <label className="row" style={{ gap: 8, marginTop: 12 }}>
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={busy} />
        Agendamento automático ativo
      </label>

      <div className="settings-fields-grid" style={{ marginTop: 12 }}>
        <SettingsField label="Dia do mês">
          <select className="input" value={dom} onChange={(e) => setDom(e.target.value)} disabled={busy || !enabled} aria-label="Dia do mês">
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
        <SettingsField
          label="Fuso horário"
          hintLabel="Fuso horário IANA"
          hint={<p>Identificador IANA, por exemplo <code>America/Sao_Paulo</code>.</p>}
        >
          <input
            className="input mono"
            value={tz}
            onChange={(e) => setTz(e.target.value)}
            placeholder={TZ_DEFAULT}
            disabled={busy || !enabled}
          />
        </SettingsField>
      </div>

      <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap" }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending || busy} onClick={() => patch.mutate()}>
          Salvar agendamento
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
          Executar agora
        </button>
      </div>
    </div>
  );
}
