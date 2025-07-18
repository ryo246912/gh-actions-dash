package logs

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Processor handles log processing and ANSI color rendering
type Processor struct {
	baseStyle lipgloss.Style
}

// NewProcessor creates a new log processor
func NewProcessor(baseStyle lipgloss.Style) *Processor {
	return &Processor{
		baseStyle: baseStyle,
	}
}

// ProcessLogContent processes log content and renders ANSI colors
func (p *Processor) ProcessLogContent(content string) string {
	if content == "" {
		return content
	}

	// Split into lines for processing
	lines := strings.Split(content, "\n")
	processedLines := make([]string, len(lines))

	for i, line := range lines {
		processedLines[i] = p.processLine(line)
	}

	return strings.Join(processedLines, "\n")
}

// processLine processes a single line and renders ANSI colors
func (p *Processor) processLine(line string) string {
	// If the line doesn't contain ANSI sequences, return as-is
	if !containsANSI(line) {
		return line
	}

	// Use ansi package to strip and process sequences
	// For now, we'll use a basic approach to preserve colors
	// while ensuring proper rendering in the TUI

	// Create a style that preserves ANSI sequences
	rendered := p.renderANSILine(line)
	return rendered
}

// renderANSILine renders a line with ANSI color codes
func (p *Processor) renderANSILine(line string) string {
	// Parse ANSI sequences and convert to lipgloss styles
	result := ""
	currentPos := 0

	// Find ANSI escape sequences
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	matches := ansiRegex.FindAllStringIndex(line, -1)

	currentStyle := p.baseStyle

	for _, match := range matches {
		// Add text before this ANSI sequence
		if match[0] > currentPos {
			text := line[currentPos:match[0]]
			result += currentStyle.Render(text)
		}

		// Process the ANSI sequence
		sequence := line[match[0]:match[1]]
		currentStyle = p.updateStyleFromANSI(currentStyle, sequence)

		currentPos = match[1]
	}

	// Add remaining text
	if currentPos < len(line) {
		text := line[currentPos:]
		result += currentStyle.Render(text)
	}

	return result
}

// updateStyleFromANSI updates lipgloss style based on ANSI sequence
func (p *Processor) updateStyleFromANSI(style lipgloss.Style, sequence string) lipgloss.Style {
	// Parse ANSI codes and map to lipgloss styles
	// Remove escape sequence prefix and suffix
	if len(sequence) < 4 {
		return style
	}

	codes := strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b["), "m")
	if codes == "" {
		codes = "0"
	}

	// Split multiple codes
	parts := strings.Split(codes, ";")

	for _, part := range parts {
		switch part {
		case "0": // Reset
			style = p.baseStyle
		case "1": // Bold
			style = style.Bold(true)
		case "2": // Dim
			style = style.Faint(true)
		case "3": // Italic
			style = style.Italic(true)
		case "4": // Underline
			style = style.Underline(true)
		case "30": // Black foreground
			style = style.Foreground(lipgloss.Color("#000000"))
		case "31": // Red foreground
			style = style.Foreground(lipgloss.Color("#ff0000"))
		case "32": // Green foreground
			style = style.Foreground(lipgloss.Color("#00ff00"))
		case "33": // Yellow foreground
			style = style.Foreground(lipgloss.Color("#ffff00"))
		case "34": // Blue foreground
			style = style.Foreground(lipgloss.Color("#0000ff"))
		case "35": // Magenta foreground
			style = style.Foreground(lipgloss.Color("#ff00ff"))
		case "36": // Cyan foreground
			style = style.Foreground(lipgloss.Color("#00ffff"))
		case "37": // White foreground
			style = style.Foreground(lipgloss.Color("#ffffff"))
		case "90": // Bright Black (Gray)
			style = style.Foreground(lipgloss.Color("#808080"))
		case "91": // Bright Red
			style = style.Foreground(lipgloss.Color("#ff8080"))
		case "92": // Bright Green
			style = style.Foreground(lipgloss.Color("#80ff80"))
		case "93": // Bright Yellow
			style = style.Foreground(lipgloss.Color("#ffff80"))
		case "94": // Bright Blue
			style = style.Foreground(lipgloss.Color("#8080ff"))
		case "95": // Bright Magenta
			style = style.Foreground(lipgloss.Color("#ff80ff"))
		case "96": // Bright Cyan
			style = style.Foreground(lipgloss.Color("#80ffff"))
		case "97": // Bright White
			style = style.Foreground(lipgloss.Color("#ffffff"))
		}
	}

	return style
}

// containsANSI checks if a string contains ANSI escape sequences
func containsANSI(s string) bool {
	return strings.Contains(s, "\x1b[")
}

// StripANSI removes ANSI escape sequences from a string
func StripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(s, "")
}

// ProcessLogLines processes multiple log lines with ANSI support
func (p *Processor) ProcessLogLines(lines []string) []string {
	processed := make([]string, len(lines))
	for i, line := range lines {
		processed[i] = p.processLine(line)
	}
	return processed
}
