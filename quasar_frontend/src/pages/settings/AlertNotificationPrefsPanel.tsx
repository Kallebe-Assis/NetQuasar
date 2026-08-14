import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, BellOff, Play, Trash2, Upload, Volume2, VolumeX } from "lucide-react";
import { useRef, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { playAlertSound } from "../../lib/alertSound";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";
import {
  BUILTIN_ALERT_SOUNDS,
  deleteAlertSound,
  fetchMyPreferences,
  patchMyPreferences,
  uploadAlertSound,
  type UserPreferences,
} from "../../lib/userPreferences";

function PrefSwitch({
  checked,
  onChange,
  disabled,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
  label: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      className={`mon-cfg__switch${checked ? " is-on" : ""}`}
      onClick={() => onChange(!checked)}
    >
      <span className="mon-cfg__switch-thumb" />
    </button>
  );
}

export function AlertNotificationPrefsPanel() {
  const qc = useQueryClient();
  const { push } = useAppToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);

  const q = useQuery({
    queryKey: queryKeys.mePreferences,
    queryFn: fetchMyPreferences,
  });
  const prefs = q.data;

  const save = useMutation({
    mutationFn: (patch: Partial<UserPreferences>) => patchMyPreferences(patch),
    onSuccess: (next) => {
      qc.setQueryData(queryKeys.mePreferences, next);
    },
    onError: (e) => toastErr(push, e, "Falha ao guardar preferência"),
  });

  async function onUpload(file: File | undefined) {
    if (!file) return;
    if (!/\.mp3$/i.test(file.name) && file.type !== "audio/mpeg" && file.type !== "audio/mp3") {
      toastErr(push, new Error("Envie um ficheiro MP3."), "Formato inválido");
      return;
    }
    setUploading(true);
    try {
      await uploadAlertSound(file);
      await qc.invalidateQueries({ queryKey: queryKeys.mePreferences });
      toastOk(push, "MP3 adicionado e seleccionado como som de alerta.");
    } catch (e) {
      toastErr(push, e, "Falha ao enviar MP3");
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function onDelete(id: string) {
    try {
      const res = await deleteAlertSound(id);
      if (res.preferences) qc.setQueryData(queryKeys.mePreferences, res.preferences);
      else await qc.invalidateQueries({ queryKey: queryKeys.mePreferences });
      toastOk(push, "Som removido.");
    } catch (e) {
      toastErr(push, e, "Falha ao remover som");
    }
  }

  const custom = prefs?.custom_sounds ?? [];
  const selected = prefs?.alert_sound_id ?? "builtin:alert";

  return (
    <div className="panel user-alert-prefs" style={{ marginBottom: 18 }}>
      <h2 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
        Notificações neste dispositivo
        <InfoHint label="Toasts e som de alerta">
          <p>
            Estas opções são só suas — cada usuário escolhe se quer toast em qualquer ecrã e se o som toca.
            Usuários novos têm ambos ligados.
          </p>
        </InfoHint>
      </h2>
      <p style={{ margin: "0 0 14px", color: "var(--muted)", fontSize: 13 }}>
        Preferência individual. Não altera limiares globais nem o Telegram.
      </p>
      {q.isLoading ? <p style={{ color: "var(--muted)" }}>A carregar…</p> : null}

      <div className="user-alert-prefs__row">
        <div className="user-alert-prefs__copy">
          <strong>
            <Bell size={16} style={{ verticalAlign: -3, marginRight: 6 }} />
            Toast em qualquer ecrã
          </strong>
          <span>
            Ligado: o aviso aparece no dashboard, frota, relatórios e restantes ecrãs. Desligado: só em
            Monitoramento e Alertas.
          </span>
        </div>
        <PrefSwitch
          label="Toast em qualquer ecrã"
          checked={prefs?.alert_toast_everywhere !== false}
          disabled={!prefs || save.isPending}
          onChange={(v) => save.mutate({ alert_toast_everywhere: v })}
        />
      </div>

      <div className="user-alert-prefs__row">
        <div className="user-alert-prefs__copy">
          <strong>
            {prefs?.alert_sound_enabled === false ? (
              <VolumeX size={16} style={{ verticalAlign: -3, marginRight: 6 }} />
            ) : (
              <Volume2 size={16} style={{ verticalAlign: -3, marginRight: 6 }} />
            )}
            Som de alerta
          </strong>
          <span>Toca quando surge um alerta novo ou atualizado (offline, PON, SFP, temperatura, latência, etc.).</span>
        </div>
        <PrefSwitch
          label="Som de alerta"
          checked={prefs?.alert_sound_enabled !== false}
          disabled={!prefs || save.isPending}
          onChange={(v) => save.mutate({ alert_sound_enabled: v })}
        />
      </div>

      <div className="user-alert-prefs__sounds">
        <div className="user-alert-prefs__sounds-head">
          <h3 style={{ margin: 0, fontSize: 14 }}>Som a reproduzir</h3>
          <div className="row" style={{ gap: 8 }}>
            <input
              ref={fileRef}
              type="file"
              accept="audio/mpeg,.mp3"
              hidden
              onChange={(e) => void onUpload(e.target.files?.[0])}
            />
            <button type="button" className="btn" disabled={uploading} onClick={() => fileRef.current?.click()}>
              <Upload size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
              {uploading ? "A enviar…" : "Adicionar MP3"}
            </button>
          </div>
        </div>
        <p style={{ margin: "6px 0 10px", fontSize: 12, color: "var(--muted)" }}>
          Sons padrão do sistema ou um MP3 seu (até 2 MB, máx. 8).
        </p>
        <div className="user-alert-prefs__grid">
          {BUILTIN_ALERT_SOUNDS.map((s) => {
            const on = selected === s.id;
            return (
              <div key={s.id} className={`user-alert-prefs__sound${on ? " is-on" : ""}`}>
                <button type="button" className="user-alert-prefs__pick" onClick={() => save.mutate({ alert_sound_id: s.id })}>
                  {s.name}
                  <span>Padrão</span>
                </button>
                <button type="button" className="btn btn--icon-menu" title="Pré-ouvir" onClick={() => void playAlertSound(s.id)}>
                  <Play size={14} />
                </button>
              </div>
            );
          })}
          {custom.map((s) => {
            const on = selected === s.id;
            return (
              <div key={s.id} className={`user-alert-prefs__sound${on ? " is-on" : ""}`}>
                <button type="button" className="user-alert-prefs__pick" onClick={() => save.mutate({ alert_sound_id: s.id })}>
                  {s.name}
                  <span>MP3</span>
                </button>
                <button type="button" className="btn btn--icon-menu" title="Pré-ouvir" onClick={() => void playAlertSound(s.id)}>
                  <Play size={14} />
                </button>
                <button type="button" className="btn btn--icon-menu" title="Remover" onClick={() => void onDelete(s.id)}>
                  <Trash2 size={14} />
                </button>
              </div>
            );
          })}
        </div>
      </div>
      {prefs?.alert_toast_everywhere === false ? (
        <p className="user-alert-prefs__note">
          <BellOff size={14} style={{ verticalAlign: -2, marginRight: 4 }} />
          Toasts só em Monitoramento e Alertas. O som, se estiver ligado, continua a tocar em qualquer ecrã.
        </p>
      ) : null}
    </div>
  );
}
