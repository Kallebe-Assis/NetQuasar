import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Search } from "lucide-react";
import { IntegrationNav } from "../components/IntegrationNav";
import { HubsoftClientResults, filterClientCards } from "../integrations/HubsoftClientResults";
import { attendanceBuscaForClient } from "../integrations/attendanceBusca";
import type {
  ClientAttendanceResponse,
  ClientCard,
  ClientSearchResponse,
  ClientLoginResponse,
  ClientWorkOrderResponse,
  ConsumerMeta,
  IntegrationDetail,
} from "../integrations/types";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import { PageToastHost, usePageToast } from "../lib/pageToast";
import { queryKeys } from "../lib/queryKeys";

function matchClient(a: ClientCard, b: ClientCard): boolean {
  const aId = a.id?.trim();
  const bId = b.id?.trim();
  if (aId && bId && aId === bId) return true;
  const aCode = a.code?.trim();
  const bCode = b.code?.trim();
  if (aCode && bCode && aCode === bCode) return true;
  if (!aId && !bId && a.document?.trim() && a.document.trim() === b.document?.trim()) return true;
  return false;
}

function clientLookupQuery(client: ClientCard, fallback: { busca: string; termo: string }) {
  const id = client.id?.trim() || client.code?.trim();
  if (id) return { busca: "codigo_cliente", termo: id };
  return fallback;
}

export function IntegrationConsultPage() {
  const { slug } = useParams<{ slug: string }>();
  const { toast, show: showToast, dismiss: dismissToast } = usePageToast();
  const [busca, setBusca] = useState("nome_razaosocial");
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
      if (r.ok) {
        showToast("ok", `${r.clients?.length ?? 0} resultado(s).`);
      } else {
        const extra =
          r.response_preview && r.message?.includes("JSON")
            ? ""
            : r.response_preview
              ? ` (${r.response_preview.slice(0, 120)}…)`
              : "";
        showToast("err", (r.message || "Consulta falhou.") + extra);
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
      const q = clientLookupQuery(client, { busca, termo: termo.trim() });
      if (!q.termo.trim()) return client;
      const r = await apiFetch<ClientSearchResponse>(`/api/v1/integrations/${slug}/consumer/client-search`, {
        method: "POST",
        json: {
          busca: q.busca,
          termo_busca: q.termo,
          detailed: true,
        },
      });
      const full = r.clients?.find((c) => matchClient(c, client));
      return full ?? client;
    },
    [slug, busca, termo],
  );

  const lookupForClient = useCallback((client: ClientCard) => {
    return attendanceBuscaForClient(client, { busca: "codigo_cliente", termo: "" });
  }, []);

  const fetchClientAttendance = useCallback(
    async (client: ClientCard) => {
      const q = lookupForClient(client);
      if (!q.termo.trim()) {
        return {
          ok: false,
          message:
            "ID do cliente não encontrado no cartão. Consulte por ID do cliente ou abra «Ver dados completos» antes dos atendimentos.",
          items: [],
        };
      }
      const r = await apiFetch<ClientAttendanceResponse>(`/api/v1/integrations/${slug}/consumer/client-attendance`, {
        method: "POST",
        json: {
          busca: q.busca,
          termo_busca: q.termo,
          apenas_pendente: "nao",
        },
      });
      return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
    },
    [slug, lookupForClient],
  );

  const fetchClientWorkOrders = useCallback(
    async (client: ClientCard) => {
      const q = lookupForClient(client);
      if (!q.termo.trim()) {
        return {
          ok: false,
          message:
            "ID do cliente não encontrado no cartão. Consulte por ID do cliente ou abra «Ver dados completos» antes das ordens de serviço.",
          items: [],
        };
      }
      const r = await apiFetch<ClientWorkOrderResponse>(`/api/v1/integrations/${slug}/consumer/client-work-order`, {
        method: "POST",
        json: {
          busca: q.busca,
          termo_busca: q.termo,
        },
      });
      return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
    },
    [slug, lookupForClient],
  );

  const fetchClientLogins = useCallback(
    async (client: ClientCard) => {
      const q = lookupForClient(client);
      if (!q.termo.trim()) {
        return {
          ok: false,
          message: "ID do cliente não encontrado no cartão.",
          items: [],
        };
      }
      const r = await apiFetch<ClientLoginResponse>(`/api/v1/integrations/${slug}/consumer/client-login`, {
        method: "POST",
        json: {
          busca: q.busca,
          termo_busca: q.termo,
        },
      });
      return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
    },
    [slug, lookupForClient],
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
          A consulta de clientes ainda não está ativa nesta integração.
          {isAdminUser() ? (
            <>
              {" "}
              Ative em{" "}
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
  const termoLabel =
    buscaOptions.find((o) => o.value === busca)?.label ??
    (busca === "cpf_cnpj" ? "CPF/CNPJ" : "Termo de busca");
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
          <p>
            {meta?.client_search_request_name || "API de clientes"}
            {meta?.client_search_provider ? ` · ${meta.client_search_provider.toUpperCase()}` : ""} — autenticação automática.
          </p>
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
                  : busca === "login" || busca === "login_radius"
                    ? "Ex.: usuario@provedor"
                    : "Valor a pesquisar"
              }
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
              onFetchAttendance={meta?.client_attendance_enabled ? fetchClientAttendance : undefined}
              onFetchWorkOrders={meta?.client_work_order_enabled ? fetchClientWorkOrders : undefined}
              onFetchLogins={meta?.client_login_enabled ? fetchClientLogins : undefined}
              attendanceEnabled={!!meta?.client_attendance_enabled}
              workOrderEnabled={!!meta?.client_work_order_enabled}
              loginEnabled={!!meta?.client_login_enabled}
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
