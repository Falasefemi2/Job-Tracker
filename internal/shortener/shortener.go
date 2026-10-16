package shortener

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Falasefemi2/jobtracker/internal/domain"
)

const DefaultBaseURL = "http://localhost:8080"

const (
	codeAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	codeLength   = 7
	codeAttempts = 5
)

var (
	// ErrCodeTaken means the generated code collided with an existing one; the
	// shortener draws another.
	ErrCodeTaken = errors.New("short code already taken")
	// ErrLongURLExists means the URL already has a code, typically because a
	// concurrent caller inserted it first; the shortener reuses that code.
	ErrLongURLExists = errors.New("long URL already shortened")
)

type Store interface {
	GetByLongURL(ctx context.Context, longURL string) (domain.ShortURL, error)
	Create(ctx context.Context, s domain.ShortURL) error
	// Resolve records a click for code and returns its long URL.
	Resolve(ctx context.Context, code string) (string, error)
}

type Shortener struct {
	store   Store
	baseURL string
}

func New(store Store, baseURL string) *Shortener {
	return &Shortener{store: store, baseURL: normalizeBaseURL(baseURL)}
}

func normalizeBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return DefaultBaseURL
	}
	return trimmed
}

func (s *Shortener) Link(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	return s.baseURL + "/s/" + code
}

func (s *Shortener) Shorten(ctx context.Context, rawURL string) (string, error) {
	longURL, err := Normalize(rawURL)
	if err != nil {
		return "", err
	}
	if longURL == "" {
		return "", nil
	}

	existing, err := s.store.GetByLongURL(ctx, longURL)
	if err == nil {
		return s.Link(existing.Code), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	code, err := s.create(ctx, longURL)
	if errors.Is(err, ErrLongURLExists) {
		// Another writer shortened the same URL between our lookup and insert;
		// hand back their code rather than failing.
		existing, lookupErr := s.store.GetByLongURL(ctx, longURL)
		if lookupErr != nil {
			return "", lookupErr
		}
		return s.Link(existing.Code), nil
	}
	if err != nil {
		return "", err
	}
	return s.Link(code), nil
}

func (s *Shortener) Lookup(ctx context.Context, longURL string) (string, error) {
	normalized, err := Normalize(longURL)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", nil
	}
	existing, err := s.store.GetByLongURL(ctx, normalized)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return s.Link(existing.Code), nil
}

func (s *Shortener) Resolve(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("empty short code: %w", sql.ErrNoRows)
	}
	return s.store.Resolve(ctx, code)
}

func (s *Shortener) create(ctx context.Context, longURL string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < codeAttempts; attempt++ {
		code, err := Code()
		if err != nil {
			return "", err
		}
		err = s.store.Create(ctx, domain.ShortURL{Code: code, LongURL: longURL})
		if err == nil {
			return code, nil
		}
		if errors.Is(err, ErrLongURLExists) {
			// Not retryable here; the caller re-reads the winning code.
			return "", err
		}
		if !errors.Is(err, ErrCodeTaken) {
			return "", err
		}
		lastErr = err
	}
	return "", fmt.Errorf("could not allocate a short code for %s: %w", longURL, lastErr)
}

func Normalize(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid URL %q: only http and https are supported", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid URL %q: missing host", raw)
	}
	return parsed.String(), nil
}

func Code() (string, error) {
	limit := byte(256 - (256 % len(codeAlphabet)))
	buf := make([]byte, codeLength)
	out := make([]byte, 0, codeLength)
	for len(out) < codeLength {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate short code: %w", err)
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, codeAlphabet[int(b)%len(codeAlphabet)])
			if len(out) == codeLength {
				break
			}
		}
	}
	return string(out), nil
}
