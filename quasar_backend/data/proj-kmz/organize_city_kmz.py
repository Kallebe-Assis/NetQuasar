"""Recorta um KMZ, padroniza nomes de CTO e organiza pastas por célula."""
from __future__ import annotations

import math
import re
import zipfile
import xml.etree.ElementTree as ET
from collections import defaultdict
from pathlib import Path

from clip_kml_polygon import (
    GX_NS,
    KML_NS,
    dms_to_dec,
    geom_hits_polygon,
    placemark_points,
)

NS = f"{{{KML_NS}}}"
GXN = f"{{{GX_NS}}}"
ET.register_namespace("", KML_NS)
ET.register_namespace("gx", GX_NS)

CTO_RE = re.compile(
    r"^C[\s,.\-_]*(\d+)[\s,.\-_]+(\d+)$",
    re.IGNORECASE,
)
# Ex.: "01-16", "1.06" (sem prefixo C) — comum no dump do app
CTO_BARE_RE = re.compile(r"^(\d{1,2})[\s,.\-_]+(\d{1,2})$")
CELULA_RE = re.compile(r"c[eé]lula[\s\-_]*0*(\d+)", re.IGNORECASE)
CAIXA_NUM_RE = re.compile(
    r"(?:caixa[^0-9]{0,24}|distribui[cç][aã]o\s*c)0*(\d+)",
    re.IGNORECASE,
)
EQUIP_KEYS = (
    "olt",
    "escrit",
    "mikrotik",
    "concentrador",
    "borda",
    "ptp",
    "bng",
    "torre",
    "servidor",
    "roteador",
    "energia",
    "conex",
    # POP é tipo próprio (is_pop)
)


def local(tag: str) -> str:
    return tag.split("}")[-1]


def el_name(el: ET.Element | None) -> str:
    if el is None:
        return ""
    nm = el.find(f"{NS}name")
    return (nm.text or "").strip() if nm is not None else ""


def set_name(el: ET.Element, text: str) -> None:
    nm = el.find(f"{NS}name")
    if nm is None:
        nm = ET.SubElement(el, f"{NS}name")
    nm.text = text


def force_visible(el: ET.Element) -> None:
    """Revisão G2 grava visibility=0; no KMZ final todos os pins/pastas têm de aparecer."""
    vis = el.find(f"{NS}visibility")
    if vis is None:
        vis = ET.SubElement(el, f"{NS}visibility")
    vis.text = "1"


def geom_kind(pm: ET.Element) -> str:
    if pm.find(f".//{NS}Point") is not None:
        return "point"
    if pm.find(f".//{NS}LineString") is not None:
        return "line"
    if pm.find(f".//{NS}Polygon") is not None:
        return "poly"
    return "other"


def first_coord(pm: ET.Element) -> tuple[float, float] | None:
    pts = placemark_points(pm)
    return pts[0] if pts else None


def coord_key(pm: ET.Element, ndigits: int = 5) -> str:
    pts = placemark_points(pm)
    if not pts:
        return ""
    return ";".join(f"{lon:.{ndigits}f},{lat:.{ndigits}f}" for lon, lat in pts)


def source_rank(path: str) -> int:
    low = path.lower()
    if "revis" in low:
        return 0
    if any(
        k in low
        for k in (
            "miracema",
            "areia",
            "paraíso",
            "paraiso",
            "tobias",
            "alegre",
            "pirapetinga",
            "estrela",
            "dalva",
            "palma",
            "flores",
            "venda",
            "laje",
            "muria",
            "pádua",
            "padua",
            "célula",
            "celula",
        )
    ):
        return 1
    return 2


def style_id_num(style_url: str) -> int:
    m = re.search(r"(\d+)$", (style_url or "").lstrip("#"))
    return int(m.group(1)) if m else 10**9


def collect_style_colors(root: ET.Element) -> dict[str, str]:
    out: dict[str, str] = {}
    for st in root.iter(f"{NS}Style"):
        sid = st.get("id")
        if not sid:
            continue
        col = (
            st.findtext(f"{NS}IconStyle/{NS}color")
            or st.findtext(f"{NS}LineStyle/{NS}color")
            or ""
        )
        out[sid] = col
    return out


def placemark_color(pm: ET.Element, color_by_id: dict[str, str]) -> str:
    su = (pm.findtext(f"{NS}styleUrl") or "").lstrip("#")
    inline = (
        pm.findtext(f".//{NS}IconStyle/{NS}color")
        or pm.findtext(f".//{NS}LineStyle/{NS}color")
        or ""
    )
    return inline or color_by_id.get(su, "") or su


def autoname_unnamed_by_color(
    kept: list[ET.Element], color_by_id: dict[str, str]
) -> dict[int, int]:
    """Se não há CTOs nomeados, agrupa pinos sem nome pela cor do estilo e gera C1-01…"""
    hints: dict[int, int] = {}
    if any(normalize_cto_name(el_name(pm)) for pm in kept):
        return hints

    groups: dict[str, list[ET.Element]] = defaultdict(list)
    for pm in kept:
        if geom_kind(pm) != "point":
            continue
        if el_name(pm).strip():
            continue
        groups[placemark_color(pm, color_by_id)].append(pm)

    cell_colors: list[tuple[str, list[ET.Element]]] = []
    for col, pms in groups.items():
        uniq: list[ET.Element] = []
        seen: set[tuple[float, float] | int] = set()
        for pm in pms:
            pt = first_coord(pm)
            key: tuple[float, float] | int = (
                (round(pt[0], 5), round(pt[1], 5)) if pt else id(pm)
            )
            if key in seen:
                continue
            seen.add(key)
            uniq.append(pm)
        if len(uniq) >= 8:
            cell_colors.append((col, uniq))
    cell_colors.sort(key=lambda it: -len(it[1]))

    for cell, (col, uniq) in enumerate(cell_colors, start=1):
        ordered = sorted(
            uniq, key=lambda pm: style_id_num(pm.findtext(f"{NS}styleUrl") or "")
        )
        coord_name: dict[tuple[float, float], str] = {}
        for i, pm in enumerate(ordered, start=1):
            nm = f"C{cell}-{i:02d}"
            set_name(pm, nm)
            hints[id(pm)] = cell
            pt = first_coord(pm)
            if pt:
                coord_name[(round(pt[0], 5), round(pt[1], 5))] = nm
        for pm in groups[col]:
            hints[id(pm)] = cell
            pt = first_coord(pm)
            if pt:
                key = (round(pt[0], 5), round(pt[1], 5))
                if key in coord_name and not el_name(pm).strip():
                    set_name(pm, coord_name[key])

    color_to_cell = {col: i for i, (col, _) in enumerate(cell_colors, start=1)}
    for pm in kept:
        if geom_kind(pm) != "line":
            continue
        cell = color_to_cell.get(placemark_color(pm, color_by_id))
        if cell:
            hints[id(pm)] = cell
    return hints


def autoname_unnamed_in_cell_folders(kept: list[ET.Element], path_of) -> None:
    """Células que só têm pinos sem nome (ex. Pirapetinga 07/08) viram C7-01…"""
    unnamed: dict[int, list[ET.Element]] = defaultdict(list)
    named: dict[int, int] = defaultdict(int)
    for pm in kept:
        if geom_kind(pm) != "point":
            continue
        cell = folder_cell(path_of(pm))
        if cell is None:
            continue
        n = el_name(pm).strip()
        if normalize_cto_name(n):
            named[cell] += 1
        elif not n or n.lower().startswith("marcador"):
            unnamed[cell].append(pm)
    for cell, pms in unnamed.items():
        if named[cell] > 0 or len(pms) < 8:
            continue
        ordered = sorted(
            pms, key=lambda pm: style_id_num(pm.findtext(f"{NS}styleUrl") or "")
        )
        seen: set[tuple[float, float] | int] = set()
        i = 0
        for pm in ordered:
            pt = first_coord(pm)
            key: tuple[float, float] | int = (
                (round(pt[0], 5), round(pt[1], 5)) if pt else id(pm)
            )
            if key in seen:
                continue
            seen.add(key)
            i += 1
            set_name(pm, f"C{cell}-{i:02d}")


def haversine_m(a: tuple[float, float], b: tuple[float, float]) -> float:
    dlat = (a[1] - b[1]) * 111_000.0
    dlon = (a[0] - b[0]) * 111_000.0 * math.cos(math.radians((a[1] + b[1]) / 2))
    return math.hypot(dlat, dlon)


def normalize_cto_name(name: str) -> str | None:
    n = name.strip().lstrip("¿?¡!").strip()
    m = CTO_RE.match(n)
    if not m:
        m = CTO_BARE_RE.match(n)
    if not m:
        return None
    cell, num = int(m.group(1)), int(m.group(2))
    if cell < 1 or cell > 99 or num < 1 or num > 99:
        return None
    # Typo frequente: C81-8 em vez de C1-8
    if cell == 81:
        cell = 1
    return f"C{cell}-{num:02d}"


def folder_cell(path: str) -> int | None:
    m = re.search(r"celula[\s\-_]*0*(\d+)", path, re.I)
    if m:
        return int(m.group(1))
    m = re.search(r"dalva[\s\-_]*0*(\d+)", path, re.I)
    if m:
        return int(m.group(1))
    return None


def is_pop(name: str, path: str) -> bool:
    blob = f"{name} {path}".lower()
    return bool(re.search(r"(^|[\s/\-_])pops?($|[\s/\-_])", blob))


def is_equipamento(name: str, path: str) -> bool:
    blob = f"{name} {path}".lower()
    if "equipamento" in blob:
        return True
    if any(k in blob for k in EQUIP_KEYS):
        return True
    # "ap" isolado (não casa "Pirapetinga" / "mapa")
    return bool(re.search(r"(^|[\s/\-_])ap($|[\s/\-_])", blob))


def is_tourist(name: str, path: str) -> bool:
    blob = f"{name} {path}".lower()
    if "meus lugares" in blob or "projeto 5g" in blob:
        return True
    return bool(
        re.search(
            r"eiffel|titanic|grand canyon|london eye|sydney|monte fuji|"
            r"cristo redentor|bas[ií]lica|cidade proibida|sede do google",
            blob,
        )
    )


def equip_group(name: str, path: str) -> str:
    blob = f"{name} {path}".lower()
    rules = (
        ("OLT", ("olt",)),
        ("PTP", ("ptp",)),
        ("Torre", ("torre",)),
        ("Escritório", ("escrit",)),
        ("MikroTik", ("mikrotik", "concentrador")),
        ("BNG / Borda", ("bng", "bgp", "borda")),
        ("Energia", ("energia",)),
        ("Conexão", ("conex",)),
    )
    for label, keys in rules:
        if any(k in blob for k in keys):
            return label
    if re.search(r"(^|[\s/\-_])ap($|[\s/\-_])", blob):
        return "AP"
    m = re.search(r"equipamentos[^/]*/\s*([^/]+)", path, re.I)
    if m:
        return m.group(1).strip()
    return "Outros"


def classify(name: str, path: str, kind: str) -> tuple[str, int | None]:
    """Retorna (tipo, célula). Célula pode ser inferida depois por proximidade."""
    n = name.strip()
    low = n.lower()
    cell = folder_cell(path)

    cto = normalize_cto_name(n)
    if cto:
        m = CTO_RE.match(cto)
        return "cto", int(m.group(1)) if m else cell

    if re.fullmatch(r"c[eé]lula", n, re.I):
        return "caixa", cell
    m_cel = CELULA_RE.search(n)
    if m_cel:
        return "caixa", int(m_cel.group(1))

    if "caixa" in low or "emenda" in low:
        m = CAIXA_NUM_RE.search(n) or re.search(r"c0*(\d+)$", n, re.I)
        if m:
            return "caixa", int(m.group(1))
        return "caixa", cell

    if is_pop(n, path):
        return "pop", None

    if is_equipamento(n, path):
        return "equip", None

    if kind == "line" or "medida" in low or "caminho" in low:
        return "cabo", cell

    if low.startswith("marcador"):
        return "marcador", cell

    if not n:
        return "sem_nome", cell

    if kind == "poly":
        return "poly", cell

    return "outro", cell


def mk_folder(parent: ET.Element, name: str, open_: bool = False) -> ET.Element:
    f = ET.SubElement(parent, f"{NS}Folder")
    ET.SubElement(f, f"{NS}name").text = name
    ET.SubElement(f, f"{NS}open").text = "1" if open_ else "0"
    return f


def add_clip_polygon(parent: ET.Element, ring: list[tuple[float, float]]) -> None:
    folder = mk_folder(parent, "Área de recorte", True)
    pm = ET.SubElement(folder, f"{NS}Placemark")
    ET.SubElement(pm, f"{NS}name").text = "Polígono informado"
    style = ET.SubElement(pm, f"{NS}Style")
    line = ET.SubElement(style, f"{NS}LineStyle")
    ET.SubElement(line, f"{NS}color").text = "ff00ffff"
    ET.SubElement(line, f"{NS}width").text = "3"
    poly_s = ET.SubElement(style, f"{NS}PolyStyle")
    ET.SubElement(poly_s, f"{NS}color").text = "3300ffff"
    poly = ET.SubElement(pm, f"{NS}Polygon")
    outer = ET.SubElement(poly, f"{NS}outerBoundaryIs")
    lr = ET.SubElement(outer, f"{NS}LinearRing")
    coords = ET.SubElement(lr, f"{NS}coordinates")
    closed = ring + [ring[0]]
    coords.text = " ".join(f"{lon:.8f},{lat:.8f},0" for lon, lat in closed)


def nearest_cell(pt: tuple[float, float], centroids: dict[int, tuple[float, float]], max_km: float = 12.0) -> int | None:
    if not centroids:
        return None
    lon, lat = pt
    best = None
    best_d = 1e18
    for cell, (clon, clat) in centroids.items():
        dlat = (lat - clat) * 111.0
        dlon = (lon - clon) * 111.0 * math.cos(math.radians(lat))
        d = math.hypot(dlat, dlon)
        if d < best_d:
            best_d = d
            best = cell
    if best is not None and best_d <= max_km:
        return best
    return None


def collect_styles(root: ET.Element) -> list[ET.Element]:
    styles: list[ET.Element] = []
    seen: set[str] = set()
    for el in root.iter():
        tag = local(el.tag)
        if tag not in ("Style", "StyleMap") and "CascadingStyle" not in tag:
            continue
        sid = el.get("id") or ""
        key = f"{tag}:{sid}:{id(el)}"
        if sid:
            if sid in seen:
                continue
            seen.add(sid)
        styles.append(el)
    return styles


def organize_group(
    kept: list[ET.Element],
    path_of,
    color_by_id: dict[str, str],
    autoname: bool = True,
) -> tuple[list[tuple[str, int | None, str, ET.Element]], int]:
    cell_hints: dict[int, int] = {}
    if autoname:
        cell_hints = autoname_unnamed_by_color(kept, color_by_id)
        autoname_unnamed_in_cell_folders(kept, path_of)

    raw_items: list[tuple[str, int | None, str, ET.Element, str]] = []
    for pm in kept:
        raw_name = el_name(pm)
        path = path_of(pm)
        kind = geom_kind(pm)
        cto = normalize_cto_name(raw_name)
        if cto:
            set_name(pm, cto)
            name = cto
        else:
            name = raw_name
        tipo, cell = classify(name, path, kind)
        if cell is None:
            cell = cell_hints.get(id(pm))
        if tipo == "caixa" and cell is not None and CELULA_RE.search(name):
            name = f"Célula {cell:02d}"
            set_name(pm, name)
        raw_items.append((tipo, cell, name, pm, path))

    def better(path_a: str, path_b: str) -> bool:
        ra, rb = source_rank(path_a), source_rank(path_b)
        if ra != rb:
            return ra < rb
        return False

    # Só remove cópia idêntica (mesmo nome + quase a mesma coordenada).
    # Levantamentos diferentes (Revisão vs app) usam o mesmo C1-06 a dezenas
    # de metros — os dois precisam ficar, senão sobram cabos sem pino.
    DUP_M = 2.5
    chosen: list[tuple[str, int | None, str, ET.Element, str]] = []
    skipped_dup = 0
    for tipo, cell, name, pm, path in raw_items:
        pt = first_coord(pm)
        ck = coord_key(pm, 6)
        dup_idx = None
        for i, (ptipo, _pcell, pname, ppm, ppath) in enumerate(chosen):
            if ptipo != tipo or pname != name:
                continue
            if tipo == "cabo":
                if ck and ck == coord_key(ppm, 6):
                    dup_idx = i
                    break
                continue
            ppt = first_coord(ppm)
            if pt is None or ppt is None:
                continue
            if haversine_m(pt, ppt) <= DUP_M:
                dup_idx = i
                break
        if dup_idx is not None:
            skipped_dup += 1
            if better(path, chosen[dup_idx][4]):
                chosen[dup_idx] = (tipo, cell, name, pm, path)
            continue
        chosen.append((tipo, cell, name, pm, path))

    items = [(t, c, n, pm) for t, c, n, pm, _p in chosen]

    centroids: dict[int, tuple[float, float]] = {}
    acc: dict[int, list[tuple[float, float]]] = defaultdict(list)
    for tipo, cell, name, pm in items:
        if tipo == "cto" and cell is not None:
            pt = first_coord(pm)
            if pt:
                acc[cell].append(pt)
    for cell, pts in acc.items():
        centroids[cell] = (
            sum(p[0] for p in pts) / len(pts),
            sum(p[1] for p in pts) / len(pts),
        )

    resolved: list[tuple[str, int | None, str, ET.Element]] = []
    for tipo, cell, name, pm in items:
        if autoname and cell is None and tipo in ("cabo", "caixa", "marcador", "sem_nome", "outro"):
            pt = first_coord(pm)
            if pt:
                cell = nearest_cell(pt, centroids)
        resolved.append((tipo, cell, name, pm))

    leftover: dict[int, list[int]] = defaultdict(list)
    if autoname:
        for i, (tipo, cell, name, pm) in enumerate(resolved):
            if tipo == "sem_nome" and cell is not None:
                leftover[cell].append(i)
        for cell, idxs in leftover.items():
            if len(idxs) < 8:
                continue
            existing_cto = sum(1 for t, c, _n, _pm in resolved if t == "cto" and c == cell)
            if existing_cto >= 8:
                continue
            ordered = sorted(
                idxs,
                key=lambda i: style_id_num(resolved[i][3].findtext(f"{NS}styleUrl") or ""),
            )
            seen: set[tuple[float, float] | int] = set()
            n = 0
            for i in ordered:
                pm = resolved[i][3]
                pt = first_coord(pm)
                key: tuple[float, float] | int = (
                    (round(pt[0], 5), round(pt[1], 5)) if pt else id(pm)
                )
                if key in seen:
                    continue
                seen.add(key)
                n += 1
                nm = f"C{cell}-{n:02d}"
                set_name(pm, nm)
                resolved[i] = ("cto", cell, nm, pm)

    resolved.sort(key=lambda it: (it[1] or 999, it[0], it[2]))
    return resolved, skipped_dup


def prune_empty_folders(folder: ET.Element) -> None:
    for ch in list(folder):
        if local(ch.tag) == "Folder":
            prune_empty_folders(ch)
            if next(ch.iter(f"{NS}Placemark"), None) is None:
                folder.remove(ch)


def typed_subfolders(parent: ET.Element) -> dict[str, ET.Element]:
    return {
        "cto": mk_folder(parent, "CTOs", True),
        "caixa": mk_folder(parent, "Caixas", True),
        "cabo": mk_folder(parent, "Cabos"),
        "marcador": mk_folder(parent, "Marcadores"),
        "sem_nome": mk_folder(parent, "Sem nome"),
        "outro": mk_folder(parent, "Outros"),
        "poly": mk_folder(parent, "Áreas"),
    }


GENERIC_CABLE_RE = re.compile(
    r"^(medida(\s+do\s+caminho)?|caminho(\s+sem\s+t[ií]tulo)?|linha|cabo(\s*\d+)?|"
    r"marcador(\s+\d+)?|marcador\s+sem\s+t[ií]tulo|as(\s+\d+)?)$",
    re.IGNORECASE,
)


def is_junk_point(tipo: str, name: str, kind: str) -> bool:
    if kind != "point":
        return False
    if tipo in ("sem_nome", "marcador"):
        return True
    n = name.strip().lower()
    return (not n) or n.startswith("marcador")


def cto_has_photo(pm: ET.Element) -> bool:
    return "<img" in (pm.findtext(f"{NS}description") or "").lower()


def cto_keep_score(pm: ET.Element) -> tuple[int, int]:
    """Menor = melhor: foto de campo primeiro, depois estilo mais recente."""
    return (0 if cto_has_photo(pm) else 1, -style_id_num(pm.findtext(f"{NS}styleUrl") or ""))


def unify_same_name_ctos(
    resolved: list[tuple[str, int | None, str, ET.Element]],
    merge_m: float = 80.0,
) -> list[tuple[str, int | None, str, ET.Element]]:
    """Um pin por C{n}-{nn}. Cópia sem foto (dump do app) sai; duas fotos longe são renomeadas."""
    groups: dict[str, list[int]] = defaultdict(list)
    for i, (tipo, _cell, name, _pm) in enumerate(resolved):
        if tipo == "cto":
            groups[name].append(i)
    drop: set[int] = set()
    rename: dict[int, str] = {}
    used = set(groups)
    extra_names: set[str] = set()

    def next_free(cell: int | None) -> str:
        c = cell if cell and cell > 0 else 1
        n = 1
        while True:
            cand = f"C{c}-{n:02d}"
            if cand not in used and cand not in extra_names:
                return cand
            n += 1

    for _name, idxs in groups.items():
        if len(idxs) < 2:
            continue
        ranked = sorted(idxs, key=lambda i: cto_keep_score(resolved[i][3]))
        primary = ranked[0]
        prim_pt = first_coord(resolved[primary][3])
        for i in ranked[1:]:
            pm = resolved[i][3]
            pt = first_coord(pm)
            close = bool(prim_pt and pt and haversine_m(prim_pt, pt) <= merge_m)
            if (not cto_has_photo(pm)) or close:
                drop.add(i)
                continue
            new = next_free(resolved[i][1])
            extra_names.add(new)
            used.add(new)
            rename[i] = new

    out: list[tuple[str, int | None, str, ET.Element]] = []
    for i, (tipo, cell, name, pm) in enumerate(resolved):
        if i in drop:
            continue
        if i in rename:
            name = rename[i]
            set_name(pm, name)
            m = CTO_RE.match(name)
            if m:
                cell = int(m.group(1))
        out.append((tipo, cell, name, pm))
    return out


def polish_city_items(
    resolved: list[tuple[str, int | None, str, ET.Element]],
) -> list[tuple[str, int | None, str, ET.Element]]:
    """Um CTO por nome/célula, cabos Cabo 01…; pinos da área não são descartados."""
    resolved = unify_same_name_ctos(resolved)
    kept: list[tuple[str, int | None, str, ET.Element]] = []
    leftovers: list[tuple[str, int | None, str, ET.Element]] = []
    for tipo, cell, name, pm in resolved:
        kind = geom_kind(pm)
        if is_junk_point(tipo, name, kind):
            leftovers.append((tipo, cell, name, pm))
            continue
        kept.append((tipo, cell, name, pm))

    cto_pts: list[tuple[float, float]] = []
    used_names: set[str] = set()
    for tipo, cell, name, pm in kept:
        if tipo == "cto":
            used_names.add(name)
            pt = first_coord(pm)
            if pt:
                cto_pts.append(pt)

    def next_cto_name(cell: int | None) -> str:
        c = cell if cell and cell > 0 else 1
        n = 1
        while True:
            cand = f"C{c}-{n:02d}"
            if cand not in used_names:
                used_names.add(cand)
                return cand
            n += 1

    for tipo, cell, name, pm in leftovers:
        if tipo in ("pop", "equip"):
            kept.append((tipo, cell, name, pm))
            continue
        pt = first_coord(pm)
        if pt is None:
            continue
        if any(haversine_m(pt, other) <= 12.0 for other in cto_pts):
            continue
        if cell is None:
            continue
        new = next_cto_name(cell)
        set_name(pm, new)
        kept.append(("cto", cell, new, pm))
        cto_pts.append(pt)

    cable_n: dict[int | None, int] = defaultdict(int)
    caixa_n: dict[tuple[int | None, str], int] = defaultdict(int)
    out: list[tuple[str, int | None, str, ET.Element]] = []
    for tipo, cell, name, pm in kept:
        if tipo == "cabo":
            raw = name.strip()
            if not raw or GENERIC_CABLE_RE.fullmatch(raw) or raw.lower().startswith("medida") or raw.lower().startswith("caminho") or raw.lower().startswith("marcador"):
                cable_n[cell] += 1
                name = f"Cabo {cable_n[cell]:02d}"
                set_name(pm, name)
        elif tipo == "caixa":
            low = name.lower()
            if "emenda" in low:
                label = "Emenda"
            elif "distrib" in low:
                label = "Caixa de distribuição"
            elif re.search(r"c[eé]lula", low):
                label = f"Célula {cell:02d}" if cell else "Célula"
            else:
                label = name.strip() or "Caixa"
            if cell and label not in (f"Célula {cell:02d}",):
                caixa_n[(cell, label)] += 1
                n = caixa_n[(cell, label)]
                name = f"{label} {cell:02d}" if n == 1 else f"{label} {cell:02d}-{n}"
            else:
                name = label
            set_name(pm, name)
        out.append((tipo, cell, name, pm))
    return out


def name_backbone_placemarks(folder: ET.Element) -> None:
    n_line = 0
    n_emenda = 0
    for pm in folder.iter(f"{NS}Placemark"):
        name = el_name(pm)
        kind = geom_kind(pm)
        if kind == "line":
            n_line += 1
            if not name.strip() or GENERIC_CABLE_RE.fullmatch(name.strip()):
                set_name(pm, f"Backbone {n_line:02d}")
        elif kind == "point":
            low = name.lower()
            if "pop" in low or "olt" in low:
                continue
            if "emenda" in low or not name.strip():
                n_emenda += 1
                set_name(pm, f"Emenda BB {n_emenda:02d}")


def fill_locality_folders(
    parent: ET.Element,
    resolved: list[tuple[str, int | None, str, ET.Element]],
) -> dict[str, int]:
    counts: dict[str, int] = defaultdict(int)
    pop_folder: ET.Element | None = None
    for tipo, _cell, _name, pm in resolved:
        if tipo != "pop":
            continue
        if pop_folder is None:
            pop_folder = mk_folder(parent, "POPs", True)
        force_visible(pm)
        pop_folder.append(pm)
        counts["POPs"] += 1
    cells = sorted({c for _, c, _, _ in resolved if c is not None})
    cell_folders: dict[int, dict[str, ET.Element]] = {}
    for c in cells:
        cf = mk_folder(parent, f"Célula {c:02d}", True)
        cell_folders[c] = typed_subfolders(cf)
    outros = mk_folder(parent, "Outros")
    outros_map = typed_subfolders(outros)
    for tipo, cell, name, pm in resolved:
        if tipo in ("equip", "pop"):
            continue
        force_visible(pm)
        dest_kind = tipo if tipo in outros_map else "outro"
        if cell in cell_folders:
            cell_folders[cell][dest_kind].append(pm)
            counts[f"C{cell:02d}/{dest_kind}"] += 1
        else:
            outros_map[dest_kind].append(pm)
            counts[f"Outros/{dest_kind}"] += 1
    for folder in parent.iter(f"{NS}Folder"):
        force_visible(folder)
    return counts


def load_kml_bytes(src: Path) -> bytes:
    raw = src.read_bytes()
    if raw[:2] == b"PK":
        with zipfile.ZipFile(src) as zf:
            kml_name = next(n for n in zf.namelist() if n.lower().endswith(".kml"))
            return zf.read(kml_name)
    if raw.startswith(b"\xef\xbb\xbf"):
        return raw[3:]
    return raw


def load_kml_root(src: Path) -> tuple[ET.Element, object]:
    raw = load_kml_bytes(src)
    root = ET.fromstring(raw)
    parent_of = {child: parent for parent in root.iter() for child in list(parent)}

    def path_of(el: ET.Element) -> str:
        parts: list[str] = []
        cur: ET.Element | None = el
        while cur is not None:
            if local(cur.tag) in ("Folder", "Document"):
                t = el_name(cur)
                if t:
                    parts.append(t)
            cur = parent_of.get(cur)
        return " / ".join(reversed(parts))

    return root, path_of


def is_padua_source_kml(path: Path) -> bool:
    n = path.name.lower()
    if path.suffix.lower() != ".kml":
        return False
    if n.startswith("padua_c"):
        return True
    has_city = "padua" in n or "pdua" in n or "santo" in n
    return has_city and ("06" in n or "célula" in n or "celula" in n)


def cells_hinted_by_filename(name: str) -> list[int]:
    n = name.lower()
    found: list[int] = []
    for m in re.finditer(r"(?:^|[_\-\s])c(\d+)(?=$|[_\-\s.])", n):
        found.append(int(m.group(1)))
    for m in re.finditer(r"c[eé]lula[\s_\-]*0*(\d+)", n):
        found.append(int(m.group(1)))
    out: list[int] = []
    seen: set[int] = set()
    for c in found:
        if 1 <= c <= 99 and c not in seen:
            seen.add(c)
            out.append(c)
    return out


def reassign_to_nearest_allowed_cells(
    resolved: list[tuple[str, int | None, str, ET.Element]],
    allowed: set[int],
) -> list[tuple[str, int | None, str, ET.Element]]:
    centroids: dict[int, tuple[float, float]] = {}
    acc: dict[int, list[tuple[float, float]]] = defaultdict(list)
    for tipo, cell, _name, pm in resolved:
        if tipo != "cto" or cell not in allowed:
            continue
        pt = first_coord(pm)
        if pt:
            acc[cell].append(pt)
    for cell, pts in acc.items():
        centroids[cell] = (
            sum(p[0] for p in pts) / len(pts),
            sum(p[1] for p in pts) / len(pts),
        )
    if not centroids:
        return resolved
    out: list[tuple[str, int | None, str, ET.Element]] = []
    for tipo, cell, name, pm in resolved:
        if tipo == "cto":
            out.append((tipo, cell, name, pm))
            continue
        raw = name.strip()
        generic = (
            not raw
            or GENERIC_CABLE_RE.fullmatch(raw)
            or raw.lower().startswith("medida")
            or raw.lower().startswith("caminho")
            or raw.lower().startswith("marcador")
            or raw.lower().startswith("as")
        )
        if tipo in ("cabo", "marcador", "sem_nome", "outro") and generic:
            pt = first_coord(pm)
            if pt:
                near = nearest_cell(pt, centroids, max_km=8.0)
                if near in allowed:
                    cell = near
        out.append((tipo, cell, name, pm))
    return out


def build_from_source_kmls(
    sources: list[Path],
    dst: Path,
    city_name: str,
    include_backbone: bool = True,
) -> dict[str, int]:
    """Une vários KML/KMZ num KMZ organizado por Célula 01, Célula 02, …"""
    all_styles: list[ET.Element] = []
    all_pms: list[ET.Element] = []
    path_map: dict[int, str] = {}
    mixed_ids: set[int] = set()
    mixed_allowed: set[int] = set()

    for i, src in enumerate(sources):
        root, path_of = load_kml_root(src)
        prefix_style_ids(root, f"s{i}_")
        all_styles.extend(collect_styles(root))
        file_cells = cells_hinted_by_filename(src.name)
        # C7+C8 vêm no mesmo ficheiro/pasta «Célula 08» — cabos sem nome vão à célula mais próxima.
        mixed = 7 in file_cells and 8 in file_cells
        for pm in list(root.iter(f"{NS}Placemark")):
            path_map[id(pm)] = f"{src.name} / {path_of(pm)}"
            all_pms.append(pm)
            if mixed:
                mixed_ids.add(id(pm))
                mixed_allowed.update(file_cells)

    def path_of_merged(el: ET.Element) -> str:
        return path_map.get(id(el), "")

    fake = ET.Element(f"{NS}kml")
    for st in all_styles:
        fake.append(st)
    color_by_id = collect_style_colors(fake)

    resolved, skipped_dup = organize_group(all_pms, path_of_merged, color_by_id)
    if mixed_ids and mixed_allowed:
        mixed_items = [it for it in resolved if id(it[3]) in mixed_ids]
        others = [it for it in resolved if id(it[3]) not in mixed_ids]
        mixed_items = reassign_to_nearest_allowed_cells(mixed_items, mixed_allowed)
        resolved = others + mixed_items
    resolved = polish_city_items(resolved)

    kml = ET.Element(f"{NS}kml")
    doc = ET.SubElement(kml, f"{NS}Document")
    ET.SubElement(doc, f"{NS}name").text = city_name
    ET.SubElement(doc, f"{NS}open").text = "1"
    for st in all_styles:
        doc.append(st)

    counts: dict[str, int] = defaultdict(int)
    counts.update(fill_locality_folders(doc, resolved))

    if include_backbone:
        backbone_dir = sources[0].parent / "backbone"
        if backbone_dir.is_dir():
            for fp in sorted(backbone_dir.iterdir()):
                if fp.suffix.lower() not in (".kml", ".kmz"):
                    continue
                if backbone_city_from_filename(fp.name) != city_name:
                    continue
                prefix = "bb" + re.sub(r"[^a-z0-9]+", "", city_name.lower()) + "_"
                bb = load_backbone_folder(fp, prefix)
                name_backbone_placemarks(bb)
                doc.append(bb)
                counts["backbone"] = counts.get("backbone", 0) + 1

    prune_empty_folders(doc)
    write_kmz(dst, kml)
    cells = sorted({c for _, c, _, _ in resolved if c is not None})
    stats = {
        "sources": len(sources),
        "kept_raw": len(all_pms),
        "kept_after_dedup": len(resolved),
        "skipped_dup": skipped_dup,
        "cells": len(cells),
    }
    stats.update(counts)
    return stats


def build_padua_from_project_kmls(base: Path) -> dict[str, int]:
    sources = sorted((p for p in base.iterdir() if is_padua_source_kml(p)), key=lambda p: p.name.lower())
    if not sources:
        raise FileNotFoundError("nenhum KML de Pádua (padua_C*.kml / Célula 06) em " + str(base))
    dst = base / "Santo Antônio de Pádua.kmz"
    stats = build_from_source_kmls(sources, dst, "Santo Antônio de Pádua")
    stats["saida"] = str(dst)
    for i, p in enumerate(sources, start=1):
        stats[f"src_{i}"] = p.name
    return stats


def fix_city_kmz(src: Path) -> dict[str, int]:
    """Remove CTOs duplicadas dum KMZ já organizado (fica a cópia com foto)."""
    root, path_of = load_kml_root(src)
    parent_of = {child: parent for parent in root.iter() for child in list(parent)}
    items: list[tuple[str, int | None, str, ET.Element]] = []
    for pm in list(root.iter(f"{NS}Placemark")):
        name = el_name(pm)
        kind = geom_kind(pm)
        cto = normalize_cto_name(name)
        if cto:
            set_name(pm, cto)
            m = CTO_RE.match(cto)
            items.append(("cto", int(m.group(1)) if m else None, cto, pm))
            continue
        tipo, cell = classify(name, path_of(pm), kind)
        items.append((tipo, cell, name, pm))
    before = sum(1 for t, _c, _n, _pm in items if t == "cto")
    kept = unify_same_name_ctos(items)
    keep_ids = {id(pm) for _t, _c, _n, pm in kept}
    removed = 0
    for tipo, _cell, _name, pm in items:
        if tipo != "cto" or id(pm) in keep_ids:
            continue
        parent = parent_of.get(pm)
        if parent is None:
            continue
        parent.remove(pm)
        removed += 1
    prune_empty_folders(root)
    write_kmz(src, root)
    after = sum(1 for t, _c, _n, _pm in kept if t == "cto")
    before_names = {n for t, _c, n, _pm in items if t == "cto"}
    after_names = {n for t, _c, n, _pm in kept if t == "cto"}
    return {
        "cto_antes": before,
        "cto_depois": after,
        "removidas": removed,
        "nomes_antes": len(before_names),
        "nomes_depois": len(after_names),
    }


def write_kmz(dst: Path, kml: ET.Element) -> None:
    for el in kml.iter():
        tag = local(el.tag)
        if tag in ("Placemark", "Folder", "Document"):
            force_visible(el)
    xml = ET.tostring(kml, encoding="utf-8", xml_declaration=True)
    dst.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(dst, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("doc.kml", xml)


def build_city_kmz(
    src: Path,
    dst: Path,
    ring: list[tuple[float, float]],
    city_name: str,
) -> dict[str, int]:
    root, path_of = load_kml_root(src)
    kept: list[ET.Element] = []
    dropped = 0
    for pm in list(root.iter(f"{NS}Placemark")):
        pts = placemark_points(pm)
        if geom_hits_polygon(pts, ring):
            kept.append(pm)
        else:
            dropped += 1

    styles = collect_styles(root)
    color_by_id = collect_style_colors(root)
    resolved, skipped_dup = organize_group(kept, path_of, color_by_id)
    resolved = polish_city_items(resolved)

    kml = ET.Element(f"{NS}kml")
    doc = ET.SubElement(kml, f"{NS}Document")
    ET.SubElement(doc, f"{NS}name").text = city_name
    ET.SubElement(doc, f"{NS}open").text = "1"
    for st in styles:
        doc.append(st)

    equip_root = mk_folder(doc, "Equipamentos", True)
    counts: dict[str, int] = defaultdict(int)
    equip_subs: dict[str, ET.Element] = {}
    for tipo, cell, name, pm in resolved:
        if tipo != "equip":
            continue
        force_visible(pm)
        gname = equip_group(name, path_of(pm))
        if gname not in equip_subs:
            equip_subs[gname] = mk_folder(equip_root, gname, True)
        equip_subs[gname].append(pm)
        counts["Equipamentos"] += 1

    counts.update(fill_locality_folders(doc, resolved))
    backbone_dir = src.parent / "backbone"
    if backbone_dir.is_dir():
        for fp in sorted(backbone_dir.iterdir()):
            if fp.suffix.lower() not in (".kml", ".kmz"):
                continue
            if backbone_city_from_filename(fp.name) != city_name:
                continue
            prefix = "bb" + re.sub(r"[^a-z0-9]+", "", city_name.lower()) + "_"
            bb = load_backbone_folder(fp, prefix)
            name_backbone_placemarks(bb)
            doc.append(bb)
            counts["backbone"] = counts.get("backbone", 0) + 1
    prune_empty_folders(doc)
    write_kmz(dst, kml)
    cells = sorted({c for _, c, _, _ in resolved if c is not None})
    stats = {
        "kept_raw": len(kept),
        "dropped": dropped,
        "kept_after_dedup": len(resolved),
        "skipped_dup": skipped_dup,
        "cells": len(cells),
    }
    stats.update(counts)
    return stats


CITY_ORDER = (
    "Laje do Muriaé",
    "Venda das Flores",
    "Palma",
    "Paraíso do Tobias",
    "Monte Alegre",
    "Miracema",
    "Pirapetinga",
    "Estrela Dalva",
    "Santo Antônio de Pádua",
)

PATH_CITY = (
    ("pádua", "Santo Antônio de Pádua"),
    ("padua", "Santo Antônio de Pádua"),
    ("pirapetinga", "Pirapetinga"),
    ("estrela", "Estrela Dalva"),
    ("dalva", "Estrela Dalva"),
    ("miracema", "Miracema"),
    ("paraíso", "Paraíso do Tobias"),
    ("paraiso", "Paraíso do Tobias"),
    ("tobias", "Paraíso do Tobias"),
    ("areia", "Paraíso do Tobias"),
    ("alegre", "Monte Alegre"),
    ("palma", "Palma"),
    ("flores", "Venda das Flores"),
    ("laje", "Laje do Muriaé"),
    ("muria", "Laje do Muriaé"),
)


def city_from_path(path: str) -> str | None:
    low = path.lower()
    for key, name in PATH_CITY:
        if key in low:
            return name
    return None


def is_padua(name: str, path: str) -> bool:
    blob = f"{name} {path}".lower()
    return "pádua" in blob or "padua" in blob


def prefix_style_ids(root: ET.Element, prefix: str) -> None:
    for el in root.iter():
        sid = el.get("id")
        if sid:
            el.set("id", prefix + sid)
        su = el.find(f"{NS}styleUrl")
        if su is not None and su.text:
            t = su.text.strip()
            if t.startswith("#"):
                su.text = "#" + prefix + t[1:]


def backbone_city_from_filename(name: str) -> str | None:
    low = name.lower()
    if "padua" in low or "pádua" in low:
        return "Santo Antônio de Pádua"
    if "monte" in low or "alegre" in low:
        return "Monte Alegre"
    if "miracema" in low:
        return "Miracema"
    if "laje" in low or "muria" in low or "areia" in low:
        return "Laje do Muriaé"
    if "palma" in low:
        return "Palma"
    if "pirapetinga" in low:
        return "Pirapetinga"
    if "estrela" in low or "dalva" in low:
        return "Estrela Dalva"
    if "paraíso" in low or "paraiso" in low or "tobias" in low:
        return "Paraíso do Tobias"
    if "flores" in low or "venda" in low:
        return "Venda das Flores"
    return None


def load_backbone_folder(kml_path: Path, style_prefix: str) -> ET.Element:
    if kml_path.suffix.lower() == ".kmz":
        with zipfile.ZipFile(kml_path) as zf:
            kml_name = next(n for n in zf.namelist() if n.lower().endswith(".kml"))
            raw = zf.read(kml_name)
    else:
        raw = kml_path.read_bytes()
    root = ET.fromstring(raw)
    prefix_style_ids(root, style_prefix)
    doc = root.find(f"{NS}Document")
    bb = ET.Element(f"{NS}Folder")
    ET.SubElement(bb, f"{NS}name").text = "Backbone"
    ET.SubElement(bb, f"{NS}open").text = "1"
    if doc is None:
        return bb
    for ch in list(doc):
        tag = local(ch.tag)
        if tag in ("name", "open", "visibility", "description"):
            continue
        bb.append(ch)
    return bb


def ring_centroid(ring: list[tuple[float, float]]) -> tuple[float, float]:
    return (
        sum(p[0] for p in ring) / len(ring),
        sum(p[1] for p in ring) / len(ring),
    )


def assign_city(
    pm: ET.Element,
    path: str,
    cities: list[tuple[str, list[tuple[float, float]]]],
) -> str | None:
    pts = placemark_points(pm)
    hits: list[tuple[float, str]] = []
    pt = pts[0] if pts else None
    for name, ring in cities:
        if not geom_hits_polygon(pts, ring):
            continue
        if pt:
            hits.append((haversine_m(pt, ring_centroid(ring)), name))
        else:
            hits.append((0.0, name))
    if hits:
        hits.sort()
        return hits[0][1]
    return city_from_path(path)


def build_completo_kmz(src: Path, dst: Path) -> dict[str, int]:
    root, path_of = load_kml_root(src)
    styles = collect_styles(root)
    color_by_id = collect_style_colors(root)
    cities = [(name, ring) for _key, (name, ring) in CITIES.items()]

    equip: list[ET.Element] = []
    by_city: dict[str, list[ET.Element]] = defaultdict(list)
    skipped_tourist = 0
    for pm in list(root.iter(f"{NS}Placemark")):
        path = path_of(pm)
        name = el_name(pm)
        if is_tourist(name, path):
            skipped_tourist += 1
            continue
        if is_equipamento(name, path) and not is_pop(name, path):
            equip.append(pm)
            continue
        city = assign_city(pm, path, cities)
        by_city[city or "Fora das localidades"].append(pm)

    kml = ET.Element(f"{NS}kml")
    doc = ET.SubElement(kml, f"{NS}Document")
    ET.SubElement(doc, f"{NS}name").text = "Completo"
    ET.SubElement(doc, f"{NS}open").text = "1"
    for st in styles:
        doc.append(st)

    stats: dict[str, int] = defaultdict(int)
    stats["tourist_skip"] = skipped_tourist

    equip_root = mk_folder(doc, "Equipamentos", True)
    equip_resolved, equip_dups = organize_group(equip, path_of, color_by_id)
    stats["equip_dups"] = equip_dups
    equip_subs: dict[str, ET.Element] = {}
    for tipo, cell, name, pm in equip_resolved:
        gname = equip_group(name, path_of(pm))
        if gname not in equip_subs:
            equip_subs[gname] = mk_folder(equip_root, gname, True)
        equip_subs[gname].append(pm)
        stats["Equipamentos"] += 1

    backbone_dir = src.parent / "backbone"
    backbone_by_city: dict[str, list[ET.Element]] = defaultdict(list)
    if backbone_dir.is_dir():
        for fp in sorted(backbone_dir.iterdir()):
            if fp.suffix.lower() not in (".kml", ".kmz"):
                continue
            cname = backbone_city_from_filename(fp.name)
            if not cname:
                continue
            prefix = "bb" + re.sub(r"[^a-z0-9]+", "", cname.lower()) + "_"
            bb = load_backbone_folder(fp, prefix)
            backbone_by_city[cname].append(bb)
            stats[f"{cname}/backbone"] = sum(1 for _ in bb.iter(f"{NS}Placemark"))

    seen_cities: list[str] = []
    for cname in CITY_ORDER:
        if cname in by_city or cname in backbone_by_city:
            seen_cities.append(cname)
    for cname in sorted(set(by_city) | set(backbone_by_city)):
        if cname not in seen_cities:
            seen_cities.append(cname)

    for cname in seen_cities:
        loc = mk_folder(doc, cname, True)
        for bb in backbone_by_city.get(cname, []):
            loc.append(bb)
        pms = by_city.get(cname, [])
        if pms:
            resolved, dups = organize_group(
                pms, path_of, color_by_id, autoname=cname != "Fora das localidades"
            )
            if cname != "Fora das localidades":
                resolved = polish_city_items(resolved)
            counts = fill_locality_folders(loc, resolved)
            stats[f"{cname}/items"] = len(resolved)
            stats[f"{cname}/dups"] = dups
            stats["kept_after_dedup"] += len(resolved)
            stats["skipped_dup"] += dups
            for k, v in counts.items():
                stats[f"{cname}/{k}"] = v

    prune_empty_folders(doc)
    write_kmz(dst, kml)
    stats["cities"] = len(seen_cities)
    return dict(stats)


CITIES: dict[str, tuple[str, list[tuple[float, float]]]] = {
    "laje": (
        "Laje do Muriaé",
        [
            (dms_to_dec(42, 10, 15.57, "W"), dms_to_dec(21, 11, 32.36, "S")),
            (dms_to_dec(42, 4, 3.39, "W"), dms_to_dec(21, 11, 21.99, "S")),
            (dms_to_dec(42, 4, 5.22, "W"), dms_to_dec(21, 14, 3.39, "S")),
            (dms_to_dec(42, 8, 58.74, "W"), dms_to_dec(21, 14, 14.29, "S")),
        ],
    ),
    "miracema": (
        "Miracema",
        [
            # superior esquerda → direita → inferior direita → esquerda
            (dms_to_dec(42, 13, 56.79, "W"), dms_to_dec(21, 22, 49.43, "S")),
            (dms_to_dec(42, 9, 3.11, "W"), dms_to_dec(21, 23, 0.54, "S")),
            (dms_to_dec(42, 8, 51.06, "W"), dms_to_dec(21, 27, 31.56, "S")),
            (dms_to_dec(42, 15, 27.89, "W"), dms_to_dec(21, 26, 57.75, "S")),
        ],
    ),
    "paraiso": (
        "Paraíso do Tobias",
        [
            (dms_to_dec(42, 8, 53.55, "W"), dms_to_dec(21, 23, 29.22, "S")),
            (dms_to_dec(42, 4, 40.85, "W"), dms_to_dec(21, 23, 24.31, "S")),
            (dms_to_dec(42, 4, 29.86, "W"), dms_to_dec(21, 27, 3.51, "S")),
            (dms_to_dec(42, 9, 0.79, "W"), dms_to_dec(21, 26, 33.97, "S")),
        ],
    ),
    "montealegre": (
        "Monte Alegre",
        [
            (dms_to_dec(42, 3, 32.59, "W"), dms_to_dec(21, 26, 59.55, "S")),
            (dms_to_dec(42, 1, 5.13, "W"), dms_to_dec(21, 27, 7.82, "S")),
            (dms_to_dec(42, 1, 14.71, "W"), dms_to_dec(21, 28, 54.85, "S")),
            (dms_to_dec(42, 3, 32.62, "W"), dms_to_dec(21, 28, 35.84, "S")),
        ],
    ),
    "pirapetinga": (
        "Pirapetinga",
        [
            (dms_to_dec(42, 22, 41.09, "W"), dms_to_dec(21, 38, 22.43, "S")),
            (dms_to_dec(42, 18, 54.04, "W"), dms_to_dec(21, 38, 0.55, "S")),
            (dms_to_dec(42, 19, 3.95, "W"), dms_to_dec(21, 40, 18.80, "S")),
            (dms_to_dec(42, 22, 26.67, "W"), dms_to_dec(21, 40, 26.65, "S")),
        ],
    ),
    "estreladalva": (
        "Estrela Dalva",
        [
            (dms_to_dec(42, 28, 31.01, "W"), dms_to_dec(21, 44, 2.20, "S")),
            (dms_to_dec(42, 26, 56.41, "W"), dms_to_dec(21, 43, 56.80, "S")),
            (dms_to_dec(42, 26, 51.09, "W"), dms_to_dec(21, 45, 25.60, "S")),
            (dms_to_dec(42, 28, 24.71, "W"), dms_to_dec(21, 45, 22.53, "S")),
        ],
    ),
    "palma": (
        "Palma",
        [
            (dms_to_dec(42, 21, 23.36, "W"), dms_to_dec(21, 20, 41.06, "S")),
            (dms_to_dec(42, 16, 47.84, "W"), dms_to_dec(21, 20, 30.11, "S")),
            (dms_to_dec(42, 16, 31.94, "W"), dms_to_dec(21, 23, 19.05, "S")),
            (dms_to_dec(42, 21, 12.11, "W"), dms_to_dec(21, 23, 40.78, "S")),
        ],
    ),
    "vendadasflores": (
        "Venda das Flores",
        [
            (dms_to_dec(42, 10, 23.42, "W"), dms_to_dec(21, 19, 13.85, "S")),
            (dms_to_dec(42, 7, 8.70, "W"), dms_to_dec(21, 18, 49.02, "S")),
            (dms_to_dec(42, 6, 56.44, "W"), dms_to_dec(21, 21, 47.35, "S")),
            (dms_to_dec(42, 10, 22.53, "W"), dms_to_dec(21, 22, 1.01, "S")),
        ],
    ),
    "padua": (
        "Santo Antônio de Pádua",
        [
            (dms_to_dec(42, 14, 50.86, "W"), dms_to_dec(21, 30, 49.81, "S")),
            (dms_to_dec(42, 8, 53.58, "W"), dms_to_dec(21, 30, 28.69, "S")),
            (dms_to_dec(42, 9, 21.27, "W"), dms_to_dec(21, 33, 37.15, "S")),
            (dms_to_dec(42, 14, 42.90, "W"), dms_to_dec(21, 33, 42.97, "S")),
        ],
    ),
}


def main() -> int:
    import shutil
    import sys

    base = Path(__file__).resolve().parent
    key = (sys.argv[1] if len(sys.argv) > 1 else "completo").strip().lower()
    origem = base / "Completo.origem.kmz"
    src = base / "Completo.kmz"
    if key in ("padua-real", "padua-cidade", "padua-files"):
        stats = build_padua_from_project_kmls(base)
        print("origem:")
        for k in sorted(stats):
            if str(k).startswith("src_"):
                print(f"  {stats[k]}")
        print(f"saida:  {stats.get('saida', '')}")
        for k in sorted(stats):
            if str(k).startswith("src_") or k == "saida":
                continue
            print(f"  {k}: {stats[k]}")
        return 0
    if key in ("miracema-fix", "fix-miracema"):
        dst = base / "Miracema.kmz"
        if not dst.exists():
            print(f"ficheiro inexistente: {dst}")
            return 2
        stats = fix_city_kmz(dst)
        print(f"saida:  {dst}")
        for k in sorted(stats):
            print(f"  {k}: {stats[k]}")
        return 0
    if key in ("completo", "all", "tudo"):
        if not origem.exists():
            shutil.copy2(src, origem)
            print(f"backup: {origem}")
        stats = build_completo_kmz(origem, src)
        print(f"origem: {origem}")
        print(f"saida:  {src}")
        for k in sorted(stats):
            print(f"  {k}: {stats[k]}")
        return 0
    if key not in CITIES:
        print(f"cidade desconhecida: {key}; use: completo, padua-real, {', '.join(sorted(CITIES))}")
        return 2
    city_name, ring = CITIES[key]
    src_city = origem if origem.exists() else src
    dst = base / f"{city_name}.kmz"
    stats = build_city_kmz(src_city, dst, ring, city_name)
    print(f"origem: {src_city}")
    print(f"saida:  {dst}")
    print(f"poligono (lon, lat): {ring}")
    for k in sorted(stats):
        print(f"  {k}: {stats[k]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
