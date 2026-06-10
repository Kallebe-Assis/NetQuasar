import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { queryKeys } from "../lib/queryKeys";

const COLLECT = "Relatório ONU mensal: a recolher dados OLT";
const TELEGRAM = "Relatório ONU mensal: a enviar Telegram";

/** Notificações do relatório ONU mensal em segundo plano (pilha global). */
export function OnuReportGlobalToast() {
  const { push } = useAppToast();
  const mon = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<{ current_activity?: string | null }>("/api/v1/monitoring/state"),
    staleTime: 1000,
    refetchInterval: false,
  });
  const activity = (mon.data?.current_activity ?? "").trim();
  const prev = useRef("");
  const toastIds = useRef<string[]>([]);

  useEffect(() => {
    if (!activity) {
      if (prev.current === TELEGRAM) {
        const id = push({ tone: "ok", text: "Relatório ONU enviado para o Telegram." });
        toastIds.current.push(id);
      }
      prev.current = "";
      return;
    }
    if (activity === prev.current) return;
    if (activity === COLLECT) {
      const id = push({ tone: "info", text: "A recolher dados para o relatório ONU mensal…" });
      toastIds.current.push(id);
    } else if (activity === TELEGRAM) {
      const id = push({ tone: "info", text: "A enviar relatório ONU para o Telegram…" });
      toastIds.current.push(id);
    }
    prev.current = activity;
  }, [activity, push]);

  return null;
}
