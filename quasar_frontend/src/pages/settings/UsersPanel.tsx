import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Filter, Plus, Shield } from "lucide-react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { ActionMenu } from "../../components/ActionMenu";
import { apiFetch, ApiError } from "../../lib/api";
import { isAdminUser } from "../../lib/auth";
import { PermissionProfilesModal } from "./PermissionProfilesModal";
import type { PermissionProfile } from "../../lib/permissions";
import { formatBRPhoneDisplay, normalizeBRPhoneForApi, validateBRPhoneMessage } from "../../lib/brPhone";

type UserRow = {
  id: string;
  display_name: string;
  email: string;
  phone?: string | null;
  role: string;
  is_active?: boolean;
  permission_profile_id?: string | null;
  permission_profile_name?: string | null;
  permission_profile_slug?: string | null;
};

export function UsersPanel() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["settings-users"], queryFn: () => apiFetch<{ users: UserRow[] }>("/api/v1/settings/users") });
  const profiles = useQuery({
    queryKey: ["permission-profiles"],
    queryFn: () => apiFetch<{ profiles: PermissionProfile[] }>("/api/v1/settings/permission-profiles"),
    staleTime: 30_000,
  });
  const [search, setSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState<"all" | string>("all");
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "inactive">("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [profilesOpen, setProfilesOpen] = useState(false);
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [profileId, setProfileId] = useState("");
  const [editId, setEditId] = useState<string | null>(null);
  const [eName, setEName] = useState("");
  const [eEmail, setEEmail] = useState("");
  const [ePhone, setEPhone] = useState("");
  const [ePass, setEPass] = useState("");
  const [eProfileId, setEProfileId] = useState("");
  const [confirmAction, setConfirmAction] = useState<null | { type: "delete" | "activate" | "deactivate" | "force_logout"; user: UserRow }>(null);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const { push: pushToast } = useAppToast();
  const [userCreateErr, setUserCreateErr] = useState("");

  const profileOptions = profiles.data?.profiles ?? [];
  const defaultUserProfileId = useMemo(
    () => profileOptions.find((p) => p.slug === "user")?.id ?? profileOptions[0]?.id ?? "",
    [profileOptions],
  );

  useEffect(() => {
    if (!profileId && defaultUserProfileId) setProfileId(defaultUserProfileId);
  }, [profileId, defaultUserProfileId]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (list.data?.users ?? []).filter((u) => {
      const active = u.is_active !== false;
      if (roleFilter !== "all") {
        const slug = u.permission_profile_slug ?? (u.role === "admin" ? "admin" : "user");
        if (slug !== roleFilter && u.permission_profile_id !== roleFilter) return false;
      }
      if (statusFilter === "active" && !active) return false;
      if (statusFilter === "inactive" && active) return false;
      if (!q) return true;
      const hay = `${u.display_name} ${u.email} ${u.phone ?? ""} ${u.role} ${u.permission_profile_name ?? ""}`.toLowerCase();
      return hay.includes(q);
    });
  }, [list.data?.users, search, roleFilter, statusFilter]);

  const create = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/settings/users", {
        method: "POST",
        json: {
          display_name: displayName.trim(),
          email: email.trim(),
          phone: normalizeBRPhoneForApi(phone),
          password,
          permission_profile_id: profileId || undefined,
        },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-users"] });
      setUserCreateErr("");
      setDisplayName("");
      setEmail("");
      setPhone("");
      setPassword("");
      setProfileId(defaultUserProfileId);
      setCreateOpen(false);
      toastOk(pushToast, "Usuário criado com sucesso.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao criar usuário."),
  });

  const patch = useMutation({
    mutationFn: () => {
      const body: Record<string, string> = {};
      if (eName.trim()) body.display_name = eName.trim();
      if (eEmail.trim()) body.email = eEmail.trim();
      body.phone = normalizeBRPhoneForApi(ePhone);
      if (ePass) body.password = ePass;
      if (eProfileId) body.permission_profile_id = eProfileId;
      return apiFetch(`/api/v1/settings/users/${editId}`, { method: "PATCH", json: body });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-users"] });
      setEditId(null);
      toastOk(pushToast, "Guardado com sucesso (usuário).");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar (usuário)."),
  });

  const setActive = useMutation({
    mutationFn: ({ id, is_active }: { id: string; is_active: boolean }) =>
      apiFetch<{ ok?: boolean; is_active?: boolean }>(`/api/v1/settings/users/${id}`, {
        method: "PATCH",
        json: { is_active },
      }),
    onSuccess: (_d, vars) => {
      qc.setQueryData<{ users: UserRow[] }>(["settings-users"], (prev) => {
        if (!prev?.users) return prev;
        return {
          ...prev,
          users: prev.users.map((u) => (u.id === vars.id ? { ...u, is_active: vars.is_active } : u)),
        };
      });
      void qc.invalidateQueries({ queryKey: ["settings-users"] });
      setConfirmAction(null);
      toastOk(pushToast, vars.is_active ? "Usuário activado." : "Usuário inactivado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao alterar estado do usuário."),
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/settings/users/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-users"] });
      setConfirmAction(null);
      toastOk(pushToast, "Usuário removido.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao remover usuário."),
  });

  // Marca users.sessions_invalidated_at="agora" no backend — o próximo pedido do usuário-alvo
  // com o token actual recebe 401 e o frontend dele redireciona automaticamente para o login
  // (ver apiFetch em lib/api.ts). Não existe "socket" para desconectar em tempo real: efectiva-se
  // na próxima acção que essa pessoa fizer no sistema.
  const forceLogout = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/settings/users/${id}/force-logout`, { method: "POST" }),
    onSuccess: () => {
      setConfirmAction(null);
      toastOk(pushToast, "Desconexão forçada — a sessão actual deste usuário será encerrada na próxima ação dele.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao forçar desconexão."),
  });

  const profileLabel = (u: UserRow) =>
    u.permission_profile_name || (u.role === "admin" ? "Administrador" : "Usuário");

  if (list.isLoading) return <p>A carregar…</p>;
  if (list.isError) {
    const ae = list.error as ApiError;
    if (ae?.status === 403) {
      return <p style={{ color: "var(--muted)" }}>Apenas administradores podem gerir usuários.</p>;
    }
    return <div className="msg msg--err">{(list.error as Error).message}</div>;
  }

  return (
    <>
      <p style={{ color: "var(--muted)", fontSize: 13, margin: "0 0 12px", maxWidth: 720 }}>
        Gestão de acessos e perfis de permissão. Novos usuários só podem ser criados aqui (não existe registo público).
      </p>

      <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "flex-end", marginBottom: 12 }}>
        <div className="field" style={{ margin: 0, flex: "1 1 640px", minWidth: 360, maxWidth: "100%" }}>
          <label style={{ fontSize: 11 }}>Pesquisar</label>
          <input
            className="input"
            style={{ width: "100%" }}
            placeholder="Nome, e-mail ou telefone…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <button
          type="button"
          className={`btn btn--icon-menu${filtersOpen || roleFilter !== "all" || statusFilter !== "all" ? " btn--primary" : ""}`}
          title={roleFilter !== "all" || statusFilter !== "all" ? "Filtros activos" : "Filtros"}
          aria-label="Filtros"
          aria-pressed={filtersOpen}
          onClick={() => setFiltersOpen((v) => !v)}
        >
          <Filter size={16} aria-hidden />
        </button>
        <button type="button" className="btn" onClick={() => setProfilesOpen(true)}>
          <Shield size={16} aria-hidden /> Perfis de permissão
        </button>
        <button type="button" className="btn btn--primary" onClick={() => setCreateOpen(true)}>
          <Plus size={16} aria-hidden /> Novo usuário
        </button>
      </div>

      {filtersOpen && (
        <div
          className="row"
          style={{
            flexWrap: "wrap",
            gap: 8,
            alignItems: "flex-end",
            marginBottom: 12,
            paddingTop: 4,
          }}
        >
          <div className="field" style={{ margin: 0, minWidth: 180 }}>
            <label style={{ fontSize: 11 }}>Perfil</label>
            <select className="input" value={roleFilter} onChange={(e) => setRoleFilter(e.target.value)}>
              <option value="all">Todos</option>
              {profileOptions.map((p) => (
                <option key={p.id} value={p.slug}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
          <div className="field" style={{ margin: 0, minWidth: 160 }}>
            <label style={{ fontSize: 11 }}>Estado</label>
            <select
              className="input"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
            >
              <option value="all">Todos</option>
              <option value="active">Activos</option>
              <option value="inactive">Inactivos</option>
            </select>
          </div>
        </div>
      )}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Nome</th>
              <th>E-mail</th>
              <th>Telefone</th>
              <th>Perfil</th>
              <th>Estado</th>
              <th style={{ width: 56 }} />
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={6} style={{ color: "var(--muted)" }}>
                  Nenhum usuário encontrado.
                </td>
              </tr>
            ) : (
              filtered.map((u) => {
                const active = u.is_active !== false;
                return (
                  <tr key={u.id} style={active ? undefined : { opacity: 0.65 }}>
                    <td>{u.display_name}</td>
                    <td className="mono">{u.email}</td>
                    <td className="mono">{formatBRPhoneDisplay(u.phone)}</td>
                    <td>{profileLabel(u)}</td>
                    <td>
                      <span className={active ? "badge badge--ok" : "badge"}>{active ? "Activo" : "Inactivo"}</span>
                    </td>
                    <td>
                      <ActionMenu
                        title={`Opções de ${u.display_name}`}
                        items={[
                          {
                            id: "edit",
                            label: "Editar",
                            onClick: () => {
                              setEditId(u.id);
                              setEName(u.display_name);
                              setEEmail(u.email);
                              setEPhone(u.phone ?? "");
                              setEProfileId(
                                u.permission_profile_id ||
                                  profileOptions.find((p) => p.slug === (u.role === "admin" ? "admin" : "user"))?.id ||
                                  "",
                              );
                              setEPass("");
                            },
                          },
                          {
                            id: "toggle",
                            label: active ? "Inactivar" : "Activar",
                            onClick: () => setConfirmAction({ type: active ? "deactivate" : "activate", user: u }),
                          },
                          ...(isAdminUser()
                            ? [
                                {
                                  id: "force_logout",
                                  label: "Forçar desconexão",
                                  onClick: () => setConfirmAction({ type: "force_logout" as const, user: u }),
                                },
                              ]
                            : []),
                          {
                            id: "delete",
                            label: "Apagar",
                            danger: true,
                            onClick: () => setConfirmAction({ type: "delete", user: u }),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {createOpen && (
        <div className="modal-backdrop" style={{ zIndex: 60 }} onClick={() => !create.isPending && setCreateOpen(false)}>
          <div className="card" style={{ width: "min(520px, 94vw)", margin: "8vh auto" }} onClick={(e) => e.stopPropagation()}>
            <h2 style={{ marginTop: 0 }}>Novo usuário</h2>
            <div style={{ display: "grid", gap: 10 }}>
              <div className="field" style={{ margin: 0 }}>
                <label>Nome</label>
                <input className="input" placeholder="Nome completo" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>E-mail</label>
                <input className="input" type="email" placeholder="email@empresa.com" value={email} onChange={(e) => setEmail(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>Telefone</label>
                <input className="input" placeholder="(11) 98765-4321" value={phone} onChange={(e) => setPhone(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>Palavra-passe</label>
                <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>Perfil de permissão</label>
                <select className="input" value={profileId} onChange={(e) => setProfileId(e.target.value)}>
                  {profileOptions.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                      {p.is_system ? " (sistema)" : ""}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            {userCreateErr ? <div className="msg msg--err">{userCreateErr}</div> : null}
            {create.isError && <div className="msg msg--err">{(create.error as Error).message}</div>}
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
              <button type="button" className="btn" disabled={create.isPending} onClick={() => setCreateOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={create.isPending}
                onClick={() => {
                  setUserCreateErr("");
                  const pe = validateBRPhoneMessage(phone);
                  if (pe) {
                    setUserCreateErr(pe);
                    return;
                  }
                  if (!displayName.trim() || !email.trim() || !password) {
                    setUserCreateErr("Preencha nome, e-mail, telefone e palavra-passe.");
                    return;
                  }
                  if (!profileId) {
                    setUserCreateErr("Seleccione um perfil de permissão.");
                    return;
                  }
                  create.mutate();
                }}
              >
                Criar usuário
              </button>
            </div>
          </div>
        </div>
      )}

      {editId && (
        <div className="modal-backdrop" style={{ zIndex: 60 }} onClick={() => !patch.isPending && setEditId(null)}>
          <div className="card" style={{ width: "min(520px, 94vw)", margin: "8vh auto" }} onClick={(e) => e.stopPropagation()}>
            <h2 style={{ marginTop: 0 }}>Editar usuário</h2>
            <div style={{ display: "grid", gap: 10 }}>
              <div className="field" style={{ margin: 0 }}>
                <label>Nome</label>
                <input className="input" value={eName} onChange={(e) => setEName(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>E-mail</label>
                <input className="input" type="email" value={eEmail} onChange={(e) => setEEmail(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>Telefone</label>
                <input className="input" value={ePhone} onChange={(e) => setEPhone(e.target.value)} placeholder="(11) 98765-4321" />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>Nova palavra-passe (opcional)</label>
                <input className="input" type="password" value={ePass} onChange={(e) => setEPass(e.target.value)} />
              </div>
              <div className="field" style={{ margin: 0 }}>
                <label>Perfil de permissão</label>
                <select className="input" value={eProfileId} onChange={(e) => setEProfileId(e.target.value)}>
                  {profileOptions.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                      {p.is_system ? " (sistema)" : ""}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            {patch.isError && <div className="msg msg--err">{(patch.error as Error).message}</div>}
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
              <button type="button" className="btn" disabled={patch.isPending} onClick={() => setEditId(null)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={patch.isPending}
                onClick={() => {
                  const pe = validateBRPhoneMessage(ePhone);
                  if (pe) {
                    toastErr(pushToast, new Error(pe));
                    return;
                  }
                  patch.mutate();
                }}
              >
                Salvar
              </button>
            </div>
          </div>
        </div>
      )}

      {confirmAction && (
        <div
          className="modal-backdrop"
          style={{ zIndex: 70 }}
          onClick={() => !(setActive.isPending || del.isPending || forceLogout.isPending) && setConfirmAction(null)}
        >
          <div className="card" style={{ width: "min(420px, 92vw)", margin: "12vh auto" }} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>
              {confirmAction.type === "delete"
                ? "Apagar usuário"
                : confirmAction.type === "activate"
                  ? "Activar usuário"
                  : confirmAction.type === "force_logout"
                    ? "Forçar desconexão"
                    : "Inactivar usuário"}
            </h3>
            <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 14px" }}>
              {confirmAction.type === "delete"
                ? `Eliminar permanentemente ${confirmAction.user.email}?`
                : confirmAction.type === "activate"
                  ? `Activar o acesso de ${confirmAction.user.email}?`
                  : confirmAction.type === "force_logout"
                    ? `Encerrar a sessão actual de ${confirmAction.user.email}? A pessoa é desconectada assim que fizer a próxima ação no sistema e precisa iniciar sessão novamente.`
                    : `Inactivar ${confirmAction.user.email}? O usuário não poderá iniciar sessão.`}
            </p>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8 }}>
              <button
                type="button"
                className="btn"
                disabled={setActive.isPending || del.isPending || forceLogout.isPending}
                onClick={() => setConfirmAction(null)}
              >
                Cancelar
              </button>
              <button
                type="button"
                className={confirmAction.type === "delete" ? "btn btn--danger" : "btn btn--primary"}
                disabled={setActive.isPending || del.isPending || forceLogout.isPending}
                onClick={() => {
                  if (confirmAction.type === "delete") del.mutate(confirmAction.user.id);
                  else if (confirmAction.type === "force_logout") forceLogout.mutate(confirmAction.user.id);
                  else setActive.mutate({ id: confirmAction.user.id, is_active: confirmAction.type === "activate" });
                }}
              >
                Confirmar
              </button>
            </div>
          </div>
        </div>
      )}

      <PermissionProfilesModal open={profilesOpen} onClose={() => setProfilesOpen(false)} />
    </>
  );
}

