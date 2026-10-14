/**
 * Exportação de tabelas para CSV, Excel (.xlsx) e PDF — sem dependências externas.
 *  - CSV: separador «;» + BOM (abre em colunas no Excel pt-BR) e proteção contra injeção de fórmula.
 *  - XLSX: pacote OOXML mínimo (zip sem compressão) com cabeçalho em negrito, filtro e primeira linha fixa.
 *  - PDF: A4 paisagem, Helvetica, cabeçalho repetido por página, quebra de linha por coluna e numeração.
 */

export type ExportFormat = "csv" | "xlsx" | "pdf";

export type ExportColumn<T> = {
  header: string;
  value: (row: T) => string | number | boolean | null | undefined;
};

export type ExportSpec<T> = {
  /** Nome do arquivo sem extensão (a data é acrescentada). */
  filename: string;
  /** Título impresso no PDF e nome da planilha. */
  title: string;
  columns: ExportColumn<T>[];
  rows: T[];
};

export const EXPORT_FORMAT_LABEL: Record<ExportFormat, string> = { csv: "CSV", xlsx: "Excel (XLSX)", pdf: "PDF" };

const enc = new TextEncoder();

function cellText(v: string | number | boolean | null | undefined): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "boolean") return v ? "Sim" : "Não";
  return String(v).replace(/\u0000/g, "");
}

function stamp(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}_${p(d.getHours())}${p(d.getMinutes())}`;
}

function safeFilename(name: string): string {
  return name.replace(/[\\/:*?"<>|]+/g, "-").replace(/\s+/g, "_").slice(0, 80) || "exportacao";
}

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/* ------------------------------------------------------------------ CSV */

/** Impede que o Excel interprete texto como fórmula (=, +, -, @) — números puros ficam como estão. */
function csvSafe(s: string): string {
  if (/^[=+\-@\t\r]/.test(s) && !/^[-+]?\d+([.,]\d+)?$/.test(s)) return `'${s}`;
  return s;
}

function csvEscape(s: string): string {
  return /[";\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function buildCsv<T>(spec: ExportSpec<T>): string {
  const head = spec.columns.map((c) => csvEscape(csvSafe(c.header))).join(";");
  const body = spec.rows.map((r) => spec.columns.map((c) => csvEscape(csvSafe(cellText(c.value(r))))).join(";"));
  return "﻿" + [head, ...body].join("\r\n") + "\r\n";
}

/* ----------------------------------------------------------------- ZIP */

let crcTable: Uint32Array | null = null;
function crc32(data: Uint8Array): number {
  if (!crcTable) {
    crcTable = new Uint32Array(256);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      crcTable[n] = c >>> 0;
    }
  }
  let c = 0xffffffff;
  for (let i = 0; i < data.length; i++) c = crcTable[(c ^ data[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

type ZipEntry = { name: string; data: Uint8Array };

/** Zip sem compressão («stored») — suficiente e válido para o Excel. */
function buildZip(entries: ZipEntry[]): Uint8Array {
  const chunks: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let offset = 0;
  const now = new Date();
  const dosTime = (now.getHours() << 11) | (now.getMinutes() << 5) | (now.getSeconds() >> 1);
  const dosDate = ((now.getFullYear() - 1980) << 9) | ((now.getMonth() + 1) << 5) | now.getDate();
  for (const e of entries) {
    const name = enc.encode(e.name);
    const crc = crc32(e.data);
    const local = new DataView(new ArrayBuffer(30));
    local.setUint32(0, 0x04034b50, true);
    local.setUint16(4, 20, true);
    local.setUint16(6, 0x0800, true); // nomes em UTF-8
    local.setUint16(8, 0, true);
    local.setUint16(10, dosTime, true);
    local.setUint16(12, dosDate, true);
    local.setUint32(14, crc, true);
    local.setUint32(18, e.data.length, true);
    local.setUint32(22, e.data.length, true);
    local.setUint16(26, name.length, true);
    local.setUint16(28, 0, true);
    chunks.push(new Uint8Array(local.buffer), name, e.data);

    const cd = new DataView(new ArrayBuffer(46));
    cd.setUint32(0, 0x02014b50, true);
    cd.setUint16(4, 20, true);
    cd.setUint16(6, 20, true);
    cd.setUint16(8, 0x0800, true);
    cd.setUint16(10, 0, true);
    cd.setUint16(12, dosTime, true);
    cd.setUint16(14, dosDate, true);
    cd.setUint32(16, crc, true);
    cd.setUint32(20, e.data.length, true);
    cd.setUint32(24, e.data.length, true);
    cd.setUint16(28, name.length, true);
    cd.setUint32(42, offset, true);
    central.push(new Uint8Array(cd.buffer), name);
    offset += 30 + name.length + e.data.length;
  }
  const cdSize = central.reduce((n, c) => n + c.length, 0);
  const end = new DataView(new ArrayBuffer(22));
  end.setUint32(0, 0x06054b50, true);
  end.setUint16(8, entries.length, true);
  end.setUint16(10, entries.length, true);
  end.setUint32(12, cdSize, true);
  end.setUint32(16, offset, true);
  const all = [...chunks, ...central, new Uint8Array(end.buffer)];
  const out = new Uint8Array(all.reduce((n, c) => n + c.length, 0));
  let p = 0;
  for (const c of all) {
    out.set(c, p);
    p += c.length;
  }
  return out;
}

/* ---------------------------------------------------------------- XLSX */

function xmlEsc(s: string): string {
  // eslint-disable-next-line no-control-regex
  return s.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g, "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

function colName(i: number): string {
  let n = i + 1;
  let s = "";
  while (n > 0) {
    const m = (n - 1) % 26;
    s = String.fromCharCode(65 + m) + s;
    n = Math.floor((n - 1) / 26);
  }
  return s;
}

function sheetName(title: string): string {
  return title.replace(/[[\]:*?/\\]/g, " ").trim().slice(0, 31) || "Dados";
}

export function buildXlsx<T>(spec: ExportSpec<T>): Uint8Array {
  const ncol = spec.columns.length;
  const widths = spec.columns.map((c) => Math.max(8, Math.min(60, c.header.length + 2)));
  const rowsXml: string[] = [];
  const headCells = spec.columns.map((c, i) => `<c r="${colName(i)}1" s="1" t="inlineStr"><is><t xml:space="preserve">${xmlEsc(c.header)}</t></is></c>`).join("");
  rowsXml.push(`<row r="1">${headCells}</row>`);
  spec.rows.forEach((row, ri) => {
    const r = ri + 2;
    const cells: string[] = [];
    spec.columns.forEach((c, ci) => {
      const raw = c.value(row);
      if (raw === null || raw === undefined || raw === "") return;
      const ref = `${colName(ci)}${r}`;
      if (typeof raw === "number" && Number.isFinite(raw)) {
        cells.push(`<c r="${ref}"><v>${raw}</v></c>`);
        widths[ci] = Math.max(widths[ci], Math.min(60, String(raw).length + 2));
      } else {
        const t = cellText(raw);
        cells.push(`<c r="${ref}" t="inlineStr"><is><t xml:space="preserve">${xmlEsc(t)}</t></is></c>`);
        widths[ci] = Math.max(widths[ci], Math.min(60, Math.max(...t.split(/\r?\n/).map((l) => l.length)) + 2));
      }
    });
    rowsXml.push(`<row r="${r}">${cells.join("")}</row>`);
  });
  const cols = widths.map((w, i) => `<col min="${i + 1}" max="${i + 1}" width="${w}" customWidth="1"/>`).join("");
  const lastRef = `${colName(Math.max(0, ncol - 1))}${spec.rows.length + 1}`;
  const sheet =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
    `<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>` +
    `<cols>${cols}</cols><sheetData>${rowsXml.join("")}</sheetData>` +
    (ncol > 0 ? `<autoFilter ref="A1:${lastRef}"/>` : "") +
    `</worksheet>`;
  const styles =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
    `<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
    `<fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill>` +
    `<fill><patternFill patternType="solid"><fgColor rgb="FFE8EEF5"/><bgColor indexed="64"/></patternFill></fill></fills>` +
    `<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
    `<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
    `<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
    `<xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/></cellXfs>` +
    `<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
    `</styleSheet>`;
  const workbook =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
    `<sheets><sheet name="${xmlEsc(sheetName(spec.title))}" sheetId="1" r:id="rId1"/></sheets></workbook>`;
  const wbRels =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
    `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
    `<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
    `</Relationships>`;
  const rels =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
    `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
    `</Relationships>`;
  const types =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
    `<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
    `<Default Extension="xml" ContentType="application/xml"/>` +
    `<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
    `<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
    `<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
    `</Types>`;
  return buildZip([
    { name: "[Content_Types].xml", data: enc.encode(types) },
    { name: "_rels/.rels", data: enc.encode(rels) },
    { name: "xl/workbook.xml", data: enc.encode(workbook) },
    { name: "xl/_rels/workbook.xml.rels", data: enc.encode(wbRels) },
    { name: "xl/styles.xml", data: enc.encode(styles) },
    { name: "xl/worksheets/sheet1.xml", data: enc.encode(sheet) },
  ]);
}

/* ----------------------------------------------------------------- PDF */

// Larguras (1/1000 em) da Helvetica para ASCII 32..126.
const HELV_W = [
  278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, 556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
  1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
  333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
];

// Caracteres fora do Latin-1 que existem no WinAnsi (cp1252).
const CP1252_EXTRA: Record<number, number> = {
  0x20ac: 0x80, 0x201a: 0x82, 0x0192: 0x83, 0x201e: 0x84, 0x2026: 0x85, 0x2020: 0x86, 0x2021: 0x87, 0x02c6: 0x88, 0x2030: 0x89, 0x0160: 0x8a, 0x2039: 0x8b,
  0x0152: 0x8c, 0x017d: 0x8e, 0x2018: 0x91, 0x2019: 0x92, 0x201c: 0x93, 0x201d: 0x94, 0x2022: 0x95, 0x2013: 0x96, 0x2014: 0x97, 0x02dc: 0x98, 0x2122: 0x99,
  0x0161: 0x9a, 0x203a: 0x9b, 0x0153: 0x9c, 0x017e: 0x9e, 0x0178: 0x9f,
};

function toWinAnsi(s: string): number[] {
  const out: number[] = [];
  for (const ch of s) {
    const cp = ch.codePointAt(0)!;
    if (cp === 9 || cp === 10 || cp === 13) out.push(32);
    else if (cp >= 32 && cp < 127) out.push(cp);
    else if (cp >= 160 && cp <= 255) out.push(cp);
    else if (CP1252_EXTRA[cp] !== undefined) out.push(CP1252_EXTRA[cp]);
    else out.push(63); // ?
  }
  return out;
}

function charW(code: number): number {
  if (code >= 32 && code <= 126) return HELV_W[code - 32];
  if (code >= 192 && code <= 255) return 600; // acentuadas ≈ largura média
  return 556;
}

function textW(codes: number[], size: number): number {
  let w = 0;
  for (const c of codes) w += charW(c);
  return (w * size) / 1000;
}

function pdfStr(codes: number[]): string {
  let s = "(";
  for (const c of codes) {
    if (c === 40 || c === 41 || c === 92) s += "\\" + String.fromCharCode(c);
    else if (c < 32 || c > 126) s += "\\" + c.toString(8).padStart(3, "0");
    else s += String.fromCharCode(c);
  }
  return s + ")";
}

function wrapCodes(codes: number[], maxW: number, size: number, maxLines: number): number[][] {
  if (codes.length === 0) return [[]];
  const words: number[][] = [];
  let cur: number[] = [];
  for (const c of codes) {
    if (c === 32) {
      if (cur.length) words.push(cur);
      cur = [];
    } else cur.push(c);
  }
  if (cur.length) words.push(cur);
  const lines: number[][] = [];
  let line: number[] = [];
  const push = () => {
    lines.push(line);
    line = [];
  };
  for (const w of words) {
    let word = w;
    // palavra maior que a coluna: quebra por caractere
    while (textW(word, size) > maxW && word.length > 1) {
      let cut = 1;
      while (cut < word.length && textW(word.slice(0, cut + 1), size) <= maxW) cut++;
      if (line.length) push();
      lines.push(word.slice(0, cut));
      word = word.slice(cut);
    }
    const trial = line.length ? [...line, 32, ...word] : word;
    if (textW(trial, size) <= maxW) line = trial;
    else {
      if (line.length) push();
      line = word;
    }
  }
  if (line.length) push();
  if (lines.length > maxLines) {
    const kept = lines.slice(0, maxLines);
    const last = kept[maxLines - 1];
    while (last.length > 1 && textW([...last, 46, 46, 46], size) > maxW) last.pop();
    kept[maxLines - 1] = [...last, 46, 46, 46];
    return kept;
  }
  return lines;
}

export function buildPdf<T>(spec: ExportSpec<T>): Uint8Array {
  const PW = 841.89;
  const PH = 595.28;
  const M = 28;
  const size = 7.5;
  const lead = 9.5;
  const headSize = 8;
  const maxLines = 4;
  const usableW = PW - 2 * M;
  const cols = spec.columns;
  const header = cols.map((c) => toWinAnsi(c.header));
  const data = spec.rows.map((r) => cols.map((c) => toWinAnsi(cellText(c.value(r)))));

  // largura natural por coluna (amostra) e escala para caber na página
  const PAD = 4;
  const natural = cols.map((_, i) => {
    let w = textW(header[i], headSize) + 2 * PAD;
    const sample = Math.min(data.length, 400);
    for (let r = 0; r < sample; r++) w = Math.max(w, Math.min(textW(data[r][i], size), 220) + 2 * PAD);
    return Math.max(34, w);
  });
  const sum = natural.reduce((a, b) => a + b, 0);
  const colW = sum > usableW ? natural.map((w) => Math.max(26, (w * usableW) / sum)) : natural.map((w) => w + (usableW - sum) / natural.length);

  type PageOps = string[];
  const pages: PageOps[] = [];
  let ops: PageOps = [];
  let y = 0;
  const top = PH - M - 30;
  const bottom = M + 14;
  const title = toWinAnsi(spec.title);
  const when = toWinAnsi(`Gerado em ${new Date().toLocaleString("pt-BR")} — ${spec.rows.length} registro(s)`);

  const startPage = () => {
    ops = [];
    pages.push(ops);
    ops.push(`BT /F2 11 Tf ${M} ${PH - M - 8} Td ${pdfStr(title)} Tj ET`);
    ops.push(`BT /F1 7 Tf 0.4 0.4 0.4 rg ${M} ${PH - M - 19} Td ${pdfStr(when)} Tj ET 0 g`);
    y = top;
    // cabeçalho da tabela
    const hh = lead + 5;
    ops.push(`0.91 0.93 0.96 rg ${M} ${y - hh + 3} ${usableW} ${hh} re f 0 g`);
    let x = M;
    header.forEach((h, i) => {
      const lines = wrapCodes(h, colW[i] - 2 * PAD, headSize, 2);
      ops.push(`BT /F2 ${headSize} Tf ${x + PAD} ${y - lead + 3} Td ${pdfStr(lines[0] ?? [])} Tj ET`);
      x += colW[i];
    });
    y -= hh;
  };

  startPage();
  data.forEach((row, ri) => {
    const wrapped = row.map((cell, i) => wrapCodes(cell, colW[i] - 2 * PAD, size, maxLines));
    const nLines = Math.max(1, ...wrapped.map((w) => w.length));
    const rowH = nLines * lead + 3;
    if (y - rowH < bottom) startPage();
    if (ri % 2 === 1) ops.push(`0.965 0.972 0.98 rg ${M} ${y - rowH + 3} ${usableW} ${rowH} re f 0 g`);
    let x = M;
    wrapped.forEach((lines, i) => {
      lines.forEach((ln, li) => {
        if (ln.length) ops.push(`BT /F1 ${size} Tf ${x + PAD} ${y - lead * (li + 1) + 3} Td ${pdfStr(ln)} Tj ET`);
      });
      x += colW[i];
    });
    y -= rowH;
  });
  if (data.length === 0) {
    ops.push(`BT /F1 9 Tf ${M} ${y - 14} Td ${pdfStr(toWinAnsi("Nenhum registro."))} Tj ET`);
  }
  pages.forEach((p, i) => {
    p.push(`BT /F1 7 Tf 0.4 0.4 0.4 rg ${PW - M - 70} ${M - 4} Td ${pdfStr(toWinAnsi(`Página ${i + 1} de ${pages.length}`))} Tj ET 0 g`);
  });

  // objetos: 1 catálogo, 2 páginas, 3 fonte, 4 fonte bold, depois (página, conteúdo) pares
  const objs: string[] = [];
  const kids: string[] = [];
  objs[1] = "<< /Type /Catalog /Pages 2 0 R >>";
  objs[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>";
  objs[4] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>";
  pages.forEach((p, i) => {
    const pageObj = 5 + i * 2;
    const contentObj = pageObj + 1;
    kids.push(`${pageObj} 0 R`);
    const stream = p.join("\n");
    objs[pageObj] = `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${PW} ${PH}] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents ${contentObj} 0 R >>`;
    objs[contentObj] = `<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`;
  });
  objs[2] = `<< /Type /Pages /Kids [${kids.join(" ")}] /Count ${pages.length} >>`;

  // o conteúdo é todo ASCII (acentos saem como escapes octais), então 1 char = 1 byte
  let out = "%PDF-1.4\n";
  const offsets: number[] = [];
  for (let i = 1; i < objs.length; i++) {
    offsets[i] = out.length;
    out += `${i} 0 obj\n${objs[i]}\nendobj\n`;
  }
  const xref = out.length;
  out += `xref\n0 ${objs.length}\n0000000000 65535 f \n`;
  for (let i = 1; i < objs.length; i++) out += `${String(offsets[i]).padStart(10, "0")} 00000 n \n`;
  out += `trailer\n<< /Size ${objs.length} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF`;
  const bytes = new Uint8Array(out.length);
  for (let i = 0; i < out.length; i++) bytes[i] = out.charCodeAt(i) & 0xff;
  return bytes;
}

/* ------------------------------------------------------------ entrada */

export function exportTable<T>(format: ExportFormat, spec: ExportSpec<T>): void {
  const name = `${safeFilename(spec.filename)}_${stamp()}`;
  if (format === "csv") {
    saveBlob(new Blob([buildCsv(spec)], { type: "text/csv;charset=utf-8" }), `${name}.csv`);
  } else if (format === "xlsx") {
    saveBlob(new Blob([buildXlsx(spec)], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" }), `${name}.xlsx`);
  } else {
    saveBlob(new Blob([buildPdf(spec)], { type: "application/pdf" }), `${name}.pdf`);
  }
}
