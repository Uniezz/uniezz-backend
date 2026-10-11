package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const tokenByteLength = 32

type SessionToken struct {
	token []byte
}

func GenerateSessionToken() *SessionToken {
	b := make([]byte, tokenByteLength)
	rand.Read(b)
	return &SessionToken{token: b}
}

func (t *SessionToken) String() string {
	return base64.RawURLEncoding.EncodeToString(t.token)
}

func (t *SessionToken) Hash() [32]byte {
	return sha256.Sum256(t.token)
}

func ParseSessionToken(raw string) (*SessionToken, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidTokenString
	}

	if len(decoded) != tokenByteLength {
		return nil, ErrInvalidTokenString
	}

	return &SessionToken{token: decoded}, nil
}

func HashTokenString(raw string) ([32]byte, error) {
	tok, err := ParseSessionToken(raw)
	if err != nil {
		return [32]byte{}, err
	}
	return tok.Hash(), nil
}
