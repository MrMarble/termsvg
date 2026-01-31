package webm

import (
	"bytes"
	"context"
	"image/color"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestRenderer_Format(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	if got := r.Format(); got != "webm" {
		t.Errorf("Format() = %v, want %v", got, "webm")
	}
}

func TestRenderer_FileExtension(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	if got := r.FileExtension(); got != ".webm" {
		t.Errorf("FileExtension() = %v, want %v", got, ".webm")
	}
}

func TestRenderer_Render_EmptyRecording(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	rec := &ir.Recording{
		Frames: []ir.Frame{},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err == nil {
		t.Error("expected error for empty recording")
	}
}

func TestRenderer_Render_SingleFrame(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: 1 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{
						Y: 0,
						Runs: []ir.TextRun{
							{
								Text:     "Hello, World!",
								StartCol: 0,
								Attrs:    ir.CellAttrs{},
							},
						},
					},
				},
				Cursor: ir.Cursor{Col: 13, Row: 0, Visible: true},
			},
		},
		Colors: colors,
		Stats:  ir.Stats{TotalFrames: 1},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// WebM files start with specific bytes (EBML header)
	// Check that we got some output
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}

	// WebM files start with 0x1A 0x45 0xDF 0xA3 (EBML ID)
	data := buf.Bytes()
	if len(data) < 4 {
		t.Error("output too short")
		return
	}

	// Check for EBML header signature
	if data[0] != 0x1A || data[1] != 0x45 || data[2] != 0xDF || data[3] != 0xA3 {
		t.Errorf("output does not appear to be a valid WebM file (missing EBML header)")
	}
}

func TestRenderer_Render_MultipleFrames(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    40,
		Height:   10,
		Duration: 2 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 500 * time.Millisecond,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 1", StartCol: 0}}},
				},
			},
			{
				Time:  500 * time.Millisecond,
				Delay: 500 * time.Millisecond,
				Index: 1,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 2", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 500 * time.Millisecond,
				Index: 2,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 3", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
		Stats:  ir.Stats{TotalFrames: 3},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestRenderer_Render_WithDebug(t *testing.T) {
	config := renderer.DefaultConfig()
	config.Debug = true

	r, err := New(config)
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    40,
		Height:   10,
		Duration: 1 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Test", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestNew_WithoutFFmpeg(t *testing.T) {
	// This test verifies that New() returns an error when FFmpeg is not installed
	// Since we can't uninstall FFmpeg during the test, we just verify the error message format
	// by checking that the error mentions FFmpeg

	// If FFmpeg IS installed, this test should skip
	// If FFmpeg is NOT installed, this documents the expected behavior
	_, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg is not installed, verify error message
		if err.Error() != "ffmpeg is not installed. Install it from: https://ffmpeg.org" {
			t.Errorf("unexpected error message: %v", err)
		}
	}
	// If no error, FFmpeg is installed and we can't test the error case
}
