import { Search, X } from "lucide-react";

/**
 * Pesquisa local (sem chamada à API) no menu lateral — controlada pelo ShellLayout, que usa o
 * texto para filtrar a própria árvore de navegação (módulos + submódulos), ocultando o que não
 * corresponde, em vez de mostrar uma lista de resultados à parte.
 */
export function SidebarSearch({ query, onQueryChange }: { query: string; onQueryChange: (q: string) => void }) {
  return (
    <div className="sidebar-search">
      <div className="sidebar-search__box">
        <Search size={14} className="sidebar-search__icon" aria-hidden />
        <input
          className="sidebar-search__input"
          type="search"
          placeholder="Pesquisar telas…"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") onQueryChange("");
          }}
          autoComplete="off"
        />
        {query ? (
          <button type="button" className="sidebar-search__clear" aria-label="Limpar pesquisa" onClick={() => onQueryChange("")}>
            <X size={13} aria-hidden />
          </button>
        ) : null}
      </div>
    </div>
  );
}
