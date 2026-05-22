import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { ActionMenu } from "../components/ActionMenu";
import type { AttendanceItem, ClientCard, WorkOrderItem } from "./types";
import { SupportItemDetailModal, type SupportDetailTarget } from "./SupportItemDetailModal";

type SupportTab = "atendimentos" | "ordens";

const DETAIL_FONT = "var(--integration-detail-font-size, 11px)";

function labelStatus(s?: string) {
  if (!s) return null;
  const low = s.toLowerCase();
  if (low.includes("habilit") || low === "ativo") return "badge badge--ok";
  if (low.includes("suspen") || low.includes("debito")) return "badge badge--err";
  if (low.includes("cancel")) return "badge badge--off";
  return "badge";
}

function formatServiceStatus(status?: string): string {
  if (!status?.trim()) return "";
  const key = status.trim().toLowerCase().replace(/\s+/g, "_");
  const labels: Record<string, string> = {
    servico_habilitado: "Serviço habilitado",
    servico_desabilitado: "Serviço desabilitado",
    servico_suspenso: "Serviço suspenso",
    servico_cancelado: "Serviço cancelado",
  };
  if (labels[key]) return labels[key];
  if (key.includes("_")) {
    return key
      .split("_")
      .filter(Boolean)
      .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
      .join(" ");
  }
  return status.trim();
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
    ...(c.services?.flatMap((s) => [s.name, s.login, s.status, s.id, s.ipv4]) ?? []),
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

function ClientCardSummary({ c }: { c: ClientCard }) {
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
      {c.services && c.services.length > 0 ? (
        <div className="integration-consult-card__services">
          {c.services.map((s, si) => {
            const statusLabel = formatServiceStatus(s.status);
            return (
              <div key={s.id ?? `${si}-${s.login}-${s.ipv4}`} className="integration-consult-card__service">
                <div className="integration-consult-card__service-cell">
                  <span className="integration-consult-card__label">Plano</span>
                  <span className="integration-consult-card__value">{s.name || s.login || "—"}</span>
                </div>
                <div className="integration-consult-card__service-cell">
                  <span className="integration-consult-card__label">IPv4</span>
                  <span className="mono integration-consult-card__value">{s.ipv4 || "—"}</span>
                </div>
                <div className="integration-consult-card__service-cell">
                  <span className="integration-consult-card__label">Login</span>
                  <span className="mono integration-consult-card__value">{s.login || "—"}</span>
                </div>
                <div className="integration-consult-card__service-cell integration-consult-card__service-cell--status">
                  <span className="integration-consult-card__label">Status</span>
                  {statusLabel ? (
                    <span className={labelStatus(s.status) ?? "badge"}>{statusLabel}</span>
                  ) : (
                    <span className="integration-consult-card__value">—</span>
                  )}
                </div>
              </div>
            );
          })}
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
  onClose,
}: {
  client: ClientCard;
  tab: SupportTab;
  onTabChange: (t: SupportTab) => void;
  attendance: { ok: boolean; message?: string; items: AttendanceItem[] };
  workOrders: { ok: boolean; message?: string; items: WorkOrderItem[] };
  loadingAttendance?: boolean;
  loadingWorkOrders?: boolean;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
  onClose: () => void;
}) {
  const showAttTab = attendanceEnabled !== false;
  const showWoTab = workOrderEnabled !== false;
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
                          {a.status ? <span className={labelStatus(a.status) ?? "badge"}>{a.status}</span> : "—"}
                          {a.pending === true ? <span className="badge integration-support-table__chip">Pendente</span> : null}
                        </td>
                        <td className="integration-support-table__cell">{a.subject || "—"}</td>
                        <td className="integration-support-table__cell integration-support-table__cell--text">
                          {a.description || "—"}
                        </td>
                        <td className="mono integration-support-table__cell integration-support-table__cell--date">
                          {a.opened_at || "—"}
                        </td>
                        <td className="mono integration-support-table__cell integration-support-table__cell--date">
                          {a.closed_at || "—"}
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
                          <span className={labelStatus(o.status_label ?? o.status) ?? "badge"}>
                            {o.status_label || o.status || "—"}
                          </span>
                        </td>
                        <td className="integration-support-table__cell integration-support-table__cell--plan">
                          <div className="integration-os-plan__title">{o.plan_name || o.description || "—"}</div>
                          {o.service_status ? (
                            <div className="integration-os-plan__meta">Estado do serviço: {o.service_status}</div>
                          ) : null}
                          {o.value ? <div className="integration-os-plan__meta">Valor: {o.value}</div> : null}
                        </td>
                        <td className="mono integration-support-table__cell">{o.attendance_protocol || "—"}</td>
                        <td className="mono integration-support-table__cell integration-support-table__cell--date">
                          {o.created_at || "—"}
                        </td>
                        <td className="mono integration-support-table__cell integration-support-table__cell--date">
                          {o.scheduled_at || "—"}
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
  attendanceEnabled,
  workOrderEnabled,
}: {
  clients: ClientCard[];
  message?: string;
  ok: boolean;
  localFilter: string;
  onFetchDetail?: (client: ClientCard) => Promise<ClientCard>;
  onFetchAttendance?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: AttendanceItem[] }>;
  onFetchWorkOrders?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: WorkOrderItem[] }>;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
}) {
  const supportEnabled = !!(attendanceEnabled || workOrderEnabled);

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

  const filtered = useMemo(() => filterClientCards(clients, localFilter), [clients, localFilter]);

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
    if (!onFetchAttendance && !onFetchWorkOrders) return;
    setSupportClient(c);
    setSupportTab(attendanceEnabled ? "atendimentos" : "ordens");
    setAttendanceItems([]);
    setWorkOrderItems([]);
    setAttendanceMessage(undefined);
    setWorkOrderMessage(undefined);
    setAttendanceOk(true);
    setWorkOrderOk(true);

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
        {filtered.map((c, idx) => (
          <article key={c.id ?? c.code ?? c.document ?? idx} className="card integration-consult-card">
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
                      disabled: !supportEnabled || (!onFetchAttendance && !onFetchWorkOrders),
                      onClick: () => void openSupport(c),
                    },
                  ]}
                />
              </div>
            </div>
            <ClientCardSummary c={c} />
          </article>
        ))}
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
          loadingAttendance={attendanceLoading}
          loadingWorkOrders={workOrderLoading}
          attendanceEnabled={attendanceEnabled}
          workOrderEnabled={workOrderEnabled}
          onClose={() => setSupportClient(null)}
        />
      ) : null}
    </>
  );
}
