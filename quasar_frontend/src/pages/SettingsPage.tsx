import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Blend, ClockFading, Cpu, Filter, Plus, Shield, Sun, ThermometerSun } from "lucide-react";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { InfoHint } from "../components/InfoHint";
import { ActionMenu } from "../components/ActionMenu";
import { apiFetch, ApiError } from "../lib/api";
import { invalidateAlertListQueries, queryKeys } from "../lib/queryKeys";
import { can, isAdminUser } from "../lib/auth";
import { AppearancePanel } from "./settings/AppearancePanel";
import { AlertNotificationPrefsPanel } from "./settings/AlertNotificationPrefsPanel";
import { MonitoringSettingsPanel } from "./settings/MonitoringPipelinePanel";
import { AuditingPanel } from "./settings/AuditingPanel";
import { ScheduledReportsPanel } from "./settings/ScheduledReportsPanel";
import { MikrotikSettingsPanel } from "./settings/MikrotikSettingsPanel";
import { SwitchSettingsPanel } from "./settings/SwitchSettingsPanel";
import { BngCollectionPanel } from "./settings/BngCollectionPanel";
import { DatabasePanel } from "./settings/DatabasePanel";
import { FleetSettingsPanel } from "./settings/FleetSettingsPanel";
import { OltVendorsPanel } from "./settings/OltVendorsPanel";
import { PermissionProfilesModal } from "./settings/PermissionProfilesModal";
import type { PermissionProfile } from "../lib/permissions";
import { formatBRPhoneDisplay, normalizeBRPhoneForApi, validateBRPhoneMessage } from "../lib/brPhone";

type SettingsTab =
  | "database"
  | "logs"
  | "users"
  | "alerts"
  | "monitoring"
  | "appearance"
  | "connection"
  | "telegram"
  | "olt"
  | "mikrotik"
  | "switch"
  | "bng"
  | "fleet"
  | "automation";

const SETTINGS_TABS: SettingsTab[] = [
  "database", "logs", "users", "alerts", "monitoring", "appearance", "connection",
  "telegram", "olt", "mikrotik", "switch", "bng", "fleet", "automation",
];

const SETTINGS_TAB_LABELS: Record<SettingsTab, string> = {
  database: "Base de dados",
  logs: "Auditoria",
  users: "Usuários",
  alerts: "Alertas",
  monitoring: "Monitoramento",
  appearance: "Aparência",
  connection: "Rede e SNMP",
  telegram: "Telegram",
  olt: "Perfis OLT",
  mikrotik: "MikroTik",
  switch: "Switch",
  bng: "BNG",
  fleet: "Frota",
  automation: "Automações",
};

function canSeeSettingsTab(tab: SettingsTab): boolean {
  if (isAdminUser()) return true;
  if (tab === "appearance" || tab === "alerts") return true;
  if (tab === "users" || tab === "logs") return can("settings.users") || can("settings.permissions");
  if (tab === "database" || tab === "connection") return can("settings.system");
  if (tab === "monitoring" || tab === "olt" || tab === "mikrotik" || tab === "switch" || tab === "bng") {
    return can("settings.monitoring");
  }
  if (tab === "telegram" || tab === "automation") return can("settings.notifications");
  if (tab === "fleet") return can("fleet.manage") || can("settings.system");
  return can("settings.view");
}

function canEditAlertThresholds(): boolean {
  return isAdminUser() || can("alerts.manage") || can("settings.monitoring");
}

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const visibleTabs = useMemo(() => SETTINGS_TABS.filter(canSeeSettingsTab), []);
  const [tab, setTab] = useState<SettingsTab>(() => {
    const raw = searchParams.get("tab");
    if (SETTINGS_TABS.includes(raw as SettingsTab) && canSeeSettingsTab(raw as SettingsTab)) {
      return raw as SettingsTab;
    }
    return visibleTabs[0] ?? "appearance";
  });

  function selectTab(next: SettingsTab) {
    setTab(next);
    const params = new URLSearchParams(searchParams);
    params.set("tab", next);
    setSearchParams(params, { replace: true });
  }
  return (
    <>
      <h1>Configurações</h1>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Base de dados, usuários, credenciais de rede, Telegram (alertas e relatórios), perfis OLT por marca/modelo, coleta MikroTik/Switch/BNG e relatórios automáticos.
      </p>
      <div className="tabs" style={{ flexWrap: "wrap" }}>
        {visibleTabs.map((k) => (
          <button key={k} type="button" className={tab === k ? "active" : ""} onClick={() => selectTab(k)}>
            {SETTINGS_TAB_LABELS[k]}
          </button>
        ))}
      </div>
      {tab === "database" && <DatabasePanel />}
      {tab === "logs" && <AuditingPanel />}
      {tab === "users" && <UsersPanel />}
      {tab === "alerts" && (
        <>
          <AlertNotificationPrefsPanel />
          {canEditAlertThresholds() ? <AlertThresholdsPanel /> : null}
        </>
      )}
      {tab === "monitoring" && <MonitoringSettingsPanel />}
      {tab === "appearance" && <AppearancePanel />}
      {tab === "connection" && <ConnectionPanel />}
      {tab === "telegram" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <TelegramPanel id="monitoring" title="Monitorização (alertas)" />
          <TelegramPanel id="reports" title="Relatórios" />
        </div>
      )}
      {tab === "olt" && <OltVendorsPanel />}
      {tab === "mikrotik" && <MikrotikSettingsPanel />}
      {tab === "switch" && <SwitchSettingsPanel />}
      {tab === "bng" && <BngCollectionPanel />}
      {tab === "fleet" && <FleetSettingsPanel />}
      {tab === "automation" && <ScheduledReportsPanel />}
    </>
  );
}


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

function UsersPanel() {
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
  const [confirmAction, setConfirmAction] = useState<null | { type: "delete" | "activate" | "deactivate"; user: UserRow }>(null);
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
      toastOk(pushToast, "Utilizador criado com sucesso.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao criar utilizador."),
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
      toastOk(pushToast, vars.is_active ? "Utilizador activado." : "Utilizador inactivado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao alterar estado do utilizador."),
  });

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/settings/users/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-users"] });
      setConfirmAction(null);
      toastOk(pushToast, "Utilizador removido.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao remover utilizador."),
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
                  Nenhum utilizador encontrado.
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
        <div className="modal-backdrop" style={{ zIndex: 70 }} onClick={() => !(setActive.isPending || del.isPending) && setConfirmAction(null)}>
          <div className="card" style={{ width: "min(420px, 92vw)", margin: "12vh auto" }} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>
              {confirmAction.type === "delete"
                ? "Apagar utilizador"
                : confirmAction.type === "activate"
                  ? "Activar utilizador"
                  : "Inactivar utilizador"}
            </h3>
            <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 14px" }}>
              {confirmAction.type === "delete"
                ? `Eliminar permanentemente ${confirmAction.user.email}?`
                : confirmAction.type === "activate"
                  ? `Activar o acesso de ${confirmAction.user.email}?`
                  : `Inactivar ${confirmAction.user.email}? O utilizador não poderá iniciar sessão.`}
            </p>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8 }}>
              <button
                type="button"
                className="btn"
                disabled={setActive.isPending || del.isPending}
                onClick={() => setConfirmAction(null)}
              >
                Cancelar
              </button>
              <button
                type="button"
                className={confirmAction.type === "delete" ? "btn btn--danger" : "btn btn--primary"}
                disabled={setActive.isPending || del.isPending}
                onClick={() => {
                  if (confirmAction.type === "delete") del.mutate(confirmAction.user.id);
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


type AlertThresholdMetric = {
  id: string;
  label: string;
  unit: string;
  scope: string;
  enabled?: boolean;
  operator: "gte" | "lte";
  green_min: string;
  warning_min: string;
  critical_min: string;
  /** Categorias de equipamento (base de dados) em minúsculas; vazio = todos. */
  apply_categories?: string[];
};

function equipScopeFromCategories(cats?: string[]): "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor" {
  const c = cats ?? [];
  if (c.length === 0) return "*";
  const low = c.map((x) => String(x).toLowerCase().trim());
  if (low.some((x) => x === "*" || x === "all" || x === "todos")) return "*";
  if (low.includes("olt") && low.length === 1) return "olt";
  if (low.includes("mikrotik") && low.length === 1) return "mikrotik";
  if (low.includes("bng") && low.length === 1) return "bng";
  if (low.includes("switch") && low.length === 1) return "switch";
  if (low.includes("servidor") || low.includes("outros")) return "servidor";
  return "*";
}

function categoriesFromEquipScope(scope: "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor"): string[] {
  switch (scope) {
    case "olt":
      return ["olt"];
    case "mikrotik":
      return ["mikrotik"];
    case "bng":
      return ["bng"];
    case "switch":
      return ["switch"];
    case "servidor":
      return ["servidor", "outros"];
    default:
      return [];
  }
}

/** Catálogo padrão de métricas da aba Configurações → Alertas.
 * IDs devem coincidir com os avaliadores no backend (`alertthresholds` / `monitorworker`).
 * operator: "gte" (≥) ou "lte" (≤) — nunca usar símbolos Unicode soltos (evita mojibake). */
function defaultAlertMetrics(): AlertThresholdMetric[] {
  return [
    { id: "cpu_usage_pct", label: "CPU utilizada", unit: "%", scope: "equipamento", enabled: true, operator: "gte", green_min: "50", warning_min: "75", critical_min: "90", apply_categories: [] },
    { id: "memory_usage_pct", label: "Memória utilizada", unit: "%", scope: "equipamento", enabled: true, operator: "gte", green_min: "55", warning_min: "75", critical_min: "90", apply_categories: [] },
    { id: "latency_ms", label: "Latência de resposta", unit: "ms", scope: "equipamento", enabled: true, operator: "gte", green_min: "50", warning_min: "120", critical_min: "220", apply_categories: [] },
    { id: "temperature_c", label: "Temperatura do equipamento", unit: "°C", scope: "equipamento", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: [] },
    { id: "uptime_minutes", label: "Uptime (minutos)", unit: "min", scope: "equipamento", enabled: true, operator: "lte", green_min: "120", warning_min: "60", critical_min: "15", apply_categories: [] },
    { id: "olt_pon_tx_dbm", label: "PON TX da OLT", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-8", warning_min: "-14", critical_min: "-20", apply_categories: ["olt"] },
    { id: "olt_pon_rx_dbm", label: "PON RX da OLT", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-10", warning_min: "-16", critical_min: "", apply_categories: ["olt"] },
    { id: "olt_onu_tx_dbm", label: "ONU TX por PON", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-8", warning_min: "-15", critical_min: "-20", apply_categories: ["olt"] },
    { id: "olt_onu_rx_dbm", label: "ONU RX por PON", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-12", warning_min: "-20", critical_min: "", apply_categories: ["olt"] },
    { id: "olt_pon_temp_c", label: "Temperatura da PON", unit: "°C", scope: "olt_pon", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: ["olt"] },
    { id: "olt_onu_drop_count", label: "Variação de ONUs online (por PON)", unit: "ONUs", scope: "olt_pon", enabled: true, operator: "gte", green_min: "0", warning_min: "2", critical_min: "5", apply_categories: ["olt"] },
    { id: "olt_onu_drop_percent", label: "Variação de ONUs online (%)", unit: "%", scope: "olt_pon", enabled: true, operator: "gte", green_min: "0", warning_min: "10", critical_min: "25", apply_categories: ["olt"] },
    { id: "bng_pppoe_drop_count", label: "Queda de PPPoE online (entre coletas)", unit: "sessões", scope: "bng", enabled: true, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_ipv4_drop_count", label: "Queda de IPv4 online (entre coletas)", unit: "sessões", scope: "bng", enabled: true, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_ipv6_drop_count", label: "Queda de IPv6 online (entre coletas)", unit: "sessões", scope: "bng", enabled: true, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_total_drop_count", label: "Queda de total online (entre coletas)", unit: "sessões", scope: "bng", enabled: false, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_dual_stack_drop_count", label: "Queda de dual-stack (entre coletas)", unit: "sessões", scope: "bng", enabled: false, operator: "gte", green_min: "0", warning_min: "20", critical_min: "80", apply_categories: ["bng"] },
    { id: "mikrotik_pppoe_drop_count", label: "Queda de sessões PPPoE MikroTik (entre coletas)", unit: "sessões", scope: "mikrotik_pppoe", enabled: true, operator: "gte", green_min: "0", warning_min: "10", critical_min: "30", apply_categories: ["mikrotik"] },
    { id: "iface_down_count", label: "Mudança de interface UP→DOWN", unit: "evento", scope: "interface", enabled: true, operator: "gte", green_min: "0", warning_min: "1", critical_min: "1", apply_categories: [] },
    { id: "mikrotik_sfp_tx_dbm", label: "SFP — potência TX", unit: "dBm", scope: "mikrotik_sfp", enabled: true, operator: "lte", green_min: "-8", warning_min: "-13", critical_min: "-18", apply_categories: ["mikrotik"] },
    { id: "mikrotik_sfp_rx_dbm", label: "SFP — potência RX", unit: "dBm", scope: "mikrotik_sfp", enabled: true, operator: "lte", green_min: "-10", warning_min: "-15", critical_min: "", apply_categories: ["mikrotik"] },
    { id: "mikrotik_sfp_temp_c", label: "Temperatura do módulo SFP", unit: "°C", scope: "mikrotik_sfp", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: ["mikrotik"] },
  ];
}

/** Painel da aba Alertas: persiste a regra «Limiar global de alertas» (condition_json schema v1). */
function AlertThresholdsPanel() {
  type RuleRow = {
    id: string;
    name: string;
    enabled: boolean;
    condition?: unknown;
  };
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-alert-threshold-rules"],
    queryFn: () => apiFetch<{ rules: RuleRow[] }>("/api/v1/alert-rules"),
  });
  const [rows, setRows] = useState<AlertThresholdMetric[]>(defaultAlertMetrics());
  const [enabled, setEnabled] = useState(true);
  const scopeOptions: { value: string; label: string }[] = [
    { value: "equipamento", label: "Equipamento" },
    { value: "olt_pon", label: "PON da OLT" },
    { value: "bng", label: "BNG (logins)" },
    { value: "mikrotik_pppoe", label: "PPPoE MikroTik" },
    { value: "mikrotik_sfp", label: "SFP da MikroTik" },
    { value: "interface", label: "Interface de rede" },
    { value: "onu", label: "ONU" },
    { value: "custom", label: "Outro" },
  ];
  const metricKeyFromLabel = (label: string, fallback: string): string => {
    const normalized = String(label)
      .toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_+|_+$/g, "");
    return normalized || fallback;
  };

  const thresholdRule = (q.data?.rules ?? []).find((r) => r.name === "Limiar global de alertas");

  useEffect(() => {
    if (!thresholdRule) return;
    setEnabled(!!thresholdRule.enabled);
    const c = (thresholdRule.condition ?? {}) as { metrics?: AlertThresholdMetric[] };
    if (Array.isArray(c.metrics) && c.metrics.length > 0) {
      const parsed: AlertThresholdMetric[] = c.metrics.map((m, idx) => {
        const ac = (m as AlertThresholdMetric).apply_categories;
        const applyCats = Array.isArray(ac) ? ac.map((x) => String(x).toLowerCase().trim()) : [];
        return {
          id: String(m.id ?? "").trim() || `metrica_${idx + 1}`,
          label: String(m.label ?? "").trim(),
          unit: String(m.unit ?? "").trim(),
          scope: String(m.scope ?? "").trim(),
          enabled: m.enabled !== false,
          operator: (m.operator === "lte" ? "lte" : "gte") as "lte" | "gte",
          green_min: String(m.green_min ?? ""),
          warning_min: String(m.warning_min ?? ""),
          critical_min: String(m.critical_min ?? ""),
          apply_categories: applyCats,
        };
      });
      const merged = [...parsed];
      for (const def of defaultAlertMetrics()) {
        if (!merged.some((m) => m.id === def.id)) merged.push(def);
      }
      setRows(merged);
    }
  }, [thresholdRule]);

  const upsert = useMutation({
    mutationFn: async () => {
      const payload = {
        schema: "netquasar.alert_thresholds.v1",
        metrics: rows
          .map((r, idx) => {
            const label = String(r.label).trim();
            const fallback = `metrica_${idx + 1}`;
            const cats = Array.isArray(r.apply_categories) ? r.apply_categories : [];
            return {
              ...r,
              id: String(r.id).trim() || metricKeyFromLabel(label, fallback),
              label,
              unit: String(r.unit).trim(),
              scope: String(r.scope).trim(),
              enabled: r.enabled !== false,
              green_min: String(r.green_min).trim(),
              warning_min: String(r.warning_min).trim(),
              critical_min: String(r.critical_min).trim(),
              apply_categories: cats,
            };
          })
          .filter((r) => r.label),
      };
      if (thresholdRule?.id) {
        return apiFetch(`/api/v1/alert-rules/${thresholdRule.id}`, {
          method: "PATCH",
          json: { name: "Limiar global de alertas", enabled, condition: payload },
        });
      }
      return apiFetch("/api/v1/alert-rules", {
        method: "POST",
        json: { name: "Limiar global de alertas", enabled, condition: payload, channels: {} },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settingsAlertThresholdRules });
      qc.invalidateQueries({ queryKey: queryKeys.alertRules });
      void invalidateAlertListQueries(qc);
    },
  });

  const updateRow = (idx: number, patch: Partial<AlertThresholdMetric>) =>
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, ...patch } : r)));

  const addRow = () =>
    setRows((prev) => [
      ...prev,
      {
        id: "",
        label: "",
        unit: "",
        scope: "equipamento",
        enabled: true,
        operator: "gte",
        green_min: "",
        warning_min: "",
        critical_min: "",
        apply_categories: [],
      },
    ]);

  const removeRow = (idx: number) => setRows((prev) => prev.filter((_, i) => i !== idx));

  const metricCatalog = defaultAlertMetrics();
  const addFromCatalog = (catalogId: string) => {
    const p = metricCatalog.find((m) => m.id === catalogId);
    if (!p) return;
    setRows((prev) => [
      ...prev,
      {
        ...p,
        id: p.id,
        label: p.label,
        apply_categories: [...(p.apply_categories ?? [])],
      },
    ]);
  };

  const equipScopeOptions: { value: "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor"; label: string }[] = [
    { value: "*", label: "Todos" },
    { value: "olt", label: "Somente OLT" },
    { value: "mikrotik", label: "Somente MikroTik" },
    { value: "bng", label: "Somente BNG" },
    { value: "switch", label: "Somente Switch" },
    { value: "servidor", label: "Servidor e outros" },
  ];
  const unitOptions = ["%", "ms", "°C", "dBm", "min", "ONUs", "evt", "Mbps"];
  const [selectedCatalog, setSelectedCatalog] = useState("");

  const scopeLabel = (scope: string): string => scopeOptions.find((s) => s.value === scope)?.label ?? scope;
  const saveHint = "Salvo em banco na regra «Limiar global de alertas» (tabela alert_rules).";
  const metricIcon = (id: string) => {
    const k = String(id).toLowerCase();
    if (k.includes("mikrotik")) return <img src="/MT_Symbol_Black.svg" alt="" width={14} height={14} />;
    if (k.includes("cpu")) return <Cpu size={14} aria-hidden />;
    if (k.includes("temperature") || k.includes("temp")) return <ThermometerSun size={14} aria-hidden />;
    if (k.includes("uptime")) return <ClockFading size={14} aria-hidden />;
    if (k.includes("sfp") || k.includes("_tx_") || k.includes("_rx_")) return <Blend size={14} aria-hidden />;
    if (k.includes("olt") || k.includes("onu") || k.includes("pon")) return <Sun size={14} aria-hidden />;
    return <Cpu size={14} aria-hidden />;
  };

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card alert-rules-card">
      <div className="alert-rules-head">
        <div>
          <h2 style={{ marginBottom: 6 }}>Configuração de Alertas</h2>
          <p style={{ color: "var(--muted)", fontSize: 13, margin: 0 }}>
            Defina por linha: tipo de equipamento, métrica, operador (≥ / ≤) e faixas <span style={{ color: "#3fb950" }}>Normal</span>,{" "}
            <span style={{ color: "#d29922" }}>Atenção</span> e <span style={{ color: "#f85149" }}>Crítico</span>.
            Os limiares são avaliados no ciclo automático de telemetria do worker e no refresh manual.
          </p>
        </div>
        <label className="row" style={{ gap: 8 }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          <span style={{ fontSize: 13 }}>Perfil ativo</span>
        </label>
      </div>

      <div className="alert-rules-toolbar">
        <div className="field" style={{ margin: 0, minWidth: 320 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>Adicionar métrica padrão</label>
          <select
            className="input"
            value={selectedCatalog}
            onChange={(e) => {
              const v = e.target.value;
              setSelectedCatalog(v);
              if (v) {
                addFromCatalog(v);
                setSelectedCatalog("");
              }
            }}
          >
            <option value="">Selecionar…</option>
            {metricCatalog.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label} ({scopeLabel(m.scope)})
              </option>
            ))}
          </select>
        </div>
        <button type="button" className="btn" onClick={addRow}>
          Novo critério
        </button>
      </div>

      <div className="alert-rules-grid-wrap">
        <table className="alert-rules-grid">
          <thead>
            <tr>
              <th>Métrica</th>
              <th>Equipamento</th>
              <th>Tipo de dado</th>
              <th>Condição</th>
              <th>Normal</th>
              <th>Atenção</th>
              <th>Crítico</th>
              <th>Habilitado</th>
              <th>Ações</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, idx) => (
              <tr key={`criterion-${idx}-${r.id || "new"}`}>
                <td>
                  <div className="alert-rules-metric-wrap">
                    <span className="alert-rules-metric-icon">{metricIcon(r.id)}</span>
                    <input className="input alert-rules-input-metric" value={r.label} onChange={(e) => updateRow(idx, { label: e.target.value })} placeholder="Nome da métrica" />
                  </div>
                </td>
                <td>
                  <select
                    className="input alert-rules-input-equip"
                    value={equipScopeFromCategories(r.apply_categories)}
                    onChange={(e) => updateRow(idx, { apply_categories: categoriesFromEquipScope(e.target.value as "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor") })}
                  >
                    {equipScopeOptions.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <select className="input alert-rules-input-unit" value={r.unit} onChange={(e) => updateRow(idx, { unit: e.target.value })}>
                    <option value="">-</option>
                    {unitOptions.map((u) => (
                      <option key={u} value={u}>
                        {u}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <div className="alert-rules-cond">
                    <select className="input alert-rules-input-scope" value={r.scope} onChange={(e) => updateRow(idx, { scope: e.target.value })}>
                      {scopeOptions.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                    <select className="input alert-rules-input-op" value={r.operator} onChange={(e) => updateRow(idx, { operator: e.target.value as "gte" | "lte" })}>
                      <option value="gte">≥ (maior ou igual)</option>
                      <option value="lte">≤ (menor ou igual)</option>
                    </select>
                  </div>
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.green_min} onChange={(e) => updateRow(idx, { green_min: e.target.value })} placeholder="0" />
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.warning_min} onChange={(e) => updateRow(idx, { warning_min: e.target.value })} placeholder="0" />
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.critical_min} onChange={(e) => updateRow(idx, { critical_min: e.target.value })} placeholder="0" />
                </td>
                <td style={{ textAlign: "center" }}>
                  <label className="toggle" htmlFor={`rule-enabled-${idx}`} style={{ justifyContent: "center" }}>
                    <span className="toggle__track">
                      <input
                        id={`rule-enabled-${idx}`}
                        type="checkbox"
                        role="switch"
                        className="toggle__input"
                        checked={r.enabled !== false}
                        onChange={(e) => updateRow(idx, { enabled: e.target.checked })}
                      />
                      <span className="toggle__thumb" aria-hidden />
                    </span>
                  </label>
                </td>
                <td>
                  <button type="button" className="btn btn--danger btn--icon" aria-label="Remover regra" title="Remover" onClick={() => removeRow(idx)}>
                    <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden>
                      <path
                        d="M9 3h6l1 2h4v2H4V5h4l1-2zm1 6h2v9h-2V9zm4 0h2v9h-2V9zM7 9h2v9H7V9z"
                        fill="currentColor"
                      />
                    </svg>
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="row" style={{ marginTop: 16, gap: 10, flexWrap: "wrap", alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={upsert.isPending} onClick={() => upsert.mutate()}>
          Salvar alterações
        </button>
        <span style={{ fontSize: 12, color: "var(--muted)" }}>{saveHint}</span>
      </div>
      {upsert.isError && <div className="msg msg--err">{(upsert.error as Error).message}</div>}
      {upsert.isSuccess && (
        <div className="msg msg--ok">
          Critérios salvos com sucesso. O monitoramento consulta estes valores para decidir se abre, atualiza ou resolve alertas.
        </div>
      )}
    </div>
  );
}

function ConnectionPanel() {
  type CategoryOverrides = {
    cpu_oid?: string;
    cpu_available_oid?: string;
    memory_used_oid?: string;
    memory_size_oid?: string;
    temp_oid?: string;
    uptime_oid?: string;
    brand_oids?: string[];
    model_oids?: string[];
    serial_oids?: string[];
    software_oids?: string[];
    hardware_oids?: string[];
    sysname_oids?: string[];
    sysdescr_oids?: string[];
    interface_oids?: string[];
    optical_oids?: string[];
    pon_oids?: string[];
    onu_oids?: string[];
    bridge_oids?: string[];
    traffic_oids?: string[];
    custom_oids?: string[];
    /** OID normalizado (sem ponto inicial) → descrição mostrada no relatório. */
    oid_labels?: Record<string, string>;
  };
  type OverridesDoc = {
    olt?: CategoryOverrides;
    mikrotik?: CategoryOverrides;
    bng?: CategoryOverrides;
    servidor?: CategoryOverrides;
    bridge?: CategoryOverrides;
  };
  type OidExtraCategory = "olt" | "mikrotik" | "bng" | "servidor" | "bridge";
  type OidArrayKey = keyof Pick<
    CategoryOverrides,
    | "brand_oids"
    | "model_oids"
    | "serial_oids"
    | "software_oids"
    | "hardware_oids"
    | "sysname_oids"
    | "sysdescr_oids"
    | "interface_oids"
    | "traffic_oids"
    | "optical_oids"
    | "pon_oids"
    | "onu_oids"
    | "bridge_oids"
    | "custom_oids"
  >;
  type OidExtraKind =
    | "brand"
    | "model"
    | "serial"
    | "software"
    | "hardware"
    | "sysname"
    | "sysdescr"
    | "interface"
    | "traffic"
    | "optical"
    | "pon"
    | "onu"
    | "bridge"
    | "custom";
  type OidKindMeta = { value: OidExtraKind; label: string; jsonKey: OidArrayKey; defaultLabel: string };
  type ExtraOidRow = { id: string; kind: OidExtraKind; oid: string; label: string };

  const OID_KIND_GROUPS: { label: string; kinds: OidKindMeta[] }[] = [
    {
      label: "Inventário / identificação",
      kinds: [
        { value: "brand", label: "Fabricante / marca", jsonKey: "brand_oids", defaultLabel: "Fabricante" },
        { value: "model", label: "Modelo", jsonKey: "model_oids", defaultLabel: "Modelo" },
        { value: "serial", label: "Número de série", jsonKey: "serial_oids", defaultLabel: "Número de série" },
        { value: "software", label: "Versão de software / firmware", jsonKey: "software_oids", defaultLabel: "Versão de software" },
        { value: "hardware", label: "Versão de hardware", jsonKey: "hardware_oids", defaultLabel: "Versão de hardware" },
        { value: "sysname", label: "Nome do sistema (sysName)", jsonKey: "sysname_oids", defaultLabel: "Nome do sistema" },
        { value: "sysdescr", label: "Descrição do sistema (sysDescr)", jsonKey: "sysdescr_oids", defaultLabel: "Descrição do sistema" },
      ],
    },
    {
      label: "Rede / telemetria",
      kinds: [
        { value: "interface", label: "Interface", jsonKey: "interface_oids", defaultLabel: "Interface" },
        { value: "traffic", label: "Tráfego (banda RX/TX etc.)", jsonKey: "traffic_oids", defaultLabel: "Tráfego" },
        { value: "optical", label: "Óptica / SFP", jsonKey: "optical_oids", defaultLabel: "Óptica" },
        { value: "pon", label: "PON", jsonKey: "pon_oids", defaultLabel: "PON" },
        { value: "onu", label: "ONU", jsonKey: "onu_oids", defaultLabel: "ONU" },
        { value: "bridge", label: "Bridge", jsonKey: "bridge_oids", defaultLabel: "Bridge" },
      ],
    },
    {
      label: "Outros",
      kinds: [{ value: "custom", label: "Outro / personalizado", jsonKey: "custom_oids", defaultLabel: "Leitura extra" }],
    },
  ];

  const OID_KIND_META: OidKindMeta[] = OID_KIND_GROUPS.flatMap((g) => g.kinds);
  const OID_KIND_BY_VALUE = Object.fromEntries(OID_KIND_META.map((k) => [k.value, k])) as Record<OidExtraKind, OidKindMeta>;
  const OID_ARRAY_KEYS: OidArrayKey[] = OID_KIND_META.map((k) => k.jsonKey);

  /** OIDs reservados nos cartões de telemetria avançada (não aparecem na lista de extras). */
  const RESERVED_OID_SLOTS: Partial<Record<OidExtraCategory, Partial<Record<OidExtraKind, number>>>> = {
    olt: { onu: 1, pon: 2 },
    mikrotik: { interface: 1, traffic: 2, optical: 2 },
  };

  const compact = (arr: Array<string | undefined | null>): string[] =>
    arr.map((s) => String(s ?? "").trim()).filter((s) => s.length > 0);

  const newOidRowId = (): string =>
    typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `oid-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;

  const oidLabelMapFromUnknown = (raw: unknown): Record<string, string> => {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
      const kk = String(k ?? "")
        .trim()
        .replace(/^\./, "");
      const vv = String(v ?? "").trim();
      if (kk && vv) out[kk] = vv;
    }
    return out;
  };

  const mergeCategoryOidLabels = (baselineBlk: CategoryOverrides | undefined, rows: ExtraOidRow[]): Record<string, string> | undefined => {
    const m: Record<string, string> = { ...oidLabelMapFromUnknown(baselineBlk?.oid_labels) };
    for (const r of rows) {
      const o = String(r.oid ?? "")
        .trim()
        .replace(/^\./, "");
      const lbl = String(r.label ?? "").trim();
      if (!o) continue;
      if (lbl) m[o] = lbl;
      else delete m[o];
    }
    return Object.keys(m).length ? m : undefined;
  };

  const oidsInCategoryArrays = (blk: CategoryOverrides): Set<string> => {
    const s = new Set<string>();
    for (const key of OID_ARRAY_KEYS) {
      for (const x of blk[key] ?? []) {
        const o = String(x).trim().replace(/^\./, "");
        if (o) s.add(o);
      }
    }
    return s;
  };

  const pruneOidLabelsToBlock = (blk: CategoryOverrides, labels: Record<string, string> | undefined): Record<string, string> | undefined => {
    if (!labels) return undefined;
    const allowed = oidsInCategoryArrays(blk);
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(labels)) {
      const kk = String(k).trim().replace(/^\./, "");
      if (!kk || !String(v).trim()) continue;
      if (allowed.has(kk)) out[kk] = String(v).trim();
    }
    return Object.keys(out).length ? out : undefined;
  };

  const emptyExtraRows = (): Record<OidExtraCategory, ExtraOidRow[]> => ({
    olt: [],
    mikrotik: [],
    bng: [],
    servidor: [],
    bridge: [],
  });

  const OID_EXTRA_CATEGORY_LABELS: Record<OidExtraCategory, string> = {
    olt: "OLT",
    mikrotik: "MikroTik",
    bng: "BNG",
    servidor: "Servidor",
    bridge: "Pontes",
  };

  type TelemetryOidValues = {
    cpu: string;
    cpuAvail: string;
    memUsed: string;
    memSize: string;
    temp: string;
    uptime: string;
  };

  const renderBaseTelemetryFields = (values: TelemetryOidValues, onChange: (patch: Partial<TelemetryOidValues>) => void) => (
    <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
      <div className="field">
        <label>CPU utilizada (uso / carga)</label>
        <input className="input mono" value={values.cpu} onChange={(e) => onChange({ cpu: e.target.value })} />
      </div>
      <div className="field">
        <label>CPU disponível (% idle)</label>
        <input className="input mono" value={values.cpuAvail} onChange={(e) => onChange({ cpuAvail: e.target.value })} placeholder="opcional" />
      </div>
      <div className="field">
        <label>Memória em uso</label>
        <input className="input mono" value={values.memUsed} onChange={(e) => onChange({ memUsed: e.target.value })} />
      </div>
      <div className="field">
        <label>Memória total</label>
        <input className="input mono" value={values.memSize} onChange={(e) => onChange({ memSize: e.target.value })} />
      </div>
      <div className="field">
        <label>Temperatura</label>
        <input className="input mono" value={values.temp} onChange={(e) => onChange({ temp: e.target.value })} />
      </div>
      <div className="field">
        <label>Tempo ligado (uptime)</label>
        <input className="input mono" value={values.uptime} onChange={(e) => onChange({ uptime: e.target.value })} />
      </div>
    </div>
  );

  /** Junta OIDs extra por tipo; mantém ordem e remove duplicados vazios. */
  const mergeOidsByKind = (rows: ExtraOidRow[]): Record<OidExtraKind, string[]> => {
    const acc = {} as Record<OidExtraKind, string[]>;
    const seen = {} as Record<OidExtraKind, Set<string>>;
    for (const meta of OID_KIND_META) {
      acc[meta.value] = [];
      seen[meta.value] = new Set();
    }
    for (const r of rows) {
      const o = String(r.oid ?? "").trim();
      if (!o) continue;
      if (seen[r.kind].has(o)) continue;
      seen[r.kind].add(o);
      acc[r.kind].push(o);
    }
    return acc;
  };

  const mergeCategoryOidArrays = (
    block: CategoryOverrides,
    rows: ExtraOidRow[],
    reserved?: Partial<Record<OidExtraKind, string[]>>,
  ) => {
    const merged = mergeOidsByKind(rows);
    for (const meta of OID_KIND_META) {
      const combined = compact([...(reserved?.[meta.value] ?? []), ...merged[meta.value]]);
      if (combined.length) (block as Record<string, unknown>)[meta.jsonKey] = combined;
      else delete (block as Record<string, unknown>)[meta.jsonKey];
    }
    Object.keys(block).forEach((k) => {
      const v = (block as Record<string, unknown>)[k];
      if (v === undefined || (Array.isArray(v) && v.length === 0) || v === "") {
        delete (block as Record<string, unknown>)[k];
      }
    });
  };

  const loadCategoryExtraRows = (
    cat: OidExtraCategory,
    block: CategoryOverrides,
    labels: Record<string, string>,
    into: ExtraOidRow[],
    fromArr: (labels: Record<string, string>, kind: OidExtraKind, list: string[] | undefined) => ExtraOidRow[],
  ) => {
    for (const meta of OID_KIND_META) {
      const skip = RESERVED_OID_SLOTS[cat]?.[meta.value] ?? 0;
      const arr = (block[meta.jsonKey] ?? []).slice(skip);
      into.push(...fromArr(labels, meta.value, arr));
    }
  };

  /**
   * Lê o JSON salvo e separa (a) campos reservados dos cartões OLT/Mikrotik
   * e (b) restantes em linhas editáveis por categoria.
   */
  const extraRowsFromOverridesDoc = (doc: OverridesDoc): Record<OidExtraCategory, ExtraOidRow[]> => {
    const out = emptyExtraRows();
    const fromArr = (labels: Record<string, string>, kind: OidExtraKind, list: string[] | undefined): ExtraOidRow[] =>
      (list ?? [])
        .map((oid) => {
          const o = String(oid).trim();
          if (!o) return null;
          const norm = o.replace(/^\./, "");
          const label = String(labels[norm] ?? labels[o] ?? "").trim();
          return { id: newOidRowId(), kind, oid: o, label };
        })
        .filter((r): r is ExtraOidRow => r != null);

    const o = doc.olt ?? {};
    const m = doc.mikrotik ?? {};
    const bn = doc.bng ?? {};
    const s = doc.servidor ?? {};
    const b = doc.bridge ?? {};
    const lo = oidLabelMapFromUnknown(o.oid_labels);
    const lm = oidLabelMapFromUnknown(m.oid_labels);
    const lbn = oidLabelMapFromUnknown(bn.oid_labels);
    const ls = oidLabelMapFromUnknown(s.oid_labels);
    const lb = oidLabelMapFromUnknown(b.oid_labels);

    loadCategoryExtraRows("olt", o, lo, out.olt, fromArr);
    loadCategoryExtraRows("mikrotik", m, lm, out.mikrotik, fromArr);
    loadCategoryExtraRows("bng", bn, lbn, out.bng, fromArr);
    loadCategoryExtraRows("servidor", s, ls, out.servidor, fromArr);
    loadCategoryExtraRows("bridge", b, lb, out.bridge, fromArr);

    return out;
  };

  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-conn-def"],
    queryFn: () =>
      apiFetch<{
        snmp_community: unknown;
        snmp_community_value: string;
        snmp_community_configured: boolean;
        telnet_user: string | null;
        telnet_password: string;
        telnet_password_configured: boolean;
        telnet_enable: string;
        telnet_enable_configured: boolean;
        ssh_user: string | null;
        ssh_password: string;
        ssh_password_configured: boolean;
        oid_defaults: {
          olt: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          mikrotik: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          bng: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          server: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
        };
        snmp_oid_overrides: unknown;
        updated_at: string;
      }>("/api/v1/settings/connection/defaults"),
  });
  const [snmp, setSnmp] = useState("");
  const [tu, setTu] = useState("");
  const [tp, setTp] = useState("");
  const [te, setTe] = useState("");
  const [su, setSu] = useState("");
  const [sp, setSp] = useState("");
  const [oltCpu, setOltCpu] = useState("");
  const [oltCpuAvail, setOltCpuAvail] = useState("");
  const [oltMemUsed, setOltMemUsed] = useState("");
  const [oltMemSize, setOltMemSize] = useState("");
  const [oltTemp, setOltTemp] = useState("");
  const [oltUptime, setOltUptime] = useState("");
  const [mkCpu, setMkCpu] = useState("");
  const [mkCpuAvail, setMkCpuAvail] = useState("");
  const [mkMemUsed, setMkMemUsed] = useState("");
  const [mkMemSize, setMkMemSize] = useState("");
  const [mkTemp, setMkTemp] = useState("");
  const [mkUptime, setMkUptime] = useState("");
  const [bngCpu, setBngCpu] = useState("");
  const [bngCpuAvail, setBngCpuAvail] = useState("");
  const [bngMemUsed, setBngMemUsed] = useState("");
  const [bngMemSize, setBngMemSize] = useState("");
  const [bngTemp, setBngTemp] = useState("");
  const [bngUptime, setBngUptime] = useState("");
  const [svCpu, setSvCpu] = useState("");
  const [svCpuAvail, setSvCpuAvail] = useState("");
  const [svMemUsed, setSvMemUsed] = useState("");
  const [svMemSize, setSvMemSize] = useState("");
  const [svTemp, setSvTemp] = useState("");
  const [svUptime, setSvUptime] = useState("");
  const [oltOnuTotalOid, setOltOnuTotalOid] = useState("");
  const [oltPonTxOid, setOltPonTxOid] = useState("");
  const [oltPonStatusOid, setOltPonStatusOid] = useState("");
  const [mkInterfacesStatusOid, setMkInterfacesStatusOid] = useState("");
  const [mkBandwidthRxOid, setMkBandwidthRxOid] = useState("");
  const [mkBandwidthTxOid, setMkBandwidthTxOid] = useState("");
  const [mkSfpTxOid, setMkSfpTxOid] = useState("");
  const [mkSfpRxOid, setMkSfpRxOid] = useState("");
  /** Base vinda do servidor (preserva scalars/hand-edits não cobertos pela UI). */
  const [overridesBaseline, setOverridesBaseline] = useState<OverridesDoc>({});
  const [extraOidRows, setExtraOidRows] = useState<Record<OidExtraCategory, ExtraOidRow[]>>(emptyExtraRows);
  const [showGeneratedJson, setShowGeneratedJson] = useState(false);

  useEffect(() => {
    if (!q.data) return;
    setSnmp((v) => (v === "" ? q.data.snmp_community_value ?? "" : v));
    setTu(q.data.telnet_user ?? "");
    setSu(q.data.ssh_user ?? "");
    setOltCpu(q.data.oid_defaults?.olt?.cpu_oid ?? "");
    setOltCpuAvail(q.data.oid_defaults?.olt?.cpu_available_oid ?? "");
    setOltMemUsed(q.data.oid_defaults?.olt?.memory_used_oid ?? "");
    setOltMemSize(q.data.oid_defaults?.olt?.memory_size_oid ?? "");
    setOltTemp(q.data.oid_defaults?.olt?.temp_oid ?? "");
    setOltUptime(q.data.oid_defaults?.olt?.uptime_oid ?? "");
    setMkCpu(q.data.oid_defaults?.mikrotik?.cpu_oid ?? "");
    setMkCpuAvail(q.data.oid_defaults?.mikrotik?.cpu_available_oid ?? "");
    setMkMemUsed(q.data.oid_defaults?.mikrotik?.memory_used_oid ?? "");
    setMkMemSize(q.data.oid_defaults?.mikrotik?.memory_size_oid ?? "");
    setMkTemp(q.data.oid_defaults?.mikrotik?.temp_oid ?? "");
    setMkUptime(q.data.oid_defaults?.mikrotik?.uptime_oid ?? "");
    setBngCpu(q.data.oid_defaults?.bng?.cpu_oid ?? "");
    setBngCpuAvail(q.data.oid_defaults?.bng?.cpu_available_oid ?? "");
    setBngMemUsed(q.data.oid_defaults?.bng?.memory_used_oid ?? "");
    setBngMemSize(q.data.oid_defaults?.bng?.memory_size_oid ?? "");
    setBngTemp(q.data.oid_defaults?.bng?.temp_oid ?? "");
    setBngUptime(q.data.oid_defaults?.bng?.uptime_oid ?? "");
    setSvCpu(q.data.oid_defaults?.server?.cpu_oid ?? "");
    setSvCpuAvail(q.data.oid_defaults?.server?.cpu_available_oid ?? "");
    setSvMemUsed(q.data.oid_defaults?.server?.memory_used_oid ?? "");
    setSvMemSize(q.data.oid_defaults?.server?.memory_size_oid ?? "");
    setSvTemp(q.data.oid_defaults?.server?.temp_oid ?? "");
    setSvUptime(q.data.oid_defaults?.server?.uptime_oid ?? "");
    try {
      const parsed = (q.data.snmp_oid_overrides ?? {}) as OverridesDoc;
      setOverridesBaseline(JSON.parse(JSON.stringify(parsed)) as OverridesDoc);
      const olt = parsed?.olt ?? {};
      const mikrotik = parsed?.mikrotik ?? {};
      setOltOnuTotalOid(olt.onu_oids?.[0] ?? "");
      setOltPonTxOid(olt.pon_oids?.[0] ?? "");
      setOltPonStatusOid(olt.pon_oids?.[1] ?? "");
      setMkInterfacesStatusOid(mikrotik.interface_oids?.[0] ?? "");
      setMkBandwidthRxOid(mikrotik.traffic_oids?.[0] ?? "");
      setMkBandwidthTxOid(mikrotik.traffic_oids?.[1] ?? "");
      setMkSfpTxOid(mikrotik.optical_oids?.[0] ?? "");
      setMkSfpRxOid(mikrotik.optical_oids?.[1] ?? "");
      setExtraOidRows(extraRowsFromOverridesDoc(parsed));
    } catch {
      setOverridesBaseline({});
      setExtraOidRows(emptyExtraRows());
    }
  }, [q.data]);

  const builtOverridesPreview = (): OverridesDoc => {
    const base = JSON.parse(JSON.stringify(overridesBaseline)) as OverridesDoc;
    base.olt = base.olt ?? {};
    base.mikrotik = base.mikrotik ?? {};
    base.bng = base.bng ?? {};
    base.servidor = base.servidor ?? {};
    base.bridge = base.bridge ?? {};

    mergeCategoryOidArrays(base.olt as CategoryOverrides, extraOidRows.olt, {
      onu: compact([oltOnuTotalOid]),
      pon: compact([oltPonTxOid, oltPonStatusOid]),
    });
    delete (base.olt as CategoryOverrides).oid_labels;
    const oltOidLabels = pruneOidLabelsToBlock(
      base.olt as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.olt, extraOidRows.olt),
    );
    if (oltOidLabels) (base.olt as CategoryOverrides).oid_labels = oltOidLabels;

    mergeCategoryOidArrays(base.mikrotik as CategoryOverrides, extraOidRows.mikrotik, {
      interface: compact([mkInterfacesStatusOid]),
      traffic: compact([mkBandwidthRxOid, mkBandwidthTxOid]),
      optical: compact([mkSfpTxOid, mkSfpRxOid]),
    });
    delete (base.mikrotik as CategoryOverrides).oid_labels;
    const mkOidLabels = pruneOidLabelsToBlock(
      base.mikrotik as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.mikrotik, extraOidRows.mikrotik),
    );
    if (mkOidLabels) (base.mikrotik as CategoryOverrides).oid_labels = mkOidLabels;

    mergeCategoryOidArrays(base.bng as CategoryOverrides, extraOidRows.bng);
    delete (base.bng as CategoryOverrides).oid_labels;
    const bngOidLabels = pruneOidLabelsToBlock(
      base.bng as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.bng, extraOidRows.bng),
    );
    if (bngOidLabels) (base.bng as CategoryOverrides).oid_labels = bngOidLabels;

    mergeCategoryOidArrays(base.servidor as CategoryOverrides, extraOidRows.servidor);
    delete (base.servidor as CategoryOverrides).oid_labels;
    const srvOidLabels = pruneOidLabelsToBlock(
      base.servidor as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.servidor, extraOidRows.servidor),
    );
    if (srvOidLabels) (base.servidor as CategoryOverrides).oid_labels = srvOidLabels;

    mergeCategoryOidArrays(base.bridge as CategoryOverrides, extraOidRows.bridge);
    delete (base.bridge as CategoryOverrides).oid_labels;
    const brOidLabels = pruneOidLabelsToBlock(
      base.bridge as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.bridge, extraOidRows.bridge),
    );
    if (brOidLabels) (base.bridge as CategoryOverrides).oid_labels = brOidLabels;

    (["olt", "mikrotik", "bng", "servidor", "bridge"] as const).forEach((ck) => {
      const blk = base[ck] as Record<string, unknown> | undefined;
      if (blk && Object.keys(blk).length === 0) {
        delete base[ck];
      }
    });
    return base;
  };

  const addExtraRow = (cat: OidExtraCategory, kind?: OidExtraKind) => {
    const k = kind ?? "brand";
    setExtraOidRows((prev) => ({
      ...prev,
      [cat]: [...prev[cat], { id: newOidRowId(), kind: k, oid: "", label: OID_KIND_BY_VALUE[k]?.defaultLabel ?? "" }],
    }));
  };

  const removeExtraRow = (cat: OidExtraCategory, id: string) =>
    setExtraOidRows((prev) => ({ ...prev, [cat]: prev[cat].filter((r) => r.id !== id) }));

  const updateExtraRow = (cat: OidExtraCategory, id: string, patchRow: Partial<Pick<ExtraOidRow, "kind" | "oid" | "label">>) =>
    setExtraOidRows((prev) => ({
      ...prev,
      [cat]: prev[cat].map((r) => {
        if (r.id !== id) return r;
        const next = { ...r, ...patchRow };
        if (patchRow.kind && patchRow.kind !== r.kind && !String(next.label ?? "").trim()) {
          next.label = OID_KIND_BY_VALUE[patchRow.kind]?.defaultLabel ?? "";
        }
        return next;
      }),
    }));

  const renderOidExtrasBlock = (cat: OidExtraCategory, title: string) => {
    const rows = extraOidRows[cat];
    return (
      <div className="settings-conn-block" style={{ marginTop: 10 }}>
        <h4 style={{ marginTop: 0 }}>
          {title}
          <InfoHint label="Leituras SNMP extra">
            <p>
              Um identificador SNMP por linha. Escolha o tipo (fabricante, modelo, série, interface, PON, etc.) para organizar os dados ao salvar.
              A descrição aparece nos relatórios de telemetria.
            </p>
          </InfoHint>
        </h4>
        {rows.length === 0 ? (
          <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Nenhum extra — use «Adicionar» para incluir mais leituras.</p>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {rows.map((r) => (
              <div key={r.id} className="row" style={{ flexWrap: "wrap", gap: 6, alignItems: "flex-end" }}>
                <select
                  title="Tipo de métrica"
                  aria-label="Tipo de métrica SNMP"
                  className="select"
                  style={{ minWidth: 220, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.kind}
                  onChange={(e) => updateExtraRow(cat, r.id, { kind: e.target.value as OidExtraKind })}
                >
                  {OID_KIND_GROUPS.map((group) => (
                    <optgroup key={group.label} label={group.label}>
                      {group.kinds.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </optgroup>
                  ))}
                </select>
                <input
                  title="Identificador numérico SNMP"
                  aria-label="Identificador SNMP"
                  className="input mono"
                  style={{ flex: "1 1 160px", minWidth: 140, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.oid}
                  onChange={(e) => updateExtraRow(cat, r.id, { oid: e.target.value })}
                />
                <input
                  title="Descrição no relatório"
                  aria-label="Descrição da leitura SNMP extra"
                  className="input"
                  placeholder="Descrição (relatório)"
                  style={{ flex: "1 1 140px", minWidth: 120, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.label}
                  onChange={(e) => updateExtraRow(cat, r.id, { label: e.target.value })}
                />
                <button type="button" className="btn" style={{ padding: "4px 8px", fontSize: 11 }} onClick={() => removeExtraRow(cat, r.id)}>
                  âˆ’
                </button>
              </div>
            ))}
          </div>
        )}
        <div className="row" style={{ marginTop: 8, gap: 6 }}>
          <button type="button" className="btn btn--primary" style={{ padding: "4px 10px", fontSize: 11 }} onClick={() => addExtraRow(cat)}>
            Adicionar
          </button>
        </div>
      </div>
    );
  };

  const patch = useMutation({
    mutationFn: () => {
      const parsedOverrides = builtOverridesPreview();
      return apiFetch("/api/v1/settings/connection/defaults", {
        method: "PATCH",
        json: {
          snmp_community: snmp || undefined,
          telnet_user: tu || undefined,
          telnet_password: tp || undefined,
          telnet_enable: te || undefined,
          ssh_user: su || undefined,
          ssh_password: sp || undefined,
          olt_cpu_oid: oltCpu || undefined,
          olt_cpu_available_oid: oltCpuAvail || undefined,
          olt_memory_used_oid: oltMemUsed || undefined,
          olt_memory_size_oid: oltMemSize || undefined,
          olt_temp_oid: oltTemp || undefined,
          olt_uptime_oid: oltUptime || undefined,
          mikrotik_cpu_oid: mkCpu || undefined,
          mikrotik_cpu_available_oid: mkCpuAvail || undefined,
          mikrotik_memory_used_oid: mkMemUsed || undefined,
          mikrotik_memory_size_oid: mkMemSize || undefined,
          mikrotik_temp_oid: mkTemp || undefined,
          mikrotik_uptime_oid: mkUptime || undefined,
          bng_cpu_oid: bngCpu || undefined,
          bng_cpu_available_oid: bngCpuAvail || undefined,
          bng_memory_used_oid: bngMemUsed || undefined,
          bng_memory_size_oid: bngMemSize || undefined,
          bng_temp_oid: bngTemp || undefined,
          bng_uptime_oid: bngUptime || undefined,
          server_cpu_oid: svCpu || undefined,
          server_cpu_available_oid: svCpuAvail || undefined,
          server_memory_used_oid: svMemUsed || undefined,
          server_memory_size_oid: svMemSize || undefined,
          server_temp_oid: svTemp || undefined,
          server_uptime_oid: svUptime || undefined,
          snmp_oid_overrides: parsedOverrides,
        },
      });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings-conn-def"] }),
  });

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Credenciais e leituras SNMP por defeito</h2>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>
        Valores aplicados quando o equipamento não traz credenciais próprias. Palavras-passe não são mostradas ao abrir esta página.
        {q.data?.updated_at ? ` Última alteração: ${q.data.updated_at}` : ""}
      </p>
      <div className="row" style={{ gap: 10, marginBottom: 8 }}>
        <span className={q.data?.snmp_community_configured ? "badge badge--ok" : "badge badge--off"}>
          Comunidade SNMP: {q.data?.snmp_community_configured ? "definida" : "não definida"}
        </span>
        <span className={q.data?.telnet_password_configured ? "badge badge--ok" : "badge badge--off"}>
          Palavra-passe Telnet: {q.data?.telnet_password_configured ? "definida" : "não definida"}
        </span>
        <span className={q.data?.ssh_password_configured ? "badge badge--ok" : "badge badge--off"}>
          Palavra-passe SSH: {q.data?.ssh_password_configured ? "definida" : "não definida"}
        </span>
      </div>
      <div className="settings-conn-section" style={{ marginTop: 14 }}>
        <h3 style={{ marginTop: 0 }}>Credenciais</h3>
        <div className="field">
          <label>Comunidade SNMP padrão</label>
          <input className="input" value={snmp} onChange={(e) => setSnmp(e.target.value)} />
        </div>
        <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
          <div className="field" style={{ minWidth: 220 }}><label>Utilizador Telnet</label><input className="input" value={tu} onChange={(e) => setTu(e.target.value)} /></div>
          <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe Telnet</label><input className="input" type="password" value={tp} onChange={(e) => setTp(e.target.value)} /></div>
          <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe enable (Telnet)</label><input className="input" type="password" value={te} onChange={(e) => setTe(e.target.value)} /></div>
        </div>
        <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: 8 }}>
          <div className="field" style={{ minWidth: 220 }}><label>Utilizador SSH</label><input className="input" value={su} onChange={(e) => setSu(e.target.value)} /></div>
          <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe SSH</label><input className="input" type="password" value={sp} onChange={(e) => setSp(e.target.value)} /></div>
        </div>
      </div>
      <div className="settings-conn-section">
        <h3 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Leituras SNMP por tipo de equipamento
          <InfoHint label="OIDs SNMP preferidos">
            <p>
              Se preencher, estes endereços têm prioridade sobre a descoberta automática. Em «CPU utilizada» indique a carga; em «CPU disponível» use normalmente
              a percentagem em idle (ociosidade). O painel tenta primeiro a utilizada e só depois deriva a partir da disponível (100 − idle).
            </p>
          </InfoHint>
        </h3>
        <div className="settings-conn-block">
          <h4>OLT</h4>
          {renderBaseTelemetryFields(
            { cpu: oltCpu, cpuAvail: oltCpuAvail, memUsed: oltMemUsed, memSize: oltMemSize, temp: oltTemp, uptime: oltUptime },
            (p) => {
              if (p.cpu !== undefined) setOltCpu(p.cpu);
              if (p.cpuAvail !== undefined) setOltCpuAvail(p.cpuAvail);
              if (p.memUsed !== undefined) setOltMemUsed(p.memUsed);
              if (p.memSize !== undefined) setOltMemSize(p.memSize);
              if (p.temp !== undefined) setOltTemp(p.temp);
              if (p.uptime !== undefined) setOltUptime(p.uptime);
            },
          )}
        </div>
        <div className="settings-conn-block">
          <h4>MikroTik</h4>
          {renderBaseTelemetryFields(
            { cpu: mkCpu, cpuAvail: mkCpuAvail, memUsed: mkMemUsed, memSize: mkMemSize, temp: mkTemp, uptime: mkUptime },
            (p) => {
              if (p.cpu !== undefined) setMkCpu(p.cpu);
              if (p.cpuAvail !== undefined) setMkCpuAvail(p.cpuAvail);
              if (p.memUsed !== undefined) setMkMemUsed(p.memUsed);
              if (p.memSize !== undefined) setMkMemSize(p.memSize);
              if (p.temp !== undefined) setMkTemp(p.temp);
              if (p.uptime !== undefined) setMkUptime(p.uptime);
            },
          )}
        </div>
        <div className="settings-conn-block">
          <h4>Servidor</h4>
          {renderBaseTelemetryFields(
            { cpu: svCpu, cpuAvail: svCpuAvail, memUsed: svMemUsed, memSize: svMemSize, temp: svTemp, uptime: svUptime },
            (p) => {
              if (p.cpu !== undefined) setSvCpu(p.cpu);
              if (p.cpuAvail !== undefined) setSvCpuAvail(p.cpuAvail);
              if (p.memUsed !== undefined) setSvMemUsed(p.memUsed);
              if (p.memSize !== undefined) setSvMemSize(p.memSize);
              if (p.temp !== undefined) setSvTemp(p.temp);
              if (p.uptime !== undefined) setSvUptime(p.uptime);
            },
          )}
        </div>
      </div>

      <div className="settings-conn-section">
        <h3 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Telemetria avançada
          <InfoHint label="PON, interfaces e SFP">
            <p>Campos rápidos para métricas frequentes em OLT e MikroTik. O restante pode ser configurado na secção de leituras extra.</p>
          </InfoHint>
        </h3>
        <div className="settings-conn-block">
          <h4>OLT — PON / GBIC / ONU</h4>
          <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Total de ONUs</label>
              <input className="input mono" value={oltOnuTotalOid} onChange={(e) => setOltOnuTotalOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Potência TX da PON</label>
              <input className="input mono" value={oltPonTxOid} onChange={(e) => setOltPonTxOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Status da PON</label>
              <input className="input mono" value={oltPonStatusOid} onChange={(e) => setOltPonStatusOid(e.target.value)} />
            </div>
          </div>
        </div>
        <div className="settings-conn-block">
          <h4 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
            MikroTik — interfaces / SFP
            <InfoHint label="MikroTik SFP e interfaces">
              <p>
                A página de interfaces faz walk em <span className="mono">mtxrOpticalTable</span> (<span className="mono">1.3.6.1.4.1.14988.1.1.19</span>, MIB MIKROTIK) e em{" "}
                <span className="mono">mtxrInterfaceStatsName</span> (<span className="mono">1.3.6.1.4.1.14988.1.1.14.1.1.2</span>) para obter o nome igual ao{" "}
                <span className="mono">ifName</span>. Potências: colunas <strong>9</strong> (TX) e <strong>10</strong> (RX), tipo <strong>IDiv1000</strong> (milésimos de dBm). O índice da linha mtxr não é o ifIndex; o cruzamento usa o nome de <span className="mono">…14.1.1.2</span>, o valor de{" "}
                <span className="mono">mtxrOpticalIndex</span> (col.1) quando coincidir com um ifIndex, e heurísticas sobre <span className="mono">mtxrOpticalName</span> (col.2). Os campos abaixo são OIDs <strong>opcionais</strong> para telemetria SNMP GET.
              </p>
            </InfoHint>
          </h4>
          <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Status das interfaces</label>
              <input className="input mono" value={mkInterfacesStatusOid} onChange={(e) => setMkInterfacesStatusOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Banda recebida (RX)</label>
              <input className="input mono" value={mkBandwidthRxOid} onChange={(e) => setMkBandwidthRxOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Banda enviada (TX)</label>
              <input className="input mono" value={mkBandwidthTxOid} onChange={(e) => setMkBandwidthTxOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Potência SFP (TX)</label>
              <input
                className="input mono"
                value={mkSfpTxOid}
                onChange={(e) => setMkSfpTxOid(e.target.value)}
                placeholder="1.3.6.1.4.1.14988.1.1.19.1.1.9"
              />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Potência SFP (RX)</label>
              <input
                className="input mono"
                value={mkSfpRxOid}
                onChange={(e) => setMkSfpRxOid(e.target.value)}
                placeholder="1.3.6.1.4.1.14988.1.1.19.1.1.10"
              />
            </div>
          </div>
        </div>
      </div>

      <div className="settings-conn-section">
        <h3 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Leituras SNMP extra
          <InfoHint label="OIDs adicionais">
            <p>Use quando precisar de mais objetos além dos cartões acima. Ao salvar, tudo é enviado para o servidor de forma estruturada (sem editar JSON à mão).</p>
          </InfoHint>
        </h3>
        {renderOidExtrasBlock("olt", "OLT")}
        {renderOidExtrasBlock("mikrotik", "MikroTik")}
        {renderOidExtrasBlock("servidor", "Servidor")}
        {renderOidExtrasBlock("bridge", "Pontes")}
      </div>
      <div className="settings-conn-section">
        <div className="settings-conn-block">
          <h4 style={{ marginTop: 0 }}>Resumo dos extras configurados</h4>
        {(["olt", "mikrotik", "bng", "servidor", "bridge"] as const).map((cat) => {
          const block = builtOverridesPreview()[cat];
          const list = OID_ARRAY_KEYS.flatMap((key) => block?.[key] ?? []).filter((v) => String(v).trim() !== "");
          return (
            <div key={cat} style={{ marginBottom: 8 }}>
              <strong>{OID_EXTRA_CATEGORY_LABELS[cat]}</strong>
              {list.length === 0 ? (
                <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--muted)" }}>Sem extras configurados.</p>
              ) : (
                <div className="row" style={{ gap: 6, flexWrap: "wrap", marginTop: 4 }}>
                  {list.map((oid) => (
                    <span key={`${cat}-${oid}`} className="badge badge--off mono" title={oid}>
                      {oid}
                    </span>
                  ))}
                </div>
              )}
            </div>
          );
        })}
        </div>
      </div>
      <div className="field" style={{ marginTop: 12 }}>
        <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer" }}>
          <input type="checkbox" checked={showGeneratedJson} onChange={(e) => setShowGeneratedJson(e.target.checked)} />
          Mostrar pré-visualização técnica (JSON)
        </label>
        {showGeneratedJson && (
          <pre className="mono" style={{ fontSize: 10, marginTop: 8, padding: 8, overflow: "auto", maxHeight: 240, background: "var(--panel2, #161b22)", borderRadius: 6 }}>
            {JSON.stringify(builtOverridesPreview(), null, 2)}
          </pre>
        )}
      </div>
      <button type="button" className="btn btn--primary" style={{ marginTop: 12 }} disabled={patch.isPending} onClick={() => patch.mutate()}>
        Salvar credenciais e SNMP
      </button>
      {patch.isError && <div className="msg msg--err">{(patch.error as Error).message}</div>}
      {patch.isSuccess && <div className="msg msg--ok">Alterações salvas.</div>}
    </div>
  );
}

function TelegramTestOutcome({ data, error }: { data: unknown; error: Error | null }) {
  if (error) {
    return (
      <div className="msg msg--err" style={{ marginTop: 10 }}>
        {error.message}
      </div>
    );
  }
  if (data === undefined) return null;
  if (data !== null && typeof data === "object") {
    const d = data as Record<string, unknown>;
    if (d.ok === true && d.sent === true) {
      return (
        <div className="msg msg--ok" style={{ marginTop: 10 }}>
          Mensagem de teste enviada com sucesso. Verifique o Telegram.
        </div>
      );
    }
    if (d.ok === false && typeof d.message === "string" && d.message.trim()) {
      return (
        <div className="msg msg--err" style={{ marginTop: 10 }}>
          {d.message}
        </div>
      );
    }
    if (d.ok === true) {
      return (
        <div className="msg msg--ok" style={{ marginTop: 10 }}>
          Requisição de teste concluído.
        </div>
      );
    }
  }
  return (
    <details style={{ marginTop: 10, fontSize: 12 }}>
      <summary style={{ cursor: "pointer", color: "var(--muted)" }}>Detalhe da resposta</summary>
      <pre className="mono" style={{ marginTop: 6, padding: 8, background: "var(--panel2)", borderRadius: 6, fontSize: 11, overflow: "auto" }}>
        {JSON.stringify(data, null, 2)}
      </pre>
    </details>
  );
}

const TELEGRAM_MONITORING_TEST_TEMPLATES: { id: string; label: string }[] = [
  { id: "default", label: "Padrão" },
  { id: "ping_unreachable", label: "Equipamento offline" },
  { id: "latency_high", label: "Latência alta" },
  { id: "uptime_restart_low", label: "Uptime / reinício" },
  { id: "sfp_rx", label: "SFP RX" },
  { id: "sfp_tx", label: "SFP TX" },
	{ id: "pon_off", label: "PON OFF / PON DOWN" },
  { id: "interface_down", label: "Interface DOWN" },
  { id: "telemetry_threshold", label: "Telemetria (limiar)" },
  { id: "snmp_failure", label: "Falha SNMP" },
];

const TELEGRAM_REPORTS_TEST_TEMPLATES: { id: string; label: string }[] = [
  { id: "default", label: "Padrão" },
  { id: "alerts_digest", label: "Resumo de alertas" },
  { id: "onu_monthly", label: "Relatório mensal ONU" },
];

function TelegramPanel({ id, title }: { id: string; title: string }) {
  const qc = useQueryClient();
  const path = id === "monitoring" ? "monitoring" : "reports";
  const templates = id === "monitoring" ? TELEGRAM_MONITORING_TEST_TEMPLATES : TELEGRAM_REPORTS_TEST_TEMPLATES;
  const q = useQuery({
    queryKey: ["settings-tg", id],
    queryFn: () =>
      apiFetch<{ id: string; bot_token: unknown; chat_id: string | null; topic_id: string | null }>(
        `/api/v1/settings/notifications/telegram/${path}`,
      ),
  });
  const [token, setToken] = useState("");
  const [chat, setChat] = useState("");
  const [topic, setTopic] = useState("");
  const [testTemplate, setTestTemplate] = useState("default");
  const { push: pushToast } = useAppToast();

  useEffect(() => {
    if (!q.data) return;
    setChat(q.data.chat_id ?? "");
    setTopic(q.data.topic_id ?? "");
  }, [q.data]);

  const patch = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/settings/notifications/telegram/${path}`, {
        method: "PATCH",
        json: { bot_token: token || undefined, chat_id: chat || undefined, topic_id: topic || undefined },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings-tg", id] });
      toastOk(pushToast, "Guardado com sucesso (Telegram).");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar (Telegram)."),
  });

  const test = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/settings/notifications/telegram/${path}/test`, {
        method: "POST",
        json: { template: testTemplate },
      }),
  });

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Telegram — {title}</h2>
      <p style={{ fontSize: 12, color: "var(--muted)" }}>
        Para alterar o bot, introduza um novo token abaixo. O valor já salvo não é mostrado por segurança.
      </p>
      <div className="field">
        <label>Token do bot (novo)</label>
        <input className="input mono" type="password" value={token} onChange={(e) => setToken(e.target.value)} />
      </div>
      <div className="row" style={{ gap: 8 }}>
        <input className="input" placeholder="ID do chat" value={chat} onChange={(e) => setChat(e.target.value)} />
        <input className="input" placeholder="ID do tópico (opcional)" value={topic} onChange={(e) => setTopic(e.target.value)} />
      </div>
      <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
        <label className="field" style={{ margin: 0, flex: "1 1 200px", minWidth: 180 }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Tipo de mensagem de teste</span>
          <select className="select" style={{ width: "100%" }} value={testTemplate} onChange={(e) => setTestTemplate(e.target.value)}>
            {templates.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </select>
        </label>
        <button type="button" className="btn btn--primary" disabled={patch.isPending} onClick={() => patch.mutate()}>
          Salvar
        </button>
        <button type="button" className="btn" disabled={test.isPending} onClick={() => test.mutate()}>
          Enviar mensagem de teste
        </button>
      </div>
      <TelegramTestOutcome data={test.data} error={test.error as Error | null} />
    </div>
  );
}
