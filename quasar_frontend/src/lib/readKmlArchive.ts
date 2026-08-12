/** Lê texto KML a partir de .kml ou .kmz (ZIP com um ou mais ficheiros KML). */

const ZIP_EOCD = 0x06054b50;
const ZIP_CD_ENTRY = 0x02014b50;
const ZIP_LOCAL = 0x04034b50;
const ZIP_STORE = 0;
const ZIP_DEFLATE = 8;

function looksLikeZip(bytes: Uint8Array): boolean {
  return bytes.length >= 4 && bytes[0] === 0x50 && bytes[1] === 0x4b && (bytes[2] === 0x03 || bytes[2] === 0x05 || bytes[2] === 0x07);
}

function looksLikeXml(bytes: Uint8Array): boolean {
  let i = 0;
  if (bytes.length >= 3 && bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) i = 3;
  while (i < bytes.length && (bytes[i] === 0x09 || bytes[i] === 0x0a || bytes[i] === 0x0d || bytes[i] === 0x20)) i++;
  return i < bytes.length && bytes[i] === 0x3c;
}

function decodeUtf8(bytes: Uint8Array): string {
  return new TextDecoder("utf-8").decode(bytes);
}

function toArrayBuffer(data: Uint8Array): ArrayBuffer {
  return data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) as ArrayBuffer;
}

async function inflateRaw(data: Uint8Array): Promise<Uint8Array> {
  const stream = new Blob([toArrayBuffer(data)]).stream().pipeThrough(new DecompressionStream("deflate-raw"));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

function findEocdOffset(view: DataView, length: number): number {
  const min = Math.max(0, length - 22 - 0xffff);
  for (let i = length - 22; i >= min; i--) {
    if (view.getUint32(i, true) !== ZIP_EOCD) continue;
    const commentLen = view.getUint16(i + 20, true);
    if (i + 22 + commentLen === length) return i;
  }
  throw new Error("Ficheiro KMZ inválido ou corrompido.");
}

function isUsableKmlName(name: string): boolean {
  const n = name.replace(/\\/g, "/");
  if (!n || n.endsWith("/")) return false;
  if (n.startsWith("__MACOSX/") || n.includes("/__MACOSX/")) return false;
  const base = n.split("/").pop() ?? n;
  if (base.startsWith(".")) return false;
  return base.toLowerCase().endsWith(".kml");
}

function kmlNameRank(name: string): [number, number, string] {
  const n = name.replace(/\\/g, "/").replace(/^\.\//, "");
  const base = (n.split("/").pop() ?? n).toLowerCase();
  const depth = n.split("/").filter(Boolean).length;
  const preferred = base === "doc.kml" ? 0 : 1;
  return [preferred, depth, n.toLowerCase()];
}

async function extractKmlTextsFromZip(bytes: Uint8Array): Promise<string[]> {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const eocd = findEocdOffset(view, bytes.byteLength);
  const entryCount = view.getUint16(eocd + 10, true);
  const cdSize = view.getUint32(eocd + 12, true);
  const cdOffset = view.getUint32(eocd + 16, true);
  if (cdOffset === 0xffffffff || cdSize === 0xffffffff || entryCount === 0xffff) {
    throw new Error("Ficheiro KMZ demasiado grande (ZIP64) não é suportado.");
  }
  if (cdOffset + cdSize > bytes.byteLength) {
    throw new Error("Ficheiro KMZ inválido ou corrompido.");
  }

  type KmlEntry = { name: string; method: number; flags: number; compSize: number; localOff: number };
  const found: KmlEntry[] = [];
  let cursor = cdOffset;
  for (let i = 0; i < entryCount; i++) {
    if (cursor + 46 > bytes.byteLength || view.getUint32(cursor, true) !== ZIP_CD_ENTRY) {
      throw new Error("Ficheiro KMZ inválido ou corrompido.");
    }
    const flags = view.getUint16(cursor + 8, true);
    const method = view.getUint16(cursor + 10, true);
    const compSize = view.getUint32(cursor + 20, true);
    const nameLen = view.getUint16(cursor + 28, true);
    const extraLen = view.getUint16(cursor + 30, true);
    const commentLen = view.getUint16(cursor + 32, true);
    const localOff = view.getUint32(cursor + 42, true);
    const name = decodeUtf8(bytes.subarray(cursor + 46, cursor + 46 + nameLen));
    cursor += 46 + nameLen + extraLen + commentLen;
    if (!isUsableKmlName(name)) continue;
    found.push({ name, method, flags, compSize, localOff });
  }

  found.sort((a, b) => {
    const ra = kmlNameRank(a.name);
    const rb = kmlNameRank(b.name);
    if (ra[0] !== rb[0]) return ra[0] - rb[0];
    if (ra[1] !== rb[1]) return ra[1] - rb[1];
    return ra[2] < rb[2] ? -1 : ra[2] > rb[2] ? 1 : 0;
  });

  if (found.length === 0) {
    throw new Error("O KMZ não contém nenhum ficheiro KML.");
  }

  const texts: string[] = [];
  for (const entry of found) {
    if (entry.flags & 0x1) {
      throw new Error("KMZ protegido por palavra-passe não é suportado.");
    }
    if (entry.localOff + 30 > bytes.byteLength || view.getUint32(entry.localOff, true) !== ZIP_LOCAL) {
      throw new Error("Ficheiro KMZ inválido ou corrompido.");
    }
    const localNameLen = view.getUint16(entry.localOff + 26, true);
    const localExtraLen = view.getUint16(entry.localOff + 28, true);
    const dataOff = entry.localOff + 30 + localNameLen + localExtraLen;
    if (dataOff + entry.compSize > bytes.byteLength) {
      throw new Error("Ficheiro KMZ inválido ou corrompido.");
    }
    const compressed = bytes.subarray(dataOff, dataOff + entry.compSize);
    let raw: Uint8Array;
    if (entry.method === ZIP_STORE) {
      raw = compressed;
    } else if (entry.method === ZIP_DEFLATE) {
      try {
        raw = await inflateRaw(compressed);
      } catch {
        throw new Error("Ficheiro KMZ inválido ou corrompido.");
      }
    } else {
      throw new Error("Compressão do KMZ não suportada.");
    }
    const text = decodeUtf8(raw).trim();
    if (text) texts.push(text);
  }

  if (texts.length === 0) {
    throw new Error("O KMZ não contém nenhum ficheiro KML.");
  }
  return texts;
}

/** Devolve os documentos KML (texto) de um .kml ou .kmz. */
export async function readKmlTextsFromFile(file: File): Promise<string[]> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  if (bytes.length === 0) {
    throw new Error("Ficheiro vazio.");
  }
  const name = file.name.toLowerCase();
  const asZip = looksLikeZip(bytes) || name.endsWith(".kmz") || file.type === "application/vnd.google-earth.kmz";
  if (asZip) {
    if (!looksLikeZip(bytes)) {
      throw new Error("Ficheiro KMZ inválido ou corrompido.");
    }
    return extractKmlTextsFromZip(bytes);
  }
  if (!looksLikeXml(bytes) && (name.endsWith(".kml") || file.type.includes("kml") || file.type.includes("xml"))) {
    throw new Error("Ficheiro KML inválido ou corrompido.");
  }
  const text = decodeUtf8(bytes).trim();
  if (!text) throw new Error("Ficheiro KML inválido ou corrompido.");
  return [text];
}
