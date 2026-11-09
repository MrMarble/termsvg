package color

import (
	"image/color"
	"testing"
)

func TestParseHex(t *testing.T) {
	tests := []struct {
		in      string
		want    color.RGBA
		wantErr bool
	}{
		{in: "#ff5733", want: color.RGBA{R: 255, G: 0x57, B: 0x33, A: 255}},
		{in: "ff5733", want: color.RGBA{R: 255, G: 0x57, B: 0x33, A: 255}},
		{in: "#f53", want: color.RGBA{R: 0xff, G: 0x55, B: 0x33, A: 255}},
		{in: "#FFFFFF", want: color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{in: "", wantErr: true},
		{in: "#ffff", wantErr: true},
		{in: "#gg0000", wantErr: true},
		{in: "#0g0000", wantErr: true},
		{in: "#0000g0", wantErr: true},
		{in: "#g00", wantErr: true},
		{in: "#0g0", wantErr: true},
		{in: "#00g", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseHex(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
