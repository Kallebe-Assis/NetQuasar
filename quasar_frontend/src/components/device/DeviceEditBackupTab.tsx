import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { apiFetch, downloadBlob } from "../../lib/api";
import { apiUrl, getStoredApiKey } from "../../lib/auth";

type BackupPayload = {
  device_id: string;
  content: string;
  updated_at: string | null;
};

type Props = {
  deviceId: string;
  deviceLabel: string;
  canMutate: boolean;
};

export function DeviceEditBackupTab({ deviceId, deviceLabel, canMutate }: Props) {
  const qc = useQueryClient();
  const [content, setContent] = useState("");
  const [dirty, setDirty] = useState(false);
  const [toast, setToast] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (!toast) return;
    const t = window.setTimeout(() => setToast(null), 10_000);
    return () => window.clearTimeout(t);
  }, [toast]);

  const backup = useQuery({
    queryKey: ["device-config-backup", deviceId],
    queryFn: () => apiFetch<BackupPayload>(`/api/v1/devices/${deviceId}/config-backup`),
    enabled: !!deviceId,
  });

  useEffect(() => {
    if (backup.data && !dirty) {
      setContent(backup.data.content ?? "");
    }
  }, [backup.data, dirty]);

  const save = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/devices/${deviceId}/config-backup`, {
        method: "PUT",
        json: { content },
      }),
    onSuccess: () => {
      setDirty(false);
      void qc.invalidateQueries({ queryKey: ["device-config-backup", deviceId] });
      setToast({ ok: true, text: "Backup salvo na base de dados." });
    },
    onError: (err) => setToast({ ok: false, text: (err as Error).message || "Falha ao salvar." }),
  });

  async function exportCsv() {
    const headers: Record<string, string> = { Accept: "text/csv" };
    const key = getStoredApiKey();
    if (key) headers["X-API-Key"] = key;
    const token = sessionStorage.getItem("netquasar_token");
    if (token) headers.Authorization = `Bearer ${token}`;
    const res = await fetch(apiUrl(`/api/v1/devices/${deviceId}/config-backup/export`), { headers });
    if (!res.ok) throw new Error(await res.text());
    const blob = await res.blob();
    const safe = deviceLabel.replace(/[^\w\-]+/g, "_").slice(0, 40) || "equipamento";
    downloadBlob(`backup_${safe}.csv`, blob);
  }

  if (backup.isLoading) return <p>A carregar backup…</p>;
  if (backup.isError) return <div className="msg msg--err">{(backup.error as Error).message}</div>;

  return (
    <>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Cole aqui o script ou texto de configuração deste equipamento (export RouterOS, running-config, etc.). O conteúdo fica
        persistido na base de dados e pode ser exportado em CSV.
      </p>
      {backup.data?.updated_at && (
        <p style={{ fontSize: 11, color: "var(--muted)" }}>
          Última gravação: <span className="mono">{backup.data.updated_at}</span>
        </p>
      )}
      <textarea
        className="input mono"
        style={{ width: "100%", minHeight: 280, resize: "vertical", fontSize: 12, lineHeight: 1.45 }}
        value={content}
        disabled={!canMutate}
        placeholder="# Cole a configuração do equipamento…"
        onChange={(e) => {
          setContent(e.target.value);
          setDirty(true);
        }}
      />
      <div className="row" style={{ marginTop: 10, gap: 8, flexWrap: "wrap" }}>
        {canMutate && (
          <button type="button" className="btn btn--primary" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "A salvar…" : "Salvar backup"}
          </button>
        )}
        <button
          type="button"
          className="btn"
          onClick={() => {
            void exportCsv().catch((e) => setToast({ ok: false, text: (e as Error).message }));
          }}
        >
          Exportar CSV
        </button>
      </div>
      {toast && (
        <div className={`msg ${toast.ok ? "msg--ok" : "msg--err"}`} style={{ marginTop: 10 }}>
          {toast.text}
        </div>
      )}
    </>
  );
}
