import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";

/**
 * Regra das telas da integração HubSoft: NADA consulta a API sozinho (nem ao abrir a aba, nem ao mexer
 * num filtro) — só o botão "Consultar". Este módulo padroniza o aviso (toast) do resultado:
 *   ok        → "Consulta realizada com sucesso"
 *   erro      → "Erro <código> na consulta: <motivo>"
 *   faltando  → "Faltando informação: <o quê>"
 */

export function describeConsultaError(e: unknown): string {
  if (e instanceof ApiError) {
    return `Erro ${e.status} na consulta: ${e.message}`;
  }
  const msg = e instanceof Error ? e.message : String(e ?? "");
  return `Erro na consulta: ${msg || "falha de comunicação"}`;
}

/** Devolve funções estáveis para avisar o resultado de uma consulta. */
export function useConsultaToast() {
  const toast = useAppToast();

  /** Avalia o retorno de uma consulta (dados e/ou erro) e mostra o toast; devolve true se deu certo. */
  const notify = useCallback(
    (data: unknown, error: unknown, okText = "Consulta realizada com sucesso"): boolean => {
      if (error) {
        toast.push({ tone: "err", text: describeConsultaError(error) });
        return false;
      }
      const d = data as { ok?: boolean; message?: string } | null | undefined;
      if (d && typeof d === "object" && d.ok === false) {
        toast.push({ tone: "err", text: `Erro na consulta: ${d.message || "a HubSoft não devolveu dados"}` });
        return false;
      }
      toast.push({ tone: "ok", text: okText });
      return true;
    },
    [toast],
  );

  const missing = useCallback(
    (what: string) => {
      toast.push({ tone: "warn", text: `Faltando informação: ${what}` });
    },
    [toast],
  );

  /** Executa um refetch do react-query e avisa o resultado. */
  const runRefetch = useCallback(
    async (refetch: () => Promise<{ data?: unknown; error?: unknown }>) => {
      const r = await refetch();
      return notify(r.data, r.error);
    },
    [notify],
  );

  return { notify, missing, runRefetch };
}

// Último resultado por tela: ao trocar de aba e voltar, o resultado da última consulta continua na
// tela (sem consultar de novo) — só o botão "Consultar" chama a API.
const LAST_RESULT = new Map<string, { params: unknown; data: unknown; at: number }>();

// Cache persistente (localStorage) para relatórios que mudam pouco — sobrevive a recarregar a página.
const PERSIST_PREFIX = "netquasar.hubsoft.consulta.";
function readPersist(key: string): { params: unknown; data: unknown; at: number } | undefined {
  try {
    const raw = window.localStorage.getItem(PERSIST_PREFIX + key);
    return raw ? (JSON.parse(raw) as { params: unknown; data: unknown; at: number }) : undefined;
  } catch {
    return undefined;
  }
}
function writePersist(key: string, v: { params: unknown; data: unknown; at: number }) {
  try {
    window.localStorage.setItem(PERSIST_PREFIX + key, JSON.stringify(v));
  } catch {
    /* quota cheia / modo privado: segue sem cache persistente */
  }
}

/** "24/09/2026 15:30" — para mostrar quando a consulta em cache foi feita. */
export function fmtConsultaAt(at: number | null): string {
  return at ? new Date(at).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" }) : "";
}

/**
 * Consulta manual: `run(params)` chama a API uma vez, avisa o resultado por toast e guarda o retorno.
 * Nada dispara sozinho (nem ao montar a tela, nem ao mudar filtros) — os filtros são só rascunho até
 * o clique em "Consultar".
 */
export function useConsulta<P, T>(key: string, fetcher: (params: P) => Promise<T>, opts?: { persist?: boolean }) {
  const { notify } = useConsultaToast();
  const last = (LAST_RESULT.get(key) ?? (opts?.persist ? readPersist(key) : undefined)) as { params: P; data: T; at: number } | undefined;
  const [state, setState] = useState<{ params: P | null; data: T | undefined; error: unknown; loading: boolean; at: number | null }>({
    params: last?.params ?? null,
    data: last?.data,
    error: null,
    loading: false,
    at: last?.at ?? null,
  });
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const seq = useRef(0);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const run = useCallback(
    async (params: P): Promise<boolean> => {
      const mine = ++seq.current;
      setState((s) => ({ ...s, params, loading: true, error: null }));
      try {
        const data = await fetcherRef.current(params);
        const at = Date.now();
        const ok = notify(data, null);
        if (ok) {
          LAST_RESULT.set(key, { params, data, at });
          if (opts?.persist) writePersist(key, { params, data, at });
        }
        if (alive.current && mine === seq.current) setState({ params, data, error: null, loading: false, at });
        return ok;
      } catch (e) {
        notify(null, e);
        if (alive.current && mine === seq.current) setState((s) => ({ ...s, loading: false, error: e }));
        return false;
      }
    },
    [key, notify, opts?.persist],
  );

  return {
    /** Parâmetros da última consulta feita (null = ainda não consultou). */
    applied: state.params,
    data: state.data,
    error: state.error,
    isFetching: state.loading,
    isLoading: state.loading && state.data === undefined,
    isError: !!state.error,
    consulted: state.params !== null,
    at: state.at,
    run,
  };
}
