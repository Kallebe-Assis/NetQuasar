import { Fragment, useMemo, useRef, useState } from "react";
import { ClipboardCheck, Download, Play, Square } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { useConsultaToast } from "./hubsoftConsulta";
import { CLIENT_FIELDS, MAX_CSV_ROWS, SERVICE_FIELDS, buildApiRows, type ImportKind } from "./hubsoftImportFields";
import { Callout, CsvDropzone, Pill, ProgressBar, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";

/**
 * Conferência de cadastros (SOMENTE LEITURA, só administradores). Recebe o MESMO CSV da importação
 * (clientes novos ou serviços adicionais) e confere, linha a linha, se o que está na HubSoft bate com o
 * que o arquivo diz. Nada é alterado na HubSoft.
 */

const BASE = "/api/v1/integrations/hubsoft/hubsoft/registration-check";
const BATCH = 10;
const OBS_URL = "/api/v1/integrations/hubsoft/hubsoft/login-observation";
const OBS_PREFIX = "Login PPPoE configurado no cliente:";

type ObsPending = { line: number; label: string; id_cliente_servico: string; login: string };
type ObsResult = { id_cliente_servico: string; login: string; ok: boolean; skipped: boolean; message: string };

type CheckItem = {
  field: string;
  label: string;
  group: "cliente" | "endereco" | "servico" | "acesso";
  expected: string;
  found: string;
  status: "ok" | "diff" | "unverified";
  note?: string;
};
type RowCheck = {
  line: number;
  label: string;
  status: "ok" | "divergente" | "nao_encontrado" | "ambiguo" | "erro";
  message?: string;
  id_cliente?: string;
  id_cliente_servico?: string;
  checks?: CheckItem[];
  ok_count: number;
  diff_count: number;
  unverified_count: number;
};
type CheckResp = { results: RowCheck[]; catalogs_missing?: string[] };

const GROUP_LABEL: Record<CheckItem["group"], string> = {
  cliente: "Cliente",
  endereco: "Endereço",
  servico: "Serviço",
  acesso: "Acesso PPPoE",
};
const GROUPS: CheckItem["group"][] = ["cliente", "endereco", "servico", "acesso"];

const STATUS_LABEL: Record<RowCheck["status"], string> = {
  ok: "Conferido — tudo certo",
  divergente: "Divergente",
  nao_encontrado: "Não encontrado",
  ambiguo: "Ambíguo",
  erro: "Erro na consulta",
};
const STATUS_TONE: Record<RowCheck["status"], "ok" | "err" | "warn" | undefined> = {
  ok: "ok",
  divergente: "err",
  nao_encontrado: "err",
  ambiguo: "warn",
  erro: "warn",
};

type Filter = "all" | "problems" | "ok";
type CheckMode = "completa" | "especifica";

const MODE_HINT: Record<CheckMode, string> = {
  especifica: "Confere só: nome, CPF/CNPJ, login, senha, valor do plano e velocidade do plano.",
  completa: "Confere todos os campos: dados do cliente, endereço, plano, status, vendedor, interface, valor, data da venda, carnê, login e senha.",
};

function MarkCell({ s }: { s: CheckItem["status"] }) {
  if (s === "ok") return <span className="hsa-checks__mark hsa-checks__mark--ok">✓ igual</span>;
  if (s === "diff") return <span className="hsa-checks__mark hsa-checks__mark--diff">✗ diferente</span>;
  return <span className="hsa-checks__mark hsa-checks__mark--unverified">— não verificável</span>;
}

function CheckDetail({ row, onlyDiff }: { row: RowCheck; onlyDiff: boolean }) {
  const checks = (row.checks ?? []).filter((c) => !onlyDiff || c.status !== "ok");
  return (
    <div className="hsa-detail">
      {row.message ? (
        <div style={{ padding: "8px 10px" }}>
          <Callout tone={row.status === "ok" ? "info" : "warn"}>{row.message}</Callout>
        </div>
      ) : null}
      {checks.length === 0 ? (
        <div className="hsa-muted" style={{ padding: "8px 12px" }}>
          {(row.checks ?? []).length === 0 ? "Sem campos para comparar." : "Nenhuma divergência — todos os campos conferidos estão iguais."}
        </div>
      ) : (
        <table className="hsa-checks">
          <thead>
            <tr>
              <th style={{ width: "18%" }}>Campo</th>
              <th style={{ width: "28%" }}>Arquivo (esperado)</th>
              <th style={{ width: "28%" }}>HubSoft (encontrado)</th>
              <th style={{ width: "26%" }}>Resultado</th>
            </tr>
          </thead>
          <tbody>
            {GROUPS.map((g) => {
              const items = checks.filter((c) => c.group === g);
              if (items.length === 0) return null;
              return (
                <Fragment key={g}>
                  <tr className="is-group">
                    <td colSpan={4}>{GROUP_LABEL[g]}</td>
                  </tr>
                  {items.map((c) => (
                    <tr key={c.field} className={c.status === "diff" ? "is-diff" : undefined}>
                      <td>{c.label}</td>
                      <td className="mono">{c.expected || "—"}</td>
                      <td className="mono">{c.found || "—"}</td>
                      <td>
                        <MarkCell s={c.status} />
                        {c.note ? <div className="hsa-muted">{c.note}</div> : null}
                      </td>
                    </tr>
                  ))}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

export function HubsoftRegistrationCheck({ fixedKind }: { fixedKind?: ImportKind } = {}) {
  const { notify, missing } = useConsultaToast();
  // fixedKind: quando a aba «Conferência» já escolheu clientes ou serviços, não repete o seletor
  const [kind, setKind] = useState<ImportKind>(fixedKind ?? "client");
  const [mode, setMode] = useState<CheckMode>("especifica");
  const [fileName, setFileName] = useState("");
  const [rawRows, setRawRows] = useState<Record<string, string>[]>([]);
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<RowCheck[]>([]);
  const [catalogsMissing, setCatalogsMissing] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState<Set<number>>(new Set());
  const [onlyDiff, setOnlyDiff] = useState(true);
  const stopRef = useRef(false);

  // Registro do login original nas Observações da autenticação (a HubSoft padroniza o login em minúsculas).
  const [obsAck, setObsAck] = useState(false);
  const [obsRunning, setObsRunning] = useState(false);
  const [obsProgress, setObsProgress] = useState({ done: 0, total: 0 });
  const [obsResults, setObsResults] = useState<ObsResult[]>([]);
  const [obsErr, setObsErr] = useState("");

  const fields = kind === "client" ? CLIENT_FIELDS : SERVICE_FIELDS;

  function reset() {
    setRawRows([]);
    setFileName("");
    setResults([]);
    setErr("");
    setCatalogsMissing([]);
    setOpen(new Set());
    setProgress({ done: 0, total: 0 });
    setObsAck(false);
    setObsResults([]);
    setObsErr("");
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
    if (objs.length > MAX_CSV_ROWS) {
      missing(`no máximo ${MAX_CSV_ROWS} linhas por conferência (o arquivo tem ${objs.length}).`);
      return;
    }
    setRawRows(objs);
  }

  async function run() {
    const rows = buildApiRows(rawRows, fields);
    stopRef.current = false;
    setRunning(true);
    setErr("");
    setResults([]);
    setOpen(new Set());
    setProgress({ done: 0, total: rows.length });
    const acc: RowCheck[] = [];
    try {
      for (let i = 0; i < rows.length && !stopRef.current; i += BATCH) {
        const batch = rows.slice(i, i + BATCH);
        const r = await apiFetch<CheckResp>(BASE, { method: "POST", json: { kind, mode, rows: batch }, timeoutMs: 4 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + BATCH, rows.length), total: rows.length });
        if (r.catalogs_missing?.length) setCatalogsMissing(r.catalogs_missing);
      }
      if (stopRef.current) setErr("Conferência interrompida pelo operador — o resultado abaixo é parcial.");
    } catch (e) {
      setErr(`Interrompido por erro de comunicação: ${(e as Error).message}. O resultado abaixo é parcial.`);
    } finally {
      setRunning(false);
    }
    const bad = acc.filter((r) => r.status !== "ok").length;
    notify({ ok: true }, null, `${acc.length} linha(s) conferida(s): ${acc.length - bad} certa(s), ${bad} com divergência ou não encontrada(s).`);
  }

  const counts = useMemo(() => {
    const c = { ok: 0, divergente: 0, nao_encontrado: 0, outros: 0, unverified: 0 };
    for (const r of results) {
      if (r.status === "ok") c.ok++;
      else if (r.status === "divergente") c.divergente++;
      else if (r.status === "nao_encontrado") c.nao_encontrado++;
      else c.outros++;
      c.unverified += r.unverified_count;
    }
    return c;
  }, [results]);

  const shown = useMemo(() => {
    const q = search.trim().toLowerCase();
    return results.filter((r) => {
      if (filter === "ok" && r.status !== "ok") return false;
      if (filter === "problems" && r.status === "ok") return false;
      if (!q) return true;
      return (
        r.label.toLowerCase().includes(q) ||
        String(r.line) === q ||
        (r.id_cliente ?? "") === q ||
        (r.id_cliente_servico ?? "") === q ||
        (r.checks ?? []).some((c) => (c.field === "login" || c.field === "cpf_cnpj") && (c.expected.toLowerCase().includes(q) || c.found.toLowerCase().includes(q)))
      );
    });
  }, [results, filter, search]);

  // Serviços cujo login a HubSoft gravou só em minúsculas e que ainda não têm o login original nas Observações.
  const obsPending = useMemo<ObsPending[]>(() => {
    const out: ObsPending[] = [];
    for (const r of results) {
      const c = (r.checks ?? []).find((x) => x.field === "observacoes_login" && x.status === "diff");
      if (c && r.id_cliente_servico) {
        out.push({ line: r.line, label: r.label, id_cliente_servico: r.id_cliente_servico, login: c.expected.slice(OBS_PREFIX.length).trim() });
      }
    }
    return out;
  }, [results]);

  async function runObs() {
    setObsRunning(true);
    setObsErr("");
    setObsResults([]);
    const acc: ObsResult[] = [];
    setObsProgress({ done: 0, total: obsPending.length });
    try {
      for (let i = 0; i < obsPending.length; i += 20) {
        const batch = obsPending.slice(i, i + 20).map((p) => ({ id_cliente_servico: p.id_cliente_servico, login: p.login }));
        const r = await apiFetch<{ results: ObsResult[] }>(OBS_URL, { method: "POST", json: { rows: batch }, timeoutMs: 4 * 60_000 });
        acc.push(...r.results);
        setObsResults([...acc]);
        setObsProgress({ done: Math.min(i + 20, obsPending.length), total: obsPending.length });
      }
    } catch (e) {
      setObsErr(`Interrompido por erro de comunicação: ${(e as Error).message}. Confira o resultado abaixo e rode a conferência de novo.`);
    } finally {
      setObsRunning(false);
    }
    const okN = acc.filter((r) => r.ok && !r.skipped).length;
    notify({ ok: true }, null, `${okN} de ${acc.length} login(s) original(is) registrado(s) nas Observações.`);
  }

  function toggle(line: number) {
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(line)) next.delete(line);
      else next.add(line);
      return next;
    });
  }

  function downloadDivergences() {
    const head = ["linha", "cliente", "situacao", "id_cliente", "id_cliente_servico", "campo", "esperado_no_arquivo", "encontrado_na_hubsoft", "observacao"];
    const rows: string[][] = [];
    for (const r of results) {
      if (r.status === "ok") continue;
      const diffs = (r.checks ?? []).filter((c) => c.status === "diff");
      if (diffs.length === 0) {
        rows.push([String(r.line), r.label, STATUS_LABEL[r.status], r.id_cliente ?? "", r.id_cliente_servico ?? "", "", "", "", r.message ?? ""]);
      }
      for (const c of diffs) {
        rows.push([String(r.line), r.label, STATUS_LABEL[r.status], r.id_cliente ?? "", r.id_cliente_servico ?? "", c.label, c.expected, c.found, c.note ?? ""]);
      }
    }
    downloadCsv(`conferencia-${mode}-${kind}-${new Date().toISOString().slice(0, 10)}.csv`, head, rows);
  }

  const problems = counts.divergente + counts.nao_encontrado + counts.outros;
  const finished = !running && results.length > 0 && progress.done >= progress.total;

  return (
    <ToolPanel
      icon={<ClipboardCheck size={20} />}
      title={fixedKind === "service" ? "Conferência de serviços" : fixedKind === "client" ? "Conferência do cadastro de clientes" : "Conferência de cadastros"}
      badge="Somente leitura"
      badgeTone="ok"
      subtitle="Envie o mesmo CSV da importação (ou qualquer lista de clientes no mesmo formato): o sistema consulta a HubSoft e confere, campo a campo, se cada cadastro está igual ao arquivo."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        {fixedKind ? null : (
        <Segmented
          label="Tipo de arquivo"
          value={kind}
          onChange={(k) => {
            setKind(k);
            reset();
          }}
          options={[
            { value: "client", label: "Clientes (principal)" },
            { value: "service", label: "Serviços adicionais" },
          ]}
        />
        )}
        <Segmented
          label="Tipo de conferência"
          value={mode}
          onChange={(m) => {
            setMode(m);
            setResults([]);
            setErr("");
          }}
          options={[
            { value: "especifica", label: "Específica" },
            { value: "completa", label: "Completa" },
          ]}
        />
        <p className="hsa-step__hint" style={{ margin: 0 }}>
          {MODE_HINT[mode]}
        </p>
        <Callout tone="info">
          Nada é alterado na HubSoft. Textos são comparados sem diferenciar maiúsculas, acentos e espaços; <strong>login e senha são exatos</strong>{" "}
          (maiúsculas contam). Dados que a consulta da HubSoft não devolve (por exemplo vencimento e forma de cobrança) aparecem como
          “não verificável” — nunca como certo nem como errado.
        </Callout>
      </div>

      <Step n={1} title="Enviar o arquivo" done={rawRows.length > 0} hint="O arquivo é lido no navegador; cada linha é consultada na HubSoft em lotes pequenos.">
        <CsvDropzone
          fileName={fileName}
          info={rawRows.length > 0 ? `${rawRows.length} linha(s) para conferir` : undefined}
          disabled={running}
          onFile={(f) => void onFile(f)}
          onClear={reset}
        />
        <div className="hsa-actions">
          <span className="hsa-spacer" />
          {running ? (
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
          <button type="button" className="btn btn--primary" disabled={running || rawRows.length === 0} onClick={() => void run()}>
            <Play size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {running ? "Conferindo…" : "Conferir cadastros"}
          </button>
        </div>
        {running ? <ProgressBar done={progress.done} total={progress.total} label={`Conferindo… ${progress.done} de ${progress.total}`} /> : null}
        {err ? <Callout tone="warn">{err}</Callout> : null}
      </Step>

      {results.length > 0 ? (
        <Step n={2} title="Resultado da conferência" done={finished && problems === 0} hint="Clique em “Ver campos” para comparar arquivo × HubSoft.">
          {catalogsMissing.length > 0 ? (
            <Callout tone="warn">
              Não foi possível ler estes catálogos da HubSoft: <b>{catalogsMissing.join(", ")}</b>. Os campos que dependem deles (plano, status,
              vendedor, interface) aparecem como “não verificável”.
            </Callout>
          ) : null}
          <div className="hsa-stats">
            <Stat label="Conferidos" value={results.length} tone="muted" />
            <Stat label="Tudo certo" value={counts.ok} tone="ok" />
            <Stat label="Divergentes" value={counts.divergente} tone={counts.divergente ? "err" : "muted"} />
            <Stat label="Não encontrados" value={counts.nao_encontrado} tone={counts.nao_encontrado ? "err" : "muted"} />
            <Stat label="Ambíguos / erro" value={counts.outros} tone={counts.outros ? "warn" : "muted"} />
            <Stat label="Campos não verificáveis" value={counts.unverified} tone="muted" />
          </div>
          <div className="hsa-actions">
            <Segmented
              label="Filtrar linhas"
              value={filter}
              onChange={setFilter}
              options={[
                { value: "all", label: "Todas" },
                { value: "problems", label: "Com problema" },
                { value: "ok", label: "Certas" },
              ]}
            />
            <input
              className="input"
              style={{ width: 230 }}
              placeholder="Buscar nome, login, CPF, linha…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              aria-label="Buscar nos resultados"
            />
            <label className="hsa-check">
              <input type="checkbox" checked={onlyDiff} onChange={(e) => setOnlyDiff(e.target.checked)} />
              Mostrar só campos diferentes
            </label>
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={problems === 0} onClick={downloadDivergences}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar divergências (CSV)
            </button>
          </div>
          {obsPending.length > 0 || obsResults.length > 0 ? (
            <Callout tone="info">
              <strong>
                {obsPending.length > 0
                  ? `${obsPending.length} serviço(s) com o login gravado em minúsculas pela HubSoft e sem o login original nas Observações.`
                  : "Registro do login original concluído."}
              </strong>{" "}
              O Radius da HubSoft não diferencia maiúsculas, então isso não causa problema hoje; o registro serve para um futuro ERP que diferencie. O texto
              gravado nas <em>Observações da autenticação</em> é <span className="mono">“{OBS_PREFIX} &lt;login original&gt;”</span> — observações já escritas
              são preservadas.
              {obsPending.length > 0 ? (
                <div className="hsa-actions" style={{ marginTop: 8 }}>
                  <label className="hsa-check">
                    <input type="checkbox" checked={obsAck} onChange={(e) => setObsAck(e.target.checked)} disabled={obsRunning} />
                    Entendo que isto altera as Observações de {obsPending.length} serviço(s) na HubSoft.
                  </label>
                  <button type="button" className="btn btn--primary" disabled={!obsAck || obsRunning} onClick={() => void runObs()}>
                    {obsRunning ? "Registrando…" : `Registrar ${obsPending.length}`}
                  </button>
                </div>
              ) : null}
              {obsRunning ? <ProgressBar done={obsProgress.done} total={obsProgress.total} label={`Registrando… ${obsProgress.done} de ${obsProgress.total}`} /> : null}
              {obsErr ? <div style={{ marginTop: 6, color: "var(--err)" }}>{obsErr}</div> : null}
              {obsResults.length > 0 ? (
                <div className="hsa-muted" style={{ marginTop: 6 }}>
                  {obsResults.filter((r) => r.ok && !r.skipped).length} registrado(s)
                  {obsResults.some((r) => r.skipped) ? `, ${obsResults.filter((r) => r.skipped).length} sem nada a fazer` : ""}
                  {obsResults.some((r) => !r.ok) ? (
                    <span style={{ color: "var(--err)" }}>
                      , {obsResults.filter((r) => !r.ok).length} com erro: {obsResults.filter((r) => !r.ok).slice(0, 3).map((r) => `${r.login} — ${r.message}`).join(" | ")}
                    </span>
                  ) : null}
                  . Rode “Conferir cadastros” de novo para confirmar.
                </div>
              ) : null}
            </Callout>
          ) : null}
          <div className="hsa-table-wrap" style={{ maxHeight: 620 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Cadastro</th>
                  <th>Situação</th>
                  <th>Campos</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {shown.map((r) => {
                  const isOpen = open.has(r.line);
                  return (
                    <Fragment key={r.line}>
                      <tr>
                        <td className="hsa-table__num mono">{r.line}</td>
                        <td>
                          {r.label || "—"}
                          {r.id_cliente ? <div className="hsa-muted">id_cliente {r.id_cliente}{r.id_cliente_servico ? ` · serviço ${r.id_cliente_servico}` : ""}</div> : null}
                        </td>
                        <td>
                          <Pill tone={STATUS_TONE[r.status]}>{STATUS_LABEL[r.status]}</Pill>
                          {r.status !== "ok" && r.message ? <div className="hsa-muted" style={{ marginTop: 4 }}>{r.message}</div> : null}
                        </td>
                        <td className="hsa-muted">
                          {(r.checks ?? []).length > 0 ? (
                            <>
                              <span style={{ color: "var(--ok)" }}>{r.ok_count} iguais</span>
                              {r.diff_count > 0 ? <span style={{ color: "var(--err)" }}> · {r.diff_count} diferentes</span> : null}
                              {r.unverified_count > 0 ? <> · {r.unverified_count} não verificáveis</> : null}
                            </>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td>
                          {(r.checks ?? []).length > 0 || r.message ? (
                            <button type="button" className="hsa-table__toggle" aria-expanded={isOpen} onClick={() => toggle(r.line)}>
                              {isOpen ? "Ocultar" : "Ver campos"}
                            </button>
                          ) : null}
                        </td>
                      </tr>
                      {isOpen ? (
                        <tr>
                          <td colSpan={5} style={{ padding: 0 }}>
                            <CheckDetail row={r} onlyDiff={onlyDiff} />
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  );
                })}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="hsa-muted" style={{ textAlign: "center", padding: 18 }}>
                      Nenhuma linha neste filtro.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </Step>
      ) : null}
    </ToolPanel>
  );
}
