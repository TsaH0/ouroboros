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

	// Truncate or pad content to fit.
	lines := strings.Split(content, "\n")
	var fitted []string
	for _, line := range lines {
		if len(line) > innerWidth {
			line = line[:innerWidth]
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
		if len(titleStr) > maxTitleLen {
			titleStr = titleStr[:maxTitleLen]
		}
		topBorder = "┌" + titleStr + strings.Repeat("─", innerWidth-len(titleStr)) + "┐"
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
		if len(line) < innerWidth {
			b.WriteString(strings.Repeat(" ", innerWidth-len(line)))
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
