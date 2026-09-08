/** Guarda global de "alterações não salvas" — usado pela Topologia (geral e 2D do POP) para
 * avisar antes de sair da tela com edições por salvar. Estado num módulo simples (não Context)
 * porque só uma tela "editora de diagrama" fica montada de cada vez, e quem precisa de ler isto
 * (o interceptador de cliques em links no ShellLayout, que envolve TODA a aplicação) fica bem
 * acima da tela na árvore — um Context exigiria um Provider lá em cima só para isto.
 *
 * ShellLayout.tsx intercepta cliques em qualquer <a> (menu lateral e links dentro da própria
 * página, ex.: o botão "Voltar") em captura, consulta getUnsavedGuard() e, se houver alterações
 * por salvar, mostra um modal em vez de navegar — três opções: cancelar, sair sem salvar, salvar
 * e sair. beforeunload cobre fechar a aba/actualizar/digitar outro URL (diálogo nativo do
 * browser, sem forma de personalizar os botões — limitação do próprio browser). */

import { useEffect } from "react";

export type UnsavedGuard = {
  dirty: boolean;
  /** Deve resolver só depois de gravar com sucesso — "salvar e sair" espera por isto antes de navegar. */
  save: () => Promise<void>;
};

let current: UnsavedGuard | null = null;

export function setUnsavedGuard(guard: UnsavedGuard | null): void {
  current = guard;
}

export function getUnsavedGuard(): UnsavedGuard | null {
  return current;
}

/** Regista (e desregista ao desmontar/mudar) o estado "sujo" desta tela no guarda global, e liga
 * o beforeunload nativo enquanto houver algo por salvar. `save` deve ser estável (useCallback)
 * para não reinstalar os efeitos a cada render. */
export function useUnsavedChangesGuard(dirty: boolean, save: () => Promise<void>): void {
  useEffect(() => {
    setUnsavedGuard(dirty ? { dirty, save } : null);
    return () => setUnsavedGuard(null);
  }, [dirty, save]);

  useEffect(() => {
    if (!dirty) return;
    function handler(e: BeforeUnloadEvent) {
      e.preventDefault();
      e.returnValue = "";
    }
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirty]);
}
