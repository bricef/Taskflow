package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
)

var (
	detailTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	detailSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).MarginTop(1)
	detailFieldLabel   = lipgloss.NewStyle().Bold(true).Width(12)
	detailFieldValue   = lipgloss.NewStyle()
)

// taskDetailData holds the fetched data for the detail pane.
type taskDetailData struct {
	task         model.Task
	comments     []model.Comment
	dependencies []model.Dependency
	attachments  []model.Attachment
	audit        []model.AuditEntry
	related      map[string]taskSummary // ref → summary for dep tree
}

type taskDetailLoaded struct {
	data taskDetailData
	err  error
}

func fetchTaskDetail(client *httpclient.Client, boardSlug string, num int) tea.Cmd {
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(num)}

		detail, err := httpclient.GetOne[model.TaskDetail](client, model.ResTaskDetail, tp, nil)
		if err != nil {
			return taskDetailLoaded{err: err}
		}

		// Fetch summaries for related tasks in the dependency tree.
		currentRef := fmt.Sprintf("%s/%d", boardSlug, num)
		related := map[string]taskSummary{}
		for _, dep := range detail.Dependencies {
			sourceRef := fmt.Sprintf("%s/%d", dep.BoardSlug, dep.TaskNum)
			targetRef := fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum)
			for _, ref := range []string{sourceRef, targetRef} {
				if ref == currentRef {
					continue
				}
				if _, ok := related[ref]; ok {
					continue
				}
				var rBoard string
				var rNum int
				if idx := strings.LastIndex(ref, "/"); idx >= 0 {
					rBoard = ref[:idx]
					fmt.Sscanf(ref[idx+1:], "%d", &rNum)
				}
				if rBoard != "" && rNum > 0 {
					rp := httpclient.PathParams{"slug": rBoard, "num": fmt.Sprint(rNum)}
					if t, err := httpclient.GetOne[model.Task](client, model.ResTaskGet, rp, nil); err == nil {
						related[ref] = taskSummary{
							BoardSlug: t.BoardSlug,
							Num:       t.Num,
							Title:     t.Title,
							State:     t.State,
						}
					}
				}
			}
		}

		return taskDetailLoaded{data: taskDetailData{
			task: detail.Task, comments: detail.Comments, dependencies: detail.Dependencies,
			attachments: detail.Attachments, audit: detail.Audit, related: related,
		}}
	}
}

// commentPosted is sent when a comment is successfully created.
type commentPosted struct {
	comment model.Comment
	err     error
}

// detailModel is the task detail overlay.
type detailModel struct {
	data    *taskDetailData
	loading bool
	err     error
	vp      viewport.Model
	content string // cached rendered content
	// Comment input
	commenting bool
	input      textarea.Model
	postErr    string
}

func (m *detailModel) startComment() {
	ti := textarea.New()
	ti.Placeholder = "Write a comment..."
	ti.CharLimit = 4000
	ti.SetWidth(60)
	ti.SetHeight(4)
	ti.Focus()
	m.input = ti
	m.commenting = true
	m.postErr = ""
	m.content = "" // invalidate cache so comment input renders
}

func (m *detailModel) submitComment(client *httpclient.Client) tea.Cmd {
	body := strings.TrimSpace(m.input.Value())
	if body == "" {
		m.commenting = false
		return nil
	}
	task := m.data.task
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": task.BoardSlug, "num": fmt.Sprint(task.Num)}
		comment, err := httpclient.Exec[model.Comment](client,model.OpCommentCreate, tp, map[string]string{"body": body})
		return commentPosted{comment: comment, err: err}
	}
}

func (m *detailModel) update(msg tea.Msg) tea.Cmd {
	if !m.commenting {
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *detailModel) scrollUp() {
	m.ensureViewport(0, 0)
	m.vp.ScrollUp(1)
}

func (m *detailModel) scrollDown() {
	m.ensureViewport(0, 0)
	m.vp.ScrollDown(1)
}

func (m *detailModel) ensureViewport(width, height int) {
	if width > 0 && height > 0 {
		m.vp.Width = width
		m.vp.Height = height
	}
	if m.content == "" && m.data != nil {
		m.content = m.render(m.vp.Width, m.vp.Height)
		m.vp.SetContent(m.content)
	}
}

func (m *detailModel) invalidateContent() {
	m.content = ""
}

func (m detailModel) view(width, height int) string {
	if m.loading {
		return dimStyle.Render("Loading task details...")
	}
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}
	if m.data == nil {
		return ""
	}

	// Update viewport dimensions and content.
	m.vp.Width = width
	m.vp.Height = height
	if m.content == "" {
		m.content = m.render(width, height)
		m.vp.SetContent(m.content)
	}

	return m.vp.View()
}

func (m detailModel) render(width, height int) string {
	d := m.data
	t := d.task
	var b strings.Builder

	// Title.
	b.WriteString(detailTitleStyle.Render(fmt.Sprintf("%s/%d: %s", t.BoardSlug, t.Num, t.Title)) + "\n\n")

	// Metadata fields.
	b.WriteString(field("State", t.State))
	b.WriteString(field("Priority", string(t.Priority)))
	assignee := "unassigned"
	if t.Assignee != nil {
		assignee = *t.Assignee
	}
	b.WriteString(field("Assignee", assignee))
	if len(t.Tags) > 0 {
		b.WriteString(field("Tags", strings.Join(t.Tags, ", ")))
	}
	if t.DueDate != nil {
		b.WriteString(field("Due", t.DueDate.Format("2006-01-02")))
	}
	b.WriteString(field("Created", fmt.Sprintf("%s by %s", t.CreatedAt.Format("2006-01-02 15:04"), t.CreatedBy)))

	// Description.
	if t.Description != "" {
		b.WriteString(detailSectionStyle.Render("Description") + "\n")
		descWidth := width - 2
		if descWidth < 20 {
			descWidth = 20
		}
		for _, line := range strings.Split(t.Description, "\n") {
			b.WriteString("  " + wrapLine(line, descWidth, "  ") + "\n")
		}
	}

	// Dependency tree.
	if len(d.dependencies) > 0 {
		b.WriteString(detailSectionStyle.Render("Dependencies") + "\n")
		b.WriteString(buildDepTree(t, d.dependencies, d.related))
	}

	// Attachments.
	if len(d.attachments) > 0 {
		b.WriteString(detailSectionStyle.Render("Attachments") + "\n")
		for _, att := range d.attachments {
			b.WriteString(fmt.Sprintf("  [%s] %s — %s\n", dimStyle.Render(string(att.RefType)), att.Label, att.Reference))
		}
	}

	// Comments.
	if len(d.comments) > 0 {
		b.WriteString(detailSectionStyle.Render(fmt.Sprintf("Comments (%d)", len(d.comments))) + "\n")
		indentLen := 2 + 16 + 1 // "  " + "YYYY-MM-DD HH:MM" + " "
		for _, c := range d.comments {
			ts := c.CreatedAt.Format("2006-01-02 15:04")
			prefixLen := indentLen + len(c.Actor) + 2 // + actor + ": "
			prefix := fmt.Sprintf("  %s %s: ", dimStyle.Render(ts), c.Actor)
			indent := strings.Repeat(" ", prefixLen)
			bodyWidth := width - prefixLen
			if bodyWidth < 20 {
				bodyWidth = 20
			}
			lines := strings.Split(c.Body, "\n")
			b.WriteString(prefix + wrapLine(lines[0], bodyWidth, indent) + "\n")
			for _, line := range lines[1:] {
				b.WriteString(indent + wrapLine(line, bodyWidth, indent) + "\n")
			}
		}
	}

	// Comment input.
	if m.commenting {
		b.WriteString(detailSectionStyle.Render("New Comment") + "\n")
		b.WriteString(m.input.View() + "\n")
		if m.postErr != "" {
			b.WriteString(errorStyle.Render(m.postErr) + "\n")
		}
	}

	// Audit (last 10).
	if len(d.audit) > 0 {
		b.WriteString(detailSectionStyle.Render("Recent Activity") + "\n")
		start := 0
		if len(d.audit) > 10 {
			start = len(d.audit) - 10
		}
		for _, a := range d.audit[start:] {
			ts := a.CreatedAt.Format("2006-01-02 15:04")
			detail := ""
			if len(a.Detail) > 0 && string(a.Detail) != "{}" {
				var m map[string]any
				if json.Unmarshal(a.Detail, &m) == nil {
					if from, ok := m["from"]; ok {
						detail = fmt.Sprintf(" (%v → %v)", from, m["to"])
					}
				}
			}
			b.WriteString(fmt.Sprintf("  %s  %-12s  %s%s\n", dimStyle.Render(ts), a.Actor, string(a.Action), detail))
		}
	}

	return b.String()
}

func field(label, value string) string {
	return fmt.Sprintf("%s %s\n", detailFieldLabel.Render(label+":"), detailFieldValue.Render(value))
}

// wrapLine word-wraps a single line to maxWidth, joining continuation lines
// with the given indent prefix.
func wrapLine(line string, maxWidth int, indent string) string {
	if len(line) <= maxWidth {
		return line
	}
	var result strings.Builder
	remaining := line
	first := true
	for len(remaining) > 0 {
		if !first {
			result.WriteString("\n" + indent)
		}
		if len(remaining) <= maxWidth {
			result.WriteString(remaining)
			break
		}
		// Find last space within maxWidth.
		cut := strings.LastIndex(remaining[:maxWidth], " ")
		if cut <= 0 {
			// No space found — hard break.
			cut = maxWidth
		}
		result.WriteString(remaining[:cut])
		remaining = strings.TrimLeft(remaining[cut:], " ")
		first = false
	}
	return result.String()
}
