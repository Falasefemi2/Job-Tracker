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

const codeConstraint = "short_urls_pkey"

// URLRepo persists the code to long URL mapping behind short links.
type URLRepo struct {
	db *sql.DB
}

func NewURLRepo(db *sql.DB) *URLRepo {
	return &URLRepo{db: db}
}

func (r *URLRepo) GetByCode(ctx context.Context, code string) (domain.ShortURL, error) {
	var s domain.ShortURL
	err := scanShortURL(r.db.QueryRowContext(ctx, `
		SELECT code, long_url, clicks, created_at
		FROM short_urls
		WHERE code = $1
		`, code), &s)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ShortURL{}, fmt.Errorf("short code %q not found: %w", code, sql.ErrNoRows)
	}
	if err != nil {
		return domain.ShortURL{}, err
	}
	return s, nil
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
// shortener.ErrCodeTaken so the caller can draw another; a clash on long_url is
// a real duplicate and surfaces as-is.
func (r *URLRepo) Create(ctx context.Context, s domain.ShortURL) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO short_urls (code, long_url)
		VALUES ($1, $2)
		`, s.Code, s.LongURL)
	if isConstraint(err, codeConstraint) {
		return fmt.Errorf("short code %q is taken: %w", s.Code, shortener.ErrCodeTaken)
	}
	if err != nil {
		return fmt.Errorf("failed to shorten %s: %w", s.LongURL, err)
	}
	return nil
}

func (r *URLRepo) IncrementClicks(ctx context.Context, code string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE short_urls
		SET clicks = clicks + 1
		WHERE code = $1
		`, code)
	if err != nil {
		return fmt.Errorf("failed to increment clicks for short code %q: %w", code, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify click increment for short code %q: %w", code, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("short code %q not found: %w", code, sql.ErrNoRows)
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
