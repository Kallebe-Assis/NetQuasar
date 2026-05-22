/** Formatação de campos na consulta de integrações (IXC / Hubsoft). */

const IXC_ATTENDANCE_STATUS: Record<string, string> = {
  N: "Novo",
  P: "Pendente",
  EP: "Em progresso",
  S: "Solucionado",
  C: "Cancelado",
};

const IXC_WORK_ORDER_STATUS: Record<string, string> = {
  A: "Aberta",
  AN: "Análise",
  EN: "Encaminhada",
  AS: "Assumida",
  AG: "Agendada",
  DS: "Deslocamento",
  EX: "Execução",
  F: "Finalizada",
  RAG: "Aguardando agendamento",
};

export function formatAttendanceStatus(item: { status?: string; status_label?: string }): string {
  if (item.status_label?.trim()) return item.status_label.trim();
  const code = (item.status ?? "").trim();
  if (!code) return "";
  return IXC_ATTENDANCE_STATUS[code.toUpperCase()] ?? code;
}

export function formatWorkOrderStatus(item: { status?: string; status_label?: string }): string {
  if (item.status_label?.trim()) return item.status_label.trim();
  const code = (item.status ?? "").trim();
  if (!code) return "";
  return IXC_WORK_ORDER_STATUS[code.toUpperCase()] ?? code;
}

/** Converte "2025-07-15 08:34:21" (e variantes ISO) para exibição local. */
const IXC_ONLINE: Record<string, string> = {
  S: "Online",
  N: "Offline",
  SS: "Sem status",
};

/** status_internet do contrato IXC (liberação do contrato, não conectividade). */
const IXC_CONTRACT_STATUS: Record<string, string> = {
  A: "Ativo",
  D: "Desativado",
  CM: "Bloqueio Manual",
  CA: "Bloqueio Automático",
  FA: "Financeiro em atraso",
  AA: "Aguardando Assinatura",
};

export function formatIXCOnline(code?: string | null, label?: string | null): string {
  if (label?.trim()) return label.trim();
  const c = (code ?? "").trim().toUpperCase();
  if (!c) return "";
  return IXC_ONLINE[c] ?? (code ?? "").trim();
}

export function formatIXCContractStatus(code?: string | null, label?: string | null): string {
  if (label?.trim()) return label.trim();
  const c = (code ?? "").trim().toUpperCase();
  if (!c) return "";
  return IXC_CONTRACT_STATUS[c] ?? "";
}

/** @deprecated use formatIXCContractStatus */
export function formatIXCStatusInternet(code?: string | null, label?: string | null): string {
  return formatIXCContractStatus(code, label);
}

export function formatIntegrationDateTime(value?: string | null): string {
  const s = (value ?? "").trim();
  if (!s) return "";
  const normalized = s.includes("T") ? s : s.replace(" ", "T");
  const d = new Date(normalized);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
