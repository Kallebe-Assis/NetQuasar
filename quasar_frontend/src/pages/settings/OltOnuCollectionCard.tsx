import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type CollectSummary = {
  olts_total?: number;
  olts_eligible?: number;
  olts_ok?: number;
  olts_failed?: number;
  olts_skipped?: number;
  duration_s?: number;
  failures?: { description?: string; host?: string; reason?: string }[];
};

type OltOnuCollectionCfg = {
  enabled: boolean;
  light_enabled: boolean;
  light_interval_minutes: number;
  full_enabled: boolean;
  full_interval_minutes: number;
  last_light_at?: string | null;
  last_full_at?: string | null;
  last_status?: string | null;
  last_error?: string | null;
  running: boolean;
  running_light: boolean;
  running_full: boolean;
  last_light_summary?: CollectSummary | null;
  last_full_summary?: CollectSummary | null;
};

function formatWhen(iso?: string | null) {
  if (!iso) return "nunca";
  try {
    return new Date(iso).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
  } catch {
    return iso;
  }
}

function summaryLine(s?: CollectSummary | null): string {
  if (!s) return "";
  const parts: string[] = [];
  if (s.olts_ok != null) parts.push(`${s.olts_ok} OLT(s) OK`);
  if (s.olts_failed) parts.push(`${s.olts_failed} com falha`);
  if (s.olts_skipped) parts.push(`${s.olts_skipped} ignorada(s)`);
  if (s.duration_s != null) parts.push(`${s.duration_s}s`);
  return parts.join(" · ");
}

export function OltOnuCollectionCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.automationOltOnuCollection,
    queryFn: () => apiFetch<OltOnuCollectionCfg>("/api/v1/settings/automation/olt-onu-collection"),
    refetchInterval: (q) => (q.state.data?.running ? 3000 : false),
  });

  const [enabled, setEnabled] = useState(true);
  const [lightEn, setLightEn] = useState(true);
  const [lightMin, setLightMin] = useState("5");
  const [fullEn, setFullEn] = useState(true);
  const [fullHours, setFullHours] = useState("6");

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setLightEn(cfg.data.light_enabled);
    setLightMin(String(cfg.data.light_interval_minutes));
    setFullEn(cfg.data.full_enabled);
    setFullHours(String(Math.round((cfg.data.full_interval_minutes / 60) * 100) / 100));
  }, [cfg.data]);

  const refreshAll = () => {
    void qc.invalidateQueries({ queryKey: queryKeys.automationOltOnuCollection });
    void qc.invalidateQueries({ queryKey: queryKeys.automationOverview });
    void qc.invalidateQueries({ queryKey: queryKeys.automationHistory });
  };

  const patch = useMutation({
    mutationFn: () => {
      const light = Math.round(Number(lightMin));
      const fullMinutes = Math.round(Number(fullHours.replace(",", ".")) * 60);
      if (!Number.isFinite(light) || light < 1 || light > 1440) {
        throw new Error("Intervalo da coleta leve: 1 a 1440 minutos.");
      }
      if (!Number.isFinite(fullMinutes) || fullMinutes < 15 || fullMinutes > 10080) {
        throw new Error("Intervalo da coleta completa: de 15 minutos (0,25 h) a 168 h.");
      }
      return apiFetch("/api/v1/settings/automation/olt-onu-collection", {
        method: "PATCH",
        json: {
          enabled,
          light_enabled: lightEn,
          light_interval_minutes: light,
          full_enabled: fullEn,
          full_interval_minutes: fullMinutes,
        },
      });
    },
    onSuccess: () => {
      refreshAll();
      toastOk(pushToast, "Coleta de ONUs salva.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar coleta de ONUs."),
  });

  const run = useMutation({
    mutationFn: (kind: "light" | "full") =>
      apiFetch("/api/v1/settings/automation/olt-onu-collection/run", { method: "POST", json: { kind } }),
    onSuccess: (_d, kind) => {
      refreshAll();
      toastOk(pushToast, kind === "full" ? "Coleta completa iniciada." : "Coleta leve iniciada.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao iniciar a coleta."),
  });

  const busy = !!cfg.data?.running || run.isPending;
  const failures = [...(cfg.data?.last_full_summary?.failures ?? []), ...(cfg.data?.last_light_summary?.failures ?? [])].slice(0, 6);

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2 style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        Coleta de ONUs (OLT)
        <InfoHint label="Coleta de ONUs em segundo plano">
          <p>
            Duas coletas independentes sobre todas as OLTs, em segundo plano:{" "}
            <strong>leve</strong> (status das ONUs/PONs + RX, frequente) e <strong>completa</strong> (serial, temperatura,
            TX, modelo — espaçada). A completa também refaz a leitura de serial PON a PON nas ONUs que ficaram sem serial.
          </p>
          <p>
            Quando as duas vencem ao mesmo tempo, roda só a completa (ela já atualiza o que a leve atualizaria). Cada OLT é
            consultada uma vez por vez, sem sobrepor outras sondas SNMP ao mesmo equipamento.
          </p>
        </InfoHint>
      </h2>
      {cfg.data?.running ? (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
          Execução em curso ({cfg.data.running_full ? "completa" : "leve"})…
        </p>
      ) : null}

      <label className="row" style={{ gap: 8, marginTop: 10 }}>
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={busy} />
        Automação ativa
      </label>

      <h3 style={{ fontSize: 13, margin: "14px 0 0" }}>Coleta leve — status + RX</h3>
      <label className="row" style={{ gap: 8, marginTop: 6 }}>
        <input type="checkbox" checked={lightEn} onChange={(e) => setLightEn(e.target.checked)} disabled={busy || !enabled} />
        Ativa
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 8 }}>
        <SettingsField label="A cada (minutos)">
          <input
            className="input"
            type="number"
            min={1}
            max={1440}
            value={lightMin}
            onChange={(e) => setLightMin(e.target.value)}
            disabled={busy || !enabled || !lightEn}
          />
        </SettingsField>
      </div>
      <p style={{ fontSize: 12, color: "var(--muted)", margin: "6px 0 0" }}>
        Última: {formatWhen(cfg.data?.last_light_at)}
        {summaryLine(cfg.data?.last_light_summary) ? ` — ${summaryLine(cfg.data?.last_light_summary)}` : ""}
      </p>

      <h3 style={{ fontSize: 13, margin: "14px 0 0" }}>Coleta completa — serial, temperatura, TX…</h3>
      <label className="row" style={{ gap: 8, marginTop: 6 }}>
        <input type="checkbox" checked={fullEn} onChange={(e) => setFullEn(e.target.checked)} disabled={busy || !enabled} />
        Ativa
      </label>
      <div className="settings-fields-grid" style={{ marginTop: 8 }}>
        <SettingsField label="A cada (horas)">
          <input
            className="input"
            type="number"
            min={0.25}
            max={168}
            step={0.25}
            value={fullHours}
            onChange={(e) => setFullHours(e.target.value)}
            disabled={busy || !enabled || !fullEn}
          />
        </SettingsField>
      </div>
      <p style={{ fontSize: 12, color: "var(--muted)", margin: "6px 0 0" }}>
        Última: {formatWhen(cfg.data?.last_full_at)}
        {summaryLine(cfg.data?.last_full_summary) ? ` — ${summaryLine(cfg.data?.last_full_summary)}` : ""}
      </p>

      {cfg.data?.last_error ? (
        <p className="msg msg--err" style={{ marginTop: 10, fontSize: 12 }}>
          {cfg.data.last_error}
        </p>
      ) : null}
      {failures.length > 0 ? (
        <ul style={{ fontSize: 12, color: "var(--muted)", margin: "8px 0 0", paddingLeft: 18 }}>
          {failures.map((f, i) => (
            <li key={`${f.host ?? ""}-${i}`}>
              {f.description || f.host}: {f.reason || "falha na coleta"}
            </li>
          ))}
        </ul>
      ) : null}

      <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap" }}>
        <button type="button" className="btn btn--primary" disabled={patch.isPending || busy} onClick={() => patch.mutate()}>
          Salvar
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate("light")}>
          Executar leve agora
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate("full")}>
          Executar completa agora
        </button>
      </div>
    </div>
  );
}
