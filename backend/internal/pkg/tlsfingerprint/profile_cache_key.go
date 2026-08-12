package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CacheKey isolates transports when any fingerprint parameter changes.
func CacheKey(profile *Profile) string {
	if profile == nil {
		return "none"
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return "marshal-error"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
