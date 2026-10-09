import { useEffect, useMemo, useRef, useState } from "react";
import { Download, ListChecks, Square, Barcode } from "lucide-react";
import { Callout, CsvDropzone, Pill, ProgressBar, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { ITEM_FIELDS, absentColumns, pickField } from "./hubsoftStockFields";

const SLUG = "hubsoft";
const BASE = `/api/v1/integrations/${SLUG}/hubsoft/stock-items`;
const CATALOG = `/api/v1/integrations/${SLUG}/hubsoft/catalog/fetch`;
const APPLY_BATCH = 5; // cada patrimônio faz ~6 chamadas à HubSoft (3 consultas de duplicidade, entrada, gravação, conferência)
const MAX_ROWS = 10000;
const DEFAULT_LOCAL_ID = "6"; // ALMOXARIFADO MATRIZ

/**
 * Cadastro em massa de PATRIMÔNIOS de estoque (passo 2 da migração do IXC). Cada linha do CSV é um patrimônio físico
 * (roteador/ONU/ONT/switch…) com identificador próprio e, quando houver, série e MAC. Fluxo: enviar CSV → validar → conferir
 * com a HubSoft (duplicidade de identificador/série/MAC em TODOS os produtos) → criar. Cada patrimônio faz uma entrada de
 * estoque de 1 unidade, recebe identificador/série/MAC e é conferido. Série/MAC duplicados nunca são criados: viram pendência.
 */

const FIELDS = ITEM_FIELDS;

type RowValidation = { line: number; valid: boolean; problems?: string[]; label?: string };
type ValidationResp = { ok: boolean; rows: RowValidation[]; valid: number; invalid: number };
type PreStatus = "novo" | "ja_existe" | "ja_existe_divergente" | "conflito_identificador" | "conflito_serie" | "conflito_mac" | "retomar" | "retomar_invalido" | "erro_produto";
type PreRow = { line: number; label: string; status: PreStatus; message: string; id_produto_item?: string };
type Summary = { produtos_patrimoniais: number; patrimonios_na_hubsoft: number; sem_identificacao: number; por_status: Record<string, number> };
type ApplyResult = {
  line: number;
  label: string;
  ok: boolean;
  action: "created" | "already_exists" | "conflict" | "rejected_local" | "failed" | "identify_failed" | "resumed";
  message: string;
  id_produto_item?: string;
  codigo_item?: string;
  created?: boolean;
  pending?: boolean;
  identify_ok?: boolean;
  identify_message?: string;
  mac_omitted?: boolean;
  mac_omitted_reason?: string;
  verified?: boolean;
  verify_message?: string;
  detail?: string;
};
type LocalOpt = { id_local_estoque: number; descricao: string };

const PRE_LABEL: Record<PreStatus, string> = {
  novo: "Novo — será criado",
  ja_existe: "Já existe — nada será criado",
  ja_existe_divergente: "Já existe com dados DIFERENTES",
  conflito_identificador: "Identificador de OUTRO produto",
  conflito_serie: "Série já existe na HubSoft",
  conflito_mac: "MAC já existe na HubSoft",
  retomar: "Patrimônio já criado — só identificar",
  retomar_invalido: "Retomada inválida",
  erro_produto: "Produto/linha inválidos",
};
const PRE_TONE: Record<PreStatus, "ok" | "err" | "warn" | undefined> = {
  novo: "ok",
  ja_existe: undefined,
  ja_existe_divergente: "warn",
  conflito_identificador: "err",
  conflito_serie: "err",
  conflito_mac: "err",
  retomar: "ok",
  retomar_invalido: "err",
  erro_produto: "err",
};
const CREATABLE = new Set<PreStatus>(["novo", "retomar"]);
const ACTION_LABEL: Record<ApplyResult["action"], string> = {
  created: "Criado e identificado",
  already_exists: "Já existia — nada foi criado",
  conflict: "Conflito — vai para as pendências",
  rejected_local: "Recusado antes de enviar",
  failed: "Falhou — nada foi criado",
  identify_failed: "Criado, mas SEM identificação",
  resumed: "Identificado (patrimônio já criado)",
};
const ACTION_TONE: Record<ApplyResult["action"], "ok" | "err" | "warn" | undefined> = {
  created: "ok",
  already_exists: undefined,
  conflict: "warn",
  rejected_local: "err",
  failed: "err",
  identify_failed: "err",
  resumed: "ok",
};

function templateRow() {
  return {
    head: FIELDS.map((f) => f.key),
    example: ["131", "ONU EXEMPLO", "EXEMPLO-001", "", "SERIE0000001", "AA:BB:CC:DD:EE:FF", "IXC cód 9999 | contrato 123", "", "9999"],
  };
}

export function HubsoftStockItems() {
  const { notify, missing } = useConsultaToast();
  const [locais, setLocais] = useState<LocalOpt[]>([]);
  const [locaisErr, setLocaisErr] = useState("");
  const [localId, setLocalId] = useState("");
  const [fileName, setFileName] = useState("");
  const [rawRows, setRawRows] = useState<Record<string, string>[]>([]);

  const [validating, setValidating] = useState(false);
  const [validation, setValidation] = useState<ValidationResp | null>(null);
  const [filter, setFilter] = useState<"all" | "valid" | "invalid">("all");

  const [preRunning, setPreRunning] = useState(false);
  const [preflight, setPreflight] = useState<PreRow[] | null>(null);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [preErr, setPreErr] = useState("");

  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<ApplyResult[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  useEffect(() => {
    let alive = true;
    apiFetch<{ locais_estoque: LocalOpt[] }>(`${CATALOG}?which=estoque_local`, { timeoutMs: 60_000 })
      .then((d) => {
        if (!alive) return;
        const list = d.locais_estoque ?? [];
        setLocais(list);
        if (list.some((l) => String(l.id_local_estoque) === DEFAULT_LOCAL_ID)) setLocalId(DEFAULT_LOCAL_ID);
      })
      .catch((e) => alive && setLocaisErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, []);

  function reset() {
    setRawRows([]);
    setFileName("");
    setValidation(null);
    setPreflight(null);
    setSummary(null);
    setPreErr("");
    setResults([]);
    setStopMsg("");
    setAck(false);
    setTyped("");
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
      return missing(`este arquivo não parece ser um CSV de patrimônios — faltam as colunas: ${absent.join(", ")}. Use o modelo (botão "Baixar modelo CSV").`);
    }
    setRawRows(objs);
  }

  const apiRows = useMemo(
    () =>
      rawRows.map((r, i) => {
        const o: Record<string, string | number> = { line: i + 2 };
        for (const f of FIELDS) o[f.key] = pickField(r, f.aliases);
        return o;
      }),
    [rawRows],
  );
  const localNome = locais.find((l) => String(l.id_local_estoque) === localId)?.descricao ?? "";

  async function runValidate() {
    if (rawRows.length === 0) return missing("envie um CSV primeiro.");
    if (!localId) return missing("escolha o local de estoque.");
    setValidating(true);
    setValidation(null);
    setPreflight(null);
    setSummary(null);
    setResults([]);
    setAck(false);
    setTyped("");
    try {
      const resp = await apiFetch<ValidationResp>(`${BASE}/validate`, { method: "POST", json: { rows: apiRows, check_products: true, id_local_estoque: localId }, timeoutMs: 3 * 60_000 });
      setValidation(resp);
      setFilter(resp.invalid > 0 ? "invalid" : "all");
      notify(resp, null, `Consulta realizada com sucesso — ${resp.valid} válida(s), ${resp.invalid} inválida(s).`);
    } catch (e) {
      notify(null, e);
    } finally {
      setValidating(false);
    }
  }

  const validAll = useMemo(() => {
    if (!validation) return [];
    const ok = new Set(validation.rows.filter((r) => r.valid).map((r) => r.line));
    return apiRows.filter((r) => ok.has(Number(r.line)));
  }, [validation, apiRows]);

  async function runPreflight() {
    setPreRunning(true);
    setPreErr("");
    setPreflight(null);
    setSummary(null);
    try {
      const r = await apiFetch<{ results: PreRow[]; summary: Summary }>(`${BASE}/preflight`, { method: "POST", json: { rows: validAll, id_local_estoque: localId }, timeoutMs: 12 * 60_000 });
      setPreflight(r.results);
      setSummary(r.summary);
    } catch (e) {
      setPreErr((e as Error).message);
    } finally {
      setPreRunning(false);
    }
  }

  // Só entram na criação as linhas válidas que a conferência liberou (novo / retomar). Sem conferência, nada é criado.
  const toCreate = useMemo(() => {
    if (!preflight) return [];
    const ok = new Set(preflight.filter((p) => CREATABLE.has(p.status)).map((p) => p.line));
    return validAll.filter((r) => ok.has(Number(r.line)));
  }, [preflight, validAll]);
  const preCounts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const p of preflight ?? []) c[p.status] = (c[p.status] ?? 0) + 1;
    return c;
  }, [preflight]);

  const shownRows = useMemo(() => {
    if (!validation) return [];
    return filter === "all" ? validation.rows : validation.rows.filter((r) => (filter === "valid" ? r.valid : !r.valid));
  }, [validation, filter]);

  const confirmText = `CRIAR ${toCreate.length}`;
  const canApply = toCreate.length > 0 && ack && typed.trim() === confirmText && !applying && results.length === 0;

  async function runApply() {
    stopRef.current = false;
    setApplying(true);
    setStopMsg("");
    setResults([]);
    const acc: ApplyResult[] = [];
    try {
      setProgress({ done: 0, total: toCreate.length });
      for (let i = 0; i < toCreate.length && !stopRef.current; i += APPLY_BATCH) {
        const batch = toCreate.slice(i, i + APPLY_BATCH);
        const r = await apiFetch<{ results: ApplyResult[]; halted: boolean }>(`${BASE}/apply`, { method: "POST", json: { rows: batch, id_local_estoque: localId }, timeoutMs: 5 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + APPLY_BATCH, toCreate.length), total: toCreate.length });
        if (r.halted) {
          setStopMsg("EXECUÇÃO INTERROMPIDA — 3 falhas seguidas. Veja a mensagem de cada linha abaixo e o histórico antes de repetir.");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador. Reenviar o mesmo arquivo continua de onde parou: o que já foi criado é reconhecido pelo identificador.");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Reenvie o mesmo arquivo para continuar — o que já foi criado é reconhecido pelo identificador. Confira o histórico.`);
    } finally {
      setApplying(false);
    }
    const okN = acc.filter((r) => r.ok).length;
    notify({ ok: true }, null, `${okN} de ${acc.length} patrimônio(s) processado(s) sem problema.`);
  }

  // ---- downloads -----------------------------------------------------------------------------------------------
  const rowByLine = useMemo(() => new Map(apiRows.map((r) => [Number(r.line), r])), [apiRows]);
  const PEND_HEAD = ["motivo", "detalhe", "linha", "id_produto", "produto", "identificador_proprio", "numero_serie", "mac_address", "observacoes", "id_produto_item_existente"];
  function pendRow(motivo: string, detalhe: string, line: number, itemId = ""): string[] {
    const r = rowByLine.get(line);
    return [motivo, detalhe, String(line), String(r?.id_produto ?? ""), String(r?.produto_nome ?? ""), String(r?.identificador_proprio ?? ""), String(r?.numero_serie ?? ""), String(r?.mac_address ?? ""), String(r?.observacoes ?? ""), itemId];
  }
  function downloadPending() {
    const rows: string[][] = [];
    for (const v of validation?.rows ?? []) if (!v.valid) rows.push(pendRow("LINHA INVÁLIDA / REPETIDA NO ARQUIVO", (v.problems ?? []).join(" | "), v.line));
    for (const p of preflight ?? []) if (["ja_existe_divergente", "conflito_identificador", "conflito_serie", "conflito_mac", "retomar_invalido", "erro_produto"].includes(p.status)) rows.push(pendRow(PRE_LABEL[p.status], p.message, p.line, p.id_produto_item ?? ""));
    for (const r of results) if (r.action === "conflict" || r.pending || r.action === "failed" || r.action === "identify_failed" || r.action === "rejected_local") rows.push(pendRow(ACTION_LABEL[r.action], r.message, r.line, r.id_produto_item ?? ""));
    downloadCsv(`pendencias-patrimonios-${todayISO()}.csv`, PEND_HEAD, rows);
  }
  function downloadResults(onlyProblems: boolean) {
    const list = onlyProblems ? results.filter((r) => !r.ok || r.verified === false) : results;
    downloadCsv(
      `patrimonios-resultado-${onlyProblems ? "problemas" : "completo"}-${todayISO()}.csv`,
      ["linha", "patrimonio", "resultado", "mensagem", "id_produto_item", "codigo_item", "identificacao", "conferencia", "mac_nao_gravado", "detalhe_resposta_hubsoft"],
      list.map((r) => [
        String(r.line), r.label, ACTION_LABEL[r.action] ?? r.action, r.message, r.id_produto_item ?? "", r.codigo_item ?? "",
        r.identify_ok === undefined ? "" : `${r.identify_ok ? "OK" : "FALHOU"} — ${r.identify_message ?? ""}`,
        r.verified === undefined ? "" : `${r.verified ? "OK" : "DIFERENÇA"} — ${r.verify_message ?? ""}`,
        r.mac_omitted ? r.mac_omitted_reason ?? "sim" : "",
        r.detail ?? "",
      ]),
    );
  }
  function downloadTemplate() {
    const t = templateRow();
    downloadCsv("modelo-patrimonios-estoque-hubsoft.csv", t.head, [t.example]);
  }

  const okCount = results.filter((r) => r.ok && r.verified !== false).length;
  const pendCount = results.filter((r) => r.pending).length;
  const macOmittedCount = results.filter((r) => r.mac_omitted).length;
  const failCount = results.length - okCount - pendCount;
  const hasPending = (validation?.invalid ?? 0) > 0 || (preflight ?? []).some((p) => !CREATABLE.has(p.status) && p.status !== "ja_existe") || pendCount > 0;

  return (
    <ToolPanel
      icon={<Barcode size={20} />}
      title="Cadastro em massa de patrimônios de estoque"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Cria cada patrimônio (roteador, ONU, ONT, switch…) no local de estoque, com identificador, série e MAC. Antes de criar confere duplicidade em toda a HubSoft; série ou MAC repetidos viram pendência."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <div className="hsa-actions">
          <button type="button" className="btn btn--sm" onClick={downloadTemplate}>
            <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            Baixar modelo CSV de patrimônios
          </button>
          <span className="hsa-step__hint" style={{ margin: 0 }}>
            Uma linha por patrimônio. <b>id_produto</b> é o produto da HubSoft (já com controle patrimonial) e <b>identificador_proprio</b> é obrigatório. A coluna opcional <b>identificador_alternativo</b> (ex.: o Nº patrimônio do IXC) só serve para não duplicar: se um patrimônio já existir na HubSoft com esse identificador, a linha é «já existe».
          </span>
        </div>
        <Callout tone="info">
          <strong>Como cria:</strong> a API da HubSoft não cria patrimônio direto — ele nasce de uma <b>entrada de estoque</b> (1 unidade por linha, no local escolhido) e só então recebe
          identificador, série e MAC. Se a entrada der certo e a gravação falhar, o resultado traz o <b>id_produto_item</b>; reenvie a linha com a coluna <code>id_produto_item</code>
          preenchida para só identificá-lo (sem nova entrada).
        </Callout>
      </div>

      <Step n={1} title="Local de estoque e CSV" done={rawRows.length > 0 && !!localId} hint="O arquivo é lido no navegador; nada é enviado à HubSoft nesta etapa.">
        <div className="hsa-actions">
          <label className="hsa-check">
            Local de estoque
            <select className="input" style={{ marginLeft: 8, minWidth: 280 }} value={localId} onChange={(e) => setLocalId(e.target.value)} disabled={applying || locais.length === 0}>
              <option value="">{locais.length === 0 && !locaisErr ? "Carregando locais…" : "Selecione…"}</option>
              {locais.map((l) => (
                <option key={l.id_local_estoque} value={String(l.id_local_estoque)}>
                  {l.id_local_estoque} — {l.descricao}
                </option>
              ))}
            </select>
          </label>
        </div>
        {locaisErr ? <Callout tone="err">Não foi possível carregar os locais de estoque: {locaisErr}</Callout> : null}
        <CsvDropzone fileName={fileName} info={rawRows.length > 0 ? `${rawRows.length} linha(s) lida(s)` : undefined} disabled={validating || applying} onFile={(f) => void onFile(f)} onClear={reset} />
        <div className="hsa-actions">
          <span className="hsa-spacer" />
          <button type="button" className="btn btn--primary" disabled={validating || rawRows.length === 0 || !localId} onClick={() => void runValidate()}>
            <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {validating ? "Validando…" : "Validar"}
          </button>
        </div>
        {validating ? <ConsultaLoading text="Validando (conferindo os produtos na HubSoft)…" /> : null}
      </Step>

      {validation ? (
        <Step n={2} title="Resultado da validação" done={validation.invalid === 0} hint="Linhas inválidas (incluindo identificador, série ou MAC repetidos no arquivo) ficam de fora e vão para as pendências.">
          <div className="hsa-stats">
            <Stat label="Válidas" value={validation.valid} tone="ok" />
            <Stat label="Inválidas (ficam de fora)" value={validation.invalid} tone={validation.invalid ? "err" : "muted"} />
          </div>
          <div className="hsa-actions">
            <Segmented
              label="Filtrar linhas"
              value={filter}
              onChange={setFilter}
              options={[
                { value: "all", label: "Todas" },
                { value: "valid", label: "Válidas" },
                { value: "invalid", label: "Inválidas" },
              ]}
            />
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={!hasPending} onClick={downloadPending}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar pendências (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap" style={{ maxHeight: 300 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Patrimônio</th>
                  <th>Situação</th>
                </tr>
              </thead>
              <tbody>
                {shownRows.slice(0, 500).map((r) => (
                  <tr key={r.line}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td>{r.label || "—"}</td>
                    <td>{r.valid ? <Pill tone="ok">Válida</Pill> : <span style={{ color: "var(--err)" }}>{(r.problems ?? []).join(" | ")}</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {shownRows.length > 500 ? <span className="hsa-muted">Mostrando as 500 primeiras de {shownRows.length}. Baixe as pendências para ver todas as inválidas.</span> : null}
        </Step>
      ) : null}

      {validation && validAll.length > 0 ? (
        <Step n={3} title="Conferir duplicidade na HubSoft (obrigatório)" done={!!preflight && !preRunning} hint="Lê TODOS os patrimônios existentes (só leitura) e compara identificador, série e MAC em todos os produtos. Pode levar alguns minutos.">
          <div className="hsa-actions">
            <button type="button" className="btn btn--primary" disabled={preRunning || applying} onClick={() => void runPreflight()}>
              <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              {preRunning ? "Consultando…" : preflight ? "Conferir de novo" : "Conferir com a HubSoft"}
            </button>
          </div>
          {preRunning ? <ConsultaLoading text="Lendo os patrimônios da HubSoft produto a produto…" /> : null}
          {preErr ? <Callout tone="err">Não foi possível conferir: {preErr}</Callout> : null}
          {summary ? (
            <div className="hsa-stats">
              <Stat label="Patrimônios hoje na HubSoft" value={summary.patrimonios_na_hubsoft.toLocaleString("pt-BR")} tone="muted" />
              <Stat label="Sem identificação (sobras)" value={summary.sem_identificacao.toLocaleString("pt-BR")} tone={summary.sem_identificacao ? "warn" : "muted"} />
            </div>
          ) : null}
          {preflight ? (
            <>
              <div className="hsa-stats">
                <Stat label="Novos (serão criados)" value={(preCounts.novo ?? 0) + (preCounts.retomar ?? 0)} tone="ok" />
                <Stat label="Já existem" value={preCounts.ja_existe ?? 0} tone="muted" />
                <Stat label="Conflitos de série/MAC/identificador" value={(preCounts.conflito_serie ?? 0) + (preCounts.conflito_mac ?? 0) + (preCounts.conflito_identificador ?? 0)} tone={(preCounts.conflito_serie ?? 0) + (preCounts.conflito_mac ?? 0) + (preCounts.conflito_identificador ?? 0) ? "err" : "muted"} />
                <Stat label="Existem com dados diferentes / inválidos" value={(preCounts.ja_existe_divergente ?? 0) + (preCounts.retomar_invalido ?? 0) + (preCounts.erro_produto ?? 0)} tone={(preCounts.ja_existe_divergente ?? 0) + (preCounts.retomar_invalido ?? 0) + (preCounts.erro_produto ?? 0) ? "warn" : "muted"} />
              </div>
              {preflight.some((p) => !CREATABLE.has(p.status)) ? (
                <div className="hsa-table-wrap" style={{ maxHeight: 320 }}>
                  <table className="hsa-table">
                    <thead>
                      <tr>
                        <th>Linha</th>
                        <th>Patrimônio</th>
                        <th>O que acontece</th>
                      </tr>
                    </thead>
                    <tbody>
                      {preflight
                        .filter((p) => !CREATABLE.has(p.status))
                        .slice(0, 500)
                        .map((p) => (
                          <tr key={p.line}>
                            <td className="hsa-table__num mono">{p.line}</td>
                            <td>{p.label}</td>
                            <td>
                              <Pill tone={PRE_TONE[p.status]}>{PRE_LABEL[p.status]}</Pill>
                              <div className="hsa-muted" style={{ marginTop: 4 }}>{p.message}</div>
                            </td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                </div>
              ) : null}
            </>
          ) : null}
        </Step>
      ) : null}

      {preflight && toCreate.length > 0 ? (
        <Step n={4} title="Criar os patrimônios" done={results.length > 0 && !applying} hint="Esta etapa cria patrimônios reais no estoque da HubSoft.">
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a conferência e entendo que isto cria {toCreate.length} patrimônio(s) de verdade no local «{localNome || localId}». Linhas com conflito ficam de fora.
          </label>
          <div className="hsa-actions">
            <input className="input" style={{ width: 210 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
            <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void runApply()}>
              {applying ? "Criando…" : `Criar ${toCreate.length}`}
            </button>
            {applying ? (
              <button type="button" className="btn" onClick={() => { stopRef.current = true; }}>
                <Square size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
          </div>
          <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão. É lento de propósito (cada patrimônio é conferido); pode parar e retomar enviando o mesmo arquivo.</span>
          {applying ? <ProgressBar done={progress.done} total={progress.total} label={`Criando… ${progress.done} de ${progress.total}`} /> : null}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step n={5} title="Resultado" done={!applying && failCount === 0 && pendCount === 0} hint={'Cada linha também fica registrada na aba "Histórico" (Patrimônios de estoque).'}>
          <div className="hsa-stats">
            <Stat label="Sem problema" value={okCount} tone="ok" />
            <Stat label="Pendências (conflito)" value={pendCount} tone={pendCount ? "warn" : "muted"} />
            <Stat label="Salvos SEM o MAC (HubSoft recusou)" value={macOmittedCount} tone={macOmittedCount ? "warn" : "muted"} />
            <Stat label="Com falha" value={failCount} tone={failCount ? "err" : "muted"} />
          </div>
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          {results.some((r) => r.action === "identify_failed") ? (
            <Callout tone="warn">
              Há patrimônios <b>criados sem identificação</b> (veja o id_produto_item na tabela). Corrija a causa e reenvie essas linhas com a coluna <code>id_produto_item</code> preenchida.
            </Callout>
          ) : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={() => downloadResults(false)}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Resultado completo (CSV)
            </button>
            <button type="button" className="btn btn--sm" disabled={okCount === results.length} onClick={() => downloadResults(true)}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Só os problemas (CSV)
            </button>
            <button type="button" className="btn btn--sm" disabled={pendCount + failCount === 0} onClick={downloadPending}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Pendências (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Patrimônio</th>
                  <th>Resultado</th>
                  <th>Identificação / conferência</th>
                  <th>ID</th>
                </tr>
              </thead>
              <tbody>
                {results.map((r, i) => (
                  <tr key={`${r.line}-${i}`}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td>{r.label || "—"}</td>
                    <td>
                      <Pill tone={ACTION_TONE[r.action]}>{ACTION_LABEL[r.action] ?? r.action}</Pill>
                      <div className="hsa-muted" style={{ marginTop: 4, color: r.ok ? undefined : "var(--err)" }}>{r.message}</div>
                      {r.detail ? (
                        <details style={{ marginTop: 4 }}>
                          <summary style={{ cursor: "pointer", fontSize: 11 }}>Resposta bruta da HubSoft</summary>
                          <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word", margin: "4px 0 0" }}>{r.detail}</pre>
                        </details>
                      ) : null}
                    </td>
                    <td className="hsa-muted">
                      {r.mac_omitted ? <div style={{ color: "var(--warn)" }}>MAC não gravado — {r.mac_omitted_reason}</div> : null}
                      {r.identify_ok !== undefined ? <div style={{ color: r.identify_ok ? "var(--ok)" : "var(--err)" }}>Identificação: {r.identify_ok ? "gravada" : "FALHOU"} — {r.identify_message}</div> : null}
                      {r.verified !== undefined ? <div style={{ color: r.verified ? "var(--ok)" : "var(--err)" }}>Conferência: {r.verified ? "OK" : "diferença"} — {r.verify_message}</div> : null}
                    </td>
                    <td className="mono">
                      {r.id_produto_item || "—"}
                      {r.codigo_item ? <div className="hsa-muted">cód. {r.codigo_item}</div> : null}
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
