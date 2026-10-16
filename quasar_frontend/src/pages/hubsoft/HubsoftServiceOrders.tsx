import { useEffect, useMemo, useRef, useState } from "react";
import { Download, FileSpreadsheet, ListChecks, Square, Wrench } from "lucide-react";
import { Callout, CsvDropzone, Pill, ProgressBar, Segmented, Stat, Step, ToolPanel } from "./hubsoftAdminKit";
import { apiFetch } from "../../lib/api";
import { useConsultaToast } from "./hubsoftConsulta";
import { csvRowsToObjects, downloadCsv, parseCsv } from "./hubsoftCsv";
import { todayISO } from "./hubsoftDates";
import { norm } from "./hubsoftStockFields";

const BASE = "/api/v1/integrations/hubsoft/hubsoft/service-order";
const PREVIEW_BATCH = 15;
const APPLY_BATCH = 5;
const MAX_ROWS = 3000;

/**
 * Abrir ATENDIMENTO com ORDEM DE SERVIÇO na HubSoft (a API não cria O.S. avulsa: toda O.S. nasce de um atendimento).
 * Um por vez (formulário) ou em lote (CSV — inclusive a exportação de O.S. do IXC, que é reconhecida pelas colunas).
 * Fluxo: padrões + linhas → conferir (somente leitura) → confirmar digitando «ABRIR N» → enviar de 5 em 5 → a resposta da
 * HubSoft é conferida. A mesma «referência» (ex.: id da O.S. do IXC) nunca é aberta duas vezes.
 */

type Item = { id: string; nome: string; extra?: string };
type Catalogs = {
  tipos_atendimento: Item[];
  status_atendimento: Item[];
  tipos_os: Item[];
  tecnicos: Item[];
  disponibilidades: Item[];
  origens_contato: Item[];
  erros?: string[];
};
type Row = {
  linha: number;
  referencia: string;
  id_cliente_servico: string;
  login_hubsoft: string;
  cliente: string;
  login_ixc: string;
  tipo_atendimento: string;
  status_atendimento: string;
  responsaveis: string;
  origem_contato: string;
  descricao: string;
  nome: string;
  telefone: string;
  email: string;
  abrir_os: string;
  tipo_os: string;
  status_os: string;
  tecnicos: string;
  disponibilidade: string;
  agenda: string;
  parametros: string;
};
type Svc = { id_cliente_servico: string; cliente: string; login?: string; plano?: string; status?: string; codigo_cliente?: string };
type Prev = { linha: number; ok: boolean; already_done?: boolean; problems?: string[]; warnings?: string[]; service?: Svc; resolved?: string[]; body?: Record<string, unknown> };
type Res = {
  linha: number;
  ok: boolean;
  action: "created" | "already_done" | "blocked" | "failed" | "verify_failed" | "skipped";
  message: string;
  id_atendimento?: string;
  protocolo?: string;
  ordens_servico?: string[];
  detail?: string;
  preview: Prev;
};
type Opts = {
  tipo_atendimento: string;
  status_atendimento: string;
  responsaveis: string;
  origem_contato: string;
  abrir_os: boolean;
  tipo_os: string;
  status_os: string;
  tecnicos: string;
  disponibilidade: string;
  agenda: string;
  validar_limite_agenda: boolean;
  validar_horario_agenda: boolean;
  permitir_duplicadas: boolean;
  telefone_padrao: string;
  nome_padrao: string;
  origem: string;
};

const EMPTY_ROW = (linha: number): Row => ({
  linha, referencia: "", id_cliente_servico: "", login_hubsoft: "", cliente: "", login_ixc: "", tipo_atendimento: "", status_atendimento: "", responsaveis: "",
  origem_contato: "", descricao: "", nome: "", telefone: "", email: "", abrir_os: "", tipo_os: "", status_os: "", tecnicos: "", disponibilidade: "", agenda: "", parametros: "",
});

const ACTION_LABEL: Record<Res["action"], string> = {
  created: "Aberto",
  already_done: "Já aberto antes — nada enviado",
  blocked: "Bloqueado antes de enviar",
  failed: "A HubSoft recusou",
  verify_failed: "Aberto, mas a conferência divergiu",
  skipped: "Não enviado (falhas anteriores)",
};
const ACTION_TONE: Record<Res["action"], "ok" | "err" | "warn" | undefined> = { created: "ok", already_done: undefined, blocked: "warn", failed: "err", verify_failed: "err", skipped: "warn" };

/** Tira o «="123"» que o Excel/IXC coloca em volta de números. */
function clean(v: string): string {
  const t = (v ?? "").trim();
  const m = t.match(/^="?(.*?)"?$/);
  return (m ? m[1] : t).trim();
}
function digits(s: string) {
  return s.replace(/\D/g, "");
}
function validPhone(s: string): boolean {
  const d = digits(s).replace(/^55/, "");
  return d.length >= 10 && d.length <= 11 && /[1-9]/.test(d.slice(2));
}
function zeroDate(s: string) {
  return !s || /^0+\/0+\/0+/.test(s.trim());
}

/** Converte uma linha de CSV (nomes de coluna livres; reconhece a exportação de O.S. do IXC) em Row. */
function rowFromRecord(o: Record<string, string>, linha: number, withHeader: boolean): Row {
  // «Telefone celular», «telefone_celular» e «Telefone-Celular» são a mesma coluna
  const k = (s: string) => norm(s).replace(/[\s-]+/g, "_");
  const byNorm = new Map<string, string>();
  for (const [h, v] of Object.entries(o)) byNorm.set(k(h), v ?? "");
  const g = (...aliases: string[]) => {
    for (const a of aliases) {
      const v = byNorm.get(k(a));
      if (v !== undefined && v.trim() !== "") return clean(v);
    }
    return "";
  };
  const r = EMPTY_ROW(linha);
  r.referencia = g("referencia", "id_os_ixc", "id_os", "id");
  r.id_cliente_servico = g("id_cliente_servico", "id_servico");
  r.login_hubsoft = g("login_hubsoft");
  r.cliente = g("cliente", "nome_cliente", "razao_social");
  r.login_ixc = g("login_ixc", "login");
  r.tipo_atendimento = g("tipo_atendimento");
  r.status_atendimento = g("status_atendimento");
  r.responsaveis = g("responsaveis", "responsavel");
  r.origem_contato = g("origem_contato");
  r.tipo_os = g("tipo_os", "tipo_ordem_servico", "assunto");
  r.status_os = g("status_os");
  r.tecnicos = g("tecnicos", "tecnico", "colaborador");
  r.disponibilidade = g("disponibilidade");
  r.agenda = g("agenda", "id_agenda");
  r.abrir_os = g("abrir_os");
  r.parametros = g("parametros");
  r.nome = g("nome", "solicitante");
  r.email = g("email", "e_mail");
  // telefone: o primeiro número válido entre as colunas conhecidas (o IXC preenche «(00) 00000-0000» quando não há)
  for (const c of ["telefone", "telefone_celular", "whatsapp", "telefone_residencial", "telefone_comercial"]) {
    const v = g(c);
    if (v && validPhone(v)) {
      r.telefone = v;
      break;
    }
  }
  const msg = g("descricao", "mensagem");
  const head: string[] = [];
  if (withHeader && byNorm.has("abertura")) {
    head.push(`Importado do IXC — O.S. ${r.referencia}`);
    const ab = g("abertura");
    if (ab) head.push(`aberta no IXC em ${ab}`);
    const pr = g("protocolo");
    if (pr) head.push(`protocolo ${pr}`);
    const st = g("status");
    if (st) head.push(`status no IXC: ${st}`);
    const ag = g("agendamento");
    if (ag && !zeroDate(ag)) head.push(`agendamento no IXC: ${ag}`);
  }
  r.descricao = head.length ? `[${head.join(" · ")}]\n\n${msg}` : msg;
  return r;
}

const MODEL_HEADER = [
  "referencia", "id_cliente_servico", "login_hubsoft", "cliente", "login_ixc", "tipo_atendimento", "descricao", "nome", "telefone", "abrir_os", "tipo_os", "tecnicos", "disponibilidade", "status_os", "agenda", "parametros",
];

export function HubsoftServiceOrders() {
  const { notify, missing } = useConsultaToast();
  const [cat, setCat] = useState<Catalogs | null>(null);
  const [catErr, setCatErr] = useState("");
  const [mode, setMode] = useState<"unico" | "csv">("csv");
  const [opts, setOpts] = useState<Opts>({
    tipo_atendimento: "", status_atendimento: "", responsaveis: "", origem_contato: "", abrir_os: true, tipo_os: "", status_os: "aguardando_agendamento", tecnicos: "", disponibilidade: "",
    agenda: "", validar_limite_agenda: false, validar_horario_agenda: false, permitir_duplicadas: false, telefone_padrao: "", nome_padrao: "", origem: "NetQuasar",
  });
  const [headerIxc, setHeaderIxc] = useState(true);
  const [fileName, setFileName] = useState("");
  const [fileRows, setFileRows] = useState<Row[]>([]);
  const [one, setOne] = useState<Row>(EMPTY_ROW(2));

  const rows = useMemo(() => (mode === "unico" ? [one] : fileRows), [mode, one, fileRows]);

  const [previewing, setPreviewing] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [previews, setPreviews] = useState<Prev[] | null>(null);
  const [preErr, setPreErr] = useState("");
  const [filtro, setFiltro] = useState<"problemas" | "todas">("problemas");
  const [ack, setAck] = useState(false);
  const [typed, setTyped] = useState("");
  const [applying, setApplying] = useState(false);
  const [results, setResults] = useState<Res[]>([]);
  const [stopMsg, setStopMsg] = useState("");
  const stopRef = useRef(false);

  useEffect(() => {
    let alive = true;
    apiFetch<Catalogs>(`${BASE}/catalogs`, { timeoutMs: 120_000 })
      .then((d) => alive && setCat(d))
      .catch((e) => alive && setCatErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, []);

  function resetRun() {
    setPreviews(null);
    setPreErr("");
    setResults([]);
    setStopMsg("");
    setAck(false);
    setTyped("");
  }
  const setOpt = <K extends keyof Opts>(k: K, v: Opts[K]) => {
    setOpts((o) => ({ ...o, [k]: v }));
    resetRun();
  };

  async function onFile(f: File | undefined, withHeader = headerIxc) {
    setFileRows([]);
    resetRun();
    if (!f) return setFileName("");
    setFileName(f.name);
    const objs = csvRowsToObjects(parseCsv(await f.text()));
    if (objs.length === 0) return missing("o CSV não tem linhas de dados.");
    if (objs.length > MAX_ROWS) return missing(`no máximo ${MAX_ROWS} linhas por arquivo (o arquivo tem ${objs.length}).`);
    setFileRows(objs.map((o, i) => rowFromRecord(o, i + 2, withHeader)));
  }

  const body = (list: Row[]) => ({ rows: list, options: opts });

  async function runPreview() {
    if (rows.length === 0) return missing(mode === "csv" ? "envie o CSV primeiro." : "preencha o formulário.");
    setPreviewing(true);
    resetRun();
    const acc: Prev[] = [];
    try {
      setProgress({ done: 0, total: rows.length });
      for (let i = 0; i < rows.length; i += PREVIEW_BATCH) {
        const r = await apiFetch<{ results: Prev[] }>(`${BASE}/preview`, { method: "POST", json: body(rows.slice(i, i + PREVIEW_BATCH)), timeoutMs: 8 * 60_000 });
        acc.push(...r.results);
        setProgress({ done: Math.min(i + PREVIEW_BATCH, rows.length), total: rows.length });
      }
      setPreviews(acc);
    } catch (e) {
      setPreErr(`${(e as Error).message}${acc.length ? ` (já conferidas: ${acc.length} de ${rows.length})` : ""}`);
    } finally {
      setPreviewing(false);
    }
  }

  const counts = useMemo(() => {
    const c = { enviar: 0, ja: 0, bloq: 0, aviso: 0 };
    for (const p of previews ?? []) {
      if (p.already_done) c.ja++;
      else if (!p.ok) c.bloq++;
      else {
        c.enviar++;
        if ((p.warnings ?? []).length) c.aviso++;
      }
    }
    return c;
  }, [previews]);
  const toSend = useMemo(() => {
    const okLines = new Set((previews ?? []).filter((p) => p.ok && !p.already_done).map((p) => p.linha));
    return rows.filter((r) => okLines.has(r.linha));
  }, [previews, rows]);

  const confirmText = `ABRIR ${toSend.length}`;
  const canApply = toSend.length > 0 && ack && typed.trim() === confirmText && !applying && results.length === 0;

  async function runApply() {
    stopRef.current = false;
    setApplying(true);
    setStopMsg("");
    setResults([]);
    const acc: Res[] = [];
    try {
      setProgress({ done: 0, total: toSend.length });
      for (let i = 0; i < toSend.length && !stopRef.current; i += APPLY_BATCH) {
        const r = await apiFetch<{ results: Res[]; halted: boolean }>(`${BASE}/apply`, { method: "POST", json: body(toSend.slice(i, i + APPLY_BATCH)), timeoutMs: 10 * 60_000 });
        acc.push(...r.results);
        setResults([...acc]);
        setProgress({ done: Math.min(i + APPLY_BATCH, toSend.length), total: toSend.length });
        if (r.halted) {
          setStopMsg("EXECUÇÃO INTERROMPIDA — três recusas seguidas da HubSoft. Veja a mensagem de cada linha antes de repetir.");
          break;
        }
      }
      if (stopRef.current) setStopMsg("Interrompido pelo operador. Reenviar o mesmo arquivo é seguro: o que já foi aberto é reconhecido pela referência e não é duplicado.");
    } catch (e) {
      setStopMsg(`Interrompido por erro de comunicação: ${(e as Error).message}. Reenvie o mesmo arquivo para continuar — o que já foi aberto não é duplicado (confira o histórico).`);
    } finally {
      setApplying(false);
    }
    notify({ ok: true }, null, `${acc.filter((r) => r.ok).length} de ${acc.length} linha(s) processada(s) sem problema.`);
  }

  const created = results.filter((r) => r.action === "created").length;
  const failCount = results.filter((r) => !r.ok).length;
  const rowByLine = useMemo(() => new Map(rows.map((r) => [r.linha, r])), [rows]);
  const shownPrev = (previews ?? []).filter((p) => filtro === "todas" || !p.ok || p.already_done || (p.warnings ?? []).length > 0);

  function downloadModel() {
    downloadCsv("modelo-atendimento-os.csv", MODEL_HEADER, [[
      "OS-0001", "", "", "NOME DO CLIENTE", "login.do.cliente", "SUPORTE", "Descrição do atendimento / motivo", "NOME DO CLIENTE", "(22) 99999-0000", "sim", "RETIRADA DE EQUIPAMENTO", "NOME DO TÉCNICO", "Manhã", "aguardando_agendamento", "", "setor=SUPORTE",
    ]]);
  }
  function downloadPending() {
    const out: string[][] = [];
    for (const p of previews ?? []) if (!p.ok && !p.already_done) out.push([String(p.linha), rowByLine.get(p.linha)?.referencia ?? "", rowByLine.get(p.linha)?.cliente ?? "", (p.problems ?? []).join(" | ")]);
    for (const r of results) if (!r.ok && r.action !== "already_done") out.push([String(r.linha), rowByLine.get(r.linha)?.referencia ?? "", rowByLine.get(r.linha)?.cliente ?? "", `${ACTION_LABEL[r.action]}: ${r.message}`]);
    downloadCsv(`pendencias-atendimento-os-${todayISO()}.csv`, ["linha", "referencia", "cliente", "motivo"], out);
  }
  function downloadResults() {
    downloadCsv(
      `atendimento-os-resultado-${todayISO()}.csv`,
      ["linha", "referencia", "cliente", "id_cliente_servico", "resultado", "mensagem", "id_atendimento", "protocolo", "ordens_servico", "detalhe_resposta_hubsoft"],
      results.map((r) => [String(r.linha), rowByLine.get(r.linha)?.referencia ?? "", r.preview.service?.cliente ?? rowByLine.get(r.linha)?.cliente ?? "", r.preview.service?.id_cliente_servico ?? "", ACTION_LABEL[r.action], r.message, r.id_atendimento ?? "", r.protocolo ?? "", (r.ordens_servico ?? []).join(" | "), r.detail ?? ""]),
    );
  }

  const sel = (items: Item[] | undefined, value: string, onChange: (v: string) => void, placeholder: string, multi = false) => (
    <select className="input" value={value} onChange={(e) => onChange(e.target.value)} disabled={previewing || applying} multiple={multi} style={multi ? { minHeight: 92 } : undefined}>
      {!multi ? <option value="">{placeholder}</option> : null}
      {(items ?? []).map((i) => (
        <option key={i.id} value={i.id}>
          {i.nome} ({i.id})
        </option>
      ))}
    </select>
  );
  const multiVal = (csv: string) => csv.split(",").map((s) => s.trim()).filter(Boolean);

  return (
    <ToolPanel
      icon={<Wrench size={20} />}
      title="Abrir atendimento com O.S."
      badge="Altera a HubSoft"
      badgeTone="warn"
      subtitle="Abre o atendimento (e a ordem de serviço junto) para um cliente — um por vez ou importando um CSV. Tudo é conferido antes e a resposta da HubSoft é conferida depois."
    >
      <div className="hsa-panel__body hsa-panel__body--pad" style={{ borderBottom: "1px solid var(--border)" }}>
        <Callout tone="info">
          A API da HubSoft <b>não cria O.S. avulsa</b>: toda O.S. nasce de um atendimento, então esta tela abre os dois juntos. O serviço do cliente é achado por <code>id_cliente_servico</code>, pelo login da HubSoft ou pelo <b>nome do cliente</b>
          (com o login do IXC para desempatar quando há mais de um serviço). A <b>referência</b> (por exemplo o id da O.S. do IXC) é gravada no atendimento e impede abrir a mesma O.S. duas vezes. Data/hora programada não existe na abertura — a O.S. entra
          como «aguardando agendamento» (ou na agenda informada) e é agendada depois.
        </Callout>
        {cat?.erros?.length ? <Callout tone="warn">Alguns cadastros da HubSoft não carregaram: {cat.erros.join(" · ")}</Callout> : null}
        {catErr ? <Callout tone="err">Não foi possível ler os cadastros da HubSoft: {catErr}</Callout> : null}
      </div>

      <Step n={1} title="Padrões do atendimento e da O.S." done={!!opts.tipo_atendimento || !!opts.tipo_os} hint="Valem para todas as linhas; um valor preenchido na linha do CSV vence o padrão.">
        <div className="hsa-fields" style={{ display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))" }}>
          <label className="field">
            <span>Tipo de atendimento</span>
            {sel(cat?.tipos_atendimento, opts.tipo_atendimento, (v) => setOpt("tipo_atendimento", v), "(padrão da HubSoft: SAC)")}
          </label>
          <label className="field">
            <span>Status do atendimento</span>
            {sel(cat?.status_atendimento, opts.status_atendimento, (v) => setOpt("status_atendimento", v), "(padrão da HubSoft)")}
          </label>
          <label className="field">
            <span>Origem do contato</span>
            {sel(cat?.origens_contato, opts.origem_contato, (v) => setOpt("origem_contato", v), "(nenhuma)")}
          </label>
          <label className="field">
            <span>Responsável(is) pelo atendimento</span>
            <select className="input" multiple value={multiVal(opts.responsaveis)} style={{ minHeight: 92 }} onChange={(e) => setOpt("responsaveis", Array.from(e.target.selectedOptions).map((o) => o.value).join(","))}>
              {(cat?.tecnicos ?? []).map((i) => (
                <option key={i.id} value={i.id}>
                  {i.nome} ({i.id})
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="hsa-check">
          <input type="checkbox" checked={opts.abrir_os} onChange={(e) => setOpt("abrir_os", e.target.checked)} /> Abrir também a Ordem de Serviço
        </label>
        {opts.abrir_os ? (
          <div className="hsa-fields" style={{ display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))" }}>
            <label className="field">
              <span>Tipo de O.S.</span>
              {sel((cat?.tipos_os ?? []).filter((t) => !t.extra || t.extra === "cliente_servico"), opts.tipo_os, (v) => setOpt("tipo_os", v), "(selecione — ou use a coluna tipo_os)")}
            </label>
            <label className="field">
              <span>Status inicial da O.S.</span>
              <select className="input" value={opts.status_os} onChange={(e) => setOpt("status_os", e.target.value)}>
                <option value="aguardando_agendamento">Aguardando agendamento</option>
                <option value="pendente">Pendente</option>
              </select>
            </label>
            <label className="field">
              <span>Técnico(s)</span>
              <select className="input" multiple value={multiVal(opts.tecnicos)} style={{ minHeight: 92 }} onChange={(e) => setOpt("tecnicos", Array.from(e.target.selectedOptions).map((o) => o.value).join(","))}>
                {(cat?.tecnicos ?? []).map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.nome} ({i.id})
                  </option>
                ))}
              </select>
              <small className="hsa-muted">Vazio = técnico padrão «SAC (Atendimento)» da HubSoft.</small>
            </label>
            <label className="field">
              <span>Disponibilidade</span>
              <select className="input" multiple value={multiVal(opts.disponibilidade)} onChange={(e) => setOpt("disponibilidade", Array.from(e.target.selectedOptions).map((o) => o.value).join(","))}>
                {(cat?.disponibilidades ?? []).map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.nome}
                  </option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>ID da agenda de O.S. (opcional)</span>
              <input className="input" value={opts.agenda} onChange={(e) => setOpt("agenda", e.target.value.replace(/\D/g, ""))} placeholder="ex.: 25" />
            </label>
          </div>
        ) : null}
        {opts.abrir_os && opts.agenda ? (
          <div className="hsa-actions">
            <label className="hsa-check">
              <input type="checkbox" checked={opts.validar_limite_agenda} onChange={(e) => setOpt("validar_limite_agenda", e.target.checked)} /> Respeitar o limite da agenda
            </label>
            <label className="hsa-check">
              <input type="checkbox" checked={opts.validar_horario_agenda} onChange={(e) => setOpt("validar_horario_agenda", e.target.checked)} /> Respeitar o horário/técnicos da agenda
            </label>
          </div>
        ) : null}
        <label className="hsa-check">
          <input type="checkbox" checked={opts.permitir_duplicadas} onChange={(e) => setOpt("permitir_duplicadas", e.target.checked)} /> Abrir mesmo se o cliente já tiver atendimento ABERTO do mesmo tipo na HubSoft
        </label>
        <span className="hsa-muted">Por padrão (desmarcado) a conferência bloqueia a linha quando o cliente já tem um atendimento aberto com o mesmo assunto (ex.: «RETIRADA DE EQUIPAMENTO»), para não duplicar.</span>
        <div className="hsa-fields" style={{ display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))" }}>
          <label className="field">
            <span>Telefone padrão (se a linha não tiver um válido)</span>
            <input className="input" value={opts.telefone_padrao} onChange={(e) => setOpt("telefone_padrao", e.target.value)} placeholder="(22) 99999-0000" />
          </label>
          <label className="field">
            <span>Nome do solicitante padrão</span>
            <input className="input" value={opts.nome_padrao} onChange={(e) => setOpt("nome_padrao", e.target.value)} placeholder="(vazio = nome do cliente)" />
          </label>
          <label className="field">
            <span>Origem gravada no atendimento</span>
            <input className="input" value={opts.origem} onChange={(e) => setOpt("origem", e.target.value)} placeholder="NetQuasar — importação IXC" />
          </label>
        </div>
      </Step>

      <Step n={2} title="Dados" done={rows.length > 0 && (mode === "unico" ? !!one.descricao : true)}>
        <Segmented
          label="Entrada"
          value={mode}
          onChange={(m) => {
            setMode(m);
            resetRun();
          }}
          options={[
            { value: "csv", label: "Importar CSV (em massa)" },
            { value: "unico", label: "Um por vez" },
          ]}
        />
        {mode === "csv" ? (
          <>
            <div className="hsa-actions">
              <button type="button" className="btn btn--sm" onClick={downloadModel}>
                <FileSpreadsheet size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Baixar modelo CSV
              </button>
              <label className="hsa-check">
                <input
                  type="checkbox"
                  checked={headerIxc}
                  onChange={(e) => {
                    setHeaderIxc(e.target.checked);
                    resetRun();
                  }}
                />
                Arquivo do IXC: acrescentar no início da descrição a origem (O.S., abertura, protocolo, status)
              </label>
            </div>
            <span className="hsa-muted">
              Colunas aceitas (nomes livres): referencia/ID, id_cliente_servico, login_hubsoft, cliente, login, tipo_atendimento, descricao/Mensagem, nome, telefone (ou Telefone celular/Whatsapp), abrir_os, tipo_os/Assunto,
              tecnicos/Colaborador, disponibilidade, status_os, agenda, parametros (chave=valor;chave=valor). A exportação de O.S. do IXC já serve como está.
            </span>
            <CsvDropzone fileName={fileName} info={fileRows.length > 0 ? `${fileRows.length} linha(s) lida(s)` : undefined} disabled={previewing || applying} onFile={(f) => void onFile(f)} onClear={() => void onFile(undefined)} />
          </>
        ) : (
          <div className="hsa-fields" style={{ display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))" }}>
            {(
              [
                ["id_cliente_servico", "ID do serviço (ou…)"],
                ["login_hubsoft", "…login do serviço na HubSoft (ou…)"],
                ["cliente", "…nome do cliente"],
                ["login_ixc", "Login (desempate, opcional)"],
                ["nome", "Nome do solicitante"],
                ["telefone", "Telefone do solicitante"],
                ["referencia", "Referência (opcional)"],
              ] as [keyof Row, string][]
            ).map(([k, label]) => (
              <label className="field" key={k}>
                <span>{label}</span>
                <input
                  className="input"
                  value={String(one[k])}
                  onChange={(e) => {
                    setOne((r) => ({ ...r, [k]: e.target.value }));
                    resetRun();
                  }}
                />
              </label>
            ))}
            <label className="field" style={{ gridColumn: "1 / -1" }}>
              <span>Descrição do atendimento</span>
              <textarea
                className="input"
                rows={4}
                value={one.descricao}
                onChange={(e) => {
                  setOne((r) => ({ ...r, descricao: e.target.value }));
                  resetRun();
                }}
              />
            </label>
          </div>
        )}
      </Step>

      <Step n={3} title="Conferir com a HubSoft (obrigatório)" done={!!previews && !previewing} hint="Somente leitura: acha o serviço de cada cliente, resolve os cadastros e mostra o que seria enviado.">
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={previewing || applying || rows.length === 0} onClick={() => void runPreview()}>
            <ListChecks size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {previewing ? "Conferindo…" : previews ? "Conferir de novo" : `Conferir ${rows.length} linha(s)`}
          </button>
        </div>
        {previewing ? <ProgressBar done={progress.done} total={progress.total} label={`Conferindo… ${progress.done} de ${progress.total}`} /> : null}
        {preErr ? <Callout tone="err">Não foi possível conferir: {preErr}</Callout> : null}
        {previews ? (
          <>
            <div className="hsa-stats">
              <Stat label="Serão abertas" value={counts.enviar} tone="ok" />
              <Stat label="…com aviso" value={counts.aviso} tone={counts.aviso ? "warn" : "muted"} />
              <Stat label="Já abertas antes" value={counts.ja} tone="muted" />
              <Stat label="Bloqueadas" value={counts.bloq} tone={counts.bloq ? "err" : "muted"} />
            </div>
            <div className="hsa-actions">
              <label className="hsa-check">
                <input type="checkbox" checked={filtro === "todas"} onChange={(e) => setFiltro(e.target.checked ? "todas" : "problemas")} />
                Mostrar também as sem aviso
              </label>
              <span className="hsa-spacer" />
              <button type="button" className="btn btn--sm" disabled={counts.bloq === 0} onClick={downloadPending}>
                <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Baixar pendências (CSV)
              </button>
            </div>
            <div className="hsa-table-wrap" style={{ maxHeight: 420 }}>
              <table className="hsa-table">
                <thead>
                  <tr>
                    <th>Linha</th>
                    <th>Cliente / serviço</th>
                    <th>O que acontece</th>
                  </tr>
                </thead>
                <tbody>
                  {shownPrev.slice(0, 400).map((p) => {
                    const r = rowByLine.get(p.linha);
                    return (
                      <tr key={p.linha}>
                        <td className="hsa-table__num mono">{p.linha}</td>
                        <td>
                          {p.service ? (
                            <>
                              {p.service.cliente}
                              <div className="hsa-muted mono">serviço {p.service.id_cliente_servico} · {p.service.login || "—"} · {p.service.status || "—"}</div>
                            </>
                          ) : (
                            r?.cliente || r?.login_hubsoft || r?.id_cliente_servico || "—"
                          )}
                          {r?.referencia ? <div className="hsa-muted">ref. {r.referencia}</div> : null}
                        </td>
                        <td>
                          {p.already_done ? <Pill>Já aberta</Pill> : !p.ok ? <Pill tone="err">Bloqueada</Pill> : <Pill tone="ok">Será aberta</Pill>}
                          {(p.problems ?? []).map((x) => (
                            <div key={x} className="hsa-muted" style={{ marginTop: 4, color: "var(--err)" }}>{x}</div>
                          ))}
                          {(p.warnings ?? []).map((x) => (
                            <div key={x} className="hsa-muted" style={{ marginTop: 4, color: "var(--warn)" }}>⚠ {x}</div>
                          ))}
                          {p.ok && !p.already_done && (p.resolved ?? []).length ? <div className="hsa-muted" style={{ marginTop: 4 }}>{(p.resolved ?? []).join(" · ")}</div> : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            {shownPrev.length > 400 ? <span className="hsa-muted">Mostrando as 400 primeiras de {shownPrev.length}. Baixe as pendências para ver todas.</span> : null}
          </>
        ) : null}
      </Step>

      {previews && toSend.length > 0 ? (
        <Step n={4} title="Abrir os atendimentos" done={results.length > 0 && !applying} hint="Esta etapa altera a HubSoft: cria atendimentos (e O.S.) de verdade.">
          <label className="hsa-check">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Revisei a conferência e entendo que isto abre {toSend.length.toLocaleString("pt-BR")} atendimento(s){opts.abrir_os ? " com O.S." : ""} na HubSoft de verdade. Bloqueadas ficam de fora.
          </label>
          <div className="hsa-actions">
            <input className="input" style={{ width: 200 }} placeholder={confirmText} value={typed} onChange={(e) => setTyped(e.target.value)} />
            <button type="button" className="btn btn--primary" disabled={!canApply} onClick={() => void runApply()}>
              {applying ? "Abrindo…" : `Abrir ${toSend.length.toLocaleString("pt-BR")}`}
            </button>
            {applying ? (
              <button type="button" className="btn" onClick={() => { stopRef.current = true; }}>
                <Square size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
                Parar
              </button>
            ) : null}
          </div>
          <span className="hsa-muted">Digite exatamente “{confirmText}”. Vai de {APPLY_BATCH} em {APPLY_BATCH}; para após 3 recusas seguidas e pode ser retomado reenviando o mesmo arquivo (a referência evita duplicar).</span>
          {applying ? <ProgressBar done={progress.done} total={progress.total} label={`Abrindo… ${progress.done} de ${progress.total}`} /> : null}
        </Step>
      ) : null}

      {results.length > 0 ? (
        <Step n={5} title="Resultado" done={!applying && failCount === 0} hint={'Cada linha também fica na aba "Histórico" (Atendimento / O.S.).'}>
          <div className="hsa-stats">
            <Stat label="Abertos agora" value={created} tone="ok" />
            <Stat label="Já abertos / sem problema" value={results.filter((r) => r.ok).length - created} tone="muted" />
            <Stat label="Com problema" value={failCount} tone={failCount ? "err" : "muted"} />
          </div>
          {stopMsg ? <Callout tone="err">{stopMsg}</Callout> : null}
          <div className="hsa-actions">
            <span className="hsa-spacer" />
            <button type="button" className="btn btn--sm" onClick={downloadResults}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Resultado completo (CSV)
            </button>
            <button type="button" className="btn btn--sm" disabled={failCount === 0} onClick={downloadPending}>
              <Download size={13} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Pendências (CSV)
            </button>
          </div>
          <div className="hsa-table-wrap" style={{ maxHeight: 440 }}>
            <table className="hsa-table">
              <thead>
                <tr>
                  <th>Linha</th>
                  <th>Cliente</th>
                  <th>Resultado</th>
                </tr>
              </thead>
              <tbody>
                {results.filter((r) => !r.ok || results.length <= 200).slice(0, 500).map((r) => (
                  <tr key={r.linha}>
                    <td className="hsa-table__num mono">{r.linha}</td>
                    <td>
                      {r.preview.service?.cliente ?? rowByLine.get(r.linha)?.cliente ?? "—"}
                      <div className="hsa-muted mono">{rowByLine.get(r.linha)?.referencia ? `ref. ${rowByLine.get(r.linha)?.referencia}` : ""}</div>
                    </td>
                    <td>
                      <Pill tone={ACTION_TONE[r.action]}>{ACTION_LABEL[r.action]}</Pill>
                      <div className="hsa-muted" style={{ marginTop: 4, color: r.ok ? undefined : "var(--err)" }}>{r.message}</div>
                      {r.detail ? (
                        <details style={{ marginTop: 4 }}>
                          <summary style={{ cursor: "pointer", fontSize: 11 }}>Resposta bruta da HubSoft</summary>
                          <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word", margin: "4px 0 0" }}>{r.detail}</pre>
                        </details>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {results.length > 200 ? <span className="hsa-muted">Com muitos itens, a tabela mostra só os com problema; baixe o resultado completo para ver todos.</span> : null}
        </Step>
      ) : null}
    </ToolPanel>
  );
}
