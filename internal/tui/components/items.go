package components

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ryo246912/gh-actions-dash/internal/models"
)

// WorkflowItem represents a workflow in the list
type WorkflowItem struct {
	Workflow models.Workflow
}

// FilterValue returns the value to filter on
func (w WorkflowItem) FilterValue() string {
	return w.Workflow.Name
}

// Styles interface for avoiding circular dependency
type Styles interface {
	StatusStyle(status string) lipgloss.Style
	ListItem() lipgloss.Style
	SelectedItem() lipgloss.Style
	GetTitle() lipgloss.Style
	GetSubtitle() lipgloss.Style
	GetHelp() lipgloss.Style
	GetContent() lipgloss.Style
	GetStatusInProgress() lipgloss.Style
}

// StatusIcon returns an appropriate icon for a status
func StatusIcon(status string) string {
	switch status {
	case "success", "completed":
		return "✓"
	case "failure", "failed":
		return "✗"
	case "pending", "queued":
		return "⏳"
	case "in_progress", "running":
		return "⏵"
	case "skipped":
		return "⊘"
	default:
		return "○"
	}
}

// GetCIStatus returns a detailed CI status based on workflow run status and conclusion
func GetCIStatus(status, conclusion string) string {
	if status == "completed" {
		return conclusion
	} else {
		return status
	}
}

// WorkflowItemDelegate handles rendering of workflow items
type WorkflowItemDelegate struct {
	styles Styles
}

// NewWorkflowItemDelegate creates a new workflow item delegate
func NewWorkflowItemDelegate(styles Styles) *WorkflowItemDelegate {
	return &WorkflowItemDelegate{styles: styles}
}

// Height returns the height of the item
func (d *WorkflowItemDelegate) Height() int {
	return 1
}

// Spacing returns the spacing between items
func (d *WorkflowItemDelegate) Spacing() int {
	return 0
}

// Update handles updates to the item
func (d *WorkflowItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// Render renders the workflow item
func (d *WorkflowItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(WorkflowItem)
	if !ok {
		return
	}

	workflow := item.Workflow

	// Status icon and color
	statusIcon := StatusIcon(workflow.State)
	statusStyle := d.styles.StatusStyle(workflow.State)

	// Format the item with fuller display
	name := workflow.Name
	if len(name) > 50 {
		name = name[:47] + "..."
	}

	// Extract workflow filename from path
	pathParts := strings.Split(workflow.Path, "/")
	filename := pathParts[len(pathParts)-1]
	if len(filename) > 30 {
		filename = filename[:27] + "..."
	}

	// Build the display string
	status := statusStyle.Render(fmt.Sprintf("%s %s", statusIcon, workflow.State))

	// Single line: status, name, and filename
	line := fmt.Sprintf("%s %s • %s", status, name, filename)

	// Apply selection styling
	if index == m.Index() {
		line = d.styles.SelectedItem().Render(line)
	} else {
		line = d.styles.ListItem().Render(line)
	}

	// Write the output
	_, _ = fmt.Fprint(w, line)
}

// WorkflowRunItem represents a workflow run in the list
type WorkflowRunItem struct {
	Run models.WorkflowRun
}

// FilterValue returns the value to filter on
func (w WorkflowRunItem) FilterValue() string {
	return fmt.Sprintf("%s #%d", w.Run.Name, w.Run.RunNumber)
}

// WorkflowRunItemDelegate handles rendering of workflow run items
type WorkflowRunItemDelegate struct {
	styles Styles
}

// NewWorkflowRunItemDelegate creates a new workflow run item delegate
func NewWorkflowRunItemDelegate(styles Styles) *WorkflowRunItemDelegate {
	return &WorkflowRunItemDelegate{styles: styles}
}

// Height returns the height of the item
func (d *WorkflowRunItemDelegate) Height() int {
	return 1
}

// Spacing returns the spacing between items
func (d *WorkflowRunItemDelegate) Spacing() int {
	return 1
}

// Update handles updates to the item
func (d *WorkflowRunItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// Render renders the workflow run item in table format
func (d *WorkflowRunItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(WorkflowRunItem)
	if !ok {
		return
	}

	run := item.Run

	// Get detailed CI status
	ciStatus := GetCIStatus(run.Status, run.Conclusion)

	// Status icon and color based on conclusion if completed, otherwise status
	var statusIcon string
	var statusStyle lipgloss.Style

	if run.Status == "completed" {
		statusIcon = StatusIcon(run.Conclusion)
		statusStyle = d.styles.StatusStyle(run.Conclusion)
	} else {
		statusIcon = StatusIcon(run.Status)
		statusStyle = d.styles.StatusStyle(run.Status)
	}

	// Workflow name with run number (truncated)
	name := fmt.Sprintf("%s(#%d)", run.Name, run.RunNumber)
	if len(name) > 25 {
		name = name[:22] + "..."
	}
	name = fmt.Sprintf("%-25s", name)

	// Status column (without styling yet)
	statusText := fmt.Sprintf("%s %-10s", statusIcon, ciStatus)

	// Branch name (truncated)
	branch := run.HeadBranch
	if len(branch) > 18 {
		branch = branch[:15] + "..."
	}
	branch = fmt.Sprintf("%-18s", branch)

	// Actor name (truncated)
	actor := run.Actor.Login
	if len(actor) > 15 {
		actor = actor[:12] + "..."
	}
	actor = fmt.Sprintf("%-15s", actor)

	// PR information (truncated)
	prInfo := ""
	if len(run.PullRequests) > 0 {
		pr := run.PullRequests[0]
		prInfo = fmt.Sprintf("#%d", pr.Number)
		if len(pr.Title) > 8 {
			prInfo = fmt.Sprintf("#%d:%s...", pr.Number, pr.Title[:4])
		} else if pr.Title != "" {
			prInfo = fmt.Sprintf("#%d:%s", pr.Number, pr.Title)
		}
	}
	if prInfo == "" {
		prInfo = "-"
	}
	prInfo = fmt.Sprintf("%-12s", prInfo)

	// Duration
	durationStr := ""
	if run.Status == "completed" && !run.RunStartedAt.IsZero() && !run.UpdatedAt.IsZero() {
		duration := run.UpdatedAt.Sub(run.RunStartedAt)
		if duration > 0 {
			if duration < time.Minute {
				durationStr = fmt.Sprintf("%.0fs", duration.Seconds())
			} else if duration < time.Hour {
				durationStr = fmt.Sprintf("%.0fm", duration.Minutes())
			} else {
				durationStr = fmt.Sprintf("%.1fh", duration.Hours())
			}
		}
	}
	if durationStr == "" {
		durationStr = "-"
	}
	durationStr = fmt.Sprintf("%-6s", durationStr)

	// Time formatting
	timeStr := run.CreatedAt.Format("01-02 15:04")

	// Build table row
	line := fmt.Sprintf("%s %s %s %s %s %s %s",
		name, statusText, branch, actor, prInfo, durationStr, timeStr)

	// Apply selection styling to the entire line, then apply status color to just the status part
	if index == m.Index() {
		line = d.styles.SelectedItem().Render(line)
	} else {
		// For non-selected items, apply status color to the status part
		parts := []string{name, statusStyle.Render(statusText), branch, actor, prInfo, durationStr, timeStr}
		line = strings.Join(parts, " ")
		line = d.styles.ListItem().Render(line)
	}

	_, _ = fmt.Fprint(w, line)
}
