// Package webm provides a WebM video renderer for terminal recordings.
// It generates WebM video files using FFmpeg for VP9 encoding.
package webm

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/raster"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for WebM output.
type Renderer struct {
	config     renderer.Config
	rasterizer *raster.Rasterizer
}

// New creates a new WebM renderer with the given configuration.
func New(config renderer.Config) (*Renderer, error) {
	// Check if FFmpeg is installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg is not installed. Install it from: https://ffmpeg.org")
	}

	rasterizer, err := renderer.NewRasterizer(config)
	if err != nil {
		return nil, err
	}

	return &Renderer{
		config:     config,
		rasterizer: rasterizer,
	}, nil
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "webm"
}

// FileExtension returns the file extension for WebM files.
func (r *Renderer) FileExtension() string {
	return ".webm"
}

// Render generates a WebM video from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}

	startTime := time.Now()
	if r.config.Debug {
		log.Printf("[WebM] Starting WebM generation for %d frames", len(rec.Frames))
	}

	// Phase 1: Rasterize frames to RGBA images
	rasterStart := time.Now()
	rgbaFrames, err := r.rasterizer.Rasterize(rec)
	if err != nil {
		return fmt.Errorf("failed to rasterize frames: %w", err)
	}
	rasterDuration := time.Since(rasterStart)

	if r.config.Debug {
		log.Printf("[WebM] Phase 1 - IR rasterization: %v (%d frames)", rasterDuration, len(rgbaFrames))
	}

	// Check for cancellation after rendering
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Phase 2: Encode to WebM using FFmpeg
	encodeStart := time.Now()
	if err := r.encodeToWebM(rgbaFrames, w); err != nil {
		return fmt.Errorf("failed to encode WebM: %w", err)
	}

	if r.config.Debug {
		log.Printf("[WebM] Phase 2 - FFmpeg encoding: %v", time.Since(encodeStart))
		log.Printf("[WebM] Total time: %v", time.Since(startTime))
	}

	return nil
}

// encodeToWebM encodes RGBA frames to WebM format using FFmpeg.
// Uses fixed 30 FPS with frame filtering to skip rapid events.
//
//nolint:gocognit,funlen // WebM encoding with FFmpeg requires complex frame handling
func (r *Renderer) encodeToWebM(frames []raster.RasterFrame, w io.Writer) error {
	if len(frames) == 0 {
		return fmt.Errorf("no frames to encode")
	}

	// Filter frames to skip rapid events (similar to GIF deduplication)
	// At 30 FPS, minimum display time is ~33ms
	filteredFrames := r.filterFrames(frames)

	if r.config.Debug {
		// Calculate total frames after duplication
		const frameDuration = time.Second / 30
		totalDuplicatedFrames := 0
		for _, frame := range filteredFrames {
			if frame.Image != nil {
				count := int(frame.Delay / frameDuration)
				if count < 1 {
					count = 1
				}
				totalDuplicatedFrames += count
			}
		}
		log.Printf("[WebM] Filtered %d frames -> %d frames (skipped %d rapid frames)",
			len(frames), len(filteredFrames), len(frames)-len(filteredFrames))
		log.Printf("[WebM] Total video frames after duplication: %d", totalDuplicatedFrames)
	}

	if len(filteredFrames) == 0 {
		return fmt.Errorf("no valid frames after filtering")
	}

	// Get frame dimensions from first frame
	firstFrame := filteredFrames[0].Image
	if firstFrame == nil {
		return fmt.Errorf("no valid frames to encode")
	}

	bounds := firstFrame.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Use fixed 30 FPS for consistent playback
	const frameRate = 30.0

	// Build FFmpeg command
	// Input: raw RGBA frames from stdin
	// Output: WebM with VP9 codec
	args := []string{
		"-y", // Overwrite output
		"-f", "rawvideo",
		"-vcodec", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-r", fmt.Sprintf("%f", frameRate),
		"-i", "-", // Read from stdin
		"-c:v", "libvpx-vp9",
		"-pix_fmt", "yuv420p",
		"-deadline", "good",
		"-cpu-used", "5",
		"-row-mt", "1",
		"-f", "webm",
		"pipe:1", // Write to stdout
	}

	// Add bitrate if specified
	if r.config.VideoBitrate > 0 {
		args = append(args, "-b:v", fmt.Sprintf("%dk", r.config.VideoBitrate))
	}

	cmd := exec.Command("ffmpeg", args...) //nolint:gosec // args are constructed from validated config

	// Get stdin pipe for writing frames
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Get stdout pipe for reading output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start FFmpeg
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Write frames to FFmpeg in a goroutine
	// Each frame is duplicated based on its delay to respect recording timing
	go func() {
		defer stdin.Close()

		const frameDuration = time.Second / 30 // ~33.33ms per frame at 30 FPS

		for _, frame := range filteredFrames {
			if frame.Image == nil {
				continue
			}

			// Calculate how many times to duplicate this frame based on its delay
			// At 30 FPS, each frame is 33.33ms, so a 500ms delay = 15 frames
			frameCount := int(frame.Delay / frameDuration)
			if frameCount < 1 {
				frameCount = 1 // Minimum 1 frame
			}

			// Write the frame multiple times to match the delay
			for i := 0; i < frameCount; i++ {
				_, err := stdin.Write(frame.Image.Pix)
				if err != nil {
					return
				}
			}
		}
	}()

	// Copy FFmpeg output to writer
	buf := make([]byte, 32*1024)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write output: %w", writeErr)
			}
		}
		if err != nil {
			break
		}
	}

	// Wait for FFmpeg to finish
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg encoding failed: %w", err)
	}

	return nil
}

// filterFrames filters frames to skip rapid events.
// At 30 FPS, each frame displays for ~33ms. Frames with shorter delays are skipped
// and their delay is accumulated to the next frame.
func (r *Renderer) filterFrames(frames []raster.RasterFrame) []raster.RasterFrame {
	const minDelay = 33 * time.Millisecond // Minimum display time at 30 FPS

	var filtered []raster.RasterFrame
	var accumulatedDelay time.Duration

	for i, frame := range frames {
		// Skip nil frames
		if frame.Image == nil {
			accumulatedDelay += frame.Delay
			continue
		}

		totalDelay := frame.Delay + accumulatedDelay

		// If this is not the last frame and total delay is below minimum, skip it
		if totalDelay < minDelay && i < len(frames)-1 {
			accumulatedDelay = totalDelay
			continue
		}

		// Create a new frame with accumulated delay
		filteredFrame := raster.RasterFrame{
			Image: frame.Image,
			Delay: totalDelay,
			Index: frame.Index,
		}
		filtered = append(filtered, filteredFrame)
		accumulatedDelay = 0
	}

	return filtered
}
