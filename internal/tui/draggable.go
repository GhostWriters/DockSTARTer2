package tui

import (
	"DockSTARTer2/internal/console"

	"codeberg.org/tslocum/cview"
	"github.com/gdamore/tcell/v3"
)

// DraggableGrid wraps a Grid to make it draggable
type DraggableGrid struct {
	*cview.Grid

	dragging     bool
	dragStartX   int
	dragStartY   int
	dialogX      int
	dialogY      int
	dialogWidth  int
	dialogHeight int
	titleHeight  int // Height of title bar area where dragging is allowed (Border + Title + Separator)
}

// NewDraggableGrid creates a new draggable grid wrapper
func NewDraggableGrid(grid *cview.Grid, width, height int) *DraggableGrid {
	dg := &DraggableGrid{
		Grid:         grid,
		dialogWidth:  width,
		dialogHeight: height,
		titleHeight:  3, // Title bar is typically first 3 rows
	}

	// Calculate initial centered position
	termWidth, termHeight, _ := console.GetTerminalSize()
	dg.dialogX = (termWidth - width) / 2
	dg.dialogY = (termHeight - height) / 2

	// Ensure it stays on screen
	if dg.dialogX < 0 {
		dg.dialogX = 0
	}
	if dg.dialogY < 0 {
		dg.dialogY = 0
	}

	return dg
}

// MouseHandler handles mouse events for dragging
func (dg *DraggableGrid) MouseHandler() func(action cview.MouseAction, event *tcell.EventMouse, setFocus func(p cview.Primitive)) (consumed bool, capture cview.Primitive) {
	return func(action cview.MouseAction, event *tcell.EventMouse, setFocus func(p cview.Primitive)) (consumed bool, capture cview.Primitive) {
		x, y := event.Position()

		// Convert screen coordinates to dialog-relative coordinates
		relX := x - dg.dialogX
		relY := y - dg.dialogY

		switch action {
		case cview.MouseLeftDown:
			// Check if click is in title bar area
			if relX >= 0 && relX < dg.dialogWidth && relY >= 0 && relY < dg.titleHeight {
				dg.dragging = true
				dg.dragStartX = x
				dg.dragStartY = y
				return true, dg
			}

		case cview.MouseMove:
			if dg.dragging {
				// Calculate new position
				deltaX := x - dg.dragStartX
				deltaY := y - dg.dragStartY

				dg.dialogX += deltaX
				dg.dialogY += deltaY

				// Update drag start position for next move
				dg.dragStartX = x
				dg.dragStartY = y

				// Ensure dialog stays on screen
				termWidth, termHeight, _ := console.GetTerminalSize()
				if dg.dialogX < 0 {
					dg.dialogX = 0
				}
				if dg.dialogY < 0 {
					dg.dialogY = 0
				}
				if dg.dialogX+dg.dialogWidth > termWidth {
					dg.dialogX = termWidth - dg.dialogWidth
				}
				if dg.dialogY+dg.dialogHeight > termHeight {
					dg.dialogY = termHeight - dg.dialogHeight
				}

				// Force redraw
				app.Draw()
				return true, dg
			}

		case cview.MouseLeftUp:
			if dg.dragging {
				dg.dragging = false
				return true, nil
			}
		}

		// Pass through to underlying grid if not handling drag
		if !dg.dragging {
			if handler := dg.Grid.MouseHandler(); handler != nil {
				return handler(action, event, setFocus)
			}
		}

		return false, nil
	}
}

// Draw draws the draggable grid at its current position
func (dg *DraggableGrid) Draw(screen tcell.Screen) {
	// Get the screen dimensions
	width, height := screen.Size()

	// Only draw if position is valid
	if dg.dialogX >= width || dg.dialogY >= height {
		return
	}

	// Calculate the actual drawable area
	drawWidth := dg.dialogWidth
	drawHeight := dg.dialogHeight

	if dg.dialogX+drawWidth > width {
		drawWidth = width - dg.dialogX
	}
	if dg.dialogY+drawHeight > height {
		drawHeight = height - dg.dialogY
	}

	// Create a sub-screen at the dialog position
	// Draw the underlying grid
	dg.Grid.SetRect(dg.dialogX, dg.dialogY, drawWidth, drawHeight)
	dg.Grid.Draw(screen)
}

// GetRect returns the current position and size
func (dg *DraggableGrid) GetRect() (int, int, int, int) {
	return dg.dialogX, dg.dialogY, dg.dialogWidth, dg.dialogHeight
}

// SetRect updates the size (position managed internally for drag)
func (dg *DraggableGrid) SetRect(x, y, width, height int) {
	dg.dialogWidth = width
	dg.dialogHeight = height
	// Don't override position during drag
	if !dg.dragging {
		dg.dialogX = x
		dg.dialogY = y
	}
}
