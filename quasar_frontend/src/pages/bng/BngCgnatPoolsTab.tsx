import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import { formatBngDateTime } from "../../lib/bngDisplay";

// Subconjunto de campos de BngInfrastructure/BngCGNATMapping (definidos em BngPage.tsx)
// realmente usado nesta aba — mantido local (não exportado de BngPage.tsx) para este
// componente ficar independente, no mesmo padrão de BngVlansTab.tsx.
type BngIPPoolRow = {
  index?: string;
  name?: string;
  total_ips?: number;
  used_ips?: number;
  idle_ips?: number;
  used_percent?: number;
  vrf?: string;
};

type BngIPv6PoolRow = {
  index?: string;
  name?: string;
  address_total?: number;
  address_used?: number;
  address_used_percent?: number;
  pd_prefix_used_percent?: number;
};

type BngCGN = {
  current_sessions?: string;
  license_total_m?: string;
  license_used_m?: string;
  license_free_m?: string;
  bit_throughput_up?: string;
  bit_throughput_down?: string;
};

type BngCGNPublicPool = {
  index?: string;
  instance?: string;
  pool_name?: string;
  start_addr?: string;
  end_addr?: string;
  usage_percent?: number;
};

type BngCGNATMapping = {
  private_ip?: string;
  public_hint?: string;
  pool_name?: string;
  cgnat?: boolean;
  session_count?: number;
};

type CgnatPoolsResponse = {
  captured_at?: string;
  infrastructure?: {
    collected_at?: string;
    ipv4_pools?: BngIPPoolRow[];
    ipv6_pools?: BngIPv6PoolRow[];
    cgn?: BngCGN;
    cgn_public_pools?: BngCGNPublicPool[];
  };
  infrastructure_captured_at?: string;
  infrastructure_note?: string;
  cgnat_summary?: BngCGNATMapping[];
};

type Props = {
  deviceId: string | null;
  active: boolean;
};

/**
 * Aba "CGNAT e Pools" — extraída da aba "Relatório" (BngInfrastructureReport/BngSessionReportPanel
 * em BngPage.tsx) para não pagar o custo de BuildCGNATSummary (loop completo sobre todas as
 * sessões PPPoE) sempre que o utilizador só quer ver totais/tempo online. Chama o mesmo
 * endpoint com ?cgnat=1 — só pedido quando esta aba está activa.
 */
export function BngCgnatPoolsTab({ deviceId, active }: Props) {
  const query = useQuery({
    queryKey: ["bng-cgnat-summary", deviceId],
    enabled: !!deviceId && active,
    placeholderData: keepPreviousData,
    queryFn: () => apiFetch<CgnatPoolsResponse>(`/api/v1/bng/devices/${deviceId}/sessions/report?cgnat=1`),
    refetchInterval: active ? 60_000 : false,
  });

  if (query.isLoading) return <p style={{ fontSize: 13, color: "var(--muted)" }}>A carregar pools e CGNAT…</p>;

  const infra = query.data?.infrastructure;
  const cgnatSummary = query.data?.cgnat_summary ?? [];
  const poolTotals = (infra?.ipv4_pools ?? []).reduce(
    (acc, p) => ({
      total: acc.total + (p.total_ips ?? 0),
      used: acc.used + (p.used_ips ?? 0),
      idle: acc.idle + (p.idle_ips ?? 0),
    }),
    { total: 0, used: 0, idle: 0 },
  );

  if (!infra) {
    return (
      <div className="msg msg--warn" style={{ marginBottom: 16 }}>
        {query.data?.infrastructure_note ||
          "Execute a coleta completa SNMP ou «Coletar totais agora» para obter pools e CGNAT."}
      </div>
    );
  }

  return (
    <>
      <h3 style={{ fontSize: 14, margin: "0 0 10px", color: "var(--muted)", fontWeight: 600 }}>
        Pools e CGNAT
        {query.data?.infrastructure_captured_at ? ` · ${formatBngDateTime(query.data.infrastructure_captured_at)}` : ""}
      </h3>

      {poolTotals.total > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Pools IPv4 (totais)</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 12 }}>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>IPs totais</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{poolTotals.total.toLocaleString("pt-PT")}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Em uso</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{poolTotals.used.toLocaleString("pt-PT")}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Ociosos</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{poolTotals.idle.toLocaleString("pt-PT")}</div>
            </div>
          </div>
          {(infra.ipv4_pools ?? []).length > 0 && (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Pool</th>
                    <th>VRF</th>
                    <th>Total</th>
                    <th>Usados</th>
                    <th>Ociosos</th>
                    <th>% uso</th>
                  </tr>
                </thead>
                <tbody>
                  {infra.ipv4_pools!.map((p) => (
                    <tr key={`${p.index}-${p.name}`}>
                      <td className="mono">{p.name}</td>
                      <td className="mono">{p.vrf || "—"}</td>
                      <td>{p.total_ips?.toLocaleString("pt-PT") ?? "—"}</td>
                      <td>{p.used_ips?.toLocaleString("pt-PT") ?? "—"}</td>
                      <td>{p.idle_ips?.toLocaleString("pt-PT") ?? "—"}</td>
                      <td>{p.used_percent != null ? `${p.used_percent}%` : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {(infra.ipv6_pools ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Pools IPv6</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Pool</th>
                  <th>Endereços %</th>
                  <th>PD %</th>
                  <th>Usados / total</th>
                </tr>
              </thead>
              <tbody>
                {infra.ipv6_pools!.map((p) => (
                  <tr key={`${p.index}-${p.name}`}>
                    <td className="mono">{p.name}</td>
                    <td>{p.address_used_percent != null ? `${p.address_used_percent}%` : "—"}</td>
                    <td>{p.pd_prefix_used_percent != null ? `${p.pd_prefix_used_percent}%` : "—"}</td>
                    <td>
                      {p.address_used ?? "—"} / {p.address_total ?? "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {infra.cgn && Object.values(infra.cgn).some(Boolean) && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>CGN / NAT</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
            {[
              { label: "Sessões actuais", value: infra.cgn.current_sessions },
              { label: "Licença total (M)", value: infra.cgn.license_total_m },
              { label: "Licença usada (M)", value: infra.cgn.license_used_m },
              { label: "Licença livre (M)", value: infra.cgn.license_free_m },
              { label: "Throughput up (bits)", value: infra.cgn.bit_throughput_up },
              { label: "Throughput down (bits)", value: infra.cgn.bit_throughput_down },
            ].map((c) => (
              <div key={c.label}>
                <div style={{ fontSize: 11, color: "var(--muted)" }}>{c.label}</div>
                <div style={{ fontSize: 15, fontWeight: 600 }}>{c.value ?? "—"}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {(infra.cgn_public_pools ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Pools públicos CGNAT</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Pool</th>
                  <th>Instância</th>
                  <th>Range público</th>
                  <th>% uso</th>
                </tr>
              </thead>
              <tbody>
                {infra.cgn_public_pools!.map((p) => (
                  <tr key={p.index ?? `${p.pool_name}-${p.start_addr}`}>
                    <td className="mono">{p.pool_name || "—"}</td>
                    <td>{p.instance || "—"}</td>
                    <td className="mono">
                      {p.start_addr && p.end_addr && p.start_addr !== p.end_addr
                        ? `${p.start_addr} – ${p.end_addr}`
                        : p.start_addr || p.end_addr || "—"}
                    </td>
                    <td>{p.usage_percent != null ? `${p.usage_percent}%` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {cgnatSummary.length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 6px", fontSize: 15 }}>CGNAT — IP privado × pool público</h3>
          <p style={{ margin: "0 0 10px", fontSize: 12, color: "var(--muted)", lineHeight: 1.45 }}>
            O IP por sessão em hwAccessIPAddress é o endereço WAN do cliente (privado em CGNAT). O mapeamento exacto
            privado→público por sessão não existe na HUAWEI-AAA-MIB; abaixo são listados os pools públicos CGNAT
            (HUAWEI-CGN-MIB) associados a cada faixa privada.
          </p>
          <div className="table-wrap" style={{ maxHeight: 360, overflow: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>IP privado / WAN</th>
                  <th>Pool / IP público</th>
                  <th>Pool</th>
                  <th>Sessões</th>
                </tr>
              </thead>
              <tbody>
                {cgnatSummary.map((row) => (
                  <tr key={row.private_ip}>
                    <td className="mono">{row.private_ip || "—"}</td>
                    <td className="mono">{row.public_hint || "—"}</td>
                    <td>{row.pool_name || (row.cgnat ? "CGNAT" : "—")}</td>
                    <td>{row.session_count?.toLocaleString("pt-PT") ?? "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </>
  );
}
