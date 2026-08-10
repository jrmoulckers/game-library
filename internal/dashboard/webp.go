package dashboard

import (
	"bytes"
	"encoding/binary"
	"image"

	"golang.org/x/image/webp"
)

// Steam's modern logo and hero art is extended WebP (a VP8X container),
// frequently animated and occasionally tens of megabytes. golang.org/x/image
// only decodes the simple VP8 and VP8L forms, so these assets failed to
// decode and were served at full size into the library grid.
//
// A thumbnail only needs one still frame. normalizeExtendedWebP finds the
// first coded frame inside the container and rewraps it as a simple WebP
// that the available decoder accepts. Alpha and animation are discarded,
// which is fine because thumbnails are flattened onto an opaque background.

const (
	riffHeaderSize = 12
	riffChunkSize  = 8
	// An ANMF chunk begins with a 16-byte frame header before its own
	// nested image chunks.
	anmfHeaderSize = 16
)

// decodeWebP decodes both simple and extended WebP images.
func decodeWebP(source []byte) (image.Image, bool) {
	if img, err := webp.Decode(bytes.NewReader(source)); err == nil {
		return img, true
	}
	simple, ok := normalizeExtendedWebP(source)
	if !ok {
		return nil, false
	}
	img, err := webp.Decode(bytes.NewReader(simple))
	if err != nil {
		return nil, false
	}
	return img, true
}

func normalizeExtendedWebP(source []byte) ([]byte, bool) {
	if len(source) < riffHeaderSize {
		return nil, false
	}
	if !bytes.Equal(source[0:4], []byte("RIFF")) || !bytes.Equal(source[8:12], []byte("WEBP")) {
		return nil, false
	}
	fourCC, payload, ok := firstCodedFrame(source[riffHeaderSize:])
	if !ok {
		return nil, false
	}

	padded := len(payload)
	if padded%2 == 1 {
		padded++
	}
	out := make([]byte, 0, riffHeaderSize+riffChunkSize+padded)
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(4+riffChunkSize+padded))
	out = append(out, "WEBP"...)
	out = append(out, fourCC...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(payload)))
	out = append(out, payload...)
	if padded != len(payload) {
		out = append(out, 0)
	}
	return out, true
}

// firstCodedFrame walks a RIFF chunk sequence and returns the first VP8 or
// VP8L payload, descending into an animation frame when it finds one.
func firstCodedFrame(chunks []byte) (string, []byte, bool) {
	for offset := 0; offset+riffChunkSize <= len(chunks); {
		fourCC := string(chunks[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(chunks[offset+4 : offset+riffChunkSize]))
		if size < 0 {
			return "", nil, false
		}
		start := offset + riffChunkSize
		if start+size > len(chunks) {
			return "", nil, false
		}
		payload := chunks[start : start+size]

		switch fourCC {
		case "VP8 ", "VP8L":
			return fourCC, payload, true
		case "ANMF":
			if len(payload) > anmfHeaderSize {
				if cc, frame, ok := firstCodedFrame(payload[anmfHeaderSize:]); ok {
					return cc, frame, true
				}
			}
		}

		advance := size
		if advance%2 == 1 {
			advance++
		}
		offset = start + advance
	}
	return "", nil, false
}
