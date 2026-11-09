package raster

import "time"

// Frame timing policy. A frame is shown from the event that makes it until the
// next one, with these limits for what a format can play:
const (
	// LastFrameHold is the least time the last frame stays on screen. A
	// recording ends at its last event, so that frame would otherwise flash by,
	// and a looping GIF or video would restart before the final screen could be
	// read. It also means no format needs a floor for the last frame.
	LastFrameHold = 2 * time.Second

	// MinGIFDelay is the shortest GIF frame delay: browsers clamp delays under
	// 20ms to 100ms.
	MinGIFDelay = 20 * time.Millisecond

	// MinVideoDelay is the shortest video frame: one frame of a 30 FPS video.
	MinVideoDelay = time.Second / 30
)
