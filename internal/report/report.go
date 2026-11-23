// Package report turns a Stageproof JSON report into the committed index envelope.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Normalize decodes a Stageproof report and removes the two fields that change
// between identical runs: timestamp and duration_ms.
func Normalize(buf []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("report is not json: %w", err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("report is not a json object")
	}
	delete(obj, "timestamp")
	delete(obj, "duration_ms")
	return obj, nil
}

// Verdict reads the top-level verdict.
func Verdict(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := obj["verdict"].(string)
	return s
}

// BlockingRules returns blocker findings that are not ignored, in report order.
func BlockingRules(v any) []string {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := obj["findings"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range raw {
		f, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if f["severity"] != "blocker" {
			continue
		}
		if ignored, ok := f["ignored"].(bool); ok && ignored {
			continue
		}
		id, _ := f["rule_id"].(string)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// SameBlockingRules reports whether two rule lists are identical.
func SameBlockingRules(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Marshal writes JSON with sorted object keys and a trailing newline.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MinorKeys returns the k8s minor keys of an envelope in sorted order.
func MinorKeys(results map[string]any) []string {
	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
