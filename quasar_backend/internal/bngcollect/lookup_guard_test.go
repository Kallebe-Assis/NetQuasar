package bngcollect

import (
	"testing"

	"github.com/google/uuid"
)

func TestSuspiciousMassDropAdiaEAceitaNaSegunda(t *testing.T) {
	id := uuid.New()
	if suspiciousMassDrop(id, 2800, 2790) {
		t.Fatal("variação normal não é suspeita")
	}
	if !suspiciousMassDrop(id, 2800, 0) {
		t.Fatal("leitura vazia deve ser adiada na primeira vez")
	}
	if suspiciousMassDrop(id, 2800, 0) {
		t.Fatal("a segunda leitura seguida confirma a queda")
	}
	// uma leitura normal zera a contagem
	suspiciousMassDrop(id, 2800, 100)
	suspiciousMassDrop(id, 2800, 2790)
	if !suspiciousMassDrop(id, 2800, 100) {
		t.Fatal("depois de uma leitura normal, a próxima queda volta a ser adiada")
	}
	if suspiciousMassDrop(uuid.New(), 10, 0) {
		t.Fatal("BNG pequeno (<50 online) não entra na regra")
	}
}

func TestLooseLoginKeyEAliasIPv4(t *testing.T) {
	if looseLoginKey(" Fulano.Silva@g2.com.br\x00") != "fulano.silva" {
		t.Errorf("chave tolerante: %q", looseLoginKey(" Fulano.Silva@g2.com.br\x00"))
	}
	if ipv4AliasOfASCII4("1234") != "49.50.51.52" {
		t.Errorf("alias: %q", ipv4AliasOfASCII4("1234"))
	}
	if ipv4AliasOfASCII4("ab12") != "" || ipv4AliasOfASCII4("12345") != "" {
		t.Error("só 4 caracteres sem letras têm alias")
	}
}
