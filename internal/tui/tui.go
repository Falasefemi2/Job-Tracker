package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Falasefemi2/jobtracker/internal/domain"
	"github.com/Falasefemi2/jobtracker/internal/shortener"
)

type applicationStore interface {
	List(ctx context.Context) ([]domain.Application, error)
	Stats(ctx context.Context) (domain.Stats, error)
	GetByID(ctx context.Context, id int64) (domain.Application, error)
	Create(ctx context.Context, a domain.Application) (int64, error)
	Update(ctx context.Context, id int64, a domain.Application) error
	Delete(ctx context.Context, id int64) error
	ExportAsCSV(ctx context.Context, w io.Writer) error
}

type linkShortener interface {
	Shorten(ctx context.Context, rawURL string) (string, error)
	Lookup(ctx context.Context, longURL string) (string, error)
}

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
	viewAdd
)

const (
	fieldCompany = iota
	fieldTitle
	fieldStatus
	fieldLocation
	fieldSource
	fieldJobURL
	fieldNotes
	fieldAppliedAt
	fieldCount
)

type formModel struct {
	fields []textinput.Model
	index  int
}

type Model struct {
	ctx       context.Context
	store     applicationStore
	shortener linkShortener
	view      viewMode
	apps      []domain.Application
	filter    []domain.Application
	stats     domain.Stats

	cursor int
	offset int
	width  int
	height int

	statuses  []string
	statusIdx int

	searching bool
	search    textinput.Model
	query     string

	selected  domain.Application
	shortLink string
	form      formModel
	formErr   string

	statusEditing bool
	pendingStatus domain.Status
	statusErr     string

	confirmingDelete bool
	deleteTarget     domain.Application
	exporting        bool
	exportInput      textinput.Model
	opErr            string

	loading bool
	notice  string
	err     error
}

type dataLoadedMsg struct {
	apps  []domain.Application
	stats domain.Stats
	err   error
}

type applicationSavedMsg struct {
	id        int64
	shortLink string
	err       error
}

type shortLinkMsg struct {
	link string
	err  error
}

type statusSavedMsg struct {
	app domain.Application
	err error
}

type deleteSavedMsg struct {
	id  int64
	err error
}

type exportSavedMsg struct {
	path string
	err  error
}

func NewModel(ctx context.Context, store applicationStore, shortener linkShortener) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	search := textinput.New()
	search.Placeholder = "Filter by company, title, or notes"
	search.Prompt = "Filter: "
	search.CharLimit = 64
	search.Width = 40

	exportInput := textinput.New()
	exportInput.Placeholder = "applications.csv"
	exportInput.Prompt = "File: "
	exportInput.CharLimit = 160
	exportInput.Width = 52

	return Model{
		ctx:       ctx,
		store:     store,
		shortener: shortener,
		statuses: []string{
			"all",
			string(domain.StatusApplied),
			string(domain.StatusScreening),
			string(domain.StatusInterviewing),
			string(domain.StatusOffer),
			string(domain.StatusAccepted),
			string(domain.StatusRejected),
			string(domain.StatusWithdrawn),
		},
		search:      search,
		exportInput: exportInput,
		form:        newForm(),
		loading:     true,
	}
}

func (m *Model) clearOpErr() {
	m.opErr = ""
}

func Run(ctx context.Context, store applicationStore, shortener linkShortener) error {
	if store == nil {
		return errors.New("application store is required")
	}
	if shortener == nil {
		return errors.New("link shortener is required")
	}
	program := tea.NewProgram(NewModel(ctx, store, shortener), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newForm() formModel {
	placeholders := []string{
		"Acme Corp",
		"Backend Engineer",
		"applied",
		"Lagos / Remote",
		"LinkedIn",
		"https://example.com/jobs/123",
		"Recruiter reached out on Tuesday",
		"2026-09-25",
	}
	fields := make([]textinput.Model, 0, fieldCount)
	for i, placeholder := range placeholders {
		input := textinput.New()
		input.Placeholder = placeholder
		input.Prompt = "> "
		input.CharLimit = 160
		input.Width = 52
		if i == 0 {
			input.Focus()
		}
		fields = append(fields, input)
	}
	return formModel{fields: fields}
}

func (m Model) Init() tea.Cmd {
	return m.reload()
}

func (m Model) reload() tea.Cmd {
	store := m.store
	ctx := m.ctx
	return func() tea.Msg {
		apps, err := store.List(ctx)
		if err != nil {
			return dataLoadedMsg{err: err}
		}
		stats, err := store.Stats(ctx)
		if err != nil {
			return dataLoadedMsg{err: err}
		}
		return dataLoadedMsg{apps: apps, stats: stats}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case dataLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.apps = msg.apps
		m.stats = msg.stats
		m.applyFilter()
		return m, nil
	case applicationSavedMsg:
		if msg.err != nil {
			m.formErr = msg.err.Error()
			m.view = viewAdd
			return m, nil
		}
		m.form = newForm()
		m.formErr = ""
		m.view = viewList
		m.notice = "Created application #" + itoa(int(msg.id))
		if msg.shortLink != "" {
			m.notice += " — " + msg.shortLink
		}
		m.loading = true
		return m, m.reload()
	case shortLinkMsg:
		if msg.err != nil {
			m.opErr = "Could not resolve short link: " + msg.err.Error()
			return m, nil
		}
		m.opErr = ""
		m.shortLink = msg.link
		return m, nil
	case statusSavedMsg:
		if msg.err != nil {
			m.statusErr = "Could not save status: " + msg.err.Error()
			return m, nil
		}
		m.statusEditing = false
		m.statusErr = ""
		m.selected = msg.app
		m.notice = fmt.Sprintf("Updated #%d to %s", msg.app.ID, msg.app.Status)
		m.loading = true
		return m, m.reload()
	case deleteSavedMsg:
		if msg.err != nil {
			m.opErr = "Could not delete: " + msg.err.Error()
			return m, nil
		}
		m.opErr = ""
		m.view = viewList
		m.notice = fmt.Sprintf("Deleted application #%d", msg.id)
		m.loading = true
		return m, m.reload()
	case exportSavedMsg:
		if msg.err != nil {
			m.opErr = "Could not export: " + msg.err.Error()
			return m, nil
		}
		m.opErr = ""
		m.notice = "Exported applications to " + msg.path
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.confirmingDelete {
			return m.updateConfirm(msg)
		}
		if m.exporting {
			return m.updateExportPrompt(msg)
		}
		switch m.view {
		case viewDetail:
			return m.updateDetail(msg)
		case viewAdd:
			return m.updateForm(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searching {
		switch msg.String() {
		case "enter", "esc":
			m.searching = false
			m.search.Blur()
			m.applyFilter()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.query = m.search.Value()
		m.applyFilter()
		return m, cmd
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
		m.notice = ""
		m.clearOpErr()
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		m.notice = ""
		m.clearOpErr()
		return m, nil
	case "/":
		m.searching = true
		m.notice = ""
		return m, m.search.Focus()
	case "s":
		m.statusIdx = (m.statusIdx + 1) % len(m.statuses)
		m.notice = ""
		m.applyFilter()
		return m, nil
	case "enter":
		if len(m.filter) == 0 {
			return m, nil
		}
		m.selected = m.filter[m.cursor]
		m.view = viewDetail
		m.notice = ""
		m.shortLink = ""
		return m, m.lookupShortLink(m.selected)
	case "a":
		m.form = newForm()
		m.formErr = ""
		m.view = viewAdd
		m.notice = ""
		m.clearOpErr()
		return m, m.form.fields[0].Focus()
	case "e":
		m.exporting = true
		m.exportInput.SetValue(defaultExportFilename())
		m.notice = ""
		m.clearOpErr()
		return m, m.exportInput.Focus()
	case "d":
		if len(m.filter) == 0 {
			return m, nil
		}
		m.deleteTarget = m.filter[m.cursor]
		m.confirmingDelete = true
		m.notice = ""
		m.clearOpErr()
		return m, nil
	case "r":
		m.loading = true
		m.notice = ""
		m.clearOpErr()
		return m, m.reload()
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y":
		target := m.deleteTarget
		store := m.store
		ctx := m.ctx
		m.confirmingDelete = false
		return m, func() tea.Msg {
			if err := store.Delete(ctx, target.ID); err != nil {
				return deleteSavedMsg{err: err}
			}
			return deleteSavedMsg{id: target.ID}
		}
	case "n", "esc":
		m.confirmingDelete = false
		return m, nil
	}
	return m, nil
}

func (m Model) updateExportPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.exporting = false
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.exportInput.Value())
		if path == "" {
			path = defaultExportFilename()
		}
		m.exporting = false
		store := m.store
		ctx := m.ctx
		return m, func() tea.Msg {
			f, err := os.Create(path)
			if err != nil {
				return exportSavedMsg{err: err}
			}
			exportErr := store.ExportAsCSV(ctx, f)
			closeErr := f.Close()
			if exportErr != nil {
				return exportSavedMsg{err: exportErr}
			}
			if closeErr != nil {
				return exportSavedMsg{err: closeErr}
			}
			return exportSavedMsg{path: path}
		}
	}
	var cmd tea.Cmd
	m.exportInput, cmd = m.exportInput.Update(msg)
	return m, cmd
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "b":
		if m.statusEditing {
			m.statusEditing = false
			m.statusErr = ""
			return m, nil
		}
		m.view = viewList
		m.clearOpErr()
		return m, nil
	case "enter":
		if m.statusEditing {
			return m.saveStatus()
		}
		return m, nil
	case "d":
		if !m.statusEditing {
			m.deleteTarget = m.selected
			m.confirmingDelete = true
			m.notice = ""
			m.clearOpErr()
		}
		return m, nil
	case "u":
		if !m.statusEditing {
			m.statusEditing = true
			m.pendingStatus = m.selected.Status
			m.statusErr = ""
			m.notice = ""
		}
		return m, nil
	case "left", "h":
		if m.statusEditing {
			m.pendingStatus = nextStatus(m.pendingStatus, -1)
		}
		return m, nil
	case "right", "l":
		if m.statusEditing {
			m.pendingStatus = nextStatus(m.pendingStatus, 1)
		}
		return m, nil
	case "up", "k":
		if !m.statusEditing {
			m.moveSelection(-1)
			return m, m.lookupShortLink(m.selected)
		}
		return m, nil
	case "down", "j":
		if !m.statusEditing {
			m.moveSelection(1)
			return m, m.lookupShortLink(m.selected)
		}
		return m, nil
	case "r":
		m.statusEditing = false
		m.statusErr = ""
		m.loading = true
		m.view = viewList
		return m, m.reload()
	}
	return m, nil
}

func (m Model) saveStatus() (Model, tea.Cmd) {
	updated := m.selected
	updated.Status = m.pendingStatus
	store := m.store
	ctx := m.ctx
	return m, func() tea.Msg {
		if err := store.Update(ctx, updated.ID, updated); err != nil {
			return statusSavedMsg{err: err}
		}
		return statusSavedMsg{app: updated}
	}
}

func nextStatus(current domain.Status, delta int) domain.Status {
	order := []domain.Status{
		domain.StatusApplied,
		domain.StatusScreening,
		domain.StatusInterviewing,
		domain.StatusOffer,
		domain.StatusAccepted,
		domain.StatusRejected,
		domain.StatusWithdrawn,
	}
	index := 0
	for i, status := range order {
		if status == current {
			index = i
			break
		}
	}
	index = (index + delta + len(order)) % len(order)
	return order[index]
}

func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		m.formErr = ""
		return m, nil
	case "up", "shift+tab":
		m.focusField((m.form.index - 1 + len(m.form.fields)) % len(m.form.fields))
		return m, nil
	case "tab", "down":
		m.focusField((m.form.index + 1) % len(m.form.fields))
		return m, nil
	case "enter":
		if m.form.index < len(m.form.fields)-1 {
			m.focusField(m.form.index + 1)
			return m, nil
		}
		return m.submitForm()
	}

	var cmd tea.Cmd
	m.form.fields[m.form.index], cmd = m.form.fields[m.form.index].Update(msg)
	return m, cmd
}

func (m *Model) focusField(index int) {
	m.form.fields[m.form.index].Blur()
	m.form.index = index
	m.form.fields[m.form.index].Focus()
}

func (m Model) submitForm() (Model, tea.Cmd) {
	company := strings.TrimSpace(m.form.fields[fieldCompany].Value())
	title := strings.TrimSpace(m.form.fields[fieldTitle].Value())
	if company == "" {
		m.formErr = "Company is required"
		return m, nil
	}
	if title == "" {
		m.formErr = "Job title is required"
		return m, nil
	}
	status, err := parseStatus(m.form.fields[fieldStatus].Value())
	if err != nil {
		m.formErr = err.Error()
		return m, nil
	}
	appliedAt, err := parseDate(m.form.fields[fieldAppliedAt].Value())
	if err != nil {
		m.formErr = err.Error()
		return m, nil
	}
	m.formErr = ""
	rawJobURL := strings.TrimSpace(m.form.fields[fieldJobURL].Value())
	jobURL, err := shortener.Normalize(rawJobURL)
	if err != nil {
		m.formErr = err.Error()
		return m, nil
	}
	app := domain.Application{
		Company:   company,
		JobTitle:  title,
		Status:    status,
		Location:  strings.TrimSpace(m.form.fields[fieldLocation].Value()),
		Source:    strings.TrimSpace(m.form.fields[fieldSource].Value()),
		JobURL:    jobURL,
		Notes:     strings.TrimSpace(m.form.fields[fieldNotes].Value()),
		AppliedAt: appliedAt,
	}
	store := m.store
	ctx := m.ctx
	shortener := m.shortener
	return m, func() tea.Msg {
		link := ""
		if shortener != nil {
			if link, err = shortener.Shorten(ctx, jobURL); err != nil {
				return applicationSavedMsg{err: err}
			}
		}
		id, err := store.Create(ctx, app)
		return applicationSavedMsg{id: id, shortLink: link, err: err}
	}
}

func (m Model) lookupShortLink(app domain.Application) tea.Cmd {
	if m.shortener == nil || strings.TrimSpace(app.JobURL) == "" {
		return nil
	}
	shortener := m.shortener
	ctx := m.ctx
	return func() tea.Msg {
		link, err := shortener.Lookup(ctx, app.JobURL)
		return shortLinkMsg{link: link, err: err}
	}
}

func (m *Model) moveCursor(delta int) {
	if len(m.filter) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filter) {
		m.cursor = len(m.filter) - 1
	}
	m.offset, _ = visibleWindow(m.cursor, m.offset, m.pageSize(), len(m.filter))
}

func (m *Model) moveSelection(delta int) {
	if len(m.filter) == 0 {
		return
	}
	index := 0
	for i, app := range m.filter {
		if app.ID == m.selected.ID {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(m.filter) {
		index = len(m.filter) - 1
	}
	m.cursor = index
	m.selected = m.filter[index]
	m.offset, _ = visibleWindow(m.cursor, m.offset, m.pageSize(), len(m.filter))
}

func (m *Model) applyFilter() {
	m.filter = filterApplications(m.apps, m.query, m.statuses[m.statusIdx])
	m.cursor = 0
	m.offset = 0
}

func (m Model) pageSize() int {
	height := m.height - 18
	if height < 3 {
		height = 3
	}
	return height
}
