package dashboard

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// buildICOWithPNGFrame produces a valid single-frame .ico whose payload is
// a PNG, which is how modern high-resolution icons are stored.
func buildICOWithPNGFrame(t *testing.T, width, height int) []byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 90, A: 255})
		}
	}
	var payload bytes.Buffer
	if err := png.Encode(&payload, src); err != nil {
		t.Fatalf("encode png frame: %v", err)
	}
	return assembleICO(t, width, height, 32, payload.Bytes())
}

// buildICOWithDIBFrame produces a valid single-frame .ico whose payload is
// a headerless 32bpp bitmap with the doubled height and trailing AND mask
// the format requires.
func buildICOWithDIBFrame(t *testing.T, width, height int) []byte {
	t.Helper()
	header := make([]byte, dibHeaderSize)
	binary.LittleEndian.PutUint32(header[0:4], dibHeaderSize)
	binary.LittleEndian.PutUint32(header[4:8], uint32(width))
	// The declared height covers the colour rows plus the mask rows.
	binary.LittleEndian.PutUint32(header[8:12], uint32(height*2))
	binary.LittleEndian.PutUint16(header[12:14], 1)
	binary.LittleEndian.PutUint16(header[14:16], 32)

	pixels := make([]byte, width*height*4)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 40, 140, 210, 255
	}
	maskRowBytes := ((width + 31) / 32) * 4
	mask := make([]byte, maskRowBytes*height)

	payload := append(append(append([]byte{}, header...), pixels...), mask...)
	return assembleICO(t, width, height, 32, payload)
}

func assembleICO(t *testing.T, width, height, bits int, payload []byte) []byte {
	t.Helper()
	out := make([]byte, icoHeaderSize+icoEntrySize)
	binary.LittleEndian.PutUint16(out[2:4], 1)
	binary.LittleEndian.PutUint16(out[4:6], 1)

	entry := out[icoHeaderSize:]
	if width < 256 {
		entry[0] = byte(width)
	}
	if height < 256 {
		entry[1] = byte(height)
	}
	binary.LittleEndian.PutUint16(entry[6:8], uint16(bits))
	binary.LittleEndian.PutUint32(entry[8:12], uint32(len(payload)))
	binary.LittleEndian.PutUint32(entry[12:16], uint32(icoHeaderSize+icoEntrySize))
	return append(out, payload...)
}

// ICO is the dominant cover format in a real Playnite library, so a
// failure here silently returns the library grid to full-size images.
func TestMakeThumbnailDecodesICOWithPNGFrame(t *testing.T) {
	source := buildICOWithPNGFrame(t, 512, 512)

	thumb, ok := makeThumbnail(source)
	if !ok {
		t.Fatal("expected a PNG-framed icon to be thumbnailed, not passed through at full size")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("format = %q, want jpeg", format)
	}
	if cfg.Width != thumbMaxDimension || cfg.Height != thumbMaxDimension {
		t.Fatalf("thumbnail = %dx%d, want %dx%d", cfg.Width, cfg.Height, thumbMaxDimension, thumbMaxDimension)
	}
}

func TestMakeThumbnailDecodesICOWithBitmapFrame(t *testing.T) {
	source := buildICOWithDIBFrame(t, 64, 64)

	thumb, ok := makeThumbnail(source)
	if !ok {
		t.Fatal("expected a bitmap-framed icon to be thumbnailed")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	// The frame is already smaller than the bound, so it must not be
	// upscaled, and the doubled ICO height must not leak through.
	if cfg.Width != 64 || cfg.Height != 64 {
		t.Fatalf("thumbnail = %dx%d, want 64x64", cfg.Width, cfg.Height)
	}
}

// The largest frame is the one worth thumbnailing; icons routinely carry a
// 16x16 frame alongside a 256x256 one.
func TestLargestICOFramePrefersHighestResolution(t *testing.T) {
	small := []byte("SMALL-FRAME-BYTES")
	large := []byte("LARGE-FRAME-BYTES-XXXX")

	out := make([]byte, icoHeaderSize+icoEntrySize*2)
	binary.LittleEndian.PutUint16(out[2:4], 1)
	binary.LittleEndian.PutUint16(out[4:6], 2)

	first := out[icoHeaderSize:]
	first[0], first[1] = 16, 16
	binary.LittleEndian.PutUint32(first[8:12], uint32(len(small)))
	binary.LittleEndian.PutUint32(first[12:16], uint32(icoHeaderSize+icoEntrySize*2))

	second := out[icoHeaderSize+icoEntrySize:]
	// A stored dimension of zero means 256.
	second[0], second[1] = 0, 0
	binary.LittleEndian.PutUint32(second[8:12], uint32(len(large)))
	binary.LittleEndian.PutUint32(second[12:16], uint32(icoHeaderSize+icoEntrySize*2+len(small)))

	out = append(out, small...)
	out = append(out, large...)

	frame, err := largestICOFrame(out)
	if err != nil {
		t.Fatalf("largestICOFrame: %v", err)
	}
	if !bytes.Equal(frame, large) {
		t.Fatalf("frame = %q, want the 256x256 frame %q", frame, large)
	}
}

func TestLargestICOFrameRejectsOutOfRangeEntry(t *testing.T) {
	out := make([]byte, icoHeaderSize+icoEntrySize)
	binary.LittleEndian.PutUint16(out[2:4], 1)
	binary.LittleEndian.PutUint16(out[4:6], 1)
	entry := out[icoHeaderSize:]
	entry[0], entry[1] = 32, 32
	// Claim a payload that runs past the end of the file.
	binary.LittleEndian.PutUint32(entry[8:12], 4096)
	binary.LittleEndian.PutUint32(entry[12:16], uint32(icoHeaderSize+icoEntrySize))

	if _, err := largestICOFrame(out); err == nil {
		t.Fatal("expected an entry pointing past end-of-file to be rejected")
	}
}

func riffChunk(fourCC string, payload []byte) []byte {
	out := append([]byte(fourCC), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(payload)))
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func riffContainer(chunks ...[]byte) []byte {
	body := []byte{}
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := append([]byte("RIFF"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(4+len(body)))
	out = append(out, "WEBP"...)
	return append(out, body...)
}

// Steam ships extended (VP8X) WebP art, so the still frame has to be
// recoverable from inside the container.
func TestNormalizeExtendedWebPExtractsStillFrame(t *testing.T) {
	coded := []byte("vp8l-payload-bytes")
	source := riffContainer(
		riffChunk("VP8X", make([]byte, 10)),
		riffChunk("ICCP", []byte("colour-profile")),
		riffChunk("ALPH", []byte("alpha")),
		riffChunk("VP8L", coded),
	)

	out, ok := normalizeExtendedWebP(source)
	if !ok {
		t.Fatal("expected an extended WebP to be normalized")
	}
	if !bytes.Equal(out[0:4], []byte("RIFF")) || !bytes.Equal(out[8:12], []byte("WEBP")) {
		t.Fatalf("output is not a WebP container: %q", out[:12])
	}
	if got := string(out[12:16]); got != "VP8L" {
		t.Fatalf("chunk = %q, want VP8L", got)
	}
	if got := out[riffHeaderSize+riffChunkSize : riffHeaderSize+riffChunkSize+len(coded)]; !bytes.Equal(got, coded) {
		t.Fatalf("payload = %q, want %q", got, coded)
	}
	declared := binary.LittleEndian.Uint32(out[4:8])
	if int(declared) != len(out)-riffChunkSize {
		t.Fatalf("declared RIFF size %d does not match body length %d", declared, len(out)-riffChunkSize)
	}
}

// Animated art stores its frames inside ANMF chunks; the first one is a
// perfectly good thumbnail.
func TestNormalizeExtendedWebPExtractsFirstAnimationFrame(t *testing.T) {
	coded := []byte("first-animation-frame")
	later := []byte("second-animation-frame")
	source := riffContainer(
		riffChunk("VP8X", make([]byte, 10)),
		riffChunk("ANIM", make([]byte, 6)),
		riffChunk("ANMF", append(make([]byte, anmfHeaderSize), riffChunk("VP8 ", coded)...)),
		riffChunk("ANMF", append(make([]byte, anmfHeaderSize), riffChunk("VP8 ", later)...)),
	)

	out, ok := normalizeExtendedWebP(source)
	if !ok {
		t.Fatal("expected an animated WebP to yield its first frame")
	}
	if got := string(out[12:16]); got != "VP8 " {
		t.Fatalf("chunk = %q, want \"VP8 \"", got)
	}
	body := out[riffHeaderSize+riffChunkSize:]
	if !bytes.HasPrefix(body, coded) {
		t.Fatalf("payload = %q, want the first frame %q", body, coded)
	}
}

func TestNormalizeExtendedWebPRejectsNonWebP(t *testing.T) {
	if _, ok := normalizeExtendedWebP([]byte("not a riff container at all")); ok {
		t.Fatal("expected non-WebP input to be rejected")
	}
	if _, ok := normalizeExtendedWebP(riffContainer(riffChunk("VP8X", make([]byte, 10)))); ok {
		t.Fatal("expected a container with no coded frame to be rejected")
	}
}

// A small file can declare enormous dimensions. Decoding one would
// allocate hundreds of megabytes, so it must be refused before the pixels
// are read and served as-is instead.
func TestMakeThumbnailRefusesOversizedImage(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	source := buf.Bytes()
	// Rewrite the IHDR dimensions to claim a bomb-sized image.
	binary.BigEndian.PutUint32(source[16:20], 40000)
	binary.BigEndian.PutUint32(source[20:24], 40000)

	if _, ok := makeThumbnail(source); ok {
		t.Fatal("expected an image exceeding the pixel ceiling to be refused")
	}
}
