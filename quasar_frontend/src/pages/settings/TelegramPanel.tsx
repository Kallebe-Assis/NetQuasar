import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { apiFetch } from "../../lib/api";

function TelegramTestOutcome({ data, error }: { data: unknown; error: Error | null }) {
  if (error) {
    return (
      <div className="msg msg--err" style={{ marginTop: 10 }}>
        {error.message}
      </div>
    );
  }
  if (data === undefined) return null;
  if (data !== null && typeof data === "object") {
    const d = data as Record<string, unknown>;
    if (d.ok === true && d.sent === true) {
      return (
        <div className="msg msg--ok" style={{ marginTop: 10 }}>
          Mensagem de teste enviada com sucesso. Verifique o Telegram.
        </div>
      );
    }
    if (d.ok === false && typeof d.message === "string" && d.message.trim()) {
      return (
        <div className="msg msg--err" style={{ marginTop: 10 }}>
          {d.message}
        </div>
      );
    }
    if (d.ok === true) {
      return (
        <div className="msg msg--ok" style={{ marginTop: 10 }}>
          Requisição de teste concluído.
        </div>
      );
    }
  }
  return (
    <details style={{ marginTop: 10, fontSize: 12 }}>
      <summary style={{ cursor: "pointer", color: "var(--muted)" }}>Detalhe da resposta</summary>
      <pre className="mono" style={{ marginTop: 6, padding: 8, background: "var(--panel2)", borderRadius: 6, fontSize: 11, overflow: "auto" }}>
        {JSON.stringify(data, null, 2)}
      </pre>
    </details>
  );
}

const TELEGRAM_MONITORING_TEST_TEMPLATES: { id: string; label: string }[] = [
  { id: "default", label: "Padrão" },
  { id: "ping_unreachable", label: "Equipamento offline" },
  { id: "latency_high", label: "Latência alta" },
  { id: "uptime_restart_low", label: "Uptime / reinício" },
  { id: "sfp_rx", label: "SFP RX" },
  { id: "sfp_tx", label: "SFP TX" },
	{ id: "pon_off", label: "PON OFF / PON DOWN" },
  { id: "interface_down", label: "Interface DOWN" },
  { id: "telemetry_threshold", label: "Telemetria (limiar)" },
  { id: "snmp_failure", label: "Falha SNMP" },
];

const TELEGRAM_REPORTS_TEST_TEMPLATES: { id: string; label: string }[] = [
  { id: "default", label: "Padrão" },
  { id: "alerts_digest", label: "Resumo de alertas" },
  { id: "onu_monthly", label: "Relatório mensal ONU" },
];

export function TelegramPanel({ id, title }: { id: string; title: string }) {
  const qc = useQueryClient();
  const path = id === "monitoring" ? "monitoring" : "reports";
  const templates = id === "monitoring" ? TELEGRAM_MONITORING_TEST_TEMPLATES : TELEGRAM_REPORTS_TEST_TEMPLATES;
  const q = useQuery({
    queryKey: ["settings-tg", id],
    queryFn: () =>
      apiFetch<{ id: string; bot_token: unknown; chat_id: string | null; topic_id: string | null }>(
        `/api/v1/settings/notifications/telegram/${path}`,
      ),
  });
  const [token, setToken] = useState("");
  const [chat, setChat] = useState("");
  const [topic, setTopic] = useState("");
  const [testTemplate, setTestTemplate] = useState("default");
  const { push: pushToast } = useAppToast();

  useEffect(() => {
    if (!q.data) return;
    setChat(q.data.chat_id ?? "");
    setTopic(q.data.topic_id ?? "");
  }, [q.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/settings/notifications/telegram/${path}`, {
        method: "PATCH",
        json: { bot_token: token || undefined, chat_id: chat || undefined, topic_id: topic || undefined },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-tg", id] });
      toastOk(pushToast, "Guardado com sucesso (Telegram).");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar (Telegram)."),
  });

  const test = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/settings/notifications/telegram/${path}/test`, {
        method: "POST",
        json: { template: testTemplate },
      }),
  });

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Telegram — {title}</h2>
      <p style={{ fontSize: 12, color: "var(--muted)" }}>
        Para alterar o bot, introduza um novo token abaixo. O valor já salvo não é mostrado por segurança.
      </p>
      <div className="field">
        <label>Token do bot (novo)</label>
        <input className="input mono" type="password" value={token} onChange={(e) => setToken(e.target.value)} />
      </div>
      <div className="row" style={{ gap: 8 }}>
        <input className="input" placeholder="ID do chat" value={chat} onChange={(e) => setChat(e.target.value)} />
        <input className="input" placeholder="ID do tópico (opcional)" value={topic} onChange={(e) => setTopic(e.target.value)} />
      </div>
      <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
        <label className="field" style={{ margin: 0, flex: "1 1 200px", minWidth: 180 }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Tipo de mensagem de teste</span>
          <select className="select" style={{ width: "100%" }} value={testTemplate} onChange={(e) => setTestTemplate(e.target.value)}>
            {templates.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </select>
        </label>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          Salvar
        </button>
        <button type="button" className="btn" disabled={test.isPending} onClick={() => test.mutate()}>
          Enviar mensagem de teste
        </button>
      </div>
      <TelegramTestOutcome data={test.data} error={test.error as Error | null} />
    </div>
  );
}
