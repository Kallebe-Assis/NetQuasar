import { DEFAULT_USER_PERMISSIONS, hasAnyPermission, hasPermission, type PermissionKey } from "./permissions";

const K_BASE = "netquasar_api_base";
const K_KEY = "netquasar_api_key";
const K_READY = "netquasar_session_ready";
const K_CLIENT = "netquasar_client_configured";
const K_AUTH = "netquasar_auth_token";
const K_ROLE = "netquasar_user_role";
const K_USER_LABEL = "netquasar_user_label";
const K_PERMS = "netquasar_user_permissions";
const K_PROFILE = "netquasar_permission_profile_id";

/** Base da API sem barra final, ex.: http://localhost:8080 ou vazio (mesma origem + proxy Vite). */
export function getApiBase(): string {
  const v = import.meta.env.VITE_API_BASE?.trim();
  if (v) return v.replace(/\/$/, "");
  return "";
}

export function getStoredApiBase(): string {
  return localStorage.getItem(K_BASE) ?? getApiBase();
}

export function getStoredApiKey(): string {
  return localStorage.getItem(K_KEY) ?? "";
}

export const AUTH_CHANGED_EVENT = "netquasar-auth-changed";

function emitAuthChanged() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(AUTH_CHANGED_EVENT));
}

export function getAuthToken(): string {
  return localStorage.getItem(K_AUTH) ?? "";
}

export function saveAuthToken(token: string) {
  const t = token.trim();
  if (t) localStorage.setItem(K_AUTH, t);
  else localStorage.removeItem(K_AUTH);
  emitAuthChanged();
}

export function saveSession(apiBase: string, apiKey: string) {
  localStorage.setItem(K_BASE, apiBase.replace(/\/$/, ""));
  localStorage.setItem(K_KEY, apiKey);
}

/** Marca que este browser já passou pelo assistente de URL/chave (não voltar a pedir até limpar dados). */
export function markClientConfigured() {
  localStorage.setItem(K_CLIENT, "1");
}

/**
 * Indica se o dispositivo já tem destino de API definido (ou build-time VITE_API_BASE).
 * Migração: quem já tinha netquasar_api_base gravado conta como configurado.
 */
export function isClientConfigured(): boolean {
  if (localStorage.getItem(K_CLIENT) === "1") return true;
  if (localStorage.getItem(K_BASE)?.trim()) return true;
  if (getApiBase().trim()) return true;
  return false;
}

export function clearClientSetup() {
  localStorage.removeItem(K_CLIENT);
}

/** Limpa URL/chave do servidor (volta ao assistente `/client-setup`). */
export function clearClientSetupAndApi() {
  localStorage.removeItem(K_CLIENT);
  localStorage.removeItem(K_BASE);
  localStorage.removeItem(K_KEY);
}

/** Chamado após login com credenciais válidas — rotas internas exigem isto. */
export function markSessionReady() {
  localStorage.setItem(K_READY, "1");
}

export function isSessionReady(): boolean {
  return localStorage.getItem(K_READY) === "1";
}

/** Termina a sessão do utilizador (mantém URL/chave do servidor para voltar a entrar mais rápido). */
export function clearSession() {
  localStorage.removeItem(K_READY);
  localStorage.removeItem(K_AUTH);
  localStorage.removeItem(K_ROLE);
  localStorage.removeItem(K_USER_LABEL);
  localStorage.removeItem(K_PERMS);
  localStorage.removeItem(K_PROFILE);
  emitAuthChanged();
}

/** Nome ou e-mail mostrado na shell (gravado no login). */
export function saveUserDisplayLabel(label: string) {
  const t = label.trim();
  if (t) localStorage.setItem(K_USER_LABEL, t);
  else localStorage.removeItem(K_USER_LABEL);
}

export function getStoredUserDisplayLabel(): string {
  return localStorage.getItem(K_USER_LABEL)?.trim() ?? "";
}

/** Grava o papel devolvido pelo login (`admin` | `viewer`). */
export function saveUserRole(role: string) {
  const t = role.trim().toLowerCase();
  if (t === "admin" || t === "viewer") {
    localStorage.setItem(K_ROLE, t);
  } else {
    localStorage.removeItem(K_ROLE);
  }
}

export function getStoredUserRole(): "admin" | "viewer" | null {
  const r = (localStorage.getItem(K_ROLE) ?? "").trim().toLowerCase();
  if (r === "admin" || r === "viewer") return r;
  return null;
}

export function saveUserPermissions(perms: string[] | null | undefined) {
  if (!perms || perms.length === 0) {
    localStorage.removeItem(K_PERMS);
    return;
  }
  localStorage.setItem(K_PERMS, JSON.stringify(perms));
}

/**
 * Permissões da sessão actual.
 * Viewer sem lista gravada (sessões antigas / login sem payload) herda o catálogo de visualização padrão,
 * alinhado ao perfil «Usuário» do backend — senão o menu lateral fica vazio.
 */
export function getStoredUserPermissions(): string[] {
  const raw = localStorage.getItem(K_PERMS);
  if (!raw) {
    if (getStoredUserRole() === "admin") return ["*"];
    if (getStoredUserRole() === "viewer") return [...DEFAULT_USER_PERMISSIONS];
    return [];
  }
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      if (getStoredUserRole() === "viewer") return [...DEFAULT_USER_PERMISSIONS];
      return [];
    }
    const list = parsed.map((x) => String(x)).filter(Boolean);
    if (list.length === 0 && getStoredUserRole() === "viewer") {
      return [...DEFAULT_USER_PERMISSIONS];
    }
    return list;
  } catch {
    if (getStoredUserRole() === "viewer") return [...DEFAULT_USER_PERMISSIONS];
    return [];
  }
}

/** Chave estável para re-renderizar UI quando as permissões da sessão mudam. */
export function getStoredUserPermissionsKey(): string {
  return getStoredUserPermissions().slice().sort().join("|");
}

export function savePermissionProfileId(id: string | null | undefined) {
  const t = String(id ?? "").trim();
  if (t) localStorage.setItem(K_PROFILE, t);
  else localStorage.removeItem(K_PROFILE);
}

export function getStoredPermissionProfileId(): string | null {
  const t = localStorage.getItem(K_PROFILE)?.trim();
  return t || null;
}

/** Visitante (viewer) — só leitura nas áreas restritas (legado). */
export function isViewerUser(): boolean {
  return getStoredUserRole() === "viewer" && !hasPermission(getStoredUserPermissions(), "*");
}

/**
 * Utilizador com permissões de administrador na UI.
 * Sessões antigas sem `K_ROLE` gravado tratam-se como admin (compatível com tokens já emitidos).
 */
export function isAdminUser(): boolean {
  if (!getAuthToken()) return false;
  if (hasPermission(getStoredUserPermissions(), "*")) return true;
  if (getStoredUserRole() === "admin") return true;
  if (isViewerUser()) return false;
  // Sessão antiga sem role/perms.
  if (!getStoredUserRole() && getStoredUserPermissions().length === 0) return true;
  return false;
}

/** Verifica permissão unitária da sessão actual. */
export function can(permission: PermissionKey | string): boolean {
  if (!getAuthToken()) return false;
  if (isAdminUser()) return true;
  return hasPermission(getStoredUserPermissions(), permission);
}

export function canAny(...permissions: Array<PermissionKey | string>): boolean {
  if (!getAuthToken()) return false;
  if (isAdminUser()) return true;
  return hasAnyPermission(getStoredUserPermissions(), permissions);
}

export function apiUrl(path: string): string {
  const base = getStoredApiBase();
  const p = path.startsWith("/") ? path : `/${path}`;
  if (!base) return p;
  return `${base}${p}`;
}
