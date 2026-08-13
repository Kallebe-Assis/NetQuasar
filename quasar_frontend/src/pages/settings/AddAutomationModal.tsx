import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import {
  AUTOMATION_JOBS,
  emptyRecurrence,
  formatRecurrence,
  recurrenceToPatch,
  type AutomationJobType,
  type RecurrenceDraft,
} from "../../lib/automationJobs";
import { queryKeys } from "../../lib/queryKeys";
import { AutomationRecurrenceFields } from "./AutomationRecurrenceFields";

type Props = {
  open: boolean;
  takenIds: string[];
  onClose: () => void;
  onCreated: (jobType: AutomationJobType) => void;
};

export function AddAutomationModal({ open, takenIds, onClose, onCreated }: Props) {
  const qc = useQueryClient();
  const available = useMemo(() => AUTOMATION_JOBS.filter((j) => !takenIds.includes(j.id)), [takenIds]);
  const [jobId, setJobId] = useState<AutomationJobType | "">("");
  const [rec, setRec] = useState<RecurrenceDraft>(emptyRecurrence("daily"));
  const [err, setErr] = useState<string | null>(null);

  const job = AUTOMATION_JOBS.find((j) => j.id === jobId) ?? null;

  function pickJob(id: AutomationJobType) {
    const def = AUTOMATION_JOBS.find((j) => j.id === id);
    setJobId(id);
    setErr(null);
    setRec(emptyRecurrence(def?.recurrences[0] ?? "daily"));
  }

  const save = useMutation({
    mutationFn: async () => {
      if (!job) throw new Error("Seleccione a automação.");
      if (job.recurrences.includes("custom") && rec.kind === "custom" && rec.weekdays.length === 0) {
        throw new Error("Seleccione pelo menos um dia da semana.");
      }
      await apiFetch(job.patchPath, { method: "PATCH", json: recurrenceToPatch(job.id, rec, true) });
      return job.id;
    },
    onSuccess: async (id) => {
      await qc.invalidateQueries({ queryKey: queryKeys.automationOverview });
      await qc.invalidateQueries({ queryKey: queryKeys.automationAlertsDigest });
      await qc.invalidateQueries({ queryKey: queryKeys.automationBngStats });
      await qc.invalidateQueries({ queryKey: queryKeys.automationCommercial });
      await qc.invalidateQueries({ queryKey: queryKeys.automationOnu });
      await qc.invalidateQueries({ queryKey: queryKeys.automationDatabaseBackup });
      onCreated(id);
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao cadastrar."),
  });

  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal automation-add-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="automation-add-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
          <h3 id="automation-add-title" style={{ margin: 0 }}>
            Nova automação
          </h3>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>
        <p style={{ margin: "0 0 14px", fontSize: 13, color: "var(--muted)" }}>
          Escolha a função, a recorrência e a hora. Depois pode ajustar canais, testes e histórico no cadastro.
        </p>

        {available.length === 0 ? (
          <p style={{ fontSize: 13, color: "var(--muted)" }}>Todas as automações já estão cadastradas.</p>
        ) : (
          <>
            <div className="automation-add-types" role="list">
              {available.map((j) => (
                <button
                  key={j.id}
                  type="button"
                  role="listitem"
                  className={`automation-add-type${jobId === j.id ? " is-active" : ""}`}
                  onClick={() => pickJob(j.id)}
                >
                  <span className="automation-add-type__name">{j.label}</span>
                  <span className="automation-add-type__desc">{j.description}</span>
                </button>
              ))}
            </div>

            {job ? (
              <div style={{ marginTop: 16 }}>
                <div className="automation-add-modal__label">Recorrência</div>
                <AutomationRecurrenceFields value={rec} allowed={job.recurrences} onChange={setRec} />
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "10px 0 0" }}>{formatRecurrence(rec)}</p>
              </div>
            ) : null}

            {err ? <div className="msg msg--err" style={{ marginTop: 12 }}>{err}</div> : null}

            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
              <button type="button" className="btn" onClick={onClose} disabled={save.isPending}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={!job || save.isPending}
                onClick={() => save.mutate()}
              >
                {save.isPending ? "A guardar…" : "Cadastrar"}
              </button>
            </div>
          </>
        )}
      </div>
    </div>,
    document.body,
  );
}
