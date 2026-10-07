// Pré-conferência (somente leitura) do que a importação vai fazer em cada linha.

package integrationhubsoft

import (
	"context"
	"fmt"
	"strings"
)

// --- Pré-conferência da importação (somente leitura) ----------------------------------------------------
//
// Antes de importar, diz para cada linha de "clientes novos" o que a importação VAI fazer, consultando a
// HubSoft: criar cliente novo, ou cliente que já existe — e, nesse caso, se o endereço da linha é diferente
// do já cadastrado (o serviço é criado no endereço do CSV, mas o operador precisa saber), se o login já
// existe, etc. Nada é alterado.

const (
	PreNew          = "novo"
	PreExistsSame   = "existe_mesmo_endereco"
	PreExistsOther  = "existe_outro_endereco"
	PreAlreadyDone  = "ja_importado"
	PreLoginTaken   = "login_em_uso"
	PrePlaceholder  = "login_padrao_a_corrigir"
	PreLookupFailed = "erro"
)

type PreflightRow struct {
	Line      int      `json:"line"`
	Label     string   `json:"label"`
	Status    string   `json:"status"`
	Message   string   `json:"message"`
	IDCliente string   `json:"id_cliente,omitempty"`
	CSVAddr   string   `json:"csv_address,omitempty"`
	HubAddrs  []string `json:"hubsoft_addresses,omitempty"`
	Logins    []string `json:"existing_logins,omitempty"`
}

func PreflightClientRow(ctx context.Context, cfg Config, token string, r ClientImportRow) PreflightRow {
	pr := PreflightRow{Line: r.Line, Label: r.NomeRazaoSocial,
		CSVAddr: strings.TrimSpace(fmt.Sprintf("%s, %s — %s (CEP %s)", r.EnderecoLogradouro, r.EnderecoNumero, r.EnderecoBairro, onlyDigitsBulk(r.EnderecoCEP)))}
	cpf, _ := normalizeCPFCNPJ(r.TipoPessoa, r.CPFCNPJ)
	if cpf == "" {
		pr.Status, pr.Message = PreLookupFailed, "linha sem CPF/CNPJ"
		return pr
	}
	existing, err := findClient(ctx, cfg, token, "cpf_cnpj", cpf)
	if err != nil {
		pr.Status, pr.Message = PreLookupFailed, "falha ao consultar a HubSoft: "+err.Error()
		return pr
	}
	if existing == nil {
		hit, lerr := findLoginHit(ctx, cfg, token, r.Login)
		switch {
		case lerr != nil:
			pr.Status, pr.Message = PreLookupFailed, "falha ao conferir o login: "+lerr.Error()
		case hit != nil:
			pr.Status = PreLoginTaken
			pr.Message = fmt.Sprintf("o login %q já existe na HubSoft (cliente %s %s) — a linha NÃO será cadastrada", r.Login, hit.ClientID, hit.ClientName)
			pr.IDCliente = hit.ClientID
		default:
			pr.Status, pr.Message = PreNew, "cliente novo — será criado"
		}
		return pr
	}
	pr.IDCliente = existing.IDCliente
	for _, s := range existing.Servicos {
		if s.Login != "" {
			pr.Logins = append(pr.Logins, s.Login)
		}
	}
	if findServiceByLogin(existing, r.Login) != nil {
		pr.Status, pr.Message = PreAlreadyDone, "este login já está cadastrado neste cliente — nada será criado"
		return pr
	}
	if hit, lerr := findLoginHit(ctx, cfg, token, r.Login); lerr == nil && hit != nil {
		pr.Status = PreLoginTaken
		pr.Message = fmt.Sprintf("o login %q já existe na HubSoft em outro cadastro (cliente %s %s) — a linha NÃO será cadastrada", r.Login, hit.ClientID, hit.ClientName)
		return pr
	}
	if rep := repairPlaceholderService(existing, r.Login, r.Line, existing.IDCliente); rep != nil && rep.OK {
		pr.Status, pr.Message = PrePlaceholder, "cliente existe com um serviço ainda no login padrão — só o login será corrigido"
		return pr
	}
	differs, addrs := existingAddressesDifferent(existing, r.EnderecoCEP, r.EnderecoLogradouro, r.EnderecoNumero)
	pr.HubAddrs = addrs
	if differs {
		pr.Status = PreExistsOther
		pr.Message = "cliente JÁ EXISTE na HubSoft com outro endereço — será adicionado um serviço novo no endereço do CSV; confirme que é esse o endereço do ponto"
	} else {
		pr.Status = PreExistsSame
		pr.Message = "cliente já existe na HubSoft (mesmo endereço) — será adicionado um serviço novo"
	}
	return pr
}
