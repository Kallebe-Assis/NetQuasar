package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Catálogos de configuração (permissão integrations.hubsoft_bulk) ---------------------------------
//
// A API de cadastro de cliente/serviço exige IDs internos da conta HubSoft (id_servico,
// id_servico_status, id_usuario_vendedor, id_forma_cobranca, id_vencimento, etc.) que não existem
// em nenhum export do sistema antigo — só dá para resolver consultando a própria HubSoft. Estes são
// os endpoints de "Configuração" (somente leitura) que devolvem esses catálogos; a tela de Edição em
// massa lista aqui para o operador conferir os valores reais antes de preencher um CSV de importação.

// CatalogEndpoints — chave usada pelo front → caminho do endpoint GET na HubSoft.
var CatalogEndpoints = map[string]string{
	"servico":               "/api/v1/integracao/configuracao/servico",
	"servico_composicao":    "/api/v1/integracao/configuracao/servico", // mesmo endpoint, com dados_composicao_servico=sim
	"servico_status":        "/api/v1/integracao/configuracao/servico_status",
	"servico_tecnologia":    "/api/v1/integracao/configuracao/servico_tecnologia",
	"vendedor":              "/api/v1/integracao/configuracao/vendedor",
	"forma_cobranca":        "/api/v1/integracao/configuracao/forma_cobranca",
	"vencimento":            "/api/v1/integracao/configuracao/vencimento",
	"origem_cliente":        "/api/v1/integracao/configuracao/origem_cliente",
	"motivo_contratacao":    "/api/v1/integracao/configuracao/motivo_contratacao",
	"grupo_cliente":         "/api/v1/integracao/configuracao/grupo_cliente",
	"grupo_cliente_servico": "/api/v1/integracao/configuracao/grupo_cliente_servico",
	"tipo_servico":          "/api/v1/integracao/configuracao/tipo_servico",
	"cidade":                "/api/v1/integracao/configuracao/cidade",
	// "equipamento" não é de Configuração, é de Rede — devolve os equipamentos de rede (POPs, roteadores
	// etc.) com suas interfaces aninhadas (campo "interfaces", cada uma com id_interface_conexao/nome/
	// tipo) — é o único jeito de descobrir o ID certo para usar em configurar_autenticacao (não existe
	// catálogo próprio de "interface de conexão" isolado).
	"equipamento": "/api/v1/integracao/rede/equipamento",
	// "pop" é a relação inversa de "equipamento": cada POP devolvido já traz, aninhado, os seus
	// próprios equipamentos de conexão — útil pra confirmar qual POP contém qual equipamento/interface.
	"pop": "/api/v1/integracao/rede/pop",

	// --- Estoque / patrimônios (cadastro de produtos e entrada de patrimônios) ---------------------------------
	// Locais de estoque (almoxarifados): a API não expõe "repartições" dentro de um local — só id, descrição, empresa,
	// endereço e usuários. Paginado (lido inteiro pelo servidor).
	"estoque_local": "/api/v1/integracao/estoque/local_estoque",
	// Produtos cadastrados (ex.: «ROTEADOR MERCUSYS MR30G AC1200»); cada produto com controle_patrimonial=true gera um
	// patrimônio por unidade na entrada de estoque. Paginado.
	"estoque_produto": "/api/v1/integracao/estoque/produto",
	// IDs exigidos por POST /estoque/produto (categoria e marca são obrigatórias, tipo é opcional).
	"produto_categoria": "/api/v1/integracao/configuracao/categoria",
	"produto_marca":     "/api/v1/integracao/configuracao/produto_marca",
	"produto_tipo":      "/api/v1/integracao/configuracao/produto_tipo",
	// Status possíveis de um patrimônio (ESTOQUE, COMODATO, Vendido, MANUTENÇÃO…) — o prefixo é usado em alterar_status.
	"produto_item_status": "/api/v1/integracao/configuracao/produto_item_status",
	// Empresas (CNPJ) — cada local de estoque pertence a uma.
	"empresa": "/api/v1/integracao/configuracao/empresa",
}

// catalogPaged — catálogos cujo endpoint EXIGE paginação (pagina=0…): o servidor lê todas as páginas e devolve um único
// corpo com o mesmo formato (a tela de catálogos não precisa saber de páginas). ArrayKey é a chave do array na resposta.
var catalogPaged = map[string]struct {
	ArrayKey string
	Query    map[string]string
}{
	"estoque_local":   {ArrayKey: "locais_estoque", Query: map[string]string{"relacoes": "usuarios"}},
	"estoque_produto": {ArrayKey: "produtos"},
}

// catalogMaxPages — 100 itens por página: até 20 mil registros por catálogo.
const catalogMaxPages = 200

// CatalogList — ordem de exibição na tela (CatalogEndpoints é um map, sem ordem garantida).
var CatalogList = []struct{ ID, Label string }{
	{"servico", "Serviços (planos)"},
	{"servico_composicao", "Serviços — composição (empresa, p/ duplicar entre empresas)"},
	{"servico_status", "Status de serviço"},
	{"forma_cobranca", "Formas de cobrança"},
	{"vendedor", "Vendedores"},
	{"vencimento", "Vencimentos"},
	{"servico_tecnologia", "Tecnologias de serviço"},
	{"tipo_servico", "Tipos de serviço"},
	{"origem_cliente", "Origens de cliente"},
	{"motivo_contratacao", "Motivos de contratação"},
	{"grupo_cliente", "Grupos de cliente"},
	{"grupo_cliente_servico", "Grupos de serviço do cliente"},
	{"cidade", "Cidades"},
	{"equipamento", "Equipamentos de rede (POPs/interfaces de conexão)"},
	{"pop", "POPs de conexão"},
	{"estoque_local", "Estoque — locais de estoque (almoxarifados)"},
	{"estoque_produto", "Estoque — produtos cadastrados"},
	{"produto_categoria", "Estoque — categorias de produto"},
	{"produto_marca", "Estoque — marcas de produto"},
	{"produto_tipo", "Estoque — tipos de produto"},
	{"produto_item_status", "Estoque — status de patrimônio"},
	{"empresa", "Empresas"},
}

// FetchCatalog chama um dos endpoints de Configuração acima e devolve o corpo JSON tal como veio da
// HubSoft — a tela só exibe/exporta o resultado, então não há necessidade de tipar cada catálogo.
func FetchCatalog(ctx context.Context, cfg Config, token, which string) (int, []byte, error) {
	which = strings.TrimSpace(which)
	path, ok := CatalogEndpoints[which]
	if !ok {
		return 0, nil, fmt.Errorf("catálogo desconhecido: %q", which)
	}
	if paged, ok := catalogPaged[which]; ok {
		return fetchPagedCatalog(ctx, cfg, token, path, paged.ArrayKey, paged.Query)
	}
	req := integrationhttp.RequestConfig{Method: "GET", Path: path}
	if which == "servico_composicao" {
		// dados_composicao_servico=sim faz a HubSoft incluir o objeto servico_composicao (empresa,
		// tipo de documento fiscal, descrição da composição) — não vem por padrão. Junto com
		// servicos_inativos_com_cliente=sim para não esconder planos inativos que ainda têm cliente
		// (útil justamente para duplicar um plano existente numa empresa nova).
		req.QueryParams = paramKVs(map[string]string{
			"dados_composicao_servico":      "sim",
			"servicos_inativos_com_cliente": "sim",
		})
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), req)
	if !res.OK && res.StatusCode == 0 {
		return 0, nil, fmt.Errorf("%s", firstNonEmpty(res.ErrorMessage, "falha ao consultar a HubSoft"))
	}
	return res.StatusCode, ResponseBodyBytes(res), nil
}

// fetchPagedCatalog lê todas as páginas de um catálogo paginado e devolve um corpo único no formato habitual da HubSoft
// ({status, msg, paginacao, <arrayKey>: [...]}), para a tela de catálogos tratar igual aos não paginados.
func fetchPagedCatalog(ctx context.Context, cfg Config, token, path, arrayKey string, query map[string]string) (int, []byte, error) {
	items, total, err := fetchAllPages(ctx, cfg, token, path, query, catalogMaxPages, arrayKey)
	if err != nil {
		return 0, nil, err
	}
	if items == nil {
		items = []map[string]any{}
	}
	out, mErr := json.Marshal(map[string]any{
		"status": "success", "msg": "Dados consultados com sucesso",
		"paginacao": map[string]any{"total_registros": total, "lidos": len(items)},
		arrayKey:    items,
	})
	if mErr != nil {
		return 0, nil, mErr
	}
	return 200, out, nil
}
