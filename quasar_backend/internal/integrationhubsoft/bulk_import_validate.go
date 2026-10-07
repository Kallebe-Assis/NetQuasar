// Validação local (sem tocar a HubSoft) das linhas de importação: formato de datas, CPF/CNPJ, e-mail,
// coordenadas, valores e — quando os catálogos são informados — existência de cada ID.

package integrationhubsoft

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func isPosInt(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// dateInputLayouts — a HubSoft só aceita YYYY-MM-DD, mas um CSV que passou pelo Excel volta com a
// data reformatada no padrão local (DD/MM/AAAA é o comum no Brasil) sem avisar ninguém — Excel
// reconhece uma célula "2026-03-15" como data e, ao salvar de novo em CSV, exporta no formato de
// exibição da localidade do Windows, não no formato original do texto. Aceitar esses formatos aqui
// evita que uma reabertura inocente no Excel quebre a importação.
var dateInputLayouts = []string{"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006"}

// normalizeDate tenta cada layout conhecido e devolve a data em YYYY-MM-DD (o único formato que a
// HubSoft aceita), já pronta para ir no corpo do pedido — nunca a string original sem conferir.
func normalizeDate(s string) (canonical string, t time.Time, ok bool) {
	s = strings.TrimSpace(s)
	for _, layout := range dateInputLayouts {
		if parsed, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return parsed.Format("2006-01-02"), parsed, true
		}
	}
	return "", time.Time{}, false
}

// isValidCPF/isValidCNPJ — conferem os dígitos verificadores pelo algoritmo oficial (mesmo usado pela
// Receita Federal), não só o tamanho. Servem de base para normalizeCPFCNPJ: só aceitamos recuperar um
// zero à esquerda perdido quando os dígitos verificadores CONFIRMAM que aquele era o valor original —
// nunca é um palpite.
func isValidCPF(cpf string) bool {
	if len(cpf) != 11 {
		return false
	}
	allSame := true
	for i := 1; i < 11; i++ {
		if cpf[i] != cpf[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}
	d := make([]int, 11)
	for i := 0; i < 11; i++ {
		d[i] = int(cpf[i] - '0')
	}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += d[i] * (10 - i)
	}
	r := sum % 11
	d10 := 0
	if r >= 2 {
		d10 = 11 - r
	}
	if d[9] != d10 {
		return false
	}
	sum = 0
	for i := 0; i < 10; i++ {
		sum += d[i] * (11 - i)
	}
	r = sum % 11
	d11 := 0
	if r >= 2 {
		d11 = 11 - r
	}
	return d[10] == d11
}

func isValidCNPJ(cnpj string) bool {
	if len(cnpj) != 14 {
		return false
	}
	allSame := true
	for i := 1; i < 14; i++ {
		if cnpj[i] != cnpj[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}
	d := make([]int, 14)
	for i := 0; i < 14; i++ {
		d[i] = int(cnpj[i] - '0')
	}
	w1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += d[i] * w1[i]
	}
	r := sum % 11
	d13 := 0
	if r >= 2 {
		d13 = 11 - r
	}
	if d[12] != d13 {
		return false
	}
	w2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		sum += d[i] * w2[i]
	}
	r = sum % 11
	d14 := 0
	if r >= 2 {
		d14 = 11 - r
	}
	return d[13] == d14
}

// normalizeCPFCNPJ recupera zero(s) à esquerda perdidos — o jeito mais comum disso acontecer é abrir
// o CSV no Excel: ele reconhece a célula só-dígitos como número e, ao salvar de novo, descarta os
// zeros à esquerda (mesma classe de problema que já tínhamos com datas, ver normalizeDate). Só aceita
// a recuperação quando, depois de completar com zero(s) à esquerda até o tamanho esperado, o dígito
// verificador bate — nunca "chuta" o zero que falta, só confirma que era mesmo ele. Limitado a no
// máximo 2 zeros recuperados (perder mais que isso não é explicado por esse mecanismo, é outro
// problema de dado que precisa de revisão manual).
func normalizeCPFCNPJ(tipoPessoa, raw string) (canonical string, recovered bool) {
	d := onlyDigitsBulk(raw)
	tp := strings.ToLower(strings.TrimSpace(tipoPessoa))
	var wantLen int
	switch tp {
	case "pf":
		wantLen = 11
	case "pj":
		wantLen = 14
	default:
		// tipo_pessoa desconhecido — não dá pra saber se o alvo é CPF(11) ou CNPJ(14), então não
		// tenta recuperar nada (isso já vira o próprio problema "tipo_pessoa inválido" em outro lugar).
		return d, false
	}
	if len(d) == wantLen || len(d) < wantLen-2 || len(d) >= wantLen {
		return d, false
	}
	padded := strings.Repeat("0", wantLen-len(d)) + d
	if wantLen == 11 && isValidCPF(padded) {
		return padded, true
	}
	if wantLen == 14 && isValidCNPJ(padded) {
		return padded, true
	}
	return d, false
}

// scientificNotationRe detecta um valor que o Excel converteu pra notação científica (ex.: "3,11E+13"
// para um CNPJ de 14 dígitos) — nesse caso o número original já foi perdido na própria planilha, não
// tem zero-à-esquerda pra recuperar; onlyDigitsBulk ingenuamente pegaria "3", "11" e "13" e formaria
// um CPF/CNPJ de mentira ("31113") em vez de rejeitar, por isso essa checagem precisa vir ANTES do
// onlyDigitsBulk de normalizeCPFCNPJ.
var scientificNotationRe = regexp.MustCompile(`^\s*\d[\d.,]*\s*[eE]\s*[+-]?\d+\s*$`)

// normalizeLatLon aceita ponto OU vírgula como separador decimal — o Excel, quando configurado para
// o Brasil, às vezes exporta um decimal simples com vírgula ("-21,4081228"); troca a vírgula por
// ponto antes de validar, já que "-21.4081228" é o único formato que a HubSoft aceita. Isso é
// diferente do caso de separador de milhar perdido ("-214.011.544", esse sim corrompido e
// irrecuperável — sobram 2 pontos mesmo depois da troca, então ParseFloat continua rejeitando): aqui
// é só o separador decimal, uma vírgula a mais nunca "inventa" um dígito que não existia.
func normalizeLatLon(s string, min, max float64) (canonical string, ok bool) {
	s = strings.TrimSpace(s)
	dotted := strings.Replace(s, ",", ".", 1)
	v, err := strconv.ParseFloat(dotted, 64)
	if err != nil || v < min || v > max {
		return s, false
	}
	return dotted, true
}

func latLonOK(s string, min, max float64) bool {
	_, ok := normalizeLatLon(s, min, max)
	return ok
}

func isValidValor(s string) bool {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	v, err := strconv.ParseFloat(s, 64)
	return err == nil && v >= 0
}

var validTaxaInstalacaoTipo = map[string]bool{"nao_cobrar_taxa": true, "gerar_fatura_agora": true, "lancar_proximo_faturamento": true}

// validateLoginFields — login/senha são "tudo ou nada" (um login sem senha, ou vice-versa, deixaria o
// serviço com autenticação pela metade); os limites de tamanho são os documentados pela própria
// HubSoft para POST /cliente/configurar_autenticacao (login 3-255 caracteres, senha min. 3) — conferir
// aqui evita descobrir só na hora de aplicar.
func validateLoginFields(login, senha, idInterfaceConexao string) []string {
	var p []string
	if (login == "") != (senha == "") {
		p = append(p, "login e senha precisam ser preenchidos juntos (ou nenhum dos dois)")
	}
	if login != "" && (len(login) < 3 || len(login) > 255) {
		p = append(p, fmt.Sprintf("login precisa ter entre 3 e 255 caracteres (tem %d)", len(login)))
	}
	if senha != "" && len(senha) < 3 {
		p = append(p, fmt.Sprintf("senha precisa ter pelo menos 3 caracteres (tem %d)", len(senha)))
	}
	if idInterfaceConexao != "" && !isPosInt(idInterfaceConexao) {
		p = append(p, fmt.Sprintf("id_interface_conexao precisa ser um número positivo (veio %q)", idInterfaceConexao))
	}
	return p
}

// checkCatalogID adiciona um problema se o catálogo foi carregado (mapa não-nil) e o ID não existe nele.
func checkCatalogID(p *[]string, label, id string, set map[string]bool) {
	if id == "" || set == nil {
		return
	}
	if !set[id] {
		*p = append(*p, fmt.Sprintf("%s: id %q não existe no catálogo consultado", label, id))
	}
}

// validateClientRowProblems — validação de UMA linha (formato sempre; catálogo quando `cat` traz os
// mapas). Usada tanto pela validação em lote quanto, sem catálogo, como último portão antes de
// ApplyClientImportRow montar e enviar o pedido à HubSoft — ver comentário no topo do ficheiro.
func validateClientRowProblems(r ClientImportRow, cat CatalogSets) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }

	if strings.TrimSpace(r.NomeRazaoSocial) == "" {
		add("nome_razaosocial vazio")
	}
	tp := strings.ToLower(strings.TrimSpace(r.TipoPessoa))
	if tp != "pf" && tp != "pj" {
		add("tipo_pessoa precisa ser pf ou pj (veio %q)", r.TipoPessoa)
	}
	// Antes de medir o tamanho, tenta recuperar zero(s) à esquerda perdidos (ex.: Excel reabrindo o
	// CSV) — só aceita se o dígito verificador confirmar; senão usa o valor como veio.
	cpf, _ := normalizeCPFCNPJ(tp, r.CPFCNPJ)
	switch {
	case cpf == "":
		add("cpf_cnpj vazio")
	case scientificNotationRe.MatchString(r.CPFCNPJ):
		add("cpf_cnpj veio em notação científica (%q) — o Excel converteu o número e os dígitos originais foram perdidos, não dá pra recuperar; reabra o CSV formatando a coluna como Texto antes", r.CPFCNPJ)
	case tp == "pf" && len(cpf) != 11:
		add("cpf_cnpj tem %d dígitos, CPF (pf) precisa ter 11", len(cpf))
	case tp == "pj" && len(cpf) != 14:
		add("cpf_cnpj tem %d dígitos, CNPJ (pj) precisa ter 14", len(cpf))
	case tp != "pf" && tp != "pj" && len(cpf) != 11 && len(cpf) != 14:
		add("cpf_cnpj com %d dígitos (nem CPF nem CNPJ)", len(cpf))
	}
	switch n := len(onlyDigitsBulk(r.TelefonePrimario)); {
	case n == 0:
		add("telefone_primario vazio")
	case n < 10:
		// A HubSoft recusa o cadastro inteiro: "O campo telefone_primario deverá conter no mínimo 10 caracteres".
		add("telefone_primario com %d dígito(s) — a HubSoft exige no mínimo 10 (DDD + número): %q", n, r.TelefonePrimario)
	}
	if r.EmailPrincipal != "" && !validEmailStrict(r.EmailPrincipal) {
		add("email_principal não é um e-mail válido para a HubSoft (só letras sem acento, números e . _ %% + -; sem ponto no fim): %q", r.EmailPrincipal)
	}
	if r.DataNascimento != "" {
		if _, _, ok := normalizeDate(r.DataNascimento); !ok {
			add("data_nascimento inválida (use YYYY-MM-DD ou DD/MM/AAAA): %q", r.DataNascimento)
		}
	}
	if len(onlyDigitsBulk(r.EnderecoCEP)) != 8 {
		add("endereco_cep precisa ter 8 dígitos (veio %q)", r.EnderecoCEP)
	}
	if strings.TrimSpace(r.EnderecoBairro) == "" {
		add("endereco_bairro vazio")
	}
	if strings.TrimSpace(r.EnderecoLogradouro) == "" {
		add("endereco_logradouro vazio")
	}
	if strings.TrimSpace(r.EnderecoNumero) == "" {
		add("endereco_numero vazio")
	}
	if r.EnderecoLatitude != "" && !latLonOK(r.EnderecoLatitude, -90, 90) {
		add("endereco_latitude inválida (veio %q) — confira se não foi corrompida ao abrir o CSV no Excel (ex.: o ponto decimal virou separador de milhar)", r.EnderecoLatitude)
	}
	if r.EnderecoLongitude != "" && !latLonOK(r.EnderecoLongitude, -180, 180) {
		add("endereco_longitude inválida (veio %q) — confira se não foi corrompida ao abrir o CSV no Excel (ex.: o ponto decimal virou separador de milhar)", r.EnderecoLongitude)
	}
	if !isPosInt(r.IDServico) {
		add("id_servico precisa ser um número positivo (veio %q)", r.IDServico)
	} else {
		checkCatalogID(&p, "id_servico", r.IDServico, cat.Servico)
	}
	if !isPosInt(r.IDVencimento) {
		add("id_vencimento precisa ser um número positivo (veio %q)", r.IDVencimento)
	} else {
		checkCatalogID(&p, "id_vencimento", r.IDVencimento, cat.Vencimento)
	}
	if !isPosInt(r.IDUsuarioVendedor) {
		add("id_usuario_vendedor precisa ser um número positivo (veio %q)", r.IDUsuarioVendedor)
	} else {
		checkCatalogID(&p, "id_usuario_vendedor", r.IDUsuarioVendedor, cat.Vendedor)
	}
	if !isPosInt(r.IDFormaCobranca) {
		add("id_forma_cobranca precisa ser um número positivo (veio %q)", r.IDFormaCobranca)
	} else {
		checkCatalogID(&p, "id_forma_cobranca", r.IDFormaCobranca, cat.FormaCobranca)
	}
	if !isPosInt(r.IDServicoStatus) {
		add("id_servico_status precisa ser um número positivo (veio %q)", r.IDServicoStatus)
	} else {
		checkCatalogID(&p, "id_servico_status", r.IDServicoStatus, cat.ServicoStatus)
	}
	for _, g := range strings.Split(r.IDsGruposCliente, ",") {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if !isPosInt(g) {
			add("ids_grupos_cliente com valor inválido: %q", g)
		} else {
			checkCatalogID(&p, "ids_grupos_cliente", g, cat.GrupoCliente)
		}
	}
	if !isValidValor(r.Valor) {
		add("valor inválido (veio %q)", r.Valor)
	}
	if _, dv, ok := normalizeDate(r.DataVenda); !ok {
		add("data_venda inválida (use YYYY-MM-DD ou DD/MM/AAAA): %q", r.DataVenda)
	} else if dv.After(time.Now()) {
		add("data_venda no futuro: %q", r.DataVenda)
	}
	carne := strings.ToLower(strings.TrimSpace(r.Carne))
	if carne != "true" && carne != "false" {
		add("carne precisa ser true ou false (veio %q)", r.Carne)
	}
	if !validTaxaInstalacaoTipo[strings.TrimSpace(r.TaxaInstalacaoTipo)] {
		add("taxa_instalacao_tipo inválido (veio %q) — use nao_cobrar_taxa, gerar_fatura_agora ou lancar_proximo_faturamento", r.TaxaInstalacaoTipo)
	}
	p = append(p, validateLoginFields(r.Login, r.Senha, r.IDInterfaceConexao)...)
	return p
}

// validateServiceRowProblems — mesma ideia de validateClientRowProblems, para POST /cliente/cliente_servico.
func validateServiceRowProblems(r ServiceImportRow, cat CatalogSets) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }

	if !isPosInt(r.IDCliente) {
		add("id_cliente precisa ser um número positivo (veio %q) — preencha com o id_cliente criado na importação principal", r.IDCliente)
	}
	if !isPosInt(r.IDServico) {
		add("id_servico precisa ser um número positivo (veio %q)", r.IDServico)
	} else {
		checkCatalogID(&p, "id_servico", r.IDServico, cat.Servico)
	}
	if !isPosInt(r.IDVencimento) {
		add("id_vencimento precisa ser um número positivo (veio %q)", r.IDVencimento)
	} else {
		checkCatalogID(&p, "id_vencimento", r.IDVencimento, cat.Vencimento)
	}
	if !isPosInt(r.IDUsuarioVendedor) {
		add("id_usuario_vendedor precisa ser um número positivo (veio %q)", r.IDUsuarioVendedor)
	} else {
		checkCatalogID(&p, "id_usuario_vendedor", r.IDUsuarioVendedor, cat.Vendedor)
	}
	if !isPosInt(r.IDFormaCobranca) {
		add("id_forma_cobranca precisa ser um número positivo (veio %q)", r.IDFormaCobranca)
	} else {
		checkCatalogID(&p, "id_forma_cobranca", r.IDFormaCobranca, cat.FormaCobranca)
	}
	if !isPosInt(r.IDServicoStatus) {
		add("id_servico_status precisa ser um número positivo (veio %q)", r.IDServicoStatus)
	} else {
		checkCatalogID(&p, "id_servico_status", r.IDServicoStatus, cat.ServicoStatus)
	}
	if !isValidValor(r.Valor) {
		add("valor inválido (veio %q)", r.Valor)
	}
	if _, dv, ok := normalizeDate(r.DataVenda); !ok {
		add("data_venda inválida (use YYYY-MM-DD ou DD/MM/AAAA): %q", r.DataVenda)
	} else if dv.After(time.Now()) {
		add("data_venda no futuro: %q", r.DataVenda)
	}
	carne := strings.ToLower(strings.TrimSpace(r.Carne))
	if carne != "true" && carne != "false" {
		add("carne precisa ser true ou false (veio %q)", r.Carne)
	}
	if !validTaxaInstalacaoTipo[strings.TrimSpace(r.TaxaInstalacaoTipo)] {
		add("taxa_instalacao_tipo inválido (veio %q)", r.TaxaInstalacaoTipo)
	}
	// endereco_instalacao é opcional, mas se algum campo dele veio preenchido, todos os obrigatórios
	// do bloco também precisam vir — senão ApplyServiceImportRow ficaria com a escolha errada de
	// enviar um endereço incompleto ou de descartar em silêncio o que foi preenchido.
	anyInst := r.EnderecoInstalacaoCEP != "" || r.EnderecoInstalacaoBairro != "" ||
		r.EnderecoInstalacaoLogradouro != "" || r.EnderecoInstalacaoNumero != ""
	if anyInst {
		if len(onlyDigitsBulk(r.EnderecoInstalacaoCEP)) != 8 {
			add("endereco_instalacao_cep precisa ter 8 dígitos (veio %q)", r.EnderecoInstalacaoCEP)
		}
		if strings.TrimSpace(r.EnderecoInstalacaoBairro) == "" {
			add("endereco_instalacao_bairro vazio (endereço de instalação parcialmente preenchido)")
		}
		if strings.TrimSpace(r.EnderecoInstalacaoLogradouro) == "" {
			add("endereco_instalacao_logradouro vazio (endereço de instalação parcialmente preenchido)")
		}
		if strings.TrimSpace(r.EnderecoInstalacaoNumero) == "" {
			add("endereco_instalacao_numero vazio (endereço de instalação parcialmente preenchido)")
		}
	}
	if r.EnderecoInstalacaoLatitude != "" && !latLonOK(r.EnderecoInstalacaoLatitude, -90, 90) {
		add("endereco_instalacao_latitude inválida (veio %q) — confira se não foi corrompida ao abrir o CSV no Excel (ex.: o ponto decimal virou separador de milhar)", r.EnderecoInstalacaoLatitude)
	}
	if r.EnderecoInstalacaoLongitude != "" && !latLonOK(r.EnderecoInstalacaoLongitude, -180, 180) {
		add("endereco_instalacao_longitude inválida (veio %q) — confira se não foi corrompida ao abrir o CSV no Excel (ex.: o ponto decimal virou separador de milhar)", r.EnderecoInstalacaoLongitude)
	}
	p = append(p, validateLoginFields(r.Login, r.Senha, r.IDInterfaceConexao)...)
	return p
}

// ValidateClientImportRows confere cada linha (formato + IDs de catálogo, quando fornecidos). Não
// toca a HubSoft — os catálogos já vêm prontos (ver hubsoftCatalogFetch), para não repetir a mesma
// consulta a cada validação.
func ValidateClientImportRows(rows []ClientImportRow, cat CatalogSets) ImportValidationResult {
	out := ImportValidationResult{OK: true, Rows: make([]ImportRowValidation, 0, len(rows))}
	seenCPF := map[string]int{}
	for _, r := range rows {
		r, defaultedNasc := applyClientDefaults(r)
		p := validateClientRowProblems(r, cat)
		cpf, _ := normalizeCPFCNPJ(r.TipoPessoa, r.CPFCNPJ)
		if cpf != "" {
			if prev, dup := seenCPF[cpf]; dup {
				p = append(p, fmt.Sprintf("cpf_cnpj repetido nesta mesma leva (também na linha %d) — a HubSoft recusa CPF/CNPJ já cadastrado", prev))
			}
			seenCPF[cpf] = r.Line
		}
		rv := ImportRowValidation{Line: r.Line, Valid: len(p) == 0, Problems: p, Label: r.NomeRazaoSocial}
		if defaultedNasc {
			rv.Info = append(rv.Info, "data_nascimento vazia — será usado 01/01/1900")
		} else if isUnderage(r.DataNascimento, time.Now()) {
			rv.Info = append(rv.Info, "titular com menos de 18 anos — se a HubSoft recusar a data, será usado 01/01/1990")
		}
		if rv.Valid {
			out.Valid++
		} else {
			out.Invalid++
		}
		out.Rows = append(out.Rows, rv)
	}
	return out
}

// ValidateServiceImportRows — mesma lógica para POST /cliente/cliente_servico (cliente já existe).
func ValidateServiceImportRows(rows []ServiceImportRow, cat CatalogSets) ImportValidationResult {
	out := ImportValidationResult{OK: true, Rows: make([]ImportRowValidation, 0, len(rows))}
	for _, r := range rows {
		p := validateServiceRowProblems(r, cat)
		rv := ImportRowValidation{Line: r.Line, Valid: len(p) == 0, Problems: p, Label: "id_cliente " + r.IDCliente}
		if rv.Valid {
			out.Valid++
		} else {
			out.Invalid++
		}
		out.Rows = append(out.Rows, rv)
	}
	return out
}

func onlyDigitsBulk(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func floatSafe(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(strings.Replace(s, ",", ".", 1)), 64)
	return v
}
