import type { ClientCard } from "./types";

function scalarFromRaw(raw: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = raw?.[key];
  if (v == null) return undefined;
  const s = String(v).trim();
  return s || undefined;
}

/** ID do cliente para filtrar su_ticket (mesmo valor usado no teste manual: campo «id» do cliente IXC). */
function clientIdForAttendance(client: ClientCard): string | undefined {
  const raw = client.raw as Record<string, unknown> | undefined;
  return (
    scalarFromRaw(raw, "id") ??
    client.id?.trim() ??
    scalarFromRaw(raw, "id_cliente") ??
    client.code?.trim()
  );
}

/** Termo de busca para GET /integracao/cliente/atendimento a partir do cartão do cliente. */
export function attendanceBuscaForClient(
  client: ClientCard,
  fallback: { busca: string; termo: string },
): { busca: string; termo: string } {
  const id = clientIdForAttendance(client);
  if (id) return { busca: "codigo_cliente", termo: id };
  const doc = client.document?.trim();
  if (doc) return { busca: "cpf_cnpj", termo: doc };
  const svcId = client.services?.find((s) => s.id?.trim())?.id?.trim();
  if (svcId) return { busca: "id_cliente_servico", termo: svcId };
  return fallback;
}
