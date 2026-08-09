package model

import (
	"crypto/rand"
	"fmt"
)

// NewID generates a random RFC 4122 version 4 UUID, used as an Event's
// unique identifier. It is hand-rolled rather than pulled from a
// dependency: a UUIDv4 is 16 random bytes and two bit twiddles, which
// doesn't justify taking on a package for.
func NewID() string {
	var b [16]byte
	// crypto/rand.Read on the stdlib's global reader never returns an
	// error in practice (see its docs); a partial read would corrupt an
	// identifier silently, so we still check rather than ignore it.
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("model: failed to read random bytes for NewID: %v", err))
	}

	// Version 4: set the version bits (top nibble of byte 6) to 0100.
	b[6] = (b[6] & 0x0f) | 0x40
	// Variant: set the top two bits of byte 8 to 10 (RFC 4122 variant).
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
