package tui

import (
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Falasefemi2/jobtracker/internal/domain"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A78BFA"))
	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))
	statLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))
	statValueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F9FAFB"))
	headerCellStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#9CA3AF"))
	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7C3AED"))
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4C1D95")).
			Padding(1, 2)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F87171")).
			Bold(true)
	noticeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34D399"))
	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))
	fieldLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))
	activeFieldLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22D3EE")).
				Bold(true)
)

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	sections := []string{
		titleStyle.Render("JOBTRACKER"),
		subtitleStyle.Render("Application dashboard  •  " + strings.TrimSpace(m.queryStatusLine())),
		m.renderStats(),
		m.renderBody(width),
		hintStyle.Render(m.hints()),
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
}

func (m Model) queryStatusLine() string {
	if m.loading {
		return "loading applications…"
	}
	if m.err != nil {
		return "database error"
	}
	parts := []string{`filter: ` + m.statuses[m.statusIdx]}
	if strings.TrimSpace(m.query) != "" {
		parts = append(parts, `search: "`+strings.TrimSpace(m.query)+`"`)
	}
	parts = append(parts, `showing `+itoa(len(m.filter))+` of `+itoa(len(m.apps)))
	if strings.TrimSpace(m.notice) != "" {
		return noticeStyle.Render(strings.Join(parts, "  •  ") + "  •  " + m.notice)
	}
	return strings.Join(parts, "  •  ")
}

func (m Model) renderStats() string {
	cards := []string{
		statCard("TOTAL", itoa(m.stats.Total)),
		statCard("APPLIED", itoa(m.stats.Applied)),
		statCard("INTERVIEWING", itoa(m.stats.Interviewing)),
		statCard("OFFERS", itoa(m.stats.Offers)),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func statCard(label, value string) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		statLabelStyle.Render(label),
		statValueStyle.Render(value),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#312E81")).
		Padding(0, 2).
		MarginRight(1).
		Render(body)
}

func (m Model) renderBody(width int) string {
	if m.loading {
		return panelStyle.Width(panelWidth(width)).Render("Loading applications…")
	}
	if m.err != nil {
		return panelStyle.Width(panelWidth(width)).Render(errorStyle.Render("Database error: " + m.err.Error()))
	}
	if m.confirmingDelete {
		return m.renderConfirm(width)
	}
	if m.exporting {
		return m.renderExportPrompt(width)
	}
	switch m.view {
	case viewDetail:
		return m.renderDetail(width)
	case viewAdd:
		return m.renderForm(width)
	default:
		return m.renderTable(width)
	}
}

func (m Model) renderTable(width int) string {
	if len(m.filter) == 0 {
		return panelStyle.Width(panelWidth(width)).Render("No applications match the current filter.\nPress / to search, s to change status, or a to add one.")
	}
	idWidth, companyWidth, titleWidth, statusWidth, dateWidth := tableColumns(width)
	header := headerCellStyle.Render(
		padRight("ID", idWidth) + "  " +
			padRight("COMPANY", companyWidth) + "  " +
			padRight("TITLE", titleWidth) + "  " +
			padRight("STATUS", statusWidth) + "  " +
			padRight("APPLIED", dateWidth),
	)
	offset, end := visibleWindow(m.cursor, m.offset, m.pageSize(), len(m.filter))
	rows := make([]string, 0, end-offset+2)
	rows = append(rows, header)
	if strings.TrimSpace(m.opErr) != "" {
		rows = append(rows, errorStyle.Render(m.opErr))
	}
	for i := offset; i < end; i++ {
		rows = append(rows, m.renderRow(m.filter[i], i == m.cursor, idWidth, companyWidth, titleWidth, statusWidth, dateWidth))
	}
	return panelStyle.Width(panelWidth(width)).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func tableColumns(width int) (id, company, title, status, date int) {
	id, status, date = 5, 13, 12
	remaining := width - id - status - date - 14
	if remaining < 30 {
		remaining = 30
	}
	company = remaining * 38 / 100
	title = remaining - company
	if company < 12 {
		company = 12
	}
	if title < 14 {
		title = 14
	}
	return id, company, title, status, date
}

func (m Model) renderRow(app domain.Application, selected bool, idWidth, companyWidth, titleWidth, statusWidth, dateWidth int) string {
	line := padRight(itoa(int(app.ID)), idWidth) + "  " +
		padRight(truncate(app.Company, companyWidth), companyWidth) + "  " +
		padRight(truncate(app.JobTitle, titleWidth), titleWidth) + "  " +
		padRight(truncate(string(app.Status), statusWidth), statusWidth) + "  " +
		padRight(formatDate(app.AppliedAt), dateWidth)
	if selected {
		return selectedRowStyle.Render(line)
	}
	return statusTextStyle(app.Status).Render(line)
}

func statusTextStyle(status domain.Status) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(statusColor(status))
}

func statusColor(status domain.Status) lipgloss.TerminalColor {
	switch status {
	case domain.StatusApplied:
		return lipgloss.Color("#38BDF8")
	case domain.StatusScreening:
		return lipgloss.Color("#FBBF24")
	case domain.StatusInterviewing:
		return lipgloss.Color("#A78BFA")
	case domain.StatusOffer:
		return lipgloss.Color("#34D399")
	case domain.StatusAccepted:
		return lipgloss.Color("#4ADE80")
	case domain.StatusRejected:
		return lipgloss.Color("#F87171")
	case domain.StatusWithdrawn:
		return lipgloss.Color("#9CA3AF")
	default:
		return lipgloss.Color("#E5E7EB")
	}
}

func (m Model) renderDetail(width int) string {
	app := m.selected
	status := string(app.Status)
	if m.statusEditing {
		status = activeFieldLabelStyle.Render(string(m.pendingStatus)) +
			hintStyle.Render("  ←/→ choose • enter saves • esc cancels")
	}
	rows := []string{
		detailRow("Company", app.Company),
		detailRow("Title", app.JobTitle),
		detailRow("Status", status),
		detailRow("Location", fallback(app.Location)),
		detailRow("Source", fallback(app.Source)),
		detailRow("Job URL", fallback(app.JobURL)),
		detailRow("Short Link", fallback(m.shortLink)),
		detailRow("Notes", fallback(app.Notes)),
		detailRow("Applied", formatDate(app.AppliedAt)),
		detailRow("Updated", formatDate(app.UpdatedAt)),
	}
	if strings.TrimSpace(m.statusErr) != "" {
		rows = append(rows, errorStyle.Render(m.statusErr))
	}
	if strings.TrimSpace(m.opErr) != "" {
		rows = append(rows, errorStyle.Render(m.opErr))
	}
	title := titleStyle.Render("Application #"+itoa(int(app.ID))) + "\n" +
		lipgloss.JoinVertical(lipgloss.Left, rows...)
	return panelStyle.Width(panelWidth(width)).Render(title)
}

func detailRow(label, value string) string {
	return fieldLabelStyle.Render(padRight(label+":", 10)) + " " + value
}

func (m Model) renderForm(width int) string {
	labels := []string{"Company", "Title", "Status", "Location", "Source", "Job URL", "Notes", "Applied"}
	rows := make([]string, 0, len(m.form.fields)+2)
	for i, field := range m.form.fields {
		label := fieldLabelStyle
		if i == m.form.index {
			label = activeFieldLabelStyle
		}
		rows = append(rows, label.Render(padRight(labels[i], 10))+field.View())
	}
	if strings.TrimSpace(m.formErr) != "" {
		rows = append(rows, errorStyle.Render(m.formErr))
	}
	rows = append(rows, hintStyle.Render("tab/enter next • shift+tab back • enter on Applied saves • esc cancels"))
	rows = append(rows, hintStyle.Render("A job URL is shortened automatically and stored as a shareable link."))
	return panelStyle.Width(panelWidth(width)).Render(
		titleStyle.Render("Add application") + "\n" +
			lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

func (m Model) renderConfirm(width int) string {
	target := m.deleteTarget
	return panelStyle.Width(panelWidth(width)).Render(
		errorStyle.Render("Delete application #"+itoa(int(target.ID))+"?") + "\n" +
			target.Company + " — " + target.JobTitle + "\n" +
			hintStyle.Render("This cannot be undone.  y deletes • n/esc cancels"),
	)
}

func (m Model) renderExportPrompt(width int) string {
	return panelStyle.Width(panelWidth(width)).Render(
		titleStyle.Render("Export as CSV") + "\n" +
			m.exportInput.View() + "\n" +
			hintStyle.Render("enter saves • esc cancels"),
	)
}

func defaultExportFilename() string {
	return "applications-" + time.Now().Format("20060102-150405") + ".csv"
}

func (m Model) hints() string {
	if m.confirmingDelete {
		return "y confirm delete • n/esc cancel"
	}
	if m.exporting {
		return "enter saves file • esc cancels"
	}
	switch m.view {
	case viewDetail:
		if m.statusEditing {
			return "←/→ choose status • enter save • esc cancel • q quit"
		}
		return "↑/↓ move • u change status • d delete • esc back • r refresh • q quit"
	case viewAdd:
		return "Fill every required field, then save from the Applied row."
	default:
		if m.searching {
			return "Type to filter • enter/esc done"
		}
		return "↑/↓ move • enter details • / search • s status • a add • e export • d delete • r refresh • q quit"
	}
}

func panelWidth(width int) int {
	if width < 48 {
		return width
	}
	return width - 4
}

func filterApplications(apps []domain.Application, query, status string) []domain.Application {
	query = strings.ToLower(strings.TrimSpace(query))
	status = strings.ToLower(strings.TrimSpace(status))
	filtered := make([]domain.Application, 0, len(apps))
	for _, app := range apps {
		if status != "" && status != "all" && strings.ToLower(string(app.Status)) != status {
			continue
		}
		if query == "" {
			filtered = append(filtered, app)
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			app.Company, app.JobTitle, app.Location, app.Source, app.Notes,
		}, " "))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, app)
		}
	}
	return filtered
}

func visibleWindow(cursor, offset, height, count int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	if height < 1 {
		height = 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= count {
		cursor = count - 1
	}
	if offset < 0 {
		offset = 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+height {
		offset = cursor - height + 1
	}
	if maxOffset := count - height; offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + height
	if end > count {
		end = count
	}
	return offset, end
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func padRight(value string, width int) string {
	runes := []rune(value)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return value + strings.Repeat(" ", width-len(runes))
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.Format("Jan 02, 2006")
}

func fallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func parseStatus(value string) (domain.Status, error) {
	switch status := domain.Status(strings.ToLower(strings.TrimSpace(value))); status {
	case "":
		return domain.StatusApplied, nil
	case domain.StatusApplied,
		domain.StatusScreening,
		domain.StatusInterviewing,
		domain.StatusOffer,
		domain.StatusAccepted,
		domain.StatusRejected,
		domain.StatusWithdrawn:
		return status, nil
	default:
		return "", errors.New(`invalid status: use applied, screening, interviewing, offer, accepted, rejected, or withdrawn`)
	}
}

func parseDate(value string) (time.Time, error) {
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
	return time.Time{}, errors.New("invalid date: use YYYY-MM-DD, YYYY-MM-DD HH:MM:SS, or RFC3339")
}

func itoa(value int) string {
	return digits(value)
}

func digits(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var out []byte
	for value > 0 {
		out = append([]byte{byte('0' + value%10)}, out...)
		value /= 10
	}
	if negative {
		out = append([]byte{'-'}, out...)
	}
	return string(out)
}
