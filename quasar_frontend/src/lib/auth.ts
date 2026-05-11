const K_BASE = "netquasar_api_base";
const K_KEY = "netquasar_api_key";
const K_READY = "netquasar_session_ready";
const K_CLIENT = "netquasar_client_configured";
const K_AUTH = "netquasar_auth_token";
const K_ROLE = "netquasar_user_role";
const K_USER_LABEL = "netquasar_user_label";

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

export function getAuthToken(): string {
  return localStorage.getItem(K_AUTH) ?? "";
}

export function saveAuthToken(token: string) {
  const t = token.trim();
  if (t) localStorage.setItem(K_AUTH, t);
  else localStorage.removeItem(K_AUTH);
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

/** Visitante (viewer) — só leitura nas áreas restritas. */
export function isViewerUser(): boolean {
  return getStoredUserRole() === "viewer";
}

/**
 * Utilizador com permissões de administrador na UI.
 * Sessões antigas sem `K_ROLE` gravado tratam-se como admin (compatível com tokens já emitidos).
 */
export function isAdminUser(): boolean {
  if (!getAuthToken()) return false;
  if (isViewerUser()) return false;
  return true;
}

export function apiUrl(path: string): string {
  const base = getStoredApiBase();
  const p = path.startsWith("/") ? path : `/${path}`;
  if (!base) return p;
  return `${base}${p}`;
}
