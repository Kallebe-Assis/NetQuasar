import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { ActionMenu } from "../components/ActionMenu";
import type { AttendanceItem, ClientCard, ClientServiceSummary, WorkOrderItem } from "./types";
import {
  formatAttendanceStatus,
  formatIntegrationDateTime,
  formatIXCOnline,
  formatIXCContractStatus,
  formatWorkOrderStatus,
} from "./integrationDisplay";
import { TableCellExpandableText } from "./TableCellExpandableText";
import { SupportItemDetailModal, type SupportDetailTarget } from "./SupportItemDetailModal";

type SupportTab = "atendimentos" | "ordens" | "logins";

const DETAIL_FONT = "var(--integration-detail-font-size, 11px)";

function labelStatus(s?: string) {
  if (!s) return null;
  const low = s.toLowerCase();
  if (low.includes("habilit") || low === "ativo" || low.includes("online")) return "badge badge--ok";
  if (
    low.includes("suspen") ||
    low.includes("debito") ||
    low.includes("bloqueio") ||
    low.includes("atraso") ||
    low.includes("offline") ||
    low.includes("desativ")
  )
    return "badge badge--err";
  if (low.includes("cancel") || low.includes("sem status")) return "badge badge--off";
  return "badge";
}

function serviceStableKey(s: ClientServiceSummary, index: number): string {
  return [s.id, s.login, s.ipv4, s.contrato, s.mac, String(index)].filter(Boolean).join("|");
}

function resolveContractStatus(s: ClientServiceSummary): string {
  const code = (s.status_internet ?? "").trim();
  if (!code) {
    return "";
  }
  return formatIXCContractStatus(code, s.status_label) || code;
}

function ServiceSummaryCells({ s }: { s: ClientServiceSummary }) {
  const plan = s.plano_venda || s.name || s.login || "—";
  const online = formatIXCOnline(s.online, s.online_label);
  const contractStatus = resolveContractStatus(s);

  return (
    <>
      {s.contrato ? (
        <div className="integration-consult-card__service-cell">
          <span className="integration-consult-card__label">Contrato</span>
          <span className="mono integration-consult-card__value">{s.contrato}</span>
        </div>
      ) : null}
      <div className="integration-consult-card__service-cell">
        <span className="integration-consult-card__label">Plano</span>
        <span className="integration-consult-card__value">{plan}</span>
      </div>
      <div className="integration-consult-card__service-cell">
        <span className="integration-consult-card__label">Login</span>
        <span className="mono integration-consult-card__value">{s.login || "—"}</span>
      </div>
      <div className="integration-consult-card__service-cell">
        <span className="integration-consult-card__label">IPv4</span>
        <span className="mono integration-consult-card__value">{s.ipv4 || "—"}</span>
      </div>
      {s.mac ? (
        <div className="integration-consult-card__service-cell">
          <span className="integration-consult-card__label">MAC</span>
          <span className="mono integration-consult-card__value">{s.mac}</span>
        </div>
      ) : null}
      {online ? (
        <div className="integration-consult-card__service-cell integration-consult-card__service-cell--status">
          <span className="integration-consult-card__label">Online</span>
          <span className={labelStatus(online) ?? "badge"}>{online}</span>
        </div>
      ) : null}
      <div className="integration-consult-card__service-cell integration-consult-card__service-cell--status">
        <span className="integration-consult-card__label">Status do contrato</span>
        {contractStatus ? (
          <span className={labelStatus(contractStatus) ?? "badge"}>{contractStatus}</span>
        ) : (
          <span className="integration-consult-card__value">—</span>
        )}
      </div>
    </>
  );
}

function normalizeForSearch(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}

function collectSearchableValues(v: unknown, out: string[]): void {
  if (v === null || v === undefined) return;
  if (typeof v === "object") {
    if (Array.isArray(v)) {
      v.forEach((item) => collectSearchableValues(item, out));
      return;
    }
    Object.values(v as Record<string, unknown>).forEach((item) => collectSearchableValues(item, out));
    return;
  }
  const s = String(v).trim();
  if (s && s !== "[object Object]") out.push(s);
}

export function clientStableKey(c: ClientCard, index: number): string {
  return [c.id, c.code, c.document, c.name, String(index)].filter(Boolean).join("|");
}

export function clientSearchBlob(c: ClientCard): string {
  const parts = [
    c.name,
    c.trade_name,
    c.code,
    c.document,
    c.email,
    c.phone,
    c.ipv4,
    c.address,
    c.status,
    ...(c.services?.flatMap((s) => [
      s.name,
      s.login,
      s.status,
      s.status_label,
      s.status_internet,
      s.id,
      s.ipv4,
      s.mac,
      s.contrato,
      s.plano_venda,
      s.online,
      s.online_label,
    ]) ?? []),
    ...(c.details ? Object.entries(c.details).flatMap(([k, v]) => [k, v]) : []),
  ];
  if (c.raw) collectSearchableValues(c.raw, parts as string[]);
  return normalizeForSearch(parts.filter((p): p is string => Boolean(p)).join(" "));
}

export function filterClientCards(clients: ClientCard[], query: string): ClientCard[] {
  const q = normalizeForSearch(query);
  if (!q) return clients;
  const terms = q.split(/\s+/).filter(Boolean);
  if (terms.length === 0) return clients;
  return clients.filter((c) => {
    const blob = clientSearchBlob(c);
    return terms.every((t) => blob.includes(t));
  });
}

function FieldInline({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value?.trim()) return null;
  return (
    <span className="integration-consult-card__field">
      <span className="integration-consult-card__label">{label}: </span>
      <span className={mono ? "mono integration-consult-card__value" : "integration-consult-card__value"}>{value}</span>
    </span>
  );
}

function ClientCardSummary({
  c,
  selectedServiceIndex,
  onSelectService,
}: {
  c: ClientCard;
  selectedServiceIndex: number;
  onSelectService: (index: number) => void;
}) {
  const services = c.services ?? [];
  const safeIdx =
    services.length === 0 ? 0 : Math.min(Math.max(0, selectedServiceIndex), services.length - 1);
  const active = services[safeIdx];

  return (
    <>
      <div className="integration-consult-card__meta">
        <FieldInline label="ID" value={c.id ?? c.code} mono />
        {c.code && c.id && c.code !== c.id ? <FieldInline label="Código" value={c.code} mono /> : null}
        <FieldInline label="CPF/CNPJ" value={c.document} mono />
        <FieldInline label="Tel." value={c.phone} />
        <FieldInline label="E-mail" value={c.email} />
      </div>
      {c.address ? (
        <p className="integration-consult-card__line">
          <span className="integration-consult-card__label">End.: </span>
          {c.address}
        </p>
      ) : null}
      {services.length > 0 ? (
        <div className="integration-consult-card__services">
          {services.length > 1 ? (
            <div className="integration-consult-card__login-picker" role="tablist" aria-label="Logins do cliente">
              {services.map((s, si) => {
                const label = s.login?.trim() || s.name?.trim() || `Serviço ${si + 1}`;
                return (
                  <button
                    key={serviceStableKey(s, si)}
                    type="button"
                    role="tab"
                    aria-selected={si === safeIdx}
                    className={si === safeIdx ? "integration-consult-card__login-tab active" : "integration-consult-card__login-tab"}
                    onClick={() => onSelectService(si)}
                  >
                    {label}
                  </button>
                );
              })}
            </div>
          ) : null}
          {active ? (
            <div key={serviceStableKey(active, safeIdx)} className="integration-consult-card__service">
              <ServiceSummaryCells s={active} />
            </div>
          ) : null}
        </div>
      ) : null}
    </>
  );
}

function formatFieldLabel(key: string): string {
  return key
    .replace(/_/g, " ")
    .replace(/\b\w/g, (ch) => ch.toUpperCase());
}

function formatScalar(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "boolean") return v ? "Sim" : "Não";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v).trim();
}

function DetailScalar({ label, value }: { label: string; value: unknown }) {
  const text = formatScalar(value);
  if (!text) return null;
  return (
    <div className="integration-detail__row">
      <span className="integration-detail__label">{label}</span>
      <span className="integration-detail__value">{text}</span>
    </div>
  );
}

function DetailObjectBlock({ title, data }: { title: string; data: Record<string, unknown> }) {
  const rows = Object.entries(data).filter(([, v]) => formatScalar(v) !== "");
  if (rows.length === 0) return null;
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">{title}</h4>
      <div className="integration-detail__rows">
        {rows.map(([k, v]) => {
          if (v !== null && typeof v === "object") return null;
          return <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />;
        })}
      </div>
    </section>
  );
}

function DetailArrayBlock({ title, items }: { title: string; items: unknown[] }) {
  if (items.length === 0) return null;
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">
        {title} <span className="integration-detail__count">({items.length})</span>
      </h4>
      <div className="integration-detail__array">
        {items.map((item, i) => {
          if (item && typeof item === "object" && !Array.isArray(item)) {
            const obj = item as Record<string, unknown>;
            const scalars = Object.entries(obj).filter(([, v]) => v === null || typeof v !== "object");
            if (scalars.length === 0) {
              return (
                <pre key={i} className="integration-detail__json mono">
                  {JSON.stringify(obj, null, 2)}
                </pre>
              );
            }
            return (
              <div key={i} className="integration-detail__array-item">
                {scalars.map(([k, v]) => (
                  <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
                ))}
              </div>
            );
          }
          const text = formatScalar(item);
          if (!text) return null;
          return (
            <div key={i} className="integration-detail__array-item">
              <span className="integration-detail__value">{text}</span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function ClientDetailBody({ raw }: { raw?: Record<string, unknown> }) {
  if (!raw || Object.keys(raw).length === 0) {
    return <p className="integration-detail__empty">Sem dados detalhados disponíveis.</p>;
  }

  const scalarRows: [string, unknown][] = [];
  const objectSections: { key: string; data: Record<string, unknown> }[] = [];
  const arraySections: { key: string; items: unknown[] }[] = [];

  for (const [key, value] of Object.entries(raw)) {
    if (value === null || value === undefined) continue;
    if (Array.isArray(value)) {
      arraySections.push({ key, items: value });
    } else if (typeof value === "object") {
      objectSections.push({ key, data: value as Record<string, unknown> });
    } else {
      scalarRows.push([key, value]);
    }
  }

  const priority = ["nome_razaosocial", "nome", "codigo_cliente", "cpf_cnpj", "email_principal", "telefone", "status_cadastro"];
  scalarRows.sort(([a], [b]) => {
    const ia = priority.indexOf(a);
    const ib = priority.indexOf(b);
    if (ia >= 0 && ib >= 0) return ia - ib;
    if (ia >= 0) return -1;
    if (ib >= 0) return 1;
    return a.localeCompare(b);
  });

  return (
    <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
      {scalarRows.length > 0 ? (
        <section className="integration-detail__section">
          <h4 className="integration-detail__section-title">Geral</h4>
          <div className="integration-detail__rows">
            {scalarRows.map(([k, v]) => (
              <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
            ))}
          </div>
        </section>
      ) : null}
      {objectSections.map(({ key, data }) => (
        <DetailObjectBlock key={key} title={formatFieldLabel(key)} data={data} />
      ))}
      {arraySections.map(({ key, items }) => (
        <DetailArrayBlock key={key} title={formatFieldLabel(key)} items={items} />
      ))}
    </div>
  );
}

function ClientDetailModal({
  client,
  loading,
  onClose,
}: {
  client: ClientCard;
  loading?: boolean;
  onClose: () => void;
}) {
  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal integration-detail-modal"
        role="dialog"
        aria-labelledby="client-detail-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="integration-detail-modal__head">
          <div style={{ minWidth: 0 }}>
            <h3 id="client-detail-title" className="integration-detail-modal__title">
              {client.name || "Cliente"}
            </h3>
            {client.trade_name ? <p className="integration-detail-modal__subtitle">{client.trade_name}</p> : null}
          </div>
          <button type="button" className="btn" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        {client.status ? (
          <div style={{ marginBottom: 8 }}>
            <span className={labelStatus(client.status) ?? "badge"}>{client.status}</span>
          </div>
        ) : null}
        {loading ? (
          <p className="integration-detail__empty">A carregar detalhes…</p>
        ) : (
          <ClientDetailBody raw={client.raw} />
        )}
      </div>
    </div>,
    document.body,
  );
}

function SupportModal({
  client,
  tab,
  onTabChange,
  attendance,
  workOrders,
  loadingAttendance,
  loadingWorkOrders,
  attendanceEnabled,
  workOrderEnabled,
  loginEnabled,
  logins,
  loadingLogins,
  onClose,
}: {
  client: ClientCard;
  tab: SupportTab;
  onTabChange: (t: SupportTab) => void;
  attendance: { ok: boolean; message?: string; items: AttendanceItem[] };
  workOrders: { ok: boolean; message?: string; items: WorkOrderItem[] };
  logins: { ok: boolean; message?: string; items: ClientServiceSummary[] };
  loadingAttendance?: boolean;
  loadingWorkOrders?: boolean;
  loadingLogins?: boolean;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
  loginEnabled?: boolean;
  onClose: () => void;
}) {
  const showAttTab = attendanceEnabled !== false;
  const showWoTab = workOrderEnabled !== false;
  const showLoginTab = loginEnabled !== false;
  const [detailTarget, setDetailTarget] = useState<SupportDetailTarget | null>(null);

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal integration-support-modal"
        role="dialog"
        aria-labelledby="client-support-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="integration-support-modal__head">
          <div className="integration-support-modal__head-text">
            <h3 id="client-support-title" className="integration-support-modal__title">
              Atendimentos e ordens de serviço
            </h3>
            <p className="integration-support-modal__client-name">{client.name || "Cliente"}</p>
            {client.id || client.code ? (
              <p className="integration-support-modal__subtitle mono">
                ID {client.id || client.code}
                {client.code && client.id && client.code !== client.id ? ` · Código ${client.code}` : ""}
              </p>
            ) : null}
          </div>
          <button type="button" className="btn" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="tabs integration-support-modal__tabs">
          {showAttTab ? (
            <button type="button" className={tab === "atendimentos" ? "active" : ""} onClick={() => onTabChange("atendimentos")}>
              Atendimentos
            </button>
          ) : null}
          {showWoTab ? (
            <button type="button" className={tab === "ordens" ? "active" : ""} onClick={() => onTabChange("ordens")}>
              Ordens de serviço
            </button>
          ) : null}
          {showLoginTab ? (
            <button type="button" className={tab === "logins" ? "active" : ""} onClick={() => onTabChange("logins")}>
              Logins
            </button>
          ) : null}
        </div>

        <div className="integration-support-modal__body">
          {tab === "atendimentos" && showAttTab ? (
            loadingAttendance ? (
              <p className="integration-detail__empty">A carregar atendimentos…</p>
            ) : !attendance.ok && attendance.message ? (
              <div className="msg msg--err">{attendance.message}</div>
            ) : attendance.items.length === 0 ? (
              <div className="msg">{attendance.message || "Nenhum atendimento encontrado."}</div>
            ) : (
              <div className="table-wrap integration-support-table">
                <table className="integration-support-table__grid integration-support-table__grid--att">
                  <thead>
                    <tr>
                      <th>Protocolo</th>
                      <th>Estado</th>
                      <th>Assunto</th>
                      <th>Descrição</th>
                      <th>Abertura</th>
                      <th>Fechamento</th>
                      <th className="integration-support-table__col-actions" />
                    </tr>
                  </thead>
                  <tbody>
                    {attendance.items.map((a, i) => (
                      <tr key={a.id ?? a.protocol ?? i}>
                        <td className="mono integration-support-table__cell">{a.protocol || "—"}</td>
                        <td className="integration-support-table__cell">
                          {formatAttendanceStatus(a) ? (
                            <span className={labelStatus(formatAttendanceStatus(a)) ?? "badge"}>
                              {formatAttendanceStatus(a)}
                            </span>
                          ) : (
                            "—"
                          )}
                          {a.pending === true ? <span className="badge integration-support-table__chip">Pendente</span> : null}
                        </td>
                        <td className="integration-support-table__cell">{a.subject || "—"}</td>
                        <td className="integration-support-table__cell integration-support-table__cell--text">
                          <TableCellExpandableText text={a.description} />
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--date">
                          {formatIntegrationDateTime(a.opened_at) || "—"}
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--date">
                          {formatIntegrationDateTime(a.closed_at) || "—"}
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--actions">
                          <button
                            type="button"
                            className="btn btn--sm integration-support-table__more-btn"
                            onClick={() => setDetailTarget({ kind: "attendance", item: a })}
                          >
                            Ver mais
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )
          ) : null}

          {tab === "ordens" && showWoTab ? (
            loadingWorkOrders ? (
              <p className="integration-detail__empty">A carregar ordens de serviço…</p>
            ) : !workOrders.ok && workOrders.message ? (
              <div className="msg msg--err">{workOrders.message}</div>
            ) : workOrders.items.length === 0 ? (
              <div className="msg">{workOrders.message || "Nenhuma ordem de serviço encontrada."}</div>
            ) : (
              <div className="table-wrap integration-support-table">
                <table className="integration-support-table__grid integration-support-table__grid--os">
                  <thead>
                    <tr>
                      <th>N.º O.S.</th>
                      <th>Estado O.S.</th>
                      <th>Plano / serviço</th>
                      <th>Atendimento</th>
                      <th>Cadastro</th>
                      <th>Agendamento</th>
                      <th className="integration-support-table__col-actions" />
                    </tr>
                  </thead>
                  <tbody>
                    {workOrders.items.map((o, i) => (
                      <tr key={o.id ?? o.number ?? i}>
                        <td className="mono integration-support-table__cell">{o.number || "—"}</td>
                        <td className="integration-support-table__cell">
                          <span className={labelStatus(formatWorkOrderStatus(o)) ?? "badge"}>
                            {formatWorkOrderStatus(o) || "—"}
                          </span>
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--plan">
                          <div className="integration-os-plan__title">
                            <TableCellExpandableText text={o.plan_name || o.description} maxLength={60} />
                          </div>
                          {o.service_status ? (
                            <div className="integration-os-plan__meta">Estado do serviço: {o.service_status}</div>
                          ) : null}
                          {o.value ? <div className="integration-os-plan__meta">Valor: {o.value}</div> : null}
                        </td>
                        <td className="mono integration-support-table__cell">{o.attendance_protocol || "—"}</td>
                        <td className="integration-support-table__cell integration-support-table__cell--date">
                          {formatIntegrationDateTime(o.created_at) || "—"}
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--date">
                          {formatIntegrationDateTime(o.scheduled_at) || "—"}
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--actions">
                          <button
                            type="button"
                            className="btn btn--sm integration-support-table__more-btn"
                            onClick={() => setDetailTarget({ kind: "work_order", item: o })}
                          >
                            Ver mais
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )
          ) : null}

          {tab === "logins" && showLoginTab ? (
            loadingLogins ? (
              <p className="integration-detail__empty">A carregar logins…</p>
            ) : !logins.ok && logins.message ? (
              <div className="msg msg--err">{logins.message}</div>
            ) : logins.items.length === 0 ? (
              <div className="msg">{logins.message || "Nenhum login encontrado."}</div>
            ) : (
              <div className="table-wrap integration-support-table">
                <table className="integration-support-table__grid integration-support-table__grid--login">
                  <thead>
                    <tr>
                      <th>Login</th>
                      <th>Contrato</th>
                      <th>Plano</th>
                      <th>Online</th>
                      <th>Status contrato</th>
                      <th>MAC</th>
                      <th>IPv4</th>
                    </tr>
                  </thead>
                  <tbody>
                    {logins.items.map((s, i) => {
                      const online = formatIXCOnline(s.online, s.online_label);
                      const statusInternet = formatIXCContractStatus(s.status_internet, s.status_label);
                      return (
                        <tr key={s.id ?? s.login ?? i}>
                          <td className="mono integration-support-table__cell">{s.login || "—"}</td>
                          <td className="mono integration-support-table__cell">{s.contrato || "—"}</td>
                          <td className="integration-support-table__cell">
                            {s.plano_venda || s.name || "—"}
                          </td>
                          <td className="integration-support-table__cell">
                            {online ? (
                              <span className={labelStatus(online) ?? "badge"}>{online}</span>
                            ) : (
                              "—"
                            )}
                          </td>
                          <td className="integration-support-table__cell">
                            {statusInternet ? (
                              <span className={labelStatus(statusInternet) ?? "badge"}>{statusInternet}</span>
                            ) : (
                              "—"
                            )}
                          </td>
                          <td className="mono integration-support-table__cell">{s.mac || "—"}</td>
                          <td className="mono integration-support-table__cell">{s.ipv4 || "—"}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )
          ) : null}
        </div>
        {detailTarget ? <SupportItemDetailModal target={detailTarget} onClose={() => setDetailTarget(null)} /> : null}
      </div>
    </div>,
    document.body,
  );
}

export function HubsoftClientResults({
  clients,
  message,
  ok,
  localFilter,
  onFetchDetail,
  onFetchAttendance,
  onFetchWorkOrders,
  onFetchLogins,
  attendanceEnabled,
  workOrderEnabled,
  loginEnabled,
}: {
  clients: ClientCard[];
  message?: string;
  ok: boolean;
  localFilter: string;
  onFetchDetail?: (client: ClientCard) => Promise<ClientCard>;
  onFetchAttendance?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: AttendanceItem[] }>;
  onFetchWorkOrders?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: WorkOrderItem[] }>;
  onFetchLogins?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: ClientServiceSummary[] }>;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
  loginEnabled?: boolean;
}) {
  const supportEnabled = !!(attendanceEnabled || workOrderEnabled || loginEnabled);

  const [detailClient, setDetailClient] = useState<ClientCard | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [supportClient, setSupportClient] = useState<ClientCard | null>(null);
  const [supportTab, setSupportTab] = useState<SupportTab>("atendimentos");
  const [attendanceItems, setAttendanceItems] = useState<AttendanceItem[]>([]);
  const [workOrderItems, setWorkOrderItems] = useState<WorkOrderItem[]>([]);
  const [attendanceLoading, setAttendanceLoading] = useState(false);
  const [workOrderLoading, setWorkOrderLoading] = useState(false);
  const [attendanceOk, setAttendanceOk] = useState(true);
  const [workOrderOk, setWorkOrderOk] = useState(true);
  const [attendanceMessage, setAttendanceMessage] = useState<string | undefined>();
  const [workOrderMessage, setWorkOrderMessage] = useState<string | undefined>();
  const [loginItems, setLoginItems] = useState<ClientServiceSummary[]>([]);
  const [loginLoading, setLoginLoading] = useState(false);
  const [loginOk, setLoginOk] = useState(true);
  const [loginMessage, setLoginMessage] = useState<string | undefined>();
  const [selectedServiceByClient, setSelectedServiceByClient] = useState<Record<string, number>>({});

  const filtered = useMemo(() => filterClientCards(clients, localFilter), [clients, localFilter]);

  const selectServiceForClient = (clientKey: string, index: number) => {
    setSelectedServiceByClient((prev) => ({ ...prev, [clientKey]: index }));
  };

  const openDetail = async (c: ClientCard) => {
    setDetailClient(c);
    if (!onFetchDetail) return;
    setDetailLoading(true);
    try {
      const full = await onFetchDetail(c);
      setDetailClient(full);
    } finally {
      setDetailLoading(false);
    }
  };

  const openSupport = async (c: ClientCard) => {
    if (!onFetchAttendance && !onFetchWorkOrders && !onFetchLogins) return;
    setSupportClient(c);
    setSupportTab(
      attendanceEnabled ? "atendimentos" : workOrderEnabled ? "ordens" : "logins",
    );
    setAttendanceItems([]);
    setWorkOrderItems([]);
    setLoginItems([]);
    setAttendanceMessage(undefined);
    setWorkOrderMessage(undefined);
    setLoginMessage(undefined);
    setAttendanceOk(true);
    setWorkOrderOk(true);
    setLoginOk(true);

    let client = c;
    if (onFetchDetail) {
      try {
        client = await onFetchDetail(c);
        setSupportClient(client);
      } catch {
        /* usa cartão da listagem */
      }
    }

    const jobs: Promise<void>[] = [];
    if (onFetchAttendance && attendanceEnabled) {
      setAttendanceLoading(true);
      jobs.push(
        onFetchAttendance(client)
          .then((r) => {
            setAttendanceOk(!!r.ok);
            setAttendanceMessage(r.message);
            setAttendanceItems(r.items ?? []);
          })
          .catch((e) => {
            setAttendanceOk(false);
            setAttendanceMessage(e instanceof Error ? e.message : String(e));
            setAttendanceItems([]);
          })
          .finally(() => setAttendanceLoading(false)),
      );
    }
    if (onFetchWorkOrders && workOrderEnabled) {
      setWorkOrderLoading(true);
      jobs.push(
        onFetchWorkOrders(client)
          .then((r) => {
            setWorkOrderOk(!!r.ok);
            setWorkOrderMessage(r.message);
            setWorkOrderItems(r.items ?? []);
          })
          .catch((e) => {
            setWorkOrderOk(false);
            setWorkOrderMessage(e instanceof Error ? e.message : String(e));
            setWorkOrderItems([]);
          })
          .finally(() => setWorkOrderLoading(false)),
      );
    }
    if (onFetchLogins && loginEnabled) {
      setLoginLoading(true);
      jobs.push(
        onFetchLogins(client)
          .then((r) => {
            setLoginOk(!!r.ok);
            setLoginMessage(r.message);
            setLoginItems(r.items ?? []);
          })
          .catch((e) => {
            setLoginOk(false);
            setLoginMessage(e instanceof Error ? e.message : String(e));
            setLoginItems([]);
          })
          .finally(() => setLoginLoading(false)),
      );
    }
    await Promise.all(jobs);
  };

  if (!ok && message) {
    return <div className="msg msg--err">{message}</div>;
  }
  if (clients.length === 0) {
    return <div className="msg">{message || "Nenhum cliente encontrado."}</div>;
  }
  if (filtered.length === 0) {
    return (
      <div className="msg">
        Nenhum resultado corresponde ao filtro &quot;{localFilter.trim()}&quot;.
      </div>
    );
  }

  return (
    <>
      {localFilter.trim() && filtered.length < clients.length ? (
        <p className="integration-consult-results__filter-hint">
          A mostrar {filtered.length} de {clients.length} resultado(s).
        </p>
      ) : null}
      <div className="integration-consult-cards">
        {filtered.map((c, idx) => {
          const cardKey = clientStableKey(c, idx);
          const serviceIdx = selectedServiceByClient[cardKey] ?? 0;
          return (
          <article key={cardKey} className="card integration-consult-card">
            <div className="integration-consult-card__head">
              <div className="integration-consult-card__title-wrap">
                <div className="integration-consult-card__title">{c.name || "—"}</div>
                {c.trade_name ? <div className="integration-consult-card__subtitle">{c.trade_name}</div> : null}
              </div>
              <div className="integration-consult-card__actions">
                {c.status ? <span className={labelStatus(c.status) ?? "badge"}>{c.status}</span> : null}
                <ActionMenu
                  title="Opções do cliente"
                  align="end"
                  items={[
                    {
                      id: "detail",
                      label: "Ver dados completos",
                      onClick: () => void openDetail(c),
                    },
                    {
                      id: "support",
                      label: "Atendimentos e ordens de serviço",
                      disabled:
                        !supportEnabled ||
                        (!onFetchAttendance && !onFetchWorkOrders && !onFetchLogins),
                      onClick: () => void openSupport(c),
                    },
                  ]}
                />
              </div>
            </div>
            <ClientCardSummary
              c={c}
              selectedServiceIndex={serviceIdx}
              onSelectService={(i) => selectServiceForClient(cardKey, i)}
            />
          </article>
          );
        })}
      </div>
      {detailClient ? (
        <ClientDetailModal client={detailClient} loading={detailLoading} onClose={() => setDetailClient(null)} />
      ) : null}
      {supportClient ? (
        <SupportModal
          client={supportClient}
          tab={supportTab}
          onTabChange={setSupportTab}
          attendance={{ ok: attendanceOk, message: attendanceMessage, items: attendanceItems }}
          workOrders={{ ok: workOrderOk, message: workOrderMessage, items: workOrderItems }}
          logins={{ ok: loginOk, message: loginMessage, items: loginItems }}
          loadingAttendance={attendanceLoading}
          loadingWorkOrders={workOrderLoading}
          loadingLogins={loginLoading}
          attendanceEnabled={attendanceEnabled}
          workOrderEnabled={workOrderEnabled}
          loginEnabled={loginEnabled}
          onClose={() => setSupportClient(null)}
        />
      ) : null}
    </>
  );
}
