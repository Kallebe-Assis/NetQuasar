import { useMemo, useRef, useState } from "react";
import { Download, ListChecks, Square, UserPlus } from "lucide-react";
import { Callout, CsvDropzone, Pill, ProgressBar, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { CLIENT_FIELDS, SERVICE_FIELDS, pick, templateFor } from "./hubsoftImportFields";

const SLUG = "hubsoft";
const BASE = `/api/v1/integrations/${SLUG}/hubsoft/bulk-import`;
const APPLY_BATCH = 15;
const MAX_ROWS = 2000;

/**
 * Importação em massa de clientes/serviços a partir de um CSV
 * (o formato é o dos arquivos "hubsoft-clientes-principal.csv" / "hubsoft-servicos-adicionais.csv").
 * Fluxo: enviar CSV → validar (formato + IDs de catálogo reais, sem alterar nada) → linhas com
 * problema ficam de fora, só as válidas seguem → aplicar em lotes pequenos, parando ao primeiro
 * sinal de problema sistémico → cada linha aplicada fica gravada no histórico (aba "Histórico").
 */

type Kind = "client" | "service";

type RowValidation = { line: number; valid: boolean; problems?: string[]; label?: string; info?: string[] };
type ValidationResp = {
  ok: boolean;
  message?: string;
  rows: RowValidation[];
  valid: number;
  invalid: number;
  unchecked_catalogs?: string[];
};
type ApplyResult = {
  line: number;
  ok: boolean;
  message: string;
  id_cliente?: string;
  id_cliente_servico?: string;
  rejected?: boolean;
  login_ok?: boolean;
  login_message?: string;
  label?: string;
  dedup_action?: "cliente_criado" | "servico_adicionado" | "ja_existe" | "login_a_corrigir" | "login_em_uso";
};

type PreflightRow = {
  line: number;
  label: string;
  status: "novo" | "existe_mesmo_endereco" | "existe_outro_endereco" | "ja_importado" | "login_em_uso" | "login_padrao_a_corrigir" | "erro";
  message: string;
  id_cliente?: string;
  csv_address?: string;
  hubsoft_addresses?: string[];
  existing_logins?: string[];
};

const PRE_LABEL: Record<PreflightRow["status"], string> = {
  novo: "Cliente novo",
  existe_mesmo_endereco: "Já existe (mesmo endereço)",
  existe_outro_endereco: "Já existe — OUTRO endereço",
  ja_importado: "Login já cadastrado",
  login_em_uso: "Login em uso em outro cadastro",
  login_padrao_a_corrigir: "Só corrigir login",
  erro: "Erro na consulta",
};
const PRE_TONE: Record<PreflightRow["status"], "ok" | "err" | "warn" | undefined> = {
  novo: "ok",
  existe_mesmo_endereco: undefined,
  existe_outro_endereco: "warn",
  ja_importado: undefined,
  login_em_uso: "err",
  login_padrao_a_corrigir: undefined,
  erro: "err",
};

const DEDUP_ACTION_LABELS: Record<string, string> = {
  cliente_criado: "Cliente novo criado",
  servico_adicionado: "Cliente já existia — serviço novo adicionado",
  ja_existe: "Já existia (mesmo login) — nada foi criado",
  login_a_corrigir: "Serviço já existia com o login padrão — nada criado, só o login foi corrigido",
  login_em_uso: "Não cadastrado — o login já existe na HubSoft",
};

export function HubsoftBulkImportSection() {
  const { notify, missing } = useConsultaToast();
  const [kind, setKind] = useState<Kind>("client");
  const [fileName, setFileName] = useState("");
  const [rawRows, setRawRows] = useState<Record<string, string>[]>([]);
  const [checkCatalogs, setCheckCatalogs] = useState(true);

  const [validating, setValidating] = useState(false);
  const [validation, setValidation] = useState<ValidationResp | null>(null);
  const [filter, setFilter] = useState<"all" | "valid" | "invalid">("all");

  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<ApplyResult[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  // Pré-conferência (só clientes novos): consulta a HubSoft ANTES de importar e diz o que cada linha vai fazer.
  const [preRunning, setPreRunning] = useState(false);
  const [preProgress, setPreProgress] = useState({ done: 0, total: 0 });
  const [preflight, setPreflight] = useState<PreflightRow[] | null>(null);
  const [skipOtherAddr, setSkipOtherAddr] = useState(false); // por padrão NÃO pula: login diferente = serviço novo legítimo
  const [preErr, setPreErr] = useState("");

  const fields = kind === "client" ? CLIENT_FIELDS : SERVICE_FIELDS;

  function reset() {
    setRawRows([]);
    setFileName("");
    setValidation(null);
    setResults([]);
    setStopMsg("");
    setAck(false);
    setTyped("");
    setPreflight(null);
    setPreErr("");
  }

  async function onFile(f: File | undefined) {
    reset();
    if (!f) return;
    setFileName(f.name);
    const objs = csvRowsToObjects(parseCsv(await f.text()));
    if (objs.length === 0) {
      missing("o CSV não tem linhas de dados.");
      return;
    }
    if (objs.length > MAX_ROWS) {
      missing(`no máximo ${MAX_ROWS} linhas por importação (o arquivo tem ${objs.length}).`);
      return;
    }
    setRawRows(objs);
  }

  function buildApiRows() {
    return rawRows.map((r, i) => {
      const o: Record<string, string> = { line: String(i + 2) };
      for (const f of fields) o[f] = pick(r, f);
      return o;
    });
  }

  async function runValidate() {
    if (rawRows.length === 0) return missing("envie um CSV primeiro.");
    setValidating(true);
    setValidation(null);
    setResults([]);
    setAck(false);
    setTyped("");
    try {
      const rows = buildApiRows().map((r) => ({ ...r, line: Number(r.line) }));
      const resp = await apiFetch<ValidationResp>(`${BASE}/validate`, {
        method: "POST",
        json: { kind, rows, check_catalogs: checkCatalogs },
        timeoutMs: 90_000,
      });
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
    const okLines = new Set(validation.rows.filter((r) => r.valid).map((r) => r.line));
    return buildApiRows()
      .filter((r) => okLines.has(Number(r.line)))
      .map((r) => ({ ...r, line: Number(r.line) }));
  }, [validation, rawRows, kind]); // eslint-disable-line react-hooks/exhaustive-deps

  // Linhas que realmente serão importadas: as válidas, menos (opcional) as de cliente que já existe com outro endereço.
  const skippedLines = useMemo(() => {
    if (!preflight || !skipOtherAddr) return new Set<number>();
    return new Set(preflight.filter((p) => p.status === "existe_outro_endereco").map((p) => p.line));
  }, [preflight, skipOtherAddr]);
  const validRows = useMemo(() => validAll.filter((r) => !skippedLines.has(Number(r.line))), [validAll, skippedLines]);

  const preCounts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const p of preflight ?? []) c[p.status] = (c[p.status] ?? 0) + 1;
    return c;
  }, [preflight]);

  async function runPreflight() {
    stopRef.current = false;
    setPreRunning(true);
    setPreErr("");
    setPreflight(null);
    const acc: PreflightRow[] = [];
    setPreProgress({ done: 0, total: validAll.length });
    try {
      for (let i = 0; i < validAll.length && !stopRef.current; i += 10) {
        const batch = validAll.slice(i, i + 10);
        const r = await apiFetch<{ results: PreflightRow[] }>(`${BASE}/preflight`, {
          method: "POST",
          json: { kind: "client", rows: batch },
          timeoutMs: 4 * 60_000,
        });
        acc.push(...r.results);
        setPreflight([...acc]);
        setPreProgress({ done: Math.min(i + 10, validAll.length), total: validAll.length });
      }
      if (stopRef.current) setPreErr("Pré-conferência interrompida — o resultado é parcial.");
    } catch (e) {
      setPreErr(`Interrompido por erro de comunicação: ${(e as Error).message}`);
    } finally {
      setPreRunning(false);
    }
  }

  const shownRows = useMemo(() => {
    if (!validation) return [];
    if (filter === "all") return validation.rows;
    return validation.rows.filter((r) => (filter === "valid" ? r.valid : !r.valid));
  }, [validation, filter]);

  function downloadInvalid() {
    if (!validation) return;
    const head = ["linha", "identificação", "motivo"];
    const rows = validation.rows
      .filter((r) => !r.valid)
      .map((r) => [String(r.line), r.label ?? "", (r.problems ?? []).join(" | ")]);
    downloadCsv(`linhas-invalidas-${kind}-${new Date().toISOString().slice(0, 10)}.csv`, head, rows);
  }

  const confirmText = `IMPORTAR ${validRows.length}`;
  const canApply = validation && validRows.length > 0 && ack && typed.trim() === confirmText && !applying && results.length === 0;

  async function runApply() {
    stopRef.current = false;
    setApplying(true);
    setStopMsg("");
    setResults([]);
    const acc: ApplyResult[] = [];
    try {
      setProgress({ done: 0, total: validRows.length });
      for (let i = 0; i < validRows.length && !stopRef.current; i += APPLY_BATCH) {
        const batch = validRows.slice(i, i + APPLY_BATCH);
        const r = await apiFetch<{ results: ApplyResult[]; halted: boolean }>(`${BASE}/apply`, {
          method: "POST",
          json: { kind, rows: batch },
          timeoutMs: 3 * 60_000,
        });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + APPLY_BATCH, validRows.length), total: validRows.length });
        if (r.halted) {
          setStopMsg("EXECUÇÃO INTERROMPIDA — 3 falhas seguidas na HubSoft. Confira o histórico antes de repetir.");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador.");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Confira o histórico antes de repetir.`);
    } finally {
      setApplying(false);
    }
    const okN = acc.filter((r) => r.ok).length;
    notify({ ok: true }, null, `${okN} de ${acc.length} linha(s) importada(s) com sucesso.`);
  }

  function downloadFailed() {
    if (results.length === 0) return;
    const head = ["linha", "cliente", "resultado", "mensagem", "id_cliente", "id_cliente_servico", "login"];
    const rows = results
      .filter((r) => !r.ok || r.login_ok === false)
      .map((r) => [
        String(r.line),
        r.label ?? "",
        r.ok ? "OK (cliente criado)" : r.rejected ? "Recusada (não enviada)" : "Erro da HubSoft",
        r.message,
        r.id_cliente ?? "",
        r.id_cliente_servico ?? "",
        r.login_ok === false ? `falhou — ${r.login_message}` : "",
      ]);
    downloadCsv(`falhas-importacao-${kind}-${new Date().toISOString().slice(0, 10)}.csv`, head, rows);
  }

  function downloadTemplate(k: Kind) {
    const t = templateFor(k);
    downloadCsv(k === "client" ? "modelo-clientes-novos-hubsoft.csv" : "modelo-servicos-adicionais-hubsoft.csv", t.head, [t.example]);
  }

  const okCount = results.filter((r) => r.ok).length;
  const loginFailCount = results.filter((r) => r.login_ok === false).length;

  return (
    <ToolPanel
      icon={<UserPlus size={20} />}
      title="Importação em massa de clientes"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Cria clientes e serviços de verdade na HubSoft a partir de um CSV. Cada linha é conferida antes de aplicar; as com problema ficam de fora."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <Segmented
          label="Tipo de importação"
          value={kind}
          onChange={(k) => {
            setKind(k);
            reset();
          }}
          options={[
            { value: "client", label: "Clientes novos" },
            { value: "service", label: "Serviços adicionais (cliente já existe)" },
          ]}
        />
        <p className="hsa-step__hint" style={{ margin: 0 }}>
          {kind === "client"
            ? "Um cliente novo por linha — formato de hubsoft-clientes-principal.csv."
            : "Um serviço adicional por linha, para um id_cliente que já existe — formato de hubsoft-servicos-adicionais.csv."}
        </p>
        <div className="hsa-actions">
          <button type="button" className="btn btn--sm" onClick={() => downloadTemplate(kind)}>
            <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {kind === "client" ? "Baixar modelo CSV de clientes novos" : "Baixar modelo CSV de serviços adicionais"}
          </button>
          <span className="hsa-step__hint" style={{ margin: 0 }}>
            Já vem com os cabeçalhos certos e uma linha de exemplo (CPF inválido de propósito — apague-a antes de importar).
          </span>
        </div>
        <Callout tone="info">
          <strong>Login e senha PPPoE:</strong> a API da HubSoft não cria a autenticação do serviço, só altera uma que já existe. Mantenha ativa na
          HubSoft a automação que gera logins automaticamente (máscara de login): o serviço nasce com um login padrão e o NetQuasar o troca pelo do
          CSV logo depois. Login e senha diferenciam maiúsculas de minúsculas.
        </Callout>
      </div>

      <Step n={1} title="Enviar o CSV" done={rawRows.length > 0} hint="O arquivo é lido no navegador; nada é enviado à HubSoft nesta etapa.">
        <CsvDropzone
          fileName={fileName}
          info={rawRows.length > 0 ? `${rawRows.length} linha(s) lida(s)` : undefined}
          disabled={validating || applying}
          onFile={(f) => void onFile(f)}
          onClear={reset}
        />
        <div className="hsa-actions">
          <label className="hsa-check">
            <input type="checkbox" checked={checkCatalogs} onChange={(e) => setCheckCatalogs(e.target.checked)} />
            Conferir os IDs contra os catálogos reais (recomendado)
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
        <Step n={2} title="Resultado da validação" done={validation.invalid === 0} hint="Linhas inválidas ficam de fora da importação.">
          {validation.unchecked_catalogs && validation.unchecked_catalogs.length > 0 ? (
            <Callout tone="warn">
              Não foi possível conferir estes catálogos contra a HubSoft: <b>{validation.unchecked_catalogs.join(", ")}</b>. Os IDs desses campos
              passaram só na checagem de formato — confira manualmente antes de importar, ou valide de novo.
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
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Identificação</th>
                  <th>Situação</th>
                </tr>
              </thead>
              <tbody>
                {shownRows.map((r) => (
                  <tr key={r.line}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td>{r.label || "—"}</td>
                    <td>
                      {r.valid ? <Pill tone="ok">Válida</Pill> : <span style={{ color: "var(--err)" }}>{(r.problems ?? []).join(" | ")}</span>}
                      {r.info && r.info.length > 0 ? <div className="hsa-muted">{r.info.join(" | ")}</div> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Step>
      ) : null}

      {kind === "client" && validation && validAll.length > 0 ? (
        <Step
          n={3}
          title="Conferir com a HubSoft antes de importar (recomendado)"
          done={!!preflight && !preRunning}
          hint="Consulta a HubSoft (só leitura) e mostra o que cada linha vai fazer: criar cliente novo ou adicionar serviço a um cliente que já existe — e se o endereço é diferente do cadastrado."
        >
          <div className="hsa-actions">
            <button type="button" className="btn btn--primary" disabled={preRunning || applying} onClick={() => void runPreflight()}>
              <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              {preRunning ? "Consultando…" : preflight ? "Conferir de novo" : "Conferir com a HubSoft"}
            </button>
            {preRunning ? (
              <button
                type="button"
                className="btn"
                onClick={() => {
                  stopRef.current = true;
                }}
              >
                <Square size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
          </div>
          {preRunning ? <ProgressBar done={preProgress.done} total={preProgress.total} label={`Consultando… ${preProgress.done} de ${preProgress.total}`} /> : null}
          {preErr ? <Callout tone="warn">{preErr}</Callout> : null}
          {preflight ? (
            <>
              <div className="hsa-stats">
                <Stat label="Clientes novos" value={preCounts.novo ?? 0} tone="ok" />
                <Stat label="Já existem (mesmo endereço)" value={preCounts.existe_mesmo_endereco ?? 0} tone="muted" />
                <Stat label="Já existem — outro endereço" value={preCounts.existe_outro_endereco ?? 0} tone={preCounts.existe_outro_endereco ? "warn" : "muted"} />
                <Stat label="Login já cadastrado / em uso" value={(preCounts.ja_importado ?? 0) + (preCounts.login_em_uso ?? 0)} tone={preCounts.login_em_uso ? "err" : "muted"} />
              </div>
              {(preCounts.existe_outro_endereco ?? 0) > 0 ? (
                <Callout tone="warn">
                  <strong>{preCounts.existe_outro_endereco} cliente(s) já existem na HubSoft com endereço diferente do CSV.</strong> Como o login da linha é
                  diferente dos já cadastrados, é um serviço novo desse cliente: ele será criado no endereço do CSV e vinculado ao cadastro existente. Confira a
                  tabela abaixo — se algum for o mesmo ponto com endereço desatualizado, ou um homônimo, marque a opção para deixá-lo de fora e tratar à mão.
                  <label className="hsa-check" style={{ display: "flex", marginTop: 8 }}>
                    <input type="checkbox" checked={skipOtherAddr} onChange={(e) => setSkipOtherAddr(e.target.checked)} />
                    Não importar estas {preCounts.existe_outro_endereco} linhas agora (deixar para revisão manual)
                  </label>
                </Callout>
              ) : null}
              <div className="hsa-table-wrap" style={{ maxHeight: 360 }}>
                <table className="hsa-table">
                  <thead>
                    <tr>
                      <th>Linha</th>
                      <th>Cliente</th>
                      <th>O que vai acontecer</th>
                      <th>Endereço no CSV × na HubSoft</th>
                    </tr>
                  </thead>
                  <tbody>
                    {preflight
                      .filter((p) => p.status !== "novo")
                      .map((p) => (
                        <tr key={p.line}>
                          <td className="hsa-table__num mono">{p.line}</td>
                          <td>
                            {p.label}
                            {p.id_cliente ? <div className="hsa-muted">id_cliente {p.id_cliente}</div> : null}
                          </td>
                          <td>
                            <Pill tone={PRE_TONE[p.status]}>{PRE_LABEL[p.status]}</Pill>
                            <div className="hsa-muted" style={{ marginTop: 4 }}>{p.message}</div>
                          </td>
                          <td className="hsa-muted">
                            {p.csv_address ? <div>CSV: {p.csv_address}</div> : null}
                            {(p.hubsoft_addresses ?? []).map((a) => (
                              <div key={a}>HubSoft: {a}</div>
                            ))}
                          </td>
                        </tr>
                      ))}
                    {preflight.every((p) => p.status === "novo") ? (
                      <tr>
                        <td colSpan={4} className="hsa-muted" style={{ textAlign: "center", padding: 16 }}>
                          Todas as linhas conferidas são clientes novos.
                        </td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </>
          ) : null}
        </Step>
      ) : null}

      {validation && validRows.length > 0 ? (
        <Step n={kind === "client" ? 4 : 3} title="Importar" done={results.length > 0 && !applying} hint="Esta etapa cria cadastros reais na HubSoft.">
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a validação e entendo que isto cria {validRows.length} {kind === "client" ? "cliente(s) novo(s)" : "serviço(s) adicional(is)"} de verdade
            na HubSoft.
          </label>
          <div className="hsa-actions">
            <input className="input" style={{ width: 210 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
            <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void runApply()}>
              {applying ? "Importando…" : `Importar ${validRows.length}`}
            </button>
            {applying ? (
              <button
                type="button"
                className="btn"
                onClick={() => {
                  stopRef.current = true;
                }}
              >
                <Square size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
          </div>
          <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão.</span>
          {applying ? <ProgressBar done={progress.done} total={progress.total} label={`Importando… ${progress.done} de ${progress.total}`} /> : null}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step
          n={(validRows.length > 0 ? 4 : 3) + (kind === "client" && validAll.length > 0 ? 1 : 0)}
          title="Resultado da importação"
          done={!applying && okCount === results.length && loginFailCount === 0}
          hint={'Cada linha também fica registrada na aba "Histórico".'}
        >
          <div className="hsa-stats">
            <Stat label="OK" value={okCount} tone="ok" />
            <Stat label="Com problema" value={results.length - okCount} tone={results.length - okCount ? "err" : "muted"} />
            <Stat label="Login não configurado" value={loginFailCount} tone={loginFailCount ? "warn" : "muted"} />
          </div>
          {applying ? <span className="hsa-muted">Processando… {results.length}/{validRows.length}</span> : null}
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={okCount === results.length && loginFailCount === 0} onClick={downloadFailed}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar linhas com problema (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap">
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Cliente</th>
                  <th>Resultado</th>
                  <th>ID</th>
                </tr>
              </thead>
              <tbody>
                {results.map((r, i) => (
                  <tr key={`${r.line}-${i}`}>
                    <td className="hsa-table__num mono">{r.line}</td>
                    <td>{r.label || "—"}</td>
                    <td>
                      <div style={{ color: r.ok ? "var(--ok)" : r.dedup_action === "login_em_uso" ? "var(--warn)" : "var(--err)" }}>
                        {r.ok ? "OK" : r.dedup_action === "login_em_uso" ? "Não cadastrado" : r.rejected ? "Recusada (não enviada)" : "Erro"} — {r.message}
                      </div>
                      {r.dedup_action ? (
                        <div className="hsa-muted" style={r.dedup_action === "ja_existe" || r.dedup_action === "login_em_uso" ? { color: "var(--warn)" } : undefined}>
                          {DEDUP_ACTION_LABELS[r.dedup_action] ?? r.dedup_action}
                        </div>
                      ) : null}
                      {r.login_ok !== undefined ? (
                        <div className="hsa-muted" style={{ color: r.login_ok ? "var(--ok)" : "var(--err)" }}>
                          Login: {r.login_ok ? "OK" : "falhou"} — {r.login_message}
                        </div>
                      ) : null}
                    </td>
                    <td className="mono">{r.id_cliente || r.id_cliente_servico || "—"}</td>
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
