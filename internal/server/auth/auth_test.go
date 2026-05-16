package auth

import (
	"bytes"
	"testing"
)

func TestArgon2idHasherRoundTrip(t *testing.T) {
	hasher := Argon2idHasher{Memory: 8, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 16, Random: bytes.NewReader(make([]byte, 16))}
	encoded, err := hasher.Hash("password")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := hasher.Verify("password", encoded)
	if err != nil || !ok {
		t.Fatalf("verify failed: %v", err)
	}
	ok, err = hasher.Verify("wrong", encoded)
	if err != nil || ok {
		t.Fatalf("wrong password accepted")
	}
}

func TestTokenGenerator(t *testing.T) {
	generator := TokenGenerator{Length: 32, Random: bytes.NewReader(make([]byte, 32))}
	token, hash, err := generator.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || !bytes.Equal(hash, HashToken(token)) {
		t.Fatal("invalid generated token")
	}
}

func TestIDGenerator(t *testing.T) {
	id, err := (IDGenerator{Random: bytes.NewReader(make([]byte, 16))}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "00000000-0000-4000-8000-000000000000" {
		t.Fatalf("unexpected UUID: %s", id)
	}
}

func TestArgon2idHasherRejectsUnsafeStoredParameters(t *testing.T) {
	hasher := NewArgon2idHasher()
	encoded := "$argon2id$v=19$m=1048577,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAA"

	if _, err := hasher.Verify("password", encoded); err == nil {
		t.Fatal("Verify() accepted excessive Argon2id memory parameters")
	}
}
