import { useMemo, useState } from "react";
import { Download, MapPinned, Search } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { downloadCsv } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { Callout, Pill, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Conferência dos 4 endereços do serviço (fiscal, cadastral, cobrança e instalação) — SOMENTE LEITURA. Lista os serviços em que
 * os quatro não são iguais. A API pública da HubSoft (documentação oficial, conferida) NÃO tem rota para editar nem
 * sincronizar endereço — só criar serviço/migrar plano com um endereço de instalação novo —, então a correção é feita na
 * HubSoft, com a função «sincronizar endereços» dela. O caso típico é «só a instalação é outra» (fiscal, cadastral e
 * cobrança iguais entre si): o CSV traz, para cada serviço, o endereço da instalação como endereço-alvo.
 */

const BASE = "/api/v1/integrations/hubsoft/hubsoft/address-check";

type Row = {
  id_cliente: string;
  codigo_cliente?: string;
  nome: string;
  id_cliente_servico: string;
  login: string;
  status?: string;
  n_servicos: number;
  padrao: "fiscal_diferente" | "instalacao_diferente" | "outra";
  diferentes: string[];
  fiscal: string;
  cadastral: string;
  cobranca: string;
  instalacao: string;
};
type Scan = { clientes_lidos: number; servicos_lidos: number; iguais: number; rows: Row[]; incompleto?: boolean; aviso?: string };

const PADRAO: Record<Row["padrao"], { text: string; tone: "ok" | "err" | "warn" }> = {
  fiscal_diferente: { text: "Fiscal diferente dos outros 3", tone: "err" },
  instalacao_diferente: { text: "Instalação diferente (os outros 3 são iguais entre si)", tone: "warn" },
  outra: { text: "Outra divergência", tone: "warn" },
};
const PADRAO_ORDER: Row["padrao"][] = ["instalacao_diferente", "fiscal_diferente", "outra"];

const TIPO_LABEL: Record<string, string> = { cadastral: "Cadastral", cobranca: "Cobrança", instalacao: "Instalação" };

export function HubsoftAddressCheck() {
  const [scanning, setScanning] = useState(false);
  const [scan, setScan] = useState<Scan | null>(null);
  const [err, setErr] = useState("");
  const [cancelados, setCancelados] = useState(false);
  const [onlyMulti, setOnlyMulti] = useState(false);
  // padrão mostrado: por omissão o caso pedido — instalação num endereço e os outros 3 em outro, iguais entre si
  const [padrao, setPadrao] = useState<"" | Row["padrao"]>("instalacao_diferente");

  async function run() {
    setScanning(true);
    setErr("");
    setScan(null);
    try {
      setScan(await apiFetch<Scan>(`${BASE}/scan${cancelados ? "?cancelados=1" : ""}`, { timeoutMs: 12 * 60_000 }));
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setScanning(false);
    }
  }

  const rows = useMemo(() => {
    let r = scan?.rows ?? [];
    if (onlyMulti) r = r.filter((x) => x.n_servicos >= 2);
    if (padrao) r = r.filter((x) => x.padrao === padrao);
    return r;
  }, [scan, onlyMulti, padrao]);
  const total = scan?.rows ?? [];
  const nFiscal = total.filter((r) => r.padrao === "fiscal_diferente").length;
  const countByPadrao = (p: Row["padrao"]) => total.filter((r) => r.padrao === p).length;

  function download() {
    downloadCsv(
      `enderecos-divergentes-${todayISO()}.csv`,
      ["id_cliente", "codigo_cliente", "cliente", "id_cliente_servico", "login", "status_servico", "servicos_do_cliente", "padrao", "tipos_que_diferem_do_fiscal", "endereco_alvo_(igual_a_instalacao)", "fiscal", "cadastral", "cobranca", "instalacao"],
      rows.map((r) => [
        r.id_cliente, r.codigo_cliente ?? "", r.nome, r.id_cliente_servico, r.login, r.status ?? "", String(r.n_servicos), PADRAO[r.padrao].text,
        r.diferentes.map((d) => TIPO_LABEL[d] ?? d).join(" | "), r.instalacao, r.fiscal, r.cadastral, r.cobranca, r.instalacao,
      ]),
    );
  }

  return (
    <ToolPanel
      icon={<MapPinned size={20} />}
      title="Conferir endereços dos serviços"
      badge="Somente leitura"
      badgeTone="ok"
      subtitle="Compara os 4 endereços de cada serviço (fiscal, cadastral, cobrança e instalação) e lista os que não são iguais. A API da HubSoft não tem rota para alterar nem sincronizar endereço — para igualar os 4 use a função «sincronizar endereços» da própria HubSoft; o CSV traz o endereço da instalação de cada serviço como alvo."
    >
      <Step n={1} title="Conferir a base" done={!!scan && !scanning} hint="Lê todos os clientes e serviços da HubSoft — leva cerca de 1 a 2 minutos.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={scanning} onClick={() => void run()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {scanning ? "Consultando a HubSoft…" : scan ? "Conferir de novo" : "Conferir endereços"}
          </button>
          <label className="hsa-check">
            <input type="checkbox" checked={cancelados} onChange={(e) => setCancelados(e.target.checked)} disabled={scanning} />
            Incluir serviços cancelados
          </label>
        </div>
        {err ? <Callout tone="err">{err}</Callout> : null}
        {scan?.incompleto && scan.aviso ? <Callout tone="warn">{scan.aviso}</Callout> : null}
        {scan ? (
          <div className="hsa-stats">
            <Stat label="Serviços verificados" value={scan.servicos_lidos} tone="muted" />
            <Stat label="Com os 4 endereços iguais" value={scan.iguais} tone="ok" />
            <Stat label="Com divergência" value={total.length} tone={total.length ? "warn" : "ok"} />
            <Stat label="Fiscal diferente dos outros 3" value={nFiscal} tone={nFiscal ? "err" : "ok"} />
          </div>
        ) : null}
        {scan && nFiscal === 0 ? <Callout tone="info">Nenhum serviço tem o endereço fiscal diferente dos outros três.</Callout> : null}
      </Step>

      {scan ? (
        <Step n={2} title="Divergências" done={false}>
          <div className="hsa-actions">
            <label className="hsa-check">
              <input type="checkbox" checked={onlyMulti} onChange={(e) => setOnlyMulti(e.target.checked)} />
              Só clientes com 2 ou mais serviços
            </label>
            <label className="hsa-check">
              Padrão
              <select className="input" style={{ marginLeft: 8 }} value={padrao} onChange={(e) => setPadrao(e.target.value as typeof padrao)}>
                <option value="">Todos ({total.length})</option>
                {PADRAO_ORDER.map((p) => (
                  <option key={p} value={p}>
                    {PADRAO[p].text} ({countByPadrao(p)})
                  </option>
                ))}
              </select>
            </label>
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={rows.length === 0} onClick={download}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar lista (CSV)
            </button>
          </div>
          {rows.length === 0 ? (
            <Callout tone="info">Nenhum serviço com esses filtros.</Callout>
          ) : (
            <div className="hsa-table-wrap" style={{ maxHeight: 520 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <th>Cliente</th>
                    <th>Serviço</th>
                    <th>Padrão</th>
                    <th>Endereços</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.id_cliente_servico}>
                      <td>
                        {r.nome}
                        <div className="hsa-muted">
                          id_cliente {r.id_cliente}
                          {r.codigo_cliente ? ` (código ${r.codigo_cliente})` : ""} · {r.n_servicos} serviço(s)
                        </div>
                      </td>
                      <td className="mono">{r.login || "—"}</td>
                      <td>
                        <Pill tone={PADRAO[r.padrao].tone}>{PADRAO[r.padrao].text}</Pill>
                        <div className="hsa-muted">Diferem do fiscal: {r.diferentes.map((d) => TIPO_LABEL[d] ?? d).join(", ")}</div>
                      </td>
                      <td style={{ fontSize: 11 }}>
                        <div><b>Fiscal:</b> {r.fiscal || "—"}</div>
                        <div><b>Cadastral:</b> {r.cadastral || "—"}</div>
                        <div><b>Cobrança:</b> {r.cobranca || "—"}</div>
                        <div><b>Instalação:</b> {r.instalacao || "—"}</div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
