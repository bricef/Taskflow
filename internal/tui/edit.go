package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
)

// Edit form field indices.
const (
	editFieldTitle = iota
	editFieldDesc
	editFieldPriority
	editFieldAssignee
	editFieldTags
	editFieldDueDate
	editFieldDeps
	editFieldAttach
	editFieldCount
)

var priorities = []model.Priority{
	model.PriorityNone,
	model.PriorityLow,
	model.PriorityMedium,
	model.PriorityHigh,
	model.PriorityCritical,
}

var depTypes = []model.DependencyType{
	model.DependencyTypeDependsOn,
	model.DependencyTypeRelatesTo,
}

var refTypes = []model.RefType{
	model.RefTypeURL,
	model.RefTypeFile,
	model.RefTypeGitCommit,
	model.RefTypeGitBranch,
	model.RefTypeGitPR,
}

// editResult is sent when the edit form saves.
type editResult struct {
	task model.Task
	err  error
}

// depAdded / depRemoved are sent after dependency API calls.
type depAdded struct {
	dep model.Dependency
	err error
}

type depRemoved struct {
	id  int
	err error
}

type depTasksLoaded struct {
	tasks []model.Task
	err   error
}

type attachAdded struct {
	attach model.Attachment
	err    error
}

type attachRemoved struct {
	id  int
	err error
}

// editModel is a form overlay for editing task fields.
type editModel struct {
	task        model.Task // original task for diffing
	boardSlug   string
	currentUser *model.Actor
	field       int // currently focused field

	titleInput   textinput.Model
	descInput    textarea.Model
	priority     model.Priority
	assignee     *string // resolved name or nil
	tagsInput    textinput.Model
	dueDateInput textinput.Model

	// Assignee selector sub-state.
	selectingAssignee bool
	actors            []string
	actorFilter       textinput.Model
	actorCursor       int

	// Dependencies sub-state.
	deps      []model.Dependency
	depCursor int
	addingDep    bool
	depType      int // index into depTypes
	depTasks     []model.Task    // all non-terminal tasks for the selector
	depFilter    textinput.Model // filter for task ref selector
	depSelCursor int

	// Attachments sub-state.
	attachments    []model.Attachment
	attachCursor   int
	addingAttach   bool
	attachRefType  int // index into refTypes
	attachRef      textinput.Model
	attachLabel    textinput.Model
	attachField    int // 0=type, 1=reference, 2=label

	err string
}

func newEdit(client *httpclient.Client, boardSlug string, task model.Task, currentUser *model.Actor, deps []model.Dependency, attachments []model.Attachment) (*editModel, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "Task title"
	ti.CharLimit = 200
	ti.Width = 50
	ti.SetValue(task.Title)
	ti.Focus()

	desc := textarea.New()
	desc.Placeholder = "Description..."
	desc.CharLimit = 4000
	desc.MaxWidth = 60
	desc.MaxHeight = 6
	desc.ShowLineNumbers = false
	desc.SetValue(task.Description)

	tags := textinput.New()
	tags.Placeholder = "tag1, tag2, ..."
	tags.CharLimit = 200
	tags.Width = 50
	if len(task.Tags) > 0 {
		tags.SetValue(strings.Join(task.Tags, ", "))
	}

	dueDate := textinput.New()
	dueDate.Placeholder = "YYYY-MM-DD"
	dueDate.CharLimit = 10
	dueDate.Width = 12
	if task.DueDate != nil {
		dueDate.SetValue(task.DueDate.Format("2006-01-02"))
	}

	af := textinput.New()
	af.Placeholder = "type to filter..."
	af.CharLimit = 50
	af.Width = 30

	df := textinput.New()
	df.Placeholder = "type to filter tasks..."
	df.CharLimit = 50
	df.Width = 30

	ar := textinput.New()
	ar.Placeholder = "https://... or path"
	ar.CharLimit = 200
	ar.Width = 40

	al := textinput.New()
	al.Placeholder = "Label"
	al.CharLimit = 100
	al.Width = 30

	m := &editModel{
		task:         task,
		boardSlug:    boardSlug,
		currentUser:  currentUser,
		titleInput:   ti,
		descInput:    desc,
		priority:     task.Priority,
		assignee:     task.Assignee,
		tagsInput:    tags,
		dueDateInput: dueDate,
		actorFilter:  af,
		deps:         deps,
		depFilter:    df,
		attachments:  attachments,
		attachRef:    ar,
		attachLabel:  al,
	}

	// Fetch actors and all tasks (for dependency selector) in parallel.
	return m, tea.Batch(
		func() tea.Msg {
			actors, err := httpclient.GetMany[model.Actor](client, model.ResActorList, nil, nil)
			return actorsLoaded{actors: actors, err: err}
		},
		func() tea.Msg {
			tasks, err := httpclient.GetMany[model.Task](client, model.ResTaskSearch, nil, nil)
			return depTasksLoaded{tasks: tasks, err: err}
		},
	)
}

func (m *editModel) assigneeDisplay() string {
	if m.assignee == nil {
		return dimStyle.Render("unassigned")
	}
	if m.currentUser != nil && *m.assignee == m.currentUser.Name {
		return meStyle.Render("@me")
	}
	return *m.assignee
}

func (m *editModel) focusField() tea.Cmd {
	m.titleInput.Blur()
	m.descInput.Blur()
	m.tagsInput.Blur()
	m.dueDateInput.Blur()
	m.depFilter.Blur()

	switch m.field {
	case editFieldTitle:
		return m.titleInput.Focus()
	case editFieldDesc:
		return m.descInput.Focus()
	case editFieldTags:
		return m.tagsInput.Focus()
	case editFieldDueDate:
		return m.dueDateInput.Focus()
	}
	return nil
}

func (m *editModel) nextField() tea.Cmd {
	m.field = (m.field + 1) % editFieldCount
	return m.focusField()
}

func (m *editModel) prevField() tea.Cmd {
	m.field = (m.field - 1 + editFieldCount) % editFieldCount
	return m.focusField()
}

func (m *editModel) cyclePriority(dir int) {
	for i, p := range priorities {
		if p == m.priority {
			next := (i + dir + len(priorities)) % len(priorities)
			m.priority = priorities[next]
			return
		}
	}
	m.priority = model.PriorityNone
}

func (m *editModel) filteredActors() []string {
	query := strings.ToLower(m.actorFilter.Value())
	if query == "" {
		return m.actors
	}
	var result []string
	for _, name := range m.actors {
		display := name
		if name == assignMeSentinel && m.currentUser != nil {
			display = fmt.Sprintf("@me (%s)", m.currentUser.Name)
		}
		if strings.Contains(strings.ToLower(display), query) {
			result = append(result, name)
		}
	}
	return result
}

func (m *editModel) filteredDepTasks() []model.Task {
	query := strings.ToLower(m.depFilter.Value())
	var result []model.Task
	for _, t := range m.depTasks {
		// Exclude the task being edited.
		if t.BoardSlug == m.boardSlug && t.Num == m.task.Num {
			continue
		}
		ref := fmt.Sprintf("%s/%d", t.BoardSlug, t.Num)
		if query == "" || strings.Contains(strings.ToLower(ref), query) || strings.Contains(strings.ToLower(t.Title), query) {
			result = append(result, t)
		}
	}
	return result
}

func (m *editModel) addSelectedDep(client *httpclient.Client) tea.Cmd {
	filtered := m.filteredDepTasks()
	if m.depSelCursor < 0 || m.depSelCursor >= len(filtered) {
		return nil
	}
	target := filtered[m.depSelCursor]
	dt := depTypes[m.depType]
	boardSlug := m.boardSlug
	taskNum := m.task.Num
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(taskNum)}
		dep, err := httpclient.Exec[model.Dependency](client, model.OpDepCreate, tp, map[string]any{
			"depends_on_board": target.BoardSlug,
			"depends_on_num":   target.Num,
			"dep_type":         string(dt),
		})
		return depAdded{dep: dep, err: err}
	}
}

func (m *editModel) removeDep(client *httpclient.Client) tea.Cmd {
	if m.depCursor < 0 || m.depCursor >= len(m.deps) {
		return nil
	}
	dep := m.deps[m.depCursor]
	return func() tea.Msg {
		err := httpclient.ExecNoResult(client, model.OpDepDelete, httpclient.PathParams{"id": fmt.Sprint(dep.ID)}, nil)
		return depRemoved{id: dep.ID, err: err}
	}
}

func (m *editModel) addAttach(client *httpclient.Client) tea.Cmd {
	ref := strings.TrimSpace(m.attachRef.Value())
	label := strings.TrimSpace(m.attachLabel.Value())
	if ref == "" {
		m.err = "Reference must not be empty"
		return nil
	}
	if label == "" {
		m.err = "Label must not be empty"
		return nil
	}
	rt := refTypes[m.attachRefType]
	boardSlug := m.boardSlug
	taskNum := m.task.Num
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(taskNum)}
		att, err := httpclient.Exec[model.Attachment](client, model.OpAttachCreate, tp, map[string]any{
			"ref_type":  string(rt),
			"reference": ref,
			"label":     label,
		})
		return attachAdded{attach: att, err: err}
	}
}

func (m *editModel) removeAttach(client *httpclient.Client) tea.Cmd {
	if m.attachCursor < 0 || m.attachCursor >= len(m.attachments) {
		return nil
	}
	att := m.attachments[m.attachCursor]
	return func() tea.Msg {
		err := httpclient.ExecNoResult(client, model.OpAttachDelete, httpclient.PathParams{"id": fmt.Sprint(att.ID)}, nil)
		return attachRemoved{id: att.ID, err: err}
	}
}

func (m *editModel) save(client *httpclient.Client) tea.Cmd {
	body := map[string]any{}

	if m.titleInput.Value() != m.task.Title {
		body["title"] = m.titleInput.Value()
	}
	if m.descInput.Value() != m.task.Description {
		body["description"] = m.descInput.Value()
	}
	if m.priority != m.task.Priority {
		body["priority"] = string(m.priority)
	}

	origAssignee := ""
	if m.task.Assignee != nil {
		origAssignee = *m.task.Assignee
	}
	newAssignee := ""
	if m.assignee != nil {
		newAssignee = *m.assignee
	}
	if origAssignee != newAssignee {
		body["assignee"] = m.assignee
	}

	newTags := parseTags(m.tagsInput.Value())
	if !tagsEqual(newTags, m.task.Tags) {
		body["tags"] = newTags
	}

	dueDateStr := strings.TrimSpace(m.dueDateInput.Value())
	origDueDate := ""
	if m.task.DueDate != nil {
		origDueDate = m.task.DueDate.Format("2006-01-02")
	}
	if dueDateStr != origDueDate {
		if dueDateStr == "" {
			body["due_date"] = nil
		} else {
			t, err := time.Parse("2006-01-02", dueDateStr)
			if err != nil {
				m.err = "Invalid date format (use YYYY-MM-DD)"
				return nil
			}
			body["due_date"] = t.Format(time.RFC3339)
		}
	}

	if len(body) == 0 {
		return func() tea.Msg {
			return editResult{task: m.task}
		}
	}

	boardSlug := m.boardSlug
	num := m.task.Num
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(num)}
		task, err := httpclient.Exec[model.Task](client, model.OpTaskUpdate, tp, body)
		return editResult{task: task, err: err}
	}
}

func (m *editModel) update(msg tea.Msg, client *httpclient.Client) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case actorsLoaded:
		if msg.err != nil {
			return false, nil
		}
		m.actors = []string{"(unassign)"}
		if m.currentUser != nil {
			m.actors = append(m.actors, assignMeSentinel)
		}
		for _, a := range msg.actors {
			if a.Active && (m.currentUser == nil || a.Name != m.currentUser.Name) {
				m.actors = append(m.actors, a.Name)
			}
		}

	case depTasksLoaded:
		if msg.err == nil {
			m.depTasks = msg.tasks
		}
		return false, nil

	case depAdded:
		m.addingDep = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.deps = append(m.deps, msg.dep)
			m.err = ""
		}
		return false, nil

	case attachAdded:
		m.addingAttach = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.attachments = append(m.attachments, msg.attach)
			m.err = ""
		}
		return false, nil

	case attachRemoved:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			for i, a := range m.attachments {
				if a.ID == msg.id {
					m.attachments = append(m.attachments[:i], m.attachments[i+1:]...)
					break
				}
			}
			if m.attachCursor >= len(m.attachments) {
				m.attachCursor = len(m.attachments) - 1
			}
			if m.attachCursor < 0 {
				m.attachCursor = 0
			}
			m.err = ""
		}
		return false, nil

	case depRemoved:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			for i, d := range m.deps {
				if d.ID == msg.id {
					m.deps = append(m.deps[:i], m.deps[i+1:]...)
					break
				}
			}
			if m.depCursor >= len(m.deps) {
				m.depCursor = len(m.deps) - 1
			}
			if m.depCursor < 0 {
				m.depCursor = 0
			}
			m.err = ""
		}
		return false, nil

	case tea.KeyMsg:
		// Assignee sub-selector mode.
		if m.selectingAssignee {
			return m.updateAssigneeSelector(msg)
		}

		// Adding dependency mode.
		if m.addingDep {
			return m.updateAddDep(msg, client)
		}

		// Adding attachment mode.
		if m.addingAttach {
			return m.updateAddAttach(msg, client)
		}

		// Attachments field: navigate, add, remove.
		if m.field == editFieldAttach {
			switch msg.String() {
			case "ctrl+c":
				return true, nil
			case "esc":
				return true, nil
			case "enter":
				return false, m.save(client)
			case "tab":
				return false, m.nextField()
			case "shift+tab":
				return false, m.prevField()
			case "up", "k":
				if m.attachCursor > 0 {
					m.attachCursor--
				}
				return false, nil
			case "down", "j":
				if m.attachCursor < len(m.attachments)-1 {
					m.attachCursor++
				}
				return false, nil
			case "n":
				m.addingAttach = true
				m.attachRefType = 0
				m.attachField = 1 // start on reference
				m.attachRef.SetValue("")
				m.attachLabel.SetValue("")
				m.err = ""
				return false, m.attachRef.Focus()
			case "x":
				return false, m.removeAttach(client)
			}
			return false, nil
		}

		// Dependencies field: navigate, add, remove.
		if m.field == editFieldDeps {
			switch msg.String() {
			case "ctrl+c":
				return true, nil
			case "esc":
				return true, nil
			case "enter":
				return false, m.save(client)
			case "tab":
				return false, m.nextField()
			case "shift+tab":
				return false, m.prevField()
			case "up", "k":
				if m.depCursor > 0 {
					m.depCursor--
				}
				return false, nil
			case "down", "j":
				if m.depCursor < len(m.deps)-1 {
					m.depCursor++
				}
				return false, nil
			case "n":
				m.addingDep = true
				m.depFilter.SetValue("")
				m.depSelCursor = 0
				m.depType = 0
				m.err = ""
				return false, m.depFilter.Focus()
			case "x":
				return false, m.removeDep(client)
			}
			return false, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return true, nil
		case "esc":
			return true, nil
		case "enter":
			if m.field == editFieldAssignee {
				m.selectingAssignee = true
				m.actorFilter.SetValue("")
				m.actorCursor = 0
				return false, m.actorFilter.Focus()
			}
			return false, m.save(client)
		case "tab":
			return false, m.nextField()
		case "shift+tab":
			return false, m.prevField()
		case "left":
			if m.field == editFieldPriority {
				m.cyclePriority(-1)
				return false, nil
			}
		case "right":
			if m.field == editFieldPriority {
				m.cyclePriority(1)
				return false, nil
			}
		case "ctrl+j":
			if m.field == editFieldDesc {
				m.descInput.InsertString("\n")
				return false, nil
			}
		}

		// Delegate to active input.
		var cmd tea.Cmd
		switch m.field {
		case editFieldTitle:
			m.titleInput, cmd = m.titleInput.Update(msg)
		case editFieldDesc:
			m.descInput, cmd = m.descInput.Update(msg)
		case editFieldTags:
			m.tagsInput, cmd = m.tagsInput.Update(msg)
		case editFieldDueDate:
			m.dueDateInput, cmd = m.dueDateInput.Update(msg)
		}
		return false, cmd

	case editResult:
		if msg.err != nil {
			m.err = msg.err.Error()
			return false, nil
		}
		return true, nil
	}

	return false, nil
}

func (m *editModel) updateAddDep(msg tea.KeyMsg, client *httpclient.Client) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addingDep = false
		m.err = ""
		return false, m.focusField()
	case "enter":
		return false, m.addSelectedDep(client)
	case "up":
		if m.depSelCursor > 0 {
			m.depSelCursor--
		}
		return false, nil
	case "down":
		filtered := m.filteredDepTasks()
		if m.depSelCursor < len(filtered)-1 {
			m.depSelCursor++
		}
		return false, nil
	case "left":
		m.depType = (m.depType - 1 + len(depTypes)) % len(depTypes)
		return false, nil
	case "right":
		m.depType = (m.depType + 1) % len(depTypes)
		return false, nil
	}
	prevValue := m.depFilter.Value()
	var cmd tea.Cmd
	m.depFilter, cmd = m.depFilter.Update(msg)
	if m.depFilter.Value() != prevValue {
		m.depSelCursor = 0
	}
	return false, cmd
}

func (m *editModel) updateAddAttach(msg tea.KeyMsg, client *httpclient.Client) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addingAttach = false
		m.err = ""
		return false, m.focusField()
	case "enter":
		return false, m.addAttach(client)
	case "tab":
		// Cycle through: type(0) → reference(1) → label(2)
		m.attachRef.Blur()
		m.attachLabel.Blur()
		m.attachField = (m.attachField + 1) % 3
		switch m.attachField {
		case 1:
			return false, m.attachRef.Focus()
		case 2:
			return false, m.attachLabel.Focus()
		}
		return false, nil
	case "shift+tab":
		m.attachRef.Blur()
		m.attachLabel.Blur()
		m.attachField = (m.attachField + 2) % 3 // -1 mod 3
		switch m.attachField {
		case 1:
			return false, m.attachRef.Focus()
		case 2:
			return false, m.attachLabel.Focus()
		}
		return false, nil
	case "left":
		if m.attachField == 0 {
			m.attachRefType = (m.attachRefType - 1 + len(refTypes)) % len(refTypes)
			return false, nil
		}
	case "right":
		if m.attachField == 0 {
			m.attachRefType = (m.attachRefType + 1) % len(refTypes)
			return false, nil
		}
	}
	var cmd tea.Cmd
	switch m.attachField {
	case 1:
		m.attachRef, cmd = m.attachRef.Update(msg)
	case 2:
		m.attachLabel, cmd = m.attachLabel.Update(msg)
	}
	return false, cmd
}

func (m *editModel) updateAssigneeSelector(msg tea.KeyMsg) (bool, tea.Cmd) {
	filtered := m.filteredActors()
	switch msg.String() {
	case "esc":
		m.selectingAssignee = false
		return false, m.focusField()
	case "up":
		if m.actorCursor > 0 {
			m.actorCursor--
		}
		return false, nil
	case "down":
		if m.actorCursor < len(filtered)-1 {
			m.actorCursor++
		}
		return false, nil
	case "enter":
		if m.actorCursor < len(filtered) {
			selected := filtered[m.actorCursor]
			if selected == "(unassign)" {
				m.assignee = nil
			} else if selected == assignMeSentinel && m.currentUser != nil {
				name := m.currentUser.Name
				m.assignee = &name
			} else {
				m.assignee = &selected
			}
		}
		m.selectingAssignee = false
		return false, m.focusField()
	}

	prevValue := m.actorFilter.Value()
	var cmd tea.Cmd
	m.actorFilter, cmd = m.actorFilter.Update(msg)
	if m.actorFilter.Value() != prevValue {
		m.actorCursor = 0
	}
	return false, cmd
}

var (
	editBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")).
			Padding(1, 2)
	editLabelStyle  = lipgloss.NewStyle().Bold(true).Width(14)
	editActiveLabel = lipgloss.NewStyle().Bold(true).Width(14).Foreground(lipgloss.Color("39"))
)

func (m editModel) view(width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Edit %s/%d", m.boardSlug, m.task.Num)) + "\n")
	b.WriteString(dimStyle.Render(m.task.Title) + "\n\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}

	if m.selectingAssignee {
		b.WriteString(editActiveLabel.Render("Assignee:") + "\n")
		b.WriteString(m.actorFilter.View() + "\n\n")
		filtered := m.filteredActors()
		for i, name := range filtered {
			cursor := "  "
			style := dimStyle
			if i == m.actorCursor {
				cursor = "▸ "
				style = transitionSelected
			}
			display := name
			if name == assignMeSentinel && m.currentUser != nil {
				display = fmt.Sprintf("@me (%s)", m.currentUser.Name)
			}
			b.WriteString(style.Render(cursor+display) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("enter select  esc back"))
	} else if m.addingAttach {
		b.WriteString(editActiveLabel.Render("Add Attachment") + "\n\n")
		typeLabel := editLabelStyle
		refLabel := editLabelStyle
		labelLabel := editLabelStyle
		switch m.attachField {
		case 0:
			typeLabel = editActiveLabel
		case 1:
			refLabel = editActiveLabel
		case 2:
			labelLabel = editActiveLabel
		}
		b.WriteString(typeLabel.Render("Type:") + " " + renderRefTypeSelector(m.attachRefType, m.attachField == 0) + "\n")
		b.WriteString(refLabel.Render("Reference:") + " " + m.attachRef.View() + "\n")
		b.WriteString(labelLabel.Render("Label:") + " " + m.attachLabel.View() + "\n")
		b.WriteString("\n" + dimStyle.Render("enter add  tab next field  ←/→ type  esc back"))
	} else if m.addingDep {
		b.WriteString(editActiveLabel.Render("Add Dependency") + "\n\n")
		b.WriteString(editLabelStyle.Render("Type:") + " " + renderDepTypeSelector(m.depType, true) + "\n\n")
		b.WriteString(m.depFilter.View() + "\n\n")
		filtered := m.filteredDepTasks()
		maxShow := 8
		if len(filtered) < maxShow {
			maxShow = len(filtered)
		}
		start := 0
		if m.depSelCursor >= maxShow {
			start = m.depSelCursor - maxShow + 1
		}
		end := start + maxShow
		if end > len(filtered) {
			end = len(filtered)
		}
		for i := start; i < end; i++ {
			t := filtered[i]
			ref := fmt.Sprintf("%s/%d", t.BoardSlug, t.Num)
			cursor := "  "
			style := dimStyle
			if i == m.depSelCursor {
				cursor = "▸ "
				style = lipgloss.NewStyle()
			}
			b.WriteString(style.Render(fmt.Sprintf("%s%-20s %s", cursor, ref, truncate(t.Title, 30))) + "\n")
		}
		if len(filtered) == 0 {
			b.WriteString(dimStyle.Render("  No matching tasks.") + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("enter add  ←/→ type  ↑/↓ select  esc back"))
	} else {
		for i := 0; i < editFieldCount; i++ {
			label := editLabelStyle
			if i == m.field {
				label = editActiveLabel
			}
			switch i {
			case editFieldTitle:
				b.WriteString(label.Render("Title:") + " " + m.titleInput.View() + "\n")
			case editFieldDesc:
				b.WriteString(label.Render("Description:") + "\n" + m.descInput.View() + "\n")
			case editFieldPriority:
				b.WriteString(label.Render("Priority:") + " " + renderPrioritySelector(m.priority, i == m.field) + "\n")
			case editFieldAssignee:
				hint := ""
				if i == m.field {
					hint = dimStyle.Render("  (enter to change)")
				}
				b.WriteString(label.Render("Assignee:") + " " + m.assigneeDisplay() + hint + "\n")
			case editFieldTags:
				b.WriteString(label.Render("Tags:") + " " + m.tagsInput.View() + "\n")
			case editFieldDueDate:
				b.WriteString(label.Render("Due Date:") + " " + m.dueDateInput.View() + "\n")
			case editFieldDeps:
				b.WriteString(label.Render("Dependencies:") + "\n")
				if len(m.deps) == 0 {
					b.WriteString("  " + dimStyle.Render("none") + "\n")
				} else {
					for di, dep := range m.deps {
						ref := fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum)
						cursor := "  "
						style := dimStyle
						if i == m.field && di == m.depCursor {
							cursor = "▸ "
							style = lipgloss.NewStyle()
						}
						b.WriteString(style.Render(fmt.Sprintf("%s%s  %s", cursor, ref, dep.DependencyType)) + "\n")
					}
				}
				if i == m.field {
					b.WriteString(dimStyle.Render("  n add  x remove  ↑/↓ navigate") + "\n")
					b.WriteString(dimStyle.Render("  (dependency changes are saved immediately)") + "\n")
				}
			case editFieldAttach:
				b.WriteString(label.Render("Attachments:") + "\n")
				if len(m.attachments) == 0 {
					b.WriteString("  " + dimStyle.Render("none") + "\n")
				} else {
					for ai, att := range m.attachments {
						cursor := "  "
						style := dimStyle
						if i == m.field && ai == m.attachCursor {
							cursor = "▸ "
							style = lipgloss.NewStyle()
						}
						b.WriteString(style.Render(fmt.Sprintf("%s[%s] %s — %s", cursor, att.RefType, att.Label, att.Reference)) + "\n")
					}
				}
				if i == m.field {
					b.WriteString(dimStyle.Render("  n add  x remove  ↑/↓ navigate") + "\n")
					b.WriteString(dimStyle.Render("  (attachment changes are saved immediately)") + "\n")
				}
			}
		}
		b.WriteString("\n" + dimStyle.Render("tab next  shift+tab prev  enter save  esc cancel"))
	}

	boxWidth := width * 2 / 3
	if boxWidth < 50 {
		boxWidth = 50
	}
	return editBorder.Width(boxWidth).Render(b.String())
}

// cycleSelector renders a horizontal list of options with the current one
// highlighted. When focused, shows a ←/→ hint.
func cycleSelector(options []string, current int, focused bool, styleSelected func(string) string) string {
	var parts []string
	for i, opt := range options {
		if i == current {
			parts = append(parts, styleSelected("[ "+opt+" ]"))
		} else {
			parts = append(parts, dimStyle.Render(opt))
		}
	}
	hint := ""
	if focused {
		hint = dimStyle.Render("  ←/→")
	}
	return strings.Join(parts, "  ") + hint
}

func renderPrioritySelector(current model.Priority, focused bool) string {
	options := make([]string, len(priorities))
	idx := 0
	for i, p := range priorities {
		options[i] = string(p)
		if p == current {
			idx = i
		}
	}
	return cycleSelector(options, idx, focused, func(s string) string {
		if st, ok := priorityStyle[current]; ok {
			return st.Bold(true).Render(s)
		}
		return s
	})
}

func renderRefTypeSelector(current int, focused bool) string {
	options := make([]string, len(refTypes))
	for i, rt := range refTypes {
		options[i] = string(rt)
	}
	return cycleSelector(options, current, focused, func(s string) string { return transitionSelected.Render(s) })
}

func renderDepTypeSelector(current int, focused bool) string {
	options := make([]string, len(depTypes))
	for i, dt := range depTypes {
		options[i] = string(dt)
	}
	return cycleSelector(options, current, focused, func(s string) string { return transitionSelected.Render(s) })
}

func parseTags(s string) []string {
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func tagsEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
