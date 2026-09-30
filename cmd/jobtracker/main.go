package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Falasefemi2/jobtracker/internal/db"
	"github.com/Falasefemi2/jobtracker/internal/domain"
	"github.com/Falasefemi2/jobtracker/internal/repo"
	"github.com/Falasefemi2/jobtracker/internal/shortener"
	"github.com/Falasefemi2/jobtracker/internal/tui"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	sqlDB         *sql.DB
	appRepo       *repo.ApplicationRepo
	urlRepo       *repo.URLRepo
	linkShortener *shortener.Shortener
)

type applicationInput struct {
	company   string
	title     string
	status    string
	location  string
	source    string
	jobURL    string
	notes     string
	appliedAt string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()
	root := &cobra.Command{
		Use:   "jobtracker",
		Short: "Track job applications",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			d, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
			if err != nil {
				return err
			}
			sqlDB = d
			appRepo = repo.NewApplicationRepo(sqlDB)
			urlRepo = repo.NewURLRepo(sqlDB)
			linkShortener = shortener.New(urlRepo, os.Getenv("BASE_URL"))
			return nil
		},
	}
	defer func() {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	root.AddCommand(
		newListCommand(),
		newShowCommand(),
		newAddCommand(),
		newUpdateCommand(),
		newStatsCommand(),
		newExportCommand(),
		newDeleteCommand(),
		newTUICommand(),
	)
	return root.Execute()
}

func newListCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List applications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "table":
				apps, err := appRepo.List(cmd.Context())
				if err != nil {
					return err
				}
				for _, a := range apps {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-4d %-15s %-25s %-12s %s\n",
						a.ID, a.Company, a.JobTitle, a.Status, a.AppliedAt.Format("Jan 02")); err != nil {
						return err
					}
				}
				return nil
			case "csv":
				return appRepo.ExportAsCSV(cmd.Context(), cmd.OutOrStdout())
			default:
				return errors.New(`invalid format: expected "table" or "csv"`)
			}
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format: table or csv")
	return cmd
}

func newShowCommand() *cobra.Command {
	var id int64
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show one application",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if id <= 0 {
				return errors.New("a positive --id is required")
			}
			a, err := appRepo.GetByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			shortLink, err := linkShortener.Lookup(cmd.Context(), a.JobURL)
			if err != nil {
				return err
			}
			cmd.Printf("ID: %d\nCompany: %s\nTitle: %s\nStatus: %s\nLocation: %s\nSource: %s\nJob URL: %s\nShort Link: %s\nNotes: %s\nApplied: %s\nCreated: %s\nUpdated: %s\n",
				a.ID,
				a.Company,
				a.JobTitle,
				a.Status,
				a.Location,
				a.Source,
				a.JobURL,
				displayShortLink(shortLink),
				a.Notes,
				a.AppliedAt.Format(time.RFC3339),
				a.CreatedAt.Format(time.RFC3339),
				a.UpdatedAt.Format(time.RFC3339),
			)
			return nil
		},
	}
	cmd.Flags().Int64VarP(&id, "id", "i", 0, "Application ID")
	return cmd
}

func newAddCommand() *cobra.Command {
	var input applicationInput
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an application",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(input.company) == "" {
				return errors.New("--company is required")
			}
			if strings.TrimSpace(input.title) == "" {
				return errors.New("--title is required")
			}
			status, err := normalizeStatus(input.status)
			if err != nil {
				return err
			}
			appliedAt, err := parseAppliedDate(input.appliedAt)
			if err != nil {
				return err
			}
			jobURL, shortLink, err := prepareJobURL(cmd.Context(), input.jobURL)
			if err != nil {
				return err
			}
			id, err := appRepo.Create(cmd.Context(), domain.Application{
				Company:   strings.TrimSpace(input.company),
				JobTitle:  strings.TrimSpace(input.title),
				Status:    status,
				Location:  input.location,
				Source:    input.source,
				JobURL:    jobURL,
				Notes:     input.notes,
				AppliedAt: appliedAt,
			})
			if err != nil {
				return err
			}
			if shortLink != "" {
				cmd.Printf("Created application ID %d\nShort link: %s\n", id, shortLink)
				return nil
			}
			cmd.Printf("Created application ID %d\n", id)
			return nil
		},
	}
	addApplicationFlags(cmd, &input, string(domain.StatusApplied))
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var id int64
	var input applicationInput
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an application",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if id <= 0 {
				return errors.New("a positive --id is required")
			}
			updated, err := appRepo.GetByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			var updatedShortLink string
			if cmd.Flags().Changed("company") {
				updated.Company = input.company
			}
			if cmd.Flags().Changed("title") {
				updated.JobTitle = input.title
			}
			if cmd.Flags().Changed("status") {
				status, err := normalizeStatus(input.status)
				if err != nil {
					return err
				}
				updated.Status = status
			}
			if cmd.Flags().Changed("location") {
				updated.Location = input.location
			}
			if cmd.Flags().Changed("source") {
				updated.Source = input.source
			}
			if cmd.Flags().Changed("job-url") {
				jobURL, shortLink, err := prepareJobURL(cmd.Context(), input.jobURL)
				if err != nil {
					return err
				}
				updated.JobURL = jobURL
				updatedShortLink = shortLink
			}
			if cmd.Flags().Changed("notes") {
				updated.Notes = input.notes
			}
			if cmd.Flags().Changed("applied-at") {
				if strings.TrimSpace(input.appliedAt) == "" {
					return errors.New("applied-at cannot be empty when provided; omit the flag to keep the current value")
				}
				appliedAt, err := parseAppliedDate(input.appliedAt)
				if err != nil {
					return err
				}
				updated.AppliedAt = appliedAt
			}
			if strings.TrimSpace(updated.Company) == "" {
				return errors.New("--company cannot be empty")
			}
			if strings.TrimSpace(updated.JobTitle) == "" {
				return errors.New("--title cannot be empty")
			}
			if err := appRepo.Update(cmd.Context(), id, updated); err != nil {
				return err
			}
			if updatedShortLink != "" {
				cmd.Printf("Updated application ID %d\nShort link: %s\n", id, updatedShortLink)
				return nil
			}
			cmd.Printf("Updated application ID %d\n", id)
			return nil
		},
	}
	cmd.Flags().Int64VarP(&id, "id", "i", 0, "Application ID")
	addApplicationFlags(cmd, &input, "")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var id int64
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an application",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if id <= 0 {
				return errors.New("a positive --id is required")
			}
			if err := appRepo.Delete(cmd.Context(), id); err != nil {
				return err
			}
			cmd.Printf("Deleted application ID %d\n", id)
			return nil
		},
	}
	cmd.Flags().Int64VarP(&id, "id", "i", 0, "Application ID")
	return cmd
}

func newStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show application counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := appRepo.Stats(cmd.Context())
			if err != nil {
				return err
			}
			cmd.Printf("Total: %d\nApplied: %d\nInterviewing: %d\nOffers: %d\n",
				stats.Total, stats.Applied, stats.Interviewing, stats.Offers)
			return nil
		},
	}
}

func newExportCommand() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export applications as CSV",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := strings.TrimSpace(file)
			if path == "" || path == "-" {
				return appRepo.ExportAsCSV(cmd.Context(), cmd.OutOrStdout())
			}
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			exportErr := appRepo.ExportAsCSV(cmd.Context(), f)
			closeErr := f.Close()
			if exportErr != nil {
				return exportErr
			}
			if closeErr != nil {
				return closeErr
			}
			cmd.Printf("Exported applications to %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "CSV output file; omit or use - for stdout")
	return cmd
}

func newTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive terminal dashboard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(cmd.Context(), appRepo, linkShortener)
		},
	}
}

// prepareJobURL normalizes a job URL for storage and returns it with the short
// link minted for it, so saving a posting never leaves it without one.
func prepareJobURL(ctx context.Context, raw string) (jobURL, shortLink string, err error) {
	jobURL, err = shortener.Normalize(raw)
	if err != nil {
		return "", "", err
	}
	if jobURL == "" {
		return "", "", nil
	}
	shortLink, err = linkShortener.Shorten(ctx, jobURL)
	if err != nil {
		return "", "", err
	}
	return jobURL, shortLink, nil
}

func displayShortLink(shortLink string) string {
	if shortLink == "" {
		return "-"
	}
	return shortLink
}

func addApplicationFlags(cmd *cobra.Command, input *applicationInput, defaultStatus string) {
	cmd.Flags().StringVarP(&input.company, "company", "c", "", "Company name")
	cmd.Flags().StringVarP(&input.title, "title", "t", "", "Job title")
	cmd.Flags().StringVarP(&input.status, "status", "s", defaultStatus, "Application status")
	cmd.Flags().StringVarP(&input.location, "location", "l", "", "Location")
	cmd.Flags().StringVar(&input.source, "source", "", "Application source")
	cmd.Flags().StringVarP(&input.jobURL, "job-url", "u", "", "Job posting URL")
	cmd.Flags().StringVarP(&input.notes, "notes", "n", "", "Notes")
	cmd.Flags().StringVar(&input.appliedAt, "applied-at", "", "Applied date: YYYY-MM-DD, YYYY-MM-DD HH:MM:SS, or RFC3339")
}

func normalizeStatus(value string) (domain.Status, error) {
	switch status := domain.Status(strings.ToLower(strings.TrimSpace(value))); status {
	case domain.StatusApplied,
		domain.StatusScreening,
		domain.StatusInterviewing,
		domain.StatusOffer,
		domain.StatusAccepted,
		domain.StatusRejected,
		domain.StatusWithdrawn:
		return status, nil
	default:
		return "", errors.New(`invalid status: expected one of "applied", "screening", "interviewing", "offer", "accepted", "rejected", or "withdrawn"`)
	}
}

func parseAppliedDate(value string) (time.Time, error) {
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
	return time.Time{}, errors.New("invalid applied-at date: use YYYY-MM-DD, YYYY-MM-DD HH:MM:SS, or RFC3339")
}
