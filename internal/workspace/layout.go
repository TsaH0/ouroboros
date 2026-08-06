package workspace

import "strings"

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
			l.Pane.View.Resize(width-2, height-2)
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
		return joinHorizontal(left, right)
	case LayoutVSplit:
		top := l.Left.Render()
		bottom := l.Right.Render()
		return top + "\n" + bottom
	case LayoutGrid:
		topLeft := l.Left.Render()
		topRight := l.Right.Render()
		return joinHorizontal(topLeft, topRight)
	}
	return ""
}

// joinHorizontal joins two rendered panes side by side.
func joinHorizontal(left, right string) string {
	leftLines := splitLines(left)
	rightLines := splitLines(right)

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	// Pad both to same height.
	for len(leftLines) < maxLines {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, "")
	}

	// Find max width of left side.
	leftWidth := 0
	for _, line := range leftLines {
		if len(line) > leftWidth {
			leftWidth = len(line)
		}
	}

	var result []string
	for i := 0; i < maxLines; i++ {
		l := leftLines[i]
		r := rightLines[i]
		// Pad left line to leftWidth.
		if len(l) < leftWidth {
			l += strings.Repeat(" ", leftWidth-len(l))
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
