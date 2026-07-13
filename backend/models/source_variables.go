package models

import (
	"encoding/json"
	"errors"
	"strings"
)

// These limits deliberately match the bounded parser runtime. Persistent
// values must never turn a restart, restore, or source switch into a way to
// bypass the request-time safety limits.
const (
	MaxSourceRuleVariables       = 32
	MaxSourceRuleVariableKeySize = 128
	MaxSourceRuleVariableValue   = 4096
	MaxSourceRuleVariableBytes   = 16 * 1024
)

var ErrInvalidSourceRuleVariables = errors.New("invalid source rule variables")

// NormalizeSourceRuleVariables accepts only the reader-dev-compatible JSON
// string map used by Book.variable and BookChapter.variable. It returns a
// deterministic compact representation, or the empty string for an empty map.
// Values are intentionally data only: selectors, JavaScript, headers, and
// filesystem paths are not interpreted here.
func NormalizeSourceRuleVariables(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return "", nil
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return "", ErrInvalidSourceRuleVariables
	}
	if len(values) == 0 {
		return "", nil
	}
	if len(values) > MaxSourceRuleVariables {
		return "", ErrInvalidSourceRuleVariables
	}

	total := 0
	for key, value := range values {
		if key == "" || key != strings.TrimSpace(key) || len(key) > MaxSourceRuleVariableKeySize || len(value) > MaxSourceRuleVariableValue {
			return "", ErrInvalidSourceRuleVariables
		}
		total += len(key) + len(value)
		if total > MaxSourceRuleVariableBytes {
			return "", ErrInvalidSourceRuleVariables
		}
	}

	normalized, err := json.Marshal(values)
	if err != nil {
		return "", ErrInvalidSourceRuleVariables
	}
	return string(normalized), nil
}

// SourceRuleVariableMap decodes a previously normalized value. It is exposed
// so the parser can use the exact same persistence validation on reads.
func SourceRuleVariableMap(raw string) (map[string]string, error) {
	normalized, err := NormalizeSourceRuleVariables(raw)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	if normalized == "" {
		return values, nil
	}
	if err := json.Unmarshal([]byte(normalized), &values); err != nil {
		return nil, ErrInvalidSourceRuleVariables
	}
	return values, nil
}
