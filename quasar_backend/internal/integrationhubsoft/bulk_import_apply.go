// Aplicação (POST) de uma linha de cliente/serviço na HubSoft, sem a checagem de duplicidade
// (ver bulk_import_dedup.go para o fluxo usado em produção).

package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// hubsoftActionMessageWithErrors — como hubsoftActionMessage (hubsoft.go), mas também junta o array
// "errors" quando a HubSoft devolve um (ex.: "status":"error","msg":"Favor preencher os campos
// obrigatórios de acordo com as especificações","errors":["O campo X é obrigatório.", ...]). Sem
// isso, só a mensagem genérica chegava ao operador, escondendo qual campo falhou de verdade — usado
// só aqui na importação em massa (as outras ações que chamam hubsoftActionMessage não mudam).
func hubsoftActionMessageWithErrors(body []byte) string {
	var doc struct {
		Msg     string   `json:"msg"`
		Message string   `json:"message"`
		Errors  []string `json:"errors"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return ""
	}
	msg := firstNonEmpty(doc.Msg, doc.Message)
	if len(doc.Errors) > 0 {
		detail := strings.Join(doc.Errors, " | ")
		if msg != "" {
			return msg + ": " + detail
		}
		return detail
	}
	return msg
}

type ImportApplyResult struct {
	Line      int    `json:"line"`
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	IDCliente string `json:"id_cliente,omitempty"`
	IDServico string `json:"id_cliente_servico,omitempty"`
	// Rejected — recusada aqui mesmo, ANTES de qualquer chamada à HubSoft, por falhar na validação
	// de formato (ver comentário no topo do ficheiro). Diferente de uma falha devolvida pela HubSoft.
	Rejected bool `json:"rejected,omitempty"`
	// LoginOK/LoginMessage — resultado da configuração de login PPPoE (ver ApplyLoginConfig),
	// preenchido pelo chamador HTTP depois de um OK aqui. Deliberadamente SEPARADO de OK/Message: um
	// login que falhou (ex. "já em uso") não pode ser confundido com o cliente não ter sido criado —
	// quando LoginOK é nil, não houve tentativa de configurar login nesta linha.
	LoginOK      *bool  `json:"login_ok,omitempty"`
	LoginMessage string `json:"login_message,omitempty"`
	// DedupAction — qual dos 4 casos da checagem de duplicidade decidiu o que fazer nesta linha (ver
	// ApplyClientImportRowDedup/ApplyServiceImportRowDedup): "cliente_criado" | "servico_adicionado" |
	// "ja_existe" | "login_a_corrigir" (serviço já existia com o login padrão — só o login é corrigido).
	// Vazio quando a checagem de duplicidade nem rodou (ex.: linha rejeitada antes).
	DedupAction string `json:"dedup_action,omitempty"`
	// Label identifica a linha para o operador (nome do cliente, ou "id_cliente X · login" na aba de
	// serviços adicionais, cujo CSV não tem nome). Preenchido pelo chamador HTTP.
	Label string `json:"label,omitempty"`
}

// FallbackDataNascimentoMenor é a data usada quando a HubSoft recusa uma data_nascimento que deixa o
// titular com menos de 18 anos (valor definido pelo operador: 01/01/1990).
const FallbackDataNascimentoMenor = "1990-01-01"

// isUnderage — true se a data (qualquer formato aceito por normalizeDate) deixa a pessoa com menos de 18
// anos hoje. Data vazia/inválida → false.
func isUnderage(dataNascimento string, now time.Time) bool {
	_, t, ok := normalizeDate(dataNascimento)
	if !ok {
		return false
	}
	return t.AddDate(18, 0, 0).After(now) // faz 18 anos só depois de hoje (ou nasce no futuro)
}

// ApplyClientImportRow — POST /api/v1/integracao/cliente (cria cliente + primeiro serviço).
// Se a HubSoft recusar a linha por causa da data_nascimento E a pessoa tem menos de 18 anos, refaz UMA
// vez com 01/01/1990 (regra do operador). Só reenvia após uma recusa explícita citando a data de
// nascimento — nunca depois de falha de comunicação —, então não há risco de criar o cliente 2 vezes.
func ApplyClientImportRow(ctx context.Context, cfg Config, token string, r ClientImportRow) ImportApplyResult {
	r, _ = applyClientDefaults(r)
	res := applyClientImportRowOnce(ctx, cfg, token, r)
	if !res.OK && !res.Rejected && isUnderage(r.DataNascimento, time.Now()) &&
		mentionsAge(res.Message) {
		orig := r.DataNascimento
		r.DataNascimento = FallbackDataNascimentoMenor
		retry := applyClientImportRowOnce(ctx, cfg, token, r)
		if retry.OK {
			retry.Message += fmt.Sprintf(" (data de nascimento %s recusada pela HubSoft por idade menor que 18 anos — usado 01/01/1990)", orig)
		} else {
			retry.Message = fmt.Sprintf("%s [1ª tentativa com a data %s: %s]", retry.Message, orig, res.Message)
		}
		return retry
	}
	return res
}

// mentionsAge — a recusa da HubSoft cita a data de nascimento ou a idade (o texto exato não é
// documentado, então aceita as formas prováveis). Só é consultada para titulares já confirmados <18.
func mentionsAge(msg string) bool {
	m := strings.ToLower(msg)
	for _, k := range []string{"nascimento", "idade", "18 anos", "menor"} {
		if strings.Contains(m, k) {
			return true
		}
	}
	return false
}

func applyClientImportRowOnce(ctx context.Context, cfg Config, token string, r ClientImportRow) ImportApplyResult {
	res := ImportApplyResult{Line: r.Line}

	// Portão final: nunca construir/enviar um pedido a partir de uma linha que não valida, mesmo que
	// quem chamou este endpoint não tenha validado antes.
	if problems := validateClientRowProblems(r, CatalogSets{}); len(problems) > 0 {
		res.Rejected = true
		res.Message = "linha não enviada à HubSoft — falhou na validação: " + strings.Join(problems, " | ")
		return res
	}
	// A validação acima já confirmou que as duas datam parseiam — normaliza para YYYY-MM-DD (o único
	// formato aceite pela HubSoft) antes de montar o corpo, nunca envia a string crua da linha. O mesmo
	// vale para cpf_cnpj: se um zero à esquerda foi recuperado (ver normalizeCPFCNPJ), é o valor
	// recuperado — já confirmado pelo dígito verificador — que vai no corpo, nunca o cru sem o zero.
	dataVenda, _, _ := normalizeDate(r.DataVenda)
	dataNascimento, _, hasNascimento := normalizeDate(r.DataNascimento)
	cpfCNPJ, _ := normalizeCPFCNPJ(r.TipoPessoa, r.CPFCNPJ)

	endereco := map[string]any{
		"cep": onlyDigitsBulk(r.EnderecoCEP), "bairro": r.EnderecoBairro,
		"endereco": r.EnderecoLogradouro, "numero": r.EnderecoNumero,
	}
	if r.EnderecoComplemento != "" {
		endereco["complemento"] = r.EnderecoComplemento
	}
	if r.EnderecoLatitude != "" {
		lat, _ := normalizeLatLon(r.EnderecoLatitude, -90, 90) // já validado acima — nunca envia a vírgula crua
		endereco["latitude"] = lat
	}
	if r.EnderecoLongitude != "" {
		lon, _ := normalizeLatLon(r.EnderecoLongitude, -180, 180)
		endereco["longitude"] = lon
	}
	body := map[string]any{
		"nome_razaosocial": r.NomeRazaoSocial, "tipo_pessoa": strings.ToLower(r.TipoPessoa),
		"cpf_cnpj": cpfCNPJ, "telefone_primario": onlyDigitsBulk(r.TelefonePrimario),
		"endereco":   endereco,
		"id_servico": atoiSafe(r.IDServico), "id_vencimento": atoiSafe(r.IDVencimento),
		"id_usuario_vendedor": atoiSafe(r.IDUsuarioVendedor), "id_forma_cobranca": atoiSafe(r.IDFormaCobranca),
		"id_servico_status": atoiSafe(r.IDServicoStatus), "valor": floatSafe(r.Valor),
		"data_venda": dataVenda, "carne": strings.EqualFold(r.Carne, "true"),
		"taxa_instalacao_tipo": r.TaxaInstalacaoTipo,
	}
	if strings.EqualFold(strings.TrimSpace(r.Carne), "true") {
		// carne=true ("o plano usa cobrança do tipo carnê") exige a decisão sobre gerar o carnê — aqui sempre
		// "não gerar": o serviço é marcado como carnê, mas nenhum carnê/boleto é gerado pela importação.
		body["gerar_carne"] = "nao_gerar_carne"
	}
	if r.EmailPrincipal != "" {
		body["email_principal"] = r.EmailPrincipal
	}
	if r.DataNascimento != "" && hasNascimento {
		body["data_nascimento"] = dataNascimento
	}
	if r.RG != "" {
		body["rg"] = r.RG
	}
	if r.InscricaoEstadual != "" {
		body["inscricao_estadual"] = r.InscricaoEstadual
	}
	if r.Referencia != "" {
		body["referencia"] = r.Referencia
	}
	if r.IDsGruposCliente != "" {
		ids, allOK := parseIntListStrict(r.IDsGruposCliente)
		if !allOK {
			// já devia ter sido pego pela validação acima — defesa extra: nunca mandar a lista
			// pela metade (isso mudaria silenciosamente quais grupos o cliente recebe).
			res.Rejected = true
			res.Message = fmt.Sprintf("linha não enviada à HubSoft — ids_grupos_cliente com valor não numérico: %q", r.IDsGruposCliente)
			return res
		}
		if len(ids) > 0 {
			body["ids_grupos_cliente"] = ids
		}
	}

	raw, _ := json.Marshal(body)
	put := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "POST", Path: "/api/v1/integracao/cliente", BodyTemplate: string(raw), BodyType: "json",
	})
	respBody := ResponseBodyBytes(put)
	if !put.OK {
		if put.StatusCode == 0 {
			res.Message = "sem resposta da HubSoft — confira manualmente se o cliente foi criado antes de repetir"
			return res
		}
		if msg := hubsoftActionMessageWithErrors(respBody); msg != "" {
			res.Message = msg
			return res
		}
		res.Message = firstNonEmpty(put.ErrorMessage, fmt.Sprintf("HTTP %d", put.StatusCode))
		return res
	}
	var doc struct {
		Status  string `json:"status"`
		Msg     string `json:"msg"`
		Cliente struct {
			IDCliente json.Number `json:"id_cliente"`
			Servicos  []struct {
				IDClienteServico json.Number `json:"id_cliente_servico"`
			} `json:"servicos"`
		} `json:"cliente"`
	}
	_ = json.Unmarshal(respBody, &doc)
	if doc.Status != "success" || doc.Cliente.IDCliente.String() == "" {
		// HubSoft costuma devolver HTTP 200 mesmo para erro de validação (status:"error" no corpo) —
		// por isso usa hubsoftActionMessageWithErrors aqui também, não só doc.Msg, senão o array
		// "errors" (motivo específico, ex. "O campo data_nascimento é obrigatório.") se perde.
		res.Message = firstNonEmpty(hubsoftActionMessageWithErrors(respBody), "a HubSoft não confirmou a criação do cliente (resposta sem id_cliente)")
		return res
	}
	res.OK = true
	res.IDCliente = doc.Cliente.IDCliente.String()
	if len(doc.Cliente.Servicos) > 0 {
		res.IDServico = doc.Cliente.Servicos[0].IDClienteServico.String()
	}
	res.Message = firstNonEmpty(doc.Msg, "cliente criado")
	return res
}

// ApplyServiceImportRow — POST /api/v1/integracao/cliente/cliente_servico (adiciona serviço a cliente existente).
func ApplyServiceImportRow(ctx context.Context, cfg Config, token string, r ServiceImportRow) ImportApplyResult {
	res := ImportApplyResult{Line: r.Line}

	if problems := validateServiceRowProblems(r, CatalogSets{}); len(problems) > 0 {
		res.Rejected = true
		res.Message = "linha não enviada à HubSoft — falhou na validação: " + strings.Join(problems, " | ")
		return res
	}
	dataVenda, _, _ := normalizeDate(r.DataVenda) // já validado acima — normaliza para YYYY-MM-DD

	body := map[string]any{
		"id_cliente": atoiSafe(r.IDCliente), "id_servico": atoiSafe(r.IDServico),
		"id_vencimento": atoiSafe(r.IDVencimento), "id_usuario_vendedor": atoiSafe(r.IDUsuarioVendedor),
		"id_forma_cobranca": atoiSafe(r.IDFormaCobranca), "id_servico_status": atoiSafe(r.IDServicoStatus),
		"valor": floatSafe(r.Valor), "data_venda": dataVenda, "carne": strings.EqualFold(r.Carne, "true"),
		"taxa_instalacao_tipo": r.TaxaInstalacaoTipo,
	}
	if strings.EqualFold(strings.TrimSpace(r.Carne), "true") {
		body["gerar_carne"] = "nao_gerar_carne" // carne=true exige gerar_carne; a importação nunca gera carnê/boleto
	}
	if r.Referencia != "" {
		body["referencia"] = r.Referencia
	}
	// Validado acima como "tudo ou nada": ou os 4 campos obrigatórios do bloco vieram, ou nenhum.
	if r.EnderecoInstalacaoCEP != "" {
		inst := map[string]any{
			"cep": onlyDigitsBulk(r.EnderecoInstalacaoCEP), "bairro": r.EnderecoInstalacaoBairro,
			"endereco": r.EnderecoInstalacaoLogradouro, "numero": r.EnderecoInstalacaoNumero,
		}
		if r.EnderecoInstalacaoComplemento != "" {
			inst["complemento"] = r.EnderecoInstalacaoComplemento
		}
		if r.EnderecoInstalacaoLatitude != "" {
			lat, _ := normalizeLatLon(r.EnderecoInstalacaoLatitude, -90, 90)
			inst["latitude"] = lat
		}
		if r.EnderecoInstalacaoLongitude != "" {
			lon, _ := normalizeLatLon(r.EnderecoInstalacaoLongitude, -180, 180)
			inst["longitude"] = lon
		}
		body["endereco_instalacao"] = inst
	}

	raw, _ := json.Marshal(body)
	put := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "POST", Path: "/api/v1/integracao/cliente/cliente_servico", BodyTemplate: string(raw), BodyType: "json",
	})
	respBody := ResponseBodyBytes(put)
	if !put.OK {
		if put.StatusCode == 0 {
			res.Message = "sem resposta da HubSoft — confira manualmente se o serviço foi criado antes de repetir"
			return res
		}
		if msg := hubsoftActionMessageWithErrors(respBody); msg != "" {
			res.Message = msg
			return res
		}
		res.Message = firstNonEmpty(put.ErrorMessage, fmt.Sprintf("HTTP %d", put.StatusCode))
		return res
	}
	var doc struct {
		Status         string `json:"status"`
		Msg            string `json:"msg"`
		ClienteServico struct {
			IDClienteServico json.Number `json:"id_cliente_servico"`
		} `json:"cliente_servico"`
	}
	_ = json.Unmarshal(respBody, &doc)
	if doc.Status != "success" || doc.ClienteServico.IDClienteServico.String() == "" {
		res.Message = firstNonEmpty(hubsoftActionMessageWithErrors(respBody), "a HubSoft não confirmou a criação do serviço (resposta sem id_cliente_servico)")
		return res
	}
	res.OK = true
	res.IDServico = doc.ClienteServico.IDClienteServico.String()
	res.Message = firstNonEmpty(doc.Msg, "serviço criado")
	return res
}
