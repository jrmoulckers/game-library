package dashboard

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"io"

	"golang.org/x/image/bmp"
)

// ICO is the most common cover format in a real Playnite library, so the
// grid depends on being able to downscale it. The Go standard library has
// no ICO decoder, and without one every icon fell through to the
// full-resolution fallback path.
//
// An .ico file is a directory of independently encoded frames. Each frame
// is either a complete PNG or a headerless BMP ("DIB"). We pick the
// largest frame and hand it to the matching decoder.
func init() {
	image.RegisterFormat("ico", "\x00\x00\x01\x00", decodeICO, decodeICOConfig)
}

const (
	icoHeaderSize = 6
	icoEntrySize  = 16
	dibHeaderSize = 40
)

var errNotICO = errors.New("dashboard: not a decodable ICO")

type icoEntry struct {
	width  int
	height int
	bits   int
	offset int
	length int
}

func decodeICO(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxThumbSourceBytes))
	if err != nil {
		return nil, err
	}
	frame, err := largestICOFrame(data)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(frame, []byte("\x89PNG\r\n\x1a\n")) {
		return png.Decode(bytes.NewReader(frame))
	}
	converted, err := dibToBMP(frame)
	if err != nil {
		return nil, err
	}
	return bmp.Decode(bytes.NewReader(converted))
}

func decodeICOConfig(r io.Reader) (image.Config, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxThumbSourceBytes))
	if err != nil {
		return image.Config{}, err
	}
	frame, err := largestICOFrame(data)
	if err != nil {
		return image.Config{}, err
	}
	if bytes.HasPrefix(frame, []byte("\x89PNG\r\n\x1a\n")) {
		return png.DecodeConfig(bytes.NewReader(frame))
	}
	converted, err := dibToBMP(frame)
	if err != nil {
		return image.Config{}, err
	}
	return bmp.DecodeConfig(bytes.NewReader(converted))
}

// largestICOFrame returns the encoded bytes of the highest-resolution
// frame, preferring greater colour depth when two frames tie on area.
func largestICOFrame(data []byte) ([]byte, error) {
	if len(data) < icoHeaderSize {
		return nil, errNotICO
	}
	if binary.LittleEndian.Uint16(data[0:2]) != 0 {
		return nil, errNotICO
	}
	if kind := binary.LittleEndian.Uint16(data[2:4]); kind != 1 && kind != 2 {
		return nil, errNotICO
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 {
		return nil, errNotICO
	}
	if len(data) < icoHeaderSize+count*icoEntrySize {
		return nil, errNotICO
	}

	best := icoEntry{}
	found := false
	for i := 0; i < count; i++ {
		raw := data[icoHeaderSize+i*icoEntrySize:]
		entry := icoEntry{
			// A stored dimension of 0 means 256 pixels.
			width:  int(raw[0]),
			height: int(raw[1]),
			bits:   int(binary.LittleEndian.Uint16(raw[6:8])),
			length: int(binary.LittleEndian.Uint32(raw[8:12])),
			offset: int(binary.LittleEndian.Uint32(raw[12:16])),
		}
		if entry.width == 0 {
			entry.width = 256
		}
		if entry.height == 0 {
			entry.height = 256
		}
		if entry.length <= 0 || entry.offset < 0 {
			continue
		}
		if entry.offset+entry.length > len(data) {
			continue
		}
		if !found || betterICOFrame(entry, best) {
			best, found = entry, true
		}
	}
	if !found {
		return nil, errNotICO
	}
	return data[best.offset : best.offset+best.length], nil
}

func betterICOFrame(candidate, current icoEntry) bool {
	candidateArea := candidate.width * candidate.height
	currentArea := current.width * current.height
	if candidateArea != currentArea {
		return candidateArea > currentArea
	}
	return candidate.bits > current.bits
}

// dibToBMP wraps a headerless ICO bitmap in the 14-byte file header a BMP
// decoder expects.
//
// An ICO's DIB declares double its true height because the frame stores a
// colour image stacked on top of a 1-bit AND mask. Halving the declared
// height leaves the decoder reading only the colour rows; the mask simply
// becomes unread trailing bytes. Icon transparency is therefore dropped,
// which is harmless here because thumbnails are flattened onto an opaque
// background anyway.
func dibToBMP(frame []byte) ([]byte, error) {
	if len(frame) < dibHeaderSize {
		return nil, errNotICO
	}
	if binary.LittleEndian.Uint32(frame[0:4]) != dibHeaderSize {
		return nil, errNotICO
	}
	// Only uncompressed frames are supported; PNG-compressed frames take
	// the PNG path and RLE frames are vanishingly rare.
	if binary.LittleEndian.Uint32(frame[16:20]) != 0 {
		return nil, errNotICO
	}

	height := int32(binary.LittleEndian.Uint32(frame[8:12]))
	if height%2 != 0 || height <= 0 {
		return nil, errNotICO
	}
	bits := int(binary.LittleEndian.Uint16(frame[14:16]))
	paletteEntries := int(binary.LittleEndian.Uint32(frame[32:36]))
	if bits <= 8 && paletteEntries == 0 {
		paletteEntries = 1 << bits
	}
	if bits > 8 {
		paletteEntries = 0
	}
	pixelOffset := 14 + dibHeaderSize + paletteEntries*4
	if pixelOffset > 14+len(frame) {
		return nil, errNotICO
	}

	out := make([]byte, 0, 14+len(frame))
	header := make([]byte, 14)
	header[0], header[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(header[2:6], uint32(14+len(frame)))
	binary.LittleEndian.PutUint32(header[10:14], uint32(pixelOffset))
	out = append(out, header...)
	out = append(out, frame...)

	// Rewrite the declared height in the copied header.
	binary.LittleEndian.PutUint32(out[14+8:14+12], uint32(height/2))
	return out, nil
}
