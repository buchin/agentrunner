package agentrunner

import (
	"os"
	"strings"
	"sync"
)

const DefaultMaxOutputBytes = 1 << 20
const MaxOutputBytes = 16 << 20

type boundedOutput struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := b.limit - len(b.data)
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil // Always drain the child pipe.
}
func (b *boundedOutput) sanitized(secrets []string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	raw := string(b.data)
	if b.truncated {
		trim := 0
		for _, secret := range secrets {
			if n := partialSecretSuffix(raw, secret); n > trim {
				trim = n
			}
		}
		if trim > 0 {
			raw = raw[:len(raw)-trim] + "[REDACTED]"
		}
	}
	safe := MaskSecrets(raw, secrets)
	truncated := b.truncated
	if len(safe) > b.limit {
		safe = safe[:b.limit]
		truncated = true
	}
	return safe, truncated
}

// KMP keeps redaction at a truncated capture boundary linear in input size.
func partialSecretSuffix(raw, secret string) int {
	if secret == "" {
		return 0
	}
	table := make([]int, len(secret))
	for i, j := 1, 0; i < len(secret); i++ {
		for j > 0 && secret[i] != secret[j] {
			j = table[j-1]
		}
		if secret[i] == secret[j] {
			j++
		}
		table[i] = j
	}
	matched := 0
	for i := 0; i < len(raw); i++ {
		for matched > 0 && (matched == len(secret) || raw[i] != secret[matched]) {
			matched = table[matched-1]
		}
		if raw[i] == secret[matched] {
			matched++
		}
	}
	if matched == len(secret) {
		return 0
	}
	return matched
}
func executionEnvironment(extra map[string]string) []string {
	// Inherit only non-secret runtime essentials; never HOME, credential paths,
	// provider tokens, SSH agents or unrelated application secrets.
	keys := []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "TERM"}
	env := make([]string, 0, len(keys)+len(extra))
	for _, key := range keys {
		if _, set := extra[key]; !set {
			if value, ok := os.LookupEnv(key); ok {
				env = append(env, key+"="+value)
			}
		}
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}
func validEnvironment(extra map[string]string) bool {
	for key, value := range extra {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}
