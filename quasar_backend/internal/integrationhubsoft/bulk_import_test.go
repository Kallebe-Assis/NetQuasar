package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCatalogSetsFromJSON_realResponseShapes usa o corpo EXATO que a HubSoft devolve (documentação +
// export real de conta) para os 6 catálogos usados na validação referencial. Existe por causa de um
// bug real: "grupo_cliente" tinha o nome da lista errado ("grupos_cliente" em vez de "grupo_cliente",
// no singular) e isso rejeitava silenciosamente TODO ID válido, em vez de avisar que não deu para
// conferir. Cobre os 6 para não deixar o mesmo tipo de engano passar despercebido noutro campo.
func TestCatalogSetsFromJSON_realResponseShapes(t *testing.T) {
	cases := []struct {
		which, body, mustHaveID string
	}{
		{"servico", `{"status":"success","servicos":[{"id_servico":2792,"descricao":"10 MB"},{"id_servico":16,"descricao":"500 MB"}]}`, "16"},
		{"servico_status", `{"status":"success","servico_status":[{"id_servico_status":11,"descricao":"Serviço Habilitado","prefixo":"servico_habilitado"}]}`, "11"},
		{"vendedor", `{"status":"success","vendedores":[{"id":86,"name":"Kallebe Assis"}]}`, "86"},
		{"forma_cobranca", `{"status":"success","formas_cobranca":[{"id_forma_cobranca":10,"descricao":"MENSAL - RECEBIMENTO EM MIRACEMA","tipo_cobranca":"boleto_bancario"}]}`, "10"},
		{"vencimento", `{"status":"success","vencimentos":[{"id_vencimento":7,"dia_vencimento":25}]}`, "7"},
		{"grupo_cliente", `{"status":"success","grupo_cliente":[{"id_grupo_cliente":11,"descricao":"NF 62","ativo":true}]}`, "11"},
	}
	for _, c := range cases {
		set := CatalogSetsFromJSON(c.which, []byte(c.body))
		if set == nil {
			t.Errorf("%s: catálogo real não deveria dar unchecked (nil)", c.which)
			continue
		}
		if !set[c.mustHaveID] {
			t.Errorf("%s: id %q devia constar do catálogo lido de %s", c.which, c.mustHaveID, c.body)
		}
	}
}

// TestCatalogSetsFromJSON_unknownShapeIsUnchecked confere o outro lado: uma resposta cuja chave de
// lista não bate com o esperado tem de virar nil (= "não verificado"), nunca um mapa vazio (que faria
// checkCatalogID rejeitar QUALQUER id, mesmo os corretos).
func TestCatalogSetsFromJSON_unknownShapeIsUnchecked(t *testing.T) {
	set := CatalogSetsFromJSON("grupo_cliente", []byte(`{"status":"success","grupos_cliente":[{"id_grupo_cliente":11}]}`))
	if set != nil {
		t.Fatalf("chave de lista errada devia virar nil (não verificado), veio %v", set)
	}
}

func TestValidateClientImportRows(t *testing.T) {
	cat := CatalogSets{
		Servico:       map[string]bool{"21": true},
		Vencimento:    map[string]bool{"4": true},
		Vendedor:      map[string]bool{"86": true},
		FormaCobranca: map[string]bool{"10": true},
		ServicoStatus: map[string]bool{"1": true},
		GrupoCliente:  map[string]bool{"11": true},
	}
	rows := []ClientImportRow{
		{ // válida
			Line: 2, NomeRazaoSocial: "João da Silva", TipoPessoa: "pf", CPFCNPJ: "45455937715",
			TelefonePrimario: "22998136491", EnderecoCEP: "28460-000", EnderecoBairro: "Centro",
			EnderecoLogradouro: "Rua A", EnderecoNumero: "10", IDsGruposCliente: "11",
			IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
			IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false",
			TaxaInstalacaoTipo: "nao_cobrar_taxa",
		},
		{ // cpf inválido (10 dígitos), id_servico não existe no catálogo, tipo_pessoa errado
			Line: 3, NomeRazaoSocial: "Maria", TipoPessoa: "x", CPFCNPJ: "1234567890",
			EnderecoCEP: "123", EnderecoBairro: "", EnderecoLogradouro: "", EnderecoNumero: "",
			IDServico: "999", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
			IDServicoStatus: "1", Valor: "-5", DataVenda: "2099-01-01", Carne: "talvez",
			TaxaInstalacaoTipo: "outra_coisa",
		},
		{ // mesmo CPF da linha 2 — deve acusar duplicidade
			Line: 4, NomeRazaoSocial: "João da Silva 2", TipoPessoa: "pf", CPFCNPJ: "454.559.377-15",
			TelefonePrimario: "22998136491", EnderecoCEP: "28460-000", EnderecoBairro: "Centro",
			EnderecoLogradouro: "Rua A", EnderecoNumero: "10",
			IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
			IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false",
			TaxaInstalacaoTipo: "nao_cobrar_taxa",
		},
	}
	out := ValidateClientImportRows(rows, cat)
	if out.Valid != 1 || out.Invalid != 2 {
		t.Fatalf("esperava 1 válida / 2 inválidas, veio %d/%d: %+v", out.Valid, out.Invalid, out.Rows)
	}
	if !out.Rows[0].Valid {
		t.Fatalf("linha 2 deveria ser válida: %v", out.Rows[0].Problems)
	}
	if out.Rows[1].Valid {
		t.Fatalf("linha 3 deveria ser inválida")
	}
	joined := strings.Join(out.Rows[1].Problems, " | ")
	for _, want := range []string{"tipo_pessoa", "cpf_cnpj", "id_servico", "valor", "data_venda", "carne", "taxa_instalacao_tipo", "endereco_bairro"} {
		if !strings.Contains(joined, want) {
			t.Errorf("esperava menção a %q nos problemas da linha 3: %s", want, joined)
		}
	}
	if !strings.Contains(strings.Join(out.Rows[2].Problems, " "), "repetido") {
		t.Fatalf("linha 4 (CPF repetido) deveria acusar duplicidade: %v", out.Rows[2].Problems)
	}
}

func TestValidateServiceImportRows_addressAllOrNothing(t *testing.T) {
	rows := []ServiceImportRow{
		{
			Line: 2, IDCliente: "100", IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86",
			IDFormaCobranca: "10", IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15",
			Carne: "false", TaxaInstalacaoTipo: "nao_cobrar_taxa",
			EnderecoInstalacaoCEP: "28460000", // só o CEP, sem bairro/logradouro/número
		},
	}
	out := ValidateServiceImportRows(rows, CatalogSets{})
	if out.Valid != 0 || out.Invalid != 1 {
		t.Fatalf("endereço de instalação parcial deveria ser inválido: %+v", out.Rows)
	}
}

func TestApplyClientImportRow_roundTrip(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "msg": "Cliente cadastrado com sucesso.",
			"cliente": map[string]any{"id_cliente": 4321},
		})
	}))
	defer srv.Close()

	row := ClientImportRow{
		Line: 2, NomeRazaoSocial: "João da Silva", TipoPessoa: "pf", CPFCNPJ: "454.559.377-15",
		TelefonePrimario: "(22) 99813-6491", EnderecoCEP: "28460-000", EnderecoBairro: "Centro",
		EnderecoLogradouro: "Rua A", EnderecoNumero: "10", IDsGruposCliente: "11",
		IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
		IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false",
		TaxaInstalacaoTipo: "nao_cobrar_taxa",
	}
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK || res.IDCliente != "4321" {
		t.Fatalf("esperava sucesso com id_cliente=4321: %+v", res)
	}
	if gotBody["cpf_cnpj"] != "45455937715" {
		t.Errorf("cpf_cnpj deveria ir só com dígitos: %v", gotBody["cpf_cnpj"])
	}
	if gotBody["carne"] != false {
		t.Errorf("carne deveria ser bool false: %v (%T)", gotBody["carne"], gotBody["carne"])
	}
	grupos, _ := gotBody["ids_grupos_cliente"].([]any)
	if len(grupos) != 1 || grupos[0] != float64(11) {
		t.Errorf("ids_grupos_cliente deveria ser [11]: %v", gotBody["ids_grupos_cliente"])
	}
	endereco, _ := gotBody["endereco"].(map[string]any)
	if endereco["cep"] != "28460000" {
		t.Errorf("endereco.cep deveria ser só dígitos: %v", endereco["cep"])
	}
}

func TestApplyClientImportRow_inconsistentResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "CPF já cadastrado"})
	}))
	defer srv.Close()
	row := ClientImportRow{
		Line: 2, NomeRazaoSocial: "X", TipoPessoa: "pf", CPFCNPJ: "45455937715",
		TelefonePrimario: "22998136491", EnderecoCEP: "28460000", EnderecoBairro: "Centro",
		EnderecoLogradouro: "Rua A", EnderecoNumero: "10",
		IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
		IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false",
		TaxaInstalacaoTipo: "nao_cobrar_taxa",
	}
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if res.OK || res.Rejected {
		t.Fatalf("esperava falha vinda da HubSoft (não rejeição local): %+v", res)
	}
	if !strings.Contains(res.Message, "CPF") {
		t.Fatalf("mensagem deveria repassar o motivo da HubSoft: %q", res.Message)
	}
}

// TestApplyClientImportRow_surfacesErrorsArrayOnHTTP200 reproduz um bug real: a HubSoft devolve HTTP
// 200 mesmo para erro de validação ("status":"error" no corpo, não um HTTP de erro) — esse caminho do
// código usava só doc.Msg direto, sem passar por hubsoftActionMessageWithErrors, então o array
// "errors" (o motivo específico, ex. "O campo data_nascimento é obrigatório.") se perdia e só a
// mensagem genérica "Favor preencher os campos obrigatórios..." chegava ao operador.
func TestApplyClientImportRow_surfacesErrorsArrayOnHTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "error", "msg": "Favor preencher os campos obrigatórios de acordo com as especificações",
			"errors": []string{"O campo data_nascimento é obrigatório."},
		})
	}))
	defer srv.Close()
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", validClientRow())
	if res.OK {
		t.Fatalf("esperava falha: %+v", res)
	}
	if !strings.Contains(res.Message, "data_nascimento") {
		t.Fatalf("mensagem deveria incluir o detalhe de errors[], veio só: %q", res.Message)
	}
}

func TestApplyClientImportRow_rejectsIncompleteRowWithoutCallingHubsoft(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(500)
	}))
	defer srv.Close()

	// Falta telefone_primario, endereço e todos os IDs — uma linha assim nunca pode virar um
	// pedido HTTP (o corpo teria id_servico:0, valor:0.0 etc., parecendo dado real).
	row := ClientImportRow{Line: 2, NomeRazaoSocial: "Incompleta", TipoPessoa: "pf", CPFCNPJ: "45455937715"}
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.Rejected || res.OK {
		t.Fatalf("esperava rejeição local: %+v", res)
	}
	if called {
		t.Fatalf("não podia ter chamado a HubSoft para uma linha inválida")
	}
}

// TestApplyClientImportRow_toleratesExcelReformattedDates reproduz o caso real: um CSV gerado em
// YYYY-MM-DD que passou pelo Excel volta com data_venda/data_nascimento em DD/MM/AAAA (o Excel
// reconhece a célula como data e reexporta no formato da localidade, sem avisar). A importação
// precisa aceitar isso — mas o que chega na HubSoft tem de ser sempre o formato canônico.
func TestApplyClientImportRow_toleratesExcelReformattedDates(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "msg": "ok", "cliente": map[string]any{"id_cliente": 171},
		})
	}))
	defer srv.Close()

	row := ClientImportRow{
		Line: 2, NomeRazaoSocial: "Antonio Roberto Venancio", TipoPessoa: "pf", CPFCNPJ: "45455937715",
		TelefonePrimario: "22998136491", DataNascimento: "25/08/1971", // DD/MM/AAAA, reformatado pelo Excel
		EnderecoCEP: "28460000", EnderecoBairro: "Rodagem", EnderecoLogradouro: "Rua A", EnderecoNumero: "30",
		IDServico: "16", IDVencimento: "7", IDUsuarioVendedor: "86", IDFormaCobranca: "10", IDServicoStatus: "11",
		Valor: "89.9", DataVenda: "04/01/2019", Carne: "false", TaxaInstalacaoTipo: "nao_cobrar_taxa",
	}
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if res.Rejected || !res.OK {
		t.Fatalf("data em DD/MM/AAAA devia ser aceite: %+v", res)
	}
	if gotBody["data_venda"] != "2019-01-04" {
		t.Errorf("data_venda enviada à HubSoft devia estar normalizada para YYYY-MM-DD, veio %v", gotBody["data_venda"])
	}
	if gotBody["data_nascimento"] != "1971-08-25" {
		t.Errorf("data_nascimento enviada à HubSoft devia estar normalizada para YYYY-MM-DD, veio %v", gotBody["data_nascimento"])
	}
}

func TestApplyClientImportRow_rejectsPartialGrupoClienteList(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(500)
	}))
	defer srv.Close()
	row := ClientImportRow{
		Line: 2, NomeRazaoSocial: "X", TipoPessoa: "pf", CPFCNPJ: "45455937715",
		TelefonePrimario: "22998136491", EnderecoCEP: "28460000", EnderecoBairro: "Centro",
		EnderecoLogradouro: "Rua A", EnderecoNumero: "10", IDsGruposCliente: "11,abc",
		IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
		IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false",
		TaxaInstalacaoTipo: "nao_cobrar_taxa",
	}
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.Rejected || res.OK {
		t.Fatalf("esperava rejeição local (lista de grupos parcialmente numérica): %+v", res)
	}
	if called {
		t.Fatalf("não podia ter chamado a HubSoft com uma lista de grupos incompleta")
	}
}

func validClientRow() ClientImportRow {
	return ClientImportRow{
		Line: 2, NomeRazaoSocial: "X", TipoPessoa: "pf", CPFCNPJ: "45455937715",
		TelefonePrimario: "22998136491", EnderecoCEP: "28460000", EnderecoBairro: "Centro",
		EnderecoLogradouro: "Rua A", EnderecoNumero: "10",
		IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
		IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false",
		TaxaInstalacaoTipo: "nao_cobrar_taxa",
	}
}

// TestApplyClientImportRow_referenciaAndIDClienteServico confere dois pontos novos ao mesmo tempo:
// 1) "referencia" só vai no corpo quando preenchida; 2) id_cliente_servico (necessário para
// encadear ApplyLoginConfig) é extraído de cliente.servicos[0], exatamente como a HubSoft devolve —
// antes dessa mudança esse campo nem era lido da resposta.
func TestApplyClientImportRow_referenciaAndIDClienteServico(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "msg": "Cliente cadastrado com sucesso.",
			"cliente": map[string]any{
				"id_cliente": 4321,
				"servicos":   []map[string]any{{"id_cliente_servico": 9988}},
			},
		})
	}))
	defer srv.Close()

	row := validClientRow()
	row.Referencia = "PEDIDO-0001"
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK || res.IDServico != "9988" {
		t.Fatalf("esperava id_cliente_servico=9988 extraído de cliente.servicos[0]: %+v", res)
	}
	if gotBody["referencia"] != "PEDIDO-0001" {
		t.Errorf("referencia deveria ir no corpo: %v", gotBody["referencia"])
	}

	// sem Referencia preenchida, a chave nem deve aparecer no corpo (nunca manda "" como se fosse dado real)
	gotBody = nil
	row2 := validClientRow()
	res2 := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row2)
	if !res2.OK {
		t.Fatalf("esperava sucesso: %+v", res2)
	}
	if _, present := gotBody["referencia"]; present {
		t.Errorf("sem referencia preenchida, a chave não devia ir no corpo: %v", gotBody["referencia"])
	}
}

func TestValidateLoginFields(t *testing.T) {
	cases := []struct {
		name, login, senha, iface string
		wantProblems              bool
	}{
		{"tudo vazio, ok", "", "", "", false},
		{"login e senha completos, ok", "joao123", "s3nh4", "", false},
		{"login e senha com interface, ok", "joao123", "s3nh4", "2", false},
		{"só login, sem senha", "joao123", "", "", true},
		{"só senha, sem login", "", "s3nh4", "", true},
		{"login curto demais", "jo", "s3nh4", "", true},
		{"senha curta demais", "joao123", "se", "", true},
		{"interface não numérica", "joao123", "s3nh4", "abc", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateLoginFields(c.login, c.senha, c.iface)
			if (len(got) > 0) != c.wantProblems {
				t.Errorf("validateLoginFields(%q,%q,%q) = %v, wantProblems=%v", c.login, c.senha, c.iface, got, c.wantProblems)
			}
		})
	}
}

// TestApplyLoginConfig_roundTrip confere o formato exato do corpo enviado a
// /cliente/configurar_autenticacao e que o sucesso/erro da HubSoft é repassado corretamente.
func TestApplyLoginConfig_roundTrip(t *testing.T) {
	var gotBody map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" { // conferência do login após a troca — sem o serviço na resposta, é ignorada
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{}})
			return
		}
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Parâmetros de autenticação modificados com sucesso."})
	}))
	defer srv.Close()

	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "9988", "2", "joao123", "s3nh4")
	if !res.OK {
		t.Fatalf("esperava sucesso: %+v", res)
	}
	if gotPath != "/api/v1/integracao/cliente/configurar_autenticacao" {
		t.Errorf("rota errada: %s", gotPath)
	}
	if gotBody["id_cliente_servico"] != float64(9988) || gotBody["id_interface_conexao"] != float64(2) {
		t.Errorf("id_cliente_servico/id_interface_conexao não bateram: %v", gotBody)
	}
	if gotBody["login"] != "joao123" || gotBody["password"] != "s3nh4" {
		t.Errorf("login/password não bateram (campo da HubSoft é \"password\", não \"senha\"): %v", gotBody)
	}
}

// TestNormalizeCPFCNPJ_recoversLeadingZero reproduz o bug real relatado: abrir o CSV no Excel faz ele
// reconhecer a célula só-dígitos como número e descartar o zero à esquerda na hora de salvar de novo
// (mesma classe de problema que data_venda/data_nascimento já tinham). CPFs/CNPJs reais da base
// (com zero à esquerda íntegro) confirmam o algoritmo do dígito verificador antes de confiar nele.
func TestNormalizeCPFCNPJ_recoversLeadingZero(t *testing.T) {
	cases := []struct {
		tipo, raw, wantCanonical string
		wantRecovered            bool
	}{
		{"pf", "01772491705", "01772491705", false}, // já completo, não precisa recuperar
		{"pf", "1772491705", "01772491705", true},   // 10 dígitos — 1 zero perdido, recupera
		{"pj", "00771989000137", "00771989000137", false},
		{"pj", "771989000137", "00771989000137", true},  // 12 dígitos — 1 zero perdido
		{"pj", "0771989000137", "00771989000137", true}, // 13 dígitos — ainda tenta (dentro da janela de 2)
		{"pf", "4545593771", "", false},                 // 10 dígitos mas dígito verificador NÃO bate após completar — não inventa
	}
	for _, c := range cases {
		got, recovered := normalizeCPFCNPJ(c.tipo, c.raw)
		wantGot := c.wantCanonical
		if wantGot == "" {
			wantGot = onlyDigitsBulk(c.raw) // sem recuperação válida, devolve como veio
		}
		if got != wantGot || recovered != c.wantRecovered {
			t.Errorf("normalizeCPFCNPJ(%q,%q) = (%q,%v), esperava (%q,%v)", c.tipo, c.raw, got, recovered, wantGot, c.wantRecovered)
		}
	}
}

// TestApplyClientImportRow_recoversCPFLeadingZero confere ponta a ponta: uma linha com CPF de 10
// dígitos (zero perdido) não é rejeitada, e o valor enviado de verdade à HubSoft já vem com o zero
// recuperado — nunca o valor cru de 10 dígitos, que a HubSoft recusaria como CPF inválido.
func TestApplyClientImportRow_recoversCPFLeadingZero(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "msg": "Cliente cadastrado com sucesso.",
			"cliente": map[string]any{"id_cliente": 1},
		})
	}))
	defer srv.Close()

	row := validClientRow()
	row.CPFCNPJ = "1772491705" // real, só sem o zero à esquerda (como o Excel devolveria)
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK {
		t.Fatalf("esperava sucesso (CPF recuperável): %+v", res)
	}
	if gotBody["cpf_cnpj"] != "01772491705" {
		t.Errorf("cpf_cnpj enviado deveria ter o zero recuperado: %v", gotBody["cpf_cnpj"])
	}
}

// TestValidateClientRowProblems_scientificNotationCPFCNPJ reproduz um caso real do usuário: o Excel
// converteu um CNPJ de 14 dígitos para notação científica ("3,11E+13") antes do CSV ser salvo de
// novo. Sem essa checagem, onlyDigitsBulk extraía só os dígitos literais do texto ("3","11","13" →
// "31113", 5 dígitos) e a mensagem de erro ("tem 5 dígitos") escondia a causa real.
func TestValidateClientRowProblems_scientificNotationCPFCNPJ(t *testing.T) {
	cases := []string{"3,11E+13", "3.11E+13", "1E+10", "4,5e10"}
	for _, raw := range cases {
		row := validClientRow()
		row.TipoPessoa = "pj"
		row.CPFCNPJ = raw
		problems := validateClientRowProblems(row, CatalogSets{})
		joined := strings.Join(problems, " | ")
		if !strings.Contains(joined, "notação científica") {
			t.Errorf("CPFCNPJ=%q: esperava mensagem sobre notação científica, veio: %s", raw, joined)
		}
	}
}

// TestValidateClientRowProblems_corruptedLatLon reproduz outro caso real do usuário: o Excel trata o
// ponto decimal como separador de milhar, então "-21.4011544" vira "-214.011.544" — magnitude bem
// fora do intervalo válido de latitude/longitude. ParseFloat já rejeita (2 pontos não é float válido).
func TestValidateClientRowProblems_corruptedLatLon(t *testing.T) {
	row := validClientRow()
	row.EnderecoLatitude = "-214.011.544"
	row.EnderecoLongitude = "-421.835.798"
	problems := validateClientRowProblems(row, CatalogSets{})
	joined := strings.Join(problems, " | ")
	if !strings.Contains(joined, "endereco_latitude") || !strings.Contains(joined, "endereco_longitude") {
		t.Fatalf("esperava reclamar de latitude E longitude corrompidas: %s", joined)
	}
	// um valor legítimo não deve disparar nada
	row2 := validClientRow()
	row2.EnderecoLatitude = "-21.4011544"
	row2.EnderecoLongitude = "-42.1835798"
	if p := validateClientRowProblems(row2, CatalogSets{}); len(p) != 0 {
		t.Fatalf("lat/lon válidas não deveriam gerar problema: %v", p)
	}
}

// TestNormalizeLatLon_commaDecimal reproduz outro caso real: o Excel configurado para o Brasil às
// vezes exporta um decimal simples com vírgula ("-21,4081228") em vez de ponto — diferente do caso de
// separador de milhar perdido (que tem múltiplos pontos e perde dígitos de verdade), aqui é só o
// separador decimal, então convertido com segurança, nunca rejeitado.
func TestNormalizeLatLon_commaDecimal(t *testing.T) {
	lat, ok := normalizeLatLon("-21,4081228", -90, 90)
	if !ok || lat != "-21.4081228" {
		t.Fatalf("esperava recuperar -21.4081228, veio (%q, %v)", lat, ok)
	}
	lon, ok := normalizeLatLon("-42,1888707", -180, 180)
	if !ok || lon != "-42.1888707" {
		t.Fatalf("esperava recuperar -42.1888707, veio (%q, %v)", lon, ok)
	}
	// vírgula decimal não deve gerar problema de validação
	row := validClientRow()
	row.EnderecoLatitude = "-21,4081228"
	row.EnderecoLongitude = "-42,1888707"
	if p := validateClientRowProblems(row, CatalogSets{}); len(p) != 0 {
		t.Fatalf("lat/lon com vírgula decimal não deveriam gerar problema: %v", p)
	}
}

// TestApplyClientImportRow_normalizesCommaLatLon confere ponta a ponta que o valor enviado de
// verdade à HubSoft já vem com ponto, nunca a vírgula crua (a HubSoft só aceita ponto decimal).
func TestApplyClientImportRow_normalizesCommaLatLon(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "msg": "Cliente cadastrado com sucesso.",
			"cliente": map[string]any{"id_cliente": 1},
		})
	}))
	defer srv.Close()

	row := validClientRow()
	row.EnderecoLatitude = "-21,4081228"
	row.EnderecoLongitude = "-42,1888707"
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK {
		t.Fatalf("esperava sucesso: %+v", res)
	}
	endereco, _ := gotBody["endereco"].(map[string]any)
	if endereco["latitude"] != "-21.4081228" || endereco["longitude"] != "-42.1888707" {
		t.Errorf("latitude/longitude deveriam ir com ponto, não vírgula: %v", endereco)
	}
}

// TestHubsoftActionMessageWithErrors confere que o array "errors" (mensagem específica por campo) é
// incorporado à mensagem — o caso real do usuário foi "Favor preencher os campos obrigatórios..." sem
// nenhuma pista de qual campo, porque a versão antiga (hubsoftActionMessage) descartava esse array.
func TestHubsoftActionMessageWithErrors(t *testing.T) {
	body := []byte(`{"status":"error","msg":"Favor preencher os campos obrigatórios de acordo com as especificações","errors":["O campo endereco.latitude é inválido.","O campo endereco.longitude é inválido."]}`)
	got := hubsoftActionMessageWithErrors(body)
	if !strings.Contains(got, "latitude") || !strings.Contains(got, "longitude") {
		t.Fatalf("esperava os detalhes de errors[] na mensagem final, veio: %q", got)
	}
}

// TestApplyClientImportRowDedup_fourCases cobre os 4 casos pedidos pelo usuário:
//  1. cliente não existe -> cria cliente + serviço (POST /cliente)
//  2. cliente existe, sem serviço -> cria só o serviço (POST /cliente/cliente_servico)
//  3. cliente existe, serviço com o MESMO login -> não chama nenhum POST, só avisa
//  4. cliente existe, serviço com login DIFERENTE -> cria um serviço novo (POST /cliente/cliente_servico)
func TestApplyClientImportRowDedup_fourCases(t *testing.T) {
	cases := []struct {
		name            string
		searchResponse  string
		wantDedupAction string
		wantPostCalled  bool
		wantPostPath    string
		wantOK          bool
	}{
		{
			name:            "caso1_nao_existe",
			searchResponse:  `{"status":"success","clientes":[]}`,
			wantDedupAction: "cliente_criado",
			wantPostCalled:  true,
			wantPostPath:    "/api/v1/integracao/cliente",
			wantOK:          true,
		},
		{
			name:            "caso2_existe_sem_servico",
			searchResponse:  `{"status":"success","clientes":[{"id_cliente":555,"cpf_cnpj":"45455937715","servicos":[]}]}`,
			wantDedupAction: "servico_adicionado",
			wantPostCalled:  true,
			wantPostPath:    "/api/v1/integracao/cliente/cliente_servico",
			wantOK:          true,
		},
		{
			name:            "caso3_existe_mesmo_login",
			searchResponse:  `{"status":"success","clientes":[{"id_cliente":555,"cpf_cnpj":"45455937715","servicos":[{"id_cliente_servico":777,"login":"joao123","status":"Serviço Habilitado"}]}]}`,
			wantDedupAction: "ja_existe",
			wantPostCalled:  false,
			wantOK:          true,
		},
		{
			name:            "caso4_existe_login_diferente",
			searchResponse:  `{"status":"success","clientes":[{"id_cliente":555,"cpf_cnpj":"45455937715","servicos":[{"id_cliente_servico":777,"login":"outrologin","status":"Serviço Habilitado"}]}]}`,
			wantDedupAction: "servico_adicionado",
			wantPostCalled:  true,
			wantPostPath:    "/api/v1/integracao/cliente/cliente_servico",
			wantOK:          true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			postCalled := false
			var postPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" && r.URL.Path == "/api/v1/integracao/cliente" {
					_, _ = w.Write([]byte(c.searchResponse))
					return
				}
				postCalled = true
				postPath = r.URL.Path
				switch r.URL.Path {
				case "/api/v1/integracao/cliente":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"status": "success", "msg": "Cliente cadastrado com sucesso.",
						"cliente": map[string]any{"id_cliente": 999, "servicos": []map[string]any{{"id_cliente_servico": 888}}},
					})
				case "/api/v1/integracao/cliente/cliente_servico":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"status": "success", "msg": "Serviço criado.",
						"cliente_servico": map[string]any{"id_cliente_servico": 888},
					})
				}
			}))
			defer srv.Close()

			row := validClientRow()
			row.Login = "joao123"
			row.Senha = "s3nh4teste"
			res := ApplyClientImportRowDedup(context.Background(), Config{BaseURL: srv.URL}, "tok", row)

			if res.DedupAction != c.wantDedupAction {
				t.Errorf("DedupAction = %q, esperava %q (res=%+v)", res.DedupAction, c.wantDedupAction, res)
			}
			if res.OK != c.wantOK {
				t.Errorf("OK = %v, esperava %v: %+v", res.OK, c.wantOK, res)
			}
			if postCalled != c.wantPostCalled {
				t.Errorf("postCalled = %v, esperava %v", postCalled, c.wantPostCalled)
			}
			if c.wantPostCalled && postPath != c.wantPostPath {
				t.Errorf("POST foi para %q, esperava %q", postPath, c.wantPostPath)
			}
		})
	}
}

// TestApplyClientImportRowDedup_lookupFailureBlocksCreation confere que, se a consulta de duplicidade
// falhar, a linha NUNCA tenta criar nada — não dá pra saber se já existe, criar às cegas arriscaria
// duplicar o cliente.
func TestApplyClientImportRowDedup_lookupFailureBlocksCreation(t *testing.T) {
	postCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/integracao/cliente" {
			w.WriteHeader(500)
			return
		}
		postCalled = true
		w.WriteHeader(500)
	}))
	defer srv.Close()
	row := validClientRow()
	row.Login = "joao123"
	row.Senha = "s3nh4teste"
	res := ApplyClientImportRowDedup(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if res.OK || res.Rejected {
		t.Fatalf("esperava falha (não rejeição local, não sucesso): %+v", res)
	}
	if postCalled {
		t.Fatalf("não podia ter tentado criar nada com a checagem de duplicidade falhando")
	}
}

// A HubSoft confirmou que "parametros" não cria autenticação (só grava login/senha como parâmetro do
// plano), então login/senha do CSV nunca podem ir no corpo da criação — só na configurar_autenticacao.
func TestApplyClientImportRow_neverSendsLoginInCreationBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "msg": "Cliente cadastrado com sucesso.",
			"cliente": map[string]any{"id_cliente": 1},
		})
	}))
	defer srv.Close()

	row := validClientRow()
	row.Login = "joao123"
	row.Senha = "s3nh4teste"
	if res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row); !res.OK {
		t.Fatalf("esperava sucesso: %+v", res)
	}
	if _, present := gotBody["parametros"]; present {
		t.Errorf("parametros não devia ir no corpo: %v", gotBody["parametros"])
	}
	raw, _ := json.Marshal(gotBody)
	if strings.Contains(string(raw), "s3nh4teste") {
		t.Errorf("a senha não podia ir no corpo da criação: %s", raw)
	}
}

func TestApplyLoginConfig_noAuthRecordGivesHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "O serviço/plano informado não possui dados de autenticação"})
	}))
	defer srv.Close()
	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "123", "", "joao123", "s3nh4")
	if res.OK || !strings.Contains(res.Message, "gera logins automaticamente") {
		t.Fatalf("esperava dica da automação de login: %+v", res)
	}
}

func TestApplyLoginConfig_hubsoftRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "Login já está em uso"})
	}))
	defer srv.Close()
	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "9988", "", "joao123", "s3nh4")
	if res.OK {
		t.Fatalf("esperava falha: %+v", res)
	}
	if !strings.Contains(res.Message, "em uso") {
		t.Fatalf("mensagem deveria repassar o motivo da HubSoft: %q", res.Message)
	}
}

// Caso real: com a automação de login automático o serviço já nasce na interface certa, e a HubSoft
// recusa o pedido inteiro se id_interface_conexao for repetido. Tem de refazer só com login/senha.
func TestApplyLoginConfig_retriesWithoutInterfaceWhenAlreadyLinked(t *testing.T) {
	var bodies []map[string]any
	loginNow := "netquasar"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" { // conferência do login depois da troca
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"id_cliente": 1, "servicos": []any{map[string]any{"id_cliente_servico": 7, "login": loginNow}},
			}}})
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		if _, has := b["id_interface_conexao"]; has {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error",
				"msg": "O serviço (0) 300 MB do cliente (2223) X já encontra-se vinculado à interface de conexão (5) - INTERFACE SFP, não é necessário a alteração."})
			return
		}
		loginNow, _ = b["login"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Parametros de autenticacao modificados com sucesso!"})
	}))
	defer srv.Close()

	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "5", "joao123", "s3nh4")
	if !res.OK {
		t.Fatalf("esperava OK após refazer sem a interface: %+v", res)
	}
	if len(bodies) != 2 {
		t.Fatalf("esperava 2 pedidos (com e sem interface), vieram %d", len(bodies))
	}
	if _, has := bodies[1]["id_interface_conexao"]; has || bodies[1]["login"] != "joao123" || bodies[1]["password"] != "s3nh4" {
		t.Errorf("2º pedido devia ter só login/senha: %v", bodies[1])
	}
}

func TestApplyLoginConfig_onlyInterfaceAlreadyLinkedIsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "já encontra-se vinculado à interface de conexão (5)"})
	}))
	defer srv.Close()
	if res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "5", "", ""); !res.OK {
		t.Fatalf("só interface já vinculada devia contar como OK: %+v", res)
	}
}

// A HubSoft pode responder "success" sem de facto trocar o login — não pode passar por OK.
func TestApplyLoginConfig_failsWhenLoginDidNotChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "netquasar"}},
			}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()
	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "", "joao123", "s3nh4")
	if res.OK || !strings.Contains(res.Message, "continua") {
		t.Fatalf("devia reprovar quando o login não mudou: %+v", res)
	}
}

func TestRepairPlaceholderService(t *testing.T) {
	svc := func(id, login string) ExistingClientService {
		return ExistingClientService{IDClienteServico: id, Login: login, Status: "Aguardando Instalação"}
	}
	// 1 serviço com o login padrão → reparo (nada criado)
	r := repairPlaceholderService(&ExistingClient{IDCliente: "5", Servicos: []ExistingClientService{svc("7", "NetQuasar")}}, "joao123", 3, "5")
	if r == nil || !r.OK || r.DedupAction != "login_a_corrigir" || r.IDServico != "7" {
		t.Fatalf("devia pedir reparo do serviço 7: %+v", r)
	}
	// outros serviços com login real não são tocados
	r = repairPlaceholderService(&ExistingClient{IDCliente: "5", Servicos: []ExistingClientService{svc("6", "maria"), svc("7", "netquasar")}}, "joao123", 3, "5")
	if r == nil || r.IDServico != "7" {
		t.Fatalf("só o serviço no login padrão devia ser alvo: %+v", r)
	}
	// ambíguo → recusa, nada criado/alterado
	r = repairPlaceholderService(&ExistingClient{IDCliente: "5", Servicos: []ExistingClientService{svc("7", "netquasar"), svc("8", "netquasar")}}, "joao123", 3, "5")
	if r == nil || r.OK || r.DedupAction != "" || !strings.Contains(r.Message, "2 serviços") {
		t.Fatalf("2 serviços no login padrão devia recusar: %+v", r)
	}
	// sem serviço no padrão, sem login no CSV, ou CSV pedindo o próprio login padrão → não é reparo
	if repairPlaceholderService(&ExistingClient{Servicos: []ExistingClientService{svc("6", "maria")}}, "joao123", 1, "5") != nil {
		t.Error("sem serviço no login padrão não é reparo")
	}
	if repairPlaceholderService(&ExistingClient{Servicos: []ExistingClientService{svc("7", "netquasar")}}, "", 1, "5") != nil {
		t.Error("CSV sem login não é reparo")
	}
	if repairPlaceholderService(&ExistingClient{Servicos: []ExistingClientService{svc("7", "netquasar")}}, "netquasar", 1, "5") != nil {
		t.Error("CSV pedindo o próprio login padrão não é reparo")
	}
}

// Fluxo completo do caso real (JANDIRA): cliente existe com o serviço no login padrão → a linha NÃO cria
// serviço novo (nenhum POST em /cliente_servico) e devolve o serviço certo para a troca de login.
func TestApplyClientImportRowDedup_repairsInsteadOfDuplicating(t *testing.T) {
	var posts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			posts = append(posts, r.URL.Path)
		}
		row := validClientRow()
		cpf, _ := normalizeCPFCNPJ(row.TipoPessoa, row.CPFCNPJ)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
			"id_cliente": 2223, "cpf_cnpj": cpf,
			"servicos": []any{map[string]any{"id_cliente_servico": 9001, "login": "netquasar", "status": "Aguardando Instalação"}},
		}}})
	}))
	defer srv.Close()
	row := validClientRow()
	row.Login = "jandira.berardi"
	row.Senha = "s3nh4"
	res := ApplyClientImportRowDedup(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK || res.DedupAction != "login_a_corrigir" || res.IDServico != "9001" {
		t.Fatalf("devia pedir reparo do serviço 9001: %+v", res)
	}
	if len(posts) != 0 {
		t.Fatalf("não podia criar nada: %v", posts)
	}
}

// Caso real (REINALDO): o login já era o informado → a HubSoft recusa com "já encontra-se com o login".
// Com senha no CSV, refaz só com a senha; sem senha, não há nada a alterar e a linha está OK.
func TestApplyLoginConfig_loginAlreadySet(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "reinaldo"}},
			}}})
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		if _, has := b["login"]; has {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error",
				"msg": "O serviço (0) 100 MB do cliente (2227) X já encontra-se com o login reinaldo, não é necessário a alteração."})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()

	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "", "reinaldo", "s3nh4")
	if !res.OK || len(bodies) != 2 || bodies[1]["password"] != "s3nh4" {
		t.Fatalf("devia refazer só com a senha: ok=%v bodies=%v msg=%q", res.OK, bodies, res.Message)
	}
	if _, has := bodies[1]["login"]; has {
		t.Errorf("2º pedido não devia levar o login: %v", bodies[1])
	}

	bodies = nil
	res = ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "", "reinaldo", "")
	if !res.OK || len(bodies) != 1 {
		t.Fatalf("login já certo e sem senha → OK sem novo pedido: %+v bodies=%v", res, bodies)
	}
}

func TestApplyClientDefaults_dataNascimento(t *testing.T) {
	row := validClientRow()
	row.DataNascimento = "  "
	got, defaulted := applyClientDefaults(row)
	if !defaulted || got.DataNascimento != "1900-01-01" {
		t.Fatalf("devia preencher 1900-01-01: %+v defaulted=%v", got.DataNascimento, defaulted)
	}
	row.DataNascimento = "1985-03-20"
	if got, defaulted = applyClientDefaults(row); defaulted || got.DataNascimento != "1985-03-20" {
		t.Fatalf("data existente não pode ser trocada: %q", got.DataNascimento)
	}
	// a validação marca a linha como válida e avisa do default
	row.DataNascimento = ""
	out := ValidateClientImportRows([]ClientImportRow{row}, CatalogSets{})
	if len(out.Rows) != 1 || !out.Rows[0].Valid || len(out.Rows[0].Info) == 0 {
		t.Fatalf("validação devia aceitar e avisar do default: %+v", out.Rows)
	}
	// e o corpo enviado à HubSoft leva 1900-01-01
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok", "cliente": map[string]any{"id_cliente": 1}})
	}))
	defer srv.Close()
	if res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row); !res.OK {
		t.Fatalf("esperava sucesso: %+v", res)
	}
	if gotBody["data_nascimento"] != "1900-01-01" {
		t.Errorf("data_nascimento enviada = %v", gotBody["data_nascimento"])
	}
}

// Login já existe em OUTRO cadastro (cliente não achado pelo CPF): não pode criar nada.
func TestApplyClientImportRowDedup_loginExistsElsewhere(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			posts++
		}
		if r.URL.Query().Get("busca") == "login_radius" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"id_cliente": 99, "nome_razaosocial": "OUTRO CLIENTE",
				"servicos": []any{map[string]any{"id_cliente_servico": 500, "login": "Jandira.Berardi", "status": "Serviço Habilitado"}},
			}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{}}) // CPF não encontrado
	}))
	defer srv.Close()
	row := validClientRow()
	row.Login = "jandira.berardi"
	row.Senha = "s3nh4"
	res := ApplyClientImportRowDedup(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if res.OK || res.DedupAction != "login_em_uso" || res.IDCliente != "99" || !strings.Contains(res.Message, "OUTRO CLIENTE") {
		t.Fatalf("devia recusar por login em uso: %+v", res)
	}
	if posts != 0 {
		t.Fatalf("não podia criar nada, houve %d POST(s)", posts)
	}
}

// Mesmo caso na aba de serviços adicionais (cliente existe, login existe em outro cadastro).
func TestApplyServiceImportRowDedup_loginExistsElsewhere(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			posts++
		}
		switch r.URL.Query().Get("busca") {
		case "login_radius":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"id_cliente": 99, "nome_razaosocial": "OUTRO",
				"servicos": []any{map[string]any{"id_cliente_servico": 500, "login": "maria", "status": "Habilitado"}},
			}}})
		default: // id_cliente=5, sem o login da linha
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"id_cliente": 5, "servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "joao", "status": "Habilitado"}},
			}}})
		}
	}))
	defer srv.Close()
	row := validServiceRow()
	row.IDCliente = "5"
	row.Login = "maria"
	row.Senha = "s3nh4"
	res := ApplyServiceImportRowDedup(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if res.OK || res.DedupAction != "login_em_uso" || posts != 0 {
		t.Fatalf("devia recusar sem criar: %+v posts=%d", res, posts)
	}
}

// Caso real: login trocado, senha ficou a da máscara → a 2ª chamada (só senha) corrige e é conferida.
func TestApplyLoginConfig_fixesWrongPassword(t *testing.T) {
	senhaNow := "nq12345"
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "joao123", "senha": senhaNow}},
			}}})
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		posts = append(posts, b)
		if _, hasLogin := b["login"]; !hasLogin { // só a chamada "somente senha" realmente muda a senha
			senhaNow, _ = b["password"].(string)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()
	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "", "joao123", "RealPass9")
	if !res.OK || len(posts) != 2 || senhaNow != "RealPass9" {
		t.Fatalf("devia reaplicar a senha: ok=%v posts=%v senha=%q msg=%q", res.OK, posts, senhaNow, res.Message)
	}
	// se nem a 2ª chamada resolver, não pode ficar OK
	senhaNow = "nq12345"
	stuck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "joao123", "senha": "nq12345"}}}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer stuck.Close()
	if res = ApplyLoginConfig(context.Background(), Config{BaseURL: stuck.URL}, "tok", "7", "", "joao123", "RealPass9"); res.OK {
		t.Fatalf("senha continua errada — não pode dar OK: %+v", res)
	}
}

func TestRepairPlaceholderPassword(t *testing.T) {
	cur := "nq12345"
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "joao123", "senha": cur}}}}})
			return
		}
		posts++
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		cur, _ = b["password"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	if rep := RepairPlaceholderPassword(context.Background(), cfg, "tok", "7", "joao123", "RealPass9"); rep == nil || !rep.OK || cur != "RealPass9" {
		t.Fatalf("devia corrigir a senha padrão: %+v cur=%q", rep, cur)
	}
	// agora a senha é a real (ou qualquer outra) → nunca toca
	posts = 0
	cur = "SenhaDoCliente"
	if rep := RepairPlaceholderPassword(context.Background(), cfg, "tok", "7", "joao123", "RealPass9"); rep != nil || posts != 0 {
		t.Fatalf("não pode tocar numa senha que não é a padrão: %+v posts=%d", rep, posts)
	}
}

func validServiceRow() ServiceImportRow {
	return ServiceImportRow{
		Line: 2, IDCliente: "5", IDServico: "21", IDVencimento: "4", IDUsuarioVendedor: "86", IDFormaCobranca: "10",
		IDServicoStatus: "1", Valor: "69.9", DataVenda: "2021-03-15", Carne: "false", TaxaInstalacaoTipo: "nao_cobrar_taxa",
	}
}

// A HubSoft padroniza o login em minúsculas e o Radius dela não diferencia caixa: diferença só de caixa NÃO é
// falha — o login original vai para as Observações da autenticação. A senha continua exata.
func TestApplyLoginConfig_caseOnlyDifferenceRecordsObservation(t *testing.T) {
	obs := ""
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": "c4-07heliojunior", "senha": "Abc123", "observacoes_autenticacao": obs}}}}})
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		posts = append(posts, b)
		if v, ok := b["observacoes"].(string); ok {
			obs = v
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()
	res := ApplyLoginConfig(context.Background(), Config{BaseURL: srv.URL}, "tok", "7", "", "C4-07heliojunior", "Abc123")
	if !res.OK {
		t.Fatalf("caixa diferente devia ser OK: %+v", res)
	}
	if obs != "Login PPPoE configurado no cliente: C4-07heliojunior" {
		t.Errorf("observação gravada = %q", obs)
	}
	if !passwordMismatch("abc123", "Abc123") || passwordMismatch("Abc123", "Abc123") {
		t.Error("passwordMismatch devia continuar sensível à caixa")
	}
}

func TestRecordLoginObservation(t *testing.T) {
	cur, obs := "c4-07heliojunior", "Instalado por João"
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"clientes": []any{map[string]any{
				"servicos": []any{map[string]any{"id_cliente_servico": 7, "login": cur, "observacoes_autenticacao": obs}}}}})
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		posts = append(posts, b)
		obs, _ = b["observacoes"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	rec := RecordLoginObservation(context.Background(), cfg, "tok", "7", "C4-07heliojunior")
	if rec == nil || !rec.OK {
		t.Fatalf("devia registrar: %+v", rec)
	}
	if obs != "Instalado por João\nLogin PPPoE configurado no cliente: C4-07heliojunior" {
		t.Errorf("a observação existente do operador tem de ser preservada: %q", obs)
	}
	if _, has := posts[0]["login"]; has || len(posts[0]) != 2 {
		t.Errorf("o POST só pode levar id_cliente_servico e observacoes: %v", posts[0])
	}
	// já registrado → não repete o POST
	posts = nil
	if rec = RecordLoginObservation(context.Background(), cfg, "tok", "7", "C4-07heliojunior"); rec == nil || !rec.OK || len(posts) != 0 {
		t.Errorf("já registrado não pode gerar novo POST: %+v posts=%v", rec, posts)
	}
	// login igual ao original (HubSoft manteve a caixa) ou diferente de verdade → nada a fazer
	cur = "C4-07heliojunior"
	if RecordLoginObservation(context.Background(), cfg, "tok", "7", "C4-07heliojunior") != nil {
		t.Error("login idêntico não precisa de observação")
	}
	cur = "outrologin"
	if RecordLoginObservation(context.Background(), cfg, "tok", "7", "C4-07heliojunior") != nil {
		t.Error("login realmente diferente não pode ser tratado como padronização de caixa")
	}
}

func TestIsUnderage(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local)
	cases := map[string]bool{
		"2010-05-01": true,  // 16 anos
		"2008-10-03": true,  // faz 18 amanhã
		"2008-10-02": false, // faz 18 hoje
		"1990-01-01": false,
		"15/03/2012": true, // DD/MM/AAAA
		"":           false,
		"lixo":       false,
	}
	for in, want := range cases {
		if got := isUnderage(in, now); got != want {
			t.Errorf("isUnderage(%q) = %v, want %v", in, got, want)
		}
	}
}

// Menor de 18 recusado pela HubSoft por causa da data → refaz UMA vez com 01/01/1990.
func TestApplyClientImportRow_underageRetriesWithFallbackDate(t *testing.T) {
	var datas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		d, _ := b["data_nascimento"].(string)
		datas = append(datas, d)
		w.Header().Set("Content-Type", "application/json")
		if d != "1990-01-01" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "Favor preencher os campos obrigatórios", "errors": []string{"O cliente deve ser maior de 18 anos (data_nascimento)."}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Cliente cadastrado com sucesso.", "cliente": map[string]any{"id_cliente": 1}})
	}))
	defer srv.Close()
	row := validClientRow()
	row.DataNascimento = time.Now().AddDate(-15, 0, 0).Format("2006-01-02")
	res := ApplyClientImportRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK || len(datas) != 2 || datas[1] != "1990-01-01" || !strings.Contains(res.Message, "01/01/1990") {
		t.Fatalf("devia refazer com 1990-01-01: res=%+v datas=%v", res, datas)
	}

	// maior de idade recusado por OUTRO motivo → não troca a data nem reenvia
	datas = nil
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		d, _ := b["data_nascimento"].(string)
		datas = append(datas, d)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "CPF já cadastrado (data_nascimento ok)"})
	}))
	defer srv2.Close()
	row.DataNascimento = "1985-03-20"
	if res = ApplyClientImportRow(context.Background(), Config{BaseURL: srv2.URL}, "tok", row); res.OK || len(datas) != 1 {
		t.Fatalf("adulto não pode ter a data trocada nem reenvio: res=%+v datas=%v", res, datas)
	}
}

// Regras reais da HubSoft: telefone >= 10 dígitos e e-mail ASCII sem ponto no fim.
func TestValidateClientRow_phoneAndEmailRules(t *testing.T) {
	probs := func(mut func(*ClientImportRow)) string {
		r := validClientRow()
		r.DataNascimento = "1985-03-20"
		mut(&r)
		return strings.Join(validateClientRowProblems(r, CatalogSets{}), " | ")
	}
	if p := probs(func(r *ClientImportRow) { r.TelefonePrimario = "0" }); !strings.Contains(p, "mínimo 10") {
		t.Errorf("telefone '0' devia reprovar: %q", p)
	}
	for _, bad := range []string{"carmeneiras07@gmail.com.", "rogério35@gmail.com", "x@g2telecom", "a b@x.com", "nãotememail/carlos@g2telecom.com"} {
		if p := probs(func(r *ClientImportRow) { r.EmailPrincipal = bad }); !strings.Contains(p, "e-mail") {
			t.Errorf("e-mail %q devia reprovar: %q", bad, p)
		}
	}
	for _, good := range []string{"joao.silva+nf@gmail.com", "TESTE@G2TELECOM.COM.BR", ""} {
		if p := probs(func(r *ClientImportRow) { r.EmailPrincipal = good }); strings.Contains(p, "e-mail") {
			t.Errorf("e-mail %q devia passar: %q", good, p)
		}
	}
}

func existingClientServer(t *testing.T, posts *[]map[string]any, addr map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			*posts = append(*posts, b)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Serviço adicionado ao Cliente com sucesso",
				"cliente_servico": map[string]any{"id_cliente_servico": 2861}})
			return
		}
		if r.URL.Query().Get("busca") == "login_radius" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
			"id_cliente": 1717, "cpf_cnpj": "45455937715",
			"servicos": []any{map[string]any{"id_cliente_servico": 1949, "login": "c12-06anafilho", "status": "Habilitado", "endereco_instalacao": addr}},
		}}})
	}))
}

// Caso ANA SILVIA: cliente já existia noutro endereço → o serviço novo TEM de levar o endereço do CSV e a
// mensagem tem de avisar.
func TestApplyClientImportRowDedup_existingClientGetsCSVInstallAddress(t *testing.T) {
	var posts []map[string]any
	srv := existingClientServer(t, &posts, map[string]any{"cep": "28460000", "bairro": "SANTA TERESA", "endereco": "AVENIDA CARVALHO", "numero": "131"})
	defer srv.Close()
	row := validClientRow() // CEP 28460000, Rua A, 10, Centro
	row.EnderecoCEP, row.EnderecoBairro, row.EnderecoLogradouro, row.EnderecoNumero = "28470000", "MONTE ALEGRE", "Rua Procopio da Costa Junior", "61"
	row.DataNascimento = "1985-03-20"
	row.Login, row.Senha = "anasilvia01", "g212345"
	res := ApplyClientImportRowDedup(context.Background(), Config{BaseURL: srv.URL}, "tok", row)
	if !res.OK || res.DedupAction != "servico_adicionado" || !strings.Contains(res.Message, "outro endereço") {
		t.Fatalf("devia adicionar o serviço e avisar do endereço: %+v", res)
	}
	if len(posts) != 1 {
		t.Fatalf("posts = %d", len(posts))
	}
	inst, _ := posts[0]["endereco_instalacao"].(map[string]any)
	if inst["bairro"] != "MONTE ALEGRE" || inst["endereco"] != "Rua Procopio da Costa Junior" || inst["numero"] != "61" || inst["cep"] != "28470000" {
		t.Errorf("endereco_instalacao enviado = %v", inst)
	}
}

func TestPreflightClientRow_statuses(t *testing.T) {
	var posts []map[string]any
	row := validClientRow()
	row.DataNascimento = "1985-03-20"
	row.Login = "novo.login"
	// mesmo endereço
	srv := existingClientServer(t, &posts, map[string]any{"cep": "28460000", "bairro": "Centro", "endereco": "Rua A", "numero": "10"})
	if pr := PreflightClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row); pr.Status != PreExistsSame {
		t.Errorf("mesmo endereço: %+v", pr)
	}
	srv.Close()
	// outro endereço
	srv = existingClientServer(t, &posts, map[string]any{"cep": "28460000", "bairro": "Santa Teresa", "endereco": "Avenida Carvalho", "numero": "131"})
	if pr := PreflightClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row); pr.Status != PreExistsOther || len(pr.HubAddrs) != 1 {
		t.Errorf("outro endereço: %+v", pr)
	}
	// login já cadastrado nesse cliente
	row2 := row
	row2.Login = "c12-06anafilho"
	if pr := PreflightClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row2); pr.Status != PreAlreadyDone {
		t.Errorf("login já no cliente: %+v", pr)
	}
	srv.Close()
	// cliente novo
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","clientes":[]}`))
	}))
	defer empty.Close()
	if pr := PreflightClientRow(context.Background(), Config{BaseURL: empty.URL}, "tok", row); pr.Status != PreNew {
		t.Errorf("novo: %+v", pr)
	}
}

// carne=true exige gerar_carne na HubSoft: a importação marca o serviço como carnê mas NUNCA gera carnê/boleto.
func TestImportBodies_carneTrueSendsNaoGerarCarne(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok",
			"cliente": map[string]any{"id_cliente": 1}, "cliente_servico": map[string]any{"id_cliente_servico": 2}})
	}))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}

	c := validClientRow()
	c.DataNascimento = "1985-03-20"
	c.Carne = "true"
	if res := ApplyClientImportRow(context.Background(), cfg, "tok", c); !res.OK {
		t.Fatalf("cliente: %+v", res)
	}
	sv := validServiceRow()
	sv.Carne = "true"
	if res := ApplyServiceImportRow(context.Background(), cfg, "tok", sv); !res.OK {
		t.Fatalf("serviço: %+v", res)
	}
	for i, b := range bodies {
		if b["carne"] != true || b["gerar_carne"] != "nao_gerar_carne" {
			t.Errorf("corpo %d: carne=%v gerar_carne=%v", i, b["carne"], b["gerar_carne"])
		}
	}
	// carne=false não manda gerar_carne
	bodies = nil
	c.Carne = "false"
	_ = ApplyClientImportRow(context.Background(), cfg, "tok", c)
	if _, has := bodies[0]["gerar_carne"]; has || bodies[0]["carne"] != false {
		t.Errorf("carne=false: %v", bodies[0])
	}
}
