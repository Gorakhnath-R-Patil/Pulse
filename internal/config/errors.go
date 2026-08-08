package config

import "errors"

// Sentinel errors returned by this package. Callers should use errors.Is to
// distinguish "no config file present" (often not fatal) from "config file
// present but broken" (always fatal) and "config present but semantically
// invalid" (always fatal).
var (
	// ErrNotFound is returned when an explicitly requested configuration
	// file does not exist on disk. A path is only ever passed to the
	// loader when the caller asked for one, so this is always treated as
	// a user error rather than silently falling back to defaults.
	ErrNotFound = errors.New("config: file not found")

	// ErrInvalidSyntax is returned when a configuration file exists but
	// cannot be parsed as YAML, or contains fields the target schema does
	// not recognize.
	ErrInvalidSyntax = errors.New("config: invalid syntax")

	// ErrInvalidValue is returned when configuration was parsed
	// successfully but fails semantic validation (e.g. an unknown log
	// level).
	ErrInvalidValue = errors.New("config: invalid value")
)
