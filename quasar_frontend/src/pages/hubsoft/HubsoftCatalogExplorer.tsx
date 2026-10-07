import { useMemo, useState } from "react";
import { BookOpen, Download } from "lucide-react";
import { ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";
import { saveCsvText } from "./hubsoftCsv";

const SLUG = "hubsoft";

type CatalogInfo = { id: string; label: string };
type CatalogListResp = { ok: boolean; catalogs: CatalogInfo[] };

/** Achata um valor para exibição em célula/CSV: objetos viram "chave: valor, chave: valor". */
function flatten(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "object") {
    if (Array.isArray(v)) return v.map(flatten).join(" | ");
    return Object.entries(v as Record<string, unknown>)
      .map(([k, x]) => `${k}: ${flatten(x)}`)
      .join(", ");
  }
  return String(v);
}

function csvEsc(v: string): string {
  return /[";,\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** O corpo da HubSoft é sempre {status, msg, <algo>: [...]} — acha o primeiro array de objetos. */
function extractRows(body: unknown): { arrayKey: string; rows: Record<string, unknown>[] } | null {
  if (!body || typeof body !== "object") return null;
  for (const [k, v] of Object.entries(body as Record<string, unknown>)) {
    if (Array.isArray(v) && (v.length === 0 || typeof v[0] === "object")) {
      return { arrayKey: k, rows: v as Record<string, unknown>[] };
    }
  }
  return null;
}

/**
 * Explorador de catálogos da HubSoft — consulta ao vivo planos,
 * status de serviço, formas de cobrança, vendedores, vencimentos etc., para o operador conferir os
 * IDs reais da conta antes de preencher um CSV de cadastro em lote. Só leitura.
 */
export function HubsoftCatalogExplorer() {
  const { notify } = useConsultaToast();
  const [catalogs, setCatalogs] = useState<CatalogInfo[] | null>(null);
  const [which, setWhich] = useState("");
  const [loadingList, setLoadingList] = useState(false);
  const [loadingRows, setLoadingRows] = useState(false);
  const [result, setResult] = useState<{ arrayKey: string; rows: Record<string, unknown>[] } | null>(null);
  const [rawError, setRawError] = useState("");

  async function loadCatalogList() {
    setLoadingList(true);
    try {
      const r = await apiFetch<CatalogListResp>(`/api/v1/integrations/${SLUG}/hubsoft/catalog/list`);
      setCatalogs(r.catalogs);
      if (!which && r.catalogs.length > 0) setWhich(r.catalogs[0].id);
    } catch (e) {
      notify(null, e);
    } finally {
      setLoadingList(false);
    }
  }

  async function consult() {
    if (!which) return;
    setLoadingRows(true);
    setResult(null);
    setRawError("");
    try {
      const body = await apiFetch<Record<string, unknown>>(`/api/v1/integrations/${SLUG}/hubsoft/catalog/fetch?which=${which}`);
      const extracted = extractRows(body);
      if (!extracted) {
        setRawError(JSON.stringify(body, null, 2));
        notify(null, new Error("A HubSoft respondeu num formato inesperado — veja o JSON bruto abaixo."));
        return;
      }
      setResult(extracted);
      notify({ ok: true }, null, `Consulta realizada com sucesso — ${extracted.rows.length} registro(s).`);
    } catch (e) {
      notify(null, e);
    } finally {
      setLoadingRows(false);
    }
  }

  const columns = useMemo(() => {
    if (!result || result.rows.length === 0) return [];
    const cols = new Set<string>();
    for (const row of result.rows) Object.keys(row).forEach((k) => cols.add(k));
    return Array.from(cols);
  }, [result]);

  function exportCsv() {
    if (!result) return;
    const lines = [columns, ...result.rows.map((r) => columns.map((c) => flatten(r[c])))];
    const text = lines.map((l) => l.map((v) => csvEsc(String(v))).join(";")).join("\r\n");
    saveCsvText(`hubsoft-catalogo-${which}-${new Date().toISOString().slice(0, 10)}.csv`, text);
  }

  return (
    <ToolPanel
      icon={<BookOpen size={20} />}
      title="Catálogos da HubSoft"
      badge="Somente leitura"
      badgeTone="ok"
      subtitle="Consulta ao vivo dos IDs internos da sua conta — planos, formas de cobrança, vendedores, vencimentos, status de serviço etc. Use para resolver os IDs que um CSV de cadastro em lote precisa."
    >
      <div className="hsa-panel__body hsa-panel__body--pad">

      {!catalogs ? (
        <button type="button" className="btn btn--sm btn--primary" disabled={loadingList} onClick={() => void loadCatalogList()}>
          {loadingList ? "A carregar…" : "Carregar lista de catálogos"}
        </button>
      ) : (
        <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap", marginBottom: 10 }}>
          <select className="input" style={{ minWidth: 220 }} value={which} onChange={(e) => setWhich(e.target.value)}>
            {catalogs.map((c) => (
              <option key={c.id} value={c.id}>
                {c.label}
              </option>
            ))}
          </select>
          <button type="button" className="btn btn--sm btn--primary" disabled={loadingRows} onClick={() => void consult()}>
            {loadingRows ? "A consultar…" : "Consultar"}
          </button>
          {result ? (
            <button type="button" className="btn btn--sm" onClick={exportCsv}>
              <Download size={12} style={{ marginRight: 4, verticalAlign: -2 }} aria-hidden />
              Exportar CSV
            </button>
          ) : null}
        </div>
      )}

      {loadingRows ? <ConsultaLoading text="Consultando a HubSoft…" /> : null}

      {rawError ? (
        <>
          <div className="msg msg--err" style={{ marginBottom: 8 }}>
            Formato de resposta inesperado — resposta bruta abaixo (copie e me envie).
          </div>
          <pre style={{ fontSize: 11, background: "var(--panel2)", padding: 10, borderRadius: 8, overflow: "auto", maxHeight: 300 }}>
            {rawError}
          </pre>
        </>
      ) : null}

      {result && result.rows.length === 0 ? <div className="hubsoft-empty">Nenhum registro devolvido pela HubSoft.</div> : null}

      {result && result.rows.length > 0 ? (
        <div className="table-wrap integration-support-table" style={{ maxHeight: 480, overflow: "auto" }}>
          <table className="integration-support-table__grid">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.rows.map((r, i) => (
                <tr key={i}>
                  {columns.map((c) => (
                    <td key={c} className="mono integration-support-table__cell">
                      {flatten(r[c])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      </div>
    </ToolPanel>
  );
}
