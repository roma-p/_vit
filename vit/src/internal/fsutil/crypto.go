package fsutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func GenerateUID(byteLength int) string {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("FATAL: system entropy unavailable: %v - cannot generate secure UUIDs", err))
	}
	return hex.EncodeToString(bytes)
}

func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
