package memory

import (
	"regexp"
	"strings"
)

// THE SECRETS LAW (BINDING, MB §7.5; MB §12.2 law 3): passwords, API
// keys, private keys, payment credentials, and other secrets are NEVER
// stored in Memory — secret-manager/key-store references only; a write
// attempting secret storage is refused (MEMORY_SECRETS_REJECTED), and the
// law extends to exports, logs, URLs, and error messages.
//
// ContainsSecretMaterial is the served refusal scanner (this
// implementation's typing of the law — Proposed pending the recorded owner
// review, the B1 served-registry-bounds precedent): it flags EXPLICIT
// secret-shaped material — PEM key blocks, credential assignment patterns
// (password=/api_key=/secret=/token=<value>), bearer-credential headers,
// and the well-known credential token prefixes (AWS access keys, GitHub,
// Slack, OpenAI-style, GitLab tokens). It is a refusal heuristic, never a
// completeness claim: a value the scanner misses is not permission to
// store, and no success path is fabricated for secret material. A
// secret-manager/key-store REFERENCE (the law's allowed form) carries none
// of these shapes and passes.
func ContainsSecretMaterial(s string) bool {
	if s == "" {
		return false
	}
	if secretKeyBlockPattern.MatchString(s) {
		return true
	}
	if secretAssignPattern.MatchString(s) {
		return true
	}
	if secretBearerPattern.MatchString(s) {
		return true
	}
	for _, p := range secretTokenPrefixes {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// secretKeyBlockPattern matches PEM key/certificate blocks (a private key
// block is the unambiguous secret-material shape).
var secretKeyBlockPattern = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*(PRIVATE KEY|CERTIFICATE|EC PARAMETERS|OPENSSH PRIVATE KEY|PGP PRIVATE KEY BLOCK|ENCRYPTED PRIVATE KEY)-----`)

// secretAssignPattern matches credential assignment/declaration shapes:
// password=hunter2, "api_key": "sk-...", secret: xxx, access_token=xxx,
// private_key: xxx, auth_token=xxx — the label immediately followed by an
// assignment separator and a non-empty value. Prose about a credential
// ("the password field is missing") carries no separator-value shape and
// passes.
var secretAssignPattern = regexp.MustCompile(`(?i)(password|passwd|api[_-]?key|apikey|secret|access[_-]?token|auth[_-]?token|private[_-]?key|client[_-]?secret|card[_-]?number|credit[_-]?card)["']?\s*[:=]\s*["']?[^\s"']{4,}`)

// secretBearerPattern matches credential-bearing authorization headers.
var secretBearerPattern = regexp.MustCompile(`(?i)authorization["']?\s*[:=]\s*["']?(bearer|basic|token)\s+[^\s"']{8,}`)

// secretTokenPrefixes are the well-known credential token prefixes (the
// value shapes; presence of the shape is the refusal — never an
// identification claim about the issuing service).
var secretTokenPrefixes = []string{
	"AKIA",        // AWS access key id shape
	"ghp_",        // GitHub personal access token shape
	"github_pat_", // GitHub fine-grained token shape
	"xoxb-",       // Slack bot token shape
	"xoxp-",       // Slack user token shape
	"glpat-",      // GitLab personal access token shape
	"sk-ant-",     // Anthropic key shape
	"-----BEGIN",  // PEM block opening (any flavor)
}
