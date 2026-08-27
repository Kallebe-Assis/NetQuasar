import { useEffect, useMemo, useRef, useState } from "react";
import { downloadBlob, apiFetch } from "../../lib/api";
import { buildExcelCsvBlob } from "../../lib/excelCsv";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastInfo, toastLoading, toastOk } from "../../lib/operationToast";

type LinkSuggestion = { serial: string; client_name: string; suggested_serial: string; match_type: string };
type ImportResult = { linked: number; not_found: string[]; suggestions: LinkSuggestion[]; total: number };

const MATCH_TYPE_LABEL: Record<string, string> = {
  last5: "Últimos 5 caracteres iguais",
  tail1diff: "Só o último caractere diverge",
  last5_4of5: "4 dos últimos 5 caracteres iguais",
};

// Ordem de confiança decrescente — igual ao rank usado no backend (classifySerialMatch).
const MATCH_TYPE_ORDER = ["last5", "tail1diff", "last5_4of5"];

/** Checkbox de secção com suporte a estado indeterminado (nem tudo, nem nada selecionado). */
function SectionCheckbox({
  checked,
  indeterminate,
  onChange,
  label,
}: {
  checked: boolean;
  indeterminate: boolean;
  onChange: (checked: boolean) => void;
  label: string;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate;
  }, [indeterminate]);
  return (
    <label style={{ display: "inline-flex", alignItems: "center", gap: 6, cursor: "pointer", fontWeight: 600 }}>
      <input ref={ref} type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      {label}
    </label>
  );
}

type Props = {
  open: boolean;
  onClose: () => void;
  /** Chamado depois de uma importação com pelo menos um vínculo gravado, para refrescar a pesquisa. */
  onImported: () => void;
};

const TEMPLATE_HEADERS = ["serial", "cliente"];
const TEMPLATE_SAMPLE = ["ITBSCF8F197A", "João da Silva"];

function detectCsvSep(line: string): string {
  const semi = (line.match(/;/g) ?? []).length;
  const comma = (line.match(/,/g) ?? []).length;
  return semi >= comma ? ";" : ",";
}

/** Parser CSV simples com suporte a campos entre aspas (mesmo padrão de lib/infraCsvImport.ts). */
function parseCsvLine(line: string, sep: string): string[] {
  const out: string[] = [];
  let cur = "";
  let inQ = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (inQ) {
      if (ch === '"') {
        if (line[i + 1] === '"') {
          cur += '"';
          i++;
        } else inQ = false;
      } else cur += ch;
    } else if (ch === '"') inQ = true;
    else if (ch === sep) {
      out.push(cur);
      cur = "";
    } else cur += ch;
  }
  out.push(cur);
  return out.map((c) => c.trim());
}

async function parseOnuClientCsv(file: File): Promise<{ serial: string; client_name: string }[]> {
  let text = await file.text();
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
  const lines = text.split(/\r?\n/).filter((l) => l.trim() !== "");
  if (lines.length === 0) return [];
  const sep = detectCsvSep(lines[0]);
  const firstCells = parseCsvLine(lines[0], sep);
  const startIdx = firstCells[0]?.toLowerCase().includes("serial") ? 1 : 0;
  const rows: { serial: string; client_name: string }[] = [];
  for (let i = startIdx; i < lines.length; i++) {
    const cells = parseCsvLine(lines[i], sep);
    const serial = (cells[0] ?? "").trim();
    const client_name = (cells[1] ?? "").trim();
    if (!serial || !client_name) continue;
    rows.push({ serial, client_name });
  }
  return rows;
}

export function OltOnuClientLinkModal({ open, onClose, onImported }: Props) {
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkLinking, setBulkLinking] = useState(false);

  const groupedSuggestions = useMemo(() => {
    const suggestions = result?.suggestions ?? [];
    const byType = new Map<string, LinkSuggestion[]>();
    for (const s of suggestions) {
      const arr = byType.get(s.match_type) ?? [];
      arr.push(s);
      byType.set(s.match_type, arr);
    }
    const ordered: [string, LinkSuggestion[]][] = [];
    for (const key of MATCH_TYPE_ORDER) {
      const arr = byType.get(key);
      if (arr) {
        ordered.push([key, arr]);
        byType.delete(key);
      }
    }
    for (const [k, arr] of byType) ordered.push([k, arr]); // tipos inesperados — mostra por segurança
    return ordered;
  }, [result]);

  if (!open) return null;

  function downloadTemplate() {
    downloadBlob("modelo_vinculo_onu_cliente.csv", buildExcelCsvBlob([TEMPLATE_HEADERS, TEMPLATE_SAMPLE]));
  }

  function toggleOne(serial: string, checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) next.add(serial);
      else next.delete(serial);
      return next;
    });
  }

  function toggleSection(items: LinkSuggestion[], checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev);
      for (const it of items) {
        if (checked) next.add(it.serial);
        else next.delete(it.serial);
      }
      return next;
    });
  }

  async function acceptSuggestion(s: LinkSuggestion) {
    try {
      const res = await apiFetch<ImportResult>("/api/v1/olt/onu-client-links/import", {
        method: "POST",
        json: { rows: [{ serial: s.suggested_serial, client_name: s.client_name }] },
      });
      if (res.linked > 0) {
        onImported();
        toastOk(pushToast, `${s.client_name} vinculado a ${s.suggested_serial}.`);
        setResult((prev) => (prev ? { ...prev, linked: prev.linked + res.linked, suggestions: prev.suggestions.filter((x) => x.serial !== s.serial) } : prev));
        setSelected((prev) => {
          if (!prev.has(s.serial)) return prev;
          const next = new Set(prev);
          next.delete(s.serial);
          return next;
        });
      } else {
        toastErr(pushToast, new Error("Não foi possível gravar o vínculo."));
      }
    } catch (e) {
      toastErr(pushToast, e, "Falha ao gravar vínculo.");
    }
  }

  async function acceptSelected() {
    const targets = (result?.suggestions ?? []).filter((s) => selected.has(s.serial));
    if (targets.length === 0) return;
    setBulkLinking(true);
    const loadingId = toastLoading(pushToast, `A vincular ${targets.length} ONU(s)…`);
    try {
      const res = await apiFetch<ImportResult>("/api/v1/olt/onu-client-links/import", {
        method: "POST",
        json: { rows: targets.map((s) => ({ serial: s.suggested_serial, client_name: s.client_name })) },
      });
      if (res.linked > 0) onImported();
      // A resposta em massa refere-se aos seriais reenviados (suggested_serial) — o que não
      // voltou em not_found/suggestions foi gravado com sucesso.
      const stillUnresolved = new Set<string>([...(res.not_found ?? []), ...res.suggestions.map((x) => x.serial)]);
      setResult((prev) =>
        prev
          ? {
              ...prev,
              linked: prev.linked + res.linked,
              suggestions: prev.suggestions.filter((s) => !selected.has(s.serial) || stillUnresolved.has(s.suggested_serial)),
            }
          : prev,
      );
      setSelected(new Set());
      if (res.linked === targets.length) {
        toastOk(pushToast, `${res.linked} vínculo(s) gravado(s) com sucesso.`);
      } else {
        toastInfo(pushToast, `${res.linked} de ${targets.length} vínculo(s) gravado(s). Confira os que ficaram pendentes.`);
      }
    } catch (e) {
      toastErr(pushToast, e, "Falha ao vincular selecionados.");
    } finally {
      setBulkLinking(false);
      dismissToast(loadingId);
    }
  }

  async function handleFile(file: File) {
    setResult(null);
    setSelected(new Set());
    const rows = await parseOnuClientCsv(file);
    if (rows.length === 0) {
      toastErr(pushToast, new Error("Nenhuma linha válida no CSV. Use o modelo: coluna 1 = serial, coluna 2 = cliente."));
      return;
    }
    setImporting(true);
    const loadingId = toastLoading(pushToast, `A importar ${rows.length} vínculo(s)…`);
    try {
      const res = await apiFetch<ImportResult>("/api/v1/olt/onu-client-links/import", {
        method: "POST",
        json: { rows },
      });
      setResult(res);
      if (res.linked > 0) onImported();
      if (res.suggestions.length > 0 || res.not_found.length > 0) {
        const parts = [`${res.linked} vínculo(s) gravado(s).`];
        if (res.suggestions.length > 0) parts.push(`${res.suggestions.length} com correspondência aproximada — confira abaixo.`);
        if (res.not_found.length > 0) parts.push(`${res.not_found.length} sem nenhuma ONU parecida.`);
        toastInfo(pushToast, parts.join(" "));
      } else {
        toastOk(pushToast, `${res.linked} vínculo(s) gravado(s) com sucesso.`);
      }
    } catch (e) {
      toastErr(pushToast, e, "Falha ao importar vínculos.");
    } finally {
      setImporting(false);
      dismissToast(loadingId);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !importing && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 860, width: "94vw" }} onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>Vincular ONU ao cliente</h3>
        <p style={{ fontSize: 12, color: "var(--muted)" }}>
          Baixe o modelo, preencha uma linha por ONU (número de série e nome do cliente) e envie de volta. Só são
          gravados vínculos cujo serial já existe entre as ONUs conhecidas das OLTs.
        </p>

        <div className="row" style={{ gap: 8, marginTop: 12 }}>
          <button type="button" className="btn" onClick={downloadTemplate} disabled={importing}>
            Baixar modelo CSV
          </button>
          <button type="button" className="btn btn--primary" onClick={() => fileRef.current?.click()} disabled={importing}>
            {importing ? "A importar…" : "Escolher ficheiro preenchido…"}
          </button>
          <input
            ref={fileRef}
            type="file"
            accept=".csv,text/csv"
            style={{ display: "none" }}
            onChange={(e) => {
              const file = e.target.files?.[0];
              e.target.value = "";
              if (file) void handleFile(file);
            }}
          />
        </div>

        {result ? (
          <div style={{ marginTop: 14, fontSize: 12 }}>
            <p style={{ margin: "0 0 6px" }}>
              <strong>{result.linked}</strong> de {result.total} vínculo(s) gravado(s).
            </p>
            {result.suggestions.length > 0 ? (
              <div style={{ marginBottom: 10 }}>
                <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 6 }}>
                  <p style={{ margin: 0, color: "var(--muted)" }}>
                    Não bateram exato, mas encontramos uma ONU parecida — marque as certas e vincule em massa, ou aceite uma por uma:
                  </p>
                  <button
                    type="button"
                    className="btn btn--sm btn--primary"
                    disabled={selected.size === 0 || bulkLinking}
                    onClick={() => void acceptSelected()}
                  >
                    {bulkLinking ? "A vincular…" : `Vincular selecionados (${selected.size})`}
                  </button>
                </div>
                <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
                  {groupedSuggestions.map(([matchType, items]) => {
                    const allChecked = items.every((it) => selected.has(it.serial));
                    const someChecked = !allChecked && items.some((it) => selected.has(it.serial));
                    return (
                      <div key={matchType}>
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 8,
                            padding: "6px 8px",
                            borderBottom: "1px solid var(--border)",
                            marginBottom: 6,
                          }}
                        >
                          <SectionCheckbox
                            checked={allChecked}
                            indeterminate={someChecked}
                            onChange={(checked) => toggleSection(items, checked)}
                            label={`${MATCH_TYPE_LABEL[matchType] ?? matchType} (${items.length})`}
                          />
                        </div>
                        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                          {items.map((s) => (
                            <div
                              key={s.serial}
                              style={{
                                display: "flex",
                                alignItems: "center",
                                gap: 8,
                                padding: "6px 8px",
                                background: "var(--surface-2, rgba(127,127,127,.08))",
                                borderRadius: 6,
                              }}
                            >
                              <input
                                type="checkbox"
                                checked={selected.has(s.serial)}
                                onChange={(e) => toggleOne(s.serial, e.target.checked)}
                              />
                              <div style={{ flex: 1, minWidth: 0 }}>
                                <div className="mono">
                                  {s.serial} → <strong>{s.suggested_serial}</strong>
                                </div>
                                <div style={{ color: "var(--muted)" }}>{s.client_name}</div>
                              </div>
                              <button type="button" className="btn btn--sm" onClick={() => void acceptSuggestion(s)}>
                                Usar esta ONU
                              </button>
                            </div>
                          ))}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : null}
            {result.not_found.length > 0 ? (
              <>
                <p style={{ margin: "0 0 4px", color: "var(--muted)" }}>
                  Seriais não encontrados entre as ONUs conhecidas (confira o cadastro/coleta da OLT):
                </p>
                <div className="mono" style={{ maxHeight: 140, overflow: "auto", background: "var(--surface-2, rgba(127,127,127,.08))", borderRadius: 6, padding: 8 }}>
                  {result.not_found.map((s) => (
                    <div key={s}>{s}</div>
                  ))}
                </div>
              </>
            ) : null}
          </div>
        ) : null}

        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" disabled={importing} onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}
