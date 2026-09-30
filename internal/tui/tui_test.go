package tui

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Falasefemi2/jobtracker/internal/domain"
)

type fakeStore struct {
	apps      []domain.Application
	stats     domain.Stats
	created   []domain.Application
	updated   map[int64]domain.Application
	deleted   []int64
	deleteErr error
}

func (f *fakeStore) List(ctx context.Context) ([]domain.Application, error) {
	return f.apps, nil
}

func (f *fakeStore) Stats(ctx context.Context) (domain.Stats, error) {
	return f.stats, nil
}

func (f *fakeStore) GetByID(ctx context.Context, id int64) (domain.Application, error) {
	return f.apps[0], nil
}

func (f *fakeStore) Create(ctx context.Context, app domain.Application) (int64, error) {
	f.created = append(f.created, app)
	return int64(len(f.created)), nil
}

func (f *fakeStore) Update(ctx context.Context, id int64, app domain.Application) error {
	if f.updated == nil {
		f.updated = map[int64]domain.Application{}
	}
	f.updated[id] = app
	return nil
}

func (f *fakeStore) Delete(ctx context.Context, id int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	kept := f.apps[:0]
	for _, app := range f.apps {
		if app.ID != id {
			kept = append(kept, app)
		}
	}
	f.apps = kept
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeStore) ExportAsCSV(ctx context.Context, w io.Writer) error {
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"ID", "Company"}); err != nil {
		return err
	}
	for _, app := range f.apps {
		if err := writer.Write([]string{itoa(int(app.ID)), app.Company}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

type fakeShortener struct {
	shortened []string
	links     map[string]string
	shortErr  error
	lookupErr error
}

func (f *fakeShortener) Shorten(ctx context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return "", nil
	}
	if f.shortErr != nil {
		return "", f.shortErr
	}
	f.shortened = append(f.shortened, rawURL)
	if f.links == nil {
		f.links = map[string]string{}
	}
	link, ok := f.links[rawURL]
	if !ok {
		link = "http://localhost:8080/s/" + strings.ToUpper(rawURL[len(rawURL)-1:])
		f.links[rawURL] = link
	}
	return link, nil
}

func (f *fakeShortener) Lookup(ctx context.Context, longURL string) (string, error) {
	if f.lookupErr != nil {
		return "", f.lookupErr
	}
	return f.links[longURL], nil
}

func testApps() []domain.Application {
	return []domain.Application{
		{ID: 1, Company: "Acme", JobTitle: "Backend Engineer", Status: domain.StatusApplied, Notes: "via recruiter"},
		{ID: 2, Company: "Globex", JobTitle: "Frontend Engineer", Status: domain.StatusInterviewing, Notes: "onsite next week"},
		{ID: 3, Company: "Initech", JobTitle: "QA Engineer", Status: domain.StatusOffer, Notes: "negotiating"},
	}
}

func loadModel(t *testing.T, store *fakeStore) Model {
	t.Helper()
	return loadModelWith(t, store, &fakeShortener{})
}

func loadModelWith(t *testing.T, store *fakeStore, shortener *fakeShortener) Model {
	t.Helper()
	model := NewModel(context.Background(), store, shortener)
	msg := model.Init()()
	updated, _ := model.Update(msg)
	loaded, ok := updated.(Model)
	if !ok {
		t.Fatal("expected tui model after loading")
	}
	return loaded
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestFilterApplications(t *testing.T) {
	apps := testApps()

	filtered := filterApplications(apps, "front", "all")
	if len(filtered) != 1 || filtered[0].Company != "Globex" {
		t.Fatalf("expected Globex, got %+v", filtered)
	}

	filtered = filterApplications(apps, "", "offer")
	if len(filtered) != 1 || filtered[0].Company != "Initech" {
		t.Fatalf("expected Initech offer, got %+v", filtered)
	}

	filtered = filterApplications(apps, "engineer", "interviewing")
	if len(filtered) != 1 || filtered[0].Company != "Globex" {
		t.Fatalf("expected combined filter to match Globex, got %+v", filtered)
	}
}

func TestVisibleWindow(t *testing.T) {
	offset, end := visibleWindow(7, 0, 5, 10)
	if offset != 3 || end != 8 {
		t.Fatalf("expected window 3:8, got %d:%d", offset, end)
	}

	offset, end = visibleWindow(0, 5, 5, 10)
	if offset != 0 || end != 5 {
		t.Fatalf("expected window 0:5, got %d:%d", offset, end)
	}
}

func TestTruncateAndPad(t *testing.T) {
	if got := truncate("Backend Engineer", 7); got != "Backen…" {
		t.Fatalf("unexpected truncation %q", got)
	}
	if got := padRight("Acme", 6); got != "Acme  " {
		t.Fatalf("unexpected padding %q", got)
	}
}

func TestParseStatus(t *testing.T) {
	status, err := parseStatus("")
	if err != nil || status != domain.StatusApplied {
		t.Fatalf("expected default applied status, got %q %v", status, err)
	}
	if _, err := parseStatus("nope"); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestParseDate(t *testing.T) {
	parsed, err := parseDate("2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Format(time.DateOnly) != "2026-09-25" {
		t.Fatalf("unexpected date %s", parsed.Format(time.RFC3339))
	}
	if _, err := parseDate("25/09/2026"); err == nil {
		t.Fatal("expected invalid date error")
	}
}

func TestNextStatus(t *testing.T) {
	if got := nextStatus(domain.StatusApplied, 1); got != domain.StatusScreening {
		t.Fatalf("expected screening, got %q", got)
	}
	if got := nextStatus(domain.StatusWithdrawn, 1); got != domain.StatusApplied {
		t.Fatalf("expected wrap to applied, got %q", got)
	}
	if got := nextStatus(domain.StatusApplied, -1); got != domain.StatusWithdrawn {
		t.Fatalf("expected wrap to withdrawn, got %q", got)
	}
}

func TestDetailStatusEditAndSave(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	updated, _ = model.Update(keyMsg("u"))
	model = updated.(Model)
	if !model.statusEditing || model.pendingStatus != domain.StatusApplied {
		t.Fatalf("expected status edit mode with applied pending, got %+v", model)
	}

	updated, _ = model.Update(keyMsg("l"))
	model = updated.(Model)
	if model.pendingStatus != domain.StatusScreening {
		t.Fatalf("expected pending screening, got %q", model.pendingStatus)
	}

	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(Model)
	if cmd == nil {
		t.Fatal("expected save command")
	}
	saved := cmd()
	msg, ok := saved.(statusSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected saved message, got %#v", saved)
	}

	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.statusEditing || model.selected.Status != domain.StatusScreening {
		t.Fatalf("expected saved screening status, got %+v", model.selected)
	}
	if store.updated[1].Status != domain.StatusScreening {
		t.Fatalf("expected store update, got %+v", store.updated)
	}
	if !strings.Contains(model.notice, "screening") {
		t.Fatalf("expected notice, got %q", model.notice)
	}
}

func TestDetailStatusEditCancel(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(keyMsg("u"))
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if model.statusEditing || model.selected.Status != domain.StatusApplied {
		t.Fatalf("expected cancelled edit, got %+v", model)
	}
	if len(store.updated) != 0 {
		t.Fatalf("expected no store update, got %+v", store.updated)
	}
}

func TestListNavigationAndDetail(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(keyMsg("j"))
	model = updated.(Model)
	if model.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", model.cursor)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.view != viewDetail || model.selected.Company != "Globex" {
		t.Fatalf("expected Globex detail, got %+v", model.selected)
	}

	view := model.View()
	if !strings.Contains(view, "Globex") || !strings.Contains(view, "interviewing") {
		t.Fatal("expected detail view to render the selected application")
	}
}

func TestAddFormValidationAndSave(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(keyMsg("a"))
	model = updated.(Model)
	if model.view != viewAdd {
		t.Fatal("expected add view")
	}

	model.form.fields[fieldCompany].SetValue("Umbrella")
	model.form.fields[fieldTitle].SetValue("QA Engineer")
	model.form.fields[fieldStatus].SetValue("bogus")
	updatedModel, cmd := model.submitForm()
	model = updatedModel
	if cmd != nil || model.formErr == "" {
		t.Fatal("expected status validation error before saving")
	}

	model.form.fields[fieldStatus].SetValue("applied")
	model.form.fields[fieldAppliedAt].SetValue("2026-09-25")
	updatedModel, cmd = model.submitForm()
	model = updatedModel
	if cmd == nil {
		t.Fatal("expected save command for valid form")
	}
	saved := cmd()
	msg, ok := saved.(applicationSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected saved message, got %#v", saved)
	}
	if len(store.created) != 1 || store.created[0].Company != "Umbrella" {
		t.Fatalf("expected created application, got %+v", store.created)
	}
}

func TestAddFormShortensJobURL(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	shortener := &fakeShortener{}
	model := loadModelWith(t, store, shortener)

	updated, _ := model.Update(keyMsg("a"))
	model = updated.(Model)
	model.form.fields[fieldCompany].SetValue("Umbrella")
	model.form.fields[fieldTitle].SetValue("QA Engineer")
	model.form.fields[fieldJobURL].SetValue("boards.greenhouse.io/umbrella/jobs/1")

	updatedModel, cmd := model.submitForm()
	model = updatedModel
	if cmd == nil {
		t.Fatal("expected save command")
	}
	saved := cmd()
	msg, ok := saved.(applicationSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected saved message, got %#v", saved)
	}

	if len(shortener.shortened) != 1 || shortener.shortened[0] != "https://boards.greenhouse.io/umbrella/jobs/1" {
		t.Fatalf("expected normalized URL to be shortened, got %+v", shortener.shortened)
	}
	if len(store.created) != 1 || store.created[0].JobURL != "https://boards.greenhouse.io/umbrella/jobs/1" {
		t.Fatalf("expected the long URL to be stored, got %+v", store.created)
	}
	if msg.shortLink == "" {
		t.Fatal("expected a short link on the saved message")
	}

	updated, _ = model.Update(msg)
	model = updated.(Model)
	if !strings.Contains(model.notice, msg.shortLink) {
		t.Fatalf("expected notice to carry the short link, got %q", model.notice)
	}
}

func TestAddFormRejectsInvalidJobURL(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	shortener := &fakeShortener{}
	model := loadModelWith(t, store, shortener)

	updated, _ := model.Update(keyMsg("a"))
	model = updated.(Model)
	model.form.fields[fieldCompany].SetValue("Umbrella")
	model.form.fields[fieldTitle].SetValue("QA Engineer")
	model.form.fields[fieldJobURL].SetValue("ftp://files.example.com/job")

	updatedModel, cmd := model.submitForm()
	model = updatedModel
	if cmd != nil || !strings.Contains(model.formErr, "http and https") {
		t.Fatalf("expected scheme validation error, got %q", model.formErr)
	}
	if len(store.created) != 0 || len(shortener.shortened) != 0 {
		t.Fatal("expected nothing saved or shortened")
	}
}

func TestDetailShowsShortLink(t *testing.T) {
	store := &fakeStore{apps: []domain.Application{
		{ID: 1, Company: "Acme", JobTitle: "Backend Engineer", Status: domain.StatusApplied, JobURL: "https://acme.example.com/jobs/7"},
	}}
	shortener := &fakeShortener{links: map[string]string{
		"https://acme.example.com/jobs/7": "http://localhost:8080/s/aB3xK9z",
	}}
	model := loadModelWith(t, store, shortener)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected short link lookup command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if model.shortLink != "http://localhost:8080/s/aB3xK9z" {
		t.Fatalf("expected short link on the model, got %q", model.shortLink)
	}
	if view := model.View(); !strings.Contains(view, "http://localhost:8080/s/aB3xK9z") {
		t.Fatal("expected detail view to render the short link")
	}
}

func TestDeleteConfirmAndExecute(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(keyMsg("d"))
	model = updated.(Model)
	if !model.confirmingDelete || model.deleteTarget.Company != "Acme" {
		t.Fatalf("expected delete confirmation for Acme, got %+v", model)
	}
	if view := model.View(); !strings.Contains(view, "Delete application #1") {
		t.Fatal("expected confirmation panel to render")
	}

	updatedModel, cmd := model.Update(keyMsg("y"))
	model = updatedModel.(Model)
	if cmd == nil {
		t.Fatal("expected delete command")
	}
	deleted := cmd()
	msg, ok := deleted.(deleteSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected deleted message, got %#v", deleted)
	}

	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.confirmingDelete || !strings.Contains(model.notice, "Deleted application #1") {
		t.Fatalf("expected deleted notice, got %+v", model)
	}
	if len(store.deleted) != 1 || store.deleted[0] != 1 {
		t.Fatalf("expected store delete, got %+v", store.deleted)
	}
}

func TestDeleteCancel(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(keyMsg("d"))
	model = updated.(Model)
	updated, cmd := model.Update(keyMsg("n"))
	model = updated.(Model)

	if model.confirmingDelete || cmd != nil {
		t.Fatal("expected cancelled confirmation")
	}
	if len(store.deleted) != 0 {
		t.Fatalf("expected no store delete, got %+v", store.deleted)
	}
}

func TestDeleteError(t *testing.T) {
	store := &fakeStore{apps: testApps(), deleteErr: errors.New("boom")}
	model := loadModel(t, store)

	updated, _ := model.Update(keyMsg("d"))
	model = updated.(Model)
	updatedModel, cmd := model.Update(keyMsg("y"))
	model = updatedModel.(Model)
	if cmd == nil {
		t.Fatal("expected delete command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if !strings.Contains(model.opErr, "boom") {
		t.Fatalf("expected operation error, got %q", model.opErr)
	}
}

func TestExportToFile(t *testing.T) {
	store := &fakeStore{apps: testApps()}
	model := loadModel(t, store)

	updated, _ := model.Update(keyMsg("e"))
	model = updated.(Model)
	if !model.exporting {
		t.Fatal("expected export prompt")
	}

	path := t.TempDir() + string(os.PathSeparator) + "export.csv"
	model.exportInput.SetValue(path)
	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(Model)
	if cmd == nil {
		t.Fatal("expected export command")
	}
	exported := cmd()
	msg, ok := exported.(exportSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected exported message, got %#v", exported)
	}

	updated, _ = model.Update(msg)
	model = updated.(Model)
	if !strings.Contains(model.notice, path) {
		t.Fatalf("expected export notice, got %q", model.notice)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "ID,Company") || !strings.Contains(string(content), "Acme") {
		t.Fatalf("unexpected CSV content:\n%s", content)
	}
}
