package crypto

import (
	"crypto/rand"
	"encoding/hex"
)

func NewRandomHex(n int) (string, error) {
	b := make([]byte, n)
	offset := 0
	for offset < n {
		var (
			nread int
			err   error
		)
		if nread, err = rand.Read(b[offset:]); err != nil {
			return "", err
		}
		offset = offset + nread
	}
	return hex.EncodeToString(b), nil
}
