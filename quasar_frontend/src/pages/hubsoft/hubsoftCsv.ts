/** Utilidades de CSV partilhadas pelas telas de edição/importação em massa da integração HubSoft. */

export function csvEsc(v: string): string {
  return /[";,\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** Baixa um texto CSV já montado (com BOM, para o Excel abrir os acentos corretamente). */
export function saveCsvText(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([`﻿${text}`], { type: "text/csv;charset=utf-8;" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

export function downloadCsv(name: string, head: string[], rows: string[][]) {
  saveCsvText(name, [head, ...rows].map((r) => r.map((c) => csvEsc(c ?? "")).join(";")).join("\r\n"));
}

/** Parser CSV mínimo: separador ; ou , (detectado pelo cabeçalho), aspas duplas, BOM. */
export function parseCsv(text: string): string[][] {
  const t = text.replace(/^﻿/, "");
  const firstLine = t.split(/\r?\n/, 1)[0] ?? "";
  const sep = (firstLine.match(/;/g)?.length ?? 0) >= (firstLine.match(/,/g)?.length ?? 0) ? ";" : ",";
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let q = false;
  for (let i = 0; i < t.length; i++) {
    const c = t[i];
    if (q) {
      if (c === '"' && t[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (c === '"') q = false;
      else cell += c;
    } else if (c === '"') q = true;
    else if (c === sep) {
      row.push(cell);
      cell = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && t[i + 1] === "\n") i++;
      row.push(cell);
      cell = "";
      if (row.some((x) => x.trim() !== "")) rows.push(row);
      row = [];
    } else cell += c;
  }
  row.push(cell);
  if (row.some((x) => x.trim() !== "")) rows.push(row);
  return rows;
}

/** CSV (já lido por parseCsv) → lista de objetos, usando a 1ª linha como cabeçalho (chaves exatas). */
export function csvRowsToObjects(rows: string[][]): Record<string, string>[] {
  if (rows.length < 2) return [];
  const header = rows[0].map((h) => h.trim());
  return rows.slice(1).map((r) => {
    const o: Record<string, string> = {};
    header.forEach((h, i) => {
      o[h] = (r[i] ?? "").trim();
    });
    return o;
  });
}
