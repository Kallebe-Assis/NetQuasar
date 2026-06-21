package monitorworker

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateOltCollectReady_missingIP(t *testing.T) {
	def := "public"
	r := validateOltCollectReady(nil, nil, pingableDeviceRow{
		id: uuid.New(), brand: "VSOL", model: "V1600G1",
	}, &def)
	if r.Ready || r.Reason == "" {
		t.Fatalf("expected not ready, got %+v", r)
	}
}

func TestValidateOltCollectReady_missingCommunity(t *testing.T) {
	r := validateOltCollectReady(nil, nil, pingableDeviceRow{
		ip: "10.0.0.1", brand: "VSOL", model: "V1600G1",
	}, nil)
	if r.Ready {
		t.Fatal("expected missing community")
	}
}

func TestValidateOltCollectReady_missingBrand(t *testing.T) {
	def := "public"
	r := validateOltCollectReady(nil, nil, pingableDeviceRow{
		ip: "10.0.0.1", model: "V1600G1",
	}, &def)
	if r.Ready {
		t.Fatal("expected missing brand")
	}
}
