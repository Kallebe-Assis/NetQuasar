import { useMemo, useState } from "react";
import { Eye, X } from "lucide-react";
import type { ClientCard } from "./types";

const DETAIL_FONT = "var(--integration-detail-font-size, 11px)";

function labelStatus(s?: string) {
  if (!s) return null;
  const low = s.toLowerCase();
  if (low.includes("habilit") || low === "ativo") return "badge badge--ok";
  if (low.includes("suspen") || low.includes("debito")) return "badge badge--err";
  if (low.includes("cancel")) return "badge badge--off";
  return "badge";
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
        <FieldInline label="Código" value={c.code} mono />
        <FieldInline label="CPF/CNPJ" value={c.document} mono />
        <FieldInline label="IPv4" value={c.ipv4} mono />
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
          {c.services.slice(0, 4).map((s, si) => (
            <div key={s.id ?? si} className="integration-consult-card__service">
              <span className="integration-consult-card__value">{s.name || s.login || "Serviço"}</span>
              {s.ipv4 && !c.ipv4?.includes(s.ipv4) ? (
                <span className="mono integration-consult-card__label">{s.ipv4}</span>
              ) : null}
              {s.login ? <span className="mono integration-consult-card__label">{s.login}</span> : null}
              {s.status ? <span className={labelStatus(s.status) ?? "badge"}>{s.status}</span> : null}
            </div>
          ))}
          {c.services.length > 4 ? (
            <span className="integration-consult-card__label">+{c.services.length - 4} serviço(s)</span>
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
  return (
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
    </div>
  );
}

export function HubsoftClientResults({
  clients,
  message,
  ok,
  localFilter,
  onFetchDetail,
}: {
  clients: ClientCard[];
  message?: string;
  ok: boolean;
  localFilter: string;
  onFetchDetail?: (client: ClientCard) => Promise<ClientCard>;
}) {
  const [detailClient, setDetailClient] = useState<ClientCard | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

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
                <button
                  type="button"
                  className="btn integration-consult-card__detail-btn"
                  title="Ver detalhes completos"
                  aria-label="Ver detalhes completos"
                  onClick={() => openDetail(c)}
                >
                  <Eye size={15} />
                </button>
              </div>
            </div>
            <ClientCardSummary c={c} />
          </article>
        ))}
      </div>
      {detailClient ? (
        <ClientDetailModal client={detailClient} loading={detailLoading} onClose={() => setDetailClient(null)} />
      ) : null}
    </>
  );
}
