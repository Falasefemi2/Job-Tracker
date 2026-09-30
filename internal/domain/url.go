package domain

import "time"

type ShortURL struct {
	Code      string
	LongURL   string
	Clicks    int64
	CreatedAt time.Time
}
