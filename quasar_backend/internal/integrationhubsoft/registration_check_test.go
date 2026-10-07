package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func checkByField(rc RowCheck, field string) *CheckItem {
	for i := range rc.Checks {
		if rc.Checks[i].Field == field {
			return &rc.Checks[i]
		}
	}
	return nil
}

func testLabels() CheckLabels {
	return CheckLabels{
		Servico:   map[string][]string{"21": {"300 MB - PÓS PAGO (G2)"}},
		Status:    map[string][]string{"1": {"Serviço Habilitado", "servico_habilitado"}},
		Vendedor:  map[string][]string{"86": {"MARIA VENDEDORA"}},
		Interface: map[string][]string{"5": {"INTERFACE SFP", "OLT PADUA"}},
	}
}

func fullClientPayload(login, senha string) map[string]any {
	return map[string]any{
		"id_cliente": 2223, "nome_razaosocial": "JANDIRA APARECIDA DE PAULA BERARDI", "tipo_pessoa": "pf",
		"cpf_cnpj": "45455937715", "telefone_primario": "22998136491", "email_principal": "j@x.com",
		"data_nascmento": "1985-03-20 00:00:00", "rg": "MG123",
		"servicos": []any{map[string]any{
			"id_cliente_servico": 9001, "nome": "300 MB - PÓS PAGO (G2)", "numero_plano": 3, "valor": 69.9,
			"status": "Serviço Habilitado", "status_prefixo": "servico_habilitado", "login": login, "senha": senha,
			"data_venda": "2021-03-15 00:00:00", "carne": false, "referencia": "REF1",
			"vendedor":            map[string]any{"nome": "MARIA VENDEDORA"},
			"interface":           map[string]any{"nome": "INTERFACE SFP"},
			"equipamento_conexao": map[string]any{"nome": "OLT PADUA"},
			"endereco_cadastral": map[string]any{
				"cep": "28460-000", "bairro": "Centro", "endereco": "Rua A", "numero": "10", "complemento": "",
				"coordenadas": map[string]any{"latitude": -21.5412, "longitude": -42.1234},
			},
		}},
	}
}

func checkRow() ClientImportRow {
	r := validClientRow()
	r.NomeRazaoSocial = "Jandira Aparecida de Paula Berardi" // caixa diferente
	r.CPFCNPJ = "45455937715"
	r.TelefonePrimario = "(22) 99813-6491"
	r.EmailPrincipal = "J@X.com"
	r.DataNascimento = "20/03/1985"
	r.RG = "mg123"
	r.EnderecoCEP = "28460000"
	r.EnderecoBairro = "CENTRO"
	r.EnderecoLogradouro = "Rua A"
	r.EnderecoNumero = "10"
	r.EnderecoLatitude = "-21,5412"
	r.EnderecoLongitude = "-42,1234"
	r.IDServico, r.IDServicoStatus, r.IDUsuarioVendedor, r.IDInterfaceConexao = "21", "1", "86", "5"
	r.Valor, r.DataVenda, r.Carne = "69,90", "2021-03-15", "false"
	r.Referencia = "REF1"
	r.Login, r.Senha = "C4-Jandira", "Abc123"
	return r
}

func mockCheckServer(t *testing.T, byCPF, byLogin []any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		list := byCPF
		if r.URL.Query().Get("busca") == "login_radius" {
			list = byLogin
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": list})
	}))
}

func TestCheckClientRow_allOK(t *testing.T) {
	srv := mockCheckServer(t, []any{fullClientPayload("C4-Jandira", "Abc123")}, nil)
	defer srv.Close()
	rc := CheckClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), testLabels())
	if rc.Status != RowOK || rc.DiffCount != 0 || rc.OKCount < 15 {
		for _, c := range rc.Checks {
			t.Logf("%s: %s exp=%q found=%q %s", c.Field, c.Status, c.Expected, c.Found, c.Note)
		}
		t.Fatalf("esperava tudo OK: %+v", rc)
	}
	// vencimento/forma de cobrança não vêm na consulta → não verificável, nunca "ok" nem "diff"
	if it := checkByField(rc, "id_vencimento"); it == nil || it.Status != CheckUnverified {
		t.Errorf("id_vencimento devia ser não verificável: %+v", it)
	}
}

func TestCheckClientRow_diffsAreReported(t *testing.T) {
	// login com outra caixa, senha ainda a padrão, valor e CEP diferentes
	p := fullClientPayload("c4-jandira", "nq12345")
	svc := p["servicos"].([]any)[0].(map[string]any)
	svc["valor"] = 59.9
	svc["endereco_cadastral"].(map[string]any)["cep"] = "28460-111"
	srv := mockCheckServer(t, []any{p}, nil)
	defer srv.Close()
	rc := CheckClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), testLabels())
	if rc.Status != RowDivergent {
		t.Fatalf("devia ser divergente: %+v", rc)
	}
	if it := checkByField(rc, "senha"); it == nil || it.Status != CheckDiff || !contains(it.Note, "padrão") {
		t.Errorf("senha: %+v", it)
	}
	// login só com outra caixa: a HubSoft padroniza em minúsculas (Radius não diferencia) → não é divergência,
	// mas a observação com o login original ainda falta e aparece como pendência.
	if it := checkByField(rc, "login"); it == nil || it.Status != CheckOK {
		t.Errorf("login só-caixa devia ser ok: %+v", it)
	}
	if it := checkByField(rc, "observacoes_login"); it == nil || it.Status != CheckDiff || it.Expected != "Login PPPoE configurado no cliente: C4-Jandira" {
		t.Errorf("devia cobrar o registro do login original: %+v", it)
	}
	if it := checkByField(rc, "valor"); it == nil || it.Status != CheckDiff {
		t.Errorf("valor devia divergir: %+v", it)
	}
	if it := checkByField(rc, "endereco_cep"); it == nil || it.Status != CheckDiff {
		t.Errorf("cep devia divergir: %+v", it)
	}
	if it := checkByField(rc, "nome_razaosocial"); it == nil || it.Status != CheckOK {
		t.Errorf("nome só difere na caixa → ok: %+v", it)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestCheckClientRow_notFoundAndFoundByLogin(t *testing.T) {
	srv := mockCheckServer(t, nil, nil)
	defer srv.Close()
	rc := CheckClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), testLabels())
	if rc.Status != RowNotFound {
		t.Fatalf("sem cadastro devia ser nao_encontrado: %+v", rc)
	}

	// cadastro existe com outro CPF, achado pelo login → compara e mostra o CPF divergente
	p := fullClientPayload("C4-Jandira", "Abc123")
	p["cpf_cnpj"] = "99999999999"
	srv2 := mockCheckServer(t, nil, []any{p})
	defer srv2.Close()
	rc = CheckClientRow(context.Background(), Config{BaseURL: srv2.URL}, "tok", checkRow(), testLabels())
	if rc.Status != RowDivergent {
		t.Fatalf("devia ser divergente (CPF): %+v", rc)
	}
	if it := checkByField(rc, "cpf_cnpj"); it == nil || it.Status != CheckDiff {
		t.Errorf("cpf devia divergir: %+v", it)
	}
}

func TestCheckClientRow_placeholderServicePicked(t *testing.T) {
	srv := mockCheckServer(t, []any{fullClientPayload("netquasar", "nq12345")}, nil)
	defer srv.Close()
	rc := CheckClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), testLabels())
	if rc.Status != RowDivergent || rc.IDServico != "9001" {
		t.Fatalf("serviço com login padrão devia ser escolhido e divergir: %+v", rc)
	}
	if it := checkByField(rc, "login"); it == nil || !contains(it.Note, "padrão") {
		t.Errorf("login devia citar o login padrão: %+v", it)
	}
}

func TestCheckServiceRow_andMissingCatalog(t *testing.T) {
	srv := mockCheckServer(t, []any{fullClientPayload("C4-Jandira", "Abc123")}, nil)
	defer srv.Close()
	row := validServiceRow()
	row.IDCliente = "2223"
	row.IDServico, row.IDServicoStatus, row.IDUsuarioVendedor = "21", "1", "86"
	row.Valor, row.DataVenda = "69.9", "2021-03-15"
	row.Login, row.Senha = "C4-Jandira", "Abc123"
	labels := testLabels()
	labels.Servico = nil // catálogo de planos não lido → plano "não verificável", não "errado"
	rc := CheckServiceRow(context.Background(), Config{BaseURL: srv.URL}, "tok", row, labels)
	if rc.Status != RowOK {
		t.Fatalf("esperava ok: %+v", rc)
	}
	if it := checkByField(rc, "id_servico"); it == nil || it.Status != CheckUnverified {
		t.Errorf("plano sem catálogo devia ser não verificável: %+v", it)
	}
}

func TestPickService(t *testing.T) {
	a := map[string]any{"login": "joao"}
	b := map[string]any{"login": "netquasar"}
	c := map[string]any{"login": "netquasar"}
	if s, amb := pickService([]map[string]any{a, b}, "JOAO"); s == nil || amb {
		t.Error("login com outra caixa deve achar o serviço")
	}
	if s, amb := pickService([]map[string]any{a, b}, "maria"); s == nil || amb || pickStr(s, "login") != "netquasar" {
		t.Errorf("1 serviço com login padrão deve ser escolhido: %v %v", s, amb)
	}
	if s, amb := pickService([]map[string]any{a, b, c}, "maria"); s != nil || !amb {
		t.Error("2 serviços com login padrão → ambíguo")
	}
	if s, amb := pickService([]map[string]any{a}, "maria"); s != nil || amb {
		t.Error("nenhum serviço compatível → nem escolhe nem ambíguo")
	}
}

func TestCatalogLabelsFromJSON(t *testing.T) {
	body := []byte(`{"servicos":[{"id_servico":21,"descricao":"300 MB","numero_plano":3}],"servico_status":[{"id_servico_status":1,"descricao":"Serviço Habilitado","prefixo":"servico_habilitado"}]}`)
	if l := catalogLabelsFromJSON("servico", body); len(l["21"]) != 2 {
		t.Errorf("planos: %v", l)
	}
	if l := catalogLabelsFromJSON("servico_status", body); len(l["1"]) != 2 {
		t.Errorf("status: %v", l)
	}
	if catalogLabelsFromJSON("vendedor", body) != nil {
		t.Error("lista ausente deve devolver nil (não verificado), não mapa vazio")
	}
	if l := interfaceLabelsFromJSON([]byte(`{"equipamentos":[{"nome":"OLT X","interfaces":[{"id_interface_conexao":5,"nome":"PON 1"}]}]}`)); len(l["5"]) != 2 || l["5"][1] != "OLT X" {
		t.Errorf("interfaces: %v", l)
	}
}

// A HubSoft devolve carnê como "Sim"/"Não" (texto) — "false" no CSV tem de bater com "Não".
func TestParseYesNo_andCarneCheck(t *testing.T) {
	for in, want := range map[string]bool{"Sim": true, "NÃO": false, "false": false, "true": true, "1": true, "0": false} {
		if got, ok := parseYesNo(in); !ok || got != want {
			t.Errorf("parseYesNo(%q) = %v,%v", in, got, ok)
		}
	}
	if _, ok := parseYesNo("talvez"); ok {
		t.Error("valor desconhecido não pode ser interpretado")
	}
	p := fullClientPayload("C4-Jandira", "Abc123")
	p["servicos"].([]any)[0].(map[string]any)["carne"] = "Não"
	srv := mockCheckServer(t, []any{p}, nil)
	defer srv.Close()
	rc := CheckClientRow(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), testLabels())
	if it := checkByField(rc, "carne"); it == nil || it.Status != CheckOK {
		t.Errorf("carne \"false\" × \"Não\" devia ser ok: %+v", it)
	}
}

// Sem relacoes=endereco_instalacao a HubSoft não manda o endereço — a consulta tem de pedir.
func TestFetchClientsRaw_asksForInstallAddress(t *testing.T) {
	var q map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clientes":[]}`))
	}))
	defer srv.Close()
	if _, err := fetchClientsRaw(context.Background(), Config{BaseURL: srv.URL}, "tok", "cpf_cnpj", "123"); err != nil {
		t.Fatal(err)
	}
	if len(q["relacoes"]) == 0 || q["relacoes"][0] != "endereco_instalacao" || q["busca"][0] != "cpf_cnpj" || q["cancelado"][0] != "todos" {
		t.Errorf("parâmetros: %v", q)
	}
}

func TestSpeedParsing(t *testing.T) {
	for in, want := range map[string]int{"300 MB - PÓS PAGO (G2)": 300, "70 MEGAS FIBRA - PRÉ PAGO (G2)": 70, "1 GIGA - PÓS PAGO (G2)": 1000, "10 MEGAS RURAL - PÓS PAGO": 10, "100 MB RÁDIO - PÓS PAGO (G2)": 100} {
		if got, ok := speedFromPlanName(in); !ok || got != want {
			t.Errorf("speedFromPlanName(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := speedFromPlanName("2º PONTO"); ok {
		t.Error("nome sem velocidade não pode ser interpretado")
	}
	if v, ok := speedFromHubsoft("350 Mbits"); !ok || v != 350 {
		t.Errorf("350 Mbits → %v %v", v, ok)
	}
	if v, ok := speedFromHubsoft("1 Gbits"); !ok || v != 1000 {
		t.Errorf("1 Gbits → %v %v", v, ok)
	}
}

// Modo específico: só nome, CPF/CNPJ, login, senha, valor e velocidade do plano.
func TestCheckClientRow_specificModeOnlyFilteredFields(t *testing.T) {
	p := fullClientPayload("C4-Jandira", "Abc123")
	svc := p["servicos"].([]any)[0].(map[string]any)
	svc["velocidade_download"], svc["velocidade_upload"] = "350 Mbits", "350 Mbits"
	srv := mockCheckServer(t, []any{p}, nil)
	defer srv.Close()
	labels := testLabels()
	labels.Servico = map[string][]string{"21": {"300 MB - PÓS PAGO (G2)"}}

	full := CheckClientRowMode(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), labels, ModeComplete)
	spec := CheckClientRowMode(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), labels, ModeSpecific)
	if len(spec.Checks) >= len(full.Checks) {
		t.Fatalf("específica devia ter menos campos: %d vs %d", len(spec.Checks), len(full.Checks))
	}
	got := map[string]bool{}
	for _, c := range spec.Checks {
		got[c.Field] = true
	}
	for _, f := range []string{"nome_razaosocial", "cpf_cnpj", "login", "senha", "valor", "velocidade_plano"} {
		if !got[f] {
			t.Errorf("faltou %s na conferência específica: %v", f, got)
		}
	}
	for _, f := range []string{"endereco_cep", "data_venda", "email_principal", "carne", "id_servico_status"} {
		if got[f] {
			t.Errorf("%s não devia estar na conferência específica", f)
		}
	}
	if it := checkByField(spec, "velocidade_plano"); it == nil || it.Status != CheckOK {
		t.Errorf("350 Mbits para plano de 300 deve ser ok (margem de 50): %+v", it)
	}
	if spec.Status != RowOK {
		t.Errorf("tudo igual nos campos específicos: %+v", spec)
	}
	// velocidade muito abaixo do plano → diverge
	svc["velocidade_download"], svc["velocidade_upload"] = "100 Mbits", "100 Mbits"
	if it := checkByField(CheckClientRowMode(context.Background(), Config{BaseURL: srv.URL}, "tok", checkRow(), labels, ModeSpecific), "velocidade_plano"); it == nil || it.Status != CheckDiff {
		t.Errorf("100 Mbits para plano de 300 deve divergir: %+v", it)
	}
}
