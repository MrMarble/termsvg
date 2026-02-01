// Package progress provides progress reporting for export operations.
// It uses channels for thread-safe updates, making it easy to transition
// to concurrent operations later.
package progress

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
)

// Update represents a progress update from a processing phase.
type Update struct {
	Phase   string // Phase name: "IR Processing", "Rasterizing", "Encoding"
	Current int    // Current item number (1-indexed)
	Total   int    // Total items in this phase
}

// Reporter manages progress bars for each phase.
type Reporter struct {
	updates      <-chan Update
	done         chan struct{}
	currentPhase string
}

// Start begins listening for updates and creating bars for each phase.
func (r *Reporter) Start() {
	go func() {
		var currentBar *progressbar.ProgressBar

		for update := range r.updates {
			// If phase changed, finish the previous bar
			if update.Phase != r.currentPhase {
				if currentBar != nil {
					_ = currentBar.Finish()
					fmt.Println() // New line after each phase
				}
				r.currentPhase = update.Phase
				currentBar = newBar(update.Total, update.Phase)
			}

			// Update current bar
			if currentBar != nil {
				currentBar.Describe(fmt.Sprintf("%s... %d/%d", update.Phase, update.Current, update.Total))
				_ = currentBar.Set(update.Current)
			}
		}

		// Finish the last bar
		if currentBar != nil {
			_ = currentBar.Finish()
			fmt.Println()
		}

		close(r.done)
	}()
}

// Wait blocks until the reporter finishes (channel is closed).
func (r *Reporter) Wait() {
	<-r.done
}

// newBar creates a new progress bar with consistent settings.
func newBar(total int, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions(total,
		progressbar.OptionSetDescription(description+"..."),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetWriter(os.Stderr),
	)
}

// New creates a reporter with a channel for updates.
// Returns the reporter and the send-only channel.
func New() (reporter *Reporter, progressCh chan<- Update) {
	ch := make(chan Update, 100) // Buffered to prevent blocking
	return &Reporter{
		updates:      ch,
		done:         make(chan struct{}),
		currentPhase: "",
	}, ch
}
