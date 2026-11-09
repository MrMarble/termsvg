package webm

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// argValue returns the argument that follows flag, and whether flag is there.
func argValue(args []string, flag string) (string, bool) {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return "", false
	}
	return args[i+1], true
}

func TestFFmpegArgs_ConstantQualityByDefault(t *testing.T) {
	args := ffmpegArgs("/work/frames.txt", "/work/out.webm", 0)

	for flag, want := range map[string]string{
		"-f": "concat", "-i": "/work/frames.txt", "-c:v": "libvpx-vp9", "-pix_fmt": "yuv420p",
		"-crf": strconv.Itoa(videoCRF), "-b:v": "0", "-deadline": "good", "-cpu-used": "0",
	} {
		if got, ok := argValue(args, flag); !ok || got != want {
			t.Errorf("%s = %q (present %v), want %q", flag, got, ok, want)
		}
	}
	if args[len(args)-1] != "/work/out.webm" {
		t.Errorf("the output path must be last, got %q", args[len(args)-1])
	}
}

func TestFFmpegArgs_ReproducibleAcrossMachines(t *testing.T) {
	args := ffmpegArgs("l", "o", 0)
	for _, want := range [][]string{{"-fflags", "+bitexact"}, {"-flags:v", "+bitexact"}, {"-map_metadata", "-1"}} {
		if got, _ := argValue(args, want[0]); got != want[1] {
			t.Errorf("%s = %q, want %q (random metadata would make the output differ)", want[0], got, want[1])
		}
	}
	// Row multithreading makes the bitstream depend on the number of cores.
	if slices.Contains(args, "-row-mt") || slices.Contains(args, "-threads") {
		t.Error("threading options make the output differ between machines")
	}
}

func TestFFmpegArgs_BitrateCapComesBeforeTheOutput(t *testing.T) {
	// Options after the output file are ignored by ffmpeg, which is how a
	// bitrate setting used to be lost.
	args := ffmpegArgs("l", "/work/out.webm", 800)
	i := slices.Index(args, "-b:v")
	if i < 0 || args[i+1] != "800k" {
		t.Fatalf("args = %v, want -b:v 800k", args)
	}
	if i > slices.Index(args, "/work/out.webm") {
		t.Error("-b:v comes after the output file, where ffmpeg ignores it")
	}
	if got, _ := argValue(args, "-crf"); got != strconv.Itoa(videoCRF) {
		t.Errorf("the quality target must stay when a cap is set, -crf = %q", got)
	}
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestWriteFrameFiles(t *testing.T) {
	red, blue := solid(8, 4, color.RGBA{R: 200, A: 255}), solid(8, 4, color.RGBA{B: 200, A: 255})
	frames := []raster.RasterFrame{
		{Image: red, Delay: 1500 * time.Millisecond},
		{Delay: time.Second}, // nil images are skipped
		{Image: blue, Delay: 2 * time.Millisecond},
		{Image: red, Delay: 0}, // the last frame is still shown for a moment
	}
	dir := t.TempDir()

	list, err := writeFrameFiles(context.Background(), dir, frames, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(list) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	want := "file 'f000000.png'\nduration 1.500000\n" +
		"file 'f000001.png'\nduration 0.033333\n" + // a frame shorter than a video frame is rounded up
		"file 'f000002.png'\nduration 0.033333\n" +
		"file 'f000002.png'\n" // the final entry is repeated so its duration counts
	if string(got) != want {
		t.Errorf("frame list:\n%s\nwant:\n%s", got, want)
	}

	// The frames are kept exactly, in order.
	for i, original := range []*image.RGBA{red, blue, red} {
		file, err := os.Open(filepath.Join(dir, "f00000"+strconv.Itoa(i)+".png")) //nolint:gosec // test temp file
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(file)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		for y := range 4 {
			for x := range 8 {
				r1, g1, b1, _ := decoded.At(x, y).RGBA()
				r2, g2, b2, _ := original.At(x, y).RGBA()
				if r1 != r2 || g1 != g2 || b1 != b2 {
					t.Fatalf("frame %d differs from the source at (%d,%d)", i, x, y)
				}
			}
		}
	}
}

func TestWriteFrameFiles_ReportsProgress(t *testing.T) {
	ch := make(chan progress.Update, 8)
	img := solid(2, 2, color.RGBA{A: 255})
	frames := []raster.RasterFrame{{Image: img, Delay: time.Second}, {Image: img, Delay: time.Second}}
	if _, err := writeFrameFiles(context.Background(), t.TempDir(), frames, ch); err != nil {
		t.Fatal(err)
	}
	close(ch)
	var got []int
	for u := range ch {
		if u.Phase != "Encoding" || u.Total != 2 {
			t.Errorf("update = %+v", u)
		}
		got = append(got, u.Current)
	}
	if !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("progress = %v, want 0, 1, 2", got)
	}
}

func TestWriteFrameFiles_Errors(t *testing.T) {
	frame := raster.RasterFrame{Image: solid(2, 2, color.RGBA{A: 255}), Delay: time.Second}

	t.Run("no usable frames", func(t *testing.T) {
		onlyNil := []raster.RasterFrame{{Delay: time.Second}}
		if _, err := writeFrameFiles(context.Background(), t.TempDir(), onlyNil, nil); err == nil {
			t.Error("expected an error")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := writeFrameFiles(ctx, t.TempDir(), []raster.RasterFrame{frame}, nil); !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	})
	t.Run("unwritable directory", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		if _, err := writeFrameFiles(context.Background(), missing, []raster.RasterFrame{frame}, nil); err == nil {
			t.Error("expected an error")
		}
	})
}

// fakeFFmpeg puts an executable named ffmpeg ahead of any real one on PATH.
func fakeFFmpeg(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunFFmpeg_ReportsWhatFFmpegSaid(t *testing.T) {
	fakeFFmpeg(t, "echo 'Unknown encoder libvpx-vp9' >&2\nexit 1\n")
	err := runFFmpeg(context.Background(), []string{"-v", "error"})
	if err == nil || !strings.Contains(err.Error(), "Unknown encoder libvpx-vp9") {
		t.Fatalf("error = %v, want ffmpeg's own message in it", err)
	}
	if !strings.Contains(err.Error(), "ffmpeg encoding failed") {
		t.Errorf("error = %v", err)
	}
}

func TestRunFFmpeg_SilentFailureStillErrors(t *testing.T) {
	fakeFFmpeg(t, "exit 3\n")
	if err := runFFmpeg(context.Background(), nil); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunFFmpeg_CancellationStopsFFmpeg(t *testing.T) {
	fakeFFmpeg(t, "sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := runFFmpeg(ctx, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want the context's error", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("ffmpeg was not stopped when the context ended")
	}
}

func TestTail(t *testing.T) {
	if got := tail("abcdef", 3); got != "def" {
		t.Errorf("tail = %q", got)
	}
	if got := tail("abc", 10); got != "abc" {
		t.Errorf("tail = %q", got)
	}
}

// --- end to end, with the real ffmpeg ---

func requireFFmpeg(t *testing.T) *Renderer {
	t.Helper()
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Skipf("FFmpeg not installed: %v", err)
	}
	return r
}

// idleRecording shows one screen, waits a long time, then shows another.
func idleRecording() *ir.Recording {
	run := func(text string) []ir.Row { return []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: text, StartCol: 0}}}} }
	return &ir.Recording{
		Width: 30, Height: 3, Duration: 20 * time.Second,
		Colors: termcolor.NewCatalog(color.RGBA{R: 192, G: 192, B: 192, A: 255}, color.RGBA{A: 255}),
		Frames: []ir.Frame{
			{Index: 0, Time: 0, Delay: 10 * time.Second, Rows: run("before the pause")},
			{Index: 1, Time: 10 * time.Second, Delay: 10 * time.Second, Rows: run("after the pause")},
		},
	}
}

func TestRenderer_Render_WritesASeekableVariableFrameRateVideo(t *testing.T) {
	r := requireFFmpeg(t)
	var buf bytes.Buffer
	if err := r.Render(context.Background(), idleRecording(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	data := buf.Bytes()

	if !bytes.HasPrefix(data, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		t.Fatal("not a WebM file")
	}
	// Written to a seekable file, the video has a duration and a seek index,
	// which output piped to a non-seekable stream does not.
	if !bytes.Contains(data, []byte{0x44, 0x89}) {
		t.Error("no Duration element")
	}
	if !bytes.Contains(data, []byte{0x1c, 0x53, 0xbb, 0x6b}) {
		t.Error("no Cues (seek index)")
	}

	// 20 seconds of video with two distinct screens must hold about two frames,
	// not the 600 a fixed 30 FPS encode would have.
	if frames := countVideoFrames(t, data); frames > 4 {
		t.Errorf("video has %d frames, want only the distinct ones", frames)
	}
	// And it must play for as long as the recording (the first frame's delay
	// plus the second's): no drift from rounding frame counts.
	if secs := videoSeconds(t, data); secs < 19.9 || secs > 20.5 {
		t.Errorf("video lasts %.2fs, want about 20s", secs)
	}
}

func ffmpegOutput(t *testing.T, data []byte, args ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.webm")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffmpeg", append([]string{"-i", path}, args...)...) //nolint:gosec // test fixture
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg could not read the video: %v\n%s", err, out)
	}
	return string(out)
}

func countVideoFrames(t *testing.T, data []byte) int {
	t.Helper()
	out := ffmpegOutput(t, data, "-c", "copy", "-f", "null", "-")
	matches := regexp.MustCompile(`frame=\s*(\d+)`).FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		t.Fatalf("no frame count in:\n%s", out)
	}
	n, err := strconv.Atoi(matches[len(matches)-1][1])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func videoSeconds(t *testing.T, data []byte) float64 {
	t.Helper()
	out := ffmpegOutput(t, data, "-f", "null", "-")
	m := regexp.MustCompile(`Duration: (\d+):(\d+):(\d+\.\d+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no duration in:\n%s", out)
	}
	hours, _ := strconv.Atoi(m[1])
	minutes, _ := strconv.Atoi(m[2])
	seconds, _ := strconv.ParseFloat(m[3], 64)
	return float64(hours*3600+minutes*60) + seconds
}

func TestRenderer_Render_LeavesNoTemporaryFiles(t *testing.T) {
	r := requireFFmpeg(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	if err := r.Render(context.Background(), idleRecording(), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Also when it fails half way.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = r.Render(ctx, idleRecording(), &bytes.Buffer{})

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("temporary files left behind: %v", names)
	}
}

func TestRenderer_Render_FFmpegFailureCarriesItsMessageAndCleansUp(t *testing.T) {
	fakeFFmpeg(t, "echo 'something went wrong in the encoder' >&2\nexit 1\n")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	r := newUnprobedRenderer(t)

	err := r.Render(context.Background(), idleRecording(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "something went wrong in the encoder") {
		t.Fatalf("error = %v, want ffmpeg's message", err)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("%d temporary entries left behind", len(entries))
	}
}
