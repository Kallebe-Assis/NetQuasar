import { useCallback, useState, type ReactNode } from "react";

export type SortDir = "asc" | "desc";
export type SortValue = string | number | boolean | null | undefined;

function isEmpty(v: SortValue): boolean {
  return v === null || v === undefined || (typeof v === "string" && v.trim() === "");
}

/** Compara dois valores de coluna: números como número, textos sem acento/caixa (pt-BR), vazios sempre no fim. */
export function compareSortValues(a: SortValue, b: SortValue, dir: SortDir): number {
  const ea = isEmpty(a);
  const eb = isEmpty(b);
  if (ea || eb) return ea === eb ? 0 : ea ? 1 : -1; // vazio sempre por último, nos dois sentidos
  let c: number;
  if (typeof a === "number" && typeof b === "number") c = a - b;
  else if (typeof a === "boolean" && typeof b === "boolean") c = Number(a) - Number(b);
  else c = String(a).localeCompare(String(b), "pt-BR", { numeric: true, sensitivity: "base" });
  return dir === "asc" ? c : -c;
}

/** Estado de ordenação por clique no cabeçalho: 1º clique asc, 2º desc, 3º volta à ordem original. */
export function useTableSort<K extends string>(initial?: { key: K; dir: SortDir }) {
  const [sort, setSort] = useState<{ key: K; dir: SortDir } | null>(initial ?? null);
  const toggle = useCallback((key: K) => {
    setSort((cur) => {
      if (!cur || cur.key !== key) return { key, dir: "asc" };
      if (cur.dir === "asc") return { key, dir: "desc" };
      return null;
    });
  }, []);
  const sortRows = useCallback(
    <T,>(rows: T[], value: (row: T, key: K) => SortValue): T[] => {
      if (!sort) return rows;
      return rows
        .map((r, i) => ({ r, i }))
        .sort((x, y) => compareSortValues(value(x.r, sort.key), value(y.r, sort.key), sort.dir) || x.i - y.i)
        .map((x) => x.r);
    },
    [sort],
  );
  return { sort, toggle, sortRows };
}

/** Cabeçalho clicável com seta (▲ crescente / ▼ decrescente). */
export function SortableTh<K extends string>({
  col,
  sort,
  onSort,
  children,
  className,
}: {
  col: K;
  sort: { key: K; dir: SortDir } | null;
  onSort: (k: K) => void;
  children: ReactNode;
  className?: string;
}) {
  const active = sort?.key === col;
  return (
    <th
      className={`th-sortable${className ? ` ${className}` : ""}`}
      aria-sort={active ? (sort!.dir === "asc" ? "ascending" : "descending") : "none"}
      onClick={() => onSort(col)}
      title="Clique para ordenar"
      style={{ cursor: "pointer", userSelect: "none", whiteSpace: "nowrap" }}
    >
      {children}
      <span aria-hidden style={{ marginLeft: 4, fontSize: 9, opacity: active ? 1 : 0.3 }}>
        {active ? (sort!.dir === "asc" ? "▲" : "▼") : "↕"}
      </span>
    </th>
  );
}
