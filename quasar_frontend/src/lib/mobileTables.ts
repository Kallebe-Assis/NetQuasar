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

function cellText(el: Element): string {
  const t = (el.getAttribute("aria-label") || el.textContent || "").replace(/\s+/g, " ").trim();
  // Setas de ordenação (▲▼↑↓) não fazem parte do rótulo.
  return t.replace(/[▲▼↑↓⇅]+/g, "").trim();
}

function plainTable(table: HTMLTableElement) {
  table.classList.remove("tbl-cards");
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
      cells.forEach((cell, i) => {
        if (cell.getAttribute("data-label") !== labels[i]) cell.setAttribute("data-label", labels[i]);
        if (i === titleIdx) cell.setAttribute("data-card-title", "");
        // Valor comprido (endereço, lista de VLANs…) ocupa a largura inteira do cartão; curto divide a linha em 2 colunas.
        const wide = (cell.textContent ?? "").trim().length > 22 || cell.querySelectorAll("*").length > 3;
        if (wide !== cell.hasAttribute("data-card-wide")) {
          if (wide) cell.setAttribute("data-card-wide", "");
          else cell.removeAttribute("data-card-wide");
        }
      });
    }
  }
  table.classList.add("tbl-cards");
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
    run();
    // Só childList: alterar atributos/classes não pode re-disparar o observador.
    observer = new MutationObserver(schedule);
    observer.observe(document.body, { childList: true, subtree: true });
  };
  const stop = () => {
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
