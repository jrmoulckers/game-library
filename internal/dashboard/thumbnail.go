package dashboard

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"os"
	"strconv"
	"sync"

	_ "image/gif"
	_ "image/png"

	// Real libraries contain WebP hero art and ICO covers, neither of
	// which the standard library can decode. Without these the thumbnail
	// path silently fell back to serving the original bytes — including
	// individual WebP assets over 40MB.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/jrmoulckers/game-library/internal/review"
)

// mediaThumb serves a small, cacheable copy of an artwork asset for the
// library grid. The grid previously loaded full-resolution art (single
// covers exceed 800KB) with no-store caching, so every re-render
// re-downloaded the entire library.
func (h *handlers) mediaThumb(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	resolution, err := review.ResolveMedia(snapshot, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, review.ErrMediaNotFound):
			writeJSONError(w, http.StatusNotFound, "media_not_found", "no media matches that id")
		default:
			writeJSONError(w, http.StatusForbidden, "media_unsafe", "this media cannot be served safely")
		}
		return
	}
	if !isPreviewableImage(resolution.MIME) {
		writeJSONError(w, http.StatusForbidden, "media_unsafe", "this media type cannot be previewed inline")
		return
	}

	etag := `"thumb-` + resolution.SHA256 + `"`
	if resolution.SHA256 != "" && r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if cached, hit := h.thumbs.get(resolution.SHA256); hit {
		writeThumb(w, cached, "image/jpeg", etag, resolution.SHA256)
		return
	}

	source, err := os.ReadFile(resolution.Path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "media_not_found", "no media matches that id")
		return
	}
	body, mime := source, resolution.MIME
	if thumb, made := makeThumbnail(source); made {
		body, mime = thumb, "image/jpeg"
		h.thumbs.put(resolution.SHA256, thumb)
	}
	writeThumb(w, body, mime, etag, resolution.SHA256)
}

func writeThumb(w http.ResponseWriter, body []byte, mime, etag, hash string) {
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The response is keyed by a content hash, so a long-lived cache
	// entry can never serve stale bytes for changed artwork.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if hash != "" {
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

// thumbMaxDimension bounds the longest edge of a generated grid
// thumbnail. Library artwork is routinely 600x900 or larger (covers of
// 800KB are common), which is far more detail than a grid tile needs.
const thumbMaxDimension = 320

// thumbQuality is the JPEG quality used for generated thumbnails. 78 is
// visually indistinguishable at tile size while keeping payloads small.
const thumbQuality = 78

// thumbCacheEntries bounds how many decoded thumbnails are retained.
// Thumbnails are small (typically 10-30KB), so this cap keeps the whole
// working set of a large library resident without unbounded growth.
const thumbCacheEntries = 4096

// maxThumbSourceBytes bounds how much of a source asset is read while
// decoding. Real artwork runs to tens of megabytes, so the ceiling is
// generous, but it keeps a malformed or hostile file from being read
// without limit.
const maxThumbSourceBytes = 256 << 20

// maxThumbSourcePixels rejects images whose declared dimensions would
// require an unreasonable decode buffer. A decompression bomb is cheap to
// store and expensive to decode, so the size is checked from the header
// before the pixels are read.
const maxThumbSourcePixels = 80 << 20

// thumbnailCache memoises generated thumbnails keyed by the content hash
// of the source asset. Because the key is a content hash, an entry can
// never go stale: different bytes produce a different key.
type thumbnailCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	order   []string
}

func newThumbnailCache() *thumbnailCache {
	return &thumbnailCache{entries: make(map[string][]byte)}
}

func (c *thumbnailCache) get(key string) ([]byte, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.entries[key]
	return data, ok
}

func (c *thumbnailCache) put(key string, data []byte) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; exists {
		return
	}
	if len(c.order) >= thumbCacheEntries {
		evict := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, evict)
	}
	c.entries[key] = data
	c.order = append(c.order, key)
}

// makeThumbnail decodes an image and re-encodes a bounded JPEG copy. It
// returns ok=false for anything it cannot decode (for example WebP,
// which the standard library does not support) so the caller can fall
// back to serving the original bytes rather than failing the request.
func makeThumbnail(source []byte) ([]byte, bool) {
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(source)); err == nil {
		if cfg.Width <= 0 || cfg.Height <= 0 {
			return nil, false
		}
		if int64(cfg.Width)*int64(cfg.Height) > maxThumbSourcePixels {
			return nil, false
		}
	}
	src, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		// Extended (animated or alpha) WebP is not handled by the
		// generic decoder; retry via the single-frame path.
		decoded, ok := decodeWebP(source)
		if !ok {
			return nil, false
		}
		src = decoded
	}
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, false
	}

	dstW, dstH := scaledSize(srcW, srcH, thumbMaxDimension)
	// Flatten onto white first so transparent logos stay legible once
	// encoded as JPEG, which has no alpha channel.
	flat := image.NewRGBA(image.Rect(0, 0, srcW, srcH))
	draw.Draw(flat, flat.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, bounds.Min, draw.Over)

	dst := boxDownscale(flat, dstW, dstH)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: thumbQuality}); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}

// scaledSize fits a width/height inside a square bound while preserving
// aspect ratio. Images already within the bound are left untouched so a
// small asset is never upscaled.
func scaledSize(w, h, max int) (int, int) {
	if w <= max && h <= max {
		return w, h
	}
	if w >= h {
		scaled := int(float64(h) * float64(max) / float64(w))
		if scaled < 1 {
			scaled = 1
		}
		return max, scaled
	}
	scaled := int(float64(w) * float64(max) / float64(h))
	if scaled < 1 {
		scaled = 1
	}
	return scaled, max
}

// boxDownscale averages every source pixel that maps onto each
// destination pixel. For the large reduction ratios involved here a box
// filter is both cheaper and less aliased than point sampling, and it
// avoids taking on an image-resampling dependency.
func boxDownscale(src *image.RGBA, dstW, dstH int) *image.RGBA {
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	if dstW == srcW && dstH == srcH {
		draw.Draw(dst, dst.Bounds(), src, srcBounds.Min, draw.Src)
		return dst
	}

	for y := 0; y < dstH; y++ {
		y0 := y * srcH / dstH
		y1 := (y + 1) * srcH / dstH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dstW; x++ {
			x0 := x * srcW / dstW
			x1 := (x + 1) * srcW / dstW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, count uint64
			for sy := y0; sy < y1; sy++ {
				row := src.PixOffset(srcBounds.Min.X+x0, srcBounds.Min.Y+sy)
				for sx := x0; sx < x1; sx++ {
					r += uint64(src.Pix[row])
					g += uint64(src.Pix[row+1])
					b += uint64(src.Pix[row+2])
					row += 4
					count++
				}
			}
			if count == 0 {
				count = 1
			}
			off := dst.PixOffset(x, y)
			dst.Pix[off] = uint8(r / count)
			dst.Pix[off+1] = uint8(g / count)
			dst.Pix[off+2] = uint8(b / count)
			dst.Pix[off+3] = 0xff
		}
	}
	return dst
}
