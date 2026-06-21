import { ApiError } from "./api";

/** Mensagens legíveis para o utilizador (evita texto técnico cru de rede/SNMP). */
export function friendlyApiMessage(raw: unknown): string {
  const t = String(raw ?? "").trim();
  if (!t) return "Ocorreu um erro inesperado.";
  const lower = t.toLowerCase();
  if (lower.includes("context deadline exceeded") || lower.includes("deadline exceeded")) {
    return "A requisição demorou demasiado ou o equipamento não respondeu a tempo. Tente novamente.";
  }
  if (lower.includes("connection reset") || lower.includes("econnreset")) {
    return "A ligação foi interrompida. Verifique a rede e tente novamente.";
  }
  if (lower.includes("etimedout") || lower.includes("timeout") || lower.includes("i/o timeout")) {
    return "Tempo de espera esgotado. Tente novamente.";
  }
  if (
    lower.includes("failed to fetch") ||
    lower.includes("networkerror when attempting to fetch") ||
    lower.includes("load failed")
  ) {
    return "Sem ligação ao servidor. Verifique a API ou a rede.";
  }
  if (lower.includes("snmp_timeout") || lower.includes("tempo limite da coleta snmp")) {
    return "Tempo limite da coleta SNMP excedido. Verifique conectividade e comunidade SNMP.";
  }
  if (lower.includes("no_internet") || lower.includes("no internet")) {
    return "Sem acesso à internet conforme a verificação no servidor.";
  }
  if (lower.includes("not_found") || lower.includes("não encontrado") || lower.includes("nao encontrado")) {
    return t.length < 120 ? t : "Recurso não encontrado.";
  }
  if (lower.includes("internal server error")) {
    return "Erro interno no servidor. Se persistir, verifique os logs da API.";
  }
  if (t.length > 220) return `${t.slice(0, 217)}…`;
  return t;
}

export function errorMessageFromUnknown(e: unknown): string {
  if (e instanceof ApiError) return friendlyApiMessage(e.message);
  if (e instanceof Error) return friendlyApiMessage(e.message);
  return friendlyApiMessage(String(e));
}

export type ParsedApiError = {
  title: string;
  message: string;
  code?: string;
  status?: number;
};

export function parseApiErrorForModal(e: unknown, title = "Erro"): ParsedApiError {
  if (e instanceof ApiError) {
    const body = e.body as { error?: string; code?: string; message?: string } | undefined;
    const rawMsg = body?.error ?? body?.message ?? e.message;
    return {
      title: e.status >= 500 ? "Erro no servidor" : title,
      message: friendlyApiMessage(rawMsg || e.message),
      code: e.code ?? body?.code,
      status: e.status,
    };
  }
  return {
    title,
    message: errorMessageFromUnknown(e),
  };
}
