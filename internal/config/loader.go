package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// loadYAMLFile decodes the YAML file at path into out. It uses strict
// decoding so a typo in a config file (e.g. "levle" instead of "level")
// fails loudly at startup instead of being silently ignored.
//
// path must be non-empty; callers decide whether "no file requested" means
// "use defaults" before calling this function.
func loadYAMLFile(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return fmt.Errorf("config: reading %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidSyntax, path, err)
	}
	return nil
}
