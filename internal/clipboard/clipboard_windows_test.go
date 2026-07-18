//go:build windows

package clipboard

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestWindowsClipboardRoundTripsNativeFormats(t *testing.T) {
	backend, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Clear() })

	var imageData bytes.Buffer
	sourceImage := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	sourceImage.Set(0, 0, color.NRGBA{R: 255, A: 255})
	sourceImage.Set(1, 1, color.NRGBA{B: 255, A: 255})
	if err := png.Encode(&imageData, sourceImage); err != nil {
		t.Fatal(err)
	}

	tests := []Content{
		{MimeType: "text/plain", Data: []byte("ClipHub Unicode ✓")},
		{MimeType: "text/html", Data: []byte("<p><strong>ClipHub</strong> HTML</p>")},
		{MimeType: "image/png", Data: imageData.Bytes()},
	}
	for _, want := range tests {
		t.Run(want.MimeType, func(t *testing.T) {
			if err := backend.Write(want); err != nil {
				t.Fatal(err)
			}
			got, err := backend.ReadBest()
			if err != nil {
				t.Fatal(err)
			}
			if got.MimeType != want.MimeType {
				t.Fatalf("MIME type = %q, want %q", got.MimeType, want.MimeType)
			}
			if want.MimeType == "image/png" {
				decoded, err := png.Decode(bytes.NewReader(got.Data))
				if err != nil {
					t.Fatalf("decode round-trip PNG: %v", err)
				}
				if decoded.Bounds() != sourceImage.Bounds() {
					t.Fatalf("image bounds = %v, want %v", decoded.Bounds(), sourceImage.Bounds())
				}
				return
			}
			if !bytes.Equal(got.Data, want.Data) {
				t.Fatalf("content = %q, want %q", got.Data, want.Data)
			}
		})
	}
}
