import { createPortal } from "react-dom";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { errorMessageFromUnknown } from "../lib/apiErrors";
import { Switch } from "./Switch";

type OnuTarget = {
  serial?: string;
  olt_id: string;
  olt_description?: string | null;
  pon?: number;
  onu?: number;
  client_name?: string | null;
};

type ExistingMonitor = {
  serial: string;
  client_name: string;
  watch_status: boolean;
  watch_rx: boolean;
  watch_login: boolean;
  bng_login: string;
  notify_telegram: boolean;
  expires_at?: string | null;
};

type Props = {
  target: OnuTarget | null;
  onClose: () => void;
  onSaved: () => void;
};

/** Configura o monitoramento manual/temporário de uma ONU (tela OLT → ONUs → "Monitorar"). */
export function OnuMonitorModal({ target, onClose, onSaved }: Props) {
  const qc = useQueryClient();
  const serial = (target?.serial ?? "").trim().toUpperCase();

  const existingQ = useQuery({
    queryKey: ["onu-monitors"],
    queryFn: () => apiFetch<{ monitors: ExistingMonitor[] }>("/api/v1/olt/onu-monitors"),
    enabled: !!target,
    staleTime: 10_000,
  });
  const existing = existingQ.data?.monitors.find((m) => m.serial.toUpperCase() === serial);

  const [clientName, setClientName] = useState("");
  const [watchStatus, setWatchStatus] = useState(true);
  const [watchRx, setWatchRx] = useState(true);
  const [watchLogin, setWatchLogin] = useState(false);
  const [bngLogin, setBngLogin] = useState("");
  const [notifyTelegram, setNotifyTelegram] = useState(true);
  const [mode, setMode] = useState<"manual" | "temp">("manual");
  const [hours, setHours] = useState("24");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!target) return;
    setErr(null);
    if (existing) {
      setClientName(existing.client_name || target.client_name || "");
      setWatchStatus(existing.watch_status);
      setWatchRx(existing.watch_rx);
      setWatchLogin(existing.watch_login);
      setBngLogin(existing.bng_login || "");
      setNotifyTelegram(existing.notify_telegram);
      setMode(existing.expires_at ? "temp" : "manual");
    } else {
      setClientName(target.client_name || "");
      setWatchStatus(true);
      setWatchRx(true);
      setWatchLogin(false);
      setBngLogin("");
      setNotifyTelegram(true);
      setMode("manual");
      setHours("24");
    }
  }, [target, existing]);

  const save = useMutation({
    mutationFn: () => {
      if (!target) throw new Error("Sem ONU.");
      if (!serial) throw new Error("Serial da ONU não disponível.");
      if (!watchStatus && !watchRx && !watchLogin) {
        throw new Error("Selecione ao menos um monitoramento (status, RX ou login).");
      }
      if (watchLogin && !bngLogin.trim()) {
        throw new Error("Informe o login PPPoE para confirmar no BNG.");
      }
      const duration_hours = mode === "temp" ? Number(hours) || 0 : 0;
      return apiFetch("/api/v1/olt/onu-monitors", {
        method: "POST",
        json: {
          serial,
          olt_device_id: target.olt_id,
          olt_description: target.olt_description ?? "",
          pon: target.pon ?? null,
          onu: target.onu ?? null,
          client_name: clientName.trim(),
          watch_status: watchStatus,
          watch_rx: watchRx,
          watch_login: watchLogin,
          bng_login: bngLogin.trim(),
          notify_telegram: notifyTelegram,
          duration_hours,
        },
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["onu-monitors"] });
      onSaved();
      onClose();
    },
    onError: (e) => setErr(errorMessageFromUnknown(e)),
  });

  if (!target) return null;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !save.isPending && onClose()}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="onu-monitor-title"
        onMouseDown={(e) => e.stopPropagation()}
        style={{ maxWidth: 480 }}
      >
        <h3 id="onu-monitor-title" style={{ marginTop: 0 }}>
          {existing ? "Editar monitoramento da ONU" : "Monitorar ONU"}
        </h3>
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
          <span className="mono">{serial || "—"}</span> · PON {target.pon ?? "?"} / ONU {target.onu ?? "?"} ·{" "}
          {target.olt_description || "OLT"}. Os alertas aparecem na aba <strong>ONUs</strong> da tela de Alertas e
          alarmam quando o item degrada e quando normaliza.
        </p>

        <label className="splitter-modal__field" style={{ marginBottom: 12 }}>
          <span>Descrição (nome do cliente)</span>
          <input
            className="input"
            autoFocus
            placeholder="Ex.: João da Silva"
            value={clientName}
            onChange={(e) => setClientName(e.target.value)}
          />
        </label>

        <div style={{ display: "flex", flexDirection: "column", gap: 10, marginBottom: 12 }}>
          <Switch
            checked={watchStatus}
            onChange={setWatchStatus}
            label="Status online / offline"
            hint="Alarma quando cai e quando volta a ficar online."
          />
          <Switch
            checked={watchRx}
            onChange={setWatchRx}
            label="Potência óptica RX"
            hint="Alarma quando fica ruim e quando normaliza (faixas de Configurações → OLT)."
          />
          <Switch
            checked={watchLogin}
            onChange={setWatchLogin}
            label="Confirmar login no BNG"
            hint="Ao normalizar, consulta o login no BNG e confirma se está online."
          />
          {watchLogin ? (
            <label className="splitter-modal__field" style={{ marginLeft: 52 }}>
              <span>Login PPPoE</span>
              <input
                className="input"
                placeholder="login do cliente no BNG"
                value={bngLogin}
                onChange={(e) => setBngLogin(e.target.value)}
              />
            </label>
          ) : null}
          <Switch
            checked={notifyTelegram}
            onChange={setNotifyTelegram}
            label="Enviar no Telegram"
            hint="Notifica no canal de alertas na degradação e na normalização."
          />
        </div>

        <div className="splitter-modal__field" style={{ marginBottom: 12 }}>
          <span>Duração</span>
          <div style={{ display: "flex", gap: 14, alignItems: "center", flexWrap: "wrap", marginTop: 4 }}>
            <label style={{ display: "inline-flex", gap: 6, alignItems: "center", fontSize: 13 }}>
              <input type="radio" name="onu-monitor-mode" checked={mode === "manual"} onChange={() => setMode("manual")} />
              Até eu remover
            </label>
            <label style={{ display: "inline-flex", gap: 6, alignItems: "center", fontSize: 13 }}>
              <input type="radio" name="onu-monitor-mode" checked={mode === "temp"} onChange={() => setMode("temp")} />
              Temporário
            </label>
            {mode === "temp" ? (
              <span style={{ display: "inline-flex", gap: 6, alignItems: "center", fontSize: 13 }}>
                <input
                  className="input mono"
                  inputMode="numeric"
                  style={{ width: 70 }}
                  value={hours}
                  onChange={(e) => setHours(e.target.value.replace(/[^\d]/g, ""))}
                />
                horas
              </span>
            ) : null}
          </div>
        </div>

        {err ? <div className="msg msg--err" style={{ marginBottom: 10 }}>{err}</div> : null}

        <div className="row" style={{ justifyContent: "flex-end", gap: 8 }}>
          <button type="button" className="btn" disabled={save.isPending} onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "A guardar…" : existing ? "Guardar alterações" : "Iniciar monitoramento"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
