/**
 * Colunas dos CSVs de produtos e patrimônios de estoque da HubSoft — compartilhadas pela IMPORTAÇÃO (aba Estoque) e pela
 * CONFERÊNCIA. Cada coluna aceita também nomes alternativos (ex.: o CSV de revisão «nome_final_na_hubsoft»).
 */

export type StockField = { key: string; aliases: string[]; required?: boolean };

export const PRODUCT_FIELDS: StockField[] = [
  { key: "codigo", aliases: ["codigo", "id_produto_ixc", "código"] },
  { key: "nome", aliases: ["nome", "nome_final_na_hubsoft", "nome_final"] },
  { key: "id_categoria", aliases: ["id_categoria"] },
  { key: "id_marca", aliases: ["id_marca", "id_produto_marca"] },
  { key: "id_tipo", aliases: ["id_tipo", "id_produto_tipo", "tipo_produto"] },
  { key: "unidade_medida", aliases: ["unidade_medida", "unidade"] },
  { key: "controle_patrimonial", aliases: ["controle_patrimonial"] },
  { key: "epi", aliases: ["epi"] },
  { key: "valor_compra", aliases: ["valor_compra"] },
  { key: "valor_venda", aliases: ["valor_venda"] },
  { key: "estoque_minimo", aliases: ["estoque_minimo"] },
  { key: "incluir_nota_fiscal", aliases: ["incluir_nota_fiscal"] },
  { key: "permite_venda_cliente", aliases: ["permite_venda_cliente"] },
  { key: "permite_comodato_cliente", aliases: ["permite_comodato_cliente"] },
  { key: "permite_vinculo_pop", aliases: ["permite_vinculo_pop"] },
  { key: "permite_vinculo_projeto_mapeamento", aliases: ["permite_vinculo_projeto_mapeamento"] },
  { key: "permite_vinculo_usuario", aliases: ["permite_vinculo_usuario"] },
  { key: "permite_vinculo_composicao", aliases: ["permite_vinculo_composicao"] },
];
/** Colunas que podem faltar no arquivo de produtos. */
export const PRODUCT_OPTIONAL = new Set(["codigo", "id_tipo", "epi", "estoque_minimo"]);

export const ITEM_FIELDS: StockField[] = [
  { key: "id_produto", aliases: ["id_produto", "id_produto_hubsoft"], required: true },
  { key: "produto_nome", aliases: ["produto_nome", "produto_hubsoft", "produto"] },
  { key: "identificador_proprio", aliases: ["identificador_proprio", "identificador"], required: true },
  { key: "identificador_alternativo", aliases: ["identificador_alternativo", "identificador_antigo", "num_patrimonio_ixc"] },
  { key: "numero_serie", aliases: ["numero_serie", "serie", "n serie", "nº serie"] },
  { key: "mac_address", aliases: ["mac_address", "mac"] },
  { key: "observacoes", aliases: ["observacoes", "observacao"] },
  { key: "id_produto_item", aliases: ["id_produto_item"] },
  { key: "referencia", aliases: ["referencia", "codigo_ixc"] },
];

/** Normaliza um cabeçalho de CSV: sem acento, minúsculo, sem o trecho entre parênteses. */
export const norm = (h: string) =>
  h
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/\s*\(.*\)\s*/g, "")
    .trim();

/** Lê uma coluna pelo primeiro nome alternativo presente (e preenchido). */
export function pickField(r: Record<string, string>, aliases: string[]): string {
  const byNorm: Record<string, string> = {};
  for (const [k, v] of Object.entries(r)) byNorm[norm(k)] = v;
  for (const a of aliases) {
    const v = byNorm[norm(a)];
    if (v !== undefined && v.trim() !== "") return v.trim();
  }
  return "";
}

/** Colunas obrigatórias que o arquivo não tem (lista vazia = arquivo reconhecido). */
export function absentColumns(firstRow: Record<string, string>, fields: StockField[], optional?: Set<string>): string[] {
  const headers = new Set(Object.keys(firstRow).map(norm));
  const isRequired = (f: StockField) => f.required === true || (f.required === undefined && optional !== undefined && !optional.has(f.key));
  return fields.filter((f) => isRequired(f) && !f.aliases.some((a) => headers.has(norm(a)))).map((f) => f.key);
}
