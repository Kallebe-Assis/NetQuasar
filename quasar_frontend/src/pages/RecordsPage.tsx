import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, Copy, Eye, EyeOff, KeyRound } from "lucide-react";
import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ActionMenu } from "../components/ActionMenu";
import { ConfirmModal } from "../components/ConfirmModal";
import { InfoHint } from "../components/InfoHint";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { isAdminUser } from "../lib/auth";
import { toastErr, toastOk } from "../lib/operationToast";
import { queryKeys } from "../lib/queryKeys";

type Kind = "equipment" | "server" | "site";
type Mode = "user_password" | "password";

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
};

function emptyForm(): Form {
  return {
    owner_user_id: "",
    kind: "equipment",
    title: "",
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
  if (it.kind === "equipment") {
    const ip = it.device_ip ? ` (${it.device_ip})` : "";
    return `${it.device_name || it.title || "Equipamento"}${ip}`;
  }
  if (it.kind === "server") return it.host || it.title || "Servidor";
  return it.domain || it.title || "Site";
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

  const [q, setQ] = useState("");
  const [kind, setKind] = useState("");
  const [ownerId, setOwnerId] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState<Form>(emptyForm());
  const [deviceSearch, setDeviceSearch] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<RecordItem | null>(null);
  const [reveal, setReveal] = useState<{ id: string; username: string | null; password: string } | null>(null);
  const [showPass, setShowPass] = useState(false);

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
    if (users.length === 1) f.owner_user_id = users[0].id;
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
      const payload = {
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
      toastOk(push, editing ? "Registo actualizado." : "Registo guardado.");
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
      apiFetch<{ username?: string | null; password: string }>(`/api/v1/credential-records/${id}/reveal`, {
        method: "POST",
      }),
    onSuccess: (data, id) => {
      setShowPass(true);
      setReveal({ id, username: data.username ?? null, password: data.password });
    },
    onError: (e) => toastErr(push, e),
  });

  const colSpan = admin ? 6 : 5;

  return (
    <div className="vault-page">
      <div className="page-heading">
        <h1>
          Registros
          <InfoHint>
            {admin
              ? "Cofre de senhas de equipamentos, servidores e sites. Como administrador vê todos os registos e pode filtrar por utilizador. As senhas ficam cifradas; use «Ver senha» para revelar."
              : "O seu cofre de senhas de equipamentos, servidores e sites. Só você (e os administradores) vê estes registos. As senhas ficam cifradas; use «Ver senha» para revelar."}
          </InfoHint>
        </h1>
        <PageCountPill label="registos" count={listQ.data?.total ?? items.length} />
      </div>

      <div className="netev-toolbar">
        <input
          className="input"
          placeholder="Pesquisar título, host, domínio…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <select className="input" value={kind} onChange={(e) => setKind(e.target.value)} aria-label="Tipo">
          <option value="">Todos os tipos</option>
          <option value="equipment">Equipamento</option>
          <option value="server">Servidor</option>
          <option value="site">Site</option>
        </select>
        {admin ? (
          <select className="input" value={ownerId} onChange={(e) => setOwnerId(e.target.value)} aria-label="Utilizador">
            <option value="">Todos os utilizadores</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.label}
              </option>
            ))}
          </select>
        ) : null}
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
              {admin ? <th>Utilizador</th> : null}
              <th>Actualizado</th>
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
                  Nenhum registo. Guarde senhas de equipamento, servidor ou site.
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
                  <td>{it.has_username ? it.username : <span className="muted">somente senha</span>}</td>
                  {admin ? <td>{it.owner_name}</td> : null}
                  <td className="netev-when">{formatWhen(it.updated_at)}</td>
                  <td>
                    <ActionMenu
                      items={[
                        { id: "reveal", label: "Ver senha", onClick: () => revealMut.mutate(it.id) },
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
                  {(["equipment", "server", "site"] as Kind[]).map((k) => (
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
                <div className="fleet-form-grid vault-form">
                  {admin ? (
                    <label>
                      Dono
                      <select
                        className="input"
                        value={form.owner_user_id}
                        onChange={(e) => setForm((f) => ({ ...f, owner_user_id: e.target.value }))}
                      >
                        <option value="">Seleccione o utilizador</option>
                        {users.map((u) => (
                          <option key={u.id} value={u.id}>
                            {u.label}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                  <label>
                    Título (opcional)
                    <input
                      className="input"
                      value={form.title}
                      onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                      placeholder="Ex.: SSH POP Centro"
                    />
                  </label>
                  {form.kind === "equipment" ? (
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
                  {form.kind === "server" ? (
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
                  {form.kind === "site" ? (
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
                  <label className="vault-span">
                    O que guardar
                    <select
                      className="input"
                      value={form.mode}
                      onChange={(e) => setForm((f) => ({ ...f, mode: e.target.value as Mode }))}
                    >
                      <option value="user_password">Utilizador e senha</option>
                      <option value="password">Somente senha</option>
                    </select>
                  </label>
                  {form.mode === "user_password" ? (
                    <label>
                      Utilizador
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
              <div className="modal vault-reveal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <h3>
                  <KeyRound size={18} aria-hidden /> Senha
                </h3>
                {reveal.username ? (
                  <label>
                    Utilizador
                    <div className="vault-secret-row">
                      <input className="input" readOnly value={reveal.username} />
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Copiar utilizador"
                        onClick={() => void copyText(reveal.username ?? "").then((ok) => ok && toastOk(push, "Utilizador copiado."))}
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
              </div>
            </div>,
            document.body,
          )
        : null}

      <ConfirmModal
        open={!!deleteTarget}
        title="Excluir registo"
        message={deleteTarget ? `Remover «${deleteTarget.title || targetOf(deleteTarget)}»? A senha deixa de estar disponível.` : ""}
        confirmLabel="Excluir"
        danger
        busy={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && remove.mutate(deleteTarget.id)}
      />
    </div>
  );
}
