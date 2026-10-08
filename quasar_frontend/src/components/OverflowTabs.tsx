import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type OverflowTabItem<T extends string> = { key: T; label: string; icon: LucideIcon; group?: string };

const NARROW_QUERY = "(max-width: 1023px)";
/** folga para a aba ativa (negrito = um pouco mais larga que a medida sem destaque) */
const MEASURE_SLACK_PX = 8;

function useNarrow(): boolean {
  const [narrow, setNarrow] = useState(() => typeof window !== "undefined" && window.matchMedia(NARROW_QUERY).matches);
  useEffect(() => {
    const mq = window.matchMedia(NARROW_QUERY);
    const on = () => setNarrow(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return narrow;
}

/**
 * Barra de abas.
 *
 *  - Tela larga (> 1023 px): mede a largura disponível e move as abas que não couberem para um botão «···» com
 *    dropdown («outras telas») — em vez de deixar a barra quebrar linha ou vazar pela margem. A medida é feita numa
 *    linha invisível com a MESMA classe `.tabs` (mesmo padding, borda e fonte das abas reais) e usa a posição real de
 *    cada aba, então divisores de grupo e espaçamentos entram na conta. Recalcula em resize, zoom e quando a fonte carrega.
 *  - Tela estreita (≤ 1023 px): sem «···» — TODAS as abas numa única linha com rolagem horizontal (a ativa é centralizada
 *    por lib/scrollTabs.ts).
 *  - `group` (opcional) separa as abas em grupos com um divisor fino entre eles.
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
  const measureRowRef = useRef<HTMLDivElement>(null);
  const measureRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const moreBtnRef = useRef<HTMLButtonElement>(null);
  const [visibleCount, setVisibleCount] = useState(items.length);
  const [moreOpen, setMoreOpen] = useState(false);
  const narrow = useNarrow();
  // `items` é um array novo a cada render do pai: a dependência é a assinatura do CONTEÚDO
  const signature = useMemo(() => items.map((it) => `${it.key}:${it.label}:${it.group ?? ""}`).join("|"), [items]);

  useLayoutEffect(() => {
    function recompute() {
      const container = containerRef.current;
      const row = measureRowRef.current;
      if (!container || !row || items.length === 0) return;
      const available = container.clientWidth;
      const base = row.getBoundingClientRect().left;
      // borda direita (relativa ao início da linha) de cada aba, já com divisores e espaçamentos
      const rights = items.map((it) => {
        const el = measureRefs.current[it.key];
        return el ? el.getBoundingClientRect().right - base : 0;
      });
      if (rights[rights.length - 1] + MEASURE_SLACK_PX <= available) {
        setVisibleCount(items.length);
        return;
      }
      const moreWidth = (moreBtnRef.current?.getBoundingClientRect().width ?? 44) + 6;
      let count = 0;
      for (let i = 0; i < items.length; i++) {
        if (rights[i] + moreWidth + MEASURE_SLACK_PX > available) break;
        count = i + 1;
      }
      setVisibleCount(Math.max(1, count));
    }

    recompute();
    const ro = new ResizeObserver(recompute);
    if (containerRef.current) ro.observe(containerRef.current);
    window.addEventListener("resize", recompute);
    // a fonte da página pode terminar de carregar depois da 1ª medida (muda a largura das abas)
    void document.fonts?.ready.then(recompute);
    return () => {
      ro.disconnect();
      window.removeEventListener("resize", recompute);
    };
  }, [signature, items, narrow]);

  useEffect(() => {
    if (!moreOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setMoreOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [moreOpen]);

  const visibleItems = narrow ? items : items.slice(0, visibleCount);
  const overflowItems = narrow ? [] : items.slice(visibleCount);

  return (
    <div ref={wrapRef} style={{ position: "relative" }}>
      {/* Linha de medição — fora do fluxo visual; mesma classe `.tabs`, então as abas têm o tamanho real */}
      <div
        ref={measureRowRef}
        aria-hidden
        className="tabs"
        style={{ position: "absolute", visibility: "hidden", pointerEvents: "none", top: -9999, left: -9999, width: "max-content", overflow: "visible" }}
      >
        {items.map((it, i) => (
          <Fragment key={it.key}>
            {i > 0 && it.group !== items[i - 1].group ? <span className="tabs__sep" /> : null}
            <button
              ref={(el) => {
                measureRefs.current[it.key] = el;
              }}
              type="button"
              tabIndex={-1}
            >
              <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                <it.icon size={14} aria-hidden />
                {it.label}
              </span>
            </button>
          </Fragment>
        ))}
        <button ref={moreBtnRef} type="button" tabIndex={-1}>
          <MoreHorizontal size={16} />
        </button>
      </div>

      <div ref={containerRef} className="tabs" style={narrow ? undefined : { overflow: "hidden" }}>
        {visibleItems.map((it, i) => (
          <Fragment key={it.key}>
            {i > 0 && it.group !== visibleItems[i - 1].group ? <span className="tabs__sep" aria-hidden /> : null}
            <button type="button" className={active === it.key ? "active" : ""} onClick={() => onSelect(it.key)}>
              <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                <it.icon size={14} aria-hidden />
                {it.label}
              </span>
            </button>
          </Fragment>
        ))}
        {overflowItems.length > 0 ? (
          <button
            type="button"
            className={overflowItems.some((it) => it.key === active) ? "active" : ""}
            onClick={() => setMoreOpen((v) => !v)}
            aria-expanded={moreOpen}
            aria-haspopup="menu"
            aria-label={`Outras telas (${overflowItems.length})`}
            title={`Outras telas (${overflowItems.length})`}
          >
            <MoreHorizontal size={16} aria-hidden />
          </button>
        ) : null}
      </div>
      {/* O menu fica FORA do container (overflow:hidden cortaria o dropdown). */}
      {moreOpen && overflowItems.length > 0 ? (
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
  );
}
