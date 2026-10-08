/**
 * Textos explicativos somem no celular. Telas de ferramentas/configuração têm parágrafos longos de ajuda («Lista de IPs e
 * de portas (máx. 64 combinações)…») que fazem sentido no computador mas só poluem uma tela estreita.
 *
 * Duas frentes:
 *  1. Classes de «descrição/dica» conhecidas são escondidas só por CSS (ver `responsive.css`, bloco «Textos explicativos»).
 *  2. Parágrafos soltos com estilo «muted» (a maioria das páginas escreve `<p style={{ color: "var(--muted)" }}>`) não têm
 *     classe — este observador marca com `data-mobile-hint` os que são LONGOS (≥ 100 caracteres), sem botão/link/campo e fora de
 *     modais/alertas. Mensagens curtas («Nenhuma autorização registada ainda.») e avisos nunca são tocados.
 *
 * Escape: `data-mobile-keep` num elemento (ou ancestral) mantém o texto no celular.
 */

const MOBILE_QUERY = "(max-width: 767px)";
const MIN_HINT_CHARS = 100;
const CANDIDATES = "main p, main .muted, main .hsa-muted";

function isMutedStyle(el: HTMLElement): boolean {
  return el.style.color.includes("--muted") || el.classList.contains("muted") || el.classList.contains("hsa-muted");
}

function qualifies(el: HTMLElement): boolean {
  if (!isMutedStyle(el)) return false;
  if (el.closest("[data-mobile-keep], .modal, [role='alert'], [role='status'], .msg, .toast-message, label")) return false;
  if (el.querySelector("button, a, input, select, textarea")) return false;
  return (el.textContent ?? "").replace(/\s+/g, " ").trim().length >= MIN_HINT_CHARS;
}

function enhanceAll(root: ParentNode): void {
  root.querySelectorAll<HTMLElement>(CANDIDATES).forEach((el) => {
    const hint = qualifies(el);
    if (hint !== el.hasAttribute("data-mobile-hint")) {
      if (hint) el.setAttribute("data-mobile-hint", "");
      else el.removeAttribute("data-mobile-hint");
    }
  });
}

/** Liga o observador (uma vez por app, só em tela de celular). Devolve a função que o desliga. */
export function installMobileHints(): () => void {
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
    // só childList/characterData: alterar atributos não pode re-disparar o observador
    observer = new MutationObserver(schedule);
    observer.observe(document.body, { childList: true, subtree: true, characterData: true });
  };
  const stop = () => {
    observer?.disconnect();
    observer = null;
    if (raf) window.cancelAnimationFrame(raf);
    raf = 0;
    document.querySelectorAll("[data-mobile-hint]").forEach((el) => el.removeAttribute("data-mobile-hint"));
  };
  const onChange = () => (mq.matches ? start() : stop());

  onChange();
  mq.addEventListener("change", onChange);
  return () => {
    mq.removeEventListener("change", onChange);
    stop();
  };
}

/** Exposto para teste. */
export { enhanceAll as enhanceMobileHints };
