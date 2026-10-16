package domain

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidDate is returned by ParseDate for input that matches no layout.
var ErrInvalidDate = errors.New("invalid date: use YYYY-MM-DD, YYYY-MM-DD HH:MM:SS, or RFC3339")

// ParseDate accepts an empty value (returning the zero time), RFC3339, or the
// plain date/time layouts. The local timezone is assumed for the layout-based
// formats, mirroring how the CLI and TUI present dates.
func ParseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{time.DateTime, time.DateOnly} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, ErrInvalidDate
}
