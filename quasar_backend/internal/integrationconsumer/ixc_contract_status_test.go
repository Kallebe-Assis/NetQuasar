package integrationconsumer

import (
	"strings"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func TestPickIXCStatusInternet_neverUsesStatus(t *testing.T) {
	row := map[string]any{
		"status":          "Ativo",
		"status_internet": "A",
	}
	if got := pickIXCStatusInternet(row); got != "A" {
		t.Fatalf("got %q, want A", got)
	}
	row2 := map[string]any{"status": "A"}
	if got := pickIXCStatusInternet(row2); got != "" {
		t.Fatalf("status field must not map to status_internet, got %q", got)
	}
}

func TestMergeServiceSummary_keepsContractStatus(t *testing.T) {
	login := ServiceSummary{Login: "user", ID: "10"}
	contrato := ServiceSummary{Login: "user", ID: "10", StatusInternet: "A", StatusLabel: "Ativo"}
	merged := mergeServiceSummary(login, contrato)
	if merged.StatusInternet != "A" {
		t.Fatalf("StatusInternet=%q", merged.StatusInternet)
	}
}

func TestPrepareIXCClienteContratoList_path(t *testing.T) {
	rc := integrationhttp.RequestConfig{Path: "/radusuarios", Method: "GET"}
	out := prepareIXCClienteContratoList(rc, ClientSearchConfig{})
	if out.Path != "/cliente_contrato" {
		t.Fatalf("path=%q", out.Path)
	}
}

func TestApplyIXCClienteContratoBodySearchAttempt(t *testing.T) {
	body := ApplyIXCClienteContratoBodySearchAttempt("", IXCSearchAttempt{
		Qtype: "cliente_contrato.id", Query: "19588", Oper: "=",
	})
	if !strings.Contains(body, `"query":"19588"`) || !strings.Contains(body, "cliente_contrato.id") {
		t.Fatalf("body=%s", body)
	}
}

func TestContractIDsFromServices(t *testing.T) {
	ids := contractIDsFromServices([]ServiceSummary{
		{Login: "u1", ContratoID: "19588"},
		{Login: "u2", ContratoID: "19588"},
	})
	if len(ids) != 1 || ids[0] != "19588" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestContractStatusForService_byContratoID(t *testing.T) {
	s := ServiceSummary{Login: "u", Contrato: "500 MEGA", ContratoID: "19588"}
	idx := map[string]string{"19588": "A"}
	if got := contractStatusForService(s, idx); got != "A" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyContractStatusIndex(t *testing.T) {
	services := []ServiceSummary{{Login: "u", ContratoID: "55"}}
	idx := map[string]string{"55": "A"}
	out := ApplyContractStatusIndex(services, idx)
	if out[0].StatusInternet != "A" || out[0].StatusLabel != "Ativo" {
		t.Fatalf("got %+v", out[0])
	}
}
