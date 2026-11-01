package agentrunner

import (
	"testing"
)

func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		secrets  []string
		expected string
	}{
		{
			name:     "explicit secrets",
			input:    "Connecting with secret-password-123 to database",
			secrets:  []string{"secret-password-123"},
			expected: "Connecting with [REDACTED] to database",
		},
		{
			name:     "short secret ignored",
			input:    "Normal text with hi inside",
			secrets:  []string{"hi"},
			expected: "Normal text with hi inside",
		},
		{
			name:     "github pat pattern",
			input:    "git clone https://ghp_0123456789abcdefghijklmnopqrstuv:x-oauth-basic@github.com",
			secrets:  nil,
			expected: "git clone https://[REDACTED]:x-oauth-basic@github.com",
		},
		{
			name:     "api key pattern",
			input:    "API_KEY=sk-abcdefghijklmnopqrstuvwxyz123",
			secrets:  nil,
			expected: "API_KEY=[REDACTED]",
		},
		{
			name:     "bearer token pattern",
			input:    "Authorization: Bearer mysecretbearer12345678",
			secrets:  nil,
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "empty input",
			input:    "",
			secrets:  []string{"secret"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskSecrets(tt.input, tt.secrets)
			if got != tt.expected {
				t.Errorf("MaskSecrets() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeEnv(t *testing.T) {
	env := map[string]string{
		"SECRET_KEY": "supersecretpassword",
		"NORMAL_KEY": "safevalue",
	}
	secrets := []string{"supersecretpassword"}

	sanitized := SanitizeEnv(env, secrets)
	if len(sanitized) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(sanitized))
	}

	for _, e := range sanitized {
		if e == "SECRET_KEY=supersecretpassword" {
			t.Errorf("secret was not masked in env: %s", e)
		}
	}
}
