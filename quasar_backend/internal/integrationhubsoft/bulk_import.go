package integrationhubsoft

import (
	"encoding/json"
)

// --- Importação em massa de clientes (permissão integrations.hubsoft_bulk) ---------------------------
//
// Superfície sensível: cria clientes/serviços de verdade na conta HubSoft de produção do operador.
// Duas fases, como a edição em massa de data_venda:
//  1. ValidateClientImportRows/ValidateServiceImportRows — 100% local (sem tocar a HubSoft), confere
//     formato de cada campo e, quando os catálogos são informados, também que cada ID (serviço,
//     vencimento, vendedor, forma de cobrança, status, grupo de cliente) realmente existe na conta —
//     evita descobrir um typo só depois de já ter criado metade do lote. Linha inválida fica de fora.
//  2. ApplyClientImportRow/ApplyServiceImportRow — cria UM cliente/serviço por vez (POST /cliente ou
//     POST /cliente/cliente_servico); o chamador (handler HTTP) grava cada resultado em
//     ops_audit_log, que também serve de histórico (GET /api/v1/ops/audit?entity_type=...).
//
// Regra de ouro: NENHUM valor "adivinhado" pode chegar à HubSoft. `atoiSafe`/`floatSafe` convertem
// texto vazio ou inválido em 0, e `strings.EqualFold(carne, "true")` trata qualquer valor que não
// seja "true" como false — ou seja, sozinhas elas transformariam dado ausente/errado num valor que
// PARECE válido. Por isso ApplyClientImportRow/ApplyServiceImportRow chamam primeiro exatamente a
// mesma validação de formato usada na fase 1 (validateClientRowProblems/validateServiceRowProblems)
// e recusam a linha ANTES de montar o corpo do pedido se ela não passar — mesmo que quem chamou o
// endpoint (o front, ou qualquer outro cliente da API) não tenha validado antes. A validação
// referencial (o ID existe no catálogo?) fica só na fase 1, porque reconsultar 5-6 catálogos a cada
// linha aplicada seria lento e redundante — um ID inexistente aqui já resulta em rejeição explícita
// da própria HubSoft, nunca em dado incorreto sendo aceite.

type ClientImportRow struct {
	Line                int    `json:"line"`
	NomeRazaoSocial     string `json:"nome_razaosocial"`
	TipoPessoa          string `json:"tipo_pessoa"` // pf | pj
	CPFCNPJ             string `json:"cpf_cnpj"`
	TelefonePrimario    string `json:"telefone_primario"`
	EmailPrincipal      string `json:"email_principal,omitempty"`
	DataNascimento      string `json:"data_nascimento,omitempty"` // YYYY-MM-DD
	RG                  string `json:"rg,omitempty"`
	InscricaoEstadual   string `json:"inscricao_estadual,omitempty"`
	IDsGruposCliente    string `json:"ids_grupos_cliente,omitempty"` // "11" ou "11,12"
	Referencia          string `json:"referencia,omitempty"`         // referência externa livre (ex.: nº do contrato/pedido de origem)
	EnderecoCEP         string `json:"endereco_cep"`
	EnderecoBairro      string `json:"endereco_bairro"`
	EnderecoLogradouro  string `json:"endereco_logradouro"`
	EnderecoNumero      string `json:"endereco_numero"`
	EnderecoComplemento string `json:"endereco_complemento,omitempty"`
	EnderecoLatitude    string `json:"endereco_latitude,omitempty"`
	EnderecoLongitude   string `json:"endereco_longitude,omitempty"`
	IDServico           string `json:"id_servico"`
	IDVencimento        string `json:"id_vencimento"`
	IDUsuarioVendedor   string `json:"id_usuario_vendedor"`
	IDFormaCobranca     string `json:"id_forma_cobranca"`
	IDServicoStatus     string `json:"id_servico_status"`
	Valor               string `json:"valor"`
	DataVenda           string `json:"data_venda"` // YYYY-MM-DD
	Carne               string `json:"carne"`      // true | false
	TaxaInstalacaoTipo  string `json:"taxa_instalacao_tipo"`
	// Login/Senha/IDInterfaceConexao — configuração de autenticação PPPoE do serviço, aplicada numa
	// segunda chamada (POST /cliente/configurar_autenticacao) depois que o cliente/serviço é criado
	// com sucesso. Opcional: sem login preenchido, essa segunda chamada nem é tentada. Ver
	// ApplyLoginConfig.
	Login              string `json:"login,omitempty"`
	Senha              string `json:"senha,omitempty"`
	IDInterfaceConexao string `json:"id_interface_conexao,omitempty"`
}

type ServiceImportRow struct {
	Line                          int    `json:"line"`
	IDCliente                     string `json:"id_cliente"`
	IDServico                     string `json:"id_servico"`
	IDVencimento                  string `json:"id_vencimento"`
	IDUsuarioVendedor             string `json:"id_usuario_vendedor"`
	IDFormaCobranca               string `json:"id_forma_cobranca"`
	IDServicoStatus               string `json:"id_servico_status"`
	Valor                         string `json:"valor"`
	DataVenda                     string `json:"data_venda"`
	Carne                         string `json:"carne"`
	TaxaInstalacaoTipo            string `json:"taxa_instalacao_tipo"`
	Referencia                    string `json:"referencia,omitempty"`
	Login                         string `json:"login,omitempty"`
	Senha                         string `json:"senha,omitempty"`
	IDInterfaceConexao            string `json:"id_interface_conexao,omitempty"`
	EnderecoInstalacaoCEP         string `json:"endereco_instalacao_cep,omitempty"`
	EnderecoInstalacaoBairro      string `json:"endereco_instalacao_bairro,omitempty"`
	EnderecoInstalacaoLogradouro  string `json:"endereco_instalacao_logradouro,omitempty"`
	EnderecoInstalacaoNumero      string `json:"endereco_instalacao_numero,omitempty"`
	EnderecoInstalacaoComplemento string `json:"endereco_instalacao_complemento,omitempty"`
	EnderecoInstalacaoLatitude    string `json:"endereco_instalacao_latitude,omitempty"`
	EnderecoInstalacaoLongitude   string `json:"endereco_instalacao_longitude,omitempty"`
}

type ImportRowValidation struct {
	Line     int      `json:"line"`
	Valid    bool     `json:"valid"`
	Problems []string `json:"problems,omitempty"`
	// Label — identificação amigável da linha para listas/relatórios (nome do cliente, ou
	// "id_cliente 1234" para serviço adicional).
	Label string `json:"label,omitempty"`
	// Info — notas informativas, nunca bloqueiam a linha (ex.: qual POP/equipamento corresponde ao
	// id_interface_conexao preenchido) — preenchido pelo handler HTTP, não pela validação em si.
	Info []string `json:"info,omitempty"`
}

type ImportValidationResult struct {
	OK      bool                  `json:"ok"`
	Message string                `json:"message,omitempty"`
	Rows    []ImportRowValidation `json:"rows"`
	Valid   int                   `json:"valid"`
	Invalid int                   `json:"invalid"`
	// UncheckedCatalogs — catálogos que não foi possível carregar (falha ao consultar a HubSoft): os
	// IDs desses campos passaram só pela checagem de formato, NÃO foram confirmados contra a conta.
	UncheckedCatalogs []string `json:"unchecked_catalogs,omitempty"`
}

// CatalogSets — os IDs válidos de cada catálogo, para a validação referencial (fase 1). Quando um
// mapa vem nil, a checagem desse catálogo é pulada (não bloqueia a validação por falta de dados) —
// mas isso precisa ficar visível para quem pediu a validação (ver UncheckedCatalogs acima).
type CatalogSets struct {
	Servico       map[string]bool
	Vencimento    map[string]bool
	Vendedor      map[string]bool
	FormaCobranca map[string]bool
	ServicoStatus map[string]bool
	GrupoCliente  map[string]bool
}

// idKeyByCatalog/arrKeyByCatalog — nome do ID e da lista na resposta real da HubSoft para cada
// catálogo usado na validação referencial. Confirmados contra a documentação E contra respostas
// reais da conta (ver bulk_import_test.go) — um nome de campo errado aqui não dá erro de parsing,
// dá pior: uma lista vazia que rejeita SILENCIOSAMENTE todo ID válido (já aconteceu com
// "grupo_cliente": a HubSoft devolve a lista em "grupo_cliente", no singular, não "grupos_cliente").
var idKeyByCatalog = map[string]string{
	"servico": "id_servico", "vencimento": "id_vencimento", "vendedor": "id",
	"forma_cobranca": "id_forma_cobranca", "servico_status": "id_servico_status", "grupo_cliente": "id_grupo_cliente",
	// estoque (cadastro de produtos — ver stock_product.go)
	"produto_categoria": "id_categoria", "produto_marca": "id_produto_marca", "produto_tipo": "id_produto_tipo",
}
var arrKeyByCatalog = map[string]string{
	"servico": "servicos", "vencimento": "vencimentos", "vendedor": "vendedores",
	"forma_cobranca": "formas_cobranca", "servico_status": "servico_status", "grupo_cliente": "grupo_cliente",
	"produto_categoria": "categorias", "produto_marca": "produto_marcas", "produto_tipo": "produto_tipos",
}

// CatalogSetsFromJSON lê os IDs válidos de um catálogo a partir do JSON devolvido pela HubSoft.
// Devolve nil (== "não consegui confirmar este catálogo", o chamador trata isso como
// "não verificado", NUNCA como "lista vazia") sempre que a resposta não tiver exatamente a chave de
// lista esperada — isso é o que evita repetir o bug do "grupo_cliente": um nome de campo errado
// falha ABERTO (pula a checagem, avisa o operador), nunca FECHADO (rejeitar todo ID, certo ou não).
func CatalogSetsFromJSON(which string, body []byte) map[string]bool {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return nil
	}
	idKey, arrKey := idKeyByCatalog[which], arrKeyByCatalog[which]
	if idKey == "" || arrKey == "" {
		return nil
	}
	rawArr, present := root[arrKey]
	if !present {
		return nil // chave de lista não bate com a resposta — não finge que confirmou nada
	}
	arr, _ := rawArr.([]any)
	out := map[string]bool{}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if id := pickStr(m, idKey); id != "" {
			out[id] = true
		}
	}
	return out
}
