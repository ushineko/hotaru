package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

/*
A row in a list: columns of fixed width, then the things that act on it.

**The buttons sit next to the row, not at the far edge of the window.** They
were pushed right, so a wide window left a hand's width of nothing between the
end of a line and the button belonging to it -- and with twenty rows on screen
the eye has to track across that gap and back for every one. Which button
belongs to which line stops being obvious exactly when there are enough lines
for it to matter.

So every column is a fixed width and the actions follow the last one. The
columns line up down the list, the buttons form a band beside them, and the
empty space is on the outside where nobody has to cross it.
*/
func listRow(columns []fyne.CanvasObject, actions ...fyne.CanvasObject) fyne.CanvasObject {
	return ListRow(columns, actions...)
}

// ListRow is listRow, exported so a test can measure where a button lands.
func ListRow(columns []fyne.CanvasObject, actions ...fyne.CanvasObject) fyne.CanvasObject {
	row := make([]fyne.CanvasObject, 0, len(columns)+len(actions))
	row = append(row, columns...)
	row = append(row, actions...)

	// An HBox alone, at the left of a border with nothing else in it. An
	// HBox stretched to the section's width would put the gap back.
	return container.NewBorder(nil, nil, container.NewHBox(row...), nil, nil)
}

// column holds an object to a width, so the same thing is in the same place
// on every line.
func column(width float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(width, o.MinSize().Height), o)
}
