package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Falasefemi2/jobtracker/internal/domain"
	"github.com/Falasefemi2/jobtracker/internal/shortener"
)

// uniqueViolation is the Postgres SQLSTATE for a unique constraint conflict.
const uniqueViolation = "23505"

const (
	// codeConstraint is the primary key on short_urls.code; a clash here means
	// the generated code is taken and the shortener should draw another.
	codeConstraint = "short_urls_pkey"
	// longURLConstraint is the unique index on short_urls.long_url; a clash here
	// means the same URL was shortened concurrently and the caller should reuse
	// the existing code.
	longURLConstraint = "short_urls_long_url_unique"
)

// URLRepo persists the code to long URL mapping behind short links.
type URLRepo struct {
	db *sql.DB
}

func NewURLRepo(db *sql.DB) *URLRepo {
	return &URLRepo{db: db}
}

// Resolve records a click for code and returns its long URL in one statement,
// so a lookup can never succeed while its click increment fails. A missing code
// is reported as sql.ErrNoRows so the HTTP layer can answer 404.
func (r *URLRepo) Resolve(ctx context.Context, code string) (string, error) {
	var longURL string
	err := r.db.QueryRowContext(ctx, `
		UPDATE short_urls
		SET clicks = clicks + 1
		WHERE code = $1
		RETURNING long_url
		`, code).Scan(&longURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("short code %q not found: %w", code, sql.ErrNoRows)
	}
	if err != nil {
		return "", fmt.Errorf("resolve short code %q: %w", code, err)
	}
	return longURL, nil
}

func (r *URLRepo) GetByLongURL(ctx context.Context, longURL string) (domain.ShortURL, error) {
	var s domain.ShortURL
	err := scanShortURL(r.db.QueryRowContext(ctx, `
		SELECT code, long_url, clicks, created_at
		FROM short_urls
		WHERE long_url = $1
		`, longURL), &s)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ShortURL{}, fmt.Errorf("no short code for %q: %w", longURL, sql.ErrNoRows)
	}
	if err != nil {
		return domain.ShortURL{}, err
	}
	return s, nil
}

// Create stores a new mapping. A clash on the code is reported as
// shortener.ErrCodeTaken so the caller can draw another; a clash on long_url
// means a concurrent writer already shortened the same URL and is reported as
// shortener.ErrLongURLExists so the caller can reuse their code.
func (r *URLRepo) Create(ctx context.Context, s domain.ShortURL) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO short_urls (code, long_url)
		VALUES ($1, $2)
		`, s.Code, s.LongURL)
	if isConstraint(err, codeConstraint) {
		return fmt.Errorf("short code %q is taken: %w", s.Code, shortener.ErrCodeTaken)
	}
	if isConstraint(err, longURLConstraint) {
		return fmt.Errorf("long URL %s already has a short code: %w", s.LongURL, shortener.ErrLongURLExists)
	}
	if err != nil {
		return fmt.Errorf("failed to shorten %s: %w", s.LongURL, err)
	}
	return nil
}

type shortURLScanner interface {
	Scan(dest ...any) error
}

func scanShortURL(s shortURLScanner, out *domain.ShortURL) error {
	return s.Scan(&out.Code, &out.LongURL, &out.Clicks, &out.CreatedAt)
}

func isConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == name
}
