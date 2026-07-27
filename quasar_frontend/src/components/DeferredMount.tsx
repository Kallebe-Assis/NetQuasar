import { useEffect, useRef, useState, type ReactNode } from "react";

/**
 * Só monta filhos (ex.: Recharts) quando a secção entra no viewport.
 * Mantém o first paint do dashboard leve.
 */
export function DeferredMount({
  children,
  rootMargin = "200px",
  minHeight = 120,
  placeholder,
}: {
  children: ReactNode;
  rootMargin?: string;
  minHeight?: number;
  placeholder?: ReactNode;
}) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el || visible) return;
    if (typeof IntersectionObserver === "undefined") {
      setVisible(true);
      return;
    }
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setVisible(true);
          io.disconnect();
        }
      },
      { rootMargin, threshold: 0.01 },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [rootMargin, visible]);

  return (
    <div ref={ref} style={{ minHeight: visible ? undefined : minHeight }}>
      {visible ? children : placeholder ?? <p className="mk-noc-muted" style={{ fontSize: 12, margin: 0 }}>A preparar gráfico…</p>}
    </div>
  );
}
