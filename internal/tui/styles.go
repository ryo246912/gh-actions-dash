package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles defines the styling for the TUI
type Styles struct {
	// Base styles
	Base     lipgloss.Style
	Title    lipgloss.Style
	Subtitle lipgloss.Style

	// List styles
	List         lipgloss.Style
	listItem     lipgloss.Style
	selectedItem lipgloss.Style

	// Status styles
	StatusSuccess    lipgloss.Style
	StatusFailure    lipgloss.Style
	StatusPending    lipgloss.Style
	StatusInProgress lipgloss.Style
	StatusSkipped    lipgloss.Style

	// Border styles
	Border       lipgloss.Style
	ActiveBorder lipgloss.Style

	// Content styles
	Content lipgloss.Style
	Sidebar lipgloss.Style

	// Help styles
	Help     lipgloss.Style
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style
}

// ListItem returns the list item style
func (s Styles) ListItem() lipgloss.Style {
	return s.listItem
}

// SelectedItem returns the selected item style
func (s Styles) SelectedItem() lipgloss.Style {
	return s.selectedItem
}

// GetTitle returns the title style
func (s Styles) GetTitle() lipgloss.Style {
	return s.Title
}

// GetSubtitle returns the subtitle style
func (s Styles) GetSubtitle() lipgloss.Style {
	return s.Subtitle
}

// GetHelp returns the help style
func (s Styles) GetHelp() lipgloss.Style {
	return s.Help
}

// GetContent returns the content style
func (s Styles) GetContent() lipgloss.Style {
	return s.Content
}

// GetStatusInProgress returns the in-progress status style
func (s Styles) GetStatusInProgress() lipgloss.Style {
	return s.StatusInProgress
}

// DefaultStyles returns default styling
func DefaultStyles() Styles {
	var (
		// Colors
		primaryColor      = lipgloss.Color("#7c3aed")
		successColor      = lipgloss.Color("#22c55e")
		failureColor      = lipgloss.Color("#ef4444")
		warningColor      = lipgloss.Color("#f59e0b")
		infoColor         = lipgloss.Color("#3b82f6")
		mutedColor        = lipgloss.Color("#6b7280")
		borderColor       = lipgloss.Color("#374151")
		activeBorderColor = lipgloss.Color("#7c3aed")

		// Common styles
		baseBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor)
	)

	return Styles{
		Base: lipgloss.NewStyle().
			Padding(0, 1),

		Title: lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Padding(0, 1),

		Subtitle: lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 1),

		List: baseBorder.
			Padding(1, 2),

		listItem: lipgloss.NewStyle().
			Padding(0, 1),

		selectedItem: lipgloss.NewStyle().
			Foreground(primaryColor).
			Background(lipgloss.Color("#1e1b4b")).
			Padding(0, 1),

		StatusSuccess: lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true),

		StatusFailure: lipgloss.NewStyle().
			Foreground(failureColor).
			Bold(true),

		StatusPending: lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true),

		StatusInProgress: lipgloss.NewStyle().
			Foreground(infoColor).
			Bold(true),

		StatusSkipped: lipgloss.NewStyle().
			Foreground(mutedColor).
			Bold(true),

		Border: baseBorder,

		ActiveBorder: baseBorder.
			BorderForeground(activeBorderColor),

		Content: baseBorder.
			Padding(1, 2).
			AlignHorizontal(lipgloss.Left),

		Sidebar: baseBorder.
			Padding(1, 2).
			Width(30),

		Help: lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 2),

		HelpKey: lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true),

		HelpDesc: lipgloss.NewStyle().
			Foreground(mutedColor),
	}
}

// StatusStyle returns the appropriate style for a status
func (s Styles) StatusStyle(status string) lipgloss.Style {
	switch status {
	case "success", "completed":
		return s.StatusSuccess
	case "failure", "failed":
		return s.StatusFailure
	case "pending", "queued":
		return s.StatusPending
	case "in_progress", "running":
		return s.StatusInProgress
	case "skipped":
		return s.StatusSkipped
	default:
		return s.Base
	}
}
