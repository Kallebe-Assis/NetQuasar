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

export type ClientSearchProvider = "auto" | "hubsoft" | "ixc" | "generic";

export const CLIENT_SEARCH_PROVIDERS: { id: ClientSearchProvider; label: string }[] = [
  { id: "auto", label: "Detectar automaticamente (Hubsoft / IXC / genérico)" },
  { id: "ixc", label: "IXC (webservice — POST + ixcsoft:listar)" },
  { id: "hubsoft", label: "Hubsoft (GET /integracao/cliente)" },
  { id: "generic", label: "Genérico (só variáveis na requisição)" },
];

export type SearchFieldMapping = {
  qtype?: string;
  oper?: string;
  termo_format?: "digits" | "raw" | "br_document";
};

export type ConsumerConfig = {
  client_search: {
    enabled: boolean;
    request_id?: string;
    provider?: ClientSearchProvider;
    /** Header ixcsoft na listagem IXC (padrão: listar). */
    ixc_list_action?: string;
    /** Tipos de busca na Consulta; vazio = padrão do ERP. */
    busca_options?: BuscaOption[];
    /** Mapeamento por tipo (ex. cpf_cnpj → qtype/oper). */
    field_mappings?: Record<string, SearchFieldMapping>;
    /** Várias tentativas automáticas ao buscar CPF/CNPJ (padrão: true). */
    cpf_multi_attempt?: boolean;
  };
  client_attendance?: {
    enabled: boolean;
    request_id?: string;
    provider?: ClientSearchProvider;
    ixc_list_action?: string;
    field_mappings?: Record<string, SearchFieldMapping>;
  };
  client_work_order?: {
    enabled: boolean;
    request_id?: string;
    provider?: ClientSearchProvider;
    ixc_list_action?: string;
    field_mappings?: Record<string, SearchFieldMapping>;
  };
  client_login?: {
    enabled: boolean;
    request_id?: string;
    provider?: ClientSearchProvider;
    ixc_list_action?: string;
    field_mappings?: Record<string, SearchFieldMapping>;
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
  status_label?: string;
  status_internet?: string;
  login?: string;
  ipv4?: string;
  mac?: string;
  online?: string;
  online_label?: string;
  contrato?: string;
  contrato_id?: string;
  plano_venda?: string;
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
  response_preview?: string;
  provider?: ClientSearchProvider;
};

export type AttendanceItem = {
  id?: string;
  protocol?: string;
  status?: string;
  status_label?: string;
  subject?: string;
  description?: string;
  opened_at?: string;
  closed_at?: string;
  pending?: boolean;
  raw?: Record<string, unknown>;
};

export type WorkOrderItem = {
  id?: string;
  number?: string;
  status?: string;
  status_label?: string;
  plan_name?: string;
  service_status?: string;
  value?: string;
  description?: string;
  scheduled_at?: string;
  created_at?: string;
  attendance_protocol?: string;
  raw?: Record<string, unknown>;
};

export type ClientAttendanceResponse = {
  ok: boolean;
  message?: string;
  items: AttendanceItem[];
  raw_status?: string;
  status_code?: number;
  latency_ms?: number;
  request_url?: string;
  request_method?: string;
};

export type ClientWorkOrderResponse = {
  ok: boolean;
  message?: string;
  items: WorkOrderItem[];
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
  client_search_provider?: ClientSearchProvider;
  busca_options: BuscaOption[];
  client_attendance_enabled?: boolean;
  client_attendance_request_id?: string;
  client_attendance_request_name?: string;
  busca_atendimento_options?: BuscaOption[];
  client_work_order_enabled?: boolean;
  client_work_order_request_id?: string;
  client_work_order_request_name?: string;
  busca_ordem_servico_options?: BuscaOption[];
  client_login_enabled?: boolean;
  client_login_request_id?: string;
  client_login_request_name?: string;
};

export type ClientLoginResponse = {
  ok: boolean;
  message?: string;
  items: ClientServiceSummary[];
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
  { id: "basic", label: "Basic (usuário/senha)" },
  { id: "api_key", label: "API Key (header)" },
  { id: "login", label: "Login avançado (JSON manual)" },
] as const;

/** Prefixos do header Authorization para auth_type=bearer (e sessão OAuth2). */
export const AUTH_TOKEN_PREFIXES = [
  { id: "Bearer", label: "Bearer" },
  { id: "Basic", label: "Basic" },
] as const;

export const TERMO_FORMAT_OPTIONS = [
  { id: "", label: "Padrão do ERP" },
  { id: "digits", label: "Só dígitos" },
  { id: "raw", label: "Texto digitado" },
  { id: "br_document", label: "CPF/CNPJ formatado (BR)" },
] as const;

export function normalizeAuthTokenPrefix(raw?: string | null): string {
  const v = (raw ?? "Bearer").trim();
  return AUTH_TOKEN_PREFIXES.some((p) => p.id === v) ? v : "Bearer";
}

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
