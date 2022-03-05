package svg

import (
	"fmt"
	"os"
	"strings"

	svg "github.com/ajstarks/svgo"
	"github.com/hinshun/vt10x"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/css"
)

const (
	rowHeight  = 25
	colWidth   = 11
	padding    = 20
	headerSize = 3
)

type Canvas struct {
	*svg.SVG
	asciicast.Cast
	width    int
	height   int
	bgColors map[string]byte
}

func createSVG(svg *svg.SVG, cast asciicast.Cast) {
	canvas := &Canvas{SVG: svg, Cast: cast}
	canvas.width = cast.Header.Width * colWidth
	canvas.height = cast.Header.Height * rowHeight

	canvas.createWindow()
	canvas.End()
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

	c.Start(c.width, c.paddedHeight())
	c.Roundrect(0, 0, c.width, c.paddedHeight(), windowRadius, windowRadius, "fill:#282d35")

	for i := range buttonColors {
		c.Circle((i*(padding+buttonRadius/2))+padding, padding, buttonRadius, fmt.Sprintf("fill:%s", buttonColors[i]))
	}

	c.addStyles()
	c.createFrames()
	c.Gend() // Transform
	c.Gend() // Styles
}

func (c *Canvas) addStyles() {
	styles := css.CSS{
		"animation-duration":        fmt.Sprintf("%.2fs", c.Header.Duration),
		"animation-iteration-count": "infinite",
		"animation-name":            "k",
		"animation-timing-function": "steps(1,end)",
		"font-family":               "Monaco,Consolas,Menlo,'Bitstream Vera Sans Mono','Powerline Symbols',monospace",
		"font-size":                 "20px",
	}

	c.Gstyle(styles.Compile())
	c.Style("text/css", generateKeyframes(c.Cast, int32(c.paddedWidth())))
	c.Group(fmt.Sprintf(`transform="translate(%d,%d)"`, padding, padding*headerSize))
}

func (c *Canvas) createFrames() {
	term := vt10x.New(vt10x.WithSize(c.Header.Width, c.Header.Height))
	for i, event := range c.Events {
		_, err := term.Write([]byte(event.EventData))
		if err != nil {
			panic(err)
		}

		c.Gtransform(fmt.Sprintf("translate(%d)", c.paddedWidth()*i))
		c.bgColors = map[string]byte{}

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
							row*rowHeight, frame, fmt.Sprintf(`fill="%v"`, getColor(lastColor)), c.applyBG(cell.BG))

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
				c.Text(lastColummn*colWidth, row*rowHeight, frame, fmt.Sprintf(`fill="%v"`, getColor(lastColor)))
			}
		}
		c.Gend()
	}
}

func Export(input asciicast.Cast, output string) {
	file, err := os.Create(fmt.Sprintf("%s.svg", output))
	if err != nil {
		panic(err)
	}

	defer file.Close()
	input.Compress()

	createSVG(svg.New(file), input)
}

func (c *Canvas) addBG(bg vt10x.Color) {
	if bg != vt10x.DefaultBG {
		if _, ok := c.bgColors[fmt.Sprint(bg)]; !ok {
			c.Def()
			c.Filter(fmt.Sprint(bg))
			c.FeFlood(svg.Filterspec{Result: "bg"}, getColor(bg), 1.0)
			c.FeMerge([]string{`bg`, `SourceGraphic`})
			c.Fend()
			c.DefEnd()
			c.bgColors[fmt.Sprint(bg)] = 1
		}
	}
}

func (c *Canvas) applyBG(bg vt10x.Color) string {
	if bg != vt10x.DefaultBG {
		if _, ok := c.bgColors[fmt.Sprint(bg)]; ok {
			return fmt.Sprintf(`filter="url(#%s)"`, fmt.Sprint(bg))
		}
	}

	return ""
}

func getColor(c vt10x.Color) string {
	colorStr := ""

	if c.ANSI() {
		colorStr = color.ToHex(color.AnsiToColor(uint32(c)))
	} else {
		colorStr = color.ToHex(color.AnsiToColor(uint32(vt10x.LightGrey)))
	}

	return colorStr
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
	return fmt.Sprintf("%2f%%{transform:translateX(-%dpx)}", percent, translate)
}
