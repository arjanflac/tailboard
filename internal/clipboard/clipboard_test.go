package clipboard

import "testing"

func TestBestTypePrefersPlainTextOverHTML(t *testing.T) {
	got := bestType([]string{"public-file-url", "text/plain;charset=utf-8", "text/html"})
	if got != "text/plain" {
		t.Fatalf("bestType() = %q, want text/plain", got)
	}
}

func TestBestTypeStillPrefersImageOverText(t *testing.T) {
	got := bestType([]string{"text/plain", "text/html", "image/png"})
	if got != "image/png" {
		t.Fatalf("bestType() = %q, want image/png", got)
	}
}

func TestBestTypeUsesHTMLWhenItIsTheOnlySupportedRepresentation(t *testing.T) {
	got := bestType([]string{"text/html"})
	if got != "text/html" {
		t.Fatalf("bestType() = %q, want text/html", got)
	}
}
