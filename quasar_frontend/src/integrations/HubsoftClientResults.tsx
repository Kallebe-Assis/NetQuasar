import { useEffect, useMemo, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { ExternalLink, X } from "lucide-react";
import { ActionMenu } from "../components/ActionMenu";
import type {
  AttendanceItem,
  ClientCard,
  ClientServiceSummary,
  FinancialSummary,
  InvoiceItem,
  WorkOrderItem,
} from "./types";
import {
  formatAttendanceStatus,
  formatIntegrationDateTime,
  formatIXCOnline,
  formatIXCContractStatus,
  formatWorkOrderStatus,
} from "./integrationDisplay";
import { TableCellExpandableText } from "./TableCellExpandableText";
import { SupportItemDetailModal, type SupportDetailTarget } from "./SupportItemDetailModal";

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
  if (code) {
    return formatIXCContractStatus(code, s.status_label) || code;
  }
  // Hubsoft não usa status_internet (campo do IXC) — o próprio status do serviço já indica
  // habilitado/cancelado/suspenso etc. (ex.: "Serviço Habilitado").
  return (s.status ?? "").trim();
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
      {s.connected ? (
        <div className="integration-consult-card__service-cell integration-consult-card__service-cell--status">
          <span className="integration-consult-card__label">Conexão</span>
          <span className={s.connected === "true" ? "badge badge--ok" : "badge badge--err"}>
            {s.connected === "true" ? "Conectado" : "Desconectado"}
          </span>
        </div>
      ) : null}
      {s.status_text ? (
        <div className="integration-consult-card__service-cell">
          <span className="integration-consult-card__label">Situação da conexão</span>
          <span className="integration-consult-card__value">{s.status_text}</span>
        </div>
      ) : null}
      {s.last_disconnected_at ? (
        <div className="integration-consult-card__service-cell">
          <span className="integration-consult-card__label">Última desconexão</span>
          <span className="mono integration-consult-card__value">{s.last_disconnected_at}</span>
        </div>
      ) : null}
      {s.last_ipv4 ? (
        <div className="integration-consult-card__service-cell">
          <span className="integration-consult-card__label">Último IPv4</span>
          <span className="mono integration-consult-card__value">{s.last_ipv4}</span>
        </div>
      ) : null}
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

function digitsOnly(s?: string): string {
  return (s ?? "").replace(/\D+/g, "");
}

export function clientSearchBlob(c: ClientCard): string {
  const parts = [
    c.name,
    c.trade_name,
    c.code,
    c.document,
    digitsOnly(c.document),
    c.email,
    c.phone,
    digitsOnly(c.phone),
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

function isHttpUrl(v: string): boolean {
  return /^https?:\/\//i.test(v.trim());
}

function DetailScalar({ label, value }: { label: string; value: unknown }) {
  const text = formatScalar(value);
  if (!text) return null;
  if (isHttpUrl(text)) {
    return (
      <div className="integration-detail__row">
        <span className="integration-detail__label">{label}</span>
        <a className="btn btn--sm integration-detail__link-btn" href={text} target="_blank" rel="noreferrer">
          <ExternalLink size={12} /> Abrir
        </a>
      </div>
    );
  }
  // Textos longos (ex.: anotações/observações) ficavam espremidos numa coluna estreita da
  // grelha e quebravam linha de forma feia — aqui ocupam a largura toda e ganham quebra de
  // linha normal (não mono, para não forçar largura de fonte fixa em texto livre).
  const multiline = text.length > 100 || text.includes("\n");
  if (multiline) {
    return (
      <div className="integration-detail__row integration-detail__row--stack">
        <span className="integration-detail__label">{label}</span>
        <span className="integration-detail__value integration-detail__value--block">{text}</span>
      </div>
    );
  }
  return (
    <div className="integration-detail__row">
      <span className="integration-detail__label">{label}</span>
      <span className="integration-detail__value">{text}</span>
    </div>
  );
}

// Anexos (fotos/documentos) trazem um campo de link (URL directa ao ficheiro) — mostrar a
// URL completa como texto ocupava várias linhas e ainda a fatiava (id, extensão, etc.).
// Deteta o formato "anexo" (link/url + nome/descrição) e mostra uma linha compacta com
// nome + botão "Abrir" em vez da grelha de campos completa.
function findAttachmentUrl(obj: Record<string, unknown>): string | null {
  for (const key of ["link", "url", "arquivo", "anexo_url"]) {
    const v = obj[key];
    if (typeof v === "string" && isHttpUrl(v)) return v;
  }
  return null;
}

function isAttachmentLike(obj: Record<string, unknown>): boolean {
  return findAttachmentUrl(obj) !== null && (typeof obj.nome === "string" || typeof obj.descricao === "string");
}

function AttachmentItem({ obj }: { obj: Record<string, unknown> }) {
  const url = findAttachmentUrl(obj);
  if (!url) return null;
  const name = (obj.nome as string) || (obj.descricao as string) || "Anexo";
  const ext = typeof obj.extensao === "string" ? obj.extensao.toUpperCase() : "";
  return (
    <div className="integration-detail__attachment">
      <span className="integration-detail__attachment-name" title={name}>
        {name}
        {ext ? <span className="integration-detail__attachment-ext">{ext}</span> : null}
      </span>
      <a className="btn btn--sm integration-detail__attachment-btn" href={url} target="_blank" rel="noreferrer">
        <ExternalLink size={12} /> Abrir
      </a>
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
            if (isAttachmentLike(obj)) {
              return <AttachmentItem key={i} obj={obj} />;
            }
            const scalars = Object.entries(obj).filter(([, v]) => v === null || typeof v !== "object");
            // Sub-objectos (ex.: última conexão, equipamento, endereço) — antes eram descartados
            // silenciosamente aqui; agora viram sub-secções dentro do próprio item.
            const subObjects = Object.entries(obj).filter(
              (entry): entry is [string, Record<string, unknown>] =>
                !!entry[1] && typeof entry[1] === "object" && !Array.isArray(entry[1]),
            );
            if (scalars.length === 0 && subObjects.length === 0) {
              return (
                <pre key={i} className="integration-detail__json mono">
                  {JSON.stringify(obj, null, 2)}
                </pre>
              );
            }
            return (
              <div key={i} className="integration-detail__array-item">
                <div className="integration-detail__rows">
                  {scalars.map(([k, v]) => (
                    <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
                  ))}
                </div>
                {subObjects.map(([k, sub]) => {
                  const subRows = Object.entries(sub).filter(([, v]) => formatScalar(v) !== "");
                  if (subRows.length === 0) return null;
                  return (
                    <div key={k} className="integration-detail__subsection">
                      <h5 className="integration-detail__subsection-title">{formatFieldLabel(k)}</h5>
                      <div className="integration-detail__rows">
                        {subRows.map(([sk, sv]) => (
                          <DetailScalar key={sk} label={formatFieldLabel(sk)} value={sv} />
                        ))}
                      </div>
                    </div>
                  );
                })}
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

// Renderização dedicada da aba "Serviços" (raw.servicos, HubSoft) — os campos mais úteis
// (login, MAC, senha, status, tecnologia, cobrança, velocidades, valor) ganham destaque num
// grid de no máximo 3 colunas no topo do cartão; o resto continua a render genérica
// (scalars + sub-secções) usada nas outras abas. "status_txt_resumido" (duplicado de
// "status_text" já mostrado no cartão resumido) é removido — não é útil aqui.
const SERVICE_PRIORITY_KEYS = [
  "login",
  "mac_addr",
  "nome",
  "senha",
  "status",
  "status_prefixo",
  "status_txt",
  "tecnologia",
  "tipo_cobranca",
  "velocidade_download",
  "velocidade_upload",
  "valor",
];
const SERVICE_HIDDEN_KEYS = new Set(["status_txt_resumido"]);

function serviceStatusBadgeClass(status: string): string {
  const s = status.toLowerCase();
  if (s.includes("habilit")) return "badge badge--ok";
  if (s.includes("desconect")) return "badge badge--err";
  return "badge badge--off";
}

function omitHiddenServiceFields(obj: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj)) {
    if (SERVICE_HIDDEN_KEYS.has(k)) continue;
    out[k] = v;
  }
  return out;
}

function ServicoItemCard({ obj }: { obj: Record<string, unknown> }) {
  const clean = omitHiddenServiceFields(obj);
  const priorityEntries = SERVICE_PRIORITY_KEYS.map((k) => [k, clean[k]] as [string, unknown]).filter(
    ([, v]) => v !== undefined && v !== null && formatScalar(v) !== "",
  );
  const priorityKeys = new Set(priorityEntries.map(([k]) => k));

  const restEntries = Object.entries(clean).filter(([k, v]) => !priorityKeys.has(k) && v !== null && v !== undefined);
  const restScalars = restEntries.filter(([, v]) => typeof v !== "object");
  const restSubObjects = restEntries.filter(
    (entry): entry is [string, Record<string, unknown>] =>
      !!entry[1] && typeof entry[1] === "object" && !Array.isArray(entry[1]),
  );

  return (
    <div className="integration-detail__array-item">
      <div className="integration-detail__rows integration-detail__rows--service-priority">
        {priorityEntries.map(([k, v]) => {
          if (k === "status") {
            const text = formatScalar(v);
            if (!text) return null;
            return (
              <div key={k} className="integration-detail__row">
                <span className="integration-detail__label">{formatFieldLabel(k)}</span>
                <span className={serviceStatusBadgeClass(text)}>{text}</span>
              </div>
            );
          }
          return <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />;
        })}
      </div>
      {restScalars.length > 0 ? (
        <div className="integration-detail__rows" style={{ marginTop: 10 }}>
          {restScalars.map(([k, v]) => (
            <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
          ))}
        </div>
      ) : null}
      {restSubObjects.map(([k, sub]) => {
        const subRows = Object.entries(omitHiddenServiceFields(sub)).filter(([, v]) => formatScalar(v) !== "");
        if (subRows.length === 0) return null;
        return (
          <div key={k} className="integration-detail__subsection">
            <h5 className="integration-detail__subsection-title">{formatFieldLabel(k)}</h5>
            <div className="integration-detail__rows">
              {subRows.map(([sk, sv]) => (
                <DetailScalar key={sk} label={formatFieldLabel(sk)} value={sv} />
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function ServicosTabContent({ items }: { items: unknown[] }) {
  if (items.length === 0) return null;
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">
        Serviços <span className="integration-detail__count">({items.length})</span>
      </h4>
      <div className="integration-detail__array">
        {items.map((item, i) =>
          item && typeof item === "object" && !Array.isArray(item) ? (
            <ServicoItemCard key={i} obj={item as Record<string, unknown>} />
          ) : null,
        )}
      </div>
    </section>
  );
}

type DetailTabDef = { id: string; label: string; content: ReactNode };

// Agrupa os campos de topo do "raw" do cliente em secções pequenas em vez de uma lista
// única — genérico o suficiente para funcionar tanto com o formato da HubSoft como com o
// do IXC (casa por palavra-chave no nome do campo, não por nomes exactos de um só
// fornecedor). Cada array de topo (ex. "grupos", "servicos") vira a sua própria aba —
// cobre "Grupo"/"Serviços" e qualquer outro array presente ("e etc") sem hardcode.
function buildDetailTabs(raw?: Record<string, unknown>): DetailTabDef[] {
  if (!raw || Object.keys(raw).length === 0) return [];

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

  const GROUPS: { title: string; match: (key: string) => boolean }[] = [
    {
      title: "Identificação",
      match: (k) => /nome|razaosocial|fantasia|codigo_cliente|cpf|cnpj|documento|tipo_pessoa|^id(_cliente)?$|status_cadastro|^ativo$/.test(k),
    },
    {
      title: "Contacto",
      match: (k) => /email|telefone|celular|contato/.test(k),
    },
    {
      title: "Datas",
      match: (k) => /^data_|_at$/.test(k),
    },
  ];
  const grouped = GROUPS.map((g) => ({
    title: g.title,
    rows: scalarRows.filter(([k]) => g.match(k)),
  })).filter((g) => g.rows.length > 0);
  const groupedKeys = new Set(grouped.flatMap((g) => g.rows.map(([k]) => k)));
  const otherRows = scalarRows.filter(([k]) => !groupedKeys.has(k));

  const tabs: DetailTabDef[] = [];

  tabs.push({
    id: "identificacao",
    label: "Identificação",
    content: (
      <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
        {grouped.map((g) => (
          <section key={g.title} className="integration-detail__section">
            <h4 className="integration-detail__section-title">{g.title}</h4>
            <div className="integration-detail__rows">
              {g.rows.map(([k, v]) => (
                <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
              ))}
            </div>
          </section>
        ))}
        {otherRows.length > 0 ? (
          <section className="integration-detail__section">
            <h4 className="integration-detail__section-title">Outros</h4>
            <div className="integration-detail__rows">
              {otherRows.map(([k, v]) => (
                <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
              ))}
            </div>
          </section>
        ) : null}
        {objectSections.map(({ key, data }) => (
          <DetailObjectBlock key={key} title={formatFieldLabel(key)} data={data} />
        ))}
      </div>
    ),
  });

  for (const { key, items } of arraySections) {
    if (items.length === 0) continue;
    if (key === "servicos") {
      tabs.push({
        id: `array:${key}`,
        label: "Serviços",
        content: (
          <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
            <ServicosTabContent items={items} />
          </div>
        ),
      });
      continue;
    }
    tabs.push({
      id: `array:${key}`,
      label: formatFieldLabel(key),
      content: (
        <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
          <DetailArrayBlock title={formatFieldLabel(key)} items={items} />
        </div>
      ),
    });
  }

  return tabs;
}

function formatCurrencyBRL(raw?: string | number): string {
  if (raw === undefined || raw === null || raw === "") return "—";
  const n = typeof raw === "number" ? raw : parseFloat(String(raw).replace(",", "."));
  if (Number.isNaN(n)) return String(raw);
  return n.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

function invoiceStatusBadgeClass(inv: InvoiceItem): string {
  if (inv.paid) return "badge badge--ok";
  if ((inv.status ?? "").trim().toLowerCase() === "vencido") return "badge badge--err";
  return "badge badge--off";
}

function invoiceStatusLabel(inv: InvoiceItem): string {
  if (inv.paid) return "Paga";
  if ((inv.status ?? "").trim().toLowerCase() === "vencido") return "Vencida";
  return inv.status?.trim() || "Pendente";
}

function FinancialSummaryPanel({ summary }: { summary: FinancialSummary }) {
  return (
    <section className="integration-detail__section">
      <h4 className="integration-detail__section-title">Resumo</h4>
      <div className="integration-detail__rows">
        <DetailScalar label="Total de faturas" value={summary.total} />
        <DetailScalar label="Valor total" value={formatCurrencyBRL(summary.total_value)} />
        <DetailScalar label="Vencidas" value={`${summary.overdue_count} · ${formatCurrencyBRL(summary.overdue_value)}`} />
        <DetailScalar label="Pendentes" value={`${summary.pending_count} · ${formatCurrencyBRL(summary.pending_value)}`} />
        <DetailScalar label="Pagas" value={`${summary.paid_count} · ${formatCurrencyBRL(summary.paid_value)}`} />
      </div>
    </section>
  );
}

export function FinancialTabContent({
  loading,
  ok,
  message,
  invoices,
  summary,
}: {
  loading: boolean;
  ok: boolean;
  message?: string;
  invoices: InvoiceItem[];
  summary?: FinancialSummary;
}) {
  if (loading) {
    return <p className="integration-detail__empty">A carregar faturas…</p>;
  }
  if (!ok && message) {
    return <div className="msg msg--err">{message}</div>;
  }
  if (invoices.length === 0) {
    return <div className="msg">{message || "Nenhuma fatura encontrada."}</div>;
  }
  return (
    <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
      {summary ? <FinancialSummaryPanel summary={summary} /> : null}
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">
          Faturas <span className="integration-detail__count">({invoices.length})</span>
        </h4>
        <div className="table-wrap integration-support-table">
          <table className="integration-support-table__grid">
            <thead>
              <tr>
                <th>Vencimento</th>
                <th>Valor</th>
                <th>Status</th>
                <th>Pagamento</th>
                <th>Serviço</th>
                <th>Boleto</th>
              </tr>
            </thead>
            <tbody>
              {invoices.map((inv, i) => (
                <tr key={inv.id ?? i}>
                  <td className="integration-support-table__cell integration-support-table__cell--date">{inv.due_date || "—"}</td>
                  <td className="mono integration-support-table__cell">{formatCurrencyBRL(inv.value)}</td>
                  <td className="integration-support-table__cell">
                    <span className={invoiceStatusBadgeClass(inv)}>{invoiceStatusLabel(inv)}</span>
                  </td>
                  <td className="integration-support-table__cell integration-support-table__cell--date">{inv.payment_date || "—"}</td>
                  <td className="integration-support-table__cell">{inv.service_name || "—"}</td>
                  <td className="integration-support-table__cell">
                    {inv.boleto_link ? (
                      <a href={inv.boleto_link} target="_blank" rel="noreferrer">
                        Ver boleto
                      </a>
                    ) : (
                      "—"
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

export type FinancialState = {
  ok: boolean;
  message?: string;
  invoices: InvoiceItem[];
  summary?: FinancialSummary;
};
export type AttendanceState = { ok: boolean; message?: string; items: AttendanceItem[] };
export type WorkOrderState = { ok: boolean; message?: string; items: WorkOrderItem[] };
export type LoginState = { ok: boolean; message?: string; items: ClientServiceSummary[] };

export function AttendanceTabContent({
  loading,
  ok,
  message,
  items,
  onShowDetail,
}: {
  loading: boolean;
  ok: boolean;
  message?: string;
  items: AttendanceItem[];
  onShowDetail: (t: SupportDetailTarget) => void;
}) {
  if (loading) return <p className="integration-detail__empty">A carregar atendimentos…</p>;
  if (!ok && message) return <div className="msg msg--err">{message}</div>;
  if (items.length === 0) return <div className="msg">{message || "Nenhum atendimento encontrado."}</div>;
  const showClient = items.some((a) => a.client_name || a.client_code);
  return (
    <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">
          Atendimentos <span className="integration-detail__count">({items.length})</span>
        </h4>
        <div className="table-wrap integration-support-table">
          <table className="integration-support-table__grid integration-support-table__grid--att">
            <thead>
              <tr>
                {showClient ? <th>Cliente</th> : null}
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
              {items.map((a, i) => (
                <tr key={a.id ?? a.protocol ?? i}>
                  {showClient ? (
                    <td className="integration-support-table__cell">
                      {a.client_name || "—"}
                      {a.client_code ? <span className="mono integration-support-table__meta"> · {a.client_code}</span> : null}
                    </td>
                  ) : null}
                  <td className="mono integration-support-table__cell">{a.protocol || "—"}</td>
                  <td className="integration-support-table__cell">
                    {formatAttendanceStatus(a) ? (
                      <span className={labelStatus(formatAttendanceStatus(a)) ?? "badge"}>{formatAttendanceStatus(a)}</span>
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
                      onClick={() => onShowDetail({ kind: "attendance", item: a })}
                    >
                      Ver mais
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

export function WorkOrdersTabContent({
  loading,
  ok,
  message,
  items,
  onShowDetail,
}: {
  loading: boolean;
  ok: boolean;
  message?: string;
  items: WorkOrderItem[];
  onShowDetail: (t: SupportDetailTarget) => void;
}) {
  if (loading) return <p className="integration-detail__empty">A carregar ordens de serviço…</p>;
  if (!ok && message) return <div className="msg msg--err">{message}</div>;
  if (items.length === 0) return <div className="msg">{message || "Nenhuma ordem de serviço encontrada."}</div>;
  const showClient = items.some((o) => o.client_name || o.client_code);
  return (
    <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">
          Ordens de serviço <span className="integration-detail__count">({items.length})</span>
        </h4>
        <div className="table-wrap integration-support-table">
          <table className="integration-support-table__grid integration-support-table__grid--os">
            <thead>
              <tr>
                {showClient ? <th>Cliente</th> : null}
                <th>N.º O.S.</th>
                <th>Estado O.S.</th>
                <th>Plano / serviço</th>
                <th>Tipo de O.S.</th>
                <th>Cadastro</th>
                <th>Agendamento</th>
                <th className="integration-support-table__col-actions" />
              </tr>
            </thead>
            <tbody>
              {items.map((o, i) => (
                <tr key={o.id ?? o.number ?? i}>
                  {showClient ? (
                    <td className="integration-support-table__cell">
                      {o.client_name || "—"}
                      {o.client_code ? <span className="mono integration-support-table__meta"> · {o.client_code}</span> : null}
                    </td>
                  ) : null}
                  <td className="mono integration-support-table__cell">{o.number || "—"}</td>
                  <td className="integration-support-table__cell">
                    <span className={labelStatus(formatWorkOrderStatus(o)) ?? "badge"}>{formatWorkOrderStatus(o) || "—"}</span>
                  </td>
                  <td className="integration-support-table__cell integration-support-table__cell--plan">
                    <div className="integration-os-plan__title">
                      <TableCellExpandableText text={o.plan_name || o.description} maxLength={60} />
                    </div>
                    {o.service_status ? <div className="integration-os-plan__meta">Estado do serviço: {o.service_status}</div> : null}
                    {o.value ? <div className="integration-os-plan__meta">Valor: {o.value}</div> : null}
                  </td>
                  <td className="integration-support-table__cell">{o.type || "—"}</td>
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
                      onClick={() => onShowDetail({ kind: "work_order", item: o })}
                    >
                      Ver mais
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

export function LoginsTabContent({
  loading,
  ok,
  message,
  items,
}: {
  loading: boolean;
  ok: boolean;
  message?: string;
  items: ClientServiceSummary[];
}) {
  if (loading) return <p className="integration-detail__empty">A carregar logins…</p>;
  if (!ok && message) return <div className="msg msg--err">{message}</div>;
  if (items.length === 0) return <div className="msg">{message || "Nenhum login encontrado."}</div>;
  return (
    <div className="integration-detail" style={{ fontSize: DETAIL_FONT }}>
      <section className="integration-detail__section">
        <h4 className="integration-detail__section-title">
          Logins <span className="integration-detail__count">({items.length})</span>
        </h4>
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
              {items.map((s, i) => {
                const online = formatIXCOnline(s.online, s.online_label);
                const statusInternet = formatIXCContractStatus(s.status_internet, s.status_label);
                return (
                  <tr key={s.id ?? s.login ?? i}>
                    <td className="mono integration-support-table__cell">{s.login || "—"}</td>
                    <td className="mono integration-support-table__cell">{s.contrato || "—"}</td>
                    <td className="integration-support-table__cell">{s.plano_venda || s.name || "—"}</td>
                    <td className="integration-support-table__cell">
                      {online ? <span className={labelStatus(online) ?? "badge"}>{online}</span> : "—"}
                    </td>
                    <td className="integration-support-table__cell">
                      {statusInternet ? <span className={labelStatus(statusInternet) ?? "badge"}>{statusInternet}</span> : "—"}
                    </td>
                    <td className="mono integration-support-table__cell">{s.mac || "—"}</td>
                    <td className="mono integration-support-table__cell">{s.ipv4 || "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

// Exportado para reuso na aba Relatório (HubsoftReportPage.tsx) — mesma modal de "dados
// completos" já usada pela Consulta, sem duplicar a UI.
export function ClientDetailModal({
  client,
  loading,
  onClose,
  onFetchFinancial,
  onFetchAttendance,
  onFetchWorkOrders,
  onFetchLogins,
  attendanceEnabled,
  workOrderEnabled,
  loginEnabled,
  prefetchExtras,
}: {
  client: ClientCard;
  loading?: boolean;
  onClose: () => void;
  onFetchFinancial?: (client: ClientCard) => Promise<FinancialState>;
  onFetchAttendance?: (client: ClientCard) => Promise<AttendanceState>;
  onFetchWorkOrders?: (client: ClientCard) => Promise<WorkOrderState>;
  onFetchLogins?: (client: ClientCard) => Promise<LoginState>;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
  loginEnabled?: boolean;
  // Busca atendimentos/ordens de serviço em paralelo assim que o modal abre, em vez de só ao
  // clicar na aba — reduz a espera percebida ao trocar de aba. Opt-in (default false) para não
  // mudar o comportamento do IXC, que continua a buscar só ao clicar na aba.
  prefetchExtras?: boolean;
}) {
  const detailTabs = useMemo(() => buildDetailTabs(client.raw), [client.raw]);
  const tabs = useMemo(() => {
    const extra: DetailTabDef[] = [];
    if (onFetchFinancial) extra.push({ id: "financeiro", label: "Financeiro", content: null });
    if (onFetchAttendance && attendanceEnabled !== false) extra.push({ id: "atendimentos", label: "Atendimentos", content: null });
    if (onFetchWorkOrders && workOrderEnabled !== false) extra.push({ id: "ordens", label: "Ordens de serviço", content: null });
    if (onFetchLogins && loginEnabled !== false) extra.push({ id: "logins", label: "Logins", content: null });
    return [...detailTabs, ...extra];
  }, [detailTabs, onFetchFinancial, onFetchAttendance, onFetchWorkOrders, onFetchLogins, attendanceEnabled, workOrderEnabled, loginEnabled]);

  const [activeTab, setActiveTab] = useState(tabs[0]?.id ?? "identificacao");
  const [financial, setFinancial] = useState<FinancialState | null>(null);
  const [financialLoading, setFinancialLoading] = useState(false);
  const [attendance, setAttendance] = useState<AttendanceState | null>(null);
  const [attendanceLoading, setAttendanceLoading] = useState(false);
  const [workOrders, setWorkOrders] = useState<WorkOrderState | null>(null);
  const [workOrderLoading, setWorkOrderLoading] = useState(false);
  const [logins, setLogins] = useState<LoginState | null>(null);
  const [loginLoading, setLoginLoading] = useState(false);
  const [detailTarget, setDetailTarget] = useState<SupportDetailTarget | null>(null);

  useEffect(() => {
    setActiveTab(tabs[0]?.id ?? "identificacao");
    setFinancial(null);
    setFinancialLoading(false);
    setAttendance(null);
    setAttendanceLoading(false);
    setWorkOrders(null);
    setWorkOrderLoading(false);
    setLogins(null);
    setLoginLoading(false);
    setDetailTarget(null);
  }, [client.id, client.code, tabs]);

  useEffect(() => {
    if (activeTab !== "financeiro" || !onFetchFinancial || financial || financialLoading) return;
    setFinancialLoading(true);
    onFetchFinancial(client)
      .then((r) => setFinancial(r))
      .catch((e) => setFinancial({ ok: false, message: e instanceof Error ? e.message : String(e), invoices: [] }))
      .finally(() => setFinancialLoading(false));
  }, [activeTab, onFetchFinancial, financial, financialLoading, client]);

  useEffect(() => {
    if (!onFetchAttendance || attendance || attendanceLoading) return;
    if (!prefetchExtras && activeTab !== "atendimentos") return;
    setAttendanceLoading(true);
    onFetchAttendance(client)
      .then((r) => setAttendance(r))
      .catch((e) => setAttendance({ ok: false, message: e instanceof Error ? e.message : String(e), items: [] }))
      .finally(() => setAttendanceLoading(false));
  }, [activeTab, onFetchAttendance, attendance, attendanceLoading, client, prefetchExtras]);

  useEffect(() => {
    if (!onFetchWorkOrders || workOrders || workOrderLoading) return;
    if (!prefetchExtras && activeTab !== "ordens") return;
    setWorkOrderLoading(true);
    onFetchWorkOrders(client)
      .then((r) => setWorkOrders(r))
      .catch((e) => setWorkOrders({ ok: false, message: e instanceof Error ? e.message : String(e), items: [] }))
      .finally(() => setWorkOrderLoading(false));
  }, [activeTab, onFetchWorkOrders, workOrders, workOrderLoading, client, prefetchExtras]);

  useEffect(() => {
    if (activeTab !== "logins" || !onFetchLogins || logins || loginLoading) return;
    setLoginLoading(true);
    onFetchLogins(client)
      .then((r) => setLogins(r))
      .catch((e) => setLogins({ ok: false, message: e instanceof Error ? e.message : String(e), items: [] }))
      .finally(() => setLoginLoading(false));
  }, [activeTab, onFetchLogins, logins, loginLoading, client]);

  let activeContent: ReactNode;
  if (activeTab === "financeiro") {
    activeContent = (
      <FinancialTabContent
        loading={financialLoading}
        ok={financial?.ok ?? true}
        message={financial?.message}
        invoices={financial?.invoices ?? []}
        summary={financial?.summary}
      />
    );
  } else if (activeTab === "atendimentos") {
    activeContent = (
      <AttendanceTabContent
        loading={attendanceLoading}
        ok={attendance?.ok ?? true}
        message={attendance?.message}
        items={attendance?.items ?? []}
        onShowDetail={setDetailTarget}
      />
    );
  } else if (activeTab === "ordens") {
    activeContent = (
      <WorkOrdersTabContent
        loading={workOrderLoading}
        ok={workOrders?.ok ?? true}
        message={workOrders?.message}
        items={workOrders?.items ?? []}
        onShowDetail={setDetailTarget}
      />
    );
  } else if (activeTab === "logins") {
    activeContent = (
      <LoginsTabContent loading={loginLoading} ok={logins?.ok ?? true} message={logins?.message} items={logins?.items ?? []} />
    );
  } else {
    activeContent = tabs.find((t) => t.id === activeTab)?.content ?? null;
  }

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
        ) : tabs.length === 0 ? (
          <p className="integration-detail__empty">Sem dados detalhados disponíveis.</p>
        ) : (
          <>
            <div className="tabs integration-detail-modal__tabs">
              {tabs.map((t) => (
                <button key={t.id} type="button" className={t.id === activeTab ? "active" : ""} onClick={() => setActiveTab(t.id)}>
                  {t.label}
                </button>
              ))}
            </div>
            <div className="integration-detail-modal__tab-body">{activeContent}</div>
          </>
        )}
      </div>
      {detailTarget ? <SupportItemDetailModal target={detailTarget} onClose={() => setDetailTarget(null)} /> : null}
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
  onFetchFinancial,
  attendanceEnabled,
  workOrderEnabled,
  loginEnabled,
  prefetchExtras,
}: {
  clients: ClientCard[];
  message?: string;
  ok: boolean;
  localFilter: string;
  onFetchDetail?: (client: ClientCard) => Promise<ClientCard>;
  onFetchAttendance?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: AttendanceItem[] }>;
  onFetchWorkOrders?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: WorkOrderItem[] }>;
  onFetchLogins?: (client: ClientCard) => Promise<{ ok: boolean; message?: string; items: ClientServiceSummary[] }>;
  onFetchFinancial?: (client: ClientCard) => Promise<FinancialState>;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
  loginEnabled?: boolean;
  prefetchExtras?: boolean;
}) {
  const [detailClient, setDetailClient] = useState<ClientCard | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
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
                <button
                  type="button"
                  className="integration-consult-card__title integration-consult-card__title--link"
                  onClick={() => void openDetail(c)}
                >
                  {c.name || "—"}
                </button>
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
        <ClientDetailModal
          client={detailClient}
          loading={detailLoading}
          onClose={() => setDetailClient(null)}
          onFetchFinancial={onFetchFinancial}
          onFetchAttendance={onFetchAttendance}
          onFetchWorkOrders={onFetchWorkOrders}
          onFetchLogins={onFetchLogins}
          attendanceEnabled={attendanceEnabled}
          workOrderEnabled={workOrderEnabled}
          loginEnabled={loginEnabled}
          prefetchExtras={prefetchExtras}
        />
      ) : null}
    </>
  );
}
