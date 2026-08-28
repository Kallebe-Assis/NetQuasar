import { EM_DASH } from "./bgpFormat";
import { formatKilobytes } from "../../lib/formatBytes";
import type { CpuCoreReport, VSInfo, VSResourceReport } from "./bgpTypes";

type Props = {
  cpuCores: CpuCoreReport[];
  vsList: VSInfo[];
  vsResources: VSResourceReport[];
};

function pct(v?: string): string {
  if (v === undefined || v === "") return EM_DASH;
  const n = Number(v);
  return Number.isFinite(n) ? `${n}%` : v;
}

// vs_res_mem_used/vs_res_mem_total (hwVSPhysicalResTable) vêm em KB, confirmado ao vivo — sem
// isto apareciam números crus tipo "1158604".
function mem(v?: string): string {
  if (v === undefined || v === "") return EM_DASH;
  const n = Number(v);
  return Number.isFinite(n) ? formatKilobytes(n) : v;
}

/** Aba "CPU & Memória" — CPU por núcleo (HUAWEI-CPU-MIB hwMultiCpuDevTable, não só o agregado
 * já mostrado na Visão Geral) com média 1min/5min, e CPU/memória do próprio Virtual System de
 * BGP (HUAWEI-VS-MIB) — a VS pode estar sobrecarregada mesmo com o chassi inteiro saudável. */
export function BgpCpuTab({ cpuCores, vsList, vsResources }: Props) {
  return (
    <>
      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>CPU por núcleo</h3>
        {cpuCores.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem dados por núcleo (métricas da secção "cpu_nucleos" podem estar desligadas em Configurações → BGP).
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Núcleo</th>
                  <th>Actual</th>
                  <th>Média 1 min</th>
                  <th>Média 5 min</th>
                </tr>
              </thead>
              <tbody>
                {cpuCores.map((c) => (
                  <tr key={c.core_index}>
                    <td className="mono">{c.core_index}</td>
                    <td className="mono">{pct(c.duty)}</td>
                    <td className="mono">{pct(c.avg_duty_1min)}</td>
                    <td className="mono">{pct(c.avg_duty_5min)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 14 }}>
        <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Virtual System (VS)</h3>
        <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--muted)" }}>
          CPU/memória do próprio Virtual System de BGP (HUAWEI-VS-MIB) — separado da saúde geral do chassi.
        </p>
        {vsList.length === 0 && vsResources.length === 0 ? (
          <div className="msg" style={{ fontSize: 12 }}>
            Sem dados de VS (métricas da secção "vs" podem estar desligadas em Configurações → BGP).
          </div>
        ) : (
          <>
            {vsList.length > 0 && (
              <div className="table-wrap" style={{ marginBottom: vsResources.length > 0 ? 14 : 0 }}>
                <table>
                  <thead>
                    <tr>
                      <th>VS ID</th>
                      <th>Nome</th>
                      <th>Estado</th>
                    </tr>
                  </thead>
                  <tbody>
                    {vsList.map((v) => (
                      <tr key={v.vs_id}>
                        <td className="mono">{v.vs_id}</td>
                        <td>{v.name || EM_DASH}</td>
                        <td className="mono">{v.status ?? EM_DASH}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {vsResources.length > 0 && (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Slot</th>
                      <th>CPU</th>
                      <th>Memória usada</th>
                      <th>Memória total</th>
                    </tr>
                  </thead>
                  <tbody>
                    {vsResources.map((r) => (
                      <tr key={r.slot}>
                        <td className="mono">{r.slot}</td>
                        <td className="mono">{pct(r.cpu)}</td>
                        <td className="mono">{mem(r.mem_used)}</td>
                        <td className="mono">{mem(r.mem_total)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>
    </>
  );
}
