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

var ErrCodeTaken = errors.New("short code already taken")

type Store interface {
	GetByCode(ctx context.Context, code string) (domain.ShortURL, error)
	GetByLongURL(ctx context.Context, longURL string) (domain.ShortURL, error)
	Create(ctx context.Context, s domain.ShortURL) error
	IncrementClicks(ctx context.Context, code string) error
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
	short, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return "", err
	}
	if err := s.store.IncrementClicks(ctx, code); err != nil {
		return "", fmt.Errorf("record click for %s: %w", code, err)
	}
	return short.LongURL, nil
}

func (s *Shortener) create(ctx context.Context, longURL string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < codeAttempts; attempt++ {
		code, err := Code()
		if err != nil {
			return "", err
		}
		lastErr = s.store.Create(ctx, domain.ShortURL{Code: code, LongURL: longURL})
		if lastErr == nil {
			return code, nil
		}
		if !errors.Is(lastErr, ErrCodeTaken) {
			return "", lastErr
		}
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
