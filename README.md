# jobtracker

A small command-line tool for keeping track of job applications. I got tired of losing track of where I applied, what stage each application was in, and when I last followed up, so I built this.

It has two parts: a plain CLI for quick add / list / update work, and a full-screen terminal dashboard for browsing everything at once. Both talk to the same Postgres database.

## What it does

- Store every application in one place: company, role, status, location, source, job link, notes, and dates.
- Move applications through stages: applied, screening, interviewing, offer, accepted, rejected, withdrawn.
- Shorten every job URL you enter into a shareable link, and resolve it back to the posting with a small redirect server.
- Browse and search in the terminal dashboard with keyboard shortcuts.
- Get quick counts by stage.
- Export everything to CSV when you want to look at it in a spreadsheet or back it up.

## Tech stack

- **Go 1.27** — the whole app.
- **Postgres + pgx** — storage. Migrations are plain SQL files in `migrations/`.
- **Cobra** — the CLI commands (`list`, `show`, `add`, and so on).
- **Bubble Tea + Bubbles + Lipgloss** — the interactive terminal UI.
- **net/http** — the short link redirect server (`cmd/serve`).
- **godotenv** — loads `DATABASE_URL` from `.env` in development.
- **go-task** — short task aliases so you do not have to remember long `go run` commands.

## Prerequisites

You need these installed:

1. Go 1.27 or newer. Check with `go version`.
2. Postgres 14 or newer running locally (or anywhere reachable). Check with `psql --version`.
3. Optional but recommended: go-task. Install with:
   ```
   go install github.com/go-task/task/v3/cmd/task@latest
   ```
   If you skip it, every `task <name>` below has a `go run` equivalent.

## Setup

1. Clone the repo and move into it:
   ```
   git clone https://github.com/Falasefemi2/Job-Tracker.git
   cd jobtracker
   ```

2. Create the database:
   ```
   createdb jobtracker
   ```
   Or from psql:
   ```sql
   CREATE DATABASE jobtracker;
   ```

3. Create a `.env` file in the project root:
   ```
   DATABASE_URL=postgres://user:password@localhost:5432/jobtracker?sslmode=disable
   ```
   Replace `user`, `password`, and the port if yours are different. The file is already gitignored, so it will not get committed.

   If you prefer environment variables, you can skip `.env` and just export it:
   ```
   setx DATABASE_URL "postgres://user:password@localhost:5432/jobtracker?sslmode=disable"
   ```
   (On PowerShell for the current session: `$env:DATABASE_URL="postgres://..."`)

4. Run the migrations:
   ```
   task migrate
   ```
   Without task:
   ```
   go run ./cmd/migrate up
   ```
   This creates the `applications` table and a `schema_migrations` table that tracks which migrations have run.

5. Confirm it works:
   ```
   task list
   ```
   An empty list with no errors means you are set.

## Running it

### The dashboard (TUI)

This is the main way to use the app day to day.

```
task tui
```

Without task:

```
go run ./cmd/jobtracker tui
```

Keyboard shortcuts inside the dashboard:

| Key | Action |
| --- | ------ |
| Up / Down, j / k | Move through the list |
| Enter | Open details for the selected application |
| / | Search by company, title, location, or notes |
| s | Filter by status |
| a | Add a new application |
| u | Change the status of the selected one |
| e | Export to CSV (you will be asked for a filename) |
| d then y | Delete (it asks for confirmation first) |
| r | Refresh from the database |
| q or Ctrl+C | Quit |

### The CLI

Good for quick one-off actions or scripting.

List everything:

```
task list
go run ./cmd/jobtracker list
go run ./cmd/jobtracker list --format csv
```

Show one application in full:

```
task show ID=1
go run ./cmd/jobtracker show --id 1
```

Add an application (only company and title are required):

```
task add COMPANY="Acme" TITLE="Backend Engineer" LOCATION="Remote" SOURCE="LinkedIn" JOB_URL="https://example.com/jobs/1" NOTES="Referred by Ada" STATUS="applied" APPLIED_AT="2026-09-20"
```

Without task:

```
go run ./cmd/jobtracker add --company "Acme" --title "Backend Engineer" --location "Remote" --source "LinkedIn"
```

Update an application (only pass the fields you want to change):

```
task update ID=1 STATUS=interviewing
task update ID=1 COMPANY="Acme Corp" LOCATION="Lagos" NOTES="Second round next week"
```

Without task:

```
go run ./cmd/jobtracker update --id 1 --status interviewing
```

Show counts:

```
task stats
go run ./cmd/jobtracker stats
```

Output looks like:

```
Total: 12
Applied: 7
Interviewing: 3
Offers: 1
```

Export to CSV:

```
task export FILE=applications.csv
go run ./cmd/jobtracker export --file applications.csv
go run ./cmd/jobtracker export --file -   # print to stdout instead
```

Delete:

```
task delete ID=1
go run ./cmd/jobtracker delete --id 1
```

The CLI will reject bad input early with a plain message — missing `--company`, an unknown status, or a date it cannot parse. Dates accept `YYYY-MM-DD`, `YYYY-MM-DD HH:MM:SS`, or RFC3339.

## Short links

Job posting URLs get long. Whenever you enter one — via `add`, via `update --job-url`, or in the TUI add form — jobtracker stores the URL and mints a short code for it, then shows you the link:

```
$ go run ./cmd/jobtracker add --company Acme --title "Backend Engineer" \
    --job-url boards.greenhouse.io/acme/jobs/4821
Created application ID 12
Short link: http://localhost:8080/s/aB3xK9z
```

A few behaviours worth knowing:

- A bare host such as `boards.greenhouse.io/acme/jobs/4821` gets `https://` added automatically. Only `http` and `https` are accepted.
- The long URL stays in `applications.job_url`, so CSV exports and the app remain meaningful even when the server is down. The short code lives in its own table.
- The same URL always gets the same short link, so applying to the same posting twice does not create a second code.
- `show` prints the short link, and the TUI detail view has a **Short Link** row.

To follow a short link, run the redirect server:

```
task serve
# or
go run ./cmd/serve
```

```
$ curl -i http://localhost:8080/s/aB3xK9z
HTTP/1.1 302 Found
Location: https://boards.greenhouse.io/acme/jobs/4821
```

Set `BASE_URL` to the public address so the links you hand out point somewhere real:

```
BASE_URL=https://jt.example.com ADDR=:8080 go run ./cmd/serve
```

`BASE_URL` sets the domain in generated links; `ADDR` sets where the process listens (it defaults to the port in `BASE_URL`, or `8080`).

## Data model

Two tables. `applications`:

| Column | Type | Notes |
| ------ | ---- | ----- |
| id | bigint, identity | Primary key, auto-generated |
| company | text, required | Company you applied to |
| job_title | text, required | Job title |
| status | text, default `applied` | One of the statuses below |
| location | text, nullable | e.g. Remote, Lagos, London |
| source | text, nullable | e.g. LinkedIn, referral, company site |
| job_url | text, nullable | Link to the posting, normalized on save |
| notes | text, nullable | Anything you want to remember |
| applied_at | timestamptz, default now() | When you applied |
| created_at | timestamptz, default now() | When the row was created |
| updated_at | timestamptz, default now() | Last update |

And `short_urls`, backing the short links:

| Column | Type | Notes |
| ------ | ---- | ----- |
| code | text | Primary key, 7-character base62 |
| long_url | text, required, unique | The original posting URL |
| clicks | bigint, default 0 | Incremented on every redirect |
| created_at | timestamptz, default now() | When the row was created |

Valid statuses:

```
applied, screening, interviewing, offer, accepted, rejected, withdrawn
```

Status input is case-insensitive, so `Offer`, `OFFER`, and `offer` all work.

## Project structure

```
cmd/
  jobtracker/main.go   Entry point for the CLI and TUI. Cobra commands live here.
  migrate/main.go      Minimal migration runner (up / down), no external tool needed.
  serve/main.go        Short link redirect server.
internal/
  db/                  Opens the Postgres connection with pgx.
  domain/              Core types: Application, Status, Stats, ShortURL. No database code.
  repo/                SQL queries: applications CRUD, stats, CSV export, short_urls.
  shortener/           URL normalization, base62 code generation, short link building.
  tui/                 Dashboard. ui.go holds state and key handling, view.go holds rendering.
  web/                 HTTP handler behind the short links.
migrations/
  00001_...up.sql     Creates the applications table.
  00001_...down.sql   Drops it again for rollback.
  00002_...up.sql     Creates the short_urls table.
  00002_...down.sql   Drops it again for rollback.
Taskfile.yml           Short aliases for common commands.
```

A few design choices worth knowing:

- The TUI never talks to SQL directly. It goes through a small `applicationStore` interface, which is why the UI can be tested with a fake store and no database. Short links work the same way through a `linkShortener` interface.
- Rendering (`view.go`) does no database or file work. All slow work runs as Bubble Tea commands so the UI stays responsive.
- Migrations are intentionally simple — numbered `.up.sql` / `.down.sql` files plus a `schema_migrations` table. No ORM, no extra dependency.
- Short codes come from `crypto/rand` with modulo bias rejected, so codes are unguessable and evenly distributed. No base62 library needed.

## Development

Format, vet, and test:

```
task check
```

Or step by step:

```
task fmt    # gofmt -l -w .
task vet    # go vet ./...
task test   # go test ./...
```

Roll back the last migration (useful while changing the schema locally):

```
task migrate-down
go run ./cmd/migrate down
```

Add a new migration by creating the next numbered pair, for example `migrations/00003_add_deadline_column.up.sql` and `...down.sql`, then run `task migrate`.

## Configuration

| Variable | Required | Example |
| -------- | -------- | ------- |
| DATABASE_URL | Yes | postgres://user:password@localhost:5432/jobtracker?sslmode=disable |
| BASE_URL | No | https://jt.example.com — domain used in short links, defaults to http://localhost:8080 |
| ADDR | No | :9000 — address the redirect server listens on, defaults to the port in BASE_URL or 8080 |

Loaded from `.env` if present, otherwise from the environment. There is no `.env.example` checked in — copy the lines above to start your own.

CSV exports from the dashboard or CLI default to the current directory (e.g. `applications-20260925-023302.csv`). They are gitignored so they do not end up in commits.

## Troubleshooting

**`DATABASE_URL is required` or cannot connect**

Make sure `.env` exists in the project root and Postgres is running. Test the connection manually:

```
psql "postgres://user:password@localhost:5432/jobtracker?sslmode=disable" -c "select 1;"
```

**`no migration files in migrations`**

You are probably running the command from the wrong directory. Run it from the project root where the `migrations/` folder lives.

**Dashboard shows garbled output or keys do nothing**

Use a modern terminal (Windows Terminal, WezTerm, Alacritty, iTerm2, GNOME Terminal). The dashboard needs ANSI color support and an alternate screen. Old `cmd.exe` will struggle.

**`invalid status` error**

Double-check spelling against the list above. Only those seven values are accepted.

**`invalid applied-at date` error**

Use `2026-09-28`, `2026-09-28 14:30:00`, or full RFC3339 like `2026-09-28T14:30:00Z`.

**`invalid URL "..." : only http and https are supported`**

Short links only make sense for web pages. Use an `http://` or `https://` posting — a `mailto:` or `ftp:` address cannot be redirected to.

**A short link returns 404**

The code is not in `short_urls`. Either the row was rolled back with `task migrate-down`, or the server is pointed at a different database than the one the link was created against. Check that `DATABASE_URL` matches on both.

## Further reading

- Run `go run ./cmd/jobtracker --help` or `go run ./cmd/jobtracker <command> --help` for full flag reference.

## License

No license file is included yet. If you plan to share or open-source this, add one (MIT is the usual default for small Go tools) before publishing.
