package integrationhubsoft

import (
	"context"
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
}

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
}

// FetchCatalog chama um dos endpoints de Configuração acima e devolve o corpo JSON tal como veio da
// HubSoft — a tela só exibe/exporta o resultado, então não há necessidade de tipar cada catálogo.
func FetchCatalog(ctx context.Context, cfg Config, token, which string) (int, []byte, error) {
	which = strings.TrimSpace(which)
	path, ok := CatalogEndpoints[which]
	if !ok {
		return 0, nil, fmt.Errorf("catálogo desconhecido: %q", which)
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
