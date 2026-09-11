package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a time.Duration that serialises as a Go duration string
// ("30m", "6h", "0"). Using strings everywhere keeps update_schedule.every and
// source.poll_interval consistent and allows sub-hour periods.
type Duration time.Duration

// UnmarshalJSON parses a duration string.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string such as \"6h\" or \"30m\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalJSON writes the duration back as a string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Duration returns the value as a time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// IsZero reports whether the duration is unset or zero.
func (d Duration) IsZero() bool { return time.Duration(d) == 0 }

// String implements fmt.Stringer.
func (d Duration) String() string { return time.Duration(d).String() }
