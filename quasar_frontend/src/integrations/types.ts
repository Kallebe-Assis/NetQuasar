export type IntegrationSummary = {
  id: string;
  name: string;
  slug: string;
  description?: string | null;
  base_url: string;
  enabled: boolean;
  auth_type: string;
  request_count: number;
  last_test_at?: string | null;
  last_test_ok?: boolean | null;
  last_test_message?: string | null;
};

export type PathParam = {
  name: string;
  value: string;
  source: "static" | "variable";
};

export type QueryParam = {
  key: string;
  value: string;
  enabled?: boolean;
};

export type IntegrationRequest = {
  id: string;
  integration_id: string;
  name: string;
  description?: string | null;
  method: string;
  path: string;
  path_params: PathParam[];
  query_params: QueryParam[];
  headers: Record<string, string>;
  body_template?: string | null;
  body_type: string;
  extract_json_path?: string | null;
  is_login: boolean;
  sort_order: number;
  enabled: boolean;
  last_run_at?: string | null;
  last_run_ok?: boolean | null;
  last_run_status?: number | null;
  last_run_message?: string | null;
};

export type IntegrationDetail = IntegrationSummary & {
  default_headers: Record<string, string>;
  variables: Record<string, string>;
  auth_config: Record<string, unknown>;
  timeout_ms: number;
  tls_insecure: boolean;
  password_configured?: boolean;
  token_configured?: boolean;
  session_active?: boolean;
  requests: IntegrationRequest[];
};

export type RunResult = {
  ok: boolean;
  status_code?: number;
  latency_ms?: number;
  request_url?: string;
  request_method?: string;
  response_preview?: string;
  extracted?: unknown;
  error?: string;
  token_received?: boolean;
  request_id?: string;
  request_name?: string;
};

export const AUTH_TYPES = [
  { id: "none", label: "Sem autenticação" },
  { id: "bearer", label: "Bearer token" },
  { id: "basic", label: "Basic (utilizador/senha)" },
  { id: "api_key", label: "API Key (header)" },
  { id: "login", label: "Login (obter token)" },
] as const;

export const HTTP_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"] as const;

export function parsePathParams(raw: unknown): PathParam[] {
  if (!Array.isArray(raw)) return [];
  return raw
    .filter((x) => x && typeof x === "object")
    .map((x) => {
      const o = x as Record<string, unknown>;
      return {
        name: String(o.name ?? ""),
        value: String(o.value ?? ""),
        source: o.source === "variable" ? "variable" : "static",
      };
    });
}

export function parseQueryParams(raw: unknown): QueryParam[] {
  if (!Array.isArray(raw)) return [];
  return raw
    .filter((x) => x && typeof x === "object")
    .map((x) => {
      const o = x as Record<string, unknown>;
      return {
        key: String(o.key ?? ""),
        value: String(o.value ?? ""),
        enabled: o.enabled !== false,
      };
    });
}

export function parseHeaders(raw: unknown): Record<string, string> {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    out[k] = String(v ?? "");
  }
  return out;
}
