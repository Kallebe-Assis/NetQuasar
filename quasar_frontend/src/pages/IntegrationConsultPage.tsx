import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Search } from "lucide-react";
import { IntegrationNav } from "../components/IntegrationNav";
import { HubsoftClientResults, filterClientCards } from "../integrations/HubsoftClientResults";
import type { ClientCard, ClientSearchResponse, ConsumerMeta, IntegrationDetail } from "../integrations/types";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { PageToastHost, usePageToast } from "../lib/pageToast";
import { queryKeys } from "../lib/queryKeys";

function matchClient(a: ClientCard, b: ClientCard): boolean {
  if (a.id && b.id && a.id === b.id) return true;
  if (a.code && b.code && a.code === b.code) return true;
  if (a.document && b.document && a.document === b.document) return true;
  return false;
}

export function IntegrationConsultPage() {
  const { slug } = useParams<{ slug: string }>();
  const { toast, show: showToast, dismiss: dismissToast } = usePageToast();
  const [busca, setBusca] = useState("cpf_cnpj");
  const [termo, setTermo] = useState("");
  const [resultFilter, setResultFilter] = useState("");

  const detailQ = useQuery({
    queryKey: queryKeys.integrationDetail(slug ?? ""),
    queryFn: () => apiFetch<IntegrationDetail>(`/api/v1/integrations/${slug}`),
    enabled: !!slug,
  });

  const metaQ = useQuery({
    queryKey: [...queryKeys.integrationDetail(slug ?? ""), "consumer"],
    queryFn: () => apiFetch<ConsumerMeta>(`/api/v1/integrations/${slug}/consumer`),
    enabled: !!slug,
  });

  const searchM = useMutation({
    mutationFn: (opts: { busca: string; termo: string }) =>
      apiFetch<ClientSearchResponse>(`/api/v1/integrations/${slug}/consumer/client-search`, {
        method: "POST",
        json: {
          busca: opts.busca,
          termo_busca: opts.termo,
          detailed: false,
        },
      }),
    onSuccess: (r) => {
      setResultFilter("");
      showToast(r.ok ? "ok" : "err", r.ok ? `${r.clients?.length ?? 0} resultado(s).` : r.message || "Consulta falhou.");
    },
    onError: (e) => showToast("err", e instanceof Error ? e.message : String(e)),
  });

  const runSearch = useCallback(() => {
    const t = termo.trim();
    if (!t) return;
    searchM.mutate({ busca, termo: t });
  }, [busca, termo, searchM]);

  const fetchClientDetail = useCallback(
    async (client: ClientCard): Promise<ClientCard> => {
      const t = termo.trim();
      if (!t) return client;
      const r = await apiFetch<ClientSearchResponse>(`/api/v1/integrations/${slug}/consumer/client-search`, {
        method: "POST",
        json: {
          busca,
          termo_busca: t,
          detailed: true,
        },
      });
      const full = r.clients?.find((c) => matchClient(c, client));
      return full ?? client;
    },
    [slug, busca, termo],
  );

  const d = detailQ.data;
  const meta = metaQ.data;
  const result = searchM.data;
  const allClients = result?.clients ?? [];

  const filteredClients = useMemo(
    () => filterClientCards(allClients, resultFilter),
    [allClients, resultFilter],
  );

  if (detailQ.isLoading) return <p style={{ padding: 24, color: "var(--muted)" }}>A carregar…</p>;
  if (detailQ.isError || !d) {
    return (
      <div style={{ padding: 24 }}>
        <p className="msg msg--err">{(detailQ.error as Error)?.message || "Integração não encontrada."}</p>
      </div>
    );
  }

  const consultaEnabled = meta?.client_search_enabled ?? false;

  if (!consultaEnabled) {
    return (
      <div>
        <IntegrationNav slug={slug!} name={d.name} consultaEnabled={false} />
        <div className="msg" style={{ marginTop: 12 }}>
          A consulta de clientes ainda não está activa nesta integração.
          {isAdminUser() ? (
            <>
              {" "}
              Active em{" "}
              <Link to={`/integrations/${slug}/config`}>Configuração API</Link> → separador Operação.
            </>
          ) : (
            " Peça a um administrador para configurar."
          )}
        </div>
      </div>
    );
  }

  const buscaOptions = meta?.busca_options ?? [{ value: "cpf_cnpj", label: "CPF/CNPJ" }];
  const termoLabel = busca === "cpf_cnpj" ? "CPF/CNPJ" : "Termo de busca";
  const hasResults = !!result;
  const showCount = resultFilter.trim() ? filteredClients.length : allClients.length;

  return (
    <div className="integration-consult">
      <IntegrationNav slug={slug!} name={d.name} consultaEnabled />
      <PageToastHost toast={toast} onDismiss={dismissToast} />

      <div className="card integration-consult-search">
        <div className="integration-consult-search__head">
          <h2>
            <Search size={18} aria-hidden /> Consultar cliente
          </h2>
          <p>{meta?.client_search_request_name || "API de clientes"} — autenticação automática.</p>
        </div>

        <div className="integration-consult-search__row">
          <div className="field field--type">
            <label>Tipo de consulta</label>
            <select className="input" value={busca} onChange={(e) => setBusca(e.target.value)}>
              {buscaOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>
          <div className="field field--term">
            <label>{termoLabel}</label>
            <input
              className="input mono"
              value={termo}
              onChange={(e) => setTermo(e.target.value)}
              placeholder={busca === "cpf_cnpj" ? "Ex.: 12345678900" : "Valor a pesquisar"}
              onKeyDown={(e) => {
                if (e.key === "Enter") runSearch();
              }}
            />
          </div>
          <div className="integration-consult-search__actions">
            <button
              type="button"
              className="btn btn--primary"
              disabled={searchM.isPending || !termo.trim()}
              onClick={runSearch}
            >
              {searchM.isPending ? "A pesquisar…" : "Pesquisar"}
            </button>
          </div>
        </div>
      </div>

      <section className="integration-consult-results">
        {hasResults ? (
          <>
            <div className="integration-consult-results__toolbar">
              <span className="integration-consult-results__toolbar-title">
                Resultados {result.ok ? `(${showCount}${resultFilter.trim() && showCount !== allClients.length ? ` de ${allClients.length}` : ""})` : ""}
              </span>
              {result.request_url ? (
                <span className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                  HTTP {result.status_code ?? "—"}
                  {result.latency_ms != null ? ` · ${result.latency_ms} ms` : ""}
                </span>
              ) : null}
            </div>

            {allClients.length > 0 ? (
              <div className="integration-consult-results__filter">
                <div className="field">
                  <label>Filtrar nos resultados</label>
                  <input
                    className="input"
                    value={resultFilter}
                    onChange={(e) => setResultFilter(e.target.value)}
                    placeholder="Pesquisar no que já foi carregado (nome, CPF, login…)"
                  />
                </div>
              </div>
            ) : null}

            <HubsoftClientResults
              clients={allClients}
              message={result.message}
              ok={!!result.ok}
              localFilter={resultFilter}
              onFetchDetail={fetchClientDetail}
            />
          </>
        ) : (
          <p className="integration-consult-empty">
            Preencha o termo e clique em Pesquisar. Os resultados aparecem abaixo.
          </p>
        )}
      </section>
    </div>
  );
}
