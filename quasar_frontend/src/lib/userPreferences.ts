import { apiFetch } from "./api";
import { apiUrl, getAuthToken, getStoredApiKey } from "./auth";
import type { UiTheme } from "./theme";

export type UserPreferences = {
  theme: UiTheme;
  alert_toast_everywhere: boolean;
  alert_sound_enabled: boolean;
  alert_sound_id: string;
  custom_sounds?: UserAlertSound[];
};

export type UserAlertSound = {
  id: string;
  name: string;
  filename?: string;
  kind?: "builtin" | "custom";
};

export const DEFAULT_USER_PREFERENCES: UserPreferences = {
  theme: "dark",
  alert_toast_everywhere: true,
  alert_sound_enabled: true,
  alert_sound_id: "builtin:alert",
};

export const BUILTIN_ALERT_SOUNDS: { id: string; name: string; src: string }[] = [
  { id: "builtin:alert", name: "Alerta", src: "/sounds/alert.wav" },
  { id: "builtin:chime", name: "Sino", src: "/sounds/chime.wav" },
  { id: "builtin:urgent", name: "Urgente", src: "/sounds/urgent.wav" },
  { id: "builtin:ping", name: "Toque", src: "/sounds/ping.wav" },
];

export function normalizeUserPreferences(raw: Partial<UserPreferences> | null | undefined): UserPreferences {
  const theme = raw?.theme === "light" ? "light" : "dark";
  return {
    theme,
    alert_toast_everywhere: raw?.alert_toast_everywhere !== false,
    alert_sound_enabled: raw?.alert_sound_enabled !== false,
    alert_sound_id: raw?.alert_sound_id?.trim() || DEFAULT_USER_PREFERENCES.alert_sound_id,
    custom_sounds: Array.isArray(raw?.custom_sounds) ? raw.custom_sounds : [],
  };
}

export function fetchMyPreferences() {
  return apiFetch<UserPreferences>("/api/v1/me/preferences").then(normalizeUserPreferences);
}

export function patchMyPreferences(body: Partial<UserPreferences>) {
  return apiFetch<UserPreferences>("/api/v1/me/preferences", { method: "PATCH", json: body }).then(normalizeUserPreferences);
}

export function builtinSoundSrc(id: string): string | null {
  return BUILTIN_ALERT_SOUNDS.find((s) => s.id === id)?.src ?? null;
}

export function customSoundFileId(id: string): string {
  return id.startsWith("custom:") ? id.slice("custom:".length) : id;
}

function sessionHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};
  const token = getAuthToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const key = getStoredApiKey();
  if (key) headers["X-API-Key"] = key;
  return headers;
}

export async function fetchCustomAlertSoundUrl(id: string): Promise<string> {
  const res = await fetch(apiUrl(`/api/v1/me/alert-sounds/${encodeURIComponent(customSoundFileId(id))}`), {
    headers: sessionHeaders(),
  });
  if (!res.ok) throw new Error("Falha ao carregar o som");
  const blob = await res.blob();
  return URL.createObjectURL(blob);
}

export async function uploadAlertSound(file: File, name?: string) {
  const fd = new FormData();
  fd.append("file", file);
  if (name?.trim()) fd.append("name", name.trim());
  const res = await fetch(apiUrl("/api/v1/me/alert-sounds"), {
    method: "POST",
    headers: sessionHeaders(),
    body: fd,
  });
  const data = (await res.json().catch(() => ({}))) as { error?: string; id?: string; name?: string };
  if (!res.ok) throw new Error(data.error || `Falha ao enviar MP3 (${res.status})`);
  return data as UserAlertSound;
}

export function deleteAlertSound(id: string) {
  return apiFetch<{ ok: boolean; preferences?: UserPreferences }>(
    `/api/v1/me/alert-sounds/${encodeURIComponent(customSoundFileId(id))}`,
    { method: "DELETE" },
  );
}
