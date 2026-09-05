package dns

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// message is a parsed DNS message header plus its first question. Only
// the first question is parsed: virtually every real-world DNS query
// has exactly one, and DNS itself puts the question section
// immediately after the fixed 12-byte header, before any answer
// records this package has no reason to inspect.
type message struct {
	id           uint16
	isResponse   bool
	responseCode uint8 // meaningful only when isResponse
	answerCount  uint16
	name         string // the first question's domain name, e.g. "example.com"
	qtype        uint16 // the first question's QTYPE (1=A, 28=AAAA, 5=CNAME, ...)
}

// headerLen is the fixed DNS message header: ID(2) + flags(2) +
// QDCOUNT(2) + ANCOUNT(2) + NSCOUNT(2) + ARCOUNT(2).
const headerLen = 12

// parseMessage parses data as a DNS message. It returns an error
// wrapping ErrShortMessage, ErrNoQuestion, or ErrUnsupportedName for
// anything that isn't a well-formed, single-question DNS message —
// callers should treat that as "this capture wasn't actually DNS" (or
// wasn't fully captured), not as a real failure; see Loader.Read.
func parseMessage(data []byte) (message, error) {
	if len(data) < headerLen {
		return message{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortMessage, len(data), headerLen)
	}

	id := binary.BigEndian.Uint16(data[0:2])
	flags := data[2]
	isResponse := flags&0x80 != 0
	rcode := data[3] & 0x0F
	qdCount := binary.BigEndian.Uint16(data[4:6])
	anCount := binary.BigEndian.Uint16(data[6:8])

	if qdCount == 0 {
		return message{}, fmt.Errorf("%w", ErrNoQuestion)
	}

	name, offset, err := parseName(data, headerLen)
	if err != nil {
		return message{}, err
	}

	if offset+4 > len(data) {
		return message{}, fmt.Errorf("%w: truncated before QTYPE/QCLASS", ErrShortMessage)
	}
	qtype := binary.BigEndian.Uint16(data[offset : offset+2])

	return message{
		id:           id,
		isResponse:   isResponse,
		responseCode: rcode,
		answerCount:  anCount,
		name:         name,
		qtype:        qtype,
	}, nil
}

// parseName parses a DNS name (a sequence of length-prefixed labels
// ending in a zero-length label) starting at offset, returning the
// dotted name and the offset immediately after it.
//
// It does not resolve label compression (a pointer back into an
// earlier part of the message, used to avoid repeating a name DNS
// already wrote once). The first question in a message immediately
// follows the fixed 12-byte header, with nothing earlier in the
// message a compression pointer could sensibly refer to — so a
// compression byte appearing here is treated as unparseable rather
// than resolved.
func parseName(data []byte, offset int) (string, int, error) {
	var labels []string
	for {
		if offset >= len(data) {
			return "", 0, fmt.Errorf("%w: name ran past the end of the message", ErrShortMessage)
		}
		length := int(data[offset])
		if length == 0 {
			offset++
			break
		}
		if length&0xC0 != 0 {
			return "", 0, fmt.Errorf("%w", ErrUnsupportedName)
		}
		offset++
		if offset+length > len(data) {
			return "", 0, fmt.Errorf("%w: label ran past the end of the message", ErrShortMessage)
		}
		labels = append(labels, string(data[offset:offset+length]))
		offset += length
	}
	return strings.Join(labels, "."), offset, nil
}
