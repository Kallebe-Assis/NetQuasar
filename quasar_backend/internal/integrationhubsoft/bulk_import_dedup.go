// Checagem de duplicidade (cliente por CPF/CNPJ, serviço por login) e fluxo ApplyXxxDedup.

package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Checagem de duplicidade (evita criar cliente/serviço repetido) -----------------------------
//
// Pedido do usuário: antes de criar, confere se o cliente (por CPF/CNPJ) e o serviço (por login) já
// existem na HubSoft, e decide entre 4 casos:
//  1. Cliente não existe -> cria cliente + serviço (fluxo de sempre).
//  2. Cliente existe, nenhum serviço -> cria só o serviço (outro endpoint, cliente já existe).
//  3. Cliente existe, tem serviço com o MESMO login -> não faz nada, avisa que já existe.
//  4. Cliente existe, tem serviço(s) mas nenhum com esse login -> cria um serviço NOVO (cliente fica
//     com 2+ serviços) com o login certo.
// Sem login preenchido na linha não dá pra comparar "mesmo login" — nesse caso sempre cria um serviço
// novo quando o cliente já existe (nunca decide "já existe" sem ter como confirmar).

// ExistingClientService — um serviço já cadastrado do cliente encontrado por findClient.
type ExistingClientService struct {
	IDClienteServico string
	Login            string
	Status           string
	Endereco         ExistingAddress // endereço de instalação do serviço (vazio se a HubSoft não devolveu)
}

// ExistingAddress — endereço de instalação de um serviço já cadastrado.
type ExistingAddress struct {
	CEP, Bairro, Logradouro, Numero string
}

func (a ExistingAddress) empty() bool { return a.CEP == "" && a.Logradouro == "" && a.Numero == "" }

func (a ExistingAddress) String() string {
	return strings.TrimSpace(fmt.Sprintf("%s, %s — %s (CEP %s)", a.Logradouro, a.Numero, a.Bairro, a.CEP))
}

// sameAddress — mesmo endereço quando CEP (dígitos), número e logradouro (sem caixa/acento) batem.
func sameAddress(a ExistingAddress, cep, logradouro, numero string) bool {
	return onlyDigitsBulk(a.CEP) == onlyDigitsBulk(cep) && normText(a.Numero) == normText(numero) && normText(a.Logradouro) == normText(logradouro)
}

// existingAddressesDifferent — true quando o cliente já cadastrado NÃO tem nenhum serviço no endereço da linha
// (só considera serviços cujo endereço a HubSoft devolveu).
func existingAddressesDifferent(ex *ExistingClient, cep, logradouro, numero string) (differs bool, addrs []string) {
	matched := false
	for _, s := range ex.Servicos {
		if s.Endereco.empty() {
			continue
		}
		addrs = append(addrs, s.Endereco.String())
		if sameAddress(s.Endereco, cep, logradouro, numero) {
			matched = true
		}
	}
	return len(addrs) > 0 && !matched, addrs
}

var emailStrictRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(\.[A-Za-z0-9\-]+)+$`)

// validEmailStrict — o formato que a HubSoft aceita na prática: ASCII, sem ponto no fim nem "..".
func validEmailStrict(e string) bool {
	e = strings.TrimSpace(e)
	return len(e) <= 254 && emailStrictRe.MatchString(e) && !strings.Contains(e, "..") && !strings.HasSuffix(e, ".") && !strings.Contains(e, ".@")
}

// PlaceholderPassword é a senha que a mesma máscara da HubSoft dá ao serviço novo (informada pelo operador).
// Um serviço que já tem o login certo mas ainda esta senha ficou sem a troca de senha.
const PlaceholderPassword = "nq12345"

// findLoginHit procura o login em TODA a base da HubSoft (busca=login_radius), não só nos serviços do
// cliente: o login é único na conta, então se ele já existe em qualquer cliente o serviço NÃO pode ser
// criado (a HubSoft o criaria com o login padrão da máscara, deixando um serviço órfão para consertar).
// Devolve nil se o login estiver livre; erro em falha de consulta — nesse caso nada é criado.
func findLoginHit(ctx context.Context, cfg Config, token, login string) (*dvService, error) {
	login = strings.TrimSpace(login)
	if login == "" || strings.EqualFold(login, PlaceholderLogin) {
		return nil, nil
	}
	svcs, err := dvLookup(ctx, cfg, token, "login_radius", login)
	if err != nil {
		return nil, err
	}
	for i := range svcs {
		if strings.EqualFold(strings.TrimSpace(svcs[i].Login), login) {
			return &svcs[i], nil
		}
	}
	return nil, nil
}

// loginInUseResult — linha NÃO cadastrada porque o login já existe na HubSoft. Não é erro de envio
// (nada saiu para criar) nem sucesso: fica com DedupAction própria para o operador conferir.
func loginInUseResult(line int, login string, hit *dvService) ImportApplyResult {
	return ImportApplyResult{
		Line: line, DedupAction: "login_em_uso", IDCliente: hit.ClientID, IDServico: hit.ServiceID,
		Message: fmt.Sprintf("NÃO cadastrado — o login %q já existe na HubSoft (cliente id_cliente %s %s, serviço id_cliente_servico %s, status %s). Nada foi criado; confira esse cadastro",
			login, hit.ClientID, hit.ClientName, hit.ServiceID, hit.Status),
	}
}

// PlaceholderLogin é o login que a automação de login automático da HubSoft dá a todo serviço novo
// criado pela API (informado pelo operador). Um serviço que ainda o tem ficou sem a troca para o login
// definitivo — ver repairPlaceholderService.
const PlaceholderLogin = "netquasar"

// repairPlaceholderService decide o REPARO de login na checagem de duplicidade: o cliente já existe,
// nenhum serviço tem o login do CSV, mas um serviço ainda tem o login padrão — sinal de que uma
// importação anterior criou o serviço e a troca de login falhou. Devolve nil quando não é caso de
// reparo. Com exatamente 1 serviço no padrão, devolve OK + DedupAction "login_a_corrigir" (o chamador
// HTTP então roda ApplyLoginConfig nele, sem criar nada). Com mais de 1, recusa (OK=false): não dá para
// saber qual é o certo e escolher errado trocaria o login de outro serviço — nada é criado nem alterado.
func repairPlaceholderService(existing *ExistingClient, wantLogin string, line int, idCliente string) *ImportApplyResult {
	if existing == nil || wantLogin == "" || strings.EqualFold(wantLogin, PlaceholderLogin) {
		return nil
	}
	var cands []ExistingClientService
	for _, s := range existing.Servicos {
		if strings.EqualFold(s.Login, PlaceholderLogin) {
			cands = append(cands, s)
		}
	}
	switch len(cands) {
	case 0:
		return nil
	case 1:
		return &ImportApplyResult{
			Line: line, OK: true, DedupAction: "login_a_corrigir",
			IDCliente: idCliente, IDServico: cands[0].IDClienteServico,
			Message: fmt.Sprintf("cliente já tem o serviço id_cliente_servico %s (status %s) ainda com o login padrão %q — nada foi criado; o login será corrigido para %q",
				cands[0].IDClienteServico, cands[0].Status, PlaceholderLogin, wantLogin),
		}
	default:
		return &ImportApplyResult{
			Line: line, IDCliente: idCliente,
			Message: fmt.Sprintf("cliente tem %d serviços com o login padrão %q — não dá para saber qual corrigir; nada foi criado nem alterado, corrija manualmente na HubSoft", len(cands), PlaceholderLogin),
		}
	}
}

// DefaultDataNascimento é usada quando o CSV não traz data_nascimento: a HubSoft a exige em todo
// cliente novo ("O campo data_nascimento é obrigatório"). Valor definido pelo operador (01/01/1900).
const DefaultDataNascimento = "1900-01-01"

// applyClientDefaults preenche data_nascimento vazia com DefaultDataNascimento. Devolve true se
// aplicou. Usada na validação E na aplicação, para as duas fases sempre concordarem.
func applyClientDefaults(r ClientImportRow) (ClientImportRow, bool) {
	if strings.TrimSpace(r.DataNascimento) == "" {
		r.DataNascimento = DefaultDataNascimento
		return r, true
	}
	return r, false
}

// ExistingClient — resultado de findClient quando o cliente já existe na conta.
type ExistingClient struct {
	IDCliente string
	Servicos  []ExistingClientService
}

// findClient consulta GET /api/v1/integracao/cliente?busca=<buscaField>&termo_busca=<termo> — usado
// tanto para achar por CPF/CNPJ (fluxo de cliente novo) quanto por id_cliente (fluxo de serviço
// adicional). Devolve (nil, nil) quando não encontra ninguém; erro só em falha de rede/parsing —
// NUNCA finge "não encontrei" por causa de um erro, porque isso poderia levar a criar um duplicado.
func findClient(ctx context.Context, cfg Config, token, buscaField, termo string) (*ExistingClient, error) {
	if termo == "" {
		return nil, fmt.Errorf("termo de busca vazio")
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente",
		QueryParams: paramKVs(map[string]string{
			"busca": buscaField, "termo_busca": termo, "limit": "5", "cancelado": "todos", "inativo": "todos",
			"relacoes": "endereco_instalacao", // sem isto a HubSoft não devolve o endereço do serviço
		}),
	})
	body := ResponseBodyBytes(res)
	if !res.OK {
		if res.StatusCode == 0 {
			return nil, fmt.Errorf("sem resposta da HubSoft ao consultar cliente existente")
		}
		return nil, fmt.Errorf("%s", firstNonEmpty(hubsoftActionMessageWithErrors(body), fmt.Sprintf("HTTP %d", res.StatusCode)))
	}
	var doc struct {
		Status   string `json:"status"`
		Clientes []struct {
			IDCliente json.Number `json:"id_cliente"`
			CPFCNPJ   string      `json:"cpf_cnpj"`
			Servicos  []struct {
				IDClienteServico json.Number `json:"id_cliente_servico"`
				Login            string      `json:"login"`
				Status           string      `json:"status"`
				Endereco         *struct {
					CEP         string `json:"cep"`
					Bairro      string `json:"bairro"`
					Logradouro  string `json:"endereco"`
					Numero      string `json:"numero"`
					Complemento string `json:"complemento"`
				} `json:"endereco_instalacao"`
			} `json:"servicos"`
		} `json:"clientes"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("resposta inesperada ao consultar cliente existente: %w", err)
	}
	for _, c := range doc.Clientes {
		// a busca por cpf_cnpj pode não ser estritamente exata — confirma antes de confiar; para
		// busca por id_cliente, o id já veio de uma consulta anterior, então confirma do mesmo jeito.
		if buscaField == "cpf_cnpj" && onlyDigitsBulk(c.CPFCNPJ) != termo {
			continue
		}
		if buscaField == "id_cliente" && c.IDCliente.String() != termo {
			continue
		}
		ec := &ExistingClient{IDCliente: c.IDCliente.String()}
		for _, s := range c.Servicos {
			es := ExistingClientService{IDClienteServico: s.IDClienteServico.String(), Login: s.Login, Status: s.Status}
			if s.Endereco != nil {
				es.Endereco = ExistingAddress{CEP: s.Endereco.CEP, Bairro: s.Endereco.Bairro, Logradouro: s.Endereco.Logradouro, Numero: s.Endereco.Numero}
			}
			ec.Servicos = append(ec.Servicos, es)
		}
		return ec, nil
	}
	return nil, nil
}

// findServiceByLogin devolve o serviço de existing cujo login bate (case-insensitive) com login, ou
// nil se nenhum bater (ou se login estiver vazio — sem login não dá pra comparar).
func findServiceByLogin(existing *ExistingClient, login string) *ExistingClientService {
	if existing == nil || login == "" {
		return nil
	}
	for i := range existing.Servicos {
		if strings.EqualFold(existing.Servicos[i].Login, login) {
			return &existing.Servicos[i]
		}
	}
	return nil
}

// ApplyClientImportRowDedup — como ApplyClientImportRow, mas confere antes se já existe um cliente
// com esse CPF/CNPJ (ver cabeçalho da seção acima para os 4 casos). Usada pelo handler HTTP no lugar
// de ApplyClientImportRow para o fluxo "Clientes novos".
func ApplyClientImportRowDedup(ctx context.Context, cfg Config, token string, r ClientImportRow) ImportApplyResult {
	r, _ = applyClientDefaults(r)
	if problems := validateClientRowProblems(r, CatalogSets{}); len(problems) > 0 {
		return ImportApplyResult{Line: r.Line, Rejected: true,
			Message: "linha não enviada à HubSoft — falhou na validação: " + strings.Join(problems, " | ")}
	}
	cpf, _ := normalizeCPFCNPJ(r.TipoPessoa, r.CPFCNPJ)
	existing, err := findClient(ctx, cfg, token, "cpf_cnpj", cpf)
	if err != nil {
		return ImportApplyResult{Line: r.Line,
			Message: "não foi possível conferir se o cliente já existe antes de criar — nada foi enviado: " + err.Error()}
	}
	if existing == nil {
		// Cliente não existe (pelo CPF/CNPJ) — mas o login do serviço pode já existir em outro cadastro.
		hit, lerr := findLoginHit(ctx, cfg, token, r.Login)
		if lerr != nil {
			return ImportApplyResult{Line: r.Line,
				Message: "não foi possível conferir se o login já existe antes de criar — nada foi enviado: " + lerr.Error()}
		}
		if hit != nil {
			return loginInUseResult(r.Line, r.Login, hit)
		}
		res := ApplyClientImportRow(ctx, cfg, token, r)
		res.DedupAction = "cliente_criado"
		return res
	}
	if svc := findServiceByLogin(existing, r.Login); svc != nil {
		return ImportApplyResult{
			Line: r.Line, OK: true, DedupAction: "ja_existe",
			IDCliente: existing.IDCliente, IDServico: svc.IDClienteServico,
			Message: fmt.Sprintf("cliente já existe (id_cliente %s) com um serviço usando o mesmo login %q (id_cliente_servico %s, status %s) — nada foi criado",
				existing.IDCliente, r.Login, svc.IDClienteServico, svc.Status),
		}
	}
	hit, lerr := findLoginHit(ctx, cfg, token, r.Login)
	if lerr != nil {
		return ImportApplyResult{Line: r.Line,
			Message: "não foi possível conferir se o login já existe antes de criar — nada foi enviado: " + lerr.Error()}
	}
	if hit != nil {
		return loginInUseResult(r.Line, r.Login, hit)
	}
	if rep := repairPlaceholderService(existing, r.Login, r.Line, existing.IDCliente); rep != nil {
		return *rep
	}
	// O cliente já existe: sem enviar endereco_instalacao a HubSoft herda o endereço do CADASTRO ANTIGO — foi
	// assim que o serviço da ANA SILVIA (Monte Alegre) nasceu no endereço de Santa Teresa. O serviço novo sempre
	// leva o endereço da linha do CSV.
	differs, addrs := existingAddressesDifferent(existing, r.EnderecoCEP, r.EnderecoLogradouro, r.EnderecoNumero)
	svcRow := ServiceImportRow{
		Line: r.Line, IDCliente: existing.IDCliente,
		IDServico: r.IDServico, IDVencimento: r.IDVencimento, IDUsuarioVendedor: r.IDUsuarioVendedor,
		IDFormaCobranca: r.IDFormaCobranca, IDServicoStatus: r.IDServicoStatus, Valor: r.Valor,
		DataVenda: r.DataVenda, Carne: r.Carne, TaxaInstalacaoTipo: r.TaxaInstalacaoTipo,
		Referencia: r.Referencia, Login: r.Login, Senha: r.Senha, IDInterfaceConexao: r.IDInterfaceConexao,
		EnderecoInstalacaoCEP: r.EnderecoCEP, EnderecoInstalacaoBairro: r.EnderecoBairro,
		EnderecoInstalacaoLogradouro: r.EnderecoLogradouro, EnderecoInstalacaoNumero: r.EnderecoNumero,
		EnderecoInstalacaoComplemento: r.EnderecoComplemento,
		EnderecoInstalacaoLatitude:    r.EnderecoLatitude, EnderecoInstalacaoLongitude: r.EnderecoLongitude,
	}
	res := ApplyServiceImportRow(ctx, cfg, token, svcRow)
	res.DedupAction = "servico_adicionado"
	if differs && res.OK {
		res.Message += fmt.Sprintf(" — ATENÇÃO: o cliente já existia com outro endereço (%s); o serviço foi criado no endereço do CSV (%s, %s — %s)",
			strings.Join(addrs, " | "), r.EnderecoLogradouro, r.EnderecoNumero, r.EnderecoBairro)
	}
	if res.IDCliente == "" {
		res.IDCliente = existing.IDCliente // preserva no histórico mesmo sem ter sido criado agora
	}
	return res
}

// ApplyServiceImportRowDedup — como ApplyServiceImportRow, mas confere antes (por id_cliente) se já
// existe um serviço com o mesmo login nesse cliente, pro mesmo motivo do Dedup acima: nunca duplicar
// um serviço que já foi criado, por exemplo se a mesma linha for reenviada por engano.
func ApplyServiceImportRowDedup(ctx context.Context, cfg Config, token string, r ServiceImportRow) ImportApplyResult {
	if problems := validateServiceRowProblems(r, CatalogSets{}); len(problems) > 0 {
		return ImportApplyResult{Line: r.Line, Rejected: true,
			Message: "linha não enviada à HubSoft — falhou na validação: " + strings.Join(problems, " | ")}
	}
	if r.Login != "" {
		existing, err := findClient(ctx, cfg, token, "id_cliente", r.IDCliente)
		if err != nil {
			return ImportApplyResult{Line: r.Line,
				Message: "não foi possível conferir os serviços existentes do cliente antes de criar — nada foi enviado: " + err.Error()}
		}
		if svc := findServiceByLogin(existing, r.Login); svc != nil {
			return ImportApplyResult{
				Line: r.Line, OK: true, DedupAction: "ja_existe",
				IDCliente: r.IDCliente, IDServico: svc.IDClienteServico,
				Message: fmt.Sprintf("cliente já tem um serviço usando o mesmo login %q (id_cliente_servico %s, status %s) — nada foi criado",
					r.Login, svc.IDClienteServico, svc.Status),
			}
		}
		hit, lerr := findLoginHit(ctx, cfg, token, r.Login)
		if lerr != nil {
			return ImportApplyResult{Line: r.Line,
				Message: "não foi possível conferir se o login já existe antes de criar — nada foi enviado: " + lerr.Error()}
		}
		if hit != nil {
			return loginInUseResult(r.Line, r.Login, hit)
		}
		if rep := repairPlaceholderService(existing, r.Login, r.Line, r.IDCliente); rep != nil {
			return *rep
		}
	}
	res := ApplyServiceImportRow(ctx, cfg, token, r)
	res.DedupAction = "servico_adicionado"
	return res
}

// parseIntListStrict — como parseIntList, mas devolve allOK=false se QUALQUER token não for um
// inteiro positivo, em vez de simplesmente ignorar o que não reconhece.
func parseIntListStrict(s string) (out []int, allOK bool) {
	allOK = true
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			allOK = false
			continue
		}
		out = append(out, n)
	}
	return out, allOK
}
