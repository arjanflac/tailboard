package clipboard

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestWindowsHTMLClipboardEnvelope(t *testing.T) {
	fragment := []byte("<b>hello</b>")
	envelope := buildHTMLClipboard(fragment)
	if got := extractHTMLFragment(envelope); !bytes.Equal(got, fragment) {
		t.Fatalf("fragment = %q", got)
	}
}

func TestWindowsDIBPNGConversion(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.Set(0, 0, color.NRGBA{R: 255, A: 255})
	source.Set(1, 0, color.NRGBA{G: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	dib, err := pngToDIB(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := dibToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(roundTrip))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != source.Bounds() {
		t.Fatalf("bounds = %v", decoded.Bounds())
	}
}
