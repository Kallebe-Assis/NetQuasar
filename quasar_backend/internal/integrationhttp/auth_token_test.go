package integrationhttp

import "testing"

func TestBasicCredentialForHeader_encodeOff(t *testing.T) {
	raw := "2:abc123"
	if BasicCredentialForHeader(raw, false) != raw {
		t.Fatal("should not encode when disabled")
	}
}

func TestBasicCredentialForHeader_encodeOn(t *testing.T) {
	raw := "2:abc123"
	got := BasicCredentialForHeader(raw, true)
	if got != "MjphYmMxMjM=" {
		t.Fatalf("got %q", got)
	}
}

func TestBearerAuthorizationValue_encodeOptIn(t *testing.T) {
	if BearerAuthorizationValue("2:test", "Basic", false) != "Basic 2:test" {
		t.Fatal("should not encode without flag")
	}
	if BearerAuthorizationValue("2:test", "Basic", true) != "Basic Mjp0ZXN0" {
		t.Fatal("should encode when flag set")
	}
}
