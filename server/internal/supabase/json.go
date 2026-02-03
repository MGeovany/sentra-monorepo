package supabase

import "encoding/json"

func UnmarshalJSON(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// RawJSON allows embedding pre-validated JSON as a value in payload maps.
// It is used when the caller already has a JSON blob and wants to preserve its structure.
type RawJSON []byte

func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}

func MarshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
