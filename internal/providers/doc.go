// Package providers owns the provider-neutral capability model and the
// adapter surface for the approved provider set and custom OpenAI- or
// Anthropic-compatible endpoints. It depends inward on domain and security
// ports; provider SDK specifics stay behind adapters, credentials stay
// behind the secret boundary (BYOK, D-17), and unknown capabilities fail
// closed.
package providers
