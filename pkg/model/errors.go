package model

import "errors"

// Sentinel errors returned by Validate methods in this package. Callers
// use errors.Is to distinguish "a required field was empty" from "a field
// was present but malformed," mirroring the pattern established in
// internal/config/errors.go.
var (
	// ErrMissingField is returned when a required field was left at its
	// zero value.
	ErrMissingField = errors.New("model: missing required field")

	// ErrInvalidField is returned when a field is present but fails
	// format or range validation.
	ErrInvalidField = errors.New("model: invalid field value")
)
