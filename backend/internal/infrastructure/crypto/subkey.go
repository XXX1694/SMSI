package crypto

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Subkey derives a 32-byte key for one purpose from the base64 master key (HKDF-SHA256, `info` names the purpose),
// so that a value keyed with it cannot be confused with, or used to attack, the data encryption itself.
func Subkey(masterBase64, info string) ([]byte, error) {
	master, err := base64.StdEncoding.DecodeString(strings.TrimSpace(masterBase64))
	if err != nil || len(master) != 32 {
		return nil, fmt.Errorf("crypto: master key must be base64 of 32 bytes")
	}
	return hkdf.Key(sha256.New, master, nil, info, 32)
}
