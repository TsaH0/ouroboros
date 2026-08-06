package workspace

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Pane is a single view container within the workspace.
type Pane struct {
	ID      string
	View    View
	Focused bool
	Width   int
	Height  int
}

// Render returns the pane's content with a border and title.
func (p *Pane) Render() string {
	content := p.View.View()
	innerWidth := p.Width - 2
	innerHeight := p.Height - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Truncate or pad content to fit. Use lipgloss.Width for visual width
	// (ANSI codes and multi-byte runes inflate byte length).
	lines := strings.Split(content, "\n")
	var fitted []string
	for _, line := range lines {
		if lipgloss.Width(line) > innerWidth {
			line = lipgloss.NewStyle().MaxWidth(innerWidth).Render(line)
		}
		fitted = append(fitted, line)
	}
	for len(fitted) < innerHeight {
		fitted = append(fitted, "")
	}
	if len(fitted) > innerHeight {
		fitted = fitted[:innerHeight]
	}

	// Build the bordered output.
	var b strings.Builder

	// Top border with title.
	title := p.View.Title()
	topBorder := "┌" + strings.Repeat("─", innerWidth) + "┐"
	if title != "" {
		titleStr := fmt.Sprintf(" %s ", title)
		maxTitleLen := innerWidth - 2
		if lipgloss.Width(titleStr) > maxTitleLen {
			titleStr = lipgloss.NewStyle().MaxWidth(maxTitleLen).Render(titleStr)
		}
		padLen := innerWidth - lipgloss.Width(titleStr)
		if padLen < 0 {
			padLen = 0
		}
		topBorder = "┌" + titleStr + strings.Repeat("─", padLen) + "┐"
	}
	if p.Focused {
		topBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(topBorder)
	} else {
		topBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(topBorder)
	}
	b.WriteString(topBorder)
	b.WriteString("\n")

	// Content lines with side borders.
	for _, line := range fitted {
		side := "│"
		if p.Focused {
			side = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("│")
		} else {
			side = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("│")
		}
		b.WriteString(side)
		b.WriteString(line)
		padLen := innerWidth - lipgloss.Width(line)
		if padLen > 0 {
			b.WriteString(strings.Repeat(" ", padLen))
		}
		b.WriteString(side)
		b.WriteString("\n")
	}

	// Bottom border.
	bottomBorder := "└" + strings.Repeat("─", innerWidth) + "┘"
	if p.Focused {
		bottomBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(bottomBorder)
	} else {
		bottomBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(bottomBorder)
	}
	b.WriteString(bottomBorder)

	return b.String()
}
