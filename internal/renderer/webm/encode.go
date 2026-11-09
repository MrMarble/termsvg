package webm

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/raster"
)

const (
	// videoCRF is the constant-quality target. A terminal is mostly flat color
	// and sharp text, so 28 keeps text crisp while the file stays small.
	videoCRF = 28

	// minFrameDuration is the least a frame is shown: one frame of a 30 FPS video.
	minFrameDuration = time.Second / 30
)

// ffmpegArgs returns the ffmpeg arguments that encode the frames listed in the
// concat file at listPath into the WebM at outPath.
//
// Only distinct frames are encoded, each held for its own duration (a variable
// frame rate video), instead of repeating a frame 30 times a second while the
// screen does not change. That is what makes the slowest, best-compressing
// VP9 preset affordable and the files small.
//
// The result is meant to be the same bytes for the same recording on any
// machine: the options below leave out the random identifiers ffmpeg would add
// and, unlike row multithreading, do not make the output depend on how many
// cores there are.
//
// bitrateKbps, when positive, caps the bitrate of the constant-quality encode.
func ffmpegArgs(listPath, outPath string, bitrateKbps int) []string {
	bitrate := "0" // no cap: quality alone decides
	if bitrateKbps > 0 {
		bitrate = strconv.Itoa(bitrateKbps) + "k"
	}

	return []string{
		"-v", "error",
		"-y",
		"-f", "concat", "-i", listPath,
		"-c:v", "libvpx-vp9",
		"-pix_fmt", "yuv420p",
		"-crf", strconv.Itoa(videoCRF), "-b:v", bitrate,
		"-deadline", "good", "-cpu-used", "0",
		// Bit-exact output: no random segment UID, encoding date or encoder tag.
		"-fflags", "+bitexact",
		"-flags:v", "+bitexact",
		"-map_metadata", "-1",
		outPath,
	}
}

// writeFrameFiles writes each frame as a PNG in dir, and the list that tells
// ffmpeg's concat demuxer to show them in order, each for its delay. It
// returns the path of the list.
func writeFrameFiles(
	ctx context.Context,
	dir string,
	frames []raster.RasterFrame,
	progressCh chan<- progress.Update,
) (string, error) {
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	var list strings.Builder
	var last string

	report := func(done int) {
		if progressCh != nil {
			progressCh <- progress.Update{Phase: "Encoding", Current: done, Total: len(frames)}
		}
	}
	report(0)

	written := 0
	for _, frame := range frames {
		if frame.Image == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}

		name := fmt.Sprintf("f%06d.png", written)
		if err := writePNG(&encoder, filepath.Join(dir, name), frame); err != nil {
			return "", err
		}
		// The last frame has no next frame to be replaced by, so it still gets
		// a frame's worth of time even when its delay is zero.
		fmt.Fprintf(&list, "file '%s'\nduration %.6f\n", name, max(frame.Delay, minFrameDuration).Seconds())
		last = name
		written++
		report(written)
	}
	if written == 0 {
		return "", fmt.Errorf("no valid frames to encode")
	}
	// The concat demuxer ignores the duration of the final entry unless the
	// file is listed once more.
	fmt.Fprintf(&list, "file '%s'\n", last)

	listPath := filepath.Join(dir, "frames.txt")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o600); err != nil {
		return "", fmt.Errorf("failed to write the frame list: %w", err)
	}

	return listPath, nil
}

func writePNG(encoder *png.Encoder, path string, frame raster.RasterFrame) error {
	file, err := os.Create(path) //nolint:gosec // a file in our own temp dir
	if err != nil {
		return fmt.Errorf("failed to write a frame: %w", err)
	}
	if err := encoder.Encode(file, frame.Image); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to encode a frame: %w", err)
	}

	return file.Close()
}

// runFFmpeg runs ffmpeg and, when it fails, includes what it said.
func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...) //nolint:gosec // args come from ffmpegArgs
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// After the context ends, stop waiting for output even if something ffmpeg
	// started still holds its pipes open.
	cmd.WaitDelay = time.Second

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if msg := strings.TrimSpace(tail(stderr.String(), 2000)); msg != "" {
			return fmt.Errorf("ffmpeg encoding failed: %w: %s", err, msg)
		}

		return fmt.Errorf("ffmpeg encoding failed: %w", err)
	}

	return nil
}

// tail returns the last n bytes of s.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[len(s)-n:]
}
