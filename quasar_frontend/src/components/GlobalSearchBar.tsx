import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { MapPin, Search, Server, User, Wifi, X } from "lucide-react";
import { apiFetch } from "../lib/api";

type GlobalSearchKind = "device" | "connection" | "cto" | "pole" | "splice_box" | "cable" | "project" | "pop" | "onu";

type GlobalSearchResult = {
  kind: GlobalSearchKind;
  label: string;
  subtitle?: string;
  href: string;
};

type GlobalSearchResponse = { results: GlobalSearchResult[]; q: string; total?: number };

const KIND_LABELS: Record<GlobalSearchKind, string> = {
  device: "Equipamentos",
  connection: "Logins / clientes",
  cto: "CTOs",
  pole: "Postes",
  splice_box: "Emendas / foguetes",
  cable: "Cabos",
  project: "Projetos",
  pop: "POPs",
  onu: "ONUs",
};

function iconForKind(kind: GlobalSearchKind) {
  switch (kind) {
    case "device":
      return <Server size={14} aria-hidden />;
    case "connection":
      return <User size={14} aria-hidden />;
    case "onu":
      return <Wifi size={14} aria-hidden />;
    default:
      return <MapPin size={14} aria-hidden />;
  }
}

export function GlobalSearchBar() {
  const navigate = useNavigate();
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [results, setResults] = useState<GlobalSearchResult[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [searchedFor, setSearchedFor] = useState("");
  const wrapRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  async function runSearch() {
    const term = q.trim();
    if (term.length < 2) {
      setError("Digite pelo menos 2 caracteres para pesquisar.");
      setResults(null);
      setOpen(true);
      return;
    }
    setError(null);
    setLoading(true);
    setOpen(true);
    try {
      const res = await apiFetch<GlobalSearchResponse>(`/api/v1/search/global?q=${encodeURIComponent(term)}`);
      setResults(res.results);
      setSearchedFor(term);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Falha na pesquisa.");
      setResults(null);
    } finally {
      setLoading(false);
    }
  }

  const grouped = useMemo(() => {
    if (!results) return [];
    const map = new Map<GlobalSearchKind, GlobalSearchResult[]>();
    for (const r of results) {
      const arr = map.get(r.kind) ?? [];
      arr.push(r);
      map.set(r.kind, arr);
    }
    return Array.from(map.entries());
  }, [results]);

  function goTo(r: GlobalSearchResult) {
    setOpen(false);
    navigate(r.href);
  }

  function clear() {
    setQ("");
    setResults(null);
    setError(null);
    setOpen(false);
    inputRef.current?.focus();
  }

  const showPanel = open && (loading || !!error || results !== null);

  return (
    <div className="global-search" ref={wrapRef}>
      <div className={`global-search__box${loading ? " global-search__box--loading" : ""}`}>
        <Search size={16} className="global-search__icon" aria-hidden />
        <input
          ref={inputRef}
          className="global-search__input"
          type="search"
          placeholder="Pesquisar IP, nome, CTO, login, MAC, serial de ONU…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => {
            if (results !== null || error) setOpen(true);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void runSearch();
            } else if (e.key === "Escape") {
              setOpen(false);
            }
          }}
          autoComplete="off"
        />
        {loading ? <span className="page-toast__spinner global-search__spinner" aria-hidden /> : null}
        {q && !loading ? (
          <button type="button" className="global-search__clear" aria-label="Limpar pesquisa" onClick={clear}>
            <X size={14} aria-hidden />
          </button>
        ) : null}
        <button type="button" className="global-search__btn" onClick={() => void runSearch()} disabled={loading} title="Pesquisar em todo o sistema">
          {loading ? "A pesquisar…" : "Pesquisar"}
        </button>
      </div>

      <div className={`global-search__panel${showPanel ? " global-search__panel--open" : ""}`} role="listbox" aria-label="Resultados da pesquisa">
        {error ? (
          <p className="global-search__empty">{error}</p>
        ) : loading ? (
          <p className="global-search__empty">A pesquisar em equipamentos, logins, infraestrutura e ONUs…</p>
        ) : results && results.length === 0 ? (
          <p className="global-search__empty">Nenhum resultado para «{searchedFor}».</p>
        ) : results && results.length > 0 ? (
          <div className="global-search__results">
            {grouped.map(([kind, items]) => (
              <div key={kind} className="global-search__group">
                <div className="global-search__group-title">{KIND_LABELS[kind]}</div>
                {items.map((r, i) => (
                  <button key={`${kind}-${i}-${r.label}`} type="button" className="global-search__item" onClick={() => goTo(r)}>
                    <span className="global-search__item-icon">{iconForKind(r.kind)}</span>
                    <span className="global-search__item-text">
                      <span className="global-search__item-label">{r.label}</span>
                      {r.subtitle ? <span className="global-search__item-subtitle">{r.subtitle}</span> : null}
                    </span>
                  </button>
                ))}
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  );
}
