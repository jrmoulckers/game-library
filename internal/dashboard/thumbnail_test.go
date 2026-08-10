package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestMakeThumbnailShrinksLargeArtwork covers the reason the endpoint
// exists: library covers are routinely far larger than a grid tile, and
// the grid used to download them at full resolution.
func TestMakeThumbnailShrinksLargeArtwork(t *testing.T) {
	source := encodePNG(t, 1200, 1800)
	thumb, ok := makeThumbnail(source)
	if !ok {
		t.Fatal("expected a large PNG cover to produce a thumbnail")
	}
	if len(thumb) >= len(source) {
		t.Fatalf("thumbnail (%d bytes) should be smaller than the source (%d bytes)", len(thumb), len(source))
	}
	decoded, _, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("thumbnail did not decode: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > thumbMaxDimension || bounds.Dy() > thumbMaxDimension {
		t.Fatalf("thumbnail %dx%d exceeds the %d bound", bounds.Dx(), bounds.Dy(), thumbMaxDimension)
	}
	// A 2:3 cover must stay 2:3 so tiles are not distorted.
	if got, want := float64(bounds.Dx())/float64(bounds.Dy()), 2.0/3.0; got < want-0.02 || got > want+0.02 {
		t.Fatalf("aspect ratio %.3f drifted from the source 0.667", got)
	}
}

// TestMakeThumbnailLeavesSmallArtworkUnscaled guards against upscaling a
// small asset, which would waste bytes and blur the tile.
func TestMakeThumbnailLeavesSmallArtworkUnscaled(t *testing.T) {
	thumb, ok := makeThumbnail(encodePNG(t, 64, 64))
	if !ok {
		t.Fatal("expected a small PNG to produce a thumbnail")
	}
	decoded, _, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("thumbnail did not decode: %v", err)
	}
	if decoded.Bounds().Dx() != 64 || decoded.Bounds().Dy() != 64 {
		t.Fatalf("small artwork was resized to %v", decoded.Bounds())
	}
}

// TestMakeThumbnailRejectsUndecodableBytes proves the handler's fallback
// path is reachable: formats the standard library cannot decode (WebP,
// for example) must be reported so the original bytes are served.
func TestMakeThumbnailRejectsUndecodableBytes(t *testing.T) {
	if _, ok := makeThumbnail([]byte("RIFF____WEBPVP8 not-a-real-image")); ok {
		t.Fatal("expected undecodable bytes to be rejected rather than silently mangled")
	}
}

func TestThumbnailCacheEvictsOldestBeyondCap(t *testing.T) {
	cache := newThumbnailCache()
	cache.put("a", []byte("first"))
	if got, ok := cache.get("a"); !ok || string(got) != "first" {
		t.Fatal("expected a stored thumbnail to be returned")
	}
	if _, ok := cache.get("missing"); ok {
		t.Fatal("expected a miss for an unknown key")
	}
	if _, ok := cache.get(""); ok {
		t.Fatal("an empty content hash must never be treated as a cache hit")
	}
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 251), G: uint8(y % 241), B: uint8((x + y) % 239), A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}
