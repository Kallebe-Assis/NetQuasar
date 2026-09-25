import { useMutation } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { Ban, Headset, Hourglass, Layers, Send, ShieldCheck, Users, Wallet, Wrench } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import { InfoHint } from "../../components/InfoHint";
import { Switch } from "../../components/Switch";
import { EmptyState } from "../../components/EmptyState";
import { ClientDetailModal } from "../../integrations/HubsoftClientResults";
import type {
  ClientAttendanceResponse,
  ClientCard,
  ClientFinancialResponse,
  ClientSearchResponse,
  ClientServiceSummary,
  ClientWorkOrderResponse,
  HubsoftAttendanceReportResponse,
  HubsoftBlockedReportResponse,
  HubsoftFinancialReportResponse,
  HubsoftReportClientsResponse,
  HubsoftReportServiceRow,
  HubsoftServiceLocalityBreakdown,
  HubsoftServicesReportResponse,
  HubsoftWorkOrderReportResponse,
} from "../../integrations/types";
import { apiFetch } from "../../lib/api";
import { HubsoftPreventiveSection } from "./HubsoftPreventiveSection";
import { HubsoftBulkClientsModal } from "./HubsoftBulkClientsModal";
import { HubsoftTenureBandModal, type TenureBand } from "./HubsoftTenureBandModal";
import { fmtConsultaAt, useConsulta, useConsultaToast } from "./hubsoftConsulta";

import { ConsultaLoading } from "./ConsultaLoading";
const SLUG = "hubsoft";

const SERVICE_STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "Qualquer status" },
  { value: "servico_habilitado", label: "Serviço habilitado" },
  { value: "suspenso_debito", label: "Suspenso por débito" },
  { value: "suspenso_parcialmente", label: "Suspenso parcialmente" },
  { value: "suspenso_pedido_cliente", label: "Suspenso a pedido do cliente" },
  { value: "cancelado", label: "Cancelado" },
  { value: "inativo", label: "Inactivo" },
  { value: "aguardando_instalacao", label: "Aguardando instalação" },
  { value: "agendado_para_instalacao", label: "Agendado para instalação" },
  { value: "aguardando_configuracao", label: "Aguardando configuração" },
  { value: "aguardando_assinatura_contrato", label: "Aguardando assinatura do contrato" },
  { value: "aguardando_liberacao_ti", label: "Aguardando liberação TI" },
  { value: "aguardando_migracao", label: "Aguardando migração" },
  { value: "franquia_excedida", label: "Franquia excedida" },
];

type Section = "clients" | "services" | "blocked" | "preventive" | "tenure" | "attendance" | "work_orders" | "financial";

function fmtInt(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR");
}
function fmtCurrency(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}
function fmtPct(n?: number): string {
  return `${(n ?? 0).toLocaleString("pt-BR", { maximumFractionDigits: 1 })}%`;
}
function connectedBadge(v?: string) {
  if (v === "true") return <span className="badge badge--ok">Conectado</span>;
  if (v === "false") return <span className="badge badge--off">Desconectado</span>;
  return <span className="badge">—</span>;
}

// Converte uma linha do relatório (enxuta) num ClientCard mínimo para abrir a modal de
// detalhes — a modal já busca os dados completos ao abrir (onFetchDetail), isto é só o que
// aparece "instantaneamente" enquanto isso carrega.
function rowToClientCard(row: HubsoftReportServiceRow): ClientCard {
  const svc: ClientServiceSummary = {
    id: row.service_id,
    name: row.service_name,
    status: row.status,
    login: row.login,
    ipv4: row.ipv4,
    mac: row.mac,
    connected: row.connected,
  };
  return {
    id: row.client_id,
    code: row.client_code,
    name: row.client_name,
    document: row.document,
    status: row.status,
    services: [svc],
  };
}

function todayISO(offsetDays = 0): string {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return d.toISOString().slice(0, 10);
}

/** "2026-08" (valor de <input type="month">) → {from, to, label} — 1º ao último dia do mês. */
function monthRange(yyyyMm: string): { from: string; to: string; label: string } {
  const [y, m] = yyyyMm.split("-").map(Number);
  const from = `${yyyyMm}-01`;
  const lastDay = new Date(y, m, 0).getDate();
  const to = `${yyyyMm}-${String(lastDay).padStart(2, "0")}`;
  const label = new Date(y, m - 1, 1).toLocaleDateString("pt-BR", { month: "short", year: "numeric" });
  return { from, to, label };
}

function currentMonthValue(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

/** Últimos `n` meses (o actual incluído), do mais antigo para o mais recente. */
function lastNMonths(n: number): { from: string; to: string; label: string }[] {
  const out: { from: string; to: string; label: string }[] = [];
  const now = new Date();
  for (let i = n - 1; i >= 0; i--) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    out.push(monthRange(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`));
  }
  return out;
}

/** Seletor de período reutilizado pelas 3 abas de relatório (Atendimentos/O.S./Financeiro) e
 * pela Conferência (aba Ordens de serviço → botão "Conferência", HubsoftConferenceModal.tsx). */
export function PeriodPicker({
  from,
  to,
  onChange,
}: {
  from: string;
  to: string;
  onChange: (from: string, to: string) => void;
}) {
  return (
    <div className="row" style={{ gap: 8, alignItems: "flex-end", flexWrap: "wrap", marginBottom: 12 }}>
      <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
        De
        <input type="date" className="input" value={from} onChange={(e) => onChange(e.target.value, to)} />
      </label>
      <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
        Até
        <input type="date" className="input" value={to} onChange={(e) => onChange(from, e.target.value)} />
      </label>
      <div className="row" style={{ gap: 4 }}>
        {[7, 30, 90].map((days) => (
          <button
            key={days}
            type="button"
            className="btn btn--sm"
            onClick={() => onChange(todayISO(-days), todayISO())}
          >
            {days} dias
          </button>
        ))}
      </div>
    </div>
  );
}

/** Botão "Enviar por Telegram" partilhado pelas 3 secções de relatório por período — envia o
 * resumo do período actualmente seleccionado para o bot "reports" (Configurações → Telegram).
 * Ausente na secção Clientes de propósito: é uma lista filtrada, não um resumo, não faz sentido
 * enviar centenas/milhares de linhas por Telegram. */
function TelegramSendButton({ path }: { path: string }) {
  const [feedback, setFeedback] = useState<{ ok: boolean; message: string } | null>(null);
  const m = useMutation({
    mutationFn: () => apiFetch(path, { method: "POST" }),
    onSuccess: () => setFeedback({ ok: true, message: "Enviado para o Telegram." }),
    onError: (e) => setFeedback({ ok: false, message: e instanceof Error ? e.message : "Falha ao enviar." }),
  });
  return (
    <div className="row" style={{ gap: 8, alignItems: "center" }}>
      <button type="button" className="btn btn--sm" disabled={m.isPending} onClick={() => { setFeedback(null); m.mutate(); }}>
        <Send size={12} style={{ marginRight: 4, verticalAlign: -2 }} />
        {m.isPending ? "A enviar…" : "Enviar por Telegram"}
      </button>
      {feedback ? (
        <span style={{ fontSize: 11, color: feedback.ok ? "var(--ok)" : "var(--err)" }}>{feedback.message}</span>
      ) : null}
    </div>
  );
}

function StatusBreakdownTable({ items, labelHeader = "Status" }: { items: { name: string; count: number }[]; labelHeader?: string }) {
  if (items.length === 0) return null;
  return (
    <div className="table-wrap" style={{ marginTop: 10 }}>
      <table style={{ fontSize: 12 }}>
        <thead>
          <tr>
            <th>{labelHeader}</th>
            <th>Quantidade</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => (
            <tr key={it.name}>
              <td>{it.name}</td>
              <td className="mono">{fmtInt(it.count)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ClientsReportSection() {
  const [servicoStatus, setServicoStatus] = useState("");
  const [cancelado, setCancelado] = useState("");
  const [estado, setEstado] = useState("");
  const [cidade, setCidade] = useState("");
  const [bairro, setBairro] = useState("");
  const [ipv4, setIpv4] = useState("");
  const [mac, setMac] = useState("");
  const [login, setLogin] = useState("");
  const [detailClient, setDetailClient] = useState<ClientCard | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);

  const buildParams = useCallback(() => {
    const p = new URLSearchParams();
    if (servicoStatus) p.set("servico_status", servicoStatus);
    if (cancelado) p.set("cancelado", cancelado);
    if (estado.trim()) p.set("estado", estado.trim());
    if (cidade.trim()) p.set("cidade", cidade.trim());
    if (bairro.trim()) p.set("bairro", bairro.trim());
    if (ipv4.trim()) p.set("ipv4", ipv4.trim());
    if (mac.trim()) p.set("mac", mac.trim());
    if (login.trim()) p.set("login", login.trim());
    return p.toString();
  }, [servicoStatus, cancelado, estado, cidade, bairro, ipv4, mac, login]);

  const reportQ = useConsulta<string, HubsoftReportClientsResponse>("hubsoft-report-clients", (qs) =>
    apiFetch<HubsoftReportClientsResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/clients?${qs}`, { timeoutMs: 5 * 60_000 }),
  );

  function runFilter() {
    void reportQ.run(buildParams());
  }

  const fetchClientDetail = useCallback(async (client: ClientCard): Promise<ClientCard> => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) return client;
    const r = await apiFetch<ClientSearchResponse>(`/api/v1/integrations/${SLUG}/hubsoft/search`, {
      method: "POST",
      json: { busca: "codigo_cliente", termo: codigo, detailed: true },
    });
    return r.clients?.[0] ?? client;
  }, []);

  const fetchClientAttendance = useCallback(async (client: ClientCard) => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) return { ok: false, message: "Código do cliente não encontrado.", items: [] };
    const r = await apiFetch<ClientAttendanceResponse>(`/api/v1/integrations/${SLUG}/hubsoft/attendance`, {
      method: "POST",
      json: { codigo_cliente: codigo },
    });
    return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
  }, []);
  const fetchClientWorkOrders = useCallback(async (client: ClientCard) => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) return { ok: false, message: "Código do cliente não encontrado.", items: [] };
    const r = await apiFetch<ClientWorkOrderResponse>(`/api/v1/integrations/${SLUG}/hubsoft/work-orders`, {
      method: "POST",
      json: { codigo_cliente: codigo },
    });
    return { ok: !!r.ok, message: r.message, items: r.items ?? [] };
  }, []);
  const fetchClientFinancial = useCallback(async (client: ClientCard) => {
    const codigo = client.code?.trim() || client.id?.trim();
    if (!codigo) return { ok: false, message: "Código do cliente não encontrado.", invoices: [], summary: undefined };
    const r = await apiFetch<ClientFinancialResponse>(`/api/v1/integrations/${SLUG}/hubsoft/financial`, {
      method: "POST",
      json: { codigo_cliente: codigo },
    });
    return { ok: !!r.ok, message: r.message, invoices: r.invoices ?? [], summary: r.summary };
  }, []);

  async function openDetail(row: HubsoftReportServiceRow) {
    const base = rowToClientCard(row);
    setDetailClient(base);
    setDetailLoading(true);
    try {
      const full = await fetchClientDetail(base);
      setDetailClient(full);
    } finally {
      setDetailLoading(false);
    }
  }

  const rows = reportQ.data?.rows ?? [];

  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>
        Clientes e serviços
        <InfoHint label="Sobre este filtro">
          <p>
            Lista os serviços (login/IPv4/MAC/status) dos clientes que correspondem aos filtros. Preencher IPv4, MAC ou Login
            busca directamente pelo extrato de conexão (rápido); os outros filtros percorrem a base paginada por completo (não
            é amostra).
          </p>
        </InfoHint>
      </h3>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginTop: 10, marginBottom: 10 }}>
        <div className="field" style={{ margin: 0, minWidth: 200 }}>
          <label style={{ fontSize: 11 }}>Status do serviço</label>
          <select className="input" value={servicoStatus} onChange={(e) => setServicoStatus(e.target.value)}>
            {SERVICE_STATUS_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <div className="field" style={{ margin: 0, minWidth: 140 }}>
          <label style={{ fontSize: 11 }}>Cancelados</label>
          <select className="input" value={cancelado} onChange={(e) => setCancelado(e.target.value)}>
            <option value="">Excluir cancelados</option>
            <option value="sim">Incluir cancelados</option>
          </select>
        </div>
        <div className="field" style={{ margin: 0, minWidth: 140 }}>
          <label style={{ fontSize: 11 }}>Estado</label>
          <input className="input" value={estado} onChange={(e) => setEstado(e.target.value)} placeholder="RJ ou Rio de Janeiro" />
        </div>
        <div className="field" style={{ margin: 0, minWidth: 160 }}>
          <label style={{ fontSize: 11 }}>Cidade</label>
          <input className="input" value={cidade} onChange={(e) => setCidade(e.target.value)} placeholder="Cidade" />
        </div>
        <div className="field" style={{ margin: 0, minWidth: 160 }}>
          <label style={{ fontSize: 11 }}>Bairro</label>
          <input className="input" value={bairro} onChange={(e) => setBairro(e.target.value)} placeholder="Bairro" />
        </div>
        <div className="field" style={{ margin: 0, minWidth: 140 }}>
          <label style={{ fontSize: 11 }}>IPv4</label>
          <input className="input mono" value={ipv4} onChange={(e) => setIpv4(e.target.value)} placeholder="45.235.87.49" />
        </div>
        <div className="field" style={{ margin: 0, minWidth: 160 }}>
          <label style={{ fontSize: 11 }}>MAC</label>
          <input className="input mono" value={mac} onChange={(e) => setMac(e.target.value)} placeholder="98:03:8E:90:98:83" />
        </div>
        <div className="field" style={{ margin: 0, minWidth: 140 }}>
          <label style={{ fontSize: 11 }}>Login</label>
          <input className="input mono" value={login} onChange={(e) => setLogin(e.target.value)} placeholder="usuario123" />
        </div>
        <div style={{ alignSelf: "flex-end" }}>
          <div className="row" style={{ gap: 8 }}>
            <button type="button" className="btn btn--primary" disabled={reportQ.isFetching} onClick={runFilter}>
              {reportQ.isFetching ? "A consultar…" : "Consultar"}
            </button>
            <button type="button" className="btn" onClick={() => setBulkOpen(true)}>
              Consulta em massa
            </button>
          </div>
        </div>
      </div>

      {!reportQ.consulted ? (
        <EmptyState
          variant="inline"
          title="Sem relatório ainda."
          hint="Ajuste os filtros acima e clique em Consultar."
        />
      ) : reportQ.isLoading ? (
        <ConsultaLoading text="A carregar…" />
      ) : reportQ.isError ? (
        <div className="msg msg--err">{(reportQ.error as Error).message}</div>
      ) : !reportQ.data?.ok ? (
        <div className="msg msg--err">{reportQ.data?.message || "Falha ao consultar."}</div>
      ) : rows.length === 0 ? (
        <EmptyState
          title="Nenhum resultado para esses filtros."
          hint={reportQ.data?.message || "Tente alargar o período ou remover filtros de plano/localidade."}
        />
      ) : (
        <>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
            {fmtInt(rows.length)} serviço(s){reportQ.data.truncated ? " — lista truncada (base muito grande, refine os filtros)" : ""}.
            Clique numa linha para ver os dados completos do cliente.
          </p>
          <div className="table-wrap">
            <table style={{ fontSize: 12 }}>
              <thead>
                <tr>
                  <th>Cliente</th>
                  <th>Login</th>
                  <th>IPv4</th>
                  <th>MAC</th>
                  <th>Status</th>
                  <th>Conexão</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row, i) => (
                  <tr
                    key={`${row.client_code ?? ""}-${row.service_id ?? i}`}
                    style={{ cursor: "pointer" }}
                    onClick={() => void openDetail(row)}
                  >
                    <td>
                      {row.client_name || "—"}
                      {row.client_code ? <span className="mono" style={{ color: "var(--muted)", fontSize: 10 }}> · {row.client_code}</span> : null}
                    </td>
                    <td className="mono">{row.login || "—"}</td>
                    <td className="mono">{row.ipv4 || "—"}</td>
                    <td className="mono">{row.mac || "—"}</td>
                    <td>{row.status || "—"}</td>
                    <td>{connectedBadge(row.connected)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {bulkOpen ? <HubsoftBulkClientsModal onClose={() => setBulkOpen(false)} /> : null}

      {detailClient ? (
        <ClientDetailModal
          client={detailClient}
          loading={detailLoading}
          onClose={() => setDetailClient(null)}
          onFetchFinancial={fetchClientFinancial}
          onFetchAttendance={fetchClientAttendance}
          onFetchWorkOrders={fetchClientWorkOrders}
          attendanceEnabled
          workOrderEnabled
        />
      ) : null}
    </div>
  );
}

function localityLabel(loc: Pick<HubsoftServiceLocalityBreakdown, "city" | "state">): string {
  return loc.state ? `${loc.city} / ${loc.state}` : loc.city;
}

/** Tabela geral — uma linha por localidade (nome + total). Clicar numa linha abre
 * LocalityDetailModal com a repartição por status/plano só dela. */
function LocalityTable({ items, onSelect }: { items: HubsoftServiceLocalityBreakdown[]; onSelect: (loc: HubsoftServiceLocalityBreakdown) => void }) {
  if (items.length === 0) return null;
  return (
    <div className="table-wrap" style={{ marginTop: 10 }}>
      <table style={{ fontSize: 12 }}>
        <thead>
          <tr>
            <th>Localidade</th>
            <th>Total de serviços</th>
          </tr>
        </thead>
        <tbody>
          {items.map((loc) => (
            <tr
              key={`${loc.city}|${loc.state ?? ""}`}
              style={{ cursor: "pointer" }}
              onClick={() => onSelect(loc)}
            >
              <td>{localityLabel(loc)}</td>
              <td className="mono">{fmtInt(loc.total)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Modal com a repartição por status e por plano de UMA localidade — aberta ao clicar numa
 * linha de LocalityTable. */
type LocalityDetailTab = "status" | "plan" | "neighborhood";

const LOCALITY_DETAIL_TABS: { id: LocalityDetailTab; label: string; headerLabel: string }[] = [
  { id: "status", label: "Status", headerLabel: "Status" },
  { id: "plan", label: "Plano", headerLabel: "Plano" },
  { id: "neighborhood", label: "Bairro", headerLabel: "Bairro" },
];

/** Uma tabela de cada vez (Status/Plano/Bairro em abas) em vez de lado a lado — com o Status
 * (poucas linhas) e o Plano (muitas linhas) lado a lado o modal ficava desproporcional (uma
 * coluna bem mais alta que a outra, reportado). Bairro é só totais de propósito (ver
 * ByNeighborhood no backend) — não repete status/plano por bairro, já é cheio o suficiente. */
function LocalityDetailModal({ loc, onClose }: { loc: HubsoftServiceLocalityBreakdown | null; onClose: () => void }) {
  const [tab, setTab] = useState<LocalityDetailTab>("status");
  useEffect(() => {
    if (loc) setTab("status");
  }, [loc?.city, loc?.state]);

  if (!loc) return null;
  const itemsByTab: Record<LocalityDetailTab, { name: string; count: number }[]> = {
    status: loc.by_status,
    plan: loc.by_plan,
    neighborhood: loc.by_neighborhood,
  };
  const active = LOCALITY_DETAIL_TABS.find((t) => t.id === tab) ?? LOCALITY_DETAIL_TABS[0];

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 640, width: "100%", display: "flex", flexDirection: "column" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
          <div>
            <h3 style={{ margin: 0 }}>{localityLabel(loc)}</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {fmtInt(loc.total)} serviço(s) nesta localidade
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>
        <div className="row" style={{ gap: 6, marginTop: 12 }}>
          {LOCALITY_DETAIL_TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`btn btn--sm${tab === t.id ? " btn--primary" : ""}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <div style={{ maxHeight: "62vh", overflowY: "auto", marginTop: 4 }}>
          <StatusBreakdownTable items={itemsByTab[tab]} labelHeader={active.headerLabel} />
        </div>
        <div className="row" style={{ justifyContent: "flex-end", marginTop: 14 }}>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

const SERVICE_TELEGRAM_SECTIONS: { id: string; label: string }[] = [
  { id: "total", label: "Total de logins" },
  { id: "status", label: "Total por status" },
  { id: "plan", label: "Total por plano" },
  { id: "locality_totals", label: "Total por localidade" },
];

/** Botão "Enviar por Telegram" da aba Serviços — ao contrário de TelegramSendButton (relatórios
 * por período, sempre mandam o resumo inteiro), aqui o utilizador escolhe PRIMEIRO quais blocos
 * mandar (pedido explícito), porque o relatório inteiro (todos os planos de todas as localidades)
 * facilmente estoura o tamanho de uma mensagem Telegram legível. Manda os dados já carregados
 * nesta tela (data) — não pede ao backend para varrer a HubSoft outra vez, reaproveita a mesma
 * fotografia que está no ecrã (mesmo espírito do cache de 5min da consulta). */
function ServicesTelegramButton({ data }: { data: HubsoftServicesReportResponse }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [specificLocalityKey, setSpecificLocalityKey] = useState("");
  const [feedback, setFeedback] = useState<{ ok: boolean; message: string } | null>(null);

  const m = useMutation({
    mutationFn: () => {
      const sections = Array.from(selected);
      if (specificLocalityKey) sections.push("specific_locality");
      return apiFetch(`/api/v1/integrations/${SLUG}/hubsoft/report/services/telegram`, {
        method: "POST",
        json: {
          total: data.total,
          by_status: data.by_status,
          by_plan: data.by_plan,
          by_locality: data.by_locality,
          sections,
          specific_locality_key: specificLocalityKey || undefined,
        },
      });
    },
    onSuccess: () => {
      setFeedback({ ok: true, message: "Enviado para o Telegram." });
      setOpen(false);
    },
    onError: (e) => setFeedback({ ok: false, message: e instanceof Error ? e.message : "Falha ao enviar." }),
  });

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const canSend = selected.size > 0 || specificLocalityKey !== "";

  return (
    <>
      <div className="row" style={{ gap: 8, alignItems: "center" }}>
        <button
          type="button"
          className="btn btn--sm"
          onClick={() => {
            setFeedback(null);
            setOpen(true);
          }}
        >
          <Send size={12} style={{ marginRight: 4, verticalAlign: -2 }} />
          Enviar por Telegram
        </button>
        {feedback ? (
          <span style={{ fontSize: 11, color: feedback.ok ? "var(--ok)" : "var(--err)" }}>{feedback.message}</span>
        ) : null}
      </div>
      {open
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={() => setOpen(false)}>
              <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 440 }} onMouseDown={(e) => e.stopPropagation()}>
                <h3 style={{ margin: "0 0 4px" }}>Enviar por Telegram</h3>
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 12px" }}>
                  Escolha o que mandar — para não ficar uma mensagem gigante, "por localidade" manda só o total de cada
                  uma (sem detalhar plano/status). Use "Localidade específica" para ver o detalhe de uma só.
                </p>
                <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  {SERVICE_TELEGRAM_SECTIONS.map((s) => (
                    <label key={s.id} className="row" style={{ gap: 8, alignItems: "center", fontSize: 13 }}>
                      <input type="checkbox" checked={selected.has(s.id)} onChange={() => toggle(s.id)} />
                      {s.label}
                    </label>
                  ))}
                  <button
                    type="button"
                    className="btn btn--sm"
                    style={{ alignSelf: "flex-start" }}
                    onClick={() => setSelected(new Set(SERVICE_TELEGRAM_SECTIONS.map((s) => s.id)))}
                  >
                    Tudo
                  </button>
                </div>
                <div style={{ marginTop: 12, paddingTop: 12, borderTop: "1px solid var(--border)" }}>
                  <label style={{ fontSize: 13, display: "block", marginBottom: 4 }}>Localidade específica (opcional)</label>
                  <select className="input" value={specificLocalityKey} onChange={(e) => setSpecificLocalityKey(e.target.value)}>
                    <option value="">— Nenhuma —</option>
                    {data.by_locality.map((loc) => (
                      <option key={`${loc.city}|${loc.state ?? ""}`} value={`${loc.city}|${loc.state ?? ""}`}>
                        {localityLabel(loc)} ({fmtInt(loc.total)})
                      </option>
                    ))}
                  </select>
                  <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>
                    Manda o total, por status e por plano só dessa localidade.
                  </p>
                </div>
                <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
                  <button type="button" className="btn" onClick={() => setOpen(false)}>
                    Cancelar
                  </button>
                  <button type="button" className="btn btn--primary" disabled={!canSend || m.isPending} onClick={() => m.mutate()}>
                    {m.isPending ? "A enviar…" : "Enviar"}
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}

/** Aba Relatório → Serviços: quantos serviços existem, quantos em cada status, quantos em cada
 * plano, e a mesma repartição dentro de cada localidade. Sem período — fotografia do estado
 * actual da base inteira (ver BuildServicesReport no backend), por isso pode demorar mais que os
 * relatórios por período. */
function ServicesReportSection() {
  const [selectedLocality, setSelectedLocality] = useState<HubsoftServiceLocalityBreakdown | null>(null);
  // Só consulta ao clicar em "Consultar" (varre a base inteira — não deve rodar sem querer).
  const q = useConsulta<true, HubsoftServicesReportResponse>("hubsoft-report-services", () =>
    apiFetch<HubsoftServicesReportResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/services`, { timeoutMs: 5 * 60_000 }),
  );
  const d = q.data;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div className="card" style={{ padding: 14 }}>
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 12 }}>
          <div>
            <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Serviços — status, localidade e plano</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>
              Fotografia do estado actual da base inteira (não é amostra nem depende de período).
            </p>
          </div>
          <div className="row" style={{ gap: 8, alignItems: "center" }}>
            <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={() => void q.run(true)}>
              {q.isFetching ? "A consultar…" : "Consultar"}
            </button>
            {d?.ok ? <ServicesTelegramButton data={d} /> : null}
          </div>
        </div>
      </div>

      {!q.consulted ? (
        <div className="card" style={{ padding: 14 }}>
          <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Clique em <b>Consultar</b> para carregar (varre a base inteira e pode demorar). Nada é buscado automaticamente.</p>
        </div>
      ) : q.isLoading ? (
        <div className="card" style={{ padding: 14 }}>
          <ConsultaLoading text="A carregar (pode demorar — varre a base inteira)…" />
        </div>
      ) : q.isError ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="msg msg--err">{(q.error as Error).message}</div>
        </div>
      ) : !d?.ok ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="msg msg--err">{d?.message || "Falha ao consultar."}</div>
        </div>
      ) : (
        <>
          <div className="card" style={{ padding: 14 }}>
            <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "minmax(0, 220px)" }}>
              <div className="stat">
                <div className="stat__k">Total de serviços</div>
                <div className="stat__v">{fmtInt(d.total)}</div>
              </div>
            </div>
            {d.truncated ? (
              <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>
                Base maior que o teto de páginas do relatório — os números cobrem uma parte da base, não o total exacto.
              </p>
            ) : null}
          </div>

          <div className="card" style={{ padding: 14 }}>
            <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>Por status</h4>
            <StatusBreakdownTable items={d.by_status} labelHeader="Status" />
          </div>

          <div className="card" style={{ padding: 14 }}>
            <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>Por plano</h4>
            <StatusBreakdownTable items={d.by_plan} labelHeader="Plano" />
          </div>

          <div className="card" style={{ padding: 14 }}>
            <h4 style={{ margin: "0 0 4px", fontSize: 13 }}>Por localidade ({fmtInt(d.by_locality.length)})</h4>
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
              Clique numa localidade para ver a repartição por status, plano e bairro só dela.
            </p>
            <LocalityTable items={d.by_locality} onSelect={setSelectedLocality} />
          </div>
        </>
      )}
      <LocalityDetailModal loc={selectedLocality} onClose={() => setSelectedLocality(null)} />
    </div>
  );
}

function fmtDateBR(iso?: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso ?? "");
  return m ? `${m[3]}/${m[2]}/${m[1]}` : iso || "—";
}

function csvEsc(v: string): string {
  return /[",\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** Aba Relatório → Bloqueios: serviços suspensos por débito numa janela de "dias atrás" ou datas.
 * A HubSoft não expõe a data exacta da suspensão — usa-se a última alteração do serviço
 * (data_atualizacao), ver BuildBlockedServicesReport no backend. */
function BlockedReportSection() {
  // O usuário escolhe UM jeito de definir o período; só os campos do modo escolhido aparecem.
  const [mode, setMode] = useState<"preset" | "dates" | "daysago">("preset");
  const [preset, setPreset] = useState<"15" | "30" | "month">("30");
  const [from, setFrom] = useState(todayISO(-30));
  const [to, setTo] = useState(todayISO());
  const [xDays, setXDays] = useState("0");
  const [yDays, setYDays] = useState("30");
  const [includePartial, setIncludePartial] = useState(false);
  const [search, setSearch] = useState("");

  const { missing } = useConsultaToast();
  const q = useConsulta<{ from: string; to: string; partial: boolean }, HubsoftBlockedReportResponse>("hubsoft-report-blocked", (a) =>
    apiFetch<HubsoftBlockedReportResponse>(
      `/api/v1/integrations/${SLUG}/hubsoft/report/blocked?from=${a.from}&to=${a.to}` +
        `&status=${a.partial ? "suspenso_debito,suspenso_parcialmente" : "suspenso_debito"}`,
      { timeoutMs: 5 * 60_000 },
    ),
  );
  const d = q.data;
  const applied = q.applied ?? { from, to, partial: includePartial };

  function consult() {
    let f = "";
    let t = "";
    if (mode === "preset") {
      t = todayISO();
      if (preset === "month") {
        const now = new Date();
        f = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-01`;
      } else {
        f = todayISO(-Number(preset));
      }
    } else if (mode === "dates") {
      if (!from || !to) return missing("informe as duas datas (De / Até).");
      if (from > to) return missing("o período está invertido (a data inicial é maior que a final).");
      f = from;
      t = to;
    } else {
      if (xDays.trim() === "" || yDays.trim() === "") return missing("informe os dois valores de dias (de X até Y dias atrás).");
      const x = Number(xDays);
      const y = Number(yDays);
      if (x > y) return missing("“de X dias” não pode ser maior que “até Y dias” (ex.: de 0 a 30).");
      f = todayISO(-y);
      t = todayISO(-x);
    }
    void q.run({ from: f, to: t, partial: includePartial });
  }

  const rows = useMemo(() => {
    const s = search.trim().toLowerCase();
    const all = d?.rows ?? [];
    if (!s) return all;
    return all.filter((r) =>
      [r.client_name, r.client_code, r.login, r.service_name, r.city, r.phone].some((v) => (v ?? "").toLowerCase().includes(s)),
    );
  }, [d?.rows, search]);

  function exportCsv() {
    const head = ["Cliente", "Código", "Telefone", "Serviço", "Login", "Cidade", "Status", "Bloqueado desde", "Dias bloqueado"];
    const lines = [head.join(",")];
    for (const r of rows) {
      lines.push(
        [r.client_name, r.client_code, r.phone, r.service_name, r.login, r.city, r.status, fmtDateBR(r.blocked_at), String(r.days_blocked)]
          .map((v) => csvEsc(v ?? ""))
          .join(","),
      );
    }
    const blob = new Blob([`﻿${lines.join("\r\n")}`], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `bloqueios-hubsoft-${applied.from}_a_${applied.to}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div className="card" style={{ padding: 14 }}>
        <h3 style={{ margin: "0 0 12px", fontSize: 15, display: "flex", alignItems: "center", gap: 6 }}>
          Bloqueios por débito — há quantos dias
          <InfoHint label="Sobre este relatório">
            <p>
              Serviços que estão suspensos por débito agora, filtrados pela data do ÚLTIMO bloqueio (se o serviço foi liberado e
              bloqueado de novo, vale o bloqueio mais recente).
            </p>
          </InfoHint>
        </h3>

        <div className="hubsoft-filter-box">
          <div className="hubsoft-filter-box__options">
            <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3 }}>
              Como definir o período
              <select className="input" value={mode} onChange={(e) => setMode(e.target.value as "preset" | "dates" | "daysago")}>
                <option value="preset">Atalho</option>
                <option value="dates">Datas específicas</option>
                <option value="daysago">Dias atrás</option>
              </select>
            </label>
            {mode === "preset" ? (
              <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3 }}>
                Período
                <select className="input" value={preset} onChange={(e) => setPreset(e.target.value as "15" | "30" | "month")}>
                  <option value="15">Últimos 15 dias</option>
                  <option value="30">Últimos 30 dias</option>
                  <option value="month">Este mês</option>
                </select>
              </label>
            ) : mode === "dates" ? (
              <>
                <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3 }}>
                  De
                  <input type="date" className="input" value={from} onChange={(e) => setFrom(e.target.value)} />
                </label>
                <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3 }}>
                  Até
                  <input type="date" className="input" value={to} onChange={(e) => setTo(e.target.value)} />
                </label>
              </>
            ) : (
              <>
                <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3 }}>
                  Bloqueados há pelo menos (dias)
                  <input className="input" inputMode="numeric" value={xDays} onChange={(e) => setXDays(e.target.value.replace(/\D/g, ""))} />
                </label>
                <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 3 }}>
                  e no máximo (dias)
                  <input className="input" inputMode="numeric" value={yDays} onChange={(e) => setYDays(e.target.value.replace(/\D/g, ""))} />
                </label>
              </>
            )}
          </div>
          {mode === "daysago" ? (
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "-6px 0 0" }}>
              Ex.: de 0 a 30 = bloqueados nos últimos 30 dias; de 10 a 10 = bloqueados há exatamente 10 dias.
            </p>
          ) : null}

          <div className="hubsoft-filter-box__actions">
            <Switch checked={includePartial} onChange={setIncludePartial} label="Incluir suspensos parcialmente" />
            <div className="row" style={{ gap: 12, alignItems: "center" }}>
              {q.consulted ? (
                <span style={{ fontSize: 11, color: "var(--muted)" }}>
                  Última consulta: {fmtDateBR(applied.from)} a {fmtDateBR(applied.to)}
                </span>
              ) : null}
              <button type="button" className="btn btn--primary" disabled={q.isFetching} onClick={consult}>
                {q.isFetching ? "A consultar…" : "Consultar"}
              </button>
            </div>
          </div>
        </div>
      </div>

      {!q.consulted && !q.isFetching ? (
        <div className="card" style={{ padding: 14 }}>
          <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Escolha o período e clique em <b>Consultar</b>. Nada é buscado automaticamente.</p>
        </div>
      ) : q.isLoading || q.isFetching ? (
        <div className="card" style={{ padding: 14 }}>
          <ConsultaLoading text="A consultar a HubSoft… (pode demorar em períodos longos)" />
        </div>
      ) : q.isError ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="msg msg--err">{(q.error as Error).message}</div>
        </div>
      ) : !d?.ok ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="msg msg--err">{d?.message || "Falha ao consultar."}</div>
        </div>
      ) : (
        <>
          <div className="card" style={{ padding: 14 }}>
            <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(2, minmax(0, 200px))" }}>
              <div className="stat">
                <div className="stat__k">Serviços bloqueados</div>
                <div className="stat__v">{fmtInt(d.total)}</div>
              </div>
              <div className="stat">
                <div className="stat__k">Média de dias bloqueado</div>
                <div className="stat__v">{d.avg_days.toFixed(1)}</div>
              </div>
            </div>
            {d.total > 0 ? (
              <div style={{ marginTop: 10 }}>
                <StatusBreakdownTable items={d.buckets.filter((b) => b.count > 0)} labelHeader="Tempo bloqueado" />
              </div>
            ) : null}
            {d.date_source === "mixed" ? (
              <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>
                Alguns serviços não trazem a data do último bloqueio; para eles usa-se a data da última alteração do cadastro (marcados com “≈”).
              </p>
            ) : null}
            {d.truncated ? (
              <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>
                Resultado maior que o teto de páginas — a lista pode estar incompleta. Reduza o período.
              </p>
            ) : null}
          </div>

          <div className="card" style={{ padding: 14 }}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8, marginBottom: 8 }}>
              <h4 style={{ margin: 0, fontSize: 13 }}>Serviços ({fmtInt(rows.length)})</h4>
              <div className="row" style={{ gap: 6 }}>
                <input className="input" style={{ fontSize: 12 }} placeholder="Filtrar por nome, login, cidade…" value={search} onChange={(e) => setSearch(e.target.value)} />
                <button type="button" className="btn btn--sm" disabled={rows.length === 0} onClick={exportCsv}>
                  Exportar CSV
                </button>
              </div>
            </div>
            {rows.length === 0 ? (
              <div className="msg">{d.message || "Nenhum serviço encontrado."}</div>
            ) : (
              <div className="table-wrap integration-support-table" style={{ maxHeight: 520, overflow: "auto" }}>
                <table className="integration-support-table__grid">
                  <thead>
                    <tr>
                      <th>Cliente</th>
                      <th>Serviço</th>
                      <th>Login</th>
                      <th>Cidade</th>
                      <th>Telefone</th>
                      <th>Bloqueado desde</th>
                      <th>Dias</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r, i) => (
                      <tr key={r.service_id || i}>
                        <td className="integration-support-table__cell">
                          {r.client_name || "—"}
                          {r.client_code ? <span className="mono integration-support-table__meta"> · {r.client_code}</span> : null}
                        </td>
                        <td className="integration-support-table__cell">{r.service_name || "—"}</td>
                        <td className="mono integration-support-table__cell">{r.login || "—"}</td>
                        <td className="integration-support-table__cell">{r.city || "—"}</td>
                        <td className="mono integration-support-table__cell">{r.phone || "—"}</td>
                        <td className="mono integration-support-table__cell">
                          {r.date_approx ? "≈ " : ""}
                          {fmtDateBR(r.blocked_at)}
                        </td>
                        <td className="mono integration-support-table__cell" style={{ color: r.days_blocked > 30 ? "var(--err)" : undefined }}>
                          {r.days_blocked}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function AttendanceReportSection() {
  const [from, setFrom] = useState(todayISO(-30));
  const [to, setTo] = useState(todayISO());
  const { missing } = useConsultaToast();
  const q = useConsulta<{ from: string; to: string }, HubsoftAttendanceReportResponse>("hubsoft-report-attendance", (p) =>
    apiFetch<HubsoftAttendanceReportResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/attendance?data_inicio=${p.from}&data_fim=${p.to}`, { timeoutMs: 5 * 60_000 }),
  );
  const d = q.data;
  function consult() {
    if (!from || !to) return missing("informe o período (De / Até).");
    if (from > to) return missing("o período está invertido (a data inicial é maior que a final).");
    void q.run({ from, to });
  }

  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Atendimentos por período</h3>
      <PeriodPicker
        from={from}
        to={to}
        onChange={(f, t) => {
          setFrom(f);
          setTo(t);
        }}
      />
      <div className="row" style={{ marginBottom: 8, gap: 8, alignItems: "center" }}>
        <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={consult}>
          {q.isFetching ? "A consultar…" : "Consultar"}
        </button>
        {d?.ok && q.applied ? (
          <TelegramSendButton path={`/api/v1/integrations/${SLUG}/hubsoft/report/attendance/telegram?data_inicio=${q.applied.from}&data_fim=${q.applied.to}`} />
        ) : null}
      </div>
      {!q.consulted ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>Escolha o período e clique em <b>Consultar</b>. Nada é buscado automaticamente.</p>
      ) : q.isLoading ? (
        <ConsultaLoading text="A carregar…" />
      ) : q.isError ? (
        <div className="msg msg--err">{(q.error as Error).message}</div>
      ) : !d?.ok ? (
        <div className="msg msg--err">{d?.message || "Falha ao consultar."}</div>
      ) : (
        <>
          <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(4, minmax(0, 1fr))" }}>
            <div className="stat">
              <div className="stat__k">Total no período</div>
              <div className="stat__v">{fmtInt(d.total)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">Abertos (ainda sem fechamento)</div>
              <div className="stat__v" style={{ color: "var(--warn)" }}>{fmtInt(d.open)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">Realizados (fechados)</div>
              <div className="stat__v" style={{ color: "var(--ok)" }}>{fmtInt(d.closed)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">% realizados</div>
              <div className="stat__v">{fmtPct(d.closed_pct)}</div>
            </div>
          </div>
          {d.truncated ? <p style={{ fontSize: 11, color: "var(--warn)" }}>Período muito grande — resultado truncado, refine as datas.</p> : null}
          <StatusBreakdownTable items={d.by_status} />
        </>
      )}
    </div>
  );
}

function WorkOrderReportSection() {
  const [from, setFrom] = useState(todayISO(-30));
  const [to, setTo] = useState(todayISO());
  const { missing } = useConsultaToast();
  const q = useConsulta<{ from: string; to: string }, HubsoftWorkOrderReportResponse>("hubsoft-report-work-orders", (p) =>
    apiFetch<HubsoftWorkOrderReportResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/work-orders?data_inicio=${p.from}&data_fim=${p.to}`, { timeoutMs: 5 * 60_000 }),
  );
  const d = q.data;
  function consult() {
    if (!from || !to) return missing("informe o período (De / Até).");
    if (from > to) return missing("o período está invertido (a data inicial é maior que a final).");
    void q.run({ from, to });
  }

  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Ordens de serviço por período</h3>
      <PeriodPicker
        from={from}
        to={to}
        onChange={(f, t) => {
          setFrom(f);
          setTo(t);
        }}
      />
      <div className="row" style={{ marginBottom: 8, gap: 8, alignItems: "center" }}>
        <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={consult}>
          {q.isFetching ? "A consultar…" : "Consultar"}
        </button>
        {d?.ok && q.applied ? (
          <TelegramSendButton path={`/api/v1/integrations/${SLUG}/hubsoft/report/work-orders/telegram?data_inicio=${q.applied.from}&data_fim=${q.applied.to}`} />
        ) : null}
      </div>
      {!q.consulted ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>Escolha o período e clique em <b>Consultar</b>. Nada é buscado automaticamente.</p>
      ) : q.isLoading ? (
        <ConsultaLoading text="A carregar…" />
      ) : q.isError ? (
        <div className="msg msg--err">{(q.error as Error).message}</div>
      ) : !d?.ok ? (
        <div className="msg msg--err">{d?.message || "Falha ao consultar."}</div>
      ) : (
        <>
          <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(3, minmax(0, 1fr))" }}>
            <div className="stat">
              <div className="stat__k">Total no período</div>
              <div className="stat__v">{fmtInt(d.total)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">Finalizadas</div>
              <div className="stat__v" style={{ color: "var(--ok)" }}>{fmtInt(d.finished)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">% finalizadas</div>
              <div className="stat__v">{fmtPct(d.finished_pct)}</div>
            </div>
          </div>
          {d.truncated ? <p style={{ fontSize: 11, color: "var(--warn)" }}>Período muito grande — resultado truncado, refine as datas.</p> : null}

          <h4 style={{ margin: "16px 0 6px", fontSize: 13 }}>Por técnico</h4>
          {d.by_technician.length === 0 ? (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>Sem dados de técnico responsável no período.</p>
          ) : (
            <div className="table-wrap">
              <table style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>Técnico</th>
                    <th>Total de O.S.</th>
                    <th>Finalizadas</th>
                    <th>% do total finalizado</th>
                  </tr>
                </thead>
                <tbody>
                  {d.by_technician.map((t) => (
                    <tr key={t.technician}>
                      <td>{t.technician}</td>
                      <td className="mono">{fmtInt(t.total)}</td>
                      <td className="mono">{fmtInt(t.finished)}</td>
                      <td className="mono">{fmtPct(t.pct_of_total_finished)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <h4 style={{ margin: "16px 0 6px", fontSize: 13 }}>Por status</h4>
          <StatusBreakdownTable items={d.by_status} />
        </>
      )}
    </div>
  );
}

type FinancialMode = "range" | "month" | "last_n_months";

function FinancialKPIs({ d }: { d: HubsoftFinancialReportResponse }) {
  return (
    <>
      <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(4, minmax(0, 1fr))" }}>
        <div className="stat">
          <div className="stat__k">Total no período</div>
          <div className="stat__v" style={{ fontSize: 15 }}>{fmtCurrency(d.total_value)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Recebido ({fmtPct(d.paid_pct)})</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--ok)" }}>{fmtCurrency(d.paid_value)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Em aberto ({fmtPct(d.open_pct)})</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--warn)" }}>{fmtCurrency(d.open_value)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Vencido ({fmtPct(d.overdue_pct)})</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--err)" }}>{fmtCurrency(d.overdue_value)}</div>
        </div>
      </div>
      <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 10 }}>
        {fmtInt(d.total)} fatura(s) — {fmtInt(d.paid_count)} paga(s), {fmtInt(d.open_count)} em aberto, {fmtInt(d.overdue_count)} vencida(s).
        {d.truncated ? " Período muito grande — resultado truncado, refine as datas." : ""}
      </p>
    </>
  );
}

type FinancialMonthRow = { label: string; d: HubsoftFinancialReportResponse };
type FinancialConsultaParams = { mode: FinancialMode; from: string; to: string; months: number };
type FinancialConsultaResult =
  | { ok: true; kind: "single"; report: HubsoftFinancialReportResponse }
  | { ok: true; kind: "months"; months: number; rows: FinancialMonthRow[] }
  | { ok: false; message: string };

async function fetchFinancialConsulta(p: FinancialConsultaParams): Promise<FinancialConsultaResult> {
  const one = (from: string, to: string) =>
    apiFetch<HubsoftFinancialReportResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/financial?data_inicio=${from}&data_fim=${to}`, { timeoutMs: 5 * 60_000 });
  if (p.mode !== "last_n_months") {
    const r = await one(p.from, p.to);
    return r.ok ? { ok: true, kind: "single", report: r } : { ok: false, message: r.message || "Falha ao consultar." };
  }
  // "Últimos X meses": 1 pedido por mês (cada um já paginado/limitado ao próprio mês pela API), em paralelo.
  const ranges = lastNMonths(p.months);
  const res = await Promise.all(ranges.map((r) => one(r.from, r.to)));
  const bad = res.find((r) => !r.ok);
  if (bad) return { ok: false, message: bad.message || "Falha ao consultar um dos meses." };
  return { ok: true, kind: "months", months: p.months, rows: res.map((d, i) => ({ label: ranges[i].label, d })) };
}

/** Modo "Últimos X meses": totais + média a partir dos meses já consultados (botão Consultar). */
function FinancialMonthsView({ months, monthRows }: { months: number; monthRows: FinancialMonthRow[] }) {
  const rows = monthRows.map((r) => ({ label: r.label, d: r.d as HubsoftFinancialReportResponse | undefined }));
  const okRows = rows.filter((r) => r.d?.ok);
  const n = okRows.length || 1;
  const totals = okRows.reduce(
    (acc, r) => ({
      total_value: acc.total_value + (r.d?.total_value ?? 0),
      paid_value: acc.paid_value + (r.d?.paid_value ?? 0),
      open_value: acc.open_value + (r.d?.open_value ?? 0),
      overdue_value: acc.overdue_value + (r.d?.overdue_value ?? 0),
    }),
    { total_value: 0, paid_value: 0, open_value: 0, overdue_value: 0 },
  );

  return (
    <>
      <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(4, minmax(0, 1fr))" }}>
        <div className="stat">
          <div className="stat__k">Total ({months} meses)</div>
          <div className="stat__v" style={{ fontSize: 15 }}>{fmtCurrency(totals.total_value)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Recebido</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--ok)" }}>{fmtCurrency(totals.paid_value)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Em aberto</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--warn)" }}>{fmtCurrency(totals.open_value)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Vencido</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--err)" }}>{fmtCurrency(totals.overdue_value)}</div>
        </div>
      </div>
      <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(4, minmax(0, 1fr))", marginTop: 8 }}>
        <div className="stat">
          <div className="stat__k">Média mensal — total</div>
          <div className="stat__v" style={{ fontSize: 14 }}>{fmtCurrency(totals.total_value / n)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Média mensal — recebido</div>
          <div className="stat__v" style={{ fontSize: 14, color: "var(--ok)" }}>{fmtCurrency(totals.paid_value / n)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Média mensal — em aberto</div>
          <div className="stat__v" style={{ fontSize: 14, color: "var(--warn)" }}>{fmtCurrency(totals.open_value / n)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Média mensal — vencido</div>
          <div className="stat__v" style={{ fontSize: 14, color: "var(--err)" }}>{fmtCurrency(totals.overdue_value / n)}</div>
        </div>
      </div>
      <div className="table-wrap" style={{ marginTop: 14 }}>
        <table style={{ fontSize: 12 }}>
          <thead>
            <tr>
              <th>Mês</th>
              <th>Total</th>
              <th>Recebido</th>
              <th>Em aberto</th>
              <th>Vencido</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.label}>
                <td style={{ textTransform: "capitalize" }}>{r.label}</td>
                <td className="mono">{r.d?.ok ? fmtCurrency(r.d.total_value) : "—"}</td>
                <td className="mono" style={{ color: "var(--ok)" }}>{r.d?.ok ? fmtCurrency(r.d.paid_value) : "—"}</td>
                <td className="mono" style={{ color: "var(--warn)" }}>{r.d?.ok ? fmtCurrency(r.d.open_value) : "—"}</td>
                <td className="mono" style={{ color: "var(--err)" }}>{r.d?.ok ? fmtCurrency(r.d.overdue_value) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function FinancialReportSection() {
  const [mode, setMode] = useState<FinancialMode>("range");
  const [from, setFrom] = useState(todayISO(-30));
  const [to, setTo] = useState(todayISO());
  const [month, setMonth] = useState(currentMonthValue());
  const [nMonths, setNMonths] = useState(6);
  const { missing } = useConsultaToast();

  const monthPeriod = useMemo(() => monthRange(month), [month]);
  const activeFrom = mode === "month" ? monthPeriod.from : from;
  const activeTo = mode === "month" ? monthPeriod.to : to;

  const q = useConsulta<FinancialConsultaParams, FinancialConsultaResult>("hubsoft-report-financial", fetchFinancialConsulta);
  const res = q.data && q.data.ok ? q.data : undefined;
  const d = res?.kind === "single" ? res.report : undefined;

  function consult() {
    if (mode !== "last_n_months") {
      if (mode === "month" && !month) return missing("escolha o mês.");
      if (!activeFrom || !activeTo) return missing("informe o período (De / Até).");
      if (activeFrom > activeTo) return missing("o período está invertido (a data inicial é maior que a final).");
    }
    void q.run({ mode, from: activeFrom, to: activeTo, months: nMonths });
  }

  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Financeiro por período</h3>
      <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--muted)" }}>Filtra pela data de vencimento das faturas.</p>

      <div className="row" style={{ gap: 0, marginBottom: 10 }}>
        <button type="button" className={`btn btn--sm${mode === "range" ? " btn--primary" : ""}`} onClick={() => setMode("range")}>
          Período livre
        </button>
        <button type="button" className={`btn btn--sm${mode === "month" ? " btn--primary" : ""}`} onClick={() => setMode("month")}>
          Mês específico
        </button>
        <button
          type="button"
          className={`btn btn--sm${mode === "last_n_months" ? " btn--primary" : ""}`}
          onClick={() => setMode("last_n_months")}
        >
          Últimos X meses
        </button>
      </div>

      {mode === "range" && (
        <PeriodPicker
          from={from}
          to={to}
          onChange={(f, t) => {
            setFrom(f);
            setTo(t);
          }}
        />
      )}
      {mode === "month" && (
        <div className="row" style={{ gap: 8, alignItems: "flex-end", marginBottom: 12 }}>
          <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
            Mês
            <input type="month" className="input" value={month} onChange={(e) => setMonth(e.target.value)} />
          </label>
        </div>
      )}
      {mode === "last_n_months" && (
        <div className="row" style={{ gap: 6, alignItems: "center", marginBottom: 12 }}>
          <span style={{ fontSize: 11, color: "var(--muted)" }}>Meses:</span>
          {[3, 6, 12].map((n) => (
            <button key={n} type="button" className={`btn btn--sm${nMonths === n ? " btn--primary" : ""}`} onClick={() => setNMonths(n)}>
              {n}
            </button>
          ))}
        </div>
      )}

      <div className="row" style={{ marginBottom: 8, gap: 8, alignItems: "center" }}>
        <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={consult}>
          {q.isFetching ? "A consultar…" : "Consultar"}
        </button>
        {d && q.applied && q.applied.mode !== "last_n_months" ? (
          <TelegramSendButton path={`/api/v1/integrations/${SLUG}/hubsoft/report/financial/telegram?data_inicio=${q.applied.from}&data_fim=${q.applied.to}`} />
        ) : null}
      </div>

      {!q.consulted ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>Escolha o período e clique em <b>Consultar</b>. Nada é buscado automaticamente.</p>
      ) : q.isLoading ? (
        <ConsultaLoading text="A carregar…" />
      ) : q.isError ? (
        <div className="msg msg--err">{(q.error as Error).message}</div>
      ) : q.data && !q.data.ok ? (
        <div className="msg msg--err">{q.data.message || "Falha ao consultar."}</div>
      ) : res?.kind === "months" ? (
        <FinancialMonthsView months={res.months} monthRows={res.rows} />
      ) : d ? (
        <FinancialKPIs d={d} />
      ) : null}
    </div>
  );
}

type TenureReportResponse = {
  ok: boolean;
  message?: string;
  total: number;
  buckets: { name: string; count: number }[];
  services?: number[];
  /** Limites (YYYY-MM-DD) de cada faixa, na ordem de `buckets`. */
  details?: { name: string; from: string; to: string }[];
  truncated?: boolean;
};

/** Aba Relatório → Tempo de cliente: clientes ATIVOS por faixa de data da venda. Só totais. */
function TenureReportSection() {
  // Muda pouco: o resultado fica salvo (inclusive ao recarregar a página) e só "Atualizar" consulta de novo.
  const q = useConsulta<true, TenureReportResponse>(
    "hubsoft-report-tenure",
    () => apiFetch<TenureReportResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/tenure`, { timeoutMs: 6 * 60_000 }),
    { persist: true },
  );
  const d = q.data;
  const max = Math.max(1, ...(d?.buckets ?? []).map((b) => b.count));
  const [band, setBand] = useState<TenureBand | null>(null);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div className="card" style={{ padding: 14 }}>
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 12 }}>
          <div>
            <h3 style={{ margin: 0, fontSize: 15, display: "flex", alignItems: "center", gap: 6 }}>
              Tempo de cliente
              <InfoHint label="Sobre este relatório">
                <p>
                  Clientes ativos (serviço habilitado) por tempo desde a data da venda. Cliente com mais de um serviço ativo entra na faixa
                  do serviço mais antigo.
                </p>
              </InfoHint>
            </h3>
            {q.at ? (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>Atualizado em {fmtConsultaAt(q.at)}</p>
            ) : null}
          </div>
          <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={() => void q.run(true)}>
            {q.isFetching ? "A atualizar…" : q.consulted ? "Atualizar" : "Consultar"}
          </button>
        </div>
      </div>

      {!q.consulted ? (
        <div className="card" style={{ padding: 14 }}>
          <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Clique em <b>Consultar</b> (varre a base ativa e pode demorar). Nada é buscado automaticamente.</p>
        </div>
      ) : q.isLoading ? (
        <div className="card" style={{ padding: 14 }}>
          <ConsultaLoading text="A consultar a HubSoft por faixa de data…" />
        </div>
      ) : q.isError ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="msg msg--err">{(q.error as Error).message}</div>
        </div>
      ) : !d?.ok ? (
        <div className="card" style={{ padding: 14 }}>
          <div className="msg msg--err">{d?.message || "Falha ao consultar."}</div>
        </div>
      ) : (
        <div className="card" style={{ padding: 14 }}>
          <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(2, minmax(0, 200px))" }}>
            <div className="stat">
              <div className="stat__k">Clientes ativos</div>
              <div className="stat__v">{fmtInt(d.total)}</div>
            </div>
            <div className="stat">
              <div className="stat__k">Serviços ativos</div>
              <div className="stat__v">{fmtInt((d.services ?? []).reduce((acc, n) => acc + n, 0))}</div>
            </div>
          </div>
          <div className="table-wrap" style={{ marginTop: 12 }}>
            <table style={{ fontSize: 12 }}>
              <thead>
                <tr>
                  <th>Tempo de cliente</th>
                  <th>Clientes</th>
                  <th>Serviços</th>
                  <th>% dos ativos</th>
                  <th style={{ width: "35%" }} />
                </tr>
              </thead>
              <tbody>
                {d.buckets.map((b, bi) => {
                  const det = d.details?.find((x) => x.name === b.name);
                  return (
                  <tr
                    key={b.name}
                    style={det ? { cursor: "pointer" } : undefined}
                    title={det ? "Ver gráficos por localidade e por plano" : undefined}
                    onClick={det ? () => setBand({ name: b.name, from: det.from, to: det.to }) : undefined}
                  >
                    <td>{b.name}</td>
                    <td className="mono">{fmtInt(b.count)}</td>
                    <td className="mono">{d.services ? fmtInt(d.services[bi] ?? 0) : "—"}</td>
                    <td className="mono">{d.total > 0 ? fmtPct((b.count / d.total) * 100) : "—"}</td>
                    <td>
                      <div style={{ height: 8, borderRadius: 4, background: "color-mix(in srgb, var(--accent) 70%, transparent)", width: `${(b.count / max) * 100}%`, minWidth: b.count > 0 ? 2 : 0 }} />
                    </td>
                  </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
            Clique numa faixa para ver os gráficos por localidade e por plano. “Clientes” conta cada cliente uma vez; “Serviços” conta cada serviço ativo. A tela de filtro da HubSoft costuma listar serviços — compare com a coluna Serviços.
          </p>
          {d.truncated ? (
            <p style={{ fontSize: 11, color: "var(--warn)", margin: "8px 0 0" }}>Alguma faixa passou do teto de páginas — os números dela podem estar incompletos.</p>
          ) : null}
        </div>
      )}
      {band ? <HubsoftTenureBandModal band={band} onClose={() => setBand(null)} /> : null}
    </div>
  );
}

const SECTIONS: { id: Section; label: string; Icon: LucideIcon }[] = [
  { id: "clients", label: "Clientes", Icon: Users },
  { id: "services", label: "Serviços", Icon: Layers },
  { id: "blocked", label: "Bloqueios", Icon: Ban },
  { id: "preventive", label: "Desbloqueio preventivo", Icon: ShieldCheck },
  { id: "tenure", label: "Tempo de cliente", Icon: Hourglass },
  { id: "attendance", label: "Atendimentos", Icon: Headset },
  { id: "work_orders", label: "Ordens de serviço", Icon: Wrench },
  { id: "financial", label: "Financeiro", Icon: Wallet },
];

/**
 * Aba Relatório — lista de clientes/serviços filtrada (login/IPv4/MAC/status) + relatórios
 * agregados de atendimentos, ordens de serviço (por técnico) e financeiro (% recebido/aberto/
 * vencido) por período. Fala com /api/v1/integrations/hubsoft/hubsoft/report/* (ver
 * internal/api/handlers_hubsoft.go), que usam os endpoints "todos"/"listar" da HubSoft
 * (paginação real) em vez de amostras.
 */
export function HubsoftReportPage() {
  const [section, setSection] = useState<Section>("clients");

  return (
    <div className="integration-consult">
      <HubsoftHeader />
      <div className="hubsoft-tabs" role="tablist" aria-label="Relatórios da integração">
        {SECTIONS.map((s) => (
          <button
            key={s.id}
            type="button"
            role="tab"
            aria-selected={section === s.id}
            className={`hubsoft-tabs__btn${section === s.id ? " hubsoft-tabs__btn--active" : ""}`}
            onClick={() => setSection(s.id)}
          >
            <s.Icon size={16} aria-hidden />
            {s.label}
          </button>
        ))}
      </div>

      {section === "clients" && <ClientsReportSection />}
      {section === "services" && <ServicesReportSection />}
      {section === "blocked" && <BlockedReportSection />}
      {section === "preventive" && <HubsoftPreventiveSection />}
      {section === "tenure" && <TenureReportSection />}
      {section === "attendance" && <AttendanceReportSection />}
      {section === "work_orders" && <WorkOrderReportSection />}
      {section === "financial" && <FinancialReportSection />}
    </div>
  );
}
