package main

import (
	"encoding/json"
	"strings"
)

// coerceToList accepts a JSON array of strings, a JSON array of objects (each checked against
// objectKeys in order for a string value), a single string, or a comma/newline-separated string.
// Every alias field on every Actor built from this scaffold takes input this same flexible way;
// only the object key names differ (a URL-based Actor passes "url", a term-based one passes
// "term", "query", ...).
func coerceToList(v interface{}, objectKeys ...string) []string {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		return splitStringList(val)
	case []interface{}:
		var out []string
		for _, item := range val {
			switch it := item.(type) {
			case string:
				out = append(out, it)
			case map[string]interface{}:
				for _, key := range objectKeys {
					if s, ok := it[key].(string); ok {
						out = append(out, s)
						break
					}
				}
			}
		}
		return out
	case []string:
		return val
	default:
		// Might be a raw json.RawMessage-shaped value coming through re-marshal; try once more.
		b, err := json.Marshal(val)
		if err != nil {
			return nil
		}
		var arr []interface{}
		if err := json.Unmarshal(b, &arr); err == nil {
			return coerceToList(arr, objectKeys...)
		}
		var s string
		if err := json.Unmarshal(b, &s); err == nil {
			return splitStringList(s)
		}
		return nil
	}
}

func splitStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
