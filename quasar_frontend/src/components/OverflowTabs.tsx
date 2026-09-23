import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type OverflowTabItem<T extends string> = { key: T; label: string; icon: LucideIcon };

/**
 * Barra de abas que mede a largura disponível e move as que não couberem numa única linha para
 * um botão "···" com dropdown — em vez de deixar a barra quebrar linha (o que empurrava o
 * conteúdo da página para baixo de forma inconsistente). Reage a resize da janela e a zoom (que
 * também dispara "resize"), recalculando a cada mudança — reduzir o zoom devolve itens do "···"
 * para a linha normal, sem precisar de recarregar a página.
 */
export function OverflowTabs<T extends string>({
  items,
  active,
  onSelect,
}: {
  items: OverflowTabItem<T>[];
  active: T;
  onSelect: (key: T) => void;
}) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const measureRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const moreBtnRef = useRef<HTMLButtonElement>(null);
  const [visibleCount, setVisibleCount] = useState(items.length);
  const [moreOpen, setMoreOpen] = useState(false);

  useLayoutEffect(() => {
    function recompute() {
      const container = containerRef.current;
      if (!container) return;
      const available = container.clientWidth;
      const widths = items.map((it) => (measureRefs.current[it.key]?.offsetWidth ?? 0) + 4);
      const totalWidth = widths.reduce((a, b) => a + b, 0);
      if (totalWidth <= available) {
        setVisibleCount(items.length);
        return;
      }
      const moreWidth = (moreBtnRef.current?.offsetWidth ?? 40) + 4;
      let used = moreWidth;
      let count = 0;
      for (let i = 0; i < items.length; i++) {
        if (used + widths[i] > available) break;
        used += widths[i];
        count++;
      }
      setVisibleCount(Math.max(1, count));
    }

    recompute();
    const ro = new ResizeObserver(recompute);
    if (containerRef.current) ro.observe(containerRef.current);
    window.addEventListener("resize", recompute);
    return () => {
      ro.disconnect();
      window.removeEventListener("resize", recompute);
    };
  }, [items]);

  useEffect(() => {
    if (!moreOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setMoreOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [moreOpen]);

  const visibleItems = items.slice(0, visibleCount);
  const overflowItems = items.slice(visibleCount);

  return (
    <div ref={wrapRef} style={{ position: "relative" }}>
      {/* Linha de medição — fora do fluxo visual, só para saber a largura natural de cada aba */}
      <div aria-hidden style={{ position: "absolute", visibility: "hidden", pointerEvents: "none", top: -9999, left: -9999, display: "flex", gap: 4 }}>
        {items.map((it) => (
          <button
            key={it.key}
            ref={(el) => {
              measureRefs.current[it.key] = el;
            }}
            type="button"
          >
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
              <it.icon size={14} aria-hidden />
              {it.label}
            </span>
          </button>
        ))}
        <button ref={moreBtnRef} type="button">
          <MoreHorizontal size={14} />
        </button>
      </div>

      <div ref={containerRef} className="tabs" style={{ flexWrap: "nowrap", overflow: "hidden" }}>
        {visibleItems.map((it) => (
          <button key={it.key} type="button" className={active === it.key ? "active" : ""} onClick={() => onSelect(it.key)}>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
              <it.icon size={14} aria-hidden />
              {it.label}
            </span>
          </button>
        ))}
        {overflowItems.length > 0 ? (
          <div style={{ position: "relative" }}>
            <button
              type="button"
              className={overflowItems.some((it) => it.key === active) ? "active" : ""}
              onClick={() => setMoreOpen((v) => !v)}
              aria-expanded={moreOpen}
              aria-label="Mais abas"
              title="Mais abas"
            >
              <MoreHorizontal size={16} aria-hidden />
            </button>
            {moreOpen ? (
              <div className="tabs-overflow-menu" role="menu">
                {overflowItems.map((it) => (
                  <button
                    key={it.key}
                    type="button"
                    className={`tabs-overflow-menu__item${active === it.key ? " active" : ""}`}
                    role="menuitem"
                    onClick={() => {
                      onSelect(it.key);
                      setMoreOpen(false);
                    }}
                  >
                    <it.icon size={14} aria-hidden />
                    {it.label}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
