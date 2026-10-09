import { useEffect, useMemo, useState } from "react";
import { ClipboardCheck, Download, Play } from "lucide-react";
import { Callout, CsvDropzone, Pill, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { downloadCsv, parseCsv, csvRowsToObjects } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { ITEM_FIELDS, PRODUCT_FIELDS, PRODUCT_OPTIONAL, absentColumns, pickField, type StockField } from "./hubsoftStockFields";

const SLUG = "hubsoft";
const BASE = `/api/v1/integrations/${SLUG}/hubsoft`;
const CATALOG = `${BASE}/catalog/fetch`;

/**
 * Conferência (SOMENTE LEITURA) de produtos e patrimônios de estoque: compara o CSV — o mesmo formato da importação — com o que
 * está na HubSoft, campo a campo. Nada é alterado. Resultado por linha: igual, divergente (com as diferenças), não encontrado
 * ou duplicado; dá para filtrar, buscar e baixar em CSV.
 */

type Diff = { campo: string; arquivo: string; hubsoft: string };
type CheckRow = {
  line: number;
  label: string;
  status: "ok" | "divergente" | "nao_encontrado" | "duplicado";
  id?: string;
  diffs?: Diff[];
  note?: string;
  aviso?: string;
  extras?: Record<string, string>;
};
type Summary = { patrimonios_na_hubsoft: number; produtos_patrimoniais: number; sem_identificacao: number };
type LocalOpt = { id_local_estoque: number; descricao: string };

const STATUS_LABEL: Record<CheckRow["status"], string> = { ok: "Igual à HubSoft", divergente: "Divergente", nao_encontrado: "Não encontrado", duplicado: "Duplicado na HubSoft" };
const STATUS_TONE: Record<CheckRow["status"], "ok" | "err" | "warn" | undefined> = { ok: "ok", divergente: "warn", nao_encontrado: "err", duplicado: "err" };
type Filter = "all" | CheckRow["status"];

type Cfg = {
  kind: "produtos" | "patrimonios";
  title: string;
  subtitle: string;
  fields: StockField[];
  optional?: Set<string>;
  endpoint: string;
  idHeader: string;
  unit: string;
};

const CONFIGS: Record<Cfg["kind"], Cfg> = {
  produtos: {
    kind: "produtos",
    title: "Conferência de produtos de estoque",
    subtitle: "Compara cada produto do CSV com o cadastro na HubSoft: nome, código, categoria, marca, tipo, valores, controle patrimonial, EPI e unidade.",
    fields: PRODUCT_FIELDS,
    optional: PRODUCT_OPTIONAL,
    endpoint: `${BASE}/stock-products/check`,
    idHeader: "id_produto_hubsoft",
    unit: "produto(s)",
  },
  patrimonios: {
    kind: "patrimonios",
    title: "Conferência de patrimônios de estoque",
    subtitle: "Compara cada patrimônio do CSV com o que está na HubSoft: produto, identificador, série, MAC, local de estoque e observações. Mostra também onde o patrimônio está (status e cliente).",
    fields: ITEM_FIELDS,
    endpoint: `${BASE}/stock-items/check`,
    idHeader: "id_produto_item",
    unit: "patrimônio(s)",
  },
};

export function HubsoftStockCheck({ kind }: { kind: Cfg["kind"] }) {
  const cfg = CONFIGS[kind];
  const { notify, missing } = useConsultaToast();
  const [fileName, setFileName] = useState("");
  const [rawRows, setRawRows] = useState<Record<string, string>[]>([]);
  const [locais, setLocais] = useState<LocalOpt[]>([]);
  const [localId, setLocalId] = useState(""); // vazio = não conferir o local
  const [running, setRunning] = useState(false);
  const [err, setErr] = useState("");
  const [results, setResults] = useState<CheckRow[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState<Set<number>>(new Set());

  useEffect(() => {
    if (kind !== "patrimonios") return;
    let alive = true;
    apiFetch<{ locais_estoque: LocalOpt[] }>(`${CATALOG}?which=estoque_local`, { timeoutMs: 60_000 })
      .then((d) => {
        if (!alive) return;
        setLocais(d.locais_estoque ?? []);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [kind]);

  function reset() {
    setRawRows([]);
    setFileName("");
    setResults([]);
    setSummary(null);
    setErr("");
    setOpen(new Set());
  }

  async function onFile(f: File | undefined) {
    reset();
    if (!f) return;
    setFileName(f.name);
    const objs = csvRowsToObjects(parseCsv(await f.text()));
    if (objs.length === 0) return missing("o CSV não tem linhas de dados.");
    const absent = absentColumns(objs[0], cfg.fields, cfg.optional);
    if (absent.length > 0) {
      setFileName("");
      return missing(`este arquivo não parece ser um CSV de ${kind === "produtos" ? "produtos" : "patrimônios"} — faltam as colunas: ${absent.join(", ")}.`);
    }
    setRawRows(objs);
  }

  const apiRows = useMemo(
    () =>
      rawRows.map((r, i) => {
        const o: Record<string, string | number> = { line: i + 2 };
        for (const f of cfg.fields) o[f.key] = pickField(r, f.aliases);
        return o;
      }),
    [rawRows, cfg.fields],
  );

  async function run() {
    setRunning(true);
    setErr("");
    setResults([]);
    setSummary(null);
    try {
      const body: Record<string, unknown> = { rows: apiRows };
      if (kind === "patrimonios") body.id_local_estoque = localId;
      const r = await apiFetch<{ results: CheckRow[]; summary?: Summary }>(cfg.endpoint, { method: "POST", json: body, timeoutMs: 13 * 60_000 });
      setResults(r.results);
      setSummary(r.summary ?? null);
      setFilter(r.results.some((x) => x.status !== "ok") ? "all" : "all");
      notify({ ok: true }, null, `Conferência concluída — ${r.results.length} ${cfg.unit}.`);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  const counts = useMemo(() => {
    const c = { ok: 0, divergente: 0, nao_encontrado: 0, duplicado: 0 };
    for (const r of results) c[r.status]++;
    return c;
  }, [results]);

  const shown = useMemo(() => {
    const q = search.trim().toLowerCase();
    return results.filter((r) => (filter === "all" || r.status === filter) && (!q || `${r.label} ${r.id ?? ""} ${r.note ?? ""}`.toLowerCase().includes(q)));
  }, [results, filter, search]);

  function download() {
    downloadCsv(
      `conferencia-${kind}-${todayISO()}.csv`,
      ["linha", "item", "situacao", cfg.idHeader, "diferencas (campo: arquivo × hubsoft)", "observacao", "onde_esta"],
      shown.map((r) => [
        String(r.line), r.label, STATUS_LABEL[r.status], r.id ?? "",
        (r.diffs ?? []).map((d) => `${d.campo}: «${d.arquivo}» × «${d.hubsoft}»`).join(" | "),
        r.note ?? "",
        Object.entries(r.extras ?? {}).map(([k, v]) => `${k}: ${v}`).join(" | "),
      ]),
    );
  }

  const aviso = results.find((r) => r.aviso)?.aviso;

  return (
    <ToolPanel icon={<ClipboardCheck size={20} />} title={cfg.title} badge="Somente leitura" badgeTone="ok" subtitle={cfg.subtitle}>
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <Callout tone="info">
          Nada é alterado na HubSoft. Use o mesmo CSV da importação (ou qualquer lista no mesmo formato). Textos são comparados sem diferenciar maiúsculas, acentos e pontuação.
        </Callout>
      </div>

      <Step n={1} title="Enviar o arquivo" done={rawRows.length > 0} hint="O arquivo é lido no navegador; a consulta à HubSoft só começa ao clicar em «Conferir».">
        {kind === "patrimonios" ? (
          <div className="hsa-actions">
            <label className="hsa-check">
              Conferir o local de estoque
              <select className="input" style={{ marginLeft: 8, minWidth: 280 }} value={localId} onChange={(e) => setLocalId(e.target.value)} disabled={running}>
                <option value="">Não conferir o local</option>
                {locais.map((l) => (
                  <option key={l.id_local_estoque} value={String(l.id_local_estoque)}>
                    {l.id_local_estoque} — {l.descricao}
                  </option>
                ))}
              </select>
            </label>
          </div>
        ) : null}
        <CsvDropzone fileName={fileName} info={rawRows.length > 0 ? `${rawRows.length} linha(s) lida(s)` : undefined} disabled={running} onFile={(f) => void onFile(f)} onClear={reset} />
        <div className="hsa-actions">
          <span className="hsa-spacer" />
          <button type="button" className="btn btn--primary" disabled={running || rawRows.length === 0} onClick={() => void run()}>
            <Play size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {running ? "Conferindo…" : "Conferir"}
          </button>
        </div>
        {running ? <ConsultaLoading text={kind === "patrimonios" ? "Lendo os patrimônios da HubSoft produto a produto (pode levar alguns minutos)…" : "Lendo os produtos da HubSoft…"} /> : null}
        {err ? <Callout tone="err">{err}</Callout> : null}
      </Step>

      {results.length > 0 ? (
        <Step n={2} title="Resultado da conferência" done={counts.divergente + counts.nao_encontrado + counts.duplicado === 0} hint="Clique em uma linha com diferenças para ver arquivo × HubSoft.">
          {aviso ? <Callout tone="warn">{aviso}</Callout> : null}
          <div className="hsa-stats">
            <Stat label="Iguais" value={counts.ok} tone="ok" />
            <Stat label="Divergentes" value={counts.divergente} tone={counts.divergente ? "warn" : "muted"} />
            <Stat label="Não encontrados" value={counts.nao_encontrado} tone={counts.nao_encontrado ? "err" : "muted"} />
            <Stat label="Duplicados" value={counts.duplicado} tone={counts.duplicado ? "err" : "muted"} />
            {summary ? <Stat label="Patrimônios hoje na HubSoft" value={summary.patrimonios_na_hubsoft.toLocaleString("pt-BR")} tone="muted" /> : null}
          </div>
          <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
            <Segmented
              label="Filtrar"
              value={filter}
              onChange={setFilter}
              options={[
                { value: "all", label: "Todos" },
                { value: "ok", label: "Iguais" },
                { value: "divergente", label: "Divergentes" },
                { value: "nao_encontrado", label: "Não encontrados" },
                { value: "duplicado", label: "Duplicados" },
              ]}
            />
            <input type="search" className="input" style={{ width: 220 }} placeholder="Buscar no resultado" value={search} onChange={(e) => setSearch(e.target.value)} />
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" disabled={shown.length === 0} onClick={download}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Baixar (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap" style={{ maxHeight: 560 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Item</th>
                  <th>Situação</th>
                  <th>ID</th>
                </tr>
              </thead>
              <tbody>
                {shown.slice(0, 1000).map((r) => {
                  const expandable = (r.diffs?.length ?? 0) > 0 || !!r.extras;
                  const isOpen = open.has(r.line);
                  return (
                    <tr
                      key={r.line}
                      style={{ cursor: expandable ? "pointer" : undefined }}
                      onClick={() => {
                        if (!expandable) return;
                        setOpen((prev) => {
                          const n = new Set(prev);
                          if (n.has(r.line)) n.delete(r.line);
                          else n.add(r.line);
                          return n;
                        });
                      }}
                    >
                      <td className="hsa-table__num mono">{r.line}</td>
                      <td>{r.label || "—"}</td>
                      <td>
                        <Pill tone={STATUS_TONE[r.status]}>{STATUS_LABEL[r.status]}</Pill>
                        {r.note ? <div className="hsa-muted" style={{ marginTop: 4 }}>{r.note}</div> : null}
                        {(r.diffs?.length ?? 0) > 0 && !isOpen ? <div className="hsa-muted" style={{ marginTop: 4 }}>{r.diffs!.map((d) => d.campo).join(", ")} — clique para ver</div> : null}
                        {isOpen ? (
                          <div style={{ marginTop: 6, fontSize: 12 }}>
                            {(r.diffs ?? []).map((d) => (
                              <div key={d.campo}>
                                <b>{d.campo}:</b> arquivo «{d.arquivo}» × HubSoft «{d.hubsoft || "—"}»
                              </div>
                            ))}
                            {r.extras ? <div className="hsa-muted" style={{ marginTop: 4 }}>{Object.entries(r.extras).map(([k, v]) => `${k}: ${v}`).join(" · ")}</div> : null}
                          </div>
                        ) : null}
                      </td>
                      <td className="mono">{r.id || "—"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {shown.length > 1000 ? <span className="hsa-muted">Mostrando 1.000 de {shown.length}. Use os filtros ou baixe o CSV.</span> : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
