package opencode

import "strings"

const defaultDataDir = "data/opencode"

// ResolveDataDir applies explicit value, environment value, then the default.
func ResolveDataDir(value, envValue string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	if envValue = strings.TrimSpace(envValue); envValue != "" {
		return envValue
	}
	return defaultDataDir
}
