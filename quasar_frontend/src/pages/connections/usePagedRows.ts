import { useEffect, useMemo, useState } from "react";

/** Paginação client-side simples (listas até ~5000 registos). */
export function usePagedRows<T>(rows: T[], pageSize: number, resetKey: string) {
  const [page, setPage] = useState(0);

  useEffect(() => {
    setPage(0);
  }, [resetKey, pageSize]);

  return useMemo(() => {
    const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
    const safePage = Math.min(page, totalPages - 1);
    const start = safePage * pageSize;
    const pageRows = rows.slice(start, start + pageSize);
    return {
      page,
      setPage,
      safePage,
      totalPages,
      pageRows,
      rangeFrom: rows.length === 0 ? 0 : start + 1,
      rangeTo: Math.min(rows.length, start + pageSize),
    };
  }, [rows, page, pageSize]);
}
