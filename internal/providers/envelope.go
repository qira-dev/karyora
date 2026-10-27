package providers

import (
	derrs "github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// mapToEnvelope maps the neutral provider error taxonomy onto the CLOSED
// D-35 error register (C3-17: no new wire codes are invented; provider
// specifics travel inside the bounded data map). The retryable flag comes
// from the neutral kind.
func mapToEnvelope(pe *dprovider.ProviderError, correlationID string) (derrs.Envelope, error) {
	var code derrs.Code
	switch pe.Kind {
	case dprovider.ErrRateLimited:
		code = derrs.CodeRateLimited
	case dprovider.ErrAuth, dprovider.ErrPermission, dprovider.ErrEgressPolicy:
		code = derrs.CodeAuthzDenied
	case dprovider.ErrInvalidRequest, dprovider.ErrUnsupported:
		code = derrs.CodeValidationFailed
	case dprovider.ErrTimeout, dprovider.ErrUnreachable, dprovider.ErrMidStreamFailure,
		dprovider.ErrResponseTooLarge, dprovider.ErrTLS:
		code = derrs.CodeStorageUnavailable
	case dprovider.ErrCanceled:
		code = derrs.CodeConflictState
	case dprovider.ErrProtocol, dprovider.ErrUnknown:
		code = derrs.CodeInternal
	default:
		code = derrs.CodeInternal
	}
	data := map[string]any{
		"provider":  pe.Provider,
		"kind":      string(pe.Kind),
		"retryable": pe.Retryable(),
	}
	if pe.Status != 0 {
		data["provider_status"] = pe.Status
	}
	if pe.RetryAfter > 0 {
		data["retry_after_ms"] = pe.RetryAfter.Milliseconds()
	}
	return derrs.New(code, correlationID, data)
}
