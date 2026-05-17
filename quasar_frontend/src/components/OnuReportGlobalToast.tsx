import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { queryKeys } from "../lib/queryKeys";

const COLLECT = "Relatório ONU mensal: a recolher dados OLT";
const TELEGRAM = "Relatório ONU mensal: a enviar Telegram";

/** Toast global quando o relatório ONU mensal corre em segundo plano (agendador ou API). */
export function OnuReportGlobalToast() {
  const mon = useQuery({
    queryKey: queryKeys.monStateGlobal,
    queryFn: () => apiFetch<{ current_activity?: string | null }>("/api/v1/monitoring/state"),
    refetchInterval: 1500,
  });
  const activity = (mon.data?.current_activity ?? "").trim();
  const prev = useRef("");
  const [toast, setToast] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (!activity) {
      if (prev.current === TELEGRAM) {
        setToast({ ok: true, text: "Relatório ONU enviado para o Telegram." });
      }
      prev.current = "";
      return;
    }
    if (activity === prev.current) return;
    if (activity === COLLECT) {
      setToast({ ok: true, text: "A recolher dados para o relatório ONU mensal…" });
    } else if (activity === TELEGRAM) {
      setToast({ ok: true, text: "Dados recolhidos. A enviar relatório ONU para o Telegram…" });
    }
    prev.current = activity;
  }, [activity]);

  useEffect(() => {
    if (!toast) return;
    const t = window.setTimeout(() => setToast(null), 7000);
    return () => window.clearTimeout(t);
  }, [toast]);

  if (!toast) return null;
  return (
    <div
      className={`page-toast ${toast.ok ? "page-toast--ok" : "page-toast--err"}`}
      role="status"
      style={{ position: "fixed", bottom: 24, right: 24, zIndex: 9000, maxWidth: 420 }}
    >
      <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setToast(null)}>
        ×
      </button>
      {toast.text}
    </div>
  );
}
