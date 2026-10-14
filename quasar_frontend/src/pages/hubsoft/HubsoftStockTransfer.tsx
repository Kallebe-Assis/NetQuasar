import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowRightLeft, Download, ListChecks, Square } from "lucide-react";
import { Callout, CsvDropzone, Pill, ProgressBar, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { norm } from "./hubsoftStockFields";

const BASE = "/api/v1/integrations/hubsoft/hubsoft/stock-transfer";
const CATALOG = "/api/v1/integrations/hubsoft/hubsoft/catalog/fetch";
const PREVIEW_BATCH = 40;
const APPLY_BATCH = 20;
const MAX_ROWS = 10000;

/**
 * Transferência de patrimônios entre LOCAIS de estoque. A HubSoft faz isso por um movimento «Transferência entre estoques»
 * (origem → destino, com os patrimônios). Só patrimônio com status ESTOQUE é transferido; a API não cria local novo, então o
 * destino precisa já existir. Fluxo: CSV (coluna id_produto_item) + destino → conferir (somente leitura) → confirmar → enviar
 * em lotes pequenos → cada patrimônio é relido (local = destino e status ainda ESTOQUE).
 */

type Item = { id_produto_item: string; codigo_item?: string; identificador_proprio?: string; produto?: string; id_produto?: string; id_local_estoque?: string; local?: string; status?: string };
type Prev = { linha: number; ok: boolean; already_done?: boolean; problems?: string[]; item?: Item };
type Res = {
  linha: number;
  ok: boolean;
  action: "transferred" | "already_done" | "blocked" | "failed" | "verify_failed" | "skipped";
  message: string;
  id_movimento_estoque?: string;
  item?: Item;
  local_origem?: string;
  local_depois?: string;
  status_depois?: string;
  detail?: string;
};
type LocalOpt = { id_local_estoque: number; descricao: string };
type Row = { linha: number; id_produto_item: string; rotulo: string };

const ACTION_LABEL: Record<Res["action"], string> = {
  transferred: "Transferido",
  already_done: "Já estava no destino — nada enviado",
  blocked: "Bloqueado antes de enviar",
  failed: "A HubSoft recusou",
  verify_failed: "Enviado, mas a conferência divergiu",
  skipped: "Não enviado (falhas anteriores)",
};
const ACTION_TONE: Record<Res["action"], "ok" | "err" | "warn" | undefined> = { transferred: "ok", already_done: undefined, blocked: "warn", failed: "err", verify_failed: "err", skipped: "warn" };

export function HubsoftStockTransfer() {
  const { notify, missing } = useConsultaToast();
  const [locais, setLocais] = useState<LocalOpt[]>([]);
  const [locaisErr, setLocaisErr] = useState("");
  const [destino, setDestino] = useState("");
  const [obs, setObs] = useState("Organização do estoque — migração IXC");
  const [fileName, setFileName] = useState("");
  const [rows, setRows] = useState<Row[]>([]);

  const [previewing, setPreviewing] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [previews, setPreviews] = useState<Prev[] | null>(null);
  const [destNome, setDestNome] = useState("");
  const [preErr, setPreErr] = useState("");
  const [filtro, setFiltro] = useState<"problemas" | "todas">("problemas");

  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [results, setResults] = useState<Res[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  useEffect(() => {
    let alive = true;
    apiFetch<{ locais_estoque: LocalOpt[] }>(`${CATALOG}?which=estoque_local`, { timeoutMs: 60_000 })
      .then((d) => alive && setLocais(d.locais_estoque ?? []))
      .catch((e) => alive && setLocaisErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, []);

  function resetRun() {
    setPreviews(null);
    setPreErr("");
    setResults([]);
    setStopMsg("");
    setAck(false);
    setTyped("");
  }
  function reset() {
    setRows([]);
    setFileName("");
    resetRun();
  }

  async function onFile(f: File | undefined) {
    reset();
    if (!f) return;
    setFileName(f.name);
    const objs = csvRowsToObjects(parseCsv(await f.text()));
    if (objs.length === 0) return missing("o CSV não tem linhas de dados.");
    if (objs.length > MAX_ROWS) return missing(`no máximo ${MAX_ROWS} linhas por arquivo (o arquivo tem ${objs.length}).`);
    const byNorm = (r: Record<string, string>, k: string) => {
      for (const [h, v] of Object.entries(r)) if (norm(h) === k) return (v ?? "").trim();
      return "";
    };
    if (!Object.keys(objs[0]).some((h) => norm(h) === "id_produto_item")) {
      setFileName("");
      return missing('este arquivo não tem a coluna "id_produto_item". Use o arquivo "6-TODOS-os-patrimonios-em-ESTOQUE-importados.csv".');
    }
    setRows(
      objs.map((r, i) => ({
        linha: i + 2,
        id_produto_item: byNorm(r, "id_produto_item"),
        rotulo: [byNorm(r, "produto"), byNorm(r, "identificador_proprio")].filter(Boolean).join(" · "),
      })),
    );
  }

  const destNomeSel = locais.find((l) => String(l.id_local_estoque) === destino)?.descricao ?? "";

  async function runPreview() {
    if (!destino) return missing("escolha o local de destino.");
    if (rows.length === 0) return missing("envie o CSV primeiro.");
    setPreviewing(true);
    resetRun();
    const acc: Prev[] = [];
    try {
      setProgress({ done: 0, total: rows.length });
      for (let i = 0; i < rows.length; i += PREVIEW_BATCH) {
        const chunk = rows.slice(i, i + PREVIEW_BATCH);
        const r = await apiFetch<{ destino: string; results: Prev[] }>(`${BASE}/preview`, { method: "POST", json: { rows: chunk, id_local_destino: destino, observacao: obs }, timeoutMs: 5 * 60_000 });
        acc.push(...r.results);
        setDestNome(r.destino);
        setProgress({ done: Math.min(i + PREVIEW_BATCH, rows.length), total: rows.length });
        // destino inexistente: todas as linhas voltam bloqueadas — não adianta continuar
        if (r.results.length > 0 && r.results.every((x) => !x.ok) && !r.destino) break;
      }
      setPreviews(acc);
    } catch (e) {
      setPreErr(`${(e as Error).message}${acc.length ? ` (já conferidas: ${acc.length} de ${rows.length})` : ""}`);
    } finally {
      setPreviewing(false);
    }
  }

  const counts = useMemo(() => {
    const c = { mover: 0, ja: 0, bloq: 0 };
    for (const p of previews ?? []) {
      if (!p.ok) c.bloq++;
      else if (p.already_done) c.ja++;
      else c.mover++;
    }
    return c;
  }, [previews]);
  const porOrigem = useMemo(() => {
    const m = new Map<string, number>();
    for (const p of previews ?? []) if (p.ok && !p.already_done) m.set(p.item?.local || "—", (m.get(p.item?.local || "—") ?? 0) + 1);
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  }, [previews]);
  const toSend = useMemo(() => {
    const okLines = new Set((previews ?? []).filter((p) => p.ok && !p.already_done).map((p) => p.linha));
    return rows.filter((r) => okLines.has(r.linha));
  }, [previews, rows]);

  const confirmText = `TRANSFERIR ${toSend.length}`;
  const canApply = toSend.length > 0 && ack && typed.trim() === confirmText && !applying && results.length === 0;

  async function runApply() {
    stopRef.current = false;
    setApplying(true);
    setStopMsg("");
    setResults([]);
    const acc: Res[] = [];
    try {
      setProgress({ done: 0, total: toSend.length });
      for (let i = 0; i < toSend.length && !stopRef.current; i += APPLY_BATCH) {
        const chunk = toSend.slice(i, i + APPLY_BATCH);
        const r = await apiFetch<{ results: Res[]; halted: boolean }>(`${BASE}/apply`, { method: "POST", json: { rows: chunk, id_local_destino: destino, observacao: obs }, timeoutMs: 8 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + APPLY_BATCH, toSend.length), total: toSend.length });
        if (r.halted) {
          setStopMsg("EXECUÇÃO INTERROMPIDA — três grupos seguidos foram recusados pela HubSoft. Veja a mensagem de cada linha e o histórico antes de repetir.");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador. Reenviar o mesmo arquivo é seguro: o que já foi transferido vira «já estava no destino».");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Reenvie o mesmo arquivo para continuar — o que já foi transferido é reconhecido. Confira o histórico.`);
    } finally {
      setApplying(false);
    }
    const okN = acc.filter((r) => r.ok).length;
    notify({ ok: true }, null, `${okN} de ${acc.length} patrimônio(s) processado(s) sem problema.`);
  }

  const okCount = results.filter((r) => r.ok).length;
  const movedCount = results.filter((r) => r.action === "transferred").length;
  const failCount = results.length - okCount;
  const shownPrev = (previews ?? []).filter((p) => filtro === "todas" || !p.ok || p.already_done);

  function downloadPending() {
    const rowByLine = new Map(rows.map((r) => [r.linha, r]));
    const head = ["linha", "id_produto_item", "rotulo", "motivo"];
    const out: string[][] = [];
    for (const p of previews ?? []) if (!p.ok) out.push([String(p.linha), rowByLine.get(p.linha)?.id_produto_item ?? "", rowByLine.get(p.linha)?.rotulo ?? "", (p.problems ?? []).join(" | ")]);
    for (const r of results) if (!r.ok) out.push([String(r.linha), "", "", `${ACTION_LABEL[r.action]}: ${r.message}`]);
    downloadCsv(`pendencias-transferencia-${todayISO()}.csv`, head, out);
  }
  function downloadResults() {
    downloadCsv(
      `transferencia-resultado-${todayISO()}.csv`,
      ["linha", "id_produto_item", "codigo_item", "produto", "resultado", "mensagem", "local_origem", "local_depois", "status_depois", "id_movimento_estoque", "detalhe_resposta_hubsoft"],
      results.map((r) => [String(r.linha), r.item?.id_produto_item ?? "", r.item?.codigo_item ?? "", r.item?.produto ?? "", ACTION_LABEL[r.action], r.message, r.local_origem ?? "", r.local_depois ?? "", r.status_depois ?? "", r.id_movimento_estoque ?? "", r.detail ?? ""]),
    );
  }

  return (
    <ToolPanel
      icon={<ArrowRightLeft size={20} />}
      title="Transferir patrimônios de local de estoque"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Move patrimônios que estão em Estoque de um local para outro (movimento «Transferência entre estoques»). Cada patrimônio é conferido antes e relido depois."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <Callout tone="info">
          <strong>Regras:</strong> só transfere patrimônio com status <b>Estoque</b> (em comodato, usuário ou vendido nada é movido); o local de origem é o local <b>atual</b> de cada item; quem já está no destino vira «já estava».
          A API da HubSoft <b>não cria local de estoque novo</b> — crie o destino na interface antes. O arquivo pode ser o <code>6-TODOS-os-patrimonios-em-ESTOQUE-importados.csv</code> (coluna <code>id_produto_item</code>; as demais são só para você identificar).
        </Callout>
      </div>

      <Step n={1} title="Destino e arquivo" done={rows.length > 0 && !!destino} hint="O arquivo é lido no navegador; nada é enviado à HubSoft nesta etapa.">
        <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
          <label className="hsa-check">
            Local de destino
            <select className="input" style={{ marginLeft: 8, minWidth: 300 }} value={destino} onChange={(e) => { setDestino(e.target.value); resetRun(); }} disabled={previewing || applying}>
              <option value="">{locais.length === 0 && !locaisErr ? "Carregando locais…" : "Selecione…"}</option>
              {locais.map((l) => (
                <option key={l.id_local_estoque} value={String(l.id_local_estoque)}>
                  {l.id_local_estoque} — {l.descricao}
                </option>
              ))}
            </select>
          </label>
        </div>
        {locaisErr ? <Callout tone="err">Não foi possível carregar os locais: {locaisErr}</Callout> : null}
        <div className="field" style={{ marginBottom: 0 }}>
          <label>Observação do movimento</label>
          <input className="input" value={obs} onChange={(e) => { setObs(e.target.value); resetRun(); }} />
        </div>
        <CsvDropzone fileName={fileName} info={rows.length > 0 ? `${rows.length} linha(s) lida(s)` : undefined} disabled={previewing || applying} onFile={(f) => void onFile(f)} onClear={reset} />
      </Step>

      <Step n={2} title="Conferir com a HubSoft (obrigatório)" done={!!previews && !previewing} hint="Somente leitura: confere se cada patrimônio existe, está em Estoque e em que local está.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={previewing || applying || rows.length === 0 || !destino} onClick={() => void runPreview()}>
            <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {previewing ? "Conferindo…" : previews ? "Conferir de novo" : `Conferir ${rows.length} linha(s)`}
          </button>
        </div>
        {previewing ? <ProgressBar done={progress.done} total={progress.total} label={`Conferindo… ${progress.done} de ${progress.total}`} /> : null}
        {preErr ? <Callout tone="err">Não foi possível conferir: {preErr}</Callout> : null}
        {previews ? (
          <>
            <div className="hsa-stats">
              <Stat label={`Serão movidos para «${destNome || destNomeSel}»`} value={counts.mover} tone="ok" />
              <Stat label="Já estão no destino" value={counts.ja} tone="muted" />
              <Stat label="Bloqueados" value={counts.bloq} tone={counts.bloq ? "err" : "muted"} />
            </div>
            {porOrigem.length > 0 ? (
              <div className="hsa-muted">
                Saem de: {porOrigem.map(([n, q]) => `${n} (${q.toLocaleString("pt-BR")})`).join(" · ")}
              </div>
            ) : null}
            <div className="hsa-actions">
              <label className="hsa-check">
                <input type="checkbox" checked={filtro === "todas"} onChange={(e) => setFiltro(e.target.checked ? "todas" : "problemas")} />
                Mostrar também os que serão movidos
              </label>
              <span className="hsa-spacer" />
              <button type="button" className="btn btn--sm" disabled={counts.bloq === 0} onClick={downloadPending}>
                <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Baixar pendências (CSV)
              </button>
            </div>
            <div className="hsa-table-wrap" style={{ maxHeight: 380 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <th>Linha</th>
                    <th>Patrimônio</th>
                    <th>Local atual</th>
                    <th>O que acontece</th>
                  </tr>
                </thead>
                <tbody>
                  {shownPrev.slice(0, 500).map((p) => (
                    <tr key={p.linha}>
                      <td className="hsa-table__num mono">{p.linha}</td>
                      <td>
                        {p.item?.produto ?? rows.find((r) => r.linha === p.linha)?.rotulo ?? "—"}
                        <div className="hsa-muted mono">{p.item ? `id ${p.item.id_produto_item} · cód. ${p.item.codigo_item || "—"} · ${p.item.identificador_proprio || "sem identificador"}` : ""}</div>
                      </td>
                      <td>
                        {p.item?.local || "—"}
                        <div className="hsa-muted">{p.item?.status || ""}</div>
                      </td>
                      <td>
                        {!p.ok ? <Pill tone="err">Bloqueado</Pill> : p.already_done ? <Pill>Já está no destino</Pill> : <Pill tone="ok">Será movido</Pill>}
                        {(p.problems ?? []).map((x) => (
                          <div key={x} className="hsa-muted" style={{ marginTop: 4, color: "var(--err)" }}>{x}</div>
                        ))}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {shownPrev.length > 500 ? <span className="hsa-muted">Mostrando as 500 primeiras de {shownPrev.length}. Baixe as pendências para ver todas.</span> : null}
          </>
        ) : null}
      </Step>

      {previews && toSend.length > 0 ? (
        <Step n={3} title="Transferir" done={results.length > 0 && !applying} hint="Esta etapa altera a HubSoft: os patrimônios mudam de local de estoque.">
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a conferência e entendo que isto move {toSend.length.toLocaleString("pt-BR")} patrimônio(s) para «{destNome || destNomeSel}» de verdade. Bloqueados ficam de fora.
          </label>
          <div className="hsa-actions">
            <input className="input" style={{ width: 230 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
            <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void runApply()}>
              {applying ? "Transferindo…" : `Transferir ${toSend.length.toLocaleString("pt-BR")}`}
            </button>
            {applying ? (
              <button type="button" className="btn" onClick={() => { stopRef.current = true; }}>
                <Square size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
          </div>
          <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão. Vai em lotes de {APPLY_BATCH}; para após 3 recusas seguidas e pode ser retomado reenviando o mesmo arquivo.</span>
          {applying ? <ProgressBar done={progress.done} total={progress.total} label={`Transferindo… ${progress.done} de ${progress.total}`} /> : null}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step n={4} title="Resultado" done={!applying && failCount === 0} hint={'Cada patrimônio também fica na aba "Histórico" (Transferência de estoque).'}>
          <div className="hsa-stats">
            <Stat label="Transferidos agora" value={movedCount} tone="ok" />
            <Stat label="Já estavam / sem problema" value={okCount - movedCount} tone="muted" />
            <Stat label="Com problema" value={failCount} tone={failCount ? "err" : "muted"} />
          </div>
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={downloadResults}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Resultado completo (CSV)
            </button>
            <button type="button" className="btn btn--sm" disabled={failCount === 0} onClick={downloadPending}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Pendências (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap" style={{ maxHeight: 420 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Patrimônio</th>
                  <th>Resultado</th>
                </tr>
              </thead>
              <tbody>
                {results.filter((r) => !r.ok || results.length <= 200).slice(0, 500).map((r) => (
                  <tr key={r.linha}>
                    <td className="hsa-table__num mono">{r.linha}</td>
                    <td className="mono">
                      {r.item?.produto ?? "—"}
                      <div className="hsa-muted">id {r.item?.id_produto_item ?? "—"} · cód. {r.item?.codigo_item ?? "—"}</div>
                    </td>
                    <td>
                      <Pill tone={ACTION_TONE[r.action]}>{ACTION_LABEL[r.action]}</Pill>
                      <div className="hsa-muted" style={{ marginTop: 4, color: r.ok ? undefined : "var(--err)" }}>{r.message}</div>
                      {r.detail ? (
                        <details style={{ marginTop: 4 }}>
                          <summary style={{ cursor: "pointer", fontSize: 11 }}>Resposta bruta da HubSoft</summary>
                          <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word", margin: "4px 0 0" }}>{r.detail}</pre>
                        </details>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {results.length > 200 ? <span className="hsa-muted">Com muitos itens, a tabela mostra só os com problema; baixe o resultado completo para ver todos.</span> : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
