// Package layout holds the pixel geometry shared by the SVG and raster
// renderers, so that every output format draws the same terminal window.
package layout

const (
	RowHeight  = 25 // pixels per terminal row
	ColWidth   = 12 // pixels per terminal column
	Padding    = 20 // space around the terminal content
	HeaderSize = 2  // title bar height, in Padding units

	ButtonSpacing = 20 // distance between window button centers
	ButtonRadius  = 6
)

// ContentOffsetY is the distance from the top of the image to the terminal content.
func ContentOffsetY(showWindow bool) int {
	if showWindow {
		return Padding * HeaderSize
	}
	return Padding
}

// PaddedWidth is the image width for terminal content contentWidth pixels wide.
func PaddedWidth(contentWidth int) int {
	return contentWidth + 2*Padding
}

// PaddedHeight is the image height for terminal content contentHeight pixels tall.
func PaddedHeight(contentHeight int, showWindow bool) int {
	return contentHeight + ContentOffsetY(showWindow) + Padding
}
