package repo

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/Falasefemi2/jobtracker/internal/domain"
)

type ApplicationRepo struct {
	db *sql.DB
}

func NewApplicationRepo(db *sql.DB) *ApplicationRepo {
	return &ApplicationRepo{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanApplication(s rowScanner, a *domain.Application) error {
	var (
		company  string
		jobTitle string
		status   string
		location sql.NullString
		source   sql.NullString
		jobURL   sql.NullString
		notes    sql.NullString
	)
	if err := s.Scan(
		&a.ID,
		&company,
		&jobTitle,
		&status,
		&location,
		&source,
		&jobURL,
		&notes,
		&a.AppliedAt,
		&a.CreatedAt,
		&a.UpdatedAt,
	); err != nil {
		return err
	}

	a.Company = company
	a.JobTitle = jobTitle
	a.Status = domain.Status(status)
	if location.Valid {
		a.Location = location.String
	}
	if source.Valid {
		a.Source = source.String
	}
	if jobURL.Valid {
		a.JobURL = jobURL.String
	}
	if notes.Valid {
		a.Notes = notes.String
	}
	return nil
}

func (r *ApplicationRepo) GetByID(ctx context.Context, id int64) (domain.Application, error) {
	var a domain.Application
	err := scanApplication(r.db.QueryRowContext(ctx, `
		SELECT id, company, job_title, status, location, source, job_url, notes, applied_at, created_at, updated_at
		FROM applications
		WHERE id = $1
		`, id), &a)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Application{}, fmt.Errorf("application ID %d not found: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		return domain.Application{}, err
	}
	return a, nil
}

func (r *ApplicationRepo) List(ctx context.Context) ([]domain.Application, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, company, job_title, status, location, source, job_url, notes, applied_at, created_at, updated_at
		FROM applications
		ORDER BY applied_at DESC 
		`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []domain.Application
	for rows.Next() {
		var a domain.Application
		if err := scanApplication(rows, &a); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *ApplicationRepo) Create(ctx context.Context, a domain.Application) (int64, error) {
	status := string(a.Status)
	if status == "" {
		status = string(domain.StatusApplied)
	}
	appliedAt := a.AppliedAt
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO applications (company, job_title, status, location, source, job_url, notes, applied_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, a.Company, a.JobTitle, status, a.Location, a.Source, a.JobURL, a.Notes, appliedAt).Scan(&id)
	return id, err
}

func (r *ApplicationRepo) Stats(ctx context.Context) (domain.Stats, error) {
	var s domain.Stats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'applied'),
			COUNT(*) FILTER (WHERE status = 'interviewing'),
			COUNT(*) FILTER (WHERE status = 'offer')
		FROM applications
	`).Scan(&s.Total, &s.Applied, &s.Interviewing, &s.Offers)
	return s, err
}

func (r *ApplicationRepo) ExportAsCSV(ctx context.Context, w io.Writer) error {
	apps, err := r.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to query applications for export: %w", err)
	}

	writer := csv.NewWriter(w)
	headers := []string{
		"ID", "Company", "Job Title", "Status", "Location",
		"Source", "Job URL", "Notes", "Applied At", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV headers: %w", err)
	}
	for _, app := range apps {
		record := []string{
			strconv.FormatInt(app.ID, 10),
			app.Company,
			app.JobTitle,
			string(app.Status),
			app.Location,
			app.Source,
			app.JobURL,
			app.Notes,
			app.AppliedAt.Format(time.RFC3339),
			app.CreatedAt.Format(time.RFC3339),
			app.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write record line to CSV buffer: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV output: %w", err)
	}

	return nil
}

func (r *ApplicationRepo) Update(ctx context.Context, id int64, a domain.Application) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE applications 
		SET 
			company = $1, 
			job_title = $2, 
			status = $3, 
			location = $4, 
			source = $5, 
			job_url = $6, 
			notes = $7, 
			applied_at = $8,
			updated_at = NOW()
		WHERE id = $9
	`,
		a.Company,
		a.JobTitle,
		string(a.Status),
		a.Location,
		a.Source,
		a.JobURL,
		a.Notes,
		a.AppliedAt,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update application ID %d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify update for application ID %d: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("application ID %d not found: %w", id, sql.ErrNoRows)
	}

	return nil
}

func (r *ApplicationRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM applications WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete application ID %d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify delete for application ID %d: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("application ID %d not found: %w", id, sql.ErrNoRows)
	}

	return nil
}
