import { useMemo, useRef, useState } from "react";
import { Download, ListChecks, Package, Square } from "lucide-react";
import { Callout, CsvDropzone, Pill, ProgressBar, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { PRODUCT_FIELDS, PRODUCT_OPTIONAL, absentColumns, pickField } from "./hubsoftStockFields";

const SLUG = "hubsoft";
const BASE = `/api/v1/integrations/${SLUG}/hubsoft/stock-products`;
const APPLY_BATCH = 5; // cada produto faz 4 chamadas à HubSoft (checar, criar, configurar, conferir) — lotes pequenos
const MAX_ROWS = 500;

/**
 * Cadastro em massa de PRODUTOS de estoque (passo 1 da migração de patrimônios). O CSV traz um produto por linha, com a
 * categoria, a marca, os valores e a CONFIGURAÇÃO do produto (NF, venda, comodato, vínculos). Fluxo: enviar CSV → validar
 * (formato + IDs de categoria/marca/tipo reais) → conferir com a HubSoft (o que já existe) → criar. Cada produto é criado,
 * configurado e conferido; as três etapas aparecem separadas no resultado e tudo fica no histórico.
 */

const FIELDS = PRODUCT_FIELDS;

type RowValidation = { line: number; valid: boolean; problems?: string[]; label?: string };
type ValidationResp = { ok: boolean; rows: RowValidation[]; valid: number; invalid: number; unchecked_catalogs?: string[] };
type PreRow = { line: number; label: string; status: "novo" | "ja_existe_codigo" | "ja_existe_nome"; message: string; id_produto?: string };
type ApplyResult = {
  line: number;
  label: string;
  ok: boolean;
  action: "created" | "already_exists" | "rejected_local" | "failed" | "config_failed" | "brand_repaired" | "brand_repair_failed";
  message: string;
  id_produto?: string;
  created?: boolean;
  rejected?: boolean;
  config_ok?: boolean;
  config_message?: string;
  verified?: boolean;
  verify_message?: string;
  detail?: string;
  brand_repair?: string;
};

const ACTION_LABEL: Record<ApplyResult["action"], string> = {
  created: "Criado e configurado",
  already_exists: "Já existia — nada foi alterado",
  rejected_local: "Recusado antes de enviar",
  failed: "Recusado pela HubSoft",
  config_failed: "Criado, mas a configuração falhou",
  brand_repaired: "Marca corrigida",
  brand_repair_failed: "Marca NÃO corrigida",
};
const ACTION_TONE: Record<ApplyResult["action"], "ok" | "err" | "warn" | undefined> = {
  created: "ok",
  already_exists: undefined,
  rejected_local: "err",
  failed: "err",
  config_failed: "warn",
  brand_repaired: "ok",
  brand_repair_failed: "err",
};
const PRE_LABEL: Record<PreRow["status"], string> = { novo: "Novo — será criado", ja_existe_codigo: "Já existe (mesmo código)", ja_existe_nome: "Já existe (mesmo nome)" };

function templateRow(): { head: string[]; example: string[] } {
  const ex: Record<string, string> = {
    codigo: "123",
    nome: "ROTEADOR EXEMPLO AX1500",
    id_categoria: "7",
    id_marca: "5",
    id_tipo: "",
    unidade_medida: "UN",
    controle_patrimonial: "sim",
    epi: "não",
    valor_compra: "150,00",
    valor_venda: "199,90",
    estoque_minimo: "0",
    incluir_nota_fiscal: "sim",
    permite_venda_cliente: "sim",
    permite_comodato_cliente: "sim",
    permite_vinculo_pop: "sim",
    permite_vinculo_projeto_mapeamento: "sim",
    permite_vinculo_usuario: "sim",
    permite_vinculo_composicao: "não",
  };
  return { head: FIELDS.map((f) => f.key), example: FIELDS.map((f) => ex[f.key] ?? "") };
}

export function HubsoftStockProducts() {
  const { notify, missing } = useConsultaToast();
  const [fileName, setFileName] = useState("");
  const [rawRows, setRawRows] = useState<Record<string, string>[]>([]);
  const [checkCatalogs, setCheckCatalogs] = useState(true);

  const [validating, setValidating] = useState(false);
  const [validation, setValidation] = useState<ValidationResp | null>(null);
  const [filter, setFilter] = useState<"all" | "valid" | "invalid">("all");

  const [preRunning, setPreRunning] = useState(false);
  const [preflight, setPreflight] = useState<PreRow[] | null>(null);
  const [preErr, setPreErr] = useState("");

  const [repairBrand, setRepairBrand] = useState(false);
  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<ApplyResult[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  function reset() {
    setRawRows([]);
    setFileName("");
    setValidation(null);
    setPreflight(null);
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
    // O arquivo precisa ter as colunas do modelo — evita subir por engano outro CSV (ex.: o mapa de produtos IXC → HubSoft).
    const absent = absentColumns(objs[0], FIELDS, PRODUCT_OPTIONAL);
    if (absent.length > 0) {
      setFileName("");
      return missing(
        `este arquivo não parece ser um CSV de produtos — faltam as colunas: ${absent.join(", ")}. Use o modelo (botão "Baixar modelo CSV de produtos") ou o arquivo 11-IMPORTAR-no-NetQuasar-produtos-39.csv.`,
      );
    }
    if (objs.length > MAX_ROWS) return missing(`no máximo ${MAX_ROWS} linhas por importação (o arquivo tem ${objs.length}).`);
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

  async function runValidate() {
    if (rawRows.length === 0) return missing("envie um CSV primeiro.");
    setValidating(true);
    setValidation(null);
    setPreflight(null);
    setResults([]);
    setAck(false);
    setTyped("");
    try {
      const resp = await apiFetch<ValidationResp>(`${BASE}/validate`, { method: "POST", json: { rows: apiRows, check_catalogs: checkCatalogs }, timeoutMs: 90_000 });
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

  // Só vão para a criação as válidas que a conferência não marcou como «já existe» (se ela foi feita).
  const existingLines = useMemo(() => new Set((preflight ?? []).filter((p) => p.status !== "novo").map((p) => p.line)), [preflight]);
  const toCreate = useMemo(() => validAll.filter((r) => repairBrand || !existingLines.has(Number(r.line))), [validAll, existingLines, repairBrand]);

  async function runPreflight() {
    setPreRunning(true);
    setPreErr("");
    setPreflight(null);
    try {
      const r = await apiFetch<{ results: PreRow[] }>(`${BASE}/preflight`, { method: "POST", json: { rows: validAll }, timeoutMs: 120_000 });
      setPreflight(r.results);
    } catch (e) {
      setPreErr((e as Error).message);
    } finally {
      setPreRunning(false);
    }
  }

  const shownRows = useMemo(() => {
    if (!validation) return [];
    return filter === "all" ? validation.rows : validation.rows.filter((r) => (filter === "valid" ? r.valid : !r.valid));
  }, [validation, filter]);

  const confirmText = `CRIAR ${toCreate.length}`;
  const canApply = !!validation && toCreate.length > 0 && ack && typed.trim() === confirmText && !applying && results.length === 0;

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
        const r = await apiFetch<{ results: ApplyResult[]; halted: boolean }>(`${BASE}/apply`, { method: "POST", json: { rows: batch, repair_brand: repairBrand }, timeoutMs: 4 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + APPLY_BATCH, toCreate.length), total: toCreate.length });
        if (r.halted) {
          setStopMsg("EXECUÇÃO INTERROMPIDA — 3 falhas seguidas. Veja a mensagem de cada linha abaixo e o histórico antes de repetir.");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador.");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Confira o histórico antes de repetir — alguns produtos podem ter sido criados.`);
    } finally {
      setApplying(false);
    }
    const okN = acc.filter((r) => r.ok).length;
    notify({ ok: true }, null, `${okN} de ${acc.length} produto(s) processado(s) sem problema.`);
  }

  function downloadInvalid() {
    if (!validation) return;
    downloadCsv(
      `produtos-linhas-invalidas-${todayISO()}.csv`,
      ["linha", "produto", "motivo"],
      validation.rows.filter((r) => !r.valid).map((r) => [String(r.line), r.label ?? "", (r.problems ?? []).join(" | ")]),
    );
  }

  function downloadResults(onlyProblems: boolean) {
    const list = onlyProblems ? results.filter((r) => !r.ok || r.config_ok === false || r.verified === false) : results;
    downloadCsv(
      `produtos-resultado-${onlyProblems ? "problemas" : "completo"}-${todayISO()}.csv`,
      ["linha", "produto", "resultado", "mensagem", "id_produto", "configuracao", "conferencia", "detalhe_resposta_hubsoft"],
      list.map((r) => [
        String(r.line), r.label, ACTION_LABEL[r.action] ?? r.action, r.message, r.id_produto ?? "",
        r.config_ok === undefined ? "" : `${r.config_ok ? "OK" : "FALHOU"} — ${r.config_message ?? ""}`,
        r.verified === undefined ? "" : `${r.verified ? "OK" : "DIFERENÇA"} — ${r.verify_message ?? ""}`,
        r.detail ?? "",
      ]),
    );
  }

  function downloadTemplate() {
    const t = templateRow();
    downloadCsv("modelo-produtos-estoque-hubsoft.csv", t.head, [t.example]);
  }

  const okCount = results.filter((r) => r.ok && r.config_ok !== false && r.verified !== false).length;
  const problemCount = results.length - okCount;
  const preCounts = useMemo(() => {
    const c = { novo: 0, existe: 0 };
    for (const p of preflight ?? []) p.status === "novo" ? c.novo++ : c.existe++;
    return c;
  }, [preflight]);

  return (
    <ToolPanel
      icon={<Package size={20} />}
      title="Cadastro em massa de produtos de estoque"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Cria produtos (roteadores, ONUs, ONTs, switches etc.) na HubSoft a partir de um CSV, já com a configuração de cada um (NF, venda, comodato, vínculos). Cada produto é criado, configurado e conferido."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <div className="hsa-actions">
          <button type="button" className="btn btn--sm" onClick={downloadTemplate}>
            <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            Baixar modelo CSV de produtos
          </button>
          <span className="hsa-step__hint" style={{ margin: 0 }}>
            Os IDs de categoria, marca e tipo estão na aba Catálogos. Sim/não nas colunas de configuração. O campo <b>codigo</b> do produto recebe o ID do IXC.
          </span>
        </div>
        <Callout tone="info">
          <strong>Segurança:</strong> um produto que já existe (mesmo <b>código</b> ou mesmo <b>nome</b>) nunca é criado de novo nem alterado. Se a criação der certo mas a
          configuração falhar, o resultado diz isso com o id do produto criado — nada fica «meio feito» sem aviso.
        </Callout>
      </div>

      <Step n={1} title="Enviar o CSV" done={rawRows.length > 0} hint="O arquivo é lido no navegador; nada é enviado à HubSoft nesta etapa.">
        <CsvDropzone fileName={fileName} info={rawRows.length > 0 ? `${rawRows.length} linha(s) lida(s)` : undefined} disabled={validating || applying} onFile={(f) => void onFile(f)} onClear={reset} />
        <div className="hsa-actions">
          <label className="hsa-check">
            <input type="checkbox" checked={checkCatalogs} onChange={(e) => setCheckCatalogs(e.target.checked)} />
            Conferir categoria, marca e tipo contra a HubSoft (recomendado)
          </label>
          <span className="hsa-spacer" />
          <button type="button" className="btn btn--primary" disabled={validating || rawRows.length === 0} onClick={() => void runValidate()}>
            <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {validating ? "Validando…" : "Validar"}
          </button>
        </div>
        {validating ? <ConsultaLoading text="Validando (conferindo com os catálogos da HubSoft)…" /> : null}
      </Step>

      {validation ? (
        <Step n={2} title="Resultado da validação" done={validation.invalid === 0} hint="Linhas inválidas ficam de fora da criação.">
          {validation.unchecked_catalogs && validation.unchecked_catalogs.length > 0 ? (
            <Callout tone="warn">
              Não foi possível conferir na HubSoft: <b>{validation.unchecked_catalogs.join(", ")}</b>. Esses IDs passaram só na checagem de formato — confira na aba Catálogos.
            </Callout>
          ) : null}
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
            <button type="button" className="btn btn--sm" disabled={validation.invalid === 0} onClick={downloadInvalid}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar linhas inválidas (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap" style={{ maxHeight: 320 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Produto</th>
                  <th>Situação</th>
                </tr>
              </thead>
              <tbody>
                {shownRows.map((r) => (
                  <tr key={r.line}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td>{r.label || "—"}</td>
                    <td>{r.valid ? <Pill tone="ok">Válida</Pill> : <span style={{ color: "var(--err)" }}>{(r.problems ?? []).join(" | ")}</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Step>
      ) : null}

      {validation && validAll.length > 0 ? (
        <Step n={3} title="Conferir com a HubSoft antes de criar (recomendado)" done={!!preflight && !preRunning} hint="Lê os produtos já cadastrados (só leitura) e mostra quais linhas seriam criadas e quais já existem.">
          <div className="hsa-actions">
            <button type="button" className="btn btn--primary" disabled={preRunning || applying} onClick={() => void runPreflight()}>
              <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              {preRunning ? "Consultando…" : preflight ? "Conferir de novo" : "Conferir com a HubSoft"}
            </button>
          </div>
          {preRunning ? <ConsultaLoading text="Lendo os produtos da HubSoft…" /> : null}
          {preErr ? <Callout tone="err">Não foi possível conferir: {preErr}</Callout> : null}
          {preflight ? (
            <>
              <div className="hsa-stats">
                <Stat label="Novos (serão criados)" value={preCounts.novo} tone="ok" />
                <Stat label="Já existem (ficam de fora)" value={preCounts.existe} tone={preCounts.existe ? "warn" : "muted"} />
              </div>
              {preCounts.existe > 0 ? (
                <div className="hsa-table-wrap" style={{ maxHeight: 260 }}>
                  <table className="hsa-table">
                    <thead>
                      <tr>
                        <th>Linha</th>
                        <th>Produto</th>
                        <th>O que acontece</th>
                      </tr>
                    </thead>
                    <tbody>
                      {preflight
                        .filter((p) => p.status !== "novo")
                        .map((p) => (
                          <tr key={p.line}>
                            <td className="hsa-table__num mono">{p.line}</td>
                            <td>{p.label}</td>
                            <td>
                              <Pill tone="warn">{PRE_LABEL[p.status]}</Pill>
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

      {validation && toCreate.length > 0 ? (
        <Step n={4} title="Criar os produtos" done={results.length > 0 && !applying} hint="Esta etapa cria cadastros reais na HubSoft.">
          <label className="hsa-check">
            <input type="checkbox" checked={repairBrand} onChange={(e) => setRepairBrand(e.target.checked)} disabled={applying || results.length > 0} />
            Também tentar <b>corrigir a marca</b> dos produtos que já existem com o mesmo código (só altera a marca, e só se estiver diferente da pedida)
          </label>
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a validação e entendo que isto cria {toCreate.length} produto(s) de verdade na HubSoft{preflight ? "" : " (a conferência com a HubSoft não foi feita — produtos que já existirem são pulados na hora)"}.
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
          <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão.</span>
          {applying ? <ProgressBar done={progress.done} total={progress.total} label={`Criando… ${progress.done} de ${progress.total}`} /> : null}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step n={5} title="Resultado" done={!applying && problemCount === 0} hint={'Cada linha também fica registrada na aba "Histórico" (Produtos de estoque).'}>
          <div className="hsa-stats">
            <Stat label="Sem problema" value={okCount} tone="ok" />
            <Stat label="Com problema" value={problemCount} tone={problemCount ? "err" : "muted"} />
          </div>
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={() => downloadResults(false)}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar resultado completo (CSV)
            </button>
            <button type="button" className="btn btn--sm" disabled={problemCount === 0} onClick={() => downloadResults(true)}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar só os problemas (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Produto</th>
                  <th>Resultado</th>
                  <th>Configuração / conferência</th>
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
                      {r.brand_repair ? <div className="hsa-muted" style={{ marginTop: 4 }}>Marca: {r.brand_repair}</div> : null}
                      {r.detail ? (
                        <details style={{ marginTop: 4 }}>
                          <summary style={{ cursor: "pointer", fontSize: 11 }}>Resposta bruta da HubSoft</summary>
                          <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word", margin: "4px 0 0" }}>{r.detail}</pre>
                        </details>
                      ) : null}
                    </td>
                    <td className="hsa-muted">
                      {r.config_ok !== undefined ? <div style={{ color: r.config_ok ? "var(--ok)" : "var(--err)" }}>Configuração: {r.config_ok ? "gravada" : "FALHOU"} — {r.config_message}</div> : null}
                      {r.verified !== undefined ? <div style={{ color: r.verified ? "var(--ok)" : "var(--err)" }}>Conferência: {r.verified ? "OK" : "diferença"} — {r.verify_message}</div> : null}
                    </td>
                    <td className="mono">{r.id_produto || "—"}</td>
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
