import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { Send } from "lucide-react";
import { HubsoftHeader } from "./HubsoftHeader";
import { InfoHint } from "../../components/InfoHint";
import { ClientDetailModal } from "../../integrations/HubsoftClientResults";
import type {
  ClientAttendanceResponse,
  ClientCard,
  ClientFinancialResponse,
  ClientSearchResponse,
  ClientServiceSummary,
  ClientWorkOrderResponse,
  HubsoftAttendanceReportResponse,
  HubsoftFinancialReportResponse,
  HubsoftReportClientsResponse,
  HubsoftReportServiceRow,
  HubsoftWorkOrderReportResponse,
} from "../../integrations/types";
import { apiFetch } from "../../lib/api";

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

type Section = "clients" | "attendance" | "work_orders" | "financial";

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

/** Seletor de período reutilizado pelas 3 abas de relatório (Atendimentos/O.S./Financeiro). */
function PeriodPicker({
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

function StatusBreakdownTable({ items }: { items: { name: string; count: number }[] }) {
  if (items.length === 0) return null;
  return (
    <div className="table-wrap" style={{ marginTop: 10 }}>
      <table style={{ fontSize: 12 }}>
        <thead>
          <tr>
            <th>Status</th>
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
  const [appliedParams, setAppliedParams] = useState<string | null>(null);
  const [detailClient, setDetailClient] = useState<ClientCard | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

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

  const reportQ = useQuery({
    queryKey: ["hubsoft-report-clients", appliedParams],
    enabled: appliedParams !== null,
    queryFn: () => apiFetch<HubsoftReportClientsResponse>(`/api/v1/integrations/${SLUG}/hubsoft/report/clients?${appliedParams}`),
  });

  function runFilter() {
    setAppliedParams(buildParams());
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
          <button type="button" className="btn btn--primary" disabled={reportQ.isFetching} onClick={runFilter}>
            {reportQ.isFetching ? "A filtrar…" : "Filtrar"}
          </button>
        </div>
      </div>

      {appliedParams === null ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>Ajuste os filtros e clique em Filtrar.</p>
      ) : reportQ.isLoading ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p>
      ) : reportQ.isError ? (
        <div className="msg msg--err">{(reportQ.error as Error).message}</div>
      ) : !reportQ.data?.ok ? (
        <div className="msg msg--err">{reportQ.data?.message || "Falha ao consultar."}</div>
      ) : rows.length === 0 ? (
        <div className="msg msg--warn">{reportQ.data?.message || "Nenhum resultado para esses filtros."}</div>
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
          prefetchExtras
        />
      ) : null}
    </div>
  );
}

function AttendanceReportSection() {
  const [from, setFrom] = useState(todayISO(-30));
  const [to, setTo] = useState(todayISO());
  const qc = useQueryClient();

  const q = useQuery({
    queryKey: ["hubsoft-report-attendance", from, to],
    queryFn: () =>
      apiFetch<HubsoftAttendanceReportResponse>(
        `/api/v1/integrations/${SLUG}/hubsoft/report/attendance?data_inicio=${from}&data_fim=${to}`,
      ),
  });
  const d = q.data;

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
        <button
          type="button"
          className="btn btn--sm"
          disabled={q.isFetching}
          onClick={() => void qc.invalidateQueries({ queryKey: ["hubsoft-report-attendance", from, to] })}
        >
          {q.isFetching ? "A atualizar…" : "Atualizar"}
        </button>
        <TelegramSendButton path={`/api/v1/integrations/${SLUG}/hubsoft/report/attendance/telegram?data_inicio=${from}&data_fim=${to}`} />
      </div>
      {q.isLoading ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p>
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
  const qc = useQueryClient();

  const q = useQuery({
    queryKey: ["hubsoft-report-work-orders", from, to],
    queryFn: () =>
      apiFetch<HubsoftWorkOrderReportResponse>(
        `/api/v1/integrations/${SLUG}/hubsoft/report/work-orders?data_inicio=${from}&data_fim=${to}`,
      ),
  });
  const d = q.data;

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
        <button
          type="button"
          className="btn btn--sm"
          disabled={q.isFetching}
          onClick={() => void qc.invalidateQueries({ queryKey: ["hubsoft-report-work-orders", from, to] })}
        >
          {q.isFetching ? "A atualizar…" : "Atualizar"}
        </button>
        <TelegramSendButton path={`/api/v1/integrations/${SLUG}/hubsoft/report/work-orders/telegram?data_inicio=${from}&data_fim=${to}`} />
      </div>
      {q.isLoading ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p>
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

/** Modo "Últimos X meses": 1 pedido por mês (cada um já paginado/limitado ao próprio mês pela
 * API, ver report/financial) em paralelo, agregados aqui em total + média. */
function FinancialLastNMonths({ months }: { months: number }) {
  const ranges = useMemo(() => lastNMonths(months), [months]);
  const results = useQueries({
    queries: ranges.map((r) => ({
      queryKey: ["hubsoft-report-financial-month", r.from, r.to],
      queryFn: () =>
        apiFetch<HubsoftFinancialReportResponse>(
          `/api/v1/integrations/${SLUG}/hubsoft/report/financial?data_inicio=${r.from}&data_fim=${r.to}`,
        ),
    })),
  });

  const loading = results.some((r) => r.isLoading);
  const errored = results.find((r) => r.isError);
  const rows = results.map((r, i) => ({ label: ranges[i].label, d: r.data }));
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

  if (loading) return <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar {months} mês(es)…</p>;
  if (errored) return <div className="msg msg--err">{(errored.error as Error).message}</div>;

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
  const qc = useQueryClient();

  const monthPeriod = useMemo(() => monthRange(month), [month]);
  const activeFrom = mode === "month" ? monthPeriod.from : from;
  const activeTo = mode === "month" ? monthPeriod.to : to;

  const q = useQuery({
    queryKey: ["hubsoft-report-financial", activeFrom, activeTo],
    enabled: mode !== "last_n_months",
    queryFn: () =>
      apiFetch<HubsoftFinancialReportResponse>(
        `/api/v1/integrations/${SLUG}/hubsoft/report/financial?data_inicio=${activeFrom}&data_fim=${activeTo}`,
      ),
  });
  const d = q.data;

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

      {mode !== "last_n_months" && (
        <div className="row" style={{ marginBottom: 8, gap: 8, alignItems: "center" }}>
          <button
            type="button"
            className="btn btn--sm"
            disabled={q.isFetching}
            onClick={() => void qc.invalidateQueries({ queryKey: ["hubsoft-report-financial", activeFrom, activeTo] })}
          >
            {q.isFetching ? "A atualizar…" : "Atualizar"}
          </button>
          <TelegramSendButton path={`/api/v1/integrations/${SLUG}/hubsoft/report/financial/telegram?data_inicio=${activeFrom}&data_fim=${activeTo}`} />
        </div>
      )}

      {mode === "last_n_months" ? (
        <FinancialLastNMonths months={nMonths} />
      ) : q.isLoading ? (
        <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p>
      ) : q.isError ? (
        <div className="msg msg--err">{(q.error as Error).message}</div>
      ) : !d?.ok ? (
        <div className="msg msg--err">{d?.message || "Falha ao consultar."}</div>
      ) : (
        <FinancialKPIs d={d} />
      )}
    </div>
  );
}

const SECTIONS: { id: Section; label: string }[] = [
  { id: "clients", label: "Clientes" },
  { id: "attendance", label: "Atendimentos" },
  { id: "work_orders", label: "Ordens de serviço" },
  { id: "financial", label: "Financeiro" },
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
      <div className="row" style={{ gap: 6, marginBottom: 12 }}>
        {SECTIONS.map((s) => (
          <button
            key={s.id}
            type="button"
            className={`btn btn--sm${section === s.id ? " btn--primary" : ""}`}
            onClick={() => setSection(s.id)}
          >
            {s.label}
          </button>
        ))}
      </div>

      {section === "clients" && <ClientsReportSection />}
      {section === "attendance" && <AttendanceReportSection />}
      {section === "work_orders" && <WorkOrderReportSection />}
      {section === "financial" && <FinancialReportSection />}
    </div>
  );
}
