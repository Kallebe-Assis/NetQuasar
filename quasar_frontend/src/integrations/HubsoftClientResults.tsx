import { useEffect, useMemo, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import {
  AlertTriangle,
  Building2,
  CalendarClock,
  CalendarDays,
  CheckCircle2,
  Clock,
  ExternalLink,
  Eye,
  EyeOff,
  FileText,
  Gauge,
  Headset,
  IdCard,
  KeyRound,
  ClipboardList,
  LogIn,
  MapPin,
  Phone,
  Receipt,
  RefreshCw,
  Router,
  User,
  UserRound,
  Wallet,
  X,
  XCircle,
  type LucideIcon,
} from "lucide-react";
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

// Serviço cancelado/inativo — mesmo léxico de labelStatus acima (a HubSoft não expõe um booleano
// dedicado por serviço, só o texto/prefixo de status — "cancelado" é o valor real confirmado
// ao vivo, ver status_prefixo). Usado para riscar o texto do serviço (tachado), não o status dele
// (que já tem badge próprio) — dois sinais visuais complementares, não redundantes.
function isServiceInactive(s: ClientServiceSummary): boolean {
  const t = `${s.status ?? ""} ${s.status_prefix ?? ""}`.toLowerCase();
  return t.includes("cancel") || t.includes("inativ");
}

// Conectividade do login — Hubsoft usa `connected` ("true"/"false", ver ultima_conexao.conectado),
// IXC usa `online` ("S"/"N"). null = sem dado (não pinta nada, evita sugerir um estado que não
// temos certeza).
function isServiceOnline(s: ClientServiceSummary): boolean | null {
  const c = (s.connected ?? "").trim().toLowerCase();
  if (c === "true") return true;
  if (c === "false") return false;
  const o = (s.online ?? "").trim().toUpperCase();
  if (o === "S") return true;
  if (o === "N") return false;
  return null;
}

function onlineTabModifier(s: ClientServiceSummary): string {
  const online = isServiceOnline(s);
  if (online === true) return " is-online";
  if (online === false) return " is-offline";
  return "";
}

function InactiveClientMark() {
  return (
    <span className="hubsoft-inactive-mark" title="Cliente inativo" aria-label="Cliente inativo">
      <XCircle size={14} />
    </span>
  );
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
                    className={
                      (si === safeIdx ? "integration-consult-card__login-tab active" : "integration-consult-card__login-tab") +
                      onlineTabModifier(s)
                    }
                    onClick={() => onSelectService(si)}
                  >
                    <span className={isServiceInactive(s) ? "hubsoft-service-inactive" : undefined}>{label}</span>
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

type ServiceActionTarget = { client: ClientCard; service: ClientServiceSummary; mode: "enable" | "suspend" };
type ServiceActionResult = { ok: boolean; message?: string };

// Confirmação antes de habilitar/suspender — acção real contra a HubSoft, afecta o acesso à
// internet do cliente na hora, por isso não dispara direto no clique do botão do cartão.
function ServiceActionModal({
  target,
  onCancel,
  onConfirm,
}: {
  target: ServiceActionTarget;
  onCancel: () => void;
  onConfirm: (value: string) => Promise<ServiceActionResult>;
}) {
  const { client, service, mode } = target;
  const [motivo, setMotivo] = useState("");
  const [tipoSuspensao, setTipoSuspensao] = useState<"suspenso_debito" | "suspenso_pedido_cliente">("suspenso_debito");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const label = service.login?.trim() || service.name?.trim() || client.name || "este serviço";

  async function confirm() {
    setBusy(true);
    setError("");
    try {
      const r = await onConfirm(mode === "enable" ? motivo.trim() : tipoSuspensao);
      if (!r.ok) {
        setError(r.message || "Não foi possível concluir a operação.");
        return;
      }
      onCancel();
    } finally {
      setBusy(false);
    }
  }

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={busy ? undefined : onCancel}>
      <div className="modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 420 }}>
        <h3 style={{ marginTop: 0 }}>{mode === "enable" ? "Habilitar serviço" : "Suspender serviço"}</h3>
        <p style={{ fontSize: 13, color: "var(--muted)" }}>
          {mode === "enable" ? "Habilitar" : "Suspender"} <strong>{label}</strong>
          {mode === "suspend" ? " — o cliente perde acesso à internet imediatamente." : "."}
        </p>
        {mode === "enable" ? (
          <div className="field">
            <label htmlFor="hubsoft-enable-motivo">Motivo</label>
            <input
              id="hubsoft-enable-motivo"
              className="input"
              value={motivo}
              onChange={(e) => setMotivo(e.target.value)}
              placeholder="Ex.: Pagamento confirmado"
              disabled={busy}
            />
          </div>
        ) : (
          <div className="field">
            <label htmlFor="hubsoft-suspend-tipo">Motivo da suspensão</label>
            <select
              id="hubsoft-suspend-tipo"
              className="input"
              value={tipoSuspensao}
              onChange={(e) => setTipoSuspensao(e.target.value as typeof tipoSuspensao)}
              disabled={busy}
            >
              <option value="suspenso_debito">Suspenso por débito</option>
              <option value="suspenso_pedido_cliente">Suspenso a pedido do cliente</option>
            </select>
          </div>
        )}
        {error ? <div className="msg msg--err" style={{ marginTop: 8 }}>{error}</div> : null}
        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" onClick={onCancel} disabled={busy}>
            Cancelar
          </button>
          <button
            type="button"
            className={mode === "suspend" ? "btn btn--danger" : "btn btn--primary"}
            onClick={() => void confirm()}
            disabled={busy}
          >
            {busy ? "A processar…" : "Confirmar"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
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

function DetailObjectBlock({ title, data, accentIndex = 0 }: { title: string; data: Record<string, unknown>; accentIndex?: number }) {
  const rows = Object.entries(data).filter(([, v]) => formatScalar(v) !== "");
  if (rows.length === 0) return null;
  return (
    <ServiceCard icon={Building2} title={title} accent={ACCENT_CYCLE[accentIndex % ACCENT_CYCLE.length]}>
      {rows.map(([k, v]) => {
        if (v !== null && typeof v === "object") return null;
        return <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />;
      })}
    </ServiceCard>
  );
}

function DetailArrayBlock({ title, items }: { title: string; items: unknown[] }) {
  if (items.length === 0) return null;
  return (
    <section className="integration-detail__section hubsoft-generic-card">
      <h4 className="integration-detail__section-title">
        <Building2 size={13} /> {title} <span className="integration-detail__count">({items.length})</span>
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

// ---- Aba "Serviços" do modal "Ver dados completos" — layout em cartões, espelha o próprio
// painel da HubSoft (ver docs.hubsoft.com.br). Alimentado pelos campos tipados de
// ClientServiceSummary (internal/integrationhubsoft ServiceSummary), não por um dump genérico do
// JSON bruto — cada cartão só aparece quando tem pelo menos um campo preenchido.

function ServiceInfoRow({
  icon: Icon,
  label,
  value,
  strike,
}: {
  icon?: LucideIcon;
  label: string;
  value?: string;
  // Serviço cancelado/inativo — risca só o valor (o rótulo continua legível).
  strike?: boolean;
}) {
  if (!value?.trim()) return null;
  return (
    <div className="hubsoft-row">
      <span className="hubsoft-row__label">
        {Icon ? <Icon size={12} /> : null}
        {label}
      </span>
      <span className={strike ? "hubsoft-row__value hubsoft-service-inactive" : "hubsoft-row__value"}>{value}</span>
    </div>
  );
}

// Senha vem em texto simples da HubSoft (é a credencial PPPoE/autenticação do cliente, não uma
// senha de conta) — mascarada por omissão com alternância para ver, mesmo padrão do painel deles.
function ServicePasswordRow({ password }: { password?: string }) {
  const [visible, setVisible] = useState(false);
  if (!password?.trim()) return null;
  return (
    <div className="hubsoft-row">
      <span className="hubsoft-row__label">
        <KeyRound size={12} /> Senha
      </span>
      <span className="hubsoft-row__value row" style={{ gap: 6, alignItems: "center" }}>
        <span className="mono">{visible ? password : "•".repeat(Math.max(6, password.length))}</span>
        <button
          type="button"
          className="btn btn--icon btn--sm"
          onClick={() => setVisible((v) => !v)}
          aria-label={visible ? "Ocultar senha" : "Mostrar senha"}
          title={visible ? "Ocultar senha" : "Mostrar senha"}
        >
          {visible ? <EyeOff size={13} /> : <Eye size={13} />}
        </button>
      </span>
    </div>
  );
}

// Badge colorido em vez de texto simples — dá pra distinguir tecnologias à primeira vista numa
// lista de clientes com serviços mistos (fibra/rádio/cabo).
function technologyBadgeModifier(tech: string): string {
  const t = tech.toLowerCase();
  if (t.includes("fibra") || t.includes("ftt") || t.includes("gpon") || t.includes("epon")) return "hubsoft-tech-badge--fibra";
  if (t.includes("radio") || t.includes("rádio") || t.includes("wireless") || t.includes("wifi")) return "hubsoft-tech-badge--radio";
  if (t.includes("cabo") || t.includes("metal") || t.includes("ethernet")) return "hubsoft-tech-badge--cabo";
  return "hubsoft-tech-badge--outro";
}

function ServiceTechnologyRow({ value }: { value?: string }) {
  if (!value?.trim()) return null;
  return (
    <div className="hubsoft-row">
      <span className="hubsoft-row__label">
        <Router size={12} /> Tecnologia
      </span>
      <span className="hubsoft-row__value">
        <span className={`hubsoft-tech-badge ${technologyBadgeModifier(value)}`}>{value}</span>
      </span>
    </div>
  );
}

// Barra de progresso simples (não é medição em tempo real — é o plano/velocidade contratada) para
// dar uma referência visual de grandeza além do número puro. 1000 Mbits como teto de referência
// cobre a esmagadora maioria dos planos residenciais/empresariais desta operadora.
function parseMbits(v: string): number | null {
  const n = parseFloat(v.replace(",", "."));
  return Number.isFinite(n) ? n : null;
}

function ServiceSpeedRow({ label, value }: { label: string; value?: string }) {
  if (!value?.trim()) return null;
  const n = parseMbits(value);
  const pct = n != null ? Math.max(4, Math.min(100, (n / 1000) * 100)) : null;
  return (
    <div className="hubsoft-row">
      <span className="hubsoft-row__label">
        <Gauge size={12} /> {label}
      </span>
      <span className="hubsoft-row__value hubsoft-speed">
        <span className="hubsoft-speed__value">{value}</span>
        {pct != null ? (
          <span className="hubsoft-speed__bar" aria-hidden>
            <span className="hubsoft-speed__fill" style={{ width: `${pct}%` }} />
          </span>
        ) : null}
      </span>
    </div>
  );
}

function ServiceConnectionStatusBox({ text, connected }: { text?: string; connected?: string }) {
  if (!text?.trim()) return null;
  const isOn = connected === "true";
  const isOff = connected === "false";
  return (
    <div className={`hubsoft-status-box ${isOn ? "hubsoft-status-box--on" : isOff ? "hubsoft-status-box--off" : ""}`}>
      <span className={`hubsoft-status-box__dot ${isOn ? "hubsoft-status-box__dot--live" : isOff ? "hubsoft-status-box__dot--down" : ""}`} aria-hidden />
      <span>{text}</span>
    </div>
  );
}

function ServiceMapLink({ latitude, longitude }: { latitude?: string; longitude?: string }) {
  if (!latitude?.trim() || !longitude?.trim()) return null;
  const url = `https://www.google.com/maps?q=${encodeURIComponent(latitude)},${encodeURIComponent(longitude)}`;
  return (
    <a href={url} target="_blank" rel="noreferrer" className="hubsoft-map-link">
      <MapPin size={12} /> Ver no mapa
    </a>
  );
}

type ServiceCardAccent = "conexao" | "endereco" | "cadastro" | "historico" | "vendedor";
// Ordem usada para ciclar cor+ícone entre secções/abas que não têm uma cor "natural" própria
// (Identificação, Financeiro, Atendimentos, …) — reaproveita as mesmas 5 cores da aba Serviços,
// nunca aparecem lado a lado então não há confusão de significado entre abas.
const ACCENT_CYCLE: ServiceCardAccent[] = ["conexao", "cadastro", "historico", "endereco", "vendedor"];

function ServiceCard({
  icon: Icon,
  title,
  accent,
  children,
}: {
  icon: LucideIcon;
  title: string;
  accent: ServiceCardAccent;
  children: ReactNode;
}) {
  return (
    <div className={`hubsoft-service-card hubsoft-service-card--${accent}`}>
      <div className="hubsoft-service-card__head">
        <span className="hubsoft-service-card__icon">
          <Icon size={14} />
        </span>
        <h5>{title}</h5>
      </div>
      <div className="hubsoft-service-card__body">{children}</div>
    </div>
  );
}

function ServicesTabContent({
  services,
  onEnableClick,
  onSuspendClick,
}: {
  services: ClientServiceSummary[];
  onEnableClick?: (s: ClientServiceSummary) => void;
  onSuspendClick?: (s: ClientServiceSummary) => void;
}) {
  const [idx, setIdx] = useState(0);
  if (services.length === 0) {
    return <div className="msg">Nenhum serviço encontrado.</div>;
  }
  const safeIdx = Math.min(idx, services.length - 1);
  const s = services[safeIdx];
  const pending = Number(s.pending_contracts ?? "");
  const hasSeller = !!(s.seller_name || s.seller_id || s.seller_email);

  return (
    <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
      <div className="hubsoft-services-head">
        <div className="row" style={{ gap: 10, alignItems: "center", flexWrap: "wrap" }}>
          <h4 className="integration-detail__section-title" style={{ margin: 0 }}>
            Serviços <span className="integration-detail__count">({services.length})</span>
            {s.login ? <span style={{ fontWeight: 400, textTransform: "none" }}> — Login: {s.login}</span> : null}
          </h4>
          {s.status ? <span className={labelStatus(s.status) ?? "badge"}>{s.status}</span> : null}
        </div>
        {(onEnableClick || onSuspendClick) && s.id ? (
          <div className="row" style={{ gap: 8 }}>
            {onEnableClick ? (
              <button type="button" className="btn btn--sm" onClick={() => onEnableClick(s)}>
                Habilitar serviço
              </button>
            ) : null}
            {onSuspendClick ? (
              <button type="button" className="btn btn--sm btn--danger" onClick={() => onSuspendClick(s)}>
                Suspender serviço
              </button>
            ) : null}
          </div>
        ) : null}
      </div>

      {services.length > 1 ? (
        <div className="hubsoft-services-picker" role="tablist" aria-label="Logins do cliente">
          {services.map((svc, si) => (
            <button
              key={svc.id ?? si}
              type="button"
              role="tab"
              aria-selected={si === safeIdx}
              className={
                (si === safeIdx ? "hubsoft-services-picker__tab active" : "hubsoft-services-picker__tab") + onlineTabModifier(svc)
              }
              onClick={() => setIdx(si)}
            >
              <User size={12} />
              <span className={isServiceInactive(svc) ? "hubsoft-service-inactive" : undefined}>
                {svc.login?.trim() || svc.name?.trim() || `Serviço ${si + 1}`}
              </span>
            </button>
          ))}
        </div>
      ) : null}

      <div className="hubsoft-service-grid">
        <ServiceCard icon={RefreshCw} title="Conexão Atual" accent="conexao">
          {s.status_prefix ? (
            <div className="hubsoft-row">
              <span className="hubsoft-row__label">Status</span>
              <span className="hubsoft-row__value">
                <span className={labelStatus(s.status_prefix) ?? "badge"}>{s.status_prefix}</span>
              </span>
            </div>
          ) : null}
          <ServiceInfoRow label="MAC Addr" value={s.mac} />
          <ServiceInfoRow label="Phy Addr" value={s.phy_addr} />
          <ServiceTechnologyRow value={s.technology} />
          <ServiceInfoRow label="Ipv4" value={s.ipv4} />
          <ServiceInfoRow label="Último Ipv4" value={s.last_ipv4} />
          <ServiceSpeedRow label="Vel. Download" value={s.download_speed} />
          <ServiceSpeedRow label="Vel. Upload" value={s.upload_speed} />
          <ServiceInfoRow label="Última Conexão" value={s.last_connected_at} />
          <ServiceConnectionStatusBox text={s.status_text_full} connected={s.connected} />
        </ServiceCard>

        <ServiceCard icon={MapPin} title="Endereço de Instalação" accent="endereco">
          <ServiceInfoRow label="Completo" value={s.install_address} />
          <ServiceInfoRow label="CEP" value={s.address_cep} />
          <ServiceInfoRow label="Complemento" value={s.address_complement} />
          <ServiceInfoRow label="Bairro" value={s.address_neighborhood} />
          <ServiceInfoRow label="Cidade" value={s.city} />
          <ServiceInfoRow label="Estado" value={s.address_state} />
          <ServiceInfoRow label="Número" value={s.address_number} />
          <ServiceInfoRow label="País" value={s.address_country} />
          <ServiceInfoRow label="UF" value={s.address_uf} />
          <ServiceInfoRow label="Ibge Cidade" value={s.address_ibge} />
          <ServiceMapLink latitude={s.latitude} longitude={s.longitude} />
        </ServiceCard>

        <ServiceCard icon={KeyRound} title="Cadastro & Autenticação" accent="cadastro">
          <ServiceInfoRow label="Plano" value={s.name} strike={isServiceInactive(s)} />
          <ServiceInfoRow label="Plano Número" value={s.plan_number} />
          <ServiceInfoRow label="Valor" value={s.plan_value ? formatCurrencyBRL(s.plan_value) : undefined} />
          <ServiceInfoRow icon={User} label="Login PPPoE" value={s.login} />
          <ServicePasswordRow password={s.password} />
          <ServiceInfoRow label="Id Cliente Serviço" value={s.id} />
          <ServiceInfoRow label="Id Serviço Antigo" value={s.old_service_id} />
          <ServiceInfoRow label="Uuid" value={s.uuid} />
          <ServiceInfoRow label="Carnê" value={s.carne} />
          <ServiceInfoRow label="Tipo de Cobrança" value={s.billing_type} />
          {s.notes?.trim() ? (
            <div className="hubsoft-row">
              <span className="hubsoft-row__label">Anotações</span>
              <span className="hubsoft-row__value">
                <TableCellExpandableText text={s.notes} />
              </span>
            </div>
          ) : null}
        </ServiceCard>

        <ServiceCard icon={CalendarClock} title="Histórico & Prazos" accent="historico">
          <ServiceInfoRow label="Data Cadastro" value={s.registered_at} />
          <ServiceInfoRow label="Data Habilitação" value={s.enabled_at} />
          <ServiceInfoRow label="Data Venda" value={s.sold_at} />
          <ServiceInfoRow label="Início Contrato" value={s.contract_start_at} />
          <ServiceInfoRow label="Fim Contrato" value={s.contract_end_at} />
          <ServiceInfoRow label="Vigência (meses)" value={s.contract_months} />
          {s.pending_contracts ? (
            <div className="hubsoft-row">
              <span className="hubsoft-row__label">Contratos Pendentes</span>
              <span className="hubsoft-row__value">
                <span className={pending > 0 ? "badge badge--err" : "badge badge--ok"}>
                  {pending > 0 ? <AlertTriangle size={11} style={{ verticalAlign: "-2px" }} /> : null} {s.pending_contracts}
                </span>
              </span>
            </div>
          ) : null}
          <ServiceInfoRow label="Última Atualização" value={s.updated_at} />
        </ServiceCard>

        {hasSeller ? (
          <ServiceCard icon={UserRound} title="Vendedor" accent="vendedor">
            <ServiceInfoRow label="Nome" value={s.seller_name} />
            <ServiceInfoRow label="Id Vendedor" value={s.seller_id} />
            <ServiceInfoRow label="E-mail" value={s.seller_email} />
          </ServiceCard>
        ) : null}
      </div>
    </div>
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

  // Ícone + cor por secção — mesma linguagem visual da aba Serviços, ciclando pelas 5 cores já
  // definidas (não há problema em repetir entre abas, nunca aparecem lado a lado).
  const IDENT_ICONS: LucideIcon[] = [IdCard, Phone, CalendarDays, FileText];

  tabs.push({
    id: "identificacao",
    label: "Identificação",
    content: (
      <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
        <div className="hubsoft-service-grid">
          {grouped.map((g, i) => (
            <ServiceCard key={g.title} icon={IDENT_ICONS[i % IDENT_ICONS.length]} title={g.title} accent={ACCENT_CYCLE[i % ACCENT_CYCLE.length]}>
              {g.rows.map(([k, v]) => (
                <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
              ))}
            </ServiceCard>
          ))}
          {otherRows.length > 0 ? (
            <ServiceCard icon={FileText} title="Outros" accent={ACCENT_CYCLE[grouped.length % ACCENT_CYCLE.length]}>
              {otherRows.map(([k, v]) => (
                <DetailScalar key={k} label={formatFieldLabel(k)} value={v} />
              ))}
            </ServiceCard>
          ) : null}
          {objectSections.map(({ key, data }, i) => (
            <DetailObjectBlock key={key} title={formatFieldLabel(key)} data={data} accentIndex={grouped.length + 1 + i} />
          ))}
        </div>
      </div>
    ),
  });

  for (const { key, items } of arraySections) {
    if (items.length === 0) continue;
    // "servicos" ganhou aba dedicada própria (ServicesTabContent, cartões tipados) — ver
    // ClientDetailModal, injectada directamente ali a partir de client.services, não daqui.
    if (key === "servicos") continue;
    tabs.push({
      id: `array:${key}`,
      label: formatFieldLabel(key),
      content: (
        <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
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

function StatTile({
  icon: Icon,
  label,
  value,
  sub,
  accent,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  sub?: string;
  accent: ServiceCardAccent;
}) {
  return (
    <div className={`hubsoft-stat-tile hubsoft-stat-tile--${accent}`}>
      <span className="hubsoft-stat-tile__icon">
        <Icon size={16} />
      </span>
      <div className="hubsoft-stat-tile__body">
        <span className="hubsoft-stat-tile__label">{label}</span>
        <span className="hubsoft-stat-tile__value">{value}</span>
        {sub ? <span className="hubsoft-stat-tile__sub">{sub}</span> : null}
      </div>
    </div>
  );
}

function FinancialSummaryPanel({ summary }: { summary: FinancialSummary }) {
  return (
    <div className="hubsoft-stat-grid">
      <StatTile icon={Receipt} label="Total de faturas" value={String(summary.total)} accent="conexao" />
      <StatTile icon={Wallet} label="Valor total" value={formatCurrencyBRL(summary.total_value)} accent="cadastro" />
      <StatTile icon={AlertTriangle} label="Vencidas" value={String(summary.overdue_count)} sub={formatCurrencyBRL(summary.overdue_value)} accent="historico" />
      <StatTile icon={Clock} label="Pendentes" value={String(summary.pending_count)} sub={formatCurrencyBRL(summary.pending_value)} accent="vendedor" />
      <StatTile icon={CheckCircle2} label="Pagas" value={String(summary.paid_count)} sub={formatCurrencyBRL(summary.paid_value)} accent="endereco" />
    </div>
  );
}

export function FinancialTabContent({
  loading,
  ok,
  message,
  invoices,
  summary,
  onDownloadSelected,
}: {
  loading: boolean;
  ok: boolean;
  message?: string;
  invoices: InvoiceItem[];
  summary?: FinancialSummary;
  // Ausente (ex.: aba Relatório) esconde a coluna de checkbox e o botão — mesmo padrão opt-in de
  // onFetchLogins etc. acima.
  onDownloadSelected?: (invoiceIds: string[]) => Promise<void>;
}) {
  const downloadable = useMemo(() => invoices.filter((inv) => !!inv.id && !!inv.boleto_link?.trim()), [invoices]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState("");

  // Faturas mudam (troca de cliente, novo fetch) — não manter uma selecção de outra pessoa.
  useEffect(() => {
    setSelected(new Set());
    setDownloadError("");
  }, [invoices]);

  if (loading) {
    return <p className="integration-detail__empty">A carregar faturas…</p>;
  }
  if (!ok && message) {
    return <div className="msg msg--err">{message}</div>;
  }
  if (invoices.length === 0) {
    return <div className="msg">{message || "Nenhuma fatura encontrada."}</div>;
  }

  const allSelected = downloadable.length > 0 && downloadable.every((inv) => selected.has(inv.id!));
  function toggleAll() {
    setSelected(allSelected ? new Set() : new Set(downloadable.map((inv) => inv.id!)));
  }
  function toggleOne(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  async function download() {
    if (!onDownloadSelected || selected.size === 0) return;
    setDownloading(true);
    setDownloadError("");
    try {
      await onDownloadSelected([...selected]);
    } catch (e) {
      setDownloadError(e instanceof Error ? e.message : String(e));
    } finally {
      setDownloading(false);
    }
  }

  return (
    <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
      {summary ? <FinancialSummaryPanel summary={summary} /> : null}
      <section className="integration-detail__section hubsoft-generic-card">
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
          <h4 className="integration-detail__section-title" style={{ margin: 0 }}>
            <Receipt size={13} /> Faturas <span className="integration-detail__count">({invoices.length})</span>
          </h4>
          {onDownloadSelected ? (
            <button
              type="button"
              className="btn btn--sm"
              disabled={selected.size === 0 || downloading}
              onClick={() => void download()}
            >
              {downloading ? "A gerar PDF…" : `Baixar selecionados (${selected.size})`}
            </button>
          ) : null}
        </div>
        {downloadError ? <div className="msg msg--err" style={{ marginTop: 6 }}>{downloadError}</div> : null}
        <div className="table-wrap integration-support-table">
          <table className="integration-support-table__grid">
            <thead>
              <tr>
                {onDownloadSelected ? (
                  <th style={{ width: 28 }}>
                    <input
                      type="checkbox"
                      checked={allSelected}
                      disabled={downloadable.length === 0}
                      onChange={toggleAll}
                      aria-label="Selecionar todos os boletos"
                    />
                  </th>
                ) : null}
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
                  {onDownloadSelected ? (
                    <td className="integration-support-table__cell">
                      {inv.id && inv.boleto_link ? (
                        <input
                          type="checkbox"
                          checked={selected.has(inv.id)}
                          onChange={() => toggleOne(inv.id!)}
                          aria-label={`Selecionar boleto de ${inv.due_date || inv.id}`}
                        />
                      ) : null}
                    </td>
                  ) : null}
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
    <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
      <section className="integration-detail__section hubsoft-generic-card">
        <h4 className="integration-detail__section-title">
          <Headset size={13} /> Atendimentos <span className="integration-detail__count">({items.length})</span>
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
    <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
      <section className="integration-detail__section hubsoft-generic-card">
        <h4 className="integration-detail__section-title">
          <ClipboardList size={13} /> Ordens de serviço <span className="integration-detail__count">({items.length})</span>
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
                <th>Fechada por</th>
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
                  <td className="integration-support-table__cell">{o.closed_by_user || "—"}</td>
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
    <div className="integration-detail hubsoft-tab-body" style={{ fontSize: DETAIL_FONT }}>
      <section className="integration-detail__section hubsoft-generic-card">
        <h4 className="integration-detail__section-title">
          <LogIn size={13} /> Logins <span className="integration-detail__count">({items.length})</span>
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
  onDownloadBoletos,
  onEnableService,
  onSuspendService,
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
  onDownloadBoletos?: (client: ClientCard, invoiceIds: string[]) => Promise<void>;
  // Só a HubSoft tem estas duas acções (ver docs.hubsoft.com.br > Clientes > Cliente Serviço).
  onEnableService?: (client: ClientCard, service: ClientServiceSummary, motivo: string) => Promise<ServiceActionResult>;
  onSuspendService?: (
    client: ClientCard,
    service: ClientServiceSummary,
    tipo: "suspenso_debito" | "suspenso_pedido_cliente",
  ) => Promise<ServiceActionResult>;
  attendanceEnabled?: boolean;
  workOrderEnabled?: boolean;
  loginEnabled?: boolean;
  // Busca atendimentos/ordens de serviço em paralelo assim que o modal abre, em vez de só ao
  // clicar na aba — reduz a espera percebida ao trocar de aba. Opt-in (default false) para não
  // mudar o comportamento do IXC, que continua a buscar só ao clicar na aba.
  prefetchExtras?: boolean;
}) {
  const detailTabs = useMemo(() => buildDetailTabs(client.raw), [client.raw]);
  const services = client.services ?? [];
  const tabs = useMemo(() => {
    // "identificacao" é sempre a primeira (buildDetailTabs) — "Serviços" entra logo a seguir,
    // antes de Financeiro/Atendimentos/Ordens, mesma ordem do painel da própria HubSoft.
    const [identificacao, ...rest] = detailTabs;
    const withServices: DetailTabDef[] = identificacao ? [identificacao] : [];
    if (services.length > 0) {
      withServices.push({ id: "servicos", label: `Serviços (${services.length})`, content: null });
    }
    withServices.push(...rest);
    const extra: DetailTabDef[] = [];
    if (onFetchFinancial) extra.push({ id: "financeiro", label: "Financeiro", content: null });
    if (onFetchAttendance && attendanceEnabled !== false) extra.push({ id: "atendimentos", label: "Atendimentos", content: null });
    if (onFetchWorkOrders && workOrderEnabled !== false) extra.push({ id: "ordens", label: "Ordens de serviço", content: null });
    if (onFetchLogins && loginEnabled !== false) extra.push({ id: "logins", label: "Logins", content: null });
    return [...withServices, ...extra];
  }, [detailTabs, services.length, onFetchFinancial, onFetchAttendance, onFetchWorkOrders, onFetchLogins, attendanceEnabled, workOrderEnabled, loginEnabled]);

  const [activeTab, setActiveTab] = useState(tabs[0]?.id ?? "identificacao");
  const [serviceAction, setServiceAction] = useState<ServiceActionTarget | null>(null);
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
    setServiceAction(null);
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
  if (activeTab === "servicos") {
    activeContent = (
      <ServicesTabContent
        services={services}
        onEnableClick={onEnableService ? (s) => setServiceAction({ client, service: s, mode: "enable" }) : undefined}
        onSuspendClick={onSuspendService ? (s) => setServiceAction({ client, service: s, mode: "suspend" }) : undefined}
      />
    );
  } else if (activeTab === "financeiro") {
    activeContent = (
      <FinancialTabContent
        loading={financialLoading}
        ok={financial?.ok ?? true}
        message={financial?.message}
        invoices={financial?.invoices ?? []}
        summary={financial?.summary}
        onDownloadSelected={onDownloadBoletos ? (ids) => onDownloadBoletos(client, ids) : undefined}
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
    <div className="modal-backdrop hubsoft-detail-backdrop" role="presentation" onMouseDown={onClose}>
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
              {client.inactive ? <InactiveClientMark /> : null}
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
      {serviceAction ? (
        <ServiceActionModal
          target={serviceAction}
          onCancel={() => setServiceAction(null)}
          onConfirm={(value) => {
            if (serviceAction.mode === "enable") {
              if (!onEnableService) return Promise.resolve({ ok: false, message: "Ação indisponível." });
              return onEnableService(serviceAction.client, serviceAction.service, value);
            }
            if (!onSuspendService) return Promise.resolve({ ok: false, message: "Ação indisponível." });
            return onSuspendService(
              serviceAction.client,
              serviceAction.service,
              value as "suspenso_debito" | "suspenso_pedido_cliente",
            );
          }}
        />
      ) : null}
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
  onEnableService,
  onSuspendService,
  onDownloadBoletos,
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
  onEnableService?: (client: ClientCard, service: ClientServiceSummary, motivo: string) => Promise<ServiceActionResult>;
  onSuspendService?: (
    client: ClientCard,
    service: ClientServiceSummary,
    tipo: "suspenso_debito" | "suspenso_pedido_cliente",
  ) => Promise<ServiceActionResult>;
  onDownloadBoletos?: (client: ClientCard, invoiceIds: string[]) => Promise<void>;
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
                  {c.inactive ? <InactiveClientMark /> : null}
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
          onDownloadBoletos={onDownloadBoletos}
          onEnableService={onEnableService}
          onSuspendService={onSuspendService}
          attendanceEnabled={attendanceEnabled}
          workOrderEnabled={workOrderEnabled}
          loginEnabled={loginEnabled}
          prefetchExtras={prefetchExtras}
        />
      ) : null}
    </>
  );
}
