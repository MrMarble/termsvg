// Package progress provides progress reporting for export operations.
// Updates travel over a channel so producers can report from any goroutine.
package progress

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
)

// Phase names shown next to each progress bar.
const (
	PhaseIR        = "IR Processing"
	PhaseRasterize = "Rasterizing"
	PhaseEncode    = "Encoding"
)

// updateQueueSize lets producers run ahead of the terminal without blocking.
const updateQueueSize = 100

// Update represents a progress update from a processing phase.
type Update struct {
	Phase   string // one of the Phase constants
	Current int    // Current item number (1-indexed)
	Total   int    // Total items in this phase
}

// Reporter draws one progress bar per phase.
type Reporter struct {
	updates <-chan Update
	done    chan struct{}
}

// New creates a reporter and the send-only channel that feeds it.
func New() (reporter *Reporter, progressCh chan<- Update) {
	ch := make(chan Update, updateQueueSize)
	return &Reporter{updates: ch, done: make(chan struct{})}, ch
}

// Run starts a reporter, calls fn with the channel to send updates on, and
// then closes the channel and waits for the last bar to be drawn.
func Run(fn func(progressCh chan<- Update) error) error {
	reporter, ch := New()
	reporter.Start()
	defer func() {
		close(ch)
		reporter.Wait()
	}()
	return fn(ch)
}

// Start begins listening for updates and drawing a bar for each phase.
func (r *Reporter) Start() {
	go func() {
		var bar *progressbar.ProgressBar
		phase := ""

		finish := func() {
			if bar != nil {
				_ = bar.Finish()
				fmt.Println()
			}
		}

		for update := range r.updates {
			if update.Phase != phase {
				finish()
				phase = update.Phase
				bar = newBar(update.Total, update.Phase)
			}
			bar.Describe(fmt.Sprintf("%s... %d/%d", update.Phase, update.Current, update.Total))
			_ = bar.Set(update.Current)
		}

		finish()
		close(r.done)
	}()
}

// Wait blocks until the reporter finishes (channel is closed).
func (r *Reporter) Wait() {
	<-r.done
}

func newBar(total int, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions(total,
		progressbar.OptionSetDescription(description+"..."),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetWriter(os.Stderr),
	)
}
