package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/ryo246912/gh-actions-dash/internal/models"
)

// PreviewPanel represents the preview panel for workflow run details
type PreviewPanel struct {
	styles Styles
	width  int
	height int
}

// NewPreviewPanel creates a new preview panel
func NewPreviewPanel(styles Styles) *PreviewPanel {
	return &PreviewPanel{
		styles: styles,
	}
}

// SetSize sets the size of the preview panel
func (p *PreviewPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// RenderWorkflowRunPreview renders the workflow run preview with jobs and steps
func (p *PreviewPanel) RenderWorkflowRunPreview(run *models.WorkflowRun, jobs []models.Job) string {
	if run == nil {
		return p.renderEmpty()
	}

	var content strings.Builder
	
	// Header
	content.WriteString(p.styles.GetTitle().Render(fmt.Sprintf("Run #%d", run.RunNumber)))
	content.WriteString("\n\n")
	
	// Run info
	content.WriteString(p.styles.GetSubtitle().Render("Branch: "))
	content.WriteString(run.HeadBranch)
	content.WriteString("\n")
	
	content.WriteString(p.styles.GetSubtitle().Render("Event: "))
	content.WriteString(run.Event)
	content.WriteString("\n")
	
	content.WriteString(p.styles.GetSubtitle().Render("Started: "))
	content.WriteString(run.RunStartedAt.Format("2006-01-02 15:04:05"))
	content.WriteString("\n\n")
	
	// Jobs and steps
	if len(jobs) == 0 {
		content.WriteString(p.styles.GetStatusInProgress().Render("Loading jobs..."))
	} else {
		content.WriteString(p.styles.GetTitle().Render("Jobs & Steps"))
		content.WriteString("\n\n")
		
		for i, job := range jobs {
			if i > 0 {
				content.WriteString("\n")
			}
			content.WriteString(p.renderJobWithSteps(job))
		}
	}
	
	// Wrap in a bordered box
	boxContent := content.String()
	if len(boxContent) > 0 {
		return p.styles.GetContent().Width(p.width-2).Height(p.height-2).Render(boxContent)
	}
	
	return p.renderEmpty()
}

// renderJobWithSteps renders a job with its steps
func (p *PreviewPanel) renderJobWithSteps(job models.Job) string {
	var content strings.Builder
	
	// Job header
	jobStatus := job.Status
	if job.Status == "completed" {
		jobStatus = job.Conclusion
	}
	
	statusIcon := StatusIcon(jobStatus)
	statusStyle := p.styles.StatusStyle(jobStatus)
	
	// Calculate available width for job name based on panel width
	// Account for status icon (1 char) + " " (1 char) = 2 chars
	availableWidth := p.width - 6 // Panel width minus margins and icon space
	if availableWidth < 10 {
		availableWidth = 10 // Minimum reasonable width
	}
	
	jobName := job.Name
	if len(jobName) > availableWidth {
		jobName = jobName[:availableWidth-3] + "..."
	}
	
	content.WriteString(statusStyle.Render(fmt.Sprintf("%s %s", statusIcon, jobName)))
	content.WriteString("\n")
	
	// Duration if completed
	if !job.StartedAt.IsZero() && !job.CompletedAt.IsZero() {
		duration := job.CompletedAt.Sub(job.StartedAt)
		content.WriteString(p.styles.GetHelp().Render(fmt.Sprintf("  Duration: %v", duration.Round(time.Second))))
		content.WriteString("\n")
	}
	
	// Steps
	if len(job.Steps) > 0 {
		content.WriteString("\n")
		for _, step := range job.Steps {
			content.WriteString(p.renderStep(step))
		}
	}
	
	return content.String()
}

// renderStep renders a single step
func (p *PreviewPanel) renderStep(step models.Step) string {
	stepStatus := step.Status
	if step.Status == "completed" {
		stepStatus = step.Conclusion
	}
	
	statusIcon := StatusIcon(stepStatus)
	statusStyle := p.styles.StatusStyle(stepStatus)
	
	// Calculate available width for step name based on panel width
	// Account for: "  " (2 chars) + status icon (1 char) + " " (1 char) = 4 chars
	availableWidth := p.width - 8 // Panel width minus margins and icon space
	if availableWidth < 10 {
		availableWidth = 10 // Minimum reasonable width
	}
	
	stepName := step.Name
	if len(stepName) > availableWidth {
		stepName = stepName[:availableWidth-3] + "..."
	}
	
	return fmt.Sprintf("  %s %s\n", 
		statusStyle.Render(fmt.Sprintf("%s", statusIcon)), 
		stepName)
}

// RenderWorkflowPreview renders the workflow preview with basic information
func (p *PreviewPanel) RenderWorkflowPreview(workflow *models.Workflow) string {
	if workflow == nil {
		return p.renderEmpty()
	}

	var content strings.Builder
	
	// Header
	content.WriteString(p.styles.GetTitle().Render(workflow.Name))
	content.WriteString("\n\n")
	
	// Workflow info
	content.WriteString(p.styles.GetSubtitle().Render("File: "))
	// Extract filename from path
	pathParts := strings.Split(workflow.Path, "/")
	filename := pathParts[len(pathParts)-1]
	content.WriteString(filename)
	content.WriteString("\n")
	
	content.WriteString(p.styles.GetSubtitle().Render("State: "))
	if workflow.State == "active" {
		content.WriteString(p.styles.StatusStyle("success").Render("Active"))
	} else {
		content.WriteString(p.styles.StatusStyle("warning").Render(workflow.State))
	}
	content.WriteString("\n")
	
	content.WriteString(p.styles.GetSubtitle().Render("Created: "))
	content.WriteString(workflow.CreatedAt.Format("2006-01-02 15:04:05"))
	content.WriteString("\n")
	
	content.WriteString(p.styles.GetSubtitle().Render("Updated: "))
	content.WriteString(workflow.UpdatedAt.Format("2006-01-02 15:04:05"))
	content.WriteString("\n\n")
	
	// Recent activity hint
	content.WriteString(p.styles.GetHelp().Render("💡 Press Enter to view recent runs for this workflow"))
	
	// Wrap in a bordered box
	boxContent := content.String()
	if len(boxContent) > 0 {
		return p.styles.GetContent().Width(p.width-2).Height(p.height-2).Render(boxContent)
	}
	
	return p.renderEmpty()
}

// renderEmpty renders an empty state
func (p *PreviewPanel) renderEmpty() string {
	emptyText := p.styles.GetHelp().Render("Select an item to see details")
	return p.styles.GetContent().Width(p.width-2).Height(p.height-2).Render(emptyText)
}