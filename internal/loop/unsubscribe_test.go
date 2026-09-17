package loop

import "testing"

func TestTokenRoundTripsAndRefusesTampering(t *testing.T) {
	key := []byte("secret")
	tok := token(key, "soccer-team", "alice@gmail.com")
	name, email, ok := parseToken(key, tok)
	if !ok || name != "soccer-team" || email != "alice@gmail.com" {
		t.Fatalf("parsed %q %q %v", name, email, ok)
	}
	if _, _, ok := parseToken([]byte("other"), tok); ok {
		t.Fatal("another key's token was taken")
	}
	forged := token(key, "soccer-team", "bob@gmail.com")
	if _, _, ok := parseToken(key, forged[:len(forged)-2]+"AA"); ok {
		t.Fatal("a tampered token was taken")
	}
	if _, _, ok := parseToken(key, "nonsense"); ok {
		t.Fatal("nonsense was taken")
	}
}
