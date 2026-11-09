// Package webm provides a WebM video renderer for terminal recordings.
// It generates WebM video files using FFmpeg for VP9 encoding.
package webm

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// Renderer implements the renderer.Renderer interface for WebM output.
type Renderer struct {
	config     renderer.Config
	rasterizer *raster.Rasterizer
}

// New creates a new WebM renderer with the given configuration.
func New(config *renderer.Config) (*Renderer, error) {
	// Check if FFmpeg is installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg is not installed. Install it from: https://ffmpeg.org")
	}

	rasterizer, err := renderer.NewRasterizer(config)
	if err != nil {
		return nil, err
	}

	return &Renderer{
		config:     *config,
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
	if err := r.encodeToWebM(ctx, rgbaFrames, w); err != nil {
		return fmt.Errorf("failed to encode WebM: %w", err)
	}

	if r.config.Debug {
		log.Printf("[WebM] Phase 2 - FFmpeg encoding: %v", time.Since(encodeStart))
		log.Printf("[WebM] Total time: %v", time.Since(startTime))
	}

	return nil
}

func (r *Renderer) encodeToWebM(ctx context.Context, frames []raster.RasterFrame, w io.Writer) error {
	if len(frames) == 0 {
		return fmt.Errorf("no frames to encode")
	}

	// Frames shorter than one frame of a 30 FPS video are not worth encoding.
	filteredFrames := r.filterFrames(frames)
	if r.config.Debug {
		log.Printf("[WebM] Filtered %d frames -> %d frames (skipped %d rapid frames)",
			len(frames), len(filteredFrames), len(frames)-len(filteredFrames))
	}
	if len(filteredFrames) == 0 || filteredFrames[0].Image == nil {
		return fmt.Errorf("no valid frames to encode")
	}

	// ffmpeg reads the frames from a directory and writes the video to a file
	// in it: a file it can seek in gets a duration and a seek index, which a
	// pipe cannot have.
	dir, err := os.MkdirTemp("", "termsvg-webm-")
	if err != nil {
		return fmt.Errorf("failed to create a work directory: %w", err)
	}
	defer os.RemoveAll(dir)

	listPath, err := writeFrameFiles(ctx, dir, filteredFrames, r.config.ProgressCh)
	if err != nil {
		return err
	}
	outPath := filepath.Join(dir, "out.webm")
	if err := runFFmpeg(ctx, ffmpegArgs(listPath, outPath, r.config.VideoBitrate)); err != nil {
		return err
	}

	video, err := os.Open(outPath) //nolint:gosec // a file ffmpeg just wrote in our own temp dir
	if err != nil {
		return fmt.Errorf("failed to read the encoded video: %w", err)
	}
	defer video.Close()
	if _, err := io.Copy(w, video); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// filterFrames filters frames to skip rapid events.
// At 30 FPS, each frame displays for ~33ms. Frames with shorter delays are skipped
// and their delay is accumulated to the next frame.
func (r *Renderer) filterFrames(frames []raster.RasterFrame) []raster.RasterFrame {
	const minDelay = 33 * time.Millisecond // Minimum display time at 30 FPS

	filtered := make([]raster.RasterFrame, 0, len(frames))
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
