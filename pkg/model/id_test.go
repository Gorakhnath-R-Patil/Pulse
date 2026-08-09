package model_test

import (
	"regexp"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

var uuidv4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewID_MatchesUUIDv4Format(t *testing.T) {
	for i := 0; i < 50; i++ {
		id := model.NewID()
		if !uuidv4Pattern.MatchString(id) {
			t.Fatalf("NewID() = %q, does not match UUIDv4 format", id)
		}
	}
}

func TestNewID_IsUnique(t *testing.T) {
	const n = 2000
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id := model.NewID()
		if seen[id] {
			t.Fatalf("NewID() produced a duplicate after %d calls: %q", i, id)
		}
		seen[id] = true
	}
}
