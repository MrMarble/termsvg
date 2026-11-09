package raster

import (
	"fmt"
	"image"
	"image/draw"
	"runtime"
	"sync"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
)

// baseImage draws what every frame shares: the window chrome or plain
// background, and the terminal area. The terminal area uses the theme's
// WindowBackground, which is opaque and matches the chrome, so GIF delta
// encoding sees only the pixels that change.
func (r *Rasterizer) baseImage(rec *ir.Recording) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, paddedWidth(rec.Width), r.paddedHeight(rec.Height)))

	if r.config.ShowWindow {
		r.drawWindow(img)
	} else {
		r.drawBackground(img)
	}
	r.drawTerminalBackground(img, contentWidth(rec.Width), contentHeight(rec.Height))

	return img
}

// renderFrames draws every frame of rec on a clone of base, using one worker per CPU.
func renderFrames[I draw.Image](r *Rasterizer, rec *ir.Recording, base I, clone func(I) I) ([]I, error) {
	frames := rec.Frames
	images := make([]I, len(frames))

	report := func(done int) {
		if r.config.ProgressCh != nil {
			r.config.ProgressCh <- progress.Update{Phase: progress.PhaseRasterize, Current: done, Total: len(frames)}
		}
	}
	report(0)

	jobs := make(chan int, len(frames))
	errs := make(chan error, runtime.NumCPU())
	var wg sync.WaitGroup

	for range runtime.NumCPU() {
		wg.Go(func() {
			// A font face is not safe for concurrent use.
			face, err := loadFontFace(float64(r.config.FontSize), r.sysFonts)
			if err != nil {
				errs <- fmt.Errorf("failed to load font: %w", err)
				return
			}
			defer face.Close()

			for idx := range jobs {
				img := clone(base)
				for _, row := range frames[idx].Rows {
					for _, run := range row.Runs {
						r.drawTextRun(img, run, row.Y, face, rec.Colors)
					}
				}
				if r.config.ShowCursor && frames[idx].Cursor.Visible {
					r.drawCursor(img, frames[idx].Cursor, rec.Colors)
				}
				images[idx] = img

				report(idx + 1)
			}
		})
	}

	for i := range frames {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(errs)

	if err := <-errs; err != nil {
		return nil, err
	}

	return images, nil
}
