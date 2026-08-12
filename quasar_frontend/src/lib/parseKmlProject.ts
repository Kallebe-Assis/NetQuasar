/** Parser de KML/KMZ para importação de projectos FTTH (CTO, foguete/emenda, poste, cabo). */

import { readKmlTextsFromFile } from "./readKmlArchive";

export type KmlLatLng = { lat: number; lng: number };

export type KmlParsedPoint = {
  description: string;
  latitude: number;
  longitude: number;
};

export type KmlParsedCable = {
  description: string;
  path: KmlLatLng[];
  latitude: number;
  longitude: number;
};

export type ParsedKmlProject = {
  name: string;
  ctos: KmlParsedPoint[];
  splice_boxes: KmlParsedPoint[];
  poles: KmlParsedPoint[];
  cables: KmlParsedCable[];
  skipped: number;
};

export type KmlElementKind = "cto" | "splice_box" | "pole" | "cable";

/** Item editável no modal de revisão pós-KML. */
export type KmlReviewItem = {
  id: string;
  kind: KmlElementKind;
  detectedKind: KmlElementKind;
  description: string;
  latitude: number;
  longitude: number;
  path: KmlLatLng[] | null;
  folder: string;
  include: boolean;
  splitter: string;
  transmitter: string;
  fiber_color: string;
  fiber_count: string;
  cable_type: string;
  cable_status: string;
  box_model: "emenda" | "distribuicao";
  pole_type: string;
  notes: string;
};

export const KML_KIND_LABELS: Record<KmlElementKind, string> = {
  cto: "CTO",
  splice_box: "Emenda / foguete",
  pole: "Poste",
  cable: "Cabo",
};

function textContent(el: Element | null | undefined): string {
  return (el?.textContent ?? "").trim();
}

function parseCoordinates(el: Element | null | undefined): KmlLatLng[] {
  const raw = textContent(el);
  if (!raw) return [];
  const out: KmlLatLng[] = [];
  for (const tuple of raw.trim().split(/[\s\n\r]+/).filter(Boolean)) {
    const bits = tuple.split(",");
    if (bits.length < 2) continue;
    const lng = Number(bits[0]);
    const lat = Number(bits[1]);
    if (!Number.isFinite(lat) || !Number.isFinite(lng)) continue;
    if (lat < -90 || lat > 90 || lng < -180 || lng > 180) continue;
    out.push({ lat, lng });
  }
  return out;
}

function folderPath(el: Element): string {
  const parts: string[] = [];
  let cur: Element | null = el.parentElement;
  while (cur) {
    const tag = cur.tagName.toLowerCase();
    if (tag === "folder" || tag === "document") {
      const name = textContent(cur.getElementsByTagName("name")[0]);
      if (name) parts.unshift(name);
    }
    cur = cur.parentElement;
  }
  return parts.join(" / ");
}

function classifyFromText(text: string): KmlElementKind | null {
  const t = text
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();

  if (/\b(line|linha|cabo|cable|fibra|backbone|drop|feeder)\b/.test(t) && !/\b(cto|poste|emenda|foguete)\b/.test(t)) {
    return "cable";
  }
  if (/\b(cto|nap|ceo\s*termino|caixa\s*de\s*atendimento|caixa\s*termino|terminacao)\b/.test(t)) {
    return "cto";
  }
  if (/\b(foguete|emenda|splice|caixa\s*de\s*emenda|ce\b|ceo\s*emenda)\b/.test(t)) {
    return "splice_box";
  }
  if (/\b(poste|pole|postes|torre)\b/.test(t)) {
    return "pole";
  }
  return null;
}

export function classifyKmlPlacemark(name: string, folder: string, hasLine: boolean): KmlElementKind {
  if (hasLine) return "cable";
  const kind = classifyFromText(`${folder} ${name}`);
  if (kind && kind !== "cable") return kind;
  const f = folder
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();
  if (/\bcto|nap\b/.test(f)) return "cto";
  if (/\bemenda|foguete|splice\b/.test(f)) return "splice_box";
  if (/\bposte|pole\b/.test(f)) return "pole";
  if (/\bcabo|cable|fibra\b/.test(f)) return "cable";
  return "pole";
}

function firstPointCoords(pm: Element): KmlLatLng[] {
  const point = pm.getElementsByTagName("Point")[0];
  if (point) return parseCoordinates(point.getElementsByTagName("coordinates")[0]);
  return [];
}

function lineCoords(pm: Element): KmlLatLng[] {
  const line = pm.getElementsByTagName("LineString")[0];
  if (line) return parseCoordinates(line.getElementsByTagName("coordinates")[0]);
  const multi = pm.getElementsByTagName("MultiGeometry")[0];
  if (multi) {
    const lines = multi.getElementsByTagName("LineString");
    const all: KmlLatLng[] = [];
    for (let i = 0; i < lines.length; i++) {
      all.push(...parseCoordinates(lines[i].getElementsByTagName("coordinates")[0]));
    }
    return all;
  }
  return [];
}

function uniquePath(path: KmlLatLng[]): KmlLatLng[] {
  if (path.length === 0) return path;
  const out: KmlLatLng[] = [path[0]];
  for (let i = 1; i < path.length; i++) {
    const prev = out[out.length - 1];
    const cur = path[i];
    if (Math.abs(prev.lat - cur.lat) < 1e-9 && Math.abs(prev.lng - cur.lng) < 1e-9) continue;
    out.push(cur);
  }
  return out;
}

function newReviewId(prefix: string, index: number): string {
  return `${prefix}-${index}-${Math.random().toString(36).slice(2, 9)}`;
}

function emptyReviewFields(
  partial: Partial<KmlReviewItem> &
    Pick<KmlReviewItem, "id" | "kind" | "detectedKind" | "description" | "latitude" | "longitude">,
): KmlReviewItem {
  return {
    path: null,
    folder: "",
    include: true,
    splitter: "",
    transmitter: "",
    fiber_color: "Desconhecido",
    fiber_count: "",
    cable_type: "",
    cable_status: "ativo",
    box_model: "emenda",
    pole_type: "",
    notes: "",
    ...partial,
  };
}

/** Faz o parse de um ficheiro KML e classifica placemarks em CTO / emenda / poste / cabo. */
export function parseKmlProject(xmlText: string): ParsedKmlProject {
  const items = parseKmlToReviewItems(xmlText);
  const result: ParsedKmlProject = {
    name: items.projectName,
    ctos: [],
    splice_boxes: [],
    poles: [],
    cables: [],
    skipped: items.skipped,
  };
  for (const it of items.items) {
    if (it.kind === "cto") {
      result.ctos.push({ description: it.description, latitude: it.latitude, longitude: it.longitude });
    } else if (it.kind === "splice_box") {
      result.splice_boxes.push({ description: it.description, latitude: it.latitude, longitude: it.longitude });
    } else if (it.kind === "pole") {
      result.poles.push({ description: it.description, latitude: it.latitude, longitude: it.longitude });
    } else if (it.kind === "cable" && it.path && it.path.length >= 2) {
      result.cables.push({
        description: it.description,
        path: it.path,
        latitude: it.latitude,
        longitude: it.longitude,
      });
    }
  }
  return result;
}

export type ParsedKmlReview = {
  projectName: string;
  items: KmlReviewItem[];
  skipped: number;
};

/** Parse KML para lista plana editável no modal de revisão. */
export function parseKmlToReviewItems(xmlText: string): ParsedKmlReview {
  const parser = new DOMParser();
  const doc = parser.parseFromString(xmlText, "application/xml");
  if (doc.querySelector("parsererror")) {
    throw new Error("Ficheiro KML inválido ou corrompido.");
  }

  const projectName =
    textContent(doc.getElementsByTagName("Document")[0]?.getElementsByTagName("name")[0]) ||
    textContent(doc.getElementsByTagName("name")[0]) ||
    "Projecto importado";

  const items: KmlReviewItem[] = [];
  let skipped = 0;
  const placemarks = doc.getElementsByTagName("Placemark");

  for (let i = 0; i < placemarks.length; i++) {
    const pm = placemarks[i];
    const name =
      textContent(pm.getElementsByTagName("name")[0]) ||
      textContent(pm.getElementsByTagName("description")[0]) ||
      `Elemento ${i + 1}`;
    const folder = folderPath(pm);
    const line = uniquePath(lineCoords(pm));
    const point = firstPointCoords(pm);

    if (line.length >= 2) {
      const kind = classifyKmlPlacemark(name, folder, true);
      if (kind === "cable") {
        items.push(
          emptyReviewFields({
            id: newReviewId("cable", i),
            kind: "cable",
            detectedKind: "cable",
            description: name,
            latitude: line[0].lat,
            longitude: line[0].lng,
            path: line.map((p) => ({ lat: p.lat, lng: p.lng })),
            folder,
            fiber_count: "12",
            cable_type: "",
            cable_status: "ativo",
          }),
        );
      } else {
        skipped++;
      }
      continue;
    }

    if (point.length >= 1) {
      const kind = classifyKmlPlacemark(name, folder, false);
      items.push(
        emptyReviewFields({
          id: newReviewId(kind, i),
          kind,
          detectedKind: kind,
          description: name,
          latitude: point[0].lat,
          longitude: point[0].lng,
          path: null,
          folder,
          splitter: kind === "cto" ? "1x8" : "",
          fiber_count: kind === "splice_box" ? "12" : "",
        }),
      );
      continue;
    }

    skipped++;
  }

  return { projectName, items, skipped };
}

/** Lê .kml ou .kmz e devolve a lista plana para o modal de revisão. */
export async function parseKmlOrKmzFile(file: File): Promise<ParsedKmlReview> {
  const texts = await readKmlTextsFromFile(file);
  let projectName = "";
  const items: KmlReviewItem[] = [];
  let skipped = 0;
  let lastParseError: unknown = null;

  for (const text of texts) {
    try {
      const parsed = parseKmlToReviewItems(text);
      if (!projectName && parsed.projectName) projectName = parsed.projectName;
      items.push(...parsed.items);
      skipped += parsed.skipped;
    } catch (e) {
      lastParseError = e;
    }
  }

  if (items.length === 0 && lastParseError) throw lastParseError;
  return { projectName: projectName || "Projecto importado", items, skipped };
}

export function kmlImportSummary(p: ParsedKmlProject): string {
  const parts = [
    `${p.ctos.length} CTO`,
    `${p.splice_boxes.length} emenda`,
    `${p.poles.length} poste`,
    `${p.cables.length} cabo`,
  ];
  let s = parts.join(" · ");
  if (p.skipped > 0) s += ` · ${p.skipped} ignorado(s)`;
  return s;
}

export function kmlReviewSummary(items: KmlReviewItem[]): string {
  const included = items.filter((i) => i.include);
  const counts = { cto: 0, splice_box: 0, pole: 0, cable: 0 };
  for (const it of included) counts[it.kind]++;
  return `${counts.cto} CTO · ${counts.splice_box} emenda · ${counts.pole} poste · ${counts.cable} cabo`;
}

/** Agrupa items do modal de revisão no payload da API de importação. */
export function reviewItemsToImportElements(items: KmlReviewItem[]) {
  const ctos: Array<Record<string, unknown>> = [];
  const splice_boxes: Array<Record<string, unknown>> = [];
  const poles: Array<Record<string, unknown>> = [];
  const cables: Array<Record<string, unknown>> = [];

  for (const it of items) {
    if (!it.include) continue;
    const desc = it.description.trim() || "Elemento importado";
    if (it.kind === "cto") {
      ctos.push({
        description: desc,
        latitude: it.latitude,
        longitude: it.longitude,
        splitter: it.splitter.trim() || null,
        transmitter: it.transmitter.trim() || null,
        fiber_color: it.fiber_color.trim() || null,
        notes: it.notes.trim() || null,
      });
    } else if (it.kind === "splice_box") {
      const fc = it.fiber_count.trim() ? Number(it.fiber_count) : null;
      splice_boxes.push({
        description: desc,
        latitude: it.latitude,
        longitude: it.longitude,
        fiber_count: Number.isFinite(fc as number) ? fc : null,
        box_model: it.box_model || "emenda",
        splitter: it.splitter.trim() || null,
        notes: it.notes.trim() || null,
      });
    } else if (it.kind === "pole") {
      poles.push({
        description: desc,
        latitude: it.latitude,
        longitude: it.longitude,
        pole_type: it.pole_type.trim() || null,
      });
    } else if (it.kind === "cable") {
      const path = it.path && it.path.length >= 2 ? it.path : null;
      if (!path) continue;
      const fc = it.fiber_count.trim() ? Number(it.fiber_count) : null;
      cables.push({
        description: desc,
        latitude: path[0].lat,
        longitude: path[0].lng,
        path: path.map((p) => ({ lat: p.lat, lng: p.lng })),
        cable_type: it.cable_type.trim() || null,
        fiber_count: Number.isFinite(fc as number) ? fc : null,
        status: it.cable_status.trim() || "ativo",
      });
    }
  }

  return { ctos, splice_boxes, poles, cables };
}
