package credentialvault

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := DeriveKey([]byte("test-session-secret"))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("s3nh@-super-secreta")
	blob, err := Encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(key, blob)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("got %q", got)
	}
	if _, err := Decrypt(key, blob[:4]); err == nil {
		t.Fatal("expected bad blob")
	}
}
