package domain

import "time"

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
