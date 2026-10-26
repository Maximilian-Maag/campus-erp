// Package lookup implements the canonical lookup catalogue: one table keyed by
// (namespace, code) instead of the productive source's 3,532 separate key tables.
package lookup

import (
	"errors"
	"strings"
)

// Validation errors.
var (
	ErrNamespaceRequired = errors.New("lookup: a namespace is required")
	ErrCodeRequired      = errors.New("lookup: a code is required")
	ErrShortTextRequired = errors.New("lookup: a short text is required")
	ErrActorRequired     = errors.New("lookup: an actor is required")
	ErrNotFound          = errors.New("lookup: value not found")
)

// Value is one coded value in a namespace.
type Value struct {
	Namespace string
	Code      string
	ShortText string
	LongText  string
	SortOrder int
	Active    bool
}

// NewValue validates and builds a catalogue value.
func NewValue(namespace, code, shortText, longText string, sortOrder int, active bool) (Value, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return Value{}, ErrNamespaceRequired
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return Value{}, ErrCodeRequired
	}
	shortText = strings.TrimSpace(shortText)
	if shortText == "" {
		return Value{}, ErrShortTextRequired
	}
	return Value{
		Namespace: namespace, Code: code, ShortText: shortText,
		LongText: strings.TrimSpace(longText), SortOrder: sortOrder, Active: active,
	}, nil
}
