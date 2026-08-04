import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { SettingsField } from "../../components/SettingsField";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

const TZ_DEFAULT = "America/Sao_Paulo";

type BackupSchedule = {
  enabled: boolean;
  frequency?: string;
  day_of_week?: number | null;
  time_hhmm: string;
  timezone: string;
  keep_last?: number;
  last_status?: string | null;
  last_error?: string | null;
  last_object_key?: string | null;
  last_size_bytes?: number | null;
  running: boolean;
};

type B2Settings = {
  key_id?: string | null;
  bucket?: string | null;
  bucket_id?: string | null;
  endpoint?: string | null;
  region?: string | null;
  prefix?: string | null;
  application_key_set: boolean;
};

export function DatabaseBackupAutomationCard() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const cfg = useQuery({
    queryKey: queryKeys.automationDatabaseBackup,
    queryFn: () => apiFetch<BackupSchedule>("/api/v1/settings/automation/database-backup"),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  });
  const b2 = useQuery({
    queryKey: queryKeys.settingsB2Backup,
    queryFn: () => apiFetch<B2Settings>("/api/v1/settings/backup/b2"),
  });

  const [enabled, setEnabled] = useState(false);
  const [freq, setFreq] = useState("daily");
  const [dow, setDow] = useState("1");
  const [timeVal, setTimeVal] = useState("03:00");
  const [tz, setTz] = useState(TZ_DEFAULT);
  const [keep, setKeep] = useState("14");

  const [keyId, setKeyId] = useState("");
  const [appKey, setAppKey] = useState("");
  const [bucket, setBucket] = useState("");
  const [bucketId, setBucketId] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [region, setRegion] = useState("us-east-005");
  const [prefix, setPrefix] = useState("netquasar/postgres");

  useEffect(() => {
    if (!cfg.data) return;
    setEnabled(cfg.data.enabled);
    setFreq(cfg.data.frequency ?? "daily");
    setDow(cfg.data.day_of_week != null ? String(cfg.data.day_of_week) : "1");
    setTimeVal((cfg.data.time_hhmm ?? "03:00").slice(0, 5));
    setTz(cfg.data.timezone?.trim() || TZ_DEFAULT);
    setKeep(String(cfg.data.keep_last ?? 14));
  }, [cfg.data]);

  useEffect(() => {
    if (!b2.data) return;
    setKeyId(b2.data.key_id ?? "");
    setBucket(b2.data.bucket ?? "");
    setBucketId(b2.data.bucket_id ?? "");
    setEndpoint(b2.data.endpoint ?? "");
    setRegion(b2.data.region ?? "us-east-005");
    setPrefix(b2.data.prefix ?? "netquasar/postgres");
  }, [b2.data]);

  const patchSched = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/automation/database-backup", {
        method: "PATCH",
        json: {
          enabled,
          frequency: freq,
          day_of_week: Number(dow),
          time_hhmm: timeVal,
          timezone: tz,
          keep_last: Number(keep) || 14,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationDatabaseBackup });
      toastOk(pushToast, "Agendamento de backup salvo.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar agendamento."),
  });

  const patchB2 = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/backup/b2", {
        method: "PATCH",
        json: {
          key_id: keyId.trim() || null,
          application_key: appKey.trim() || null,
          bucket: bucket.trim() || null,
          bucket_id: bucketId.trim() || null,
          endpoint: endpoint.trim() || null,
          region: region.trim() || null,
          prefix: prefix.trim() || null,
        },
      }),
    onSuccess: () => {
      setAppKey("");
      void qc.invalidateQueries({ queryKey: queryKeys.settingsB2Backup });
      toastOk(pushToast, "Credenciais Backblaze salvas.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar B2."),
  });

  const testB2 = useMutation({
    mutationFn: () => apiFetch<{ dump_count?: number }>("/api/v1/settings/backup/b2/test", { method: "POST", json: {} }),
    onSuccess: (d) => toastOk(pushToast, `B2 OK (${d.dump_count ?? 0} dumps listados).`),
    onError: (err) => toastErr(pushToast, err, "Teste B2 falhou."),
  });

  const run = useMutation({
    mutationFn: () => apiFetch("/api/v1/settings/automation/database-backup/run", { method: "POST", json: {} }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.automationDatabaseBackup });
      void qc.invalidateQueries({ queryKey: queryKeys.automationHistory });
      toastOk(pushToast, "Backup iniciado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao iniciar backup."),
  });

  const busy = !!cfg.data?.running || run.isPending;

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h2 style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        Backup PostgreSQL (Backblaze B2)
        <InfoHint label="Backup periódico">
          <p>
            Gera um dump full (<span className="mono">pg_dump</span>) da base activa e envia para o bucket B2 no prefixo
            configurado. Requer <span className="mono">postgresql-client</span> no servidor (incluído na imagem Docker).
          </p>
        </InfoHint>
      </h2>
      {cfg.data?.running ? (
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>Backup em curso…</p>
      ) : null}
      {cfg.data?.last_status && (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>
          Último: {cfg.data.last_status}
          {cfg.data.last_object_key ? ` — ${cfg.data.last_object_key}` : ""}
          {cfg.data.last_size_bytes != null ? ` (${(cfg.data.last_size_bytes / (1024 * 1024)).toFixed(1)} MB)` : ""}
          {cfg.data.last_error ? ` — ${cfg.data.last_error}` : ""}
        </p>
      )}

      <h3 style={{ fontSize: 13, marginTop: 14, display: "flex", alignItems: "center", gap: 6 }}>
        Credenciais Backblaze
        <InfoHint label="Onde obter no B2">
          <p>
            No painel Backblaze: <strong>Application Keys</strong> → criar chave (ou usar a master). Copie o{" "}
            <strong>keyID</strong> e a <strong>applicationKey</strong> (a key só aparece uma vez).
          </p>
          <p>
            Em <strong>Buckets</strong>, use o nome do bucket (ex.: <span className="mono">NetQuasar</span>). O Bucket ID
            aparece na página do bucket (opcional). A região/endpoint estão na mesma página (ex.:{" "}
            <span className="mono">us-east-005</span>).
          </p>
        </InfoHint>
      </h3>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Application key: {b2.data?.application_key_set ? "configurada" : "não definida"} · Obrigatórios: Key ID, Application
        Key e Bucket.
      </p>
      <div className="settings-fields-grid" style={{ marginTop: 8 }}>
        <SettingsField
          label="Key ID"
          hintLabel="Key ID"
          hint={<p>Valor keyID ao criar a Application Key no Backblaze (Application Keys).</p>}
        >
          <input
            className="input mono"
            value={keyId}
            onChange={(e) => setKeyId(e.target.value)}
            placeholder="001a2b3c4d5e6f70000000001"
            autoComplete="off"
          />
        </SettingsField>
        <SettingsField
          label="Application Key"
          hintLabel="Application Key"
          hint={<p>Segredo mostrado uma vez ao criar a chave. Deixe vazio para manter a já salva.</p>}
        >
          <input
            className="input"
            type="password"
            value={appKey}
            onChange={(e) => setAppKey(e.target.value)}
            placeholder={b2.data?.application_key_set ? "(manter atual)" : "K001…"}
            autoComplete="new-password"
          />
        </SettingsField>
        <SettingsField label="Bucket" hintLabel="Bucket" hint={<p>Nome do bucket (ex.: NetQuasar), não o Bucket ID.</p>}>
          <input className="input" value={bucket} onChange={(e) => setBucket(e.target.value)} placeholder="NetQuasar" />
        </SettingsField>
        <SettingsField label="Bucket ID" hintLabel="Bucket ID" hint={<p>Opcional. Aparece na página do bucket no B2.</p>}>
          <input
            className="input mono"
            value={bucketId}
            onChange={(e) => setBucketId(e.target.value)}
            placeholder="opcional"
          />
        </SettingsField>
        <SettingsField
          label="Prefixo"
          hintLabel="Prefixo"
          hint={<p>Pasta lógica dos dumps no bucket. Padrão: netquasar/postgres.</p>}
        >
          <input
            className="input mono"
            value={prefix}
            onChange={(e) => setPrefix(e.target.value)}
            placeholder="netquasar/postgres"
          />
        </SettingsField>
        <SettingsField label="Região" hintLabel="Região" hint={<p>Região do bucket, ex.: us-east-005.</p>}>
          <input className="input" value={region} onChange={(e) => setRegion(e.target.value)} placeholder="us-east-005" />
        </SettingsField>
        <SettingsField
          label="Endpoint S3"
          hintLabel="Endpoint"
          hint={<p>Opcional. Ex.: https://s3.us-east-005.backblazeb2.com — se vazio, deriva da região.</p>}
        >
          <input
            className="input mono"
            value={endpoint}
            onChange={(e) => setEndpoint(e.target.value)}
            placeholder="https://s3.us-east-005.backblazeb2.com"
          />
        </SettingsField>
      </div>
      <div className="row" style={{ marginTop: 10, gap: 8, flexWrap: "wrap" }}>
        <button type="button" className="btn btn--primary" disabled={patchB2.isPending} onClick={() => patchB2.mutate()}>
          Salvar B2
        </button>
        <button type="button" className="btn" disabled={testB2.isPending} onClick={() => testB2.mutate()}>
          Testar B2
        </button>
      </div>

      <h3 style={{ fontSize: 13, marginTop: 18 }}>Agendamento</h3>
      <label className="row" style={{ gap: 8, marginTop: 8 }}>
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
          <SettingsField label="Dia da semana (0=Dom)">
            <input className="input" value={dow} onChange={(e) => setDow(e.target.value)} disabled={busy || !enabled} />
          </SettingsField>
        )}
        <SettingsField label="Hora (HH:MM)">
          <input className="input" value={timeVal} onChange={(e) => setTimeVal(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
        <SettingsField label="Fuso">
          <input className="input" value={tz} onChange={(e) => setTz(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
        <SettingsField label="Manter últimos (N)">
          <input className="input" value={keep} onChange={(e) => setKeep(e.target.value)} disabled={busy || !enabled} />
        </SettingsField>
      </div>
      <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap" }}>
        <button type="button" className="btn btn--primary" disabled={patchSched.isPending || busy} onClick={() => patchSched.mutate()}>
          Salvar agendamento
        </button>
        <button type="button" className="btn" disabled={busy} onClick={() => run.mutate()}>
          Executar agora
        </button>
      </div>
    </div>
  );
}
