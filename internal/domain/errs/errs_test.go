package errs

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestRegisterMatchesSpecification pins the Phase 2 code register to the
// shared-contracts specification (C2-05): any semantic change must be a
// deliberate, recorded contract change.
func TestRegisterMatchesSpecification(t *testing.T) {
	want := map[Code]codeDefault{
		CodeValidationFailed:       {retryable: false, class: RetryNever},
		CodeConfigConflict:         {retryable: false, class: RetryNever},
		CodeAuthzDenied:            {retryable: false, class: RetryNever},
		CodeNotFound:               {retryable: false, class: RetryNever},
		CodeConflictState:          {retryable: false, class: RetryNever},
		CodeSingleInstanceConflict: {retryable: true, class: RetryBackoff},
		CodeStorageUnavailable:     {retryable: true, class: RetryBackoff},
		CodeMigrationFailed:        {retryable: true, class: RetryReconcile},
		CodeStateCorrupt:           {retryable: true, class: RetryReconcile},
		CodeRecoveryRequired:       {retryable: true, class: RetryReconcile},
		CodeRateLimited:            {retryable: true, class: RetryBackoff},
		CodeInternal:               {retryable: true, class: RetrySafe},
	}
	if len(register) != len(want) {
		t.Fatalf("register holds %d codes, specification lists %d", len(register), len(want))
	}
	for code, d := range want {
		got, ok := register[code]
		if !ok {
			t.Errorf("missing code %s", code)
			continue
		}
		if got != d {
			t.Errorf("%s = %+v, want %+v", code, got, d)
		}
	}
}

// TestNewEnvelopeGoldenJSON pins the wire form: exactly the owner-accepted
// field names (code, retryable, correlationId, data) with the retry class
// inside bounded data.
func TestNewEnvelopeGoldenJSON(t *testing.T) {
	env, err := New(CodeRateLimited, "c1", map[string]any{"scope": "writer"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"RATE_LIMITED","retryable":true,"correlationId":"c1","data":{"retryClass":"backoff","scope":"writer"}}`
	if got := string(b); got != want {
		t.Fatalf("golden JSON mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestNewFailsClosed(t *testing.T) {
	if _, err := New(Code("UNKNOWN"), "c1", nil); err == nil {
		t.Error("unknown code must fail")
	}
	if _, err := New(CodeInternal, "", nil); err == nil {
		t.Error("empty correlationId must fail")
	}
	if _, err := New(CodeInternal, "c1", map[string]any{"retryClass": RetryNever}); err == nil {
		t.Error("reserved data key must fail")
	}
	if _, err := New(CodeInternal, "c1", map[string]any{"v": strings.Repeat("x", MaxDataValueBytes+1)}); err == nil {
		t.Error("unbounded data value must fail")
	}
	tooMany := map[string]any{}
	for i := 0; i < MaxDataKeys; i++ {
		tooMany[fmt.Sprintf("k%d", i)] = i
	}
	// MaxDataKeys caller keys + the injected retryClass exceed the bound.
	if _, err := New(CodeInternal, "c1", tooMany); err == nil {
		t.Error("too many data keys must fail")
	}
}

func TestNewCorrelationID(t *testing.T) {
	a, b := NewCorrelationID(), NewCorrelationID()
	if a == "" || b == "" || a == b {
		t.Fatalf("correlation IDs must be non-empty and unique: %q %q", a, b)
	}
}
