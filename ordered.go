package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OrderedMap is a JSON object that preserves the key order of the source file.
//
// encoding/json marshals Go maps with their keys sorted alphabetically, which
// would reshuffle every locale file and produce noisy git diffs. We parse with
// the streaming decoder so the original order is retained on the way out.
type OrderedMap struct {
	keys   []string
	values map[string]any // value is *OrderedMap | string | json.Number | bool | nil | []any
}

func NewOrderedMap() *OrderedMap {
	return &OrderedMap{values: make(map[string]any)}
}

func (m *OrderedMap) Get(key string) (any, bool) {
	v, ok := m.values[key]
	return v, ok
}

// Set updates a value, appending the key only the first time it is seen so the
// original ordering is preserved across overwrites.
func (m *OrderedMap) Set(key string, value any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *OrderedMap) Keys() []string { return m.keys }

func (m *OrderedMap) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // keep integers as integers instead of float64

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected a JSON object at the top level, got %v", tok)
	}
	return m.parseObject(dec)
}

// parseObject consumes the body of an object up to and including its closing
// brace. The opening brace must already have been read by the caller.
func (m *OrderedMap) parseObject(dec *json.Decoder) error {
	if m.values == nil {
		m.values = make(map[string]any)
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected a string key, got %T", keyTok)
		}
		val, err := parseValue(dec)
		if err != nil {
			return err
		}
		m.Set(key, val)
	}
	_, err := dec.Token() // consume closing '}'
	return err
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // string, json.Number, bool, or nil
	}
	switch delim {
	case '{':
		sub := NewOrderedMap()
		if err := sub.parseObject(dec); err != nil {
			return nil, err
		}
		return sub, nil
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token() // consume closing ']'
		return arr, err
	default:
		return nil, fmt.Errorf("unexpected delimiter %v", delim)
	}
}

func (m *OrderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.values[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
