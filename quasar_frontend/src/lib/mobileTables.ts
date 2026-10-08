/**
 * Tabelas → cartões no telefone. Em telas estreitas uma tabela com muitas colunas obriga a rolar na
 * horizontal; aqui cada linha vira um cartão "rótulo: valor" (o CSS está em responsive.css, classe
 * `.tbl-cards`). Funciona em TODAS as telas sem mexer em cada tabela: um observador marca as tabelas
 * do `<main>` e dos modais, copiando o texto do cabeçalho para `data-label` de cada célula.
 *
 * Regras:
 *  - só quando a largura é de telefone (≤ 767px) e a tabela tem 4+ colunas;
 *  - tabelas com cabeçalho de várias linhas / colspan ficam como estão (rolagem horizontal contida);
 *  - `data-no-cards` na <table> desliga a conversão.
 */

const MOBILE_QUERY = "(max-width: 767px)";
const MIN_COLUMNS = 4;
/** Campos (além do título) que o cartão mostra recolhido; o resto aparece em «Ver mais». `data-card-keep` na <table> muda o número. */
const DEFAULT_CARD_KEEP = 4;
/** Altura (px) da faixa «Ver mais» no rodapé do cartão — o toque ali expande em vez de abrir o cartão. */
const MORE_STRIP_PX = 34;
/** Célula «de ações»: sem rótulo e com botão/link/menu. No cartão ela sobe para o canto superior direito, ao lado do título. */
const ACTION_SELECTOR = "button, a[href], [role='button'], [role='menuitem'], summary";

function cellText(el: Element): string {
  const t = (el.getAttribute("aria-label") || el.textContent || "").replace(/\s+/g, " ").trim();
  // Setas de ordenação (▲▼↑↓) não fazem parte do rótulo.
  return t.replace(/[▲▼↑↓↕⇅]+/g, "").trim();
}

function plainTable(table: HTMLTableElement) {
  table.classList.remove("tbl-cards");
  table.querySelectorAll("[data-card-extra]").forEach((c) => c.removeAttribute("data-card-extra"));
  table.querySelectorAll("[data-card-actions]").forEach((c) => c.removeAttribute("data-card-actions"));
  table.querySelectorAll("tr[data-card-more], tr[data-open]").forEach((r) => {
    r.removeAttribute("data-card-more");
    r.removeAttribute("data-open");
  });
}

export function enhanceTable(table: HTMLTableElement): void {
  if (table.hasAttribute("data-no-cards")) return;
  const head = table.tHead;
  if (!head || head.rows.length !== 1) return plainTable(table);
  const headCells = Array.from(head.rows[0].cells);
  if (headCells.length < MIN_COLUMNS || headCells.some((c) => c.colSpan > 1)) return plainTable(table);

  const labels = headCells.map(cellText);
  // 1ª coluna com texto = título do cartão (colunas de seleção/expansão vêm com cabeçalho vazio).
  const titleIdx = labels.findIndex((l) => l !== "");

  for (const body of Array.from(table.tBodies)) {
    for (const row of Array.from(body.rows)) {
      const cells = Array.from(row.cells);
      if (cells.length !== labels.length) {
        row.setAttribute("data-card-span", "");
        continue;
      }
      row.removeAttribute("data-card-span");
      const keep = Math.max(1, Number(table.getAttribute("data-card-keep")) || DEFAULT_CARD_KEEP);
      const valueIdx = cells.map((_, i) => i).filter((i) => i !== titleIdx && labels[i] !== "");
      // só recolhe quando esconde ao menos 2 campos (esconder 1 e mostrar «Ver mais» seria pior)
      const hideFrom = valueIdx.length - keep >= 2 ? keep : valueIdx.length;
      const extra = new Set(valueIdx.slice(hideFrom));
      let hasActions = false;
      if (extra.size > 0) row.setAttribute("data-card-more", "");
      else row.removeAttribute("data-card-more");
      cells.forEach((cell, i) => {
        if (extra.has(i)) cell.setAttribute("data-card-extra", "");
        else cell.removeAttribute("data-card-extra");
        if (cell.getAttribute("data-label") !== labels[i]) cell.setAttribute("data-label", labels[i]);
        if (i === titleIdx) cell.setAttribute("data-card-title", "");
        const isAction = labels[i] === "" && i !== titleIdx && cell.querySelector(ACTION_SELECTOR) !== null;
        if (isAction) {
          cell.setAttribute("data-card-actions", "");
          hasActions = true;
        } else cell.removeAttribute("data-card-actions");
        // Valor comprido (endereço, lista de VLANs…) ocupa a largura inteira do cartão; curto divide a linha em 2 colunas.
        const wide = (cell.textContent ?? "").trim().length > 22 || cell.querySelectorAll("*").length > 3;
        if (wide !== cell.hasAttribute("data-card-wide")) {
          if (wide) cell.setAttribute("data-card-wide", "");
          else cell.removeAttribute("data-card-wide");
        }
      });
      if (hasActions) row.setAttribute("data-card-actions", "");
      else row.removeAttribute("data-card-actions");
    }
  }
  table.classList.add("tbl-cards");
}

/**
 * Toque na faixa «Ver mais» no rodapé do cartão = expande/recolhe. Ouvinte na fase de CAPTURA do document para rodar antes
 * do onClick do React da linha (muitas linhas abrem o detalhe ao tocar) e impedir que o toque na faixa também o abra.
 * O estado fica em `data-open` (o React não mexe em atributos que ele não conhece).
 */
function onCardMoreTap(e: MouseEvent): void {
  const target = e.target as Element | null;
  const row = target?.closest?.("table.tbl-cards > tbody > tr[data-card-more]") as HTMLElement | null;
  if (!row) return;
  const bottom = row.getBoundingClientRect().bottom;
  if (bottom - e.clientY > MORE_STRIP_PX) return;
  e.stopPropagation();
  e.preventDefault();
  if (row.hasAttribute("data-open")) row.removeAttribute("data-open");
  else row.setAttribute("data-open", "");
}

function enhanceAll(root: ParentNode): void {
  root.querySelectorAll<HTMLTableElement>("table").forEach(enhanceTable);
}

/** Liga o observador (uma vez por app). Devolve a função que o desliga. */
export function installMobileTableCards(): () => void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return () => {};
  const mq = window.matchMedia(MOBILE_QUERY);
  let observer: MutationObserver | null = null;
  let raf = 0;

  const run = () => {
    raf = 0;
    enhanceAll(document);
  };
  const schedule = () => {
    if (!raf) raf = window.requestAnimationFrame(run);
  };

  const start = () => {
    if (observer) return;
    document.addEventListener("click", onCardMoreTap, true);
    run();
    // Só childList: alterar atributos/classes não pode re-disparar o observador.
    observer = new MutationObserver(schedule);
    observer.observe(document.body, { childList: true, subtree: true });
  };
  const stop = () => {
    document.removeEventListener("click", onCardMoreTap, true);
    observer?.disconnect();
    observer = null;
    if (raf) window.cancelAnimationFrame(raf);
    raf = 0;
    document.querySelectorAll<HTMLTableElement>("table.tbl-cards").forEach(plainTable);
  };
  const onChange = () => (mq.matches ? start() : stop());

  onChange();
  mq.addEventListener("change", onChange);
  return () => {
    mq.removeEventListener("change", onChange);
    stop();
  };
}
