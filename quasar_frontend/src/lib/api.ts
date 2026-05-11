import { apiUrl, getAuthToken, getStoredApiKey } from "./auth";

export class ApiError extends Error {
  status: number;
  code?: string;
  body?: unknown;
  constructor(message: string, status: number, code?: string, body?: unknown) {
    super(message);
    this.status = status;
    this.code = code;
    this.body = body;
  }
}

type Opt = RequestInit & { json?: unknown; skipAuth?: boolean };

export async function apiFetch<T = unknown>(path: string, opts: Opt = {}): Promise<T> {
  const { json, skipAuth, headers: hIn, ...rest } = opts;
  const headers = new Headers(hIn);
  if (json !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (!skipAuth) {
    const token = getAuthToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }
  const key = getStoredApiKey();
  if (key) headers.set("X-API-Key", key);

  const res = await fetch(apiUrl(path), {
    ...rest,
    headers,
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });

  const ct = res.headers.get("content-type") ?? "";
  const isJson = ct.includes("application/json");
  const data = isJson ? await res.json().catch(() => ({})) : await res.text();

  if (!res.ok) {
    const errObj = data as { error?: string; code?: string };
    const msg = errObj?.error ?? res.statusText;
    throw new ApiError(msg, res.status, errObj?.code, data);
  }
  return data as T;
}

export function downloadBlob(filename: string, blob: Blob) {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  URL.revokeObjectURL(a.href);
}
