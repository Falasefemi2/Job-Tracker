package domain

import "time"

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

type Application struct {
	ID        int64
	Company   string
	JobTitle  string
	Status    Status
	Location  string
	Source    string
	JobURL    string
	Notes     string
	AppliedAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Stats struct {
	Total        int
	Applied      int
	Interviewing int
	Offers       int
}
