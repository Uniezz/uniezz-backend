package auth

import (
	"bytes"
	"testing"
)

func TestGenerateOTPCode_SixDigits(t *testing.T) {
	for range 1000 {
		code := generateOTPCode()
		if len(code) != 6 {
			t.Fatalf("expected 6 characters, got %q", code)
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatalf("expected only digits, got %q", code)
			}
		}
	}
}

func TestGenerateOTPCode_Varies(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		seen[generateOTPCode()] = true
	}
	// 100 draws from a million values: anything below ~95 distinct means the source is broken.
	if len(seen) < 95 {
		t.Fatalf("expected codes to vary, got only %d distinct out of 100", len(seen))
	}
}

func TestOTPHasher_DependsOnCode(t *testing.T) {
	h := newOTPHasher([]byte("test-key"))
	if bytes.Equal(h.hash("123456"), h.hash("123457")) {
		t.Fatal("expected different codes to produce different hashes")
	}
}

func TestOTPHasher_DependsOnKey(t *testing.T) {
	a := newOTPHasher([]byte("key-a")).hash("123456")
	b := newOTPHasher([]byte("key-b")).hash("123456")

	if bytes.Equal(a, b) {
		t.Fatal("expected different keys to produce different hashes")
	}
}

func TestOTPHasher_IsDeterministic(t *testing.T) {
	h := newOTPHasher([]byte("test-key"))
	if !bytes.Equal(h.hash("000042"), h.hash("000042")) {
		t.Fatal("expected the same code and key to hash identically")
	}
}
