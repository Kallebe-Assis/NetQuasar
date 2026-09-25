import { useMemo } from "react";
import { createPortal } from "react-dom";
import { Download, X } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { fmtConsultaAt, useConsulta } from "./hubsoftConsulta";

import { ConsultaLoading } from "./ConsultaLoading";
const SLUG = "hubsoft";
const TOP_SHOWN = 15;
const TOP_EXPORT = 25;

type Item = { name: string; count: number };
type DetailResp = {
  ok: boolean;
  message?: string;
  from: string;
  to: string;
  services: number;
  clients: number;
  by_city: Item[];
  by_plan: Item[];
  truncated?: boolean;
};

export type TenureBand = { name: string; from: string; to: string };

const fmtInt = (n: number) => n.toLocaleString("pt-BR");

/** "De 6 meses a 1 ano" → "6 meses a 1 ano"; "Até 6 meses" → "até 6 meses"; "Mais de 10 anos" → "mais de 10 anos". */
function bandPhrase(name: string): string {
  if (name.startsWith("De ")) return name.slice(3);
  return name.charAt(0).toLowerCase() + name.slice(1);
}

/** Top N + "Outros (k)" somando o resto. */
function topWithOthers(items: Item[], n: number): Item[] {
  if (items.length <= n) return items;
  const rest = items.slice(n);
  return [...items.slice(0, n), { name: `Outros (${rest.length})`, count: rest.reduce((a, b) => a + b.count, 0) }];
}

function fmtBR(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  return m ? `${m[3]}/${m[2]}/${m[1]}` : iso;
}

function slug(s: string): string {
  return s
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-zA-Z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .toLowerCase();
}

/** Desenha o gráfico num canvas (fundo claro fixo, para ficar legível em qualquer lugar) e baixa como PNG. */
function downloadChartPng(title: string, subtitle: string, items: Item[]): Promise<void> {
  const W = 1200;
  const rowH = 34;
  const top = 110;
  const left = 380;
  const right = 110;
  const H = top + items.length * rowH + 60;
  const scale = 2; // nítido em telas de alta densidade
  const canvas = document.createElement("canvas");
  canvas.width = W * scale;
  canvas.height = H * scale;
  const ctx = canvas.getContext("2d");
  if (!ctx) return Promise.reject(new Error("Canvas indisponível neste navegador."));
  ctx.scale(scale, scale);
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, W, H);
  ctx.fillStyle = "#111827";
  ctx.font = "bold 26px system-ui, -apple-system, Segoe UI, sans-serif";
  ctx.textBaseline = "alphabetic";
  ctx.fillText(title, 32, 46, W - 64);
  ctx.fillStyle = "#4b5563";
  ctx.font = "15px system-ui, -apple-system, Segoe UI, sans-serif";
  ctx.fillText(subtitle, 32, 74, W - 64);
  const max = Math.max(1, ...items.map((i) => i.count));
  const barW = W - left - right;
  items.forEach((it, idx) => {
    const y = top + idx * rowH;
    ctx.fillStyle = "#111827";
    ctx.font = "15px system-ui, -apple-system, Segoe UI, sans-serif";
    ctx.textAlign = "right";
    ctx.fillText(it.name, left - 14, y + 20, left - 44);
    ctx.textAlign = "left";
    ctx.fillStyle = it.name.startsWith("Outros (") ? "#9ca3af" : "#2563eb";
    const w = Math.max(2, (it.count / max) * barW);
    ctx.fillRect(left, y + 5, w, rowH - 12);
    ctx.fillStyle = "#111827";
    ctx.fillText(fmtInt(it.count), left + w + 8, y + 21);
  });
  ctx.fillStyle = "#6b7280";
  ctx.font = "12px system-ui, -apple-system, Segoe UI, sans-serif";
  ctx.textAlign = "left";
  ctx.fillText(`Gerado pelo NetQuasar em ${new Date().toLocaleString("pt-BR")}`, 32, H - 20);
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) return reject(new Error("Falha ao gerar a imagem."));
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${slug(title)}.png`;
      a.click();
      URL.revokeObjectURL(url);
      resolve();
    }, "image/png");
  });
}

function BarChart({ items }: { items: Item[] }) {
  const max = Math.max(1, ...items.map((i) => i.count));
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
      {items.map((it) => (
        <div key={it.name} style={{ display: "grid", gridTemplateColumns: "minmax(110px, 30%) 1fr 56px", gap: 8, alignItems: "center", fontSize: 12 }}>
          <span title={it.name} style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", textAlign: "right" }}>
            {it.name}
          </span>
          <div style={{ background: "color-mix(in srgb, var(--muted) 14%, transparent)", borderRadius: 4, height: 16 }}>
            <div
              style={{
                width: `${(it.count / max) * 100}%`,
                minWidth: 2,
                height: "100%",
                borderRadius: 4,
                background: it.name.startsWith("Outros (") ? "var(--muted)" : "var(--accent)",
              }}
            />
          </div>
          <span className="mono" style={{ textAlign: "right" }}>{fmtInt(it.count)}</span>
        </div>
      ))}
    </div>
  );
}

function ChartCard({ title, subtitle, items }: { title: string; subtitle: string; items: Item[] }) {
  const toast = useAppToast();
  const shown = useMemo(() => topWithOthers(items, TOP_SHOWN), [items]);
  async function save() {
    try {
      await downloadChartPng(title, subtitle, topWithOthers(items, TOP_EXPORT));
      toast.push({ tone: "ok", text: "Gráfico salvo em PNG." });
    } catch (e) {
      toast.push({ tone: "err", text: `Erro ao salvar o gráfico: ${e instanceof Error ? e.message : String(e)}` });
    }
  }
  return (
    <div className="card" style={{ padding: 14 }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, flexWrap: "wrap", marginBottom: 10 }}>
        <div style={{ minWidth: 0 }}>
          <h4 style={{ margin: 0, fontSize: 13 }}>{title}</h4>
          <p style={{ margin: "2px 0 0", fontSize: 11, color: "var(--muted)" }}>{subtitle}</p>
        </div>
        <button type="button" className="btn btn--sm" disabled={items.length === 0} onClick={() => void save()}>
          <Download size={12} style={{ marginRight: 4, verticalAlign: -2 }} />
          Salvar gráfico (PNG)
        </button>
      </div>
      {items.length === 0 ? <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Sem dados.</p> : <BarChart items={shown} />}
      {items.length > TOP_SHOWN ? (
        <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
          Mostrando os {TOP_SHOWN} maiores de {items.length}; o PNG inclui os {Math.min(TOP_EXPORT, items.length)} maiores.
        </p>
      ) : null}
    </div>
  );
}

/** Modal de uma faixa de "Tempo de cliente": serviços ativos da faixa por localidade e por plano. */
export function HubsoftTenureBandModal({ band, onClose }: { band: TenureBand; onClose: () => void }) {
  const q = useConsulta<true, DetailResp>(
    `hubsoft-tenure-detail:${band.from}:${band.to}`,
    () => apiFetch<DetailResp>(`/api/v1/integrations/${SLUG}/hubsoft/report/tenure/detail?from=${band.from}&to=${band.to}`, { timeoutMs: 4 * 60_000 }),
    { persist: true },
  );
  const d = q.data;
  const phrase = bandPhrase(band.name);
  const base = `NetQuasar - Serviços ativos com ${phrase} de cliente agrupados por`;
  const subtitle = d?.ok
    ? `${fmtInt(d.services)} serviço(s) ativo(s) · ${fmtInt(d.clients)} cliente(s) · data de venda de ${fmtBR(band.from)} a ${fmtBR(band.to)}`
    : `Data de venda de ${fmtBR(band.from)} a ${fmtBR(band.to)}`;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 900, width: "100%", maxHeight: "90vh", overflow: "auto" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 10 }}>
          <div>
            <h3 style={{ margin: 0 }}>Tempo de cliente — {band.name}</h3>
            <p style={{ margin: "2px 0 0", fontSize: 12, color: "var(--muted)" }}>{subtitle}</p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="row" style={{ gap: 10, alignItems: "center", marginBottom: 12, flexWrap: "wrap" }}>
          <button type="button" className="btn btn--sm btn--primary" disabled={q.isFetching} onClick={() => void q.run(true)}>
            {q.isFetching ? "A atualizar…" : q.consulted ? "Atualizar" : "Consultar"}
          </button>
          {q.at ? <span style={{ fontSize: 11, color: "var(--muted)" }}>Atualizado em {fmtConsultaAt(q.at)}</span> : <span style={{ fontSize: 11, color: "var(--muted)" }}>Clique em Consultar para carregar os gráficos.</span>}
        </div>

        {q.isLoading ? (
          <ConsultaLoading text="A consultar a HubSoft…" />
        ) : q.isError ? (
          <div className="msg msg--err">{(q.error as Error).message}</div>
        ) : d && !d.ok ? (
          <div className="msg msg--err">{d.message || "Falha ao consultar."}</div>
        ) : d ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {d.truncated ? (
              <p style={{ fontSize: 11, color: "var(--warn)", margin: 0 }}>Faixa maior que o teto de páginas — os gráficos podem estar incompletos.</p>
            ) : null}
            {d.services === 0 ? <div className="msg">{d.message || "Nenhum serviço ativo nesta faixa."}</div> : null}
            <ChartCard title={`${base} localidade`} subtitle={subtitle} items={d.by_city} />
            <ChartCard title={`${base} plano`} subtitle={subtitle} items={d.by_plan} />
          </div>
        ) : null}
      </div>
    </div>,
    document.body,
  );
}
