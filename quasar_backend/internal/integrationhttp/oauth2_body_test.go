package integrationhttp

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildOAuth2PasswordFormEncoded(t *testing.T) {
	ac := AuthConfig{
		ClientID:     "6",
		ClientSecret: "secret&special",
		Username:     "api@test.com",
		Password:     "p@ss",
		GrantType:    "password",
	}
	enc := BuildOAuth2PasswordFormEncoded(ac)
	vals, err := url.ParseQuery(enc)
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("client_id") != "6" {
		t.Fatalf("client_id: %q", vals.Get("client_id"))
	}
	if vals.Get("client_secret") != "secret&special" {
		t.Fatalf("client_secret: %q", vals.Get("client_secret"))
	}
	if vals.Get("grant_type") != "password" {
		t.Fatalf("grant_type: %q", vals.Get("grant_type"))
	}
}

func TestOAuth2PasswordBodyDefaultsForm(t *testing.T) {
	_, bt := OAuth2PasswordBody(AuthConfig{ClientID: "1"})
	if bt != "form" {
		t.Fatalf("expected form, got %s", bt)
	}
}

func TestOAuth2PasswordBodyJSON(t *testing.T) {
	body, bt := OAuth2PasswordBody(AuthConfig{ClientID: "1", LoginBodyType: "json"})
	if bt != "json" {
		t.Fatalf("expected json, got %s", bt)
	}
	if !strings.Contains(body, `"client_id"`) {
		t.Fatalf("body: %s", body)
	}
}
