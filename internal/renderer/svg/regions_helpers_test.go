package svg

import (
	"context"
	"io"
)

func (c *canvas) serializedRegionBytes(ctx context.Context, regions []dynamicRegion) (int64, error) {
	content, err := c.prepareLocalViewports(ctx, c.regionBands(regions))
	if err != nil {
		return 0, err
	}
	counter := &countingWriter{}
	if err := writeFinalSVG(counter, c.config.Minify, func(w io.Writer) error {
		return c.renderRegionRepresentation(ctx, w, &content)
	}); err != nil {
		return 0, err
	}
	return counter.size(), nil
}
