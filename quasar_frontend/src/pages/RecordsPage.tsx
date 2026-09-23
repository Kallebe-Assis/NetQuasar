import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, Copy, Download, Eye, EyeOff, KeyRound } from "lucide-react";
import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { getStoredUserId, isAdminUser } from "../lib/auth";
import { toastErr, toastOk } from "../lib/operationToast";
import { queryKeys } from "../lib/queryKeys";

type Kind = "equipment" | "server" | "site" | "orcamento" | "informacao" | "texto" | "outros";
type Mode = "user_password" | "password";

const CREDENTIAL_KINDS: Kind[] = ["equipment", "server", "site"];
const TEXT_KINDS: Kind[] = ["orcamento", "informacao", "texto", "outros"];

type RecordItem = {
  id: string;
  owner_user_id: string;
  owner_name: string;
  kind: Kind;
  title: string;
  device_id?: string | null;
  device_name?: string | null;
  device_ip?: string | null;
  host?: string | null;
  domain?: string | null;
  username?: string | null;
  has_username: boolean;
  has_password?: boolean;
  content?: string | null;
  notes?: string | null;
  created_at: string;
  updated_at: string;
};

type Lookups = {
  users: { id: string; label: string }[];
  devices: { id: string; description: string; ip?: string | null; category: string }[];
};

type Form = {
  owner_user_id: string;
  kind: Kind;
  title: string;
  content: string;
  device_id: string;
  host: string;
  domain: string;
  mode: Mode;
  username: string;
  password: string;
  notes: string;
};

const KIND_LABEL: Record<Kind, string> = {
  equipment: "Equipamento",
  server: "Servidor",
  site: "Site",
  orcamento: "Orçamento",
  informacao: "Informação",
  texto: "Texto",
  outros: "Outros",
};

function isTextKind(kind: Kind) {
  return TEXT_KINDS.includes(kind);
}

function contentPreview(text?: string | null, max = 80) {
  const s = String(text ?? "").trim();
  if (!s) return "—";
  return s.length > max ? `${s.slice(0, max)}…` : s;
}

function emptyForm(): Form {
  return {
    owner_user_id: "",
    kind: "equipment",
    title: "",
    content: "",
    device_id: "",
    host: "",
    domain: "",
    mode: "user_password",
    username: "",
    password: "",
    notes: "",
  };
}

function formatWhen(iso: string) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
}

function targetOf(it: RecordItem): string {
  if (isTextKind(it.kind)) return it.title || contentPreview(it.content, 48);
  if (it.kind === "equipment") {
    const ip = it.device_ip ? ` (${it.device_ip})` : "";
    return `${it.device_name || it.title || "Equipamento"}${ip}`;
  }
  if (it.kind === "server") return it.host || it.title || "Servidor";
  return it.domain || it.title || "Site";
}

function csvCell(v: string): string {
  return /[",\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** Exporta em CSV as colunas já visíveis na tabela — a senha só entra se `passwords` for
 * passado (o chamador decide isso via o modal de confirmação, nunca por omissão), usando o
 * mesmo valor já revelado registo a registo (com o seu rasto de auditoria normal). */
function exportRecordsCsv(items: RecordItem[], admin: boolean, passwords?: Record<string, string>) {
  const headers = ["Tipo", "Destino", "Acesso", ...(admin ? ["Usuário"] : []), ...(passwords ? ["Senha"] : []), "Atualizado"];
  const lines = [headers.map(csvCell).join(",")];
  for (const it of items) {
    const acesso = isTextKind(it.kind)
      ? contentPreview(it.content)
      : it.has_username
        ? (it.username ?? "")
        : "somente senha";
    const senha = isTextKind(it.kind) ? "" : (passwords?.[it.id] ?? "");
    const cols = [
      KIND_LABEL[it.kind],
      it.title || targetOf(it),
      acesso,
      ...(admin ? [it.owner_name] : []),
      ...(passwords ? [senha] : []),
      formatWhen(it.updated_at),
    ];
    lines.push(cols.map((c) => csvCell(String(c ?? ""))).join(","));
  }
  const csv = lines.join("\r\n");
  const blob = new Blob([`﻿${csv}`], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `registros-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

export function RecordsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const admin = isAdminUser();

  const currentUserId = useMemo(() => getStoredUserId(), []);

  const [q, setQ] = useState("");
  const [kind, setKind] = useState("");
  const [ownerId, setOwnerId] = useState(currentUserId);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState<Form>(emptyForm());
  const [deviceSearch, setDeviceSearch] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<RecordItem | null>(null);
  const [reveal, setReveal] = useState<
    | { id: string; kind: Kind; username: string | null; password: string }
    | { id: string; kind: Kind; content: string }
    | null
  >(null);
  const [showPass, setShowPass] = useState(false);

  // Coluna "Senha" da tabela — sempre começa oculta (sem persistência) a cada entrada/refresh da
  // tela, como pedido. O cache guarda o que já foi revelado nesta sessão para não repetir chamadas
  // de "reveal" (cada uma já fica registada em auditoria) ao alternar mostrar/ocultar.
  const [passwordsVisible, setPasswordsVisible] = useState(false);
  const [passwordCache, setPasswordCache] = useState<Record<string, string>>({});
  const [revealingAll, setRevealingAll] = useState(false);
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [exportIncludePasswords, setExportIncludePasswords] = useState(false);
  const [exportAware, setExportAware] = useState(false);
  const [exporting, setExporting] = useState(false);

  const lookupsQ = useQuery({
    queryKey: queryKeys.credentialLookups,
    queryFn: () => apiFetch<Lookups>("/api/v1/credential-records/lookups"),
  });

  const listQs = useMemo(() => {
    const p = new URLSearchParams();
    if (q.trim()) p.set("q", q.trim());
    if (kind) p.set("kind", kind);
    if (admin && ownerId) p.set("owner_user_id", ownerId);
    return p.toString();
  }, [q, kind, ownerId, admin]);

  const listQ = useQuery({
    queryKey: queryKeys.credentialRecords(listQs),
    queryFn: () => apiFetch<{ items: RecordItem[]; total: number }>(`/api/v1/credential-records?${listQs}`),
  });

  const items = listQ.data?.items ?? [];
  const lookups = lookupsQ.data;
  const users = lookups?.users ?? [];
  const devices = lookups?.devices ?? [];

  const filteredDevices = useMemo(() => {
    const s = deviceSearch.trim().toLowerCase();
    if (!s) return devices;
    return devices.filter(
      (d) =>
        d.description.toLowerCase().includes(s) ||
        (d.ip ?? "").toLowerCase().includes(s) ||
        d.category.toLowerCase().includes(s),
    );
  }, [devices, deviceSearch]);

  function invalidate() {
    return Promise.all([
      qc.invalidateQueries({ queryKey: ["credential-records"] }),
      qc.invalidateQueries({ queryKey: queryKeys.credentialLookups }),
    ]);
  }

  function openCreate() {
    const f = emptyForm();
    f.owner_user_id = currentUserId || (users.length === 1 ? users[0].id : "");
    setEditing(null);
    setForm(f);
    setDeviceSearch("");
    setFormOpen(true);
  }

  function openEdit(it: RecordItem) {
    setEditing(it.id);
    setForm({
      owner_user_id: it.owner_user_id,
      kind: it.kind,
      title: it.title,
      content: it.content ?? "",
      device_id: it.device_id ?? "",
      host: it.host ?? "",
      domain: it.domain ?? "",
      mode: it.has_username ? "user_password" : "password",
      username: it.username ?? "",
      password: "",
      notes: it.notes ?? "",
    });
    setDeviceSearch("");
    setFormOpen(true);
  }

  const save = useMutation({
    mutationFn: async () => {
      const text = isTextKind(form.kind);
      const payload = text
        ? {
            owner_user_id: admin ? form.owner_user_id || undefined : undefined,
            kind: form.kind,
            title: form.title.trim(),
            content: form.content.trim(),
            notes: form.notes.trim() || null,
          }
        : {
            owner_user_id: admin ? form.owner_user_id || undefined : undefined,
            kind: form.kind,
            title: form.title.trim() || undefined,
            device_id: form.kind === "equipment" ? form.device_id || null : null,
            host: form.kind === "server" ? form.host.trim() || null : null,
            domain: form.kind === "site" ? form.domain.trim() || null : null,
            mode: form.mode,
            username: form.mode === "user_password" ? form.username.trim() || null : null,
            password: form.password.trim() || undefined,
            notes: form.notes.trim() || null,
          };
      if (editing) {
        return apiFetch(`/api/v1/credential-records/${editing}`, { method: "PATCH", json: payload });
      }
      return apiFetch("/api/v1/credential-records", { method: "POST", json: payload });
    },
    onSuccess: async () => {
      toastOk(push, editing ? "Registo atualizado." : "Registo guardado.");
      setFormOpen(false);
      setEditing(null);
      await invalidate();
    },
    onError: (e) => toastErr(push, e),
  });

  const remove = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/credential-records/${id}`, { method: "DELETE" }),
    onSuccess: async () => {
      toastOk(push, "Registo removido.");
      setDeleteTarget(null);
      await invalidate();
    },
    onError: (e) => toastErr(push, e),
  });

  const revealMut = useMutation({
    mutationFn: (id: string) =>
      apiFetch<{ username?: string | null; password?: string; content?: string; kind?: string }>(
        `/api/v1/credential-records/${id}/reveal`,
        { method: "POST" },
      ),
    onSuccess: (data, id) => {
      const item = items.find((x) => x.id === id);
      const kind = (item?.kind ?? data.kind ?? "equipment") as Kind;
      if (isTextKind(kind) || data.content != null) {
        setReveal({ id, kind, content: data.content ?? "" });
        return;
      }
      setShowPass(true);
      setReveal({ id, kind, username: data.username ?? null, password: data.password ?? "" });
    },
    onError: (e) => toastErr(push, e),
  });

  // "Copiar credenciais" — busca a senha (mesmo endpoint de revelar) e copia direto para a
  // área de transferência já formatado, sem precisar abrir o modal de "Ver senha" primeiro.
  const copyCredentialsMut = useMutation({
    mutationFn: async (it: RecordItem) => {
      const data = await apiFetch<{ username?: string | null; password?: string }>(
        `/api/v1/credential-records/${it.id}/reveal`,
        { method: "POST" },
      );
      const label = it.title || targetOf(it);
      const lines = [`Credenciais ${label}`, ""];
      if (data.username) lines.push(`Usuario: ${data.username}`);
      lines.push(`Senha: ${data.password ?? ""}`);
      return lines.join("\n");
    },
    onSuccess: async (text) => {
      const ok = await copyText(text);
      if (ok) toastOk(push, "Credenciais copiadas.");
      else toastErr(push, new Error("Não foi possível aceder à área de transferência."));
    },
    onError: (e) => toastErr(push, e),
  });

  /** Revela (e guarda em cache) a senha dos registos de credenciais na lista — pula os que já
   * estão em cache e os de tipo texto (não têm senha). Cada chamada é a mesma rota de "Ver
   * senha" de sempre, uma por registo, com o mesmo rasto de auditoria — só automatizada aqui
   * para não obrigar a abrir um a um. */
  async function revealPasswordsFor(list: RecordItem[]): Promise<Record<string, string>> {
    const targets = list.filter((it) => !isTextKind(it.kind) && it.has_password && !(it.id in passwordCache));
    if (targets.length === 0) return passwordCache;
    const results = await Promise.allSettled(
      targets.map((it) =>
        apiFetch<{ password?: string }>(`/api/v1/credential-records/${it.id}/reveal`, { method: "POST" }),
      ),
    );
    const next = { ...passwordCache };
    targets.forEach((it, i) => {
      const r = results[i];
      if (r.status === "fulfilled") next[it.id] = r.value.password ?? "";
    });
    setPasswordCache(next);
    return next;
  }

  async function togglePasswordsVisible() {
    if (passwordsVisible) {
      setPasswordsVisible(false);
      return;
    }
    setRevealingAll(true);
    try {
      await revealPasswordsFor(items);
      setPasswordsVisible(true);
    } catch (e) {
      toastErr(push, e as Error);
    } finally {
      setRevealingAll(false);
    }
  }

  function openExportModal() {
    setExportIncludePasswords(false);
    setExportAware(false);
    setExportModalOpen(true);
  }

  async function confirmExport() {
    setExporting(true);
    try {
      const pwMap = exportIncludePasswords ? await revealPasswordsFor(items) : undefined;
      exportRecordsCsv(items, admin, pwMap);
      setExportModalOpen(false);
    } catch (e) {
      toastErr(push, e as Error);
    } finally {
      setExporting(false);
    }
  }

  const colSpan = admin ? 7 : 6;

  return (
    <div className="vault-page">
      <div className="page-heading">
        <h1>
          Registros
          <InfoHint>
            {admin
              ? "Cofre de senhas e registos de texto (orçamentos, informações, notas). Como administrador vê todos e pode filtrar por usuário."
              : "O seu cofre de senhas e registos de texto. Só você (e os administradores) vê estes registos."}
          </InfoHint>
        </h1>
        <PageCountPill label="registos" count={listQ.data?.total ?? items.length} />
      </div>

      <div className="netev-toolbar">
        <input
          className="input"
          placeholder="Pesquisar título, host, domínio, texto…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <select className="input" value={kind} onChange={(e) => setKind(e.target.value)} aria-label="Tipo">
          <option value="">Todos os tipos</option>
          <option value="equipment">Equipamento</option>
          <option value="server">Servidor</option>
          <option value="site">Site</option>
          <option value="orcamento">Orçamento</option>
          <option value="informacao">Informação</option>
          <option value="texto">Texto</option>
          <option value="outros">Outros</option>
        </select>
        {admin ? (
          <select className="input" value={ownerId} onChange={(e) => setOwnerId(e.target.value)} aria-label="Usuário">
            <option value="">Todos os usuários</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.label}
              </option>
            ))}
          </select>
        ) : null}
        <button
          type="button"
          className="btn btn--icon btn--icon-menu"
          title={passwordsVisible ? "Ocultar senhas" : "Mostrar senhas"}
          aria-label={passwordsVisible ? "Ocultar senhas" : "Mostrar senhas"}
          disabled={revealingAll}
          onClick={() => void togglePasswordsVisible()}
        >
          {passwordsVisible ? <EyeOff size={18} aria-hidden /> : <Eye size={18} aria-hidden />}
        </button>
        <button
          type="button"
          className="btn btn--icon btn--icon-menu"
          title="Exportar CSV (respeita os filtros aplicados)"
          aria-label="Exportar CSV"
          disabled={items.length === 0}
          onClick={openExportModal}
        >
          <Download size={18} aria-hidden />
        </button>
        <button
          type="button"
          className="btn btn--icon btn--icon-menu btn--primary"
          title="Novo registo"
          aria-label="Novo registo"
          onClick={openCreate}
        >
          <CirclePlus size={18} aria-hidden />
        </button>
      </div>

      <div className="card table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Tipo</th>
              <th>Destino</th>
              <th>Acesso</th>
              <th>Senha</th>
              {admin ? <th>Usuário</th> : null}
              <th>Atualizado</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {listQ.isLoading ? (
              <tr>
                <td colSpan={colSpan} className="muted">
                  A carregar…
                </td>
              </tr>
            ) : items.length === 0 ? (
              <tr>
                <td colSpan={colSpan} className="muted">
                  Nenhum registo. Guarde senhas ou textos com título e tipo.
                </td>
              </tr>
            ) : (
              items.map((it) => (
                <tr key={it.id}>
                  <td>
                    <span className={`vault-kind vault-kind--${it.kind}`}>{KIND_LABEL[it.kind]}</span>
                  </td>
                  <td>
                    <div className="vault-target">
                      <strong>{it.title || targetOf(it)}</strong>
                      {it.title && targetOf(it) !== it.title ? (
                        <span className="muted">{targetOf(it)}</span>
                      ) : null}
                    </div>
                  </td>
                  <td>
                    {isTextKind(it.kind) ? (
                      <span className="muted">{contentPreview(it.content)}</span>
                    ) : it.has_username ? (
                      it.username
                    ) : (
                      <span className="muted">somente senha</span>
                    )}
                  </td>
                  <td className="mono">
                    {isTextKind(it.kind) ? (
                      <span className="muted">—</span>
                    ) : !it.has_password ? (
                      <span className="muted">—</span>
                    ) : passwordsVisible && it.id in passwordCache ? (
                      passwordCache[it.id]
                    ) : (
                      <span className="muted">••••••••</span>
                    )}
                  </td>
                  {admin ? <td>{it.owner_name}</td> : null}
                  <td className="netev-when">{formatWhen(it.updated_at)}</td>
                  <td>
                    <ActionMenu
                      items={[
                        isTextKind(it.kind)
                          ? { id: "reveal", label: "Ver conteúdo", onClick: () => revealMut.mutate(it.id) }
                          : { id: "reveal", label: "Ver senha", onClick: () => revealMut.mutate(it.id) },
                        ...(!isTextKind(it.kind)
                          ? [{ id: "copy-creds", label: "Copiar credenciais", onClick: () => copyCredentialsMut.mutate(it) }]
                          : []),
                        { id: "edit", label: "Editar", onClick: () => openEdit(it) },
                        { id: "del", label: "Excluir", danger: true, onClick: () => setDeleteTarget(it) },
                      ]}
                    />
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {formOpen
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={() => setFormOpen(false)}>
              <div className="modal modal--wide vault-modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <h3>{editing ? "Editar registo" : "Novo registo"}</h3>
                <div className="vault-kinds">
                  <p className="vault-kinds__label">Credenciais</p>
                  <div className="vault-kinds__row">
                    {CREDENTIAL_KINDS.map((k) => (
                      <button
                        key={k}
                        type="button"
                        className={`vault-kind-btn${form.kind === k ? " is-on" : ""}`}
                        onClick={() => setForm((f) => ({ ...f, kind: k }))}
                      >
                        {KIND_LABEL[k]}
                      </button>
                    ))}
                  </div>
                  <div className="vault-kinds__row" style={{ marginTop: 10 }}>
                    {TEXT_KINDS.map((k) => (
                      <button
                        key={k}
                        type="button"
                        className={`vault-kind-btn${form.kind === k ? " is-on" : ""}`}
                        onClick={() => setForm((f) => ({ ...f, kind: k }))}
                      >
                        {KIND_LABEL[k]}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="fleet-form-grid vault-form">
                  {admin ? (
                    <label>
                      Dono
                      <select
                        className="input"
                        value={form.owner_user_id}
                        onChange={(e) => setForm((f) => ({ ...f, owner_user_id: e.target.value }))}
                      >
                        <option value="">Seleccione o usuário</option>
                        {users.map((u) => (
                          <option key={u.id} value={u.id}>
                            {u.label}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                  <label>
                    Título{isTextKind(form.kind) ? "*" : " (opcional)"}
                    <input
                      className="input"
                      value={form.title}
                      onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                      placeholder={isTextKind(form.kind) ? "Ex.: Orçamento POP Centro" : "Ex.: SSH POP Centro"}
                    />
                  </label>
                  {isTextKind(form.kind) ? (
                    <label className="vault-span">
                      Texto*
                      <textarea
                        className="input"
                        rows={8}
                        value={form.content}
                        onChange={(e) => setForm((f) => ({ ...f, content: e.target.value }))}
                        placeholder="Conteúdo livre do registo…"
                      />
                    </label>
                  ) : null}
                  {!isTextKind(form.kind) && form.kind === "equipment" ? (
                    <label className="vault-span">
                      Equipamento
                      <input
                        className="input"
                        value={deviceSearch}
                        onChange={(e) => setDeviceSearch(e.target.value)}
                        placeholder="Filtrar por nome ou IP…"
                      />
                      <select
                        className="input"
                        value={form.device_id}
                        onChange={(e) => setForm((f) => ({ ...f, device_id: e.target.value }))}
                        size={8}
                      >
                        <option value="">Seleccione…</option>
                        {filteredDevices.map((d) => (
                          <option key={d.id} value={d.id}>
                            {d.description}
                            {d.ip ? ` — ${d.ip}` : ""} {d.category ? `(${d.category})` : ""}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                  {!isTextKind(form.kind) && form.kind === "server" ? (
                    <label className="vault-span">
                      IP ou host
                      <input
                        className="input"
                        value={form.host}
                        onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))}
                        placeholder="10.0.0.10 ou nas.empresa.local"
                      />
                    </label>
                  ) : null}
                  {!isTextKind(form.kind) && form.kind === "site" ? (
                    <label className="vault-span">
                      Domínio
                      <input
                        className="input"
                        value={form.domain}
                        onChange={(e) => setForm((f) => ({ ...f, domain: e.target.value }))}
                        placeholder="painel.provedor.com"
                      />
                    </label>
                  ) : null}
                  {!isTextKind(form.kind) ? (
                    <>
                  <label className="vault-span">
                    O que guardar
                    <select
                      className="input"
                      value={form.mode}
                      onChange={(e) => setForm((f) => ({ ...f, mode: e.target.value as Mode }))}
                    >
                      <option value="user_password">Usuário e senha</option>
                      <option value="password">Somente senha</option>
                    </select>
                  </label>
                  {form.mode === "user_password" ? (
                    <label>
                      Usuário
                      <input
                        className="input"
                        value={form.username}
                        onChange={(e) => setForm((f) => ({ ...f, username: e.target.value }))}
                        autoComplete="off"
                      />
                    </label>
                  ) : null}
                  <label>
                    Senha{editing ? " (vazio = manter)" : ""}
                    <input
                      className="input"
                      type="password"
                      value={form.password}
                      onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
                      autoComplete="new-password"
                    />
                  </label>
                    </>
                  ) : null}
                  <label className="vault-span">
                    Notas
                    <textarea
                      className="input"
                      rows={2}
                      value={form.notes}
                      onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))}
                    />
                  </label>
                </div>
                <div className="row" style={{ justifyContent: "flex-end", marginTop: 12, gap: 8 }}>
                  <button type="button" className="btn" onClick={() => setFormOpen(false)}>
                    Cancelar
                  </button>
                  <button type="button" className="btn btn--primary" disabled={save.isPending} onClick={() => save.mutate()}>
                    {save.isPending ? "A guardar…" : "Guardar"}
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}

      {reveal
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={() => setReveal(null)}>
              <div
                className={`modal vault-reveal${"content" in reveal ? " vault-reveal--content" : ""}`}
                role="dialog"
                aria-modal="true"
                onMouseDown={(e) => e.stopPropagation()}
              >
                {"content" in reveal ? (
                  <>
                    <h3>Conteúdo — {KIND_LABEL[reveal.kind]}</h3>
                    <textarea className="input vault-reveal__textarea" readOnly rows={16} value={reveal.content} />
                    <div className="row" style={{ justifyContent: "flex-end", marginTop: 12, gap: 8 }}>
                      <button
                        type="button"
                        className="btn"
                        onClick={() => void copyText(reveal.content).then((ok) => ok && toastOk(push, "Texto copiado."))}
                      >
                        Copiar
                      </button>
                      <button type="button" className="btn" onClick={() => setReveal(null)}>
                        Fechar
                      </button>
                    </div>
                  </>
                ) : (
                  <>
                <h3>
                  <KeyRound size={18} aria-hidden /> Senha
                </h3>
                {reveal.username ? (
                  <label>
                    Usuário
                    <div className="vault-secret-row">
                      <input className="input" readOnly value={reveal.username} />
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Copiar usuário"
                        onClick={() => void copyText(reveal.username ?? "").then((ok) => ok && toastOk(push, "Usuário copiado."))}
                      >
                        <Copy size={16} />
                      </button>
                    </div>
                  </label>
                ) : (
                  <p className="muted">Este registo tem somente senha.</p>
                )}
                <label>
                  Senha
                  <div className="vault-secret-row">
                    <input className="input" readOnly type={showPass ? "text" : "password"} value={reveal.password} />
                    <button type="button" className="btn btn--icon" title={showPass ? "Ocultar" : "Mostrar"} onClick={() => setShowPass((v) => !v)}>
                      {showPass ? <EyeOff size={16} /> : <Eye size={16} />}
                    </button>
                    <button
                      type="button"
                      className="btn btn--icon"
                      title="Copiar senha"
                      onClick={() => void copyText(reveal.password).then((ok) => ok && toastOk(push, "Senha copiada."))}
                    >
                      <Copy size={16} />
                    </button>
                  </div>
                </label>
                <div className="row" style={{ justifyContent: "flex-end", marginTop: 12 }}>
                  <button type="button" className="btn" onClick={() => setReveal(null)}>
                    Fechar
                  </button>
                </div>
                  </>
                )}
              </div>
            </div>,
            document.body,
          )
        : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Excluir registo"
        message={deleteTarget ? `Remover «${deleteTarget.title || targetOf(deleteTarget)}»?` : ""}
        confirmLabel="Excluir"
        danger
        busy={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && remove.mutate(deleteTarget.id)}
      />

      {exportModalOpen
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={() => !exporting && setExportModalOpen(false)}>
              <div className="modal" style={{ maxWidth: 480 }} role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <h3>Exportar registos (CSV)</h3>
                <div className="msg msg--warn" style={{ fontSize: 12.5, lineHeight: 1.5 }}>
                  Este ficheiro pode conter <strong>dados sensíveis</strong> (senhas, strings de autenticação) se optar por
                  incluir as senhas abaixo. Guarde-o em local seguro e não o partilhe por canais inseguros — o sistema não
                  se responsabiliza por vazamento de dados a partir de um ficheiro exportado.
                </div>

                <label className="row" style={{ gap: 8, marginTop: 14, alignItems: "flex-start", cursor: "pointer" }}>
                  <input
                    type="checkbox"
                    checked={exportIncludePasswords}
                    onChange={(e) => setExportIncludePasswords(e.target.checked)}
                    style={{ marginTop: 3 }}
                  />
                  <span style={{ fontSize: 13, lineHeight: 1.4 }}>
                    Incluir as senhas numa coluna do CSV (senão, exporta só tipo/destino/acesso/data, como hoje).
                  </span>
                </label>

                <label className="row" style={{ gap: 8, marginTop: 10, alignItems: "flex-start", cursor: "pointer" }}>
                  <input
                    type="checkbox"
                    checked={exportAware}
                    onChange={(e) => setExportAware(e.target.checked)}
                    style={{ marginTop: 3 }}
                  />
                  <span style={{ fontSize: 13, lineHeight: 1.4 }}>
                    Estou ciente de que este ficheiro pode conter dados sensíveis e assumo a responsabilidade pela sua
                    guarda e partilha.
                  </span>
                </label>

                <div className="row" style={{ justifyContent: "flex-end", marginTop: 16, gap: 8 }}>
                  <button type="button" className="btn" disabled={exporting} onClick={() => setExportModalOpen(false)}>
                    Cancelar
                  </button>
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={exporting || !exportAware}
                    onClick={() => void confirmExport()}
                  >
                    {exporting ? "A exportar…" : "Exportar"}
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </div>
  );
}
