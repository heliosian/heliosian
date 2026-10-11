package db

import "testing"

func TestTokenRoundTripsAndRefusesTampering(t *testing.T) {
	key := []byte("secret")
	tok := unsubscribeToken(key, "grp00000000030", "alice@gmail.com")
	list, email, ok := parseToken(key, tok)
	if !ok || list != "grp00000000030" || email != "alice@gmail.com" {
		t.Fatalf("parsed %q %q %v", list, email, ok)
	}
	if _, _, ok := parseToken([]byte("other"), tok); ok {
		t.Fatal("another key's token was taken")
	}
	forged := unsubscribeToken(key, "grp00000000030", "bob@gmail.com")
	if _, _, ok := parseToken(key, forged[:len(forged)-2]+"AA"); ok {
		t.Fatal("a tampered token was taken")
	}
	if _, _, ok := parseToken(key, "nonsense"); ok {
		t.Fatal("nonsense was taken")
	}
}
