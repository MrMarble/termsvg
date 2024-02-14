package svg

import (
	"fmt"
	"io"
	"strings"

	svg "github.com/ajstarks/svgo"
	"github.com/hinshun/vt10x"
	"github.com/mrmarble/termsvg/internal/uniqueid"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/css"
)

const (
	rowHeight  = 25
	colWidth   = 12
	padding    = 20
	headerSize = 3
)

// If user passed custom background and text colors, use them
var (
	foregroundColorOverride = ""
	backgroundColorOverride = ""
)

type Canvas struct {
	*svg.SVG
	asciicast.Cast
	id     *uniqueid.ID
	width  int
	height int
	colors map[string]string
}

type Output interface {
	io.Writer
}

func Export(input asciicast.Cast, output Output, bgColor, textColor string, no_window bool) {
	// Set the custom foreground and background colors
	foregroundColorOverride = textColor
	backgroundColorOverride = bgColor

	input.Compress() // to reduce the number of frames

	createCanvas(svg.New(output), input, no_window)
}

func createCanvas(svg *svg.SVG, cast asciicast.Cast, no_window bool) {
	canvas := &Canvas{SVG: svg, Cast: cast, id: uniqueid.New(), colors: make(map[string]string)}
	canvas.width = cast.Header.Width * colWidth
	canvas.height = cast.Header.Height * rowHeight

	parseCast(canvas)
	canvas.Start(canvas.paddedWidth(), canvas.paddedHeight())
	if !no_window {
		canvas.createWindow()
		canvas.Group(fmt.Sprintf(`transform="translate(%d,%d)"`, padding, padding*headerSize))
	} else {
		if backgroundColorOverride == "" {
			canvas.Rect(0, 0, canvas.paddedWidth(), canvas.paddedHeight(), "fill:#282d35")
		} else {
			canvas.Rect(0, 0, canvas.paddedWidth(), canvas.paddedHeight(), "fill:"+backgroundColorOverride)
		}
		canvas.Group(fmt.Sprintf(`transform="translate(%d,%d)"`, padding, int(padding*1.5)))
	}
	canvas.addStyles()
	canvas.createFrames()
	canvas.Gend() // Transform
	canvas.Gend() // Styles
	canvas.End()
}

func parseCast(c *Canvas) {
	term := vt10x.New(vt10x.WithSize(c.Header.Width, c.Header.Height))

	for _, event := range c.Events {
		_, err := term.Write([]byte(event.EventData))
		if err != nil {
			panic(err)
		}

		for row := 0; row < c.Header.Height; row++ {
			for col := 0; col < c.Header.Width; col++ {
				cell := term.Cell(col, row)

				c.getColors(cell)
			}
		}
	}
}

func (c *Canvas) getColors(cell vt10x.Glyph) {
	fg := color.GetColor(cell.FG)

	if _, ok := c.colors[fg]; !ok {
		c.colors[fg] = c.id.String()
		c.id.Next()
	}

	if cell.BG != vt10x.DefaultBG {
		bg := color.GetColor(cell.BG)
		if _, ok := c.colors[bg]; !ok {
			c.colors[bg] = c.id.String()
			c.id.Next()
		}
	}
}

func (c *Canvas) paddedWidth() int {
	return c.width + (padding << 1)
}

func (c *Canvas) paddedHeight() int {
	return c.height + (padding * headerSize)
}

func (c *Canvas) createWindow() {
	windowRadius := 5
	buttonRadius := 7
	buttonColors := [3]string{"#ff5f58", "#ffbd2e", "#18c132"}

	// If the user has specified a background color, use that instead of the default
	if backgroundColorOverride != "" {
		c.Roundrect(0, 0, c.paddedWidth(), c.paddedHeight(), windowRadius, windowRadius, "fill:"+backgroundColorOverride)
	} else {
		c.Roundrect(0, 0, c.paddedWidth(), c.paddedHeight(), windowRadius, windowRadius, "fill:#282d35")
	}

	for i := range buttonColors {
		c.Circle((i*(padding+buttonRadius/2))+padding, padding, buttonRadius, fmt.Sprintf("fill:%s", buttonColors[i]))
	}
}

func (c *Canvas) addStyles() {
	c.Gstyle(css.Rules{
		"animation-duration":        fmt.Sprintf("%.2fs", c.Header.Duration),
		"animation-iteration-count": "infinite",
		"animation-name":            "k",
		"animation-timing-function": "steps(1,end)",
		"font-family":               "Monaco,Consolas,Menlo,'Bitstream Vera Sans Mono','Powerline Symbols',monospace",
		"font-size":                 "20px",
	}.String())

	// Foreground color gets set here
	colors := css.Blocks{}
	for color, class := range c.colors {
		colors = append(colors, css.Block{Selector: fmt.Sprintf(".%s", class), Rules: css.Rules{"fill": color}})
	}

	styles := generateKeyframes(c.Cast, int32(c.paddedWidth()))
	// If custom colors have been provided, use them instead
	if foregroundColorOverride != "" {
		styles += fmt.Sprintf(".a{fill:%s}", foregroundColorOverride)
	} else {
		styles += colors.String()
	}
	c.Style("text/css", styles)
}

func (c *Canvas) createFrames() {
	term := vt10x.New(vt10x.WithSize(c.Header.Width, c.Header.Height))

	for i, event := range c.Events {
		_, err := term.Write([]byte(event.EventData))
		if err != nil {
			panic(err)
		}

		c.Gtransform(fmt.Sprintf("translate(%d)", c.paddedWidth()*i))

		for row := 0; row < c.Header.Height; row++ {
			frame := ""
			lastColor := term.Cell(0, row).FG
			lastColummn := 0

			for col := 0; col < c.Header.Width; col++ {
				cell := term.Cell(col, row)
				c.addBG(cell.BG)

				if cell.Char == ' ' || cell.FG != lastColor {
					if frame != "" {
						c.Text(lastColummn*colWidth,
							row*rowHeight, frame, fmt.Sprintf(`class="%s"`, c.colors[color.GetColor(lastColor)]), c.applyBG(cell.BG))

						frame = ""
					}

					if cell.Char == ' ' {
						lastColummn = col + 1
						continue
					} else {
						lastColor = cell.FG
						lastColummn = col
					}
				}

				frame += string(cell.Char)
			}

			if strings.TrimSpace(frame) != "" {
				c.Text(lastColummn*colWidth, row*rowHeight, frame, fmt.Sprintf(`class="%s"`, c.colors[color.GetColor(lastColor)]))
			}
		}
		c.Gend()
	}
}

func (c *Canvas) addBG(bg vt10x.Color) {
	if bg != vt10x.DefaultBG {
		if _, ok := c.colors[fmt.Sprint(bg)]; !ok {
			c.Def()
			c.Filter(fmt.Sprint(bg))
			c.FeFlood(svg.Filterspec{Result: "bg"}, color.GetColor(bg), 1.0)
			c.FeMerge([]string{`bg`, `SourceGraphic`})
			c.Fend()
			c.DefEnd()
			c.colors[fmt.Sprint(bg)] = ""
		}
	}
}

func (c *Canvas) applyBG(bg vt10x.Color) string {
	if bg != vt10x.DefaultBG {
		if _, ok := c.colors[fmt.Sprint(bg)]; ok {
			return fmt.Sprintf(`filter="url(#%s)"`, fmt.Sprint(bg))
		}
	}

	return ""
}

func generateKeyframes(cast asciicast.Cast, width int32) string {
	css := "@keyframes k {"
	for i, frame := range cast.Events {
		css += generateKeyframe(float32(frame.Time*100/cast.Header.Duration), width*int32(i))
	}

	css += "}"

	return css
}

func generateKeyframe(percent float32, translate int32) string {
	return fmt.Sprintf("%.3f%%{transform:translateX(-%dpx)}", percent, translate)
}
