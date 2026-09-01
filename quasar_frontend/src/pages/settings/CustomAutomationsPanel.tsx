import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Send, Trash2 } from "lucide-react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { ConfirmModal } from "../../components/ConfirmModal";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { AutomationRecurrenceFields } from "./AutomationRecurrenceFields";
import { emptyRecurrence, TZ_DEFAULT, type RecurrenceDraft, type RecurrenceKind } from "../../lib/automationJobs";
import { fetchSystemReportCatalog, type SystemReportCatalogItem } from "../../lib/systemReports";

const FLEET_REPORT_OPTIONS: { id: string; title: string; description: string }[] = [
  { id: "fuelings", title: "Frota — Abastecimentos", description: "Lista de abastecimentos recentes." },
  { id: "by-vehicle", title: "Frota — Por veículo", description: "Consumo agregado por veículo." },
  { id: "by-driver", title: "Frota — Por motorista", description: "Consumo agregado por motorista." },
  { id: "by-station", title: "Frota — Por posto", description: "Consumo agregado por posto de combustível." },
  { id: "by-cost-center", title: "Frota — Por centro de custo", description: "Consumo agregado por centro de custo." },
];

type CustomAutomation = {
  id: string;
  name: string;
  domain: "system" | "fleet";
  report_id: string;
  period_days: number;
  channel_telegram: boolean;
  enabled: boolean;
  frequency: string;
  day_of_week?: number;
  days_of_week: number[];
  day_of_month: number;
  time_hhmm: string;
  timezone: string;
  last_run_at?: string;
  last_run_ok?: boolean;
  last_run_message?: string;
  running: boolean;
};

function formatWhen(iso?: string) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
  } catch {
    return iso;
  }
}

function draftFromAutomation(a: CustomAutomation): RecurrenceDraft {
  const kind: RecurrenceKind =
    a.frequency === "monthly" ? "monthly" : a.frequency === "custom" || a.days_of_week.length > 1 ? "custom" : a.frequency === "weekly" ? "weekly" : "daily";
  return {
    kind,
    time: (a.time_hhmm || "08:00").slice(0, 5),
    timezone: a.timezone || TZ_DEFAULT,
    weekdays: a.days_of_week.length ? a.days_of_week : a.day_of_week != null ? [a.day_of_week] : [1],
    dayOfMonth: a.day_of_month || 1,
  };
}

function scheduleText(a: CustomAutomation): string {
  const hhmm = (a.time_hhmm || "08:00").slice(0, 5);
  if (a.frequency === "monthly") return `Todo dia ${a.day_of_month} às ${hhmm}`;
  if (a.frequency === "daily") return `Todos os dias às ${hhmm}`;
  const labels = ["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"];
  const days = a.days_of_week.length ? a.days_of_week : a.day_of_week != null ? [a.day_of_week] : [1];
  return `${days.map((d) => labels[d] ?? d).join(", ")} às ${hhmm}`;
}

function AutomationFormModal({
  editing,
  systemReports,
  onClose,
  onSaved,
}: {
  editing: CustomAutomation | null;
  systemReports: SystemReportCatalogItem[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const { push: pushToast } = useAppToast();
  const [name, setName] = useState(editing?.name ?? "");
  const [domain, setDomain] = useState<"system" | "fleet">(editing?.domain ?? "system");
  const [reportId, setReportId] = useState(editing?.report_id ?? "");
  const [periodDays, setPeriodDays] = useState(editing?.period_days ?? 30);
  const [enabled, setEnabled] = useState(editing?.enabled ?? true);
  const [recurrence, setRecurrence] = useState<RecurrenceDraft>(editing ? draftFromAutomation(editing) : emptyRecurrence("daily"));
  const [saving, setSaving] = useState(false);

  const options: { id: string; title: string }[] =
    domain === "system" ? systemReports.map((r) => ({ id: r.id, title: r.title })) : FLEET_REPORT_OPTIONS.map((f) => ({ id: f.id, title: f.title }));

  useEffect(() => {
    if (!reportId && options.length > 0) setReportId(options[0].id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [domain]);

  async function save() {
    if (!name.trim() || !reportId) {
      toastErr(pushToast, new Error("Nome e relatório são obrigatórios."));
      return;
    }
    setSaving(true);
    try {
      const body = {
        name: name.trim(),
        domain,
        report_id: reportId,
        period_days: periodDays,
        channel_telegram: true,
        enabled,
        frequency: recurrence.kind,
        day_of_week: recurrence.weekdays[0] ?? 1,
        days_of_week: recurrence.weekdays,
        day_of_month: recurrence.dayOfMonth,
        time_hhmm: recurrence.time,
        timezone: recurrence.timezone,
      };
      if (editing) {
        await apiFetch(`/api/v1/settings/automation/schedules/${editing.id}`, { method: "PATCH", json: body });
      } else {
        await apiFetch("/api/v1/settings/automation/schedules", { method: "POST", json: body });
      }
      toastOk(pushToast, editing ? "Automação atualizada." : "Automação criada.");
      onSaved();
      onClose();
    } catch (e) {
      toastErr(pushToast, e, "Falha ao gravar.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !saving && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 620 }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>{editing ? "Editar automação" : "Nova automação"}</h3>
        <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 12 }}>
          <SettingsField label="Nome">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Ex.: Financeiro HubSoft semanal" />
          </SettingsField>
          <div className="settings-fields-grid">
            <SettingsField label="Categoria">
              <select className="input" value={domain} onChange={(e) => { setDomain(e.target.value as "system" | "fleet"); setReportId(""); }}>
                <option value="system">Relatórios do sistema (alertas, BGP, HubSoft, tráfego, OLT, BNG…)</option>
                <option value="fleet">Frota / combustível</option>
              </select>
            </SettingsField>
            <SettingsField label="Relatório">
              <select className="input" value={reportId} onChange={(e) => setReportId(e.target.value)}>
                {options.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.title}
                  </option>
                ))}
              </select>
            </SettingsField>
          </div>
          <SettingsField label="Janela de dados (dias)">
            <input
              className="input"
              type="number"
              min={1}
              max={366}
              value={periodDays}
              onChange={(e) => setPeriodDays(Number(e.target.value) || 30)}
              style={{ maxWidth: 120 }}
            />
          </SettingsField>
          <label className="row" style={{ gap: 8 }}>
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
            Agendamento ativo
          </label>
          <AutomationRecurrenceFields
            value={recurrence}
            allowed={["daily", "weekly", "custom", "monthly"]}
            onChange={setRecurrence}
          />
          <p style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
            Envio por Telegram (bot "reports", Configurações → Telegram).
          </p>
        </div>
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 18 }}>
          <button type="button" className="btn" disabled={saving} onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={saving} onClick={() => void save()}>
            {saving ? "A gravar…" : "Guardar"}
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * Automações personalizadas: ao contrário dos 5 cartões fixos acima (1 instância cada), aqui o
 * usuário cria QUANTAS automações quiser, cada uma escolhendo QUALQUER relatório do catálogo do
 * sistema (inclui BGP, HubSoft, alertas, tráfego, OLT, BNG, etc. — ver /api/v1/reports/system)
 * ou um relatório de frota/combustível, com a sua própria recorrência.
 */
export function CustomAutomationsPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<CustomAutomation | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CustomAutomation | null>(null);
  const [deleting, setDeleting] = useState(false);

  const listQ = useQuery({
    queryKey: ["automation-schedules"],
    queryFn: () => apiFetch<{ schedules: CustomAutomation[] }>("/api/v1/settings/automation/schedules"),
    refetchInterval: 8000,
  });
  const catalogQ = useQuery({ queryKey: ["system-report-catalog"], queryFn: fetchSystemReportCatalog });
  const schedules = listQ.data?.schedules ?? [];
  const catalog = catalogQ.data?.reports ?? [];

  const runM = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/settings/automation/schedules/${id}/run`, { method: "POST" }),
    onSuccess: () => {
      toastOk(pushToast, "Automação executada.");
      void qc.invalidateQueries({ queryKey: ["automation-schedules"] });
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao executar."),
  });

  async function removeSchedule() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await apiFetch(`/api/v1/settings/automation/schedules/${deleteTarget.id}`, { method: "DELETE" });
      toastOk(pushToast, "Automação removida.");
      setDeleteTarget(null);
      void qc.invalidateQueries({ queryKey: ["automation-schedules"] });
    } catch (e) {
      toastErr(pushToast, e, "Falha ao remover.");
    } finally {
      setDeleting(false);
    }
  }

  const reportLabel = useMemo(() => {
    const byId = new Map<string, string>(catalog.map((c) => [c.id as string, c.title]));
    return (a: CustomAutomation) => (a.domain === "fleet" ? FLEET_REPORT_OPTIONS.find((f) => f.id === a.report_id)?.title ?? a.report_id : byId.get(a.report_id) ?? a.report_id);
  }, [catalog]);

  return (
    <div className="card automations-register" style={{ marginTop: 12 }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <div className="automations-register__title" style={{ display: "flex", alignItems: "center", gap: 6 }}>
          Automações personalizadas
          <InfoHint label="Sobre automações personalizadas">
            <p>
              Crie quantas automações quiser, escolhendo qualquer relatório do sistema (alertas, BGP, HubSoft, tráfego,
              OLT, BNG, comercial, etc.) ou um relatório de frota/combustível, com a sua própria recorrência — enviado
              por Telegram (bot "reports").
            </p>
          </InfoHint>
        </div>
        <button type="button" className="btn btn--primary btn--sm" onClick={() => { setEditing(null); setFormOpen(true); }}>
          <Plus size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
          Nova automação
        </button>
      </div>

      {listQ.isLoading ? (
        <p style={{ fontSize: 13, color: "var(--muted)", marginTop: 10 }}>A carregar…</p>
      ) : schedules.length === 0 ? (
        <div className="automations-empty">
          <p>Nenhuma automação personalizada cadastrada.</p>
        </div>
      ) : (
        <div className="table-wrap" style={{ marginTop: 10 }}>
          <table style={{ fontSize: 12 }}>
            <thead>
              <tr>
                <th>Nome</th>
                <th>Relatório</th>
                <th>Recorrência</th>
                <th>Última execução</th>
                <th>Estado</th>
                <th style={{ width: 140 }} />
              </tr>
            </thead>
            <tbody>
              {schedules.map((a) => (
                <tr key={a.id}>
                  <td>{a.name}</td>
                  <td>{reportLabel(a)}</td>
                  <td>{scheduleText(a)}</td>
                  <td>
                    {formatWhen(a.last_run_at)}
                    {a.last_run_ok === false ? <div style={{ color: "var(--err)", fontSize: 10 }}>{a.last_run_message}</div> : null}
                  </td>
                  <td>
                    {a.running ? (
                      <span className="automations-pill automations-pill--run">A correr</span>
                    ) : a.enabled ? (
                      <span className="automations-pill automations-pill--on">Activa</span>
                    ) : (
                      <span className="automations-pill">Desligada</span>
                    )}
                  </td>
                  <td>
                    <div className="row" style={{ gap: 4, justifyContent: "flex-end" }}>
                      <button type="button" className="btn btn--icon" title="Executar agora" disabled={runM.isPending || a.running} onClick={() => runM.mutate(a.id)}>
                        <Send size={13} />
                      </button>
                      <button type="button" className="btn btn--icon" title="Editar" onClick={() => { setEditing(a); setFormOpen(true); }}>
                        <Pencil size={13} />
                      </button>
                      <button type="button" className="btn btn--icon" title="Remover" onClick={() => setDeleteTarget(a)}>
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formOpen ? (
        <AutomationFormModal
          editing={editing}
          systemReports={catalog}
          onClose={() => setFormOpen(false)}
          onSaved={() => void qc.invalidateQueries({ queryKey: ["automation-schedules"] })}
        />
      ) : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Remover automação"
        message={deleteTarget ? `Remover "${deleteTarget.name}"?` : ""}
        confirmLabel="Remover"
        busy={deleting}
        onCancel={() => !deleting && setDeleteTarget(null)}
        onConfirm={() => void removeSchedule()}
      />
    </div>
  );
}
