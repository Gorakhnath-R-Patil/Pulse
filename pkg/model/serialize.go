package model

import (
	"encoding/json"
	"fmt"
)

// Marshal encodes an Event as JSON. It does not validate e first — callers
// that need to guarantee well-formed output should call e.Validate()
// themselves, matching the decode/validate separation used when loading
// events back with Unmarshal.
func Marshal(e Event) ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("model: marshal event: %w", err)
	}
	return data, nil
}

// Unmarshal decodes JSON into an Event. It does not validate the result;
// callers should call Validate() on the returned Event before acting on
// it, so malformed-but-parseable input (e.g. a missing required field)
// is caught explicitly rather than assumed away.
func Unmarshal(data []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return Event{}, fmt.Errorf("model: unmarshal event: %w", err)
	}
	return e, nil
}
