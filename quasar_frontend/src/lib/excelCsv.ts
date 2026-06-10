/** Separador listado pelo Excel em locale pt-BR / pt-PT (vírgula decimal). */
const EXCEL_LIST_SEP = ";";

function escapeExcelCsvCell(value: string): string {
  const v = String(value ?? "");
  if (/[";\r\n]/.test(v)) {
    return `"${v.replace(/"/g, '""')}"`;
  }
  return v;
}

/** Linhas CSV que o Excel abre já em colunas (UTF-8 BOM + ponto e vírgula). */
export function buildExcelCsvContent(rows: string[][]): string {
  const body = rows.map((row) => row.map(escapeExcelCsvCell).join(EXCEL_LIST_SEP)).join("\r\n");
  return `\uFEFF${body}\r\n`;
}

export function buildExcelCsvBlob(rows: string[][]): Blob {
  return new Blob([buildExcelCsvContent(rows)], { type: "text/csv;charset=utf-8" });
}
