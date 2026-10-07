// Campos e leitura do CSV de cadastro — compartilhados pela importação e pela conferência de cadastros,
// para os dois lerem o MESMO arquivo exatamente do mesmo jeito.

export type ImportKind = "client" | "service";

export const CLIENT_FIELDS = [
  "nome_razaosocial", "tipo_pessoa", "cpf_cnpj", "telefone_primario", "email_principal",
  "data_nascimento", "rg", "inscricao_estadual", "ids_grupos_cliente", "referencia",
  "endereco_cep", "endereco_bairro", "endereco_logradouro", "endereco_numero", "endereco_complemento",
  "endereco_latitude", "endereco_longitude",
  "id_servico", "id_vencimento", "id_usuario_vendedor", "id_forma_cobranca", "id_servico_status",
  "valor", "data_venda", "carne", "taxa_instalacao_tipo",
  "login", "senha", "id_interface_conexao",
] as const;

export const SERVICE_FIELDS = [
  "id_cliente", "id_servico", "id_vencimento", "id_usuario_vendedor", "id_forma_cobranca",
  "id_servico_status", "valor", "data_venda", "carne", "taxa_instalacao_tipo", "referencia",
  "endereco_instalacao_cep", "endereco_instalacao_bairro", "endereco_instalacao_logradouro",
  "endereco_instalacao_numero", "endereco_instalacao_complemento",
  "endereco_instalacao_latitude", "endereco_instalacao_longitude",
  "login", "senha", "id_interface_conexao",
] as const;

// Aceita tanto o cabeçalho "cru" da API quanto o nosso CSV de migração (que tem sufixos tipo
// "tipo_pessoa (pf/pj)", "carne (true/false)" e "inscricao_estadual (só PJ)").
const HEADER_ALIASES: Record<string, string[]> = {
  tipo_pessoa: ["tipo_pessoa", "tipo_pessoa (pf/pj)"],
  carne: ["carne", "carne (true/false)"],
  inscricao_estadual: ["inscricao_estadual", "inscricao_estadual (só pj)"],
};

export function pick(row: Record<string, string>, field: string): string {
  if (row[field] !== undefined) return row[field];
  for (const alias of HEADER_ALIASES[field] ?? []) {
    const hit = Object.keys(row).find((k) => k.toLowerCase() === alias.toLowerCase());
    if (hit) return row[hit];
  }
  return "";
}

export const MAX_CSV_ROWS = 2000;

/** Linhas do CSV no formato da API: `line` numérico (cabeçalho = linha 1) + só os campos conhecidos. */
export function buildApiRows(rawRows: Record<string, string>[], fields: readonly string[]) {
  return rawRows.map((r, i) => {
    const o: Record<string, string | number> = { line: i + 2 };
    for (const f of fields) o[f] = pick(r, f);
    return o;
  });
}

// --- Modelo CSV para download ------------------------------------------------------------------------------------

// Cabeçalhos do modelo: os mesmos do CSV de migração (com os sufixos de ajuda que `pick` já aceita).
const TEMPLATE_HEADER: Record<string, string> = {
  tipo_pessoa: "tipo_pessoa (pf/pj)",
  carne: "carne (true/false)",
  inscricao_estadual: "inscricao_estadual (só PJ)",
};

// Linha de exemplo. O CPF "000.000.000-00" é inválido DE PROPÓSITO: se alguém esquecer a linha no arquivo, a
// validação a recusa — ela nunca vira um cadastro real.
const CLIENT_EXAMPLE: Record<string, string> = {
  nome_razaosocial: "EXEMPLO - APAGAR ESTA LINHA", tipo_pessoa: "pf", cpf_cnpj: "000.000.000-00", telefone_primario: "22999999999",
  email_principal: "exemplo@dominio.com.br", data_nascimento: "1990-01-31", ids_grupos_cliente: "11",
  endereco_cep: "28460000", endereco_bairro: "Centro", endereco_logradouro: "Rua Exemplo", endereco_numero: "100",
  id_servico: "57", id_vencimento: "4", id_usuario_vendedor: "86", id_forma_cobranca: "14", id_servico_status: "11",
  valor: "79.9", data_venda: "2026-01-31", carne: "true", taxa_instalacao_tipo: "nao_cobrar_taxa",
  login: "exemplo01", senha: "g212345", id_interface_conexao: "5",
};

const SERVICE_EXAMPLE: Record<string, string> = {
  id_cliente: "0", id_servico: "57", id_vencimento: "4", id_usuario_vendedor: "86", id_forma_cobranca: "14", id_servico_status: "11",
  valor: "79.9", data_venda: "2026-01-31", carne: "true", taxa_instalacao_tipo: "nao_cobrar_taxa",
  endereco_instalacao_cep: "28460000", endereco_instalacao_bairro: "Centro", endereco_instalacao_logradouro: "Rua Exemplo",
  endereco_instalacao_numero: "100", login: "exemplo01", senha: "g212345", id_interface_conexao: "5",
};

/** Cabeçalho + linha de exemplo do modelo CSV (clientes novos ou serviços adicionais). */
export function templateFor(kind: ImportKind): { head: string[]; example: string[] } {
  const fields = kind === "client" ? CLIENT_FIELDS : SERVICE_FIELDS;
  const ex = kind === "client" ? CLIENT_EXAMPLE : SERVICE_EXAMPLE;
  return { head: fields.map((f) => TEMPLATE_HEADER[f] ?? f), example: fields.map((f) => ex[f] ?? "") };
}
