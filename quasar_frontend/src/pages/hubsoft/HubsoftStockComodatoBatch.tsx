import { useEffect, useMemo, useRef, useState } from "react";
import { Download, Handshake, ListChecks, Square } from "lucide-react";
import { Callout, CsvDropzone, Pill, ProgressBar, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { absentColumns, pickField, type StockField } from "./hubsoftStockFields";

const BASE = "/api/v1/integrations/hubsoft/hubsoft/stock-comodato";
const BATCH = 5; // cada linha faz ~8 chamadas à HubSoft (serviço, patrimônio, produto, vínculos, saída, releitura…)
const MAX_ROWS = 2000;

/**
 * Comodato EM LOTE (passo 3 da migração). Cada linha do CSV liga UM patrimônio que está em estoque ao serviço de um cliente
 * (saída de estoque para o serviço, tipo de movimento «Comodato»). O patrimônio é localizado por id_produto_item, identificador,
 * série, MAC ou código do item (se vier mais de um campo, todos precisam apontar para o MESMO patrimônio). Fluxo: CSV → tipo de
 * movimento → conferir tudo (somente leitura) → confirmar → enviar de 5 em 5 → relê cada patrimônio.
 */

const FIELDS: StockField[] = [
  { key: "id_cliente_servico", aliases: ["id_cliente_servico", "id_servico"], required: true },
  { key: "id_produto_item", aliases: ["id_produto_item"] },
  { key: "identificador_proprio", aliases: ["identificador_proprio", "identificador"] },
  { key: "numero_serie", aliases: ["numero_serie", "serie"] },
  { key: "mac_address", aliases: ["mac_address", "mac"] },
  { key: "codigo_item", aliases: ["codigo_item"] },
  { key: "id_produto", aliases: ["id_produto"] },
  { key: "cliente", aliases: ["cliente", "nome_cliente"] },
  { key: "login", aliases: ["login", "login_radius"] },
  { key: "observacao", aliases: ["observacao", "observacoes"] },
];
const LOCATORS = ["id_produto_item", "identificador_proprio", "numero_serie", "mac_address", "codigo_item"];

type Row = Record<string, string | number>;
type MType = { id: string; nome: string; status_prefixo: string; status_nome: string; usos: number };
type PItem = { ID?: string; ProdutoNome?: string; Identificador?: string; Serie?: string; MAC?: string; CodigoItem?: string; Status?: string; LocalNome?: string };
type Svc = { id_cliente: string; cliente: string; id_cliente_servico: string; login?: string; plano?: string; status?: string };
type Preview = { ok: boolean; already_done?: boolean; problems?: string[]; warnings?: string[]; service?: Svc; item?: PItem; produto?: string };
type PRow = { linha: number; locator?: string; preview: Preview };
type Result = {
  ok: boolean;
  action: "created" | "already_done" | "blocked" | "failed" | "verify_failed" | "skipped";
  message: string;
  id_movimento_estoque?: string;
  id_produto_item?: string;
  status_depois?: string;
  verified?: boolean;
  verify_message?: string;
  detail?: string;
  preview: Preview;
};
type RRow = { linha: number; locator?: string; result: Result };

const ACTION_LABEL: Record<Result["action"], string> = {
  created: "Ligado como comodato",
  already_done: "Já estava neste serviço — nada enviado",
  blocked: "Bloqueado antes de enviar",
  failed: "A HubSoft recusou",
  verify_failed: "Enviado, mas a conferência divergiu",
  skipped: "Não enviado (falha anterior)",
};
const ACTION_TONE: Record<Result["action"], "ok" | "err" | "warn" | undefined> = {
  created: "ok",
  already_done: undefined,
  blocked: "warn",
  failed: "err",
  verify_failed: "err",
  skipped: "warn",
};

type Kind = "enviar" | "ja_esta" | "bloqueada" | "duplicada";
const KIND_LABEL: Record<Kind, string> = { enviar: "Será ligado", ja_esta: "Já está neste serviço", bloqueada: "Bloqueada", duplicada: "Repetida no arquivo" };
const KIND_TONE: Record<Kind, "ok" | "err" | "warn" | undefined> = { enviar: "ok", ja_esta: undefined, bloqueada: "err", duplicada: "warn" };

function itemText(i?: PItem) {
  if (!i) return "—";
  return [i.ProdutoNome, i.Identificador && `id ${i.Identificador}`, i.Serie && `série ${i.Serie}`].filter(Boolean).join(" · ") || "—";
}

export function HubsoftStockComodatoBatch() {
  const { notify, missing } = useConsultaToast();
  const [fileName, setFileName] = useState("");
  const [rawRows, setRawRows] = useState<Record<string, string>[]>([]);

  const [types, setTypes] = useState<MType[]>([]);
  const [typesMsg, setTypesMsg] = useState("");
  const [typesLoading, setTypesLoading] = useState(true);
  const [tipoId, setTipoId] = useState("");
  const [tipoManual, setTipoManual] = useState("");
  const [tipoConfirm, setTipoConfirm] = useState(false);
  const [obs, setObs] = useState("Comodato — migração do IXC");

  const [previewing, setPreviewing] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [previews, setPreviews] = useState<PRow[] | null>(null);
  const [preErr, setPreErr] = useState("");

  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [results, setResults] = useState<RRow[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  const comodatoTypes = useMemo(() => types.filter((t) => t.status_prefixo.toLowerCase() === "comodato"), [types]);
  const usingManual = comodatoTypes.length === 0 || tipoId === "__manual";
  const effTipo = usingManual ? tipoManual.trim() : tipoId;

  useEffect(() => {
    let alive = true;
    apiFetch<{ ok: boolean; message?: string; types: MType[] }>(`${BASE}/movement-types`, { timeoutMs: 5 * 60_000 })
      .then((d) => {
        if (!alive) return;
        setTypes(d.types ?? []);
        if (!d.ok) setTypesMsg(d.message ?? "Não foi possível descobrir os tipos de movimento.");
        const first = (d.types ?? []).find((t) => t.status_prefixo.toLowerCase() === "comodato");
        if (first) setTipoId(first.id);
      })
      .catch((e) => alive && setTypesMsg((e as Error).message))
      .finally(() => alive && setTypesLoading(false));
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
    setRawRows([]);
    setFileName("");
    resetRun();
  }

  async function onFile(f: File | undefined) {
    reset();
    if (!f) return;
    setFileName(f.name);
    const objs = csvRowsToObjects(parseCsv(await f.text()));
    if (objs.length === 0) return missing("o CSV não tem linhas de dados.");
    if (objs.length > MAX_ROWS) return missing(`no máximo ${MAX_ROWS} linhas por importação (o arquivo tem ${objs.length}).`);
    const absent = absentColumns(objs[0], FIELDS);
    if (absent.length > 0) {
      setFileName("");
      return missing(`este arquivo não parece ser um CSV de comodato — falta a coluna: ${absent.join(", ")}. Use o modelo (botão "Baixar modelo CSV").`);
    }
    setRawRows(objs);
  }

  const apiRows: Row[] = useMemo(
    () =>
      rawRows.map((r, i) => {
        const o: Row = { linha: i + 2 };
        for (const f of FIELDS) o[f.key] = pickField(r, f.aliases);
        return o;
      }),
    [rawRows],
  );
  // problemas que dá para ver sem a HubSoft: serviço não numérico ou sem nenhum campo para achar o patrimônio
  const localProblems = useMemo(() => {
    const m = new Map<number, string>();
    for (const r of apiRows) {
      if (!/^\d+$/.test(String(r.id_cliente_servico))) m.set(Number(r.linha), "id_cliente_servico vazio ou não numérico");
      else if (!LOCATORS.some((k) => String(r[k] ?? "") !== "")) m.set(Number(r.linha), "nenhum campo para localizar o patrimônio (id_produto_item, identificador_proprio, numero_serie, mac_address ou codigo_item)");
    }
    return m;
  }, [apiRows]);
  const sendable = useMemo(() => apiRows.filter((r) => !localProblems.has(Number(r.linha))), [apiRows, localProblems]);

  const reqExtra = () => ({ id_tipo_movimento_estoque: effTipo, tipo_confirmado_comodato: usingManual && tipoConfirm });
  const withObs = (r: Row) => ({ ...r, observacao: String(r.observacao ?? "") || obs });

  async function runPreview() {
    if (!effTipo) return missing("escolha o tipo de movimento.");
    setPreviewing(true);
    resetRun();
    const acc: PRow[] = [];
    try {
      setProgress({ done: 0, total: sendable.length });
      for (let i = 0; i < sendable.length; i += BATCH) {
        const chunk = sendable.slice(i, i + BATCH).map(withObs);
        const r = await apiFetch<{ results: PRow[]; types_warning?: string }>(`${BASE}/preview-batch`, { method: "POST", json: { rows: chunk, ...reqExtra() }, timeoutMs: 5 * 60_000 });
        acc.push(...r.results);
        setProgress({ done: Math.min(i + BATCH, sendable.length), total: sendable.length });
      }
      setPreviews(acc);
    } catch (e) {
      setPreErr(`${(e as Error).message}${acc.length ? ` (já conferidas: ${acc.length} de ${sendable.length})` : ""}`);
    } finally {
      setPreviewing(false);
    }
  }

  // classificação de cada linha conferida; o mesmo patrimônio em duas linhas só vale na primeira
  const classified = useMemo(() => {
    const seen = new Map<string, number>();
    return (previews ?? []).map((p) => {
      const id = p.preview.item?.ID ?? "";
      let kind: Kind = !p.preview.ok ? "bloqueada" : p.preview.already_done ? "ja_esta" : "enviar";
      let extra = "";
      if (id && p.preview.ok) {
        const first = seen.get(id);
        if (first !== undefined) {
          kind = "duplicada";
          extra = `o mesmo patrimônio já aparece na linha ${first} — só a primeira é usada`;
        } else seen.set(id, p.linha);
      }
      return { ...p, kind, extra };
    });
  }, [previews]);
  const counts = useMemo(() => {
    const c: Record<Kind, number> = { enviar: 0, ja_esta: 0, bloqueada: 0, duplicada: 0 };
    for (const p of classified) c[p.kind]++;
    return c;
  }, [classified]);
  const toSend = useMemo(() => {
    const ok = new Set(classified.filter((p) => p.kind === "enviar").map((p) => p.linha));
    return sendable.filter((r) => ok.has(Number(r.linha)));
  }, [classified, sendable]);

  const confirmText = `COMODATO ${toSend.length}`;
  const canApply = toSend.length > 0 && ack && typed.trim() === confirmText && !applying && results.length === 0;

  async function runApply() {
    stopRef.current = false;
    setApplying(true);
    setStopMsg("");
    setResults([]);
    const acc: RRow[] = [];
    try {
      setProgress({ done: 0, total: toSend.length });
      for (let i = 0; i < toSend.length && !stopRef.current; i += BATCH) {
        const chunk = toSend.slice(i, i + BATCH).map(withObs);
        const r = await apiFetch<{ results: RRow[] }>(`${BASE}/apply-batch`, { method: "POST", json: { rows: chunk, ...reqExtra() }, timeoutMs: 5 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + BATCH, toSend.length), total: toSend.length });
        if (r.results.some((x) => x.result.action === "failed" || x.result.action === "verify_failed")) {
          setStopMsg("EXECUÇÃO INTERROMPIDA — a HubSoft recusou ou a conferência divergiu. Veja a mensagem de cada linha abaixo e o histórico antes de repetir. Reenviar o mesmo arquivo é seguro: o que já foi ligado vira «já estava neste serviço».");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador. Reenviar o mesmo arquivo continua de onde parou: o que já foi ligado vira «já estava neste serviço».");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Reenvie o mesmo arquivo para continuar — o que já foi ligado é reconhecido. Confira o histórico.`);
    } finally {
      setApplying(false);
    }
    const okN = acc.filter((r) => r.result.ok).length;
    notify({ ok: true }, null, `${okN} de ${acc.length} comodato(s) processado(s) sem problema.`);
  }

  // ---- downloads -----------------------------------------------------------------------------------------------
  const rowByLine = useMemo(() => new Map(apiRows.map((r) => [Number(r.linha), r])), [apiRows]);
  function downloadTemplate() {
    downloadCsv("modelo-comodato-hubsoft.csv", FIELDS.map((f) => f.key), [["1197", "254", "", "", "", "", "", "MARIA JOSE BENAZIO MOREIRA", "", "Comodato — migração do IXC"]]);
  }
  function downloadPending() {
    const head = ["motivo", "linha", ...FIELDS.map((f) => f.key)];
    const line = (motivo: string, n: number) => [motivo, String(n), ...FIELDS.map((f) => String(rowByLine.get(n)?.[f.key] ?? ""))];
    const rows: string[][] = [];
    for (const [n, why] of localProblems) rows.push(line(`LINHA INVÁLIDA: ${why}`, n));
    for (const p of classified) if (p.kind === "bloqueada" || p.kind === "duplicada") rows.push(line(p.kind === "duplicada" ? p.extra : (p.preview.problems ?? []).join(" | "), p.linha));
    for (const r of results) if (!r.result.ok) rows.push(line(`${ACTION_LABEL[r.result.action]}: ${r.result.message}`, r.linha));
    downloadCsv(`pendencias-comodato-${todayISO()}.csv`, head, rows);
  }
  function downloadResults() {
    downloadCsv(
      `comodato-resultado-${todayISO()}.csv`,
      ["linha", "resultado", "mensagem", "id_cliente_servico", "id_produto_item", "id_movimento_estoque", "status_depois", "conferencia", "detalhe_resposta_hubsoft"],
      results.map((r) => [
        String(r.linha), ACTION_LABEL[r.result.action] ?? r.result.action, r.result.message, r.result.preview.service?.id_cliente_servico ?? "", r.result.id_produto_item ?? "",
        r.result.id_movimento_estoque ?? "", r.result.status_depois ?? "", r.result.verified === undefined ? "" : `${r.result.verified ? "OK" : "DIFERENÇA"} — ${r.result.verify_message ?? ""}`, r.result.detail ?? "",
      ]),
    );
  }

  const okCount = results.filter((r) => r.result.ok).length;
  const createdCount = results.filter((r) => r.result.action === "created").length;
  const failCount = results.length - okCount;
  const hasPending = localProblems.size > 0 || counts.bloqueada + counts.duplicada > 0 || failCount > 0;

  return (
    <ToolPanel
      icon={<Handshake size={20} />}
      title="Comodato em lote (CSV)"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Liga vários patrimônios que estão em estoque ao serviço de cada cliente, como comodato. Tudo é conferido linha a linha antes e cada patrimônio é relido depois."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <div className="hsa-actions">
          <button type="button" className="btn btn--sm" onClick={downloadTemplate}>
            <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            Baixar modelo CSV de comodato
          </button>
          <span className="hsa-step__hint" style={{ margin: 0 }}>
            Uma linha por patrimônio. <b>id_cliente_servico</b> é obrigatório; para achar o patrimônio use <b>id_produto_item</b>, <b>identificador_proprio</b>, <b>numero_serie</b>, <b>mac_address</b> ou <b>codigo_item</b>. Se vier mais de um, todos têm de apontar para o mesmo patrimônio. <b>id_produto</b>, <b>cliente</b> e <b>login</b> são opcionais e só servem de conferência (evitam ligar ao serviço errado).
          </span>
        </div>
        <Callout tone="info">
          <strong>Segurança:</strong> só liga patrimônio com status <b>Estoque</b>, a serviço existente e não cancelado, com tipo de movimento que gere <b>Comodato</b>. Quem já está como comodato <b>neste mesmo serviço</b> vira «já estava» (nada é enviado); em outro serviço, é bloqueado. Duas linhas com o mesmo patrimônio: só a primeira vale.
        </Callout>
      </div>

      <Step n={1} title="Arquivo CSV" done={rawRows.length > 0} hint="O arquivo é lido no navegador; nada é enviado à HubSoft nesta etapa.">
        <CsvDropzone fileName={fileName} info={rawRows.length > 0 ? `${rawRows.length} linha(s) lida(s)` : undefined} disabled={previewing || applying} onFile={(f) => void onFile(f)} onClear={reset} />
        {localProblems.size > 0 ? (
          <Callout tone="warn">
            {localProblems.size} linha(s) já estão inválidas e ficam de fora: {[...localProblems].slice(0, 5).map(([n, w]) => `linha ${n} (${w})`).join("; ")}
            {localProblems.size > 5 ? "…" : ""}
          </Callout>
        ) : null}
      </Step>

      <Step n={2} title="Tipo de movimento e observação" done={!!effTipo} hint="Só os tipos que geram o status «Comodato» são oferecidos (descobertos nos movimentos de saída para clientes que já existem na HubSoft).">
        {typesLoading ? <ConsultaLoading text="Descobrindo os tipos de movimento (lê os movimentos de estoque da HubSoft)…" /> : null}
        {typesMsg ? <Callout tone="warn">{typesMsg}</Callout> : null}
        {!typesLoading && comodatoTypes.length > 0 ? (
          <div className="hsa-actions">
            <label className="hsa-check">
              Tipo de movimento
              <select className="input" style={{ marginLeft: 8, minWidth: 320 }} value={tipoId} onChange={(e) => { setTipoId(e.target.value); resetRun(); }} disabled={previewing || applying}>
                {comodatoTypes.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.id} — {t.nome} (gera {t.status_nome}; usado {t.usos}×)
                  </option>
                ))}
                <option value="__manual">Informar outro id manualmente…</option>
              </select>
            </label>
          </div>
        ) : null}
        {!typesLoading && usingManual ? (
          <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
            <input className="input" style={{ width: 200 }} placeholder="id do tipo de movimento" value={tipoManual} onChange={(e) => { setTipoManual(e.target.value); resetRun(); }} />
            <label className="hsa-check">
              <input type="checkbox" checked={tipoConfirm} onChange={(e) => { setTipoConfirm(e.target.checked); resetRun(); }} />
              Confirmo que este tipo de movimento gera o status <b>Comodato</b> (será conferido depois de cada envio)
            </label>
          </div>
        ) : null}
        <div className="field" style={{ marginBottom: 0 }}>
          <label>Observação do movimento (usada nas linhas sem a coluna «observacao»)</label>
          <input className="input" value={obs} onChange={(e) => { setObs(e.target.value); resetRun(); }} />
        </div>
      </Step>

      <Step n={3} title="Conferir com a HubSoft (obrigatório)" done={!!previews && !previewing} hint="Somente leitura. Para cada linha: acha o patrimônio, confere o serviço, o status, o produto e o que o serviço já tem.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={previewing || applying || sendable.length === 0 || !effTipo} onClick={() => void runPreview()}>
            <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {previewing ? "Conferindo…" : previews ? "Conferir de novo" : `Conferir ${sendable.length} linha(s)`}
          </button>
          {sendable.length === 0 ? <span className="hsa-muted">Envie o CSV primeiro.</span> : !effTipo ? <span className="hsa-muted">Escolha o tipo de movimento.</span> : null}
        </div>
        {previewing ? <ProgressBar done={progress.done} total={progress.total} label={`Conferindo… ${progress.done} de ${progress.total}`} /> : null}
        {preErr ? <Callout tone="err">Não foi possível conferir: {preErr}</Callout> : null}
        {previews ? (
          <>
            <div className="hsa-stats">
              <Stat label="Serão ligados" value={counts.enviar} tone="ok" />
              <Stat label="Já estavam no serviço" value={counts.ja_esta} tone="muted" />
              <Stat label="Bloqueadas" value={counts.bloqueada} tone={counts.bloqueada ? "err" : "muted"} />
              <Stat label="Repetidas no arquivo" value={counts.duplicada} tone={counts.duplicada ? "warn" : "muted"} />
            </div>
            <div className="hsa-actions">
              <span className="hsa-spacer" />
              <button type="button" className="btn btn--sm" disabled={!hasPending} onClick={downloadPending}>
                <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Baixar pendências (CSV)
              </button>
            </div>
            <div className="hsa-table-wrap" style={{ maxHeight: 420 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <th>Linha</th>
                    <th>Patrimônio</th>
                    <th>Serviço</th>
                    <th>O que acontece</th>
                  </tr>
                </thead>
                <tbody>
                  {classified.slice(0, 500).map((p) => (
                    <tr key={p.linha}>
                      <td className="hsa-table__num mono">{p.linha}</td>
                      <td>
                        {itemText(p.preview.item)}
                        <div className="hsa-muted mono">{p.preview.item?.ID ? `id_produto_item ${p.preview.item.ID}` : ""}{p.locator ? ` · achado por ${p.locator}` : ""}</div>
                      </td>
                      <td>
                        {p.preview.service ? (
                          <>
                            {p.preview.service.cliente}
                            <div className="hsa-muted mono">serviço {p.preview.service.id_cliente_servico} · {p.preview.service.login || "sem login"} · {p.preview.service.status || "—"}</div>
                          </>
                        ) : (
                          "—"
                        )}
                      </td>
                      <td>
                        <Pill tone={KIND_TONE[p.kind]}>{KIND_LABEL[p.kind]}</Pill>
                        {p.extra ? <div className="hsa-muted" style={{ marginTop: 4 }}>{p.extra}</div> : null}
                        {(p.preview.problems ?? []).map((x) => (
                          <div key={x} className="hsa-muted" style={{ marginTop: 4, color: "var(--err)" }}>{x}</div>
                        ))}
                        {(p.preview.warnings ?? []).map((x) => (
                          <div key={x} className="hsa-muted" style={{ marginTop: 4, color: "var(--warn)" }}>{x}</div>
                        ))}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {classified.length > 500 ? <span className="hsa-muted">Mostrando as 500 primeiras de {classified.length}. Baixe as pendências para ver todas as bloqueadas.</span> : null}
          </>
        ) : null}
      </Step>

      {previews && toSend.length > 0 ? (
        <Step n={4} title="Ligar os patrimônios" done={results.length > 0 && !applying} hint="Esta etapa altera a HubSoft: cada patrimônio sai do estoque e fica vinculado ao serviço.">
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a conferência e entendo que isto liga {toSend.length} patrimônio(s) a clientes de verdade. Linhas bloqueadas ou repetidas ficam de fora.
          </label>
          <div className="hsa-actions">
            <input className="input" style={{ width: 210 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
            <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void runApply()}>
              {applying ? "Ligando…" : `Ligar ${toSend.length}`}
            </button>
            {applying ? (
              <button type="button" className="btn" onClick={() => { stopRef.current = true; }}>
                <Square size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
          </div>
          <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão. É lento de propósito (cada linha é conferida de novo antes de enviar); para na primeira recusa da HubSoft.</span>
          {applying ? <ProgressBar done={progress.done} total={progress.total} label={`Ligando… ${progress.done} de ${progress.total}`} /> : null}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step n={5} title="Resultado" done={!applying && failCount === 0} hint={'Cada linha também fica registrada na aba "Histórico" (Comodato de estoque).'}>
          <div className="hsa-stats">
            <Stat label="Ligados agora" value={createdCount} tone="ok" />
            <Stat label="Já estavam / sem problema" value={okCount - createdCount} tone="muted" />
            <Stat label="Com problema" value={failCount} tone={failCount ? "err" : "muted"} />
          </div>
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={downloadResults}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Resultado completo (CSV)
            </button>
            <button type="button" className="btn btn--sm" disabled={!hasPending} onClick={downloadPending}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Pendências (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Serviço / patrimônio</th>
                  <th>Resultado</th>
                  <th>Conferência</th>
                </tr>
              </thead>
              <tbody>
                {results.map((r) => (
                  <tr key={r.linha}>
                    <td className="hsa-table__num mono">{r.linha}</td>
                    <td className="mono">
                      serviço {r.result.preview.service?.id_cliente_servico ?? "—"}
                      <div className="hsa-muted">patrimônio {r.result.id_produto_item || "—"}{r.result.id_movimento_estoque ? ` · movimento ${r.result.id_movimento_estoque}` : ""}</div>
                    </td>
                    <td>
                      <Pill tone={ACTION_TONE[r.result.action]}>{ACTION_LABEL[r.result.action] ?? r.result.action}</Pill>
                      <div className="hsa-muted" style={{ marginTop: 4, color: r.result.ok ? undefined : "var(--err)" }}>{r.result.message}</div>
                      {r.result.detail ? (
                        <details style={{ marginTop: 4 }}>
                          <summary style={{ cursor: "pointer", fontSize: 11 }}>Resposta bruta da HubSoft</summary>
                          <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word", margin: "4px 0 0" }}>{r.result.detail}</pre>
                        </details>
                      ) : null}
                    </td>
                    <td className="hsa-muted">
                      {r.result.status_depois ? <div>Status depois: {r.result.status_depois}</div> : null}
                      {r.result.verified !== undefined ? <div style={{ color: r.result.verified ? "var(--ok)" : "var(--err)" }}>{r.result.verified ? "OK" : "Diferença"} — {r.result.verify_message}</div> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Step>
      ) : null}
    </ToolPanel>
  );
}
