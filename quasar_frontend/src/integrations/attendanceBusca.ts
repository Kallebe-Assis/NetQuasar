import type { ClientCard } from "./types";

/** Termo de busca para GET /integracao/cliente/atendimento a partir do cartão do cliente. */
export function attendanceBuscaForClient(
  client: ClientCard,
  fallback: { busca: string; termo: string },
): { busca: string; termo: string } {
  const code = client.code?.trim();
  if (code) return { busca: "codigo_cliente", termo: code };
  const doc = client.document?.trim();
  if (doc) return { busca: "cpf_cnpj", termo: doc };
  const svcId = client.services?.find((s) => s.id?.trim())?.id?.trim();
  if (svcId) return { busca: "id_cliente_servico", termo: svcId };
  return fallback;
}
