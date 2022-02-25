package svg

import (
	"fmt"
	"math"
	"os"
	"strings"

	svg "github.com/ajstarks/svgo"
	"github.com/mrmarble/termsvg/pkg/asciicast"
)

const (
	offset  = 10
	padding = 20
)

func New(cast asciicast.Cast) {
	file, err := os.Create("output.svg")
	if err != nil {
		panic(err)
	}

	defer file.Close()

	canvas := svg.New(file)
	width := cast.Header.Width * offset
	height := cast.Header.Height * offset

	createWindow(canvas, width, height)
	canvas.Group(fmt.Sprintf(`transform="translate(%d,%d)"`, padding, padding*3),
		`font-family="Monaco,Consolas,Menlo,'Bitstream Vera Sans Mono','Powerline Symbols',monospace"`)
	frames := getFrames(cast)
	canvas.Style("text/css", generateCss(len(frames), int32(width+padding*2)))
	canvas.Gstyle(fmt.Sprintf("animation-duration:%2fs;animation-iteration-count:infinite;animation-name:k;animation-timing-function:steps(1,end)", cast.Header.Duration))
	for i, frame := range frames {
		canvas.Gtransform(fmt.Sprintf("translate(%d)", (width+padding*2)*i))
		for i, str := range strings.Split(frame, "\n") {
			canvas.Text(0, 20*i, str, `font-size="20"`)
		}
		canvas.Gend()
	}
	canvas.Gend()
	canvas.Gend()
	canvas.End()
}

func createWindow(canvas *svg.SVG, w int, h int) {
	windowRadius := 5
	buttonRadius := 7
	buttonColors := [3]string{"#ff5f58", "#ffbd2e", "#18c132"}
	canvas.Start(w+padding*2, h+padding*2)
	canvas.Roundrect(0, 0, w+padding*2, h+padding*2, windowRadius, windowRadius, "fill:#282d35")

	for i := range buttonColors {
		canvas.Circle((i*(padding+buttonRadius/2))+padding, padding, buttonRadius, fmt.Sprintf("fill:%s", buttonColors[i]))
	}
}

func getFrames(cast asciicast.Cast) []string {
	duration := cast.Header.Duration
	frameCount := int32(math.Ceil(duration * 60))
	var frames []string
	frame := ""
	accTime := 0.
	fmt.Printf("Duration %2f, Frames: %d\n", duration, frameCount)
	cast.ToRelativeTime()
	cast.Events[0].Time = 0
	for _, event := range cast.Events {
		accTime += event.Time
		frame += event.EventData
		if accTime > 0.0166 {
			accTime = 0
			if len(frames) > 0 {
				frames = append(frames, frames[len(frames)-1]+frame)
			} else {
				frames = append(frames, frame)
			}
			frame = ""
		}
	}

	return frames
}

func delete_empty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func generateCss(frameCount int, width int32) string {
	step := 100. / float32(frameCount)
	css := "@keyframes k {"
	for i := 1; i < frameCount; i++ {
		css += generateKeyframe(step*float32(i), width*int32(i))
	}
	css += "}"
	return css
}

func generateKeyframe(percent float32, translate int32) string {
	return fmt.Sprintf("%2f%%{transform:translateX(-%dpx)}", percent, translate)
}
