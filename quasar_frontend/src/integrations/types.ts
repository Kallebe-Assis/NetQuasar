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

export type ConsumerConfig = {
  client_search: {
    enabled: boolean;
    request_id?: string;
  };
};

export type BuscaOption = {
  value: string;
  label: string;
};

export type ClientServiceSummary = {
  id?: string;
  name?: string;
  status?: string;
  login?: string;
  ipv4?: string;
};

export type ClientCard = {
  id?: string;
  code?: string;
  name?: string;
  trade_name?: string;
  document?: string;
  email?: string;
  phone?: string;
  ipv4?: string;
  status?: string;
  address?: string;
  services?: ClientServiceSummary[];
  details?: Record<string, string>;
  raw?: Record<string, unknown>;
};

export type ClientSearchResponse = {
  ok: boolean;
  message?: string;
  clients: ClientCard[];
  raw_status?: string;
  status_code?: number;
  latency_ms?: number;
  request_url?: string;
  request_method?: string;
};

export type ConsumerMeta = {
  integration_name: string;
  integration_enabled: boolean;
  client_search_enabled: boolean;
  client_search_request_id?: string;
  client_search_request_name?: string;
  busca_options: BuscaOption[];
};

export type IntegrationDetail = IntegrationSummary & {
  default_headers: Record<string, string>;
  variables: Record<string, string>;
  consumer_config: ConsumerConfig;
  auth_config: Record<string, unknown>;
  timeout_ms: number;
  tls_insecure: boolean;
  password_configured?: boolean;
  token_configured?: boolean;
  session_active?: boolean;
  requests: IntegrationRequest[];
};

export type IntegrationRunLog = {
  id: string;
  request_id?: string | null;
  run_kind: string;
  ok: boolean;
  status_code?: number | null;
  latency_ms?: number | null;
  request_url?: string | null;
  response_preview?: string | null;
  error_message?: string | null;
  created_at: string;
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
  { id: "oauth2_password", label: "OAuth2 — obter token (recomendado)" },
  { id: "bearer", label: "Bearer token (já tenho o token)" },
  { id: "basic", label: "Basic (utilizador/senha)" },
  { id: "api_key", label: "API Key (header)" },
  { id: "login", label: "Login avançado (JSON manual)" },
] as const;

/** Campos típicos de auth_config para OAuth2 password grant. */
export type OAuth2PasswordAuthConfig = {
  login_path?: string;
  login_method?: string;
  client_id?: string;
  client_secret?: string;
  username?: string;
  password?: string;
  grant_type?: string;
  login_body_type?: "json" | "form";
  token_json_path?: string;
  token_prefix?: string;
  client_secret_configured?: boolean;
  password_configured?: boolean;
};

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
