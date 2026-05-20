package integrationconsumer

import (
	"testing"
)

func TestParseHubsoftClientSearch_error(t *testing.T) {
	raw := []byte(`{"status":"error","msg":"Favor preencher o atributo (busca)"}`)
	r := ParseHubsoftClientSearch(raw)
	if r.OK {
		t.Fatal("expected not ok")
	}
	if r.Message == "" {
		t.Fatal("expected message")
	}
}

func TestParseHubsoftClientSearch_clients(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"clientes":[{
			"codigo_cliente":"100",
			"nome_razaosocial":"Cliente Teste",
			"cpf_cnpj":"12345678900",
			"telefone":"11999990000",
			"servicos":[{"login_radius":"user1","status":"servico_habilitado","nome":"Plano 100M"}]
		}]
	}`)
	r := ParseHubsoftClientSearch(raw)
	if !r.OK || len(r.Clients) != 1 {
		t.Fatalf("ok=%v clients=%d", r.OK, len(r.Clients))
	}
	c := r.Clients[0]
	if c.Name != "Cliente Teste" || c.Document != "12345678900" {
		t.Fatalf("card: %+v", c)
	}
	if len(c.Services) != 1 || c.Services[0].Login != "user1" {
		t.Fatalf("services: %+v", c.Services)
	}
}

func TestParseHubsoftClientSearch_ipv4(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"clientes":[{
			"codigo_cliente":"200",
			"nome_razaosocial":"Cliente IP",
			"cpf_cnpj":"98765432100",
			"servicos":[{"login_radius":"user2","ipv4":"187.45.10.88","nome":"Plano 200M"}]
		}]
	}`)
	r := ParseHubsoftClientSearch(raw)
	if !r.OK || len(r.Clients) != 1 {
		t.Fatalf("ok=%v clients=%d", r.OK, len(r.Clients))
	}
	c := r.Clients[0]
	if c.IPv4 != "187.45.10.88" {
		t.Fatalf("ipv4=%q services=%+v", c.IPv4, c.Services)
	}
	if c.Services[0].IPv4 != "187.45.10.88" {
		t.Fatalf("service ipv4=%q", c.Services[0].IPv4)
	}
}
