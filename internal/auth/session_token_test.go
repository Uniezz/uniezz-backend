package auth

import (
	"bytes"
	"errors"
	"testing"
)

func TestSessionToken_Lifecycle(t *testing.T) {
	tok := GenerateSessionToken()

	cookieVal := tok.String()
	if len(cookieVal) == 0 {
		t.Fatal("expected non-empty cookie value")
	}

	for i := 0; i < len(cookieVal); i++ {
		if cookieVal[i] == '=' || cookieVal[i] == '+' || cookieVal[i] == '/' {
			t.Fatalf("cookie string contains forbidden character: %c", cookieVal[i])
		}
	}

	parsedTok, err := ParseSessionToken(cookieVal)
	if err != nil {
		t.Fatalf("failed to parse valid cookie string: %v", err)
	}

	origHash := tok.Hash()
	parsedHash := parsedTok.Hash()

	if !bytes.Equal(origHash[:], parsedHash[:]) {
		t.Fatalf("hash mismatch:\nexpected %x\ngot      %x", origHash, parsedHash)
	}

	directHash, err := HashTokenString(cookieVal)
	if err != nil {
		t.Fatalf("unexpected error from HashTokenString: %v", err)
	}
	if !bytes.Equal(origHash[:], directHash[:]) {
		t.Fatalf("HashTokenString returned different hash")
	}
}

func TestSessionToken_InvalidInputs(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "invalid base64 characters", input: "not-a-valid-base-64!!@@##"},
		{name: "too short payload", input: "dGVzdA"},
		{name: "too long payload", input: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		{name: "tampered payload with valid base64", input: "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3OA"}, // 35 bytes
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSessionToken(tc.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
			if !errors.Is(err, ErrInvalidTokenString) {
				t.Fatalf("expected ErrInvalidTokenString, got: %v", err)
			}
		})
	}
}
