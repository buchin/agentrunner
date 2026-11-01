package agentrunner

import (
	"regexp"
	"strings"
)

var (
	githubPATRegex = regexp.MustCompile(`(ghp_[a-zA-Z0-9]{20,}|github_pat_[a-zA-Z0-9_]{20,})`)
	bearerRegex    = regexp.MustCompile(`(?i)(Bearer\s+)[a-zA-Z0-9_\-\.]{12,}`)
	dbURIRegex     = regexp.MustCompile(`(postgres(?:ql)?:\/\/[^:]+:)([^@]+)(@)`)
	passwordRegex  = regexp.MustCompile(`(?i)(password|secret|token|api_key)\s*[:=]\s*["']?([^\s"']{6,})["']?`)
	apiKeyRegex    = regexp.MustCompile(`(sk-[a-zA-Z0-9_\-]{15,}|AIzaSy[a-zA-Z0-9_\-]{30,}|xai-[a-zA-Z0-9_\-]{20,}|gsk_[a-zA-Z0-9_\-]{20,})`)
)

// MaskSecrets replaces occurrences of secret values and known credential patterns with [REDACTED].
// Ignores empty or short strings (less than 4 chars) to prevent over-masking common words.
func MaskSecrets(input string, secrets []string) string {
	if input == "" {
		return ""
	}

	result := input

	// 1. Mask explicit secret values
	for _, sec := range secrets {
		cleaned := strings.TrimSpace(sec)
		if len(cleaned) < 4 {
			continue
		}
		result = strings.ReplaceAll(result, cleaned, "[REDACTED]")
	}

	// 2. Mask known credential patterns
	result = githubPATRegex.ReplaceAllString(result, "[REDACTED]")
	result = apiKeyRegex.ReplaceAllString(result, "[REDACTED]")
	result = bearerRegex.ReplaceAllString(result, "${1}[REDACTED]")
	result = dbURIRegex.ReplaceAllString(result, "${1}[REDACTED]${3}")
	result = passwordRegex.ReplaceAllString(result, "${1}=[REDACTED]")

	return result
}

// SanitizeEnv returns a slice of environment variables in KEY=VALUE format,
// ensuring system secrets are not exposed to untrusted logging.
func SanitizeEnv(env map[string]string, secrets []string) []string {
	var list []string
	for k, v := range env {
		safeVal := MaskSecrets(v, secrets)
		list = append(list, k+"="+safeVal)
	}
	return list
}
