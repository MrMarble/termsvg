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

	rasterizer, err := raster.New(config)
	if err != nil {
		return nil, err
	}

	return &Renderer{
		config:     *config,
		rasterizer: rasterizer,
	}, nil
}

// Render generates a WebM video from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	start := time.Now()
	frames, err := r.rasterizer.Rasterize(rec)
	if err != nil {
		return fmt.Errorf("failed to rasterize frames: %w", err)
	}
	rasterized := time.Since(start)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := r.encodeToWebM(ctx, frames, w); err != nil {
		return fmt.Errorf("failed to encode WebM: %w", err)
	}
	if r.config.Debug {
		log.Printf("[WebM] %d frames: rasterized in %v, encoded in %v",
			len(frames), rasterized, time.Since(start)-rasterized)
	}

	return nil
}

func (r *Renderer) encodeToWebM(ctx context.Context, frames []raster.RasterFrame, w io.Writer) error {
	filteredFrames := filterFrames(frames)
	if r.config.Debug {
		log.Printf("[WebM] encoding %d of %d frames", len(filteredFrames), len(frames))
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

// filterFrames drops the frames that would be shown for less than one frame of
// a 30 FPS video, handing their time to the next frame. The last frame is always kept.
func filterFrames(frames []raster.RasterFrame) []raster.RasterFrame {
	filtered := make([]raster.RasterFrame, 0, len(frames))
	var accumulatedDelay time.Duration

	for i, frame := range frames {
		totalDelay := frame.Delay + accumulatedDelay
		if totalDelay < raster.VideoMergeBelow && i < len(frames)-1 {
			accumulatedDelay = totalDelay
			continue
		}

		filtered = append(filtered, raster.RasterFrame{Image: frame.Image, Delay: totalDelay})
		accumulatedDelay = 0
	}

	return filtered
}
