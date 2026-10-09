import { useEffect, useMemo, useState } from "react";
import { Handshake, Search } from "lucide-react";
import { Callout, Pill, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";

const BASE = "/api/v1/integrations/hubsoft/hubsoft/stock-comodato";

/**
 * Comodato de UM patrimônio para o serviço de um cliente (passo 3 da migração). Na HubSoft o comodato é uma saída de estoque
 * para o serviço: o patrimônio sai do local e fica vinculado ao serviço com o status do «tipo de movimento». Fluxo: achar o
 * serviço → achar o patrimônio → tipo de movimento (só os que geram «Comodato») → conferir → confirmar. Depois de enviar, o
 * patrimônio é relido (status Comodato + serviço + vínculo).
 */

type Svc = { id_cliente: string; cliente: string; codigo_cliente?: string; id_cliente_servico: string; login?: string; plano?: string; status?: string };
type Link = { id_produto: string; id_produto_item?: string; status: string; identificador_proprio?: string; numero_serie?: string; mac_address?: string; codigo_item?: string; quantidade?: string };
type SvcItem = { service: Svc; links: Link[] | null; links_error?: string };
type Item = {
  id_produto_item: string;
  id_produto: string;
  produto: string;
  id_local_estoque: string;
  local: string;
  status: string;
  status_prefixo: string;
  identificador_proprio?: string;
  numero_serie?: string;
  mac_address?: string;
  codigo_item?: string;
  cliente?: string;
};
type MType = { id: string; nome: string; prefixo?: string; status_prefixo: string; status_nome: string; usos: number };
type Preview = { ok: boolean; already_done?: boolean; problems?: string[]; warnings?: string[]; service?: Svc; item?: { ID?: string }; produto?: string; links?: Link[]; tipo?: MType; id_local_estoque?: string };
type Result = {
  ok: boolean;
  action: "created" | "already_done" | "blocked" | "failed" | "verify_failed";
  message: string;
  id_movimento_estoque?: string;
  id_produto_item?: string;
  status_depois?: string;
  verified?: boolean;
  verify_message?: string;
  detail?: string;
  preview: Preview;
};

const CAMPOS: { value: string; label: string }[] = [
  { value: "identificador_proprio", label: "Identificador próprio" },
  { value: "numero_serie", label: "Número de série" },
  { value: "mac_address", label: "MAC" },
  { value: "codigo_item", label: "Código do item" },
  { value: "id_produto_item", label: "id_produto_item" },
];

const STATUS_TONE = (p: string): "ok" | "warn" | "err" | undefined => (p === "estoque" ? "ok" : p === "comodato" ? "warn" : "err");

function LinksTable({ links }: { links: Link[] }) {
  if (links.length === 0) return <span className="hsa-muted">Nenhum produto/patrimônio vinculado a este serviço.</span>;
  return (
    <div className="hsa-table-wrap" style={{ maxHeight: 220 }}>
      <table className="hsa-table">
        <thead>
          <tr>
            <th>Patrimônio</th>
            <th>Produto</th>
            <th>Status</th>
            <th>Identificador / série / MAC</th>
          </tr>
        </thead>
        <tbody>
          {links.map((l, i) => (
            <tr key={`${l.id_produto_item ?? l.id_produto}-${i}`}>
              <td className="mono">{l.id_produto_item || "—"}{l.codigo_item ? <div className="hsa-muted">cód. {l.codigo_item}</div> : null}</td>
              <td className="mono">{l.id_produto}</td>
              <td>{l.status}</td>
              <td className="mono">{[l.identificador_proprio, l.numero_serie, l.mac_address].filter(Boolean).join(" · ") || "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function HubsoftStockComodato() {
  const { notify } = useConsultaToast();
  // 1) serviço
  const [termo, setTermo] = useState("");
  const [searchingSvc, setSearchingSvc] = useState(false);
  const [svcMsg, setSvcMsg] = useState("");
  const [svcs, setSvcs] = useState<SvcItem[]>([]);
  const [svcId, setSvcId] = useState("");
  // 2) patrimônio
  const [campo, setCampo] = useState("identificador_proprio");
  const [valor, setValor] = useState("");
  const [searchingItem, setSearchingItem] = useState(false);
  const [itemMsg, setItemMsg] = useState("");
  const [item, setItem] = useState<Item | null>(null);
  // 3) tipo de movimento
  const [types, setTypes] = useState<MType[]>([]);
  const [typesMsg, setTypesMsg] = useState("");
  const [typesLoading, setTypesLoading] = useState(true);
  const [tipoId, setTipoId] = useState("");
  const [tipoManual, setTipoManual] = useState("");
  const [tipoConfirm, setTipoConfirm] = useState(false);
  const [obs, setObs] = useState("Comodato — migração do IXC");
  // 4/5) conferência e execução
  const [previewing, setPreviewing] = useState(false);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState("");
  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [result, setResult] = useState<Result | null>(null);

  const comodatoTypes = useMemo(() => types.filter((t) => t.status_prefixo.toLowerCase() === "comodato"), [types]);
  const otherTypes = useMemo(() => types.filter((t) => t.status_prefixo.toLowerCase() !== "comodato"), [types]);
  const usingManual = comodatoTypes.length === 0 || tipoId === "__manual";
  const effTipo = usingManual ? tipoManual.trim() : tipoId;

  useEffect(() => {
    let alive = true;
    apiFetch<{ ok: boolean; message?: string; types: MType[] }>(`${BASE}/movement-types`, { timeoutMs: 5 * 60_000 })
      .then((d) => {
        if (!alive) return;
        setTypes(d.types ?? []);
        if (!d.ok) setTypesMsg(d.message ?? "Não foi possível descobrir os tipos de movimento.");
        const first = (d.types ?? []).find((t) => t.status_prefixo.toLowerCase() === "comodato");
        if (first) setTipoId(first.id);
      })
      .catch((e) => alive && setTypesMsg((e as Error).message))
      .finally(() => alive && setTypesLoading(false));
    return () => {
      alive = false;
    };
  }, []);

  function resetResult() {
    setPreview(null);
    setPreviewErr("");
    setResult(null);
    setAck(false);
    setTyped("");
  }

  async function findService() {
    setSearchingSvc(true);
    setSvcMsg("");
    setSvcs([]);
    setSvcId("");
    resetResult();
    try {
      const r = await apiFetch<{ ok: boolean; message?: string; services: SvcItem[] }>(`${BASE}/service?termo=${encodeURIComponent(termo.trim())}`, { timeoutMs: 90_000 });
      if (!r.ok) setSvcMsg(r.message ?? "Não foi possível procurar.");
      else if (r.services.length === 0) setSvcMsg("Nenhum serviço encontrado com esse id ou login.");
      else {
        setSvcs(r.services);
        if (r.services.length === 1) setSvcId(r.services[0].service.id_cliente_servico);
      }
    } catch (e) {
      setSvcMsg((e as Error).message);
    } finally {
      setSearchingSvc(false);
    }
  }

  async function findItem() {
    setSearchingItem(true);
    setItemMsg("");
    setItem(null);
    resetResult();
    try {
      const r = await apiFetch<{ ok: boolean; message?: string; item?: Item }>(`${BASE}/item?campo=${encodeURIComponent(campo)}&valor=${encodeURIComponent(valor.trim())}`, { timeoutMs: 90_000 });
      if (!r.ok || !r.item) setItemMsg(r.message ?? "Patrimônio não encontrado.");
      else setItem(r.item);
    } catch (e) {
      setItemMsg((e as Error).message);
    } finally {
      setSearchingItem(false);
    }
  }

  const body = () => ({
    id_cliente_servico: svcId,
    campo: "id_produto_item",
    valor: item?.id_produto_item ?? "",
    id_tipo_movimento_estoque: effTipo,
    observacao: obs,
    tipo_confirmado_comodato: usingManual && tipoConfirm,
  });
  const ready = !!svcId && !!item && !!effTipo;

  async function runPreview() {
    setPreviewing(true);
    resetResult();
    try {
      setPreview(await apiFetch<Preview>(`${BASE}/preview`, { method: "POST", json: body(), timeoutMs: 5 * 60_000 }));
    } catch (e) {
      setPreviewErr((e as Error).message);
    } finally {
      setPreviewing(false);
    }
  }

  const selSvc = svcs.find((s) => s.service.id_cliente_servico === svcId)?.service;
  const confirmText = "COMODATO";
  const canApply = !!preview?.ok && !preview.already_done && ack && typed.trim() === confirmText && !applying && !result;

  async function runApply() {
    setApplying(true);
    try {
      const r = await apiFetch<Result>(`${BASE}/apply`, { method: "POST", json: body(), timeoutMs: 5 * 60_000 });
      setResult(r);
      notify({ ok: r.ok }, r.ok ? null : new Error(r.message), r.message);
    } catch (e) {
      setResult({ ok: false, action: "failed", message: (e as Error).message, preview: preview ?? { ok: false } });
    } finally {
      setApplying(false);
    }
  }

  return (
    <ToolPanel
      icon={<Handshake size={20} />}
      title="Comodato de um patrimônio para o cliente"
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Liga UM patrimônio que está em estoque ao serviço de um cliente, como comodato (saída de estoque para o serviço). Tudo é conferido antes e o patrimônio é relido depois."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <Callout tone="info">
          <strong>Segurança:</strong> só aceita um patrimônio com status <b>Estoque</b>, um serviço que existe e não está cancelado, e um tipo de movimento que gere <b>Comodato</b> (um tipo de «venda»
          marcaria o equipamento como vendido). Se algo falhar, a mensagem e a resposta bruta da HubSoft aparecem aqui e ficam no histórico.
        </Callout>
      </div>

      <Step n={1} title="Serviço do cliente" done={!!svcId} hint="Procure pelo id do serviço (id_cliente_servico) ou pelo login PPPoE.">
        <div className="hsa-actions">
          <input className="input" style={{ width: 280 }} placeholder="id do serviço ou login PPPoE" value={termo} onChange={(e) => setTermo(e.target.value)} onKeyDown={(e) => e.key === "Enter" && termo.trim() && void findService()} />
          <button type="button" className="btn btn--primary" disabled={searchingSvc || !termo.trim()} onClick={() => void findService()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {searchingSvc ? "Procurando…" : "Procurar serviço"}
          </button>
        </div>
        {svcMsg ? <Callout tone="warn">{svcMsg}</Callout> : null}
        {svcs.length > 0 ? (
          <div style={{ display: "grid", gap: 8 }}>
            {svcs.map(({ service: sv, links, links_error }) => (
              <label key={sv.id_cliente_servico} className="card" style={{ padding: 10, display: "grid", gap: 6, cursor: "pointer", outline: svcId === sv.id_cliente_servico ? "2px solid var(--accent)" : undefined }}>
                <span>
                  <input type="radio" name="svc" checked={svcId === sv.id_cliente_servico} onChange={() => { setSvcId(sv.id_cliente_servico); resetResult(); }} />{" "}
                  <b>{sv.cliente}</b> <span className="hsa-muted">(id_cliente {sv.id_cliente}{sv.codigo_cliente ? ` · código ${sv.codigo_cliente}` : ""})</span>
                </span>
                <span className="mono">serviço {sv.id_cliente_servico} · {sv.login || "sem login"} · {sv.plano || "—"} · {sv.status || "—"}</span>
                {links_error ? <span className="hsa-muted" style={{ color: "var(--warn)" }}>{links_error}</span> : null}
                {svcId === sv.id_cliente_servico && links ? (
                  <div>
                    <div className="hsa-muted" style={{ marginBottom: 4 }}>Já vinculado a este serviço:</div>
                    <LinksTable links={links} />
                  </div>
                ) : null}
              </label>
            ))}
          </div>
        ) : null}
      </Step>

      <Step n={2} title="Patrimônio" done={!!item} hint="Procure pelo identificador, série, MAC ou código do item. Precisa estar em estoque.">
        <div className="hsa-actions">
          <select className="input" value={campo} onChange={(e) => setCampo(e.target.value)}>
            {CAMPOS.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
          <input className="input" style={{ width: 280 }} placeholder="valor" value={valor} onChange={(e) => setValor(e.target.value)} onKeyDown={(e) => e.key === "Enter" && valor.trim() && void findItem()} />
          <button type="button" className="btn btn--primary" disabled={searchingItem || !valor.trim()} onClick={() => void findItem()}>
            <Search size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {searchingItem ? "Procurando…" : "Procurar patrimônio"}
          </button>
        </div>
        {itemMsg ? <Callout tone="warn">{itemMsg}</Callout> : null}
        {item ? (
          <div className="card" style={{ padding: 10, display: "grid", gap: 4 }}>
            <div>
              <b>{item.produto}</b> <Pill tone={STATUS_TONE(item.status_prefixo)}>{item.status}</Pill>
              {item.cliente ? <span className="hsa-muted"> · com {item.cliente}</span> : null}
            </div>
            <div className="mono">id_produto_item {item.id_produto_item} · código {item.codigo_item || "—"} · identificador {item.identificador_proprio || "—"}</div>
            <div className="mono">série {item.numero_serie || "—"} · MAC {item.mac_address || "—"}</div>
            <div className="hsa-muted">Local: {item.local} (id {item.id_local_estoque})</div>
          </div>
        ) : null}
      </Step>

      <Step n={3} title="Tipo de movimento e observação" done={!!effTipo} hint="Só os tipos que geram o status «Comodato» são oferecidos. Eles são descobertos nos movimentos de saída para clientes que já existem na HubSoft.">
        {typesLoading ? <ConsultaLoading text="Descobrindo os tipos de movimento (lê os movimentos de estoque da HubSoft)…" /> : null}
        {typesMsg ? <Callout tone="warn">{typesMsg}</Callout> : null}
        {!typesLoading && comodatoTypes.length > 0 ? (
          <div className="hsa-actions">
            <label className="hsa-check">
              Tipo de movimento
              <select className="input" style={{ marginLeft: 8, minWidth: 320 }} value={tipoId} onChange={(e) => { setTipoId(e.target.value); resetResult(); }}>
                {comodatoTypes.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.id} — {t.nome} (gera {t.status_nome}; usado {t.usos}×)
                  </option>
                ))}
                <option value="__manual">Informar outro id manualmente…</option>
              </select>
            </label>
          </div>
        ) : null}
        {!typesLoading && comodatoTypes.length === 0 ? (
          <Callout tone="warn">
            Nenhum tipo de movimento com status <b>Comodato</b> foi encontrado nos movimentos existentes
            {otherTypes.length ? ` (encontrados: ${otherTypes.map((t) => `${t.id} ${t.nome} → ${t.status_nome}`).join("; ")}, que NÃO geram comodato)` : ""}. Informe o id do tipo de comodato abaixo e confirme.
          </Callout>
        ) : null}
        {!typesLoading && usingManual ? (
          <div className="hsa-actions" style={{ flexWrap: "wrap" }}>
            <input className="input" style={{ width: 200 }} placeholder="id do tipo de movimento" value={tipoManual} onChange={(e) => { setTipoManual(e.target.value); resetResult(); }} />
            <label className="hsa-check">
              <input type="checkbox" checked={tipoConfirm} onChange={(e) => { setTipoConfirm(e.target.checked); resetResult(); }} />
              Confirmo que este tipo de movimento gera o status <b>Comodato</b> (será conferido depois)
            </label>
          </div>
        ) : null}
        <div className="field" style={{ marginBottom: 0 }}>
          <label>Observação do movimento</label>
          <input className="input" value={obs} onChange={(e) => { setObs(e.target.value); resetResult(); }} />
        </div>
      </Step>

      <Step n={4} title="Conferir" done={!!preview?.ok} hint="Somente leitura: confere patrimônio, serviço, produto e tipo de movimento e mostra o que o serviço já tem.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={!ready || previewing || applying} onClick={() => void runPreview()}>
            {previewing ? "Conferindo…" : "Conferir o comodato"}
          </button>
          {!ready ? <span className="hsa-muted">Escolha o serviço, o patrimônio e o tipo de movimento.</span> : null}
        </div>
        {previewing ? <ConsultaLoading text="Conferindo com a HubSoft…" /> : null}
        {previewErr ? <Callout tone="err">{previewErr}</Callout> : null}
        {preview ? (
          <>
            {(preview.problems ?? []).map((p) => (
              <Callout key={p} tone="err">{p}</Callout>
            ))}
            {(preview.warnings ?? []).map((w) => (
              <Callout key={w} tone="warn">{w}</Callout>
            ))}
            {preview.ok && preview.already_done ? (
              <Callout tone="info">
                <b>Nada a fazer:</b> este patrimônio já está ligado a este serviço — nada será enviado.
              </Callout>
            ) : null}
            {preview.ok && !preview.already_done ? (
              <Callout tone="info">
                <b>Pronto para ligar:</b> «{preview.produto || item?.produto}» (patrimônio {item?.id_produto_item}) → serviço {selSvc?.id_cliente_servico} ({selSvc?.cliente} · {selSvc?.login}), tipo de movimento {effTipo}
                {preview.tipo ? ` — ${preview.tipo.nome}, gera ${preview.tipo.status_nome}` : ""}, saindo do local {item?.local}.
              </Callout>
            ) : null}
          </>
        ) : null}
      </Step>

      {preview?.ok && !preview.already_done ? (
        <Step n={5} title="Confirmar" done={!!result?.ok} hint="Esta etapa altera a HubSoft: o patrimônio sai do estoque e fica vinculado ao serviço.">
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a conferência e entendo que isto liga o patrimônio ao cliente de verdade.
          </label>
          <div className="hsa-actions">
            <input className="input" style={{ width: 210 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
            <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void runApply()}>
              {applying ? "Ligando…" : "Ligar como comodato"}
            </button>
          </div>
          <span className="hsa-muted">Digite exatamente “{confirmText}” para liberar o botão.</span>
          {applying ? <ConsultaLoading text="Enviando a saída de estoque e conferindo…" /> : null}
        </Step>
      ) : null}

      {result ? (
        <Step n={6} title="Resultado" done={result.ok}>
          <Callout tone={result.ok ? "info" : "err"}>
            <b>{result.action === "already_done" ? "Nada a fazer — o patrimônio já está neste serviço." : result.ok ? "Comodato registrado e conferido." : result.action === "blocked" ? "Nada foi enviado." : result.action === "verify_failed" ? "Enviado, mas a conferência encontrou diferença." : "A HubSoft recusou."}</b>{" "}
            {result.message}
          </Callout>
          {result.id_movimento_estoque ? <div className="mono">movimento de estoque {result.id_movimento_estoque} · patrimônio {result.id_produto_item} · status depois: {result.status_depois || "—"}</div> : null}
          {result.verify_message ? <div className="hsa-muted" style={{ color: result.verified ? "var(--ok)" : "var(--err)" }}>Conferência: {result.verify_message}</div> : null}
          {result.detail ? (
            <details>
              <summary style={{ cursor: "pointer", fontSize: 11 }}>Resposta bruta da HubSoft</summary>
              <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>{result.detail}</pre>
            </details>
          ) : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
