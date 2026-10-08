/**
 * Barras de abas viram uma linha rolável (ver accent-skin.css). Com muitas abas, a ativa pode ficar fora da área visível —
 * este observador a centraliza dentro da própria barra sempre que a aba ativa MUDA (clique, rota, `?tab=` na URL…).
 *
 * Só mexe na rolagem horizontal da barra (nunca na da página) e só quando a ativa mudou, para não brigar com o usuário
 * que está arrastando a barra manualmente.
 */

const CONTAINERS = ".tabs, .hubsoft-header__tabs, .hubsoft-tabs, .hsa-tabs";
const ACTIVE = ".active, .is-active, .hubsoft-tabs__btn--active, [aria-current='page']";

const lastActive = new WeakMap<Element, Element>();
/** Barras já posicionadas uma vez: o 1º posicionamento é instantâneo (abrir a tela), os seguintes animam. */
const positioned = new WeakSet<Element>();

function centerActive(root: ParentNode): void {
  root.querySelectorAll<HTMLElement>(CONTAINERS).forEach((bar) => {
    const el = bar.querySelector<HTMLElement>(ACTIVE);
    if (!el || lastActive.get(bar) === el) return;
    lastActive.set(bar, el);
    if (bar.scrollWidth <= bar.clientWidth + 1) return; // tudo cabe: nada a rolar
    const b = bar.getBoundingClientRect();
    const r = el.getBoundingClientRect();
    const delta = r.left - b.left - (b.width - r.width) / 2;
    if (Math.abs(delta) > 4) bar.scrollTo({ left: bar.scrollLeft + delta, behavior: positioned.has(bar) ? "smooth" : "auto" });
    positioned.add(bar);
  });
}

/** Liga o observador (uma vez por app). Devolve a função que o desliga. */
export function installScrollActiveTab(): () => void {
  if (typeof window === "undefined" || typeof MutationObserver === "undefined") return () => {};
  let timer = 0;
  const schedule = () => {
    if (timer) return;
    // setTimeout (e não requestAnimationFrame): rAF não dispara com a aba do navegador em segundo plano
    timer = window.setTimeout(() => {
      timer = 0;
      centerActive(document);
    }, 60);
  };
  const observer = new MutationObserver((muts) => {
    for (const m of muts) {
      const t = m.target as Element;
      if (m.type === "childList" || t.closest?.(CONTAINERS)) {
        schedule();
        return;
      }
    }
  });
  observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["class", "aria-current"] });
  schedule();
  return () => {
    observer.disconnect();
    if (timer) window.clearTimeout(timer);
  };
}
