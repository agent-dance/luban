package interaction

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

func newRequestID(prefix string) string {
	if prefix == "" {
		prefix = "req"
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(random[:])
}
