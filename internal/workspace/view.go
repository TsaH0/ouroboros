package workspace

import tea "charm.land/bubbletea/v2"

// View is the interface every workspace pane must implement.
type View interface {
	// ID returns a unique identifier for this view instance.
	ID() string

	// Title returns the display title shown in the pane border.
	Title() string

	// Init returns the initial command for this view.
	Init() tea.Cmd

	// Update processes a message and returns the updated view and any command.
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the view content as a string (without borders).
	View() string

	// Focus is called when the pane gains focus.
	Focus()

	// Blur is called when the pane loses focus.
	Blur()

	// Resize updates the view's dimensions.
	Resize(width, height int)

	// HelpText returns the keybindings shown in the workspace help bar.
	HelpText() string

	// IsEditing returns true when the view has a text input capturing keystrokes.
	// Global key shortcuts are suppressed while editing.
	IsEditing() bool
}
