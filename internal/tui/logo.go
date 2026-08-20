package tui

import "charm.land/lipgloss/v2"

// Logo — compact and banner variants.
const (
	logoSmall = "◉ Ouroboros"
	logoBanner = `
  ╭───── ◆ ─────╮
  │  O U R O B O R O S  │
  ╰───── ◆ ─────╯
  intercept • inspect • repeat`
)

func renderLogoSmall() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true).Render(logoSmall)
}

func renderLogoBanner(width int) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true).Align(lipgloss.Center).Width(width)
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Align(lipgloss.Center).Width(width).
		Render("intercept • inspect • repeat  —  0:hist 3:intercept 4:scope 5:recon  I:toggle")
	return style.Render("◉  O U R O B O R O S") + "\n" + sub
}
