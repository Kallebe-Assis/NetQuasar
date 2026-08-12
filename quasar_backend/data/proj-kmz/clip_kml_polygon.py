"""Recorta Placemarks de um KMZ/KML para um polígono lat/lon."""
from __future__ import annotations

import math
import sys
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path

KML_NS = "http://www.opengis.net/kml/2.2"
GX_NS = "http://www.google.com/kml/ext/2.2"
NSMAP = {"kml": KML_NS, "gx": GX_NS}

for prefix, uri in NSMAP.items():
    ET.register_namespace(prefix if prefix != "kml" else "", uri)


def dms_to_dec(deg: float, minutes: float, seconds: float, hemi: str) -> float:
    val = abs(deg) + minutes / 60.0 + seconds / 3600.0
    if hemi.upper() in ("S", "W"):
        val = -val
    return val


def point_in_ring(lon: float, lat: float, ring: list[tuple[float, float]]) -> bool:
    """Ray casting. ring = [(lon, lat), ...] fechado ou aberto."""
    n = len(ring)
    if n < 3:
        return False
    inside = False
    j = n - 1
    for i in range(n):
        xi, yi = ring[i]
        xj, yj = ring[j]
        intersects = ((yi > lat) != (yj > lat)) and (
            lon < (xj - xi) * (lat - yi) / (yj - yi + 1e-18) + xi
        )
        if intersects:
            inside = not inside
        j = i
    return inside


def segments_intersect(a1, a2, b1, b2) -> bool:
    def orient(p, q, r):
        return (q[1] - p[1]) * (r[0] - q[0]) - (q[0] - p[0]) * (r[1] - q[1])

    def on_seg(p, q, r):
        return (
            min(p[0], r[0]) - 1e-12 <= q[0] <= max(p[0], r[0]) + 1e-12
            and min(p[1], r[1]) - 1e-12 <= q[1] <= max(p[1], r[1]) + 1e-12
        )

    o1 = orient(a1, a2, b1)
    o2 = orient(a1, a2, b2)
    o3 = orient(b1, b2, a1)
    o4 = orient(b1, b2, a2)
    if o1 == 0 and on_seg(a1, b1, a2):
        return True
    if o2 == 0 and on_seg(a1, b2, a2):
        return True
    if o3 == 0 and on_seg(b1, a1, b2):
        return True
    if o4 == 0 and on_seg(b1, a2, b2):
        return True
    return (o1 > 0) != (o2 > 0) and (o3 > 0) != (o4 > 0)


def geom_hits_polygon(points: list[tuple[float, float]], ring: list[tuple[float, float]]) -> bool:
    if not points:
        return False
    if any(point_in_ring(lon, lat, ring) for lon, lat in points):
        return True
    if len(points) == 1:
        return False
    edges = list(zip(ring, ring[1:] + ring[:1]))
    segs = list(zip(points, points[1:]))
    for a1, a2 in segs:
        for b1, b2 in edges:
            if segments_intersect(a1, a2, b1, b2):
                return True
    return False


def parse_coord_text(text: str) -> list[tuple[float, float]]:
    out: list[tuple[float, float]] = []
    for tuple_s in (text or "").replace("\n", " ").split():
        bits = tuple_s.split(",")
        if len(bits) < 2:
            continue
        try:
            lon = float(bits[0])
            lat = float(bits[1])
        except ValueError:
            continue
        if math.isfinite(lat) and math.isfinite(lon):
            out.append((lon, lat))
    return out


def placemark_points(pm: ET.Element) -> list[tuple[float, float]]:
    pts: list[tuple[float, float]] = []
    for el in pm.iter():
        tag = el.tag.split("}")[-1]
        if tag == "coordinates" and el.text:
            pts.extend(parse_coord_text(el.text))
    return pts


def local(tag: str) -> str:
    return tag.split("}")[-1]


def prune_empty_folders(el: ET.Element) -> bool:
    """Remove pastas sem Placemark. True se o elemento (ou descendentes) ainda têm conteúdo útil."""
    keep_children: list[ET.Element] = []
    useful = False
    for child in list(el):
        name = local(child.tag)
        if name in ("Folder", "Document"):
            if prune_empty_folders(child):
                keep_children.append(child)
                useful = True
            else:
                el.remove(child)
        elif name == "Placemark":
            keep_children.append(child)
            useful = True
        else:
            keep_children.append(child)
    return useful or local(el.tag) in ("kml",)


def clip_kml(src_kmz: Path, dst_kmz: Path, ring: list[tuple[float, float]]) -> dict[str, int]:
    with zipfile.ZipFile(src_kmz) as zf:
        kml_name = next(n for n in zf.namelist() if n.lower().endswith(".kml"))
        raw = zf.read(kml_name)

    root = ET.fromstring(raw)
    kept = 0
    dropped = 0
    by_geom = {"Point": 0, "LineString": 0, "Polygon": 0, "other": 0}
    parent_of = {child: parent for parent in root.iter() for child in list(parent)}

    for pm in list(root.iter(f"{{{KML_NS}}}Placemark")):
        pts = placemark_points(pm)
        if geom_hits_polygon(pts, ring):
            kept += 1
            if pm.find(f".//{{{KML_NS}}}Point") is not None:
                by_geom["Point"] += 1
            elif pm.find(f".//{{{KML_NS}}}LineString") is not None:
                by_geom["LineString"] += 1
            elif pm.find(f".//{{{KML_NS}}}Polygon") is not None:
                by_geom["Polygon"] += 1
            else:
                by_geom["other"] += 1
            continue
        parent = parent_of.get(pm)
        if parent is not None:
            parent.remove(pm)
            dropped += 1

    prune_empty_folders(root)

    # Polígono de recorte para conferência no Earth
    doc = root.find(f".//{{{KML_NS}}}Document")
    if doc is None:
        doc = root.find(f".//{{{KML_NS}}}Folder")
    if doc is not None:
        folder = ET.SubElement(doc, f"{{{KML_NS}}}Folder")
        ET.SubElement(folder, f"{{{KML_NS}}}name").text = "Área de recorte"
        ET.SubElement(folder, f"{{{KML_NS}}}open").text = "1"
        pm = ET.SubElement(folder, f"{{{KML_NS}}}Placemark")
        ET.SubElement(pm, f"{{{KML_NS}}}name").text = "Polígono informado"
        style = ET.SubElement(pm, f"{{{KML_NS}}}Style")
        line = ET.SubElement(style, f"{{{KML_NS}}}LineStyle")
        ET.SubElement(line, f"{{{KML_NS}}}color").text = "ff00ffff"
        ET.SubElement(line, f"{{{KML_NS}}}width").text = "3"
        poly_s = ET.SubElement(style, f"{{{KML_NS}}}PolyStyle")
        ET.SubElement(poly_s, f"{{{KML_NS}}}color").text = "3300ffff"
        poly = ET.SubElement(pm, f"{{{KML_NS}}}Polygon")
        outer = ET.SubElement(poly, f"{{{KML_NS}}}outerBoundaryIs")
        lr = ET.SubElement(outer, f"{{{KML_NS}}}LinearRing")
        coords = ET.SubElement(lr, f"{{{KML_NS}}}coordinates")
        closed = ring + [ring[0]]
        coords.text = " ".join(f"{lon:.8f},{lat:.8f},0" for lon, lat in closed)

    xml = ET.tostring(root, encoding="utf-8", xml_declaration=True)
    dst_kmz.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(dst_kmz, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("doc.kml", xml)

    return {"kept": kept, "dropped": dropped, **by_geom}


def main() -> int:
    src = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).with_name("Laje do Muriaé.kmz")
    dst = Path(sys.argv[2]) if len(sys.argv) > 2 else src.with_name("Laje do Muriaé - recorte.kmz")

    ring = [
        (dms_to_dec(42, 10, 15.57, "W"), dms_to_dec(21, 11, 32.36, "S")),  # sup. esq.
        (dms_to_dec(42, 4, 3.39, "W"), dms_to_dec(21, 11, 21.99, "S")),  # sup. dir.
        (dms_to_dec(42, 4, 5.22, "W"), dms_to_dec(21, 14, 3.39, "S")),  # inf. dir.
        (dms_to_dec(42, 8, 58.74, "W"), dms_to_dec(21, 14, 14.29, "S")),  # inf. esq.
    ]
    stats = clip_kml(src, dst, ring)
    print(f"origem: {src}")
    print(f"saida:  {dst}")
    print(f"poligono (lon, lat): {ring}")
    print(stats)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
