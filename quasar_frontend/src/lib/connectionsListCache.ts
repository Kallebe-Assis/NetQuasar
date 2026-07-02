type ConnRow = {
  login: string;
  client_name: string;
  connection_kind: string;
  display_number: number;
  ip_address?: string | null;
  cto?: string | null;
  address?: string | null;
  neighborhood?: string | null;
  onu_mac_sn?: string | null;
  transmitter?: string | null;
};

/** Filtra lista completa em memória (evita refetch por tecla na pesquisa). */
export function filterClientConnectionsList<T extends ConnRow>(
  connections: T[],
  connectionKind: string,
  query: string,
): T[] {
  let rows = connections;
  const kind = connectionKind.trim();
  if (kind) {
    rows = rows.filter((c) => c.connection_kind === kind);
  }
  const q = query.trim().toLowerCase();
  if (!q) return rows;
  return rows.filter((c) => {
    const hay = [
      c.login,
      c.client_name,
      c.ip_address,
      c.cto,
      c.address,
      c.neighborhood,
      c.onu_mac_sn,
      c.transmitter,
      String(c.display_number),
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return hay.includes(q);
  });
}
