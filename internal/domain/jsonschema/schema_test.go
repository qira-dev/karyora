package jsonschema

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustValidate(t *testing.T, data string, schema string) error {
	t.Helper()
	return Validate([]byte(data), json.RawMessage(schema))
}

func TestObjectValidation(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"integer"}},"required":["a"],"additionalProperties":false}`
	if err := mustValidate(t, `{"a":"x","b":3}`, schema); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	cases := []string{
		`{}`,                 // missing required
		`{"a":1}`,            // wrong type
		`{"a":"x","c":true}`, // additionalProperties
		`{"a":"x","b":3.5}`,  // integer violation
		`[]`,                 // not an object
	}
	for _, doc := range cases {
		if err := mustValidate(t, doc, schema); err == nil {
			t.Errorf("document %s must fail", doc)
		}
	}
}

func TestArrayEnumStringBounds(t *testing.T) {
	schema := `{"type":"array","items":{"type":"string","enum":["x","y"],"minLength":1},"minItems":1,"maxItems":2}`
	if err := mustValidate(t, `["x","y"]`, schema); err != nil {
		t.Fatalf("valid array rejected: %v", err)
	}
	if err := mustValidate(t, `["z"]`, schema); err == nil {
		t.Fatal("enum violation accepted")
	}
	if err := mustValidate(t, `["a","b","c"]`, schema); err == nil {
		t.Fatal("maxItems violation accepted")
	}
	if err := mustValidate(t, `[]`, schema); err == nil {
		t.Fatal("minItems violation accepted")
	}
}

func TestNumberBoundsAndDepth(t *testing.T) {
	if err := mustValidate(t, `5`, `{"type":"number","minimum":1,"maximum":10}`); err != nil {
		t.Fatalf("valid number rejected: %v", err)
	}
	if err := mustValidate(t, `0`, `{"type":"number","minimum":1}`); err == nil {
		t.Fatal("minimum violation accepted")
	}
	if err := mustValidate(t, `11`, `{"type":"number","maximum":10}`); err == nil {
		t.Fatal("maximum violation accepted")
	}
}

func TestFailClosedOnMissingOrBrokenSchema(t *testing.T) {
	if err := Validate([]byte(`{}`), nil); err == nil {
		t.Fatal("missing schema must fail closed")
	}
	if err := Validate([]byte(`{}`), json.RawMessage(`{broken`)); err == nil {
		t.Fatal("broken schema must fail")
	}
	if err := Validate([]byte(`not-json`), json.RawMessage(`{"type":"object"}`)); err == nil {
		t.Fatal("invalid document must fail")
	}
}

func TestUnknownKeywordsIgnored(t *testing.T) {
	// JSON Schema semantics: unknown keywords do not constrain.
	if err := mustValidate(t, `{"a":1}`, `{"type":"object","format":"uuid","x-vendor":true,"properties":{"a":{"type":"integer"}},"required":["a"]}`); err != nil {
		t.Fatalf("unknown keywords must be ignored: %v", err)
	}
}

func TestErrorPathsAreReported(t *testing.T) {
	err := mustValidate(t, `{"a":{"b":2}}`, `{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"string"}},"required":["b"]}},"required":["a"]}`)
	if err == nil || !strings.Contains(err.Error(), "a.b") {
		t.Fatalf("expected a named violation path, got %v", err)
	}
}
