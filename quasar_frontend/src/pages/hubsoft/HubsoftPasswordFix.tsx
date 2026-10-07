import { useMemo, useState } from "react";
import { Download, KeyRound, Search, Wrench } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { downloadCsv } from "./hubsoftCsv";
import { useConsultaToast } from "./hubsoftConsulta";
import { Callout, ProgressBar, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Corrige senhas gravadas na HubSoft como texto de planilha — ="12345" — para só o conteúdo (12345).
 * 1) varre a base inteira (somente leitura); 2) corrige, em lotes, só o que ainda estiver nesse formato.
 */

const BASE = "/api/v1/integrations/hubsoft/hubsoft/password-fix";

type Row = {
  id_cliente: string;
  codigo_cliente?: string;
  nome: string;
  id_cliente_servico: string;
  login: string;
  status?: string;
  senha_atual: string;
  senha_correta: string;
  problema?: string;
};
type Scan = { clientes_lidos: number; servicos_lidos: number; rows: Row[] | null; incompleto?: boolean; aviso?: string };
type FixResult = { id_cliente_servico: string; login: string; ok: boolean; skipped: boolean; message: string };

export function HubsoftPasswordFix() {
  const { notify } = useConsultaToast();
  const [scanning, setScanning] = useState(false);
  const [scan, setScan] = useState<Scan | null>(null);
  const [err, setErr] = useState("");
  const [ack, setAck] = useState(false);
  const [fixing, setFixing] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<FixResult[]>([]);

  const rows = scan?.rows ?? [];
  const fixable = useMemo(() => rows.filter((r) => !r.problema), [rows]);
  const resultById = useMemo(() => new Map(results.map((r) => [r.id_cliente_servico, r])), [results]);
  const okCount = results.filter((r) => r.ok && !r.skipped).length;

  async function runScan() {
    setScanning(true);
    setErr("");
    setScan(null);
    setResults([]);
    setAck(false);
    try {
      const r = await apiFetch<Scan>(`${BASE}/scan`, { timeoutMs: 12 * 60_000 });
      setScan(r);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setScanning(false);
    }
  }

  async function runFix() {
    setFixing(true);
    setErr("");
    setResults([]);
    const acc: FixResult[] = [];
    setProgress({ done: 0, total: fixable.length });
    try {
      for (let i = 0; i < fixable.length; i += 25) {
        const batch = fixable.slice(i, i + 25).map((r) => ({ id_cliente_servico: r.id_cliente_servico }));
        const r = await apiFetch<{ results: FixResult[] }>(`${BASE}/apply`, { method: "POST", json: { rows: batch }, timeoutMs: 4 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + 25, fixable.length), total: fixable.length });
      }
    } catch (e) {
      setErr(`Interrompido por erro de comunicação: ${(e as Error).message}. Rode a consulta de novo para ver o que falta.`);
    } finally {
      setFixing(false);
    }
    notify({ ok: true }, null, `${acc.filter((r) => r.ok && !r.skipped).length} de ${acc.length} senha(s) corrigida(s).`);
  }

  function download() {
    downloadCsv(
      `senhas-formato-planilha-${new Date().toISOString().slice(0, 10)}.csv`,
      ["id_cliente", "codigo_cliente", "cliente", "id_cliente_servico", "login", "status", "senha_atual", "senha_correta", "problema", "resultado"],
      rows.map((r) => [r.id_cliente, r.codigo_cliente ?? "", r.nome, r.id_cliente_servico, r.login, r.status ?? "", r.senha_atual, r.senha_correta, r.problema ?? "", resultById.get(r.id_cliente_servico)?.message ?? ""]),
    );
  }

  return (
    <ToolPanel
      icon={<KeyRound size={20} />}
      title="Corrigir senhas no formato de planilha"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle={'Procura em toda a base senhas gravadas literalmente como ="12345" (com o "=" e as aspas, vindas de planilha) e troca pela senha só com o conteúdo (12345).'}
    >
      <Step n={1} title="Consultar a base" done={!!scan && !scanning} hint="Somente leitura: percorre todos os serviços não cancelados da HubSoft. Pode levar alguns minutos.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={scanning || fixing} onClick={() => void runScan()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {scanning ? "Consultando a HubSoft…" : scan ? "Consultar de novo" : "Consultar a base"}
          </button>
        </div>
        {err ? <Callout tone="err">{err}</Callout> : null}
        {scan?.incompleto && scan.aviso ? <Callout tone="warn">{scan.aviso}</Callout> : null}
        {scan ? (
          <div className="hsa-stats">
            <Stat label="Serviços verificados" value={scan.servicos_lidos} tone="muted" />
            <Stat label={'Com senha ="…"'} value={rows.length} tone={rows.length ? "warn" : "ok"} />
            <Stat label="Corrigíveis" value={fixable.length} tone={fixable.length ? "warn" : "muted"} />
          </div>
        ) : null}
        {scan && rows.length === 0 ? <Callout tone="info">Nenhuma senha nesse formato foi encontrada — nada a corrigir.</Callout> : null}
      </Step>

      {rows.length > 0 ? (
        <Step n={2} title="Corrigir" done={results.length > 0 && !fixing}>
          <div className="hsa-table-wrap" style={{ maxHeight: 420 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Cliente</th>
                  <th>Login</th>
                  <th>Senha atual</th>
                  <th>Senha correta</th>
                  <th>Situação</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => {
                  const res = resultById.get(r.id_cliente_servico);
                  return (
                    <tr key={r.id_cliente_servico}>
                      <td>
                        {r.nome}
                        <div className="hsa-muted">
                          id_cliente {r.id_cliente}
                          {r.codigo_cliente ? ` (código ${r.codigo_cliente})` : ""} · serviço {r.id_cliente_servico}
                        </div>
                      </td>
                      <td className="mono">{r.login}</td>
                      <td className="mono">{r.senha_atual}</td>
                      <td className="mono">{r.senha_correta}</td>
                      <td>
                        {res ? (
                          <span style={{ color: res.ok ? "var(--ok)" : "var(--err)" }}>{res.ok ? (res.skipped ? "Já estava certa" : "Corrigida") : "Erro"} — {res.message}</span>
                        ) : r.problema ? (
                          <span style={{ color: "var(--err)" }}>{r.problema}</span>
                        ) : (
                          <span className="hsa-muted">Pendente</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} disabled={fixing} />
            Entendo que isto altera a senha de {fixable.length} serviço(s) na HubSoft (só o formato ="…" é removido; senhas já corrigidas ou diferentes nunca são tocadas).
          </label>
          <div className="hsa-actions">
            <button type="button" className="btn btn--primary" disabled={!ack || fixing || fixable.length === 0 || (results.length > 0 && okCount === fixable.length)} onClick={() => void runFix()}>
              <Wrench size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              {fixing ? "Corrigindo…" : `Corrigir ${fixable.length}`}
            </button>
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={download}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar lista (CSV)
            </button>
          </div>
          {fixing ? <ProgressBar done={progress.done} total={progress.total} label={`Corrigindo… ${progress.done} de ${progress.total}`} /> : null}
          {results.length > 0 && !fixing ? (
            <Callout tone={okCount === fixable.length ? "info" : "warn"}>
              {okCount} de {fixable.length} corrigida(s){results.some((r) => !r.ok) ? `, ${results.filter((r) => !r.ok).length} com erro (veja a tabela)` : ""}. Rode “Consultar a base” de novo para confirmar que
              não sobrou nenhuma.
            </Callout>
          ) : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
