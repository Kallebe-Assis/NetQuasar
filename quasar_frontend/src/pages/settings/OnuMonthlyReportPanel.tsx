import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";

type OnuAutomationCfg = {
  enabled: boolean;
  mode: string;
  day_of_month: number | null;
  time_hhmm: string;
  timezone: string;
  last_run_at: string | null;
  last_run_period: string | null;
  last_status: string | null;
  last_error: string | null;
  running: boolean;
};

const TZ_DEFAULT = "America/Sao_Paulo";

function statusLabel(st: string | null | undefined): string {
  switch (st) {
    case "collecting":
      return "A recolher dados OLT";
    case "sending_telegram":
      return "A enviar Telegram";
    case "completed":
      return "Concluído";
    case "failed":
      return "Falhou";
    default:
      return st ?? "—";
  }
}

export function OnuMonthlyReportPanel() {
  const qc = useQueryClient();
  const cfg = useQuery({
    queryKey: queryKeys.automationOnu,
    queryFn: () => apiFetch<OnuAutomationCfg>("/api/v1/settings/automation/onu-monthly-report"),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  });
  const runs = useQuery({
    queryKey: queryKeys.automationOnuRuns,
    queryFn: () =>
      apiFetch<{ runs: { id: string; started_at: string; status: string | null; summary: unknown }[] }>(
        "/api/v1/settings/automation/onu-monthly-report/runs",
      ),
    refetchInterval: cfg.data?.running ? 3000 : false,
  });

  const [enabled, setEnabled] = useState(false);
  const [dom, setDom] = useState("1");
  const [timeVal, setTimeVal] = useState("08:00");
  const [tz, setTz] = useState(TZ_DEFAULT);
  const [saveToast, setSaveToast] = useState<{ ok: boolean; text: string } | null>(null);
  const prevStatus = useRef<string | null>(null);

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setDom(cfg.data.day_of_month != null ? String(cfg.data.day_of_month) : "1");
    const th = (cfg.data.time_hhmm ?? "08:00").trim();
    setTimeVal(th.length >= 5 ? th.slice(0, 5) : "08:00");
    setTz(cfg.data.timezone?.trim() || TZ_DEFAULT);
  }, [cfg.data]);

  useEffect(() => {
    const st = cfg.data?.last_status ?? null;
    if (!st || st === prevStatus.current) return;
    if (prevStatus.current === "collecting" && st === "sending_telegram") {
      setSaveToast({ ok: true, text: "Dados OLT recolhidos. A enviar relatório para o Telegram…" });
    }
    if (st === "completed" && (prevStatus.current === "sending_telegram" || prevStatus.current === "collecting")) {
      setSaveToast({ ok: true, text: "Relatório ONU enviado para o Telegram." });
    }
    if (st === "failed") {
      setSaveToast({
        ok: false,
        text: cfg.data?.last_error?.trim() || "Falha ao gerar ou enviar o relatório ONU.",
      });
    }
    prevStatus.current = st;
  }, [cfg.data?.last_status, cfg.data?.last_error]);

  useEffect(() => {
    if (!saveToast) return;
    const t = window.setTimeout(() => setSaveToast(null), 8000);
    return () => window.clearTimeout(t);
  }, [saveToast]);

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
      setSaveToast({ ok: true, text: "Agendamento guardado." });
    },
    onError: (err) => setSaveToast({ ok: false, text: (err as Error).message || "Falha ao guardar." }),
  });

  const run = useMutation({
    mutationFn: () => apiFetch<{ status: string }>("/api/v1/settings/automation/onu-monthly-report/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationOnu });
      void qc.invalidateQueries({ queryKey: queryKeys.automationOnuRuns });
      setSaveToast({ ok: true, text: "A recolher dados OLT para o relatório mensal…" });
    },
    onError: (err) => setSaveToast({ ok: false, text: (err as Error).message || "Não foi possível iniciar." }),
  });

  if (cfg.isLoading) return <p>A carregar…</p>;
  if (cfg.isError) return <div className="msg msg--err">{(cfg.error as Error).message}</div>;

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <>
      <div className="card">
        <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Relatório ONU mensal
          <InfoHint label="Relatório automático ONU">
            <p>
              No dia e hora definidos (fuso abaixo), o sistema recolhe snapshots das OLTs e envia um resumo ao canal Telegram de{" "}
              <strong>Relatórios</strong>. A verificação corre de hora em hora; se o dia agendado passar sem execução, o relatório é gerado no
              dia seguinte (desde que ainda não tenha sido feito no mês).
            </p>
          </InfoHint>
        </h2>
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
          Estado: <strong>{statusLabel(cfg.data?.last_status)}</strong>
          {cfg.data?.last_run_period ? ` · último período: ${cfg.data.last_run_period}` : ""}
          {cfg.data?.last_error ? ` · ${cfg.data.last_error}` : ""}
        </p>

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
            Guardar agendamento
          </button>
          <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
            Executar agora
          </button>
        </div>

        {saveToast && (
          <div className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 10 }}>
            <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
              ×
            </button>
            {saveToast.text}
          </div>
        )}
      </div>

      <div className="table-wrap" style={{ marginTop: 12 }}>
        <h2>Últimas execuções</h2>
        <button type="button" className="btn" style={{ marginBottom: 8 }} onClick={() => runs.refetch()}>
          Atualizar
        </button>
        {runs.isError && <div className="msg msg--err">{(runs.error as Error).message}</div>}
        <table>
          <thead>
            <tr>
              <th>Início</th>
              <th>Estado</th>
            </tr>
          </thead>
          <tbody>
            {(runs.data?.runs ?? []).map((r) => (
              <tr key={r.id}>
                <td className="mono">{r.started_at}</td>
                <td>{statusLabel(r.status)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
