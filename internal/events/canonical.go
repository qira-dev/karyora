package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalJSON renders v as the shared canonical JSON form (wire contract
// layer, C3-02): object keys sorted lexicographically at every level, no
// insignificant whitespace, numbers preserved as they decode (json.Number),
// UTF-8 strings. The same logical document always produces the same bytes
// regardless of struct field order or map iteration order. Every digest
// over a contract document (KnowledgePack roots, ContextSnapshot assembled
// representations, future Job receipts) is computed over this form, so the
// canonical authority lives here and nowhere else.
func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("events: canonical marshal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("events: canonical decode: %w", err)
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return fmt.Errorf("events: canonical key: %w", err)
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("events: canonical string: %w", err)
		}
		buf.Write(b)
	case json.Number:
		buf.WriteString(t.String())
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		return fmt.Errorf("events: canonical form of %T is undefined", v)
	}
	return nil
}
