package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
)

var otpCodeSpace = big.NewInt(1_000_000)

func generateOTPCode() string {
	n, _ := rand.Int(rand.Reader, otpCodeSpace)
	return fmt.Sprintf("%06d", n.Int64())
}

type otpHasher struct {
	key []byte
}

func newOTPHasher(key []byte) otpHasher {
	return otpHasher{key: key}
}

func (h otpHasher) hash(code string) []byte {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (h otpHasher) matches(code string, stored []byte) bool {
	return hmac.Equal(h.hash(code), stored)
}
