package tlsfingerprint

import "strings"

func SupportsHTTP2(profile *Profile) bool {
	if profile != nil && len(profile.Extensions) > 0 {
		found := false
		for _, id := range profile.Extensions {
			if id == 16 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	protocols := []string{"http/1.1"}
	if profile != nil && len(profile.ALPNProtocols) > 0 {
		protocols = profile.ALPNProtocols
	}
	for _, protocol := range protocols {
		if strings.EqualFold(strings.TrimSpace(protocol), "h2") {
			return true
		}
	}
	return false
}

// HTTP1OnlyProfile returns a copy suitable for WebSocket HTTP/1.1 Upgrade.
func HTTP1OnlyProfile(profile *Profile) *Profile {
	if profile == nil || !SupportsHTTP2(profile) {
		return profile
	}
	cloned := *profile
	protocols := make([]string, 0, len(profile.ALPNProtocols))
	hasHTTP1 := false
	for _, protocol := range profile.ALPNProtocols {
		trimmed := strings.TrimSpace(protocol)
		if trimmed == "" || strings.EqualFold(trimmed, "h2") {
			continue
		}
		if strings.EqualFold(trimmed, "http/1.1") {
			hasHTTP1 = true
		}
		protocols = append(protocols, protocol)
	}
	if !hasHTTP1 {
		protocols = append(protocols, "http/1.1")
	}
	cloned.ALPNProtocols = protocols
	return &cloned
}
