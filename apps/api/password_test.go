package main

import "testing"

func TestHashPassword(t *testing.T) {
	const password = "example-password-for-learning"

	first, err := hashPassword(password)
	if err != nil {
		t.Fatalf("first hash failed: %v", err)
	}

	second, err := hashPassword(password)
	if err != nil {
		t.Fatalf("second hash failed: %v", err)
	}

	if first == "" || second == "" {
		t.Fatal("password hash must not be empty")
	}

	if first == second {
		t.Fatal("hashing twice should produce different salted hashes")
	}
}
