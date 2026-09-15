import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, RotateCw, ShieldAlert } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr } from "../../lib/operationToast";
import { ConfirmModal } from "../../components/ConfirmModal";

const REPO_URL = "https://github.com/Kallebe-Assis/NetQuasar";

type UpdateStatus =
  | "idle"
  | "check_requested"
  | "checking"
  | "check_done"
  | "update_requested"
  | "updating"
  | "completed"
  | "failed";

const BUSY_STATUSES: UpdateStatus[] = ["check_requested", "checking", "update_requested", "updating"];

type CommitInfo = { sha: string; short: string; author: string; date: string; message: string };

type CheckResult = { ahead_by: number; behind_by: number; checked_at: string; commits: CommitInfo[] };

type UpdateState = {
  status: UpdateStatus;
  base_commit?: string;
  requested_at?: string;
  check_result?: CheckResult;
  started_at?: string;
  completed_at?: string;
  error_message?: string;
};

type VersionResponse = {
  running_commit: string;
  running_commit_short: string;
  build_time: string;
  branch: string;
  update_state: UpdateState;
};

async function fetchVersion() {
  return apiFetch<VersionResponse>("/api/v1/system/version");
}

function formatDateTime(iso?: string) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("pt-BR");
}

export function SystemVersionTab() {
  const { push: pushToast } = useAppToast();
  const qc = useQueryClient();
  const admin = isAdminUser();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [waitingReload, setWaitingReload] = useState(false);
  const sawFailureRef = useRef(false);

  const versionQ = useQuery({
    queryKey: ["system-version"],
    queryFn: fetchVersion,
    retry: waitingReload ? true : 3,
    refetchInterval: (q) => (BUSY_STATUSES.includes(q.state.data?.update_state.status ?? "idle") ? 3000 : false),
  });

  // Enquanto estivermos à espera do restart (depois de disparar "Atualizar agora"), qualquer
  // falha de rede é esperada (o container antigo morreu, o novo ainda não respondeu) — assim que
  // uma resposta suceder de novo com status "completed", recarrega a tela sozinha.
  useEffect(() => {
    if (!waitingReload) return;
    if (versionQ.isError) {
      sawFailureRef.current = true;
      return;
    }
    const status = versionQ.data?.update_state.status;
    if (sawFailureRef.current && status === "completed") {
      window.location.reload();
    } else if (status === "failed") {
      setWaitingReload(false);
    }
  }, [waitingReload, versionQ.isError, versionQ.data]);

  const checkMut = useMutation({
    mutationFn: () => apiFetch("/api/v1/system/version/check", { method: "POST" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["system-version"] }),
    onError: (e) => toastErr(pushToast, e, "Falha ao pedir verificação."),
  });

  const updateMut = useMutation({
    mutationFn: () => apiFetch("/api/v1/system/update", { method: "POST" }),
    onSuccess: () => {
      setConfirmOpen(false);
      setWaitingReload(true);
      sawFailureRef.current = false;
      void qc.invalidateQueries({ queryKey: ["system-version"] });
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao pedir atualização."),
  });

  const data = versionQ.data;
  const state = data?.update_state;
  const status = state?.status ?? "idle";
  const busy = BUSY_STATUSES.includes(status) || waitingReload;
  const behindBy = state?.check_result?.behind_by ?? 0;
  const canUpdate = admin && status === "check_done" && behindBy > 0;

  return (
    <section className="card about-section about-section--wide">
      <h3>Versão do sistema</h3>

      {versionQ.isLoading ? <p style={{ color: "var(--muted)" }}>A carregar…</p> : null}

      {data ? (
        <>
          <dl className="about-hero__meta" style={{ marginBottom: 14 }}>
            <div>
              <dt>Commit em execução</dt>
              <dd>
                {data.running_commit === "unknown" ? (
                  <span style={{ color: "var(--muted)" }}>Desconhecido (build local sem info de commit)</span>
                ) : (
                  <a href={`${REPO_URL}/commit/${data.running_commit}`} target="_blank" rel="noopener noreferrer" className="mono">
                    {data.running_commit_short}
                  </a>
                )}
              </dd>
            </div>
            <div>
              <dt>Build</dt>
              <dd>{data.build_time === "unknown" ? "—" : formatDateTime(data.build_time)}</dd>
            </div>
            <div>
              <dt>Branch</dt>
              <dd className="mono">{data.branch}</dd>
            </div>
          </dl>

          {waitingReload ? (
            <p className="msg">
              A atualizar o sistema — o servidor vai reiniciar e esta tela vai recarregar sozinha em instantes. Não feche
              esta aba.
            </p>
          ) : null}

          {!waitingReload && status === "failed" && state?.error_message ? (
            <p className="msg msg--err">{state.error_message}</p>
          ) : null}

          {!waitingReload && (status === "checking" || status === "check_requested") ? (
            <p className="msg">A verificar no GitHub…</p>
          ) : null}

          {!waitingReload && status === "check_done" && state?.check_result ? (
            behindBy === 0 ? (
              <p className="msg msg--ok">Já está na versão mais recente ({REPO_URL.replace("https://github.com/", "")}).</p>
            ) : (
              <div className="msg msg--warn">
                <p style={{ margin: "0 0 8px" }}>
                  {behindBy} commit(s) atrás de <span className="mono">{data.branch}</span> no GitHub
                  {(state.check_result.ahead_by ?? 0) > 0 ? ` (e ${state.check_result.ahead_by} à frente — verifique manualmente)` : ""}.
                </p>
                {state.check_result.commits?.length ? (
                  <ul style={{ margin: "0 0 8px", paddingLeft: 18, fontSize: 12 }}>
                    {state.check_result.commits.slice(0, 10).map((c) => (
                      <li key={c.sha}>
                        <span className="mono">{c.short}</span> {c.message} <span style={{ color: "var(--muted)" }}>— {c.author}</span>
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
            )
          ) : null}

          <div className="row" style={{ gap: 8, marginTop: 10, flexWrap: "wrap" }}>
            <button
              type="button"
              className="btn"
              disabled={busy || checkMut.isPending}
              onClick={() => checkMut.mutate()}
            >
              <RefreshCw size={14} /> {checkMut.isPending || status === "checking" ? "A verificar…" : "Verificar atualização"}
            </button>
            {admin ? (
              <button
                type="button"
                className="btn btn--primary"
                disabled={!canUpdate || busy}
                title={!canUpdate ? "Verifique se há atualização primeiro." : undefined}
                onClick={() => setConfirmOpen(true)}
              >
                <RotateCw size={14} /> Atualizar agora
              </button>
            ) : (
              <span className="row" style={{ gap: 6, color: "var(--muted)", fontSize: 12, alignItems: "center" }}>
                <ShieldAlert size={13} /> Só administradores podem aplicar atualizações.
              </span>
            )}
          </div>
        </>
      ) : null}

      <ConfirmModal
        open={confirmOpen}
        title="Atualizar o sistema"
        message="O servidor vai buscar a versão mais recente no GitHub, reconstruir e reiniciar — leva cerca de 1 a 2 minutos. O monitoramento pára automaticamente antes e volta ao estado em que estava assim que terminar. Esta tela recarrega sozinha ao fim."
        confirmLabel={updateMut.isPending ? "A iniciar…" : "Atualizar agora"}
        busy={updateMut.isPending}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => updateMut.mutate()}
      />
    </section>
  );
}
