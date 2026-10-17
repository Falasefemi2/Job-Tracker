package domain

import (
	"errors"
	"strings"
)

type Status string

const (
	StatusApplied      Status = "applied"
	StatusScreening    Status = "screening"
	StatusInterviewing Status = "interviewing"
	StatusOffer        Status = "offer"
	StatusAccepted     Status = "accepted"
	StatusRejected     Status = "rejected"
	StatusWithdrawn    Status = "withdrawn"
)

// ErrInvalidStatus is returned by ParseStatus for an unrecognised status.
var ErrInvalidStatus = errors.New(`invalid status: expected one of "applied", "screening", "interviewing", "offer", "accepted", "rejected", or "withdrawn"`)

// statuses is the canonical, ordered list of application statuses. It is the
// single source of truth for the status cycle, the status filter, and the
// database CHECK constraint.
var statuses = []Status{
	StatusApplied,
	StatusScreening,
	StatusInterviewing,
	StatusOffer,
	StatusAccepted,
	StatusRejected,
	StatusWithdrawn,
}

// Statuses returns the application statuses in lifecycle order.
func Statuses() []Status {
	out := make([]Status, len(statuses))
	copy(out, statuses)
	return out
}

// Valid reports whether s is a known application status.
func (s Status) Valid() bool {
	for _, candidate := range statuses {
		if s == candidate {
			return true
		}
	}
	return false
}

// ParseStatus normalizes free-form input into a Status. An empty value defaults
// to StatusApplied; anything unrecognised returns ErrInvalidStatus. Matching is
// case-insensitive and tolerates surrounding whitespace.
func ParseStatus(value string) (Status, error) {
	status := Status(strings.ToLower(strings.TrimSpace(value)))
	if status == "" {
		return StatusApplied, nil
	}
	if status.Valid() {
		return status, nil
	}
	return "", ErrInvalidStatus
}
