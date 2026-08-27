import { useQuery, useMutation } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { Filter, Search } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import { InfoHint } from "../../components/InfoHint";
import { HubsoftClientResults, filterClientCards } from "../../integrations/HubsoftClientResults";
import type {
  BuscaOption,
  ClientAttendanceResponse,
  ClientCard,
  ClientFinancialResponse,
  ClientSearchResponse,
  ClientWorkOrderResponse,
} from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { PageToastHost, usePageToast } from "../../lib/pageToast";

/**
 * Tela de consulta dedicada à HubSoft — fala com as rotas /hubsoft/* (ver
 * internal/api/handlers_hubsoft.go), não com o motor genérico usado pelo IXC.
 */
export function HubsoftConsultPage() {
  const slug = "hubsoft";
  const { toast, show: showToast, dismiss: dismissToast } = usePageToast();
  const [busca, setBusca] = useState("nome_razaosocial");
  const [termo, setTermo] = useState("");
  const [resultFilter, setResultFilter] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);

  const buscaOptionsQ = useQuery({
    queryKey: ["hubsoft-busca-options"],
    queryFn: () => apiFetch<{ busca_options: BuscaOption[] }>("/api/v1/integrations/hubsoft-busca-options"),
    staleTime: Infinity,
  });

  const searchM = useMutation({
    mutationFn: (opts: { busca: string; termo: string }) =>
      apiFetch<ClientSearchResponse>(`/api/v1/integrations/${slug}/hubsoft/search`, {
        method: "POST",
        json: { busca: opts.busca, termo: opts.termo, detailed: false },
      }),
    onSuccess: (r) => {
      setResultFilter("");
      if (r.ok) {
        showToast("ok", `${r.clients?.length ?? 0} resultado(s).`);
      } else {
        showToast("err", r.message || "Consulta falhou.");
      }
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
      const codigo = client.code?.trim() || client.id?.trim();
      if (!codigo) return client;
      const r = await apiFetch<ClientSearchResponse>(`/api/v1/integrations/${slug}/hubsoft/search`, {
        method: "POST",
        json: { busca: "codigo_cliente", termo: codigo, detailed: true },
      });
      return r.clients?.[0] ?? client;
    },
    [],
  );

  const fetchClientAttendance = useCallback(async (client: ClientCard) => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) {
      return { ok: false, message: "Código do cliente não encontrado no cartão.", items: [] };
    }
    const r = await apiFetch<ClientAttendanceResponse>(`/api/v1/integrations/${slug}/hubsoft/attendance`, {
      method: "POST",
      json: { codigo_cliente: codigo },
    });
    return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
  }, []);

  const fetchClientWorkOrders = useCallback(async (client: ClientCard) => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) {
      return { ok: false, message: "Código do cliente não encontrado no cartão.", items: [] };
    }
    const r = await apiFetch<ClientWorkOrderResponse>(`/api/v1/integrations/${slug}/hubsoft/work-orders`, {
      method: "POST",
      json: { codigo_cliente: codigo },
    });
    return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
  }, []);

  const fetchClientFinancial = useCallback(async (client: ClientCard) => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) {
      return { ok: false, message: "Código do cliente não encontrado no cartão.", invoices: [], summary: undefined };
    }
    const r = await apiFetch<ClientFinancialResponse>(`/api/v1/integrations/${slug}/hubsoft/financial`, {
      method: "POST",
      json: { codigo_cliente: codigo },
    });
    return { ok: !!r.ok, message: r.message, invoices: r.invoices ?? [], summary: r.summary };
  }, []);

  const result = searchM.data;
  const allClients = result?.clients ?? [];
  const filteredClients = useMemo(() => filterClientCards(allClients, resultFilter), [allClients, resultFilter]);
  const buscaOptions = buscaOptionsQ.data?.busca_options ?? [{ value: "nome_razaosocial", label: "Nome / Razão social" }];
  const termoLabel = buscaOptions.find((o) => o.value === busca)?.label ?? "Termo de busca";
  const hasResults = !!result;
  const showCount = resultFilter.trim() ? filteredClients.length : allClients.length;

  return (
    <div className="integration-consult">
      <HubsoftHeader />
      <PageToastHost toast={toast} onDismiss={dismissToast} />

      <div className="card integration-consult-search">
        {allClients.length > 0 ? (
          <div className="integration-consult-search__filter-wrap">
            <button
              type="button"
              className="btn btn--icon integration-consult-search__filter-toggle"
              aria-label="Filtrar nos resultados"
              title="Filtrar nos resultados"
              onClick={() => setFilterOpen((v) => !v)}
            >
              <Filter size={14} />
            </button>
            {filterOpen ? (
              <div className="integration-consult-search__filter-popover">
                <label>Filtrar nos resultados</label>
                <input
                  className="input"
                  autoFocus
                  value={resultFilter}
                  onChange={(e) => setResultFilter(e.target.value)}
                  placeholder="Nome, CPF, login…"
                />
              </div>
            ) : null}
          </div>
        ) : null}
        <div className="integration-consult-search__head">
          <h2>
            <Search size={18} aria-hidden /> Consultar cliente
            <InfoHint label="Sobre a consulta">Cliente, cadastro, login, IPv4 ou MAC — dados completos ao abrir um resultado.</InfoHint>
          </h2>
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
              placeholder={
                busca === "cpf_cnpj"
                  ? "Ex.: 12345678900"
                  : busca === "login_radius"
                    ? "Ex.: usuario123"
                    : busca === "mac"
                      ? "Ex.: 98:03:8E:90:98:83"
                      : busca === "ipv4"
                        ? "Ex.: 45.235.87.49"
                        : "Valor a pesquisar"
              }
              onKeyDown={(e) => {
                if (e.key === "Enter") runSearch();
              }}
            />
          </div>
          <div className="integration-consult-search__actions">
            <button type="button" className="btn btn--primary" disabled={searchM.isPending || !termo.trim()} onClick={runSearch}>
              {searchM.isPending ? "A pesquisar…" : "Pesquisar"}
            </button>
          </div>
        </div>
        {busca === "ipv4" || busca === "mac" ? (
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
            A HubSoft não tem busca nativa por {busca === "ipv4" ? "IPv4" : "MAC"} — o NetQuasar varre a listagem de clientes até
            encontrar (pode demorar alguns segundos).
          </p>
        ) : null}
      </div>

      <section className="integration-consult-results">
        {hasResults ? (
          <>
            <div className="integration-consult-results__toolbar">
              <span className="integration-consult-results__toolbar-title">
                Resultados {result.ok ? `(${showCount}${resultFilter.trim() && showCount !== allClients.length ? ` de ${allClients.length}` : ""})` : ""}
              </span>
            </div>

            <HubsoftClientResults
              clients={allClients}
              message={result.message}
              ok={!!result.ok}
              localFilter={resultFilter}
              onFetchDetail={fetchClientDetail}
              onFetchAttendance={fetchClientAttendance}
              onFetchWorkOrders={fetchClientWorkOrders}
              onFetchFinancial={fetchClientFinancial}
              attendanceEnabled
              workOrderEnabled
              prefetchExtras
            />
          </>
        ) : (
          <p className="integration-consult-empty">Preencha o termo e clique em Pesquisar. Os resultados aparecem abaixo.</p>
        )}
      </section>
    </div>
  );
}
