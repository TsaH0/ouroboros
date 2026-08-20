package workspace

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// LayoutKind describes how a layout arranges its children.
type LayoutKind int

const (
	LayoutLeaf   LayoutKind = iota // single pane
	LayoutHSplit                   // horizontal split (left/right)
	LayoutVSplit                   // vertical split (top/bottom)
	LayoutGrid                     // 2x2 grid
)

// Layout is a node in the layout tree.
type Layout struct {
	Kind   LayoutKind
	Pane   *Pane   // non-nil only for LayoutLeaf
	Left   *Layout // child for splits
	Right  *Layout // child for splits
	Weight float64 // relative size weight (0.0-1.0)
}

// NewLeaf creates a leaf layout containing a pane.
func NewLeaf(p *Pane) *Layout {
	return &Layout{Kind: LayoutLeaf, Pane: p, Weight: 1.0}
}

// NewHSplit creates a horizontal split layout.
func NewHSplit(left, right *Layout, weight float64) *Layout {
	return &Layout{Kind: LayoutHSplit, Left: left, Right: right, Weight: weight}
}

// NewVSplit creates a vertical split layout.
func NewVSplit(top, bottom *Layout, weight float64) *Layout {
	return &Layout{Kind: LayoutVSplit, Left: top, Right: bottom, Weight: weight}
}

// Panes returns all panes in the layout tree.
func (l *Layout) Panes() []*Pane {
	switch l.Kind {
	case LayoutLeaf:
		if l.Pane != nil {
			return []*Pane{l.Pane}
		}
		return nil
	case LayoutHSplit, LayoutVSplit:
		var panes []*Pane
		if l.Left != nil {
			panes = append(panes, l.Left.Panes()...)
		}
		if l.Right != nil {
			panes = append(panes, l.Right.Panes()...)
		}
		return panes
	case LayoutGrid:
		var panes []*Pane
		if l.Left != nil {
			panes = append(panes, l.Left.Panes()...)
		}
		if l.Right != nil {
			panes = append(panes, l.Right.Panes()...)
		}
		return panes
	}
	return nil
}

// FindPaneByID returns the pane with the given ID, or nil.
func (l *Layout) FindPaneByID(id string) *Pane {
	for _, p := range l.Panes() {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Resize assigns dimensions to all panes in the tree.
func (l *Layout) Resize(width, height int) {
	switch l.Kind {
	case LayoutLeaf:
		if l.Pane != nil {
			l.Pane.Width = width
			l.Pane.Height = height
			vw, vh := width-2, height-2
			if vw < 1 {
				vw = 1
			}
			if vh < 1 {
				vh = 1
			}
			l.Pane.View.Resize(vw, vh)
		}
	case LayoutHSplit:
		leftW := int(float64(width) * l.Weight)
		rightW := width - leftW
		if l.Left != nil {
			l.Left.Resize(leftW, height)
		}
		if l.Right != nil {
			l.Right.Resize(rightW, height)
		}
	case LayoutVSplit:
		topH := int(float64(height) * l.Weight)
		bottomH := height - topH
		if l.Left != nil {
			l.Left.Resize(width, topH)
		}
		if l.Right != nil {
			l.Right.Resize(width, bottomH)
		}
	case LayoutGrid:
		halfW := width / 2
		halfH := height / 2
		if l.Left != nil {
			l.Left.Resize(halfW, halfH)
		}
		if l.Right != nil {
			l.Right.Resize(halfW, halfH)
		}
	}
}

// Render renders the layout tree into a single string.
func (l *Layout) Render() string {
	switch l.Kind {
	case LayoutLeaf:
		if l.Pane != nil {
			return l.Pane.Render()
		}
		return ""
	case LayoutHSplit:
		left := l.Left.Render()
		right := l.Right.Render()
		// Use lipgloss for ANSI-safe horizontal join; keeps each pane
		// isolated so scrolling one never bleeds into the other.
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case LayoutVSplit:
		top := l.Left.Render()
		bottom := l.Right.Render()
		return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	case LayoutGrid:
		topLeft := l.Left.Render()
		topRight := l.Right.Render()
		return lipgloss.JoinHorizontal(lipgloss.Top, topLeft, topRight)
	}
	return ""
}

// joinHorizontal joins two rendered panes side by side (legacy fallback).
func joinHorizontal(left, right string) string {
	leftLines := splitLines(left)
	rightLines := splitLines(right)

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	for len(leftLines) < maxLines {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, "")
	}

	leftWidth := 0
	for _, line := range leftLines {
		if w := lipgloss.Width(line); w > leftWidth {
			leftWidth = w
		}
	}

	var result []string
	for i := 0; i < maxLines; i++ {
		l := leftLines[i]
		r := rightLines[i]
		if pad := leftWidth - lipgloss.Width(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		result = append(result, l+r)
	}
	return strings.Join(result, "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// paneAt returns the leaf pane containing (x,y) within the given bounds.
func (l *Layout) paneAt(x, y, ox, oy, w, h int) *Pane {
	switch l.Kind {
	case LayoutLeaf:
		if l.Pane != nil && x >= ox && x < ox+w && y >= oy && y < oy+h {
			return l.Pane
		}
		return nil
	case LayoutHSplit:
		leftW := int(float64(w) * l.Weight)
		rightW := w - leftW
		if l.Left != nil {
			if p := l.Left.paneAt(x, y, ox, oy, leftW, h); p != nil {
				return p
			}
		}
		if l.Right != nil {
			return l.Right.paneAt(x, y, ox+leftW, oy, rightW, h)
		}
	case LayoutVSplit:
		topH := int(float64(h) * l.Weight)
		bottomH := h - topH
		if l.Left != nil {
			if p := l.Left.paneAt(x, y, ox, oy, w, topH); p != nil {
				return p
			}
		}
		if l.Right != nil {
			return l.Right.paneAt(x, y, ox, oy+topH, w, bottomH)
		}
	}
	return nil
}
