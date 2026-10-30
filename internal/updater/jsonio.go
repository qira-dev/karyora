package updater

import (
	"bytes"
	"encoding/json"
)

// jsonMarshal / jsonUnmarshalStrict: the store's deterministic-JSON helpers.
// Marshal forbids HTML escaping ambiguity by using the standard encoder with
// SetEscapeHTML(false) so the durable records are stable byte-for-byte;
// Unmarshal uses DisallowUnknownFields so a record written by a NEWER schema
// version is refused (fail-closed) instead of silently half-understood —
// schema drift on the durable state is a typed failure (the machine never
// invents state), and the version fields above make the refusal explicit.
func jsonMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jsonUnmarshalStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
