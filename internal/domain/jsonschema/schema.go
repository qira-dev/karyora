// Package jsonschema is the bounded JSON Schema validator used where the
// Core must validate generated provider output against the selected output
// schema BEFORE a result commit (C3-20 acceptance), and where callers
// validate input documents. It supports the structural subset the product
// needs — type, properties, required, additionalProperties, items, enum,
// minItems/maxItems, minLength/maxLength, minimum/maximum — with unknown
// keywords ignored per JSON Schema semantics. It is a Phase 3 bounded
// subset, never a full draft-2020-12 claim; unsupported assertions are
// simply not enforced, which the harness records honestly.
package jsonschema

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// MaxDepth bounds schema recursion; deeper documents fail closed rather
// than risk unbounded work.
const MaxDepth = 64

// Validate checks data (a JSON document) against schema (a JSON Schema
// document). A schema that is not valid JSON, or a document that violates
// the enforced subset, returns an error naming the violation path.
func Validate(data []byte, schema json.RawMessage) error {
	if len(schema) == 0 {
		return fmt.Errorf("jsonschema: no schema supplied")
	}
	var root any
	if err := json.Unmarshal(schema, &root); err != nil {
		return fmt.Errorf("jsonschema: schema is not valid JSON: %w", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("jsonschema: document is not valid JSON: %w", err)
	}
	return validate(root, doc, "$", 0)
}

func validate(schema, doc any, path string, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("jsonschema: schema depth exceeds the bound at %s", path)
	}
	obj, ok := schema.(map[string]any)
	if !ok {
		// boolean schema or non-object schema: true = anything, false = nothing
		if b, isBool := schema.(bool); isBool {
			if b {
				return nil
			}
			return fmt.Errorf("jsonschema: %s is disallowed by the false schema", path)
		}
		return nil
	}

	if t, ok := obj["type"]; ok {
		if err := checkType(t, doc, path); err != nil {
			return err
		}
	}
	if enum, ok := obj["enum"].([]any); ok && len(enum) > 0 {
		if !containsEqual(enum, doc) {
			return fmt.Errorf("jsonschema: %s is not one of the allowed enum values", path)
		}
	}

	switch d := doc.(type) {
	case map[string]any:
		if err := validateObject(obj, d, path, depth); err != nil {
			return err
		}
	case []any:
		return validateArray(obj, d, path, depth)
	case string:
		if min, ok := numField(obj, "minLength"); ok && float64(len([]rune(d))) < min {
			return fmt.Errorf("jsonschema: %s is shorter than minLength", path)
		}
		if max, ok := numField(obj, "maxLength"); ok && float64(len([]rune(d))) > max {
			return fmt.Errorf("jsonschema: %s is longer than maxLength", path)
		}
	case float64:
		if min, ok := numField(obj, "minimum"); ok && d < min {
			return fmt.Errorf("jsonschema: %s is below minimum", path)
		}
		if max, ok := numField(obj, "maximum"); ok && d > max {
			return fmt.Errorf("jsonschema: %s is above maximum", path)
		}
	}
	return nil
}

func validateObject(schema map[string]any, doc map[string]any, path string, depth int) error {
	props, _ := schema["properties"].(map[string]any)
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			name, isStr := r.(string)
			if !isStr {
				continue
			}
			if _, present := doc[name]; !present {
				return fmt.Errorf("jsonschema: %s is missing required property %q", path, name)
			}
		}
	}
	additional := true
	if ap, ok := schema["additionalProperties"]; ok {
		if b, isBool := ap.(bool); isBool {
			additional = b
		}
	}
	for name, value := range doc {
		if propSchema, ok := props[name]; ok {
			if err := validate(propSchema, value, path+"."+name, depth+1); err != nil {
				return err
			}
			continue
		}
		if !additional {
			return fmt.Errorf("jsonschema: %s has property %q not allowed by additionalProperties:false", path, name)
		}
	}
	return nil
}

func validateArray(schema map[string]any, doc []any, path string, depth int) error {
	if min, ok := numField(schema, "minItems"); ok && float64(len(doc)) < min {
		return fmt.Errorf("jsonschema: %s has fewer than minItems", path)
	}
	if max, ok := numField(schema, "maxItems"); ok && float64(len(doc)) > max {
		return fmt.Errorf("jsonschema: %s has more than maxItems", path)
	}
	if items, ok := schema["items"]; ok {
		for i, item := range doc {
			if err := validate(items, item, fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkType(t any, doc any, path string) error {
	ts, ok := t.(string)
	if !ok {
		// type arrays are not part of the enforced subset
		return nil
	}
	switch ts {
	case "object":
		if _, ok := doc.(map[string]any); !ok {
			return fmt.Errorf("jsonschema: %s must be an object", path)
		}
	case "array":
		if _, ok := doc.([]any); !ok {
			return fmt.Errorf("jsonschema: %s must be an array", path)
		}
	case "string":
		if _, ok := doc.(string); !ok {
			return fmt.Errorf("jsonschema: %s must be a string", path)
		}
	case "integer":
		f, ok := doc.(float64)
		if !ok || f != math.Trunc(f) {
			return fmt.Errorf("jsonschema: %s must be an integer", path)
		}
	case "number":
		if _, ok := doc.(float64); !ok {
			return fmt.Errorf("jsonschema: %s must be a number", path)
		}
	case "boolean":
		if _, ok := doc.(bool); !ok {
			return fmt.Errorf("jsonschema: %s must be a boolean", path)
		}
	case "null":
		if doc != nil {
			return fmt.Errorf("jsonschema: %s must be null", path)
		}
	default:
		return fmt.Errorf("jsonschema: unknown type keyword %q at %s", ts, path)
	}
	return nil
}

func numField(obj map[string]any, key string) (float64, bool) {
	v, ok := obj[key].(float64)
	return v, ok
}

func containsEqual(list []any, v any) bool {
	for _, item := range list {
		if equalJSON(item, v) {
			return true
		}
	}
	return false
}

// equalJSON compares two decoded JSON values by their canonical encoding —
// numerics compare by value (1 == 1.0), strings byte-for-byte.
func equalJSON(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.Compare(string(ab), string(bb)) == 0
}
