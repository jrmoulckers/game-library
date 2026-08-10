package metadata

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

type bsonKind uint8

const (
	bsonMissing      bsonKind = iota
	bsonString       bsonKind = 0x02
	bsonBinary       bsonKind = 0x05
	bsonBool         bsonKind = 0x08
	bsonDateTime     bsonKind = 0x09
	bsonNull         bsonKind = 0x0a
	bsonInt32        bsonKind = 0x10
	bsonInt64        bsonKind = 0x12
	bsonDocumentKind bsonKind = 0x03
	bsonArrayKind    bsonKind = 0x04
	bsonOther        bsonKind = 0xff
)

type bsonValue struct {
	kind    bsonKind
	text    string
	bytes   []byte
	subtype byte
	boolean bool
	number  int64
	date    time.Time
	doc     *bsonDocument
}

type bsonField struct {
	name  string
	value bsonValue
}

type bsonDocument struct {
	fields []bsonField
}

func parseBSONDocument(data []byte) (*bsonDocument, int, error) {
	if len(data) < 5 {
		return nil, 0, fmt.Errorf("%w: short BSON document", errPlayniteCorrupt)
	}
	length := int64(int32(binary.LittleEndian.Uint32(data)))
	if length < 5 || length > maxBSONBytes || length > int64(len(data)) {
		return nil, 0, fmt.Errorf("%w: invalid BSON document length", errPlayniteCorrupt)
	}
	doc, end, err := parseBSONDocumentAt(data[:length], 0, 0)
	if err != nil {
		return nil, 0, err
	}
	return doc, end, nil
}

func parseBSONDocumentAt(data []byte, at, depth int) (*bsonDocument, int, error) {
	if depth > maxBSONDepth || at < 0 || at+4 > len(data) {
		return nil, 0, fmt.Errorf("%w: BSON nesting or length limit exceeded", errPlayniteCorrupt)
	}
	length := int64(int32(binary.LittleEndian.Uint32(data[at:])))
	if length < 5 || length > maxBSONBytes || length > int64(len(data)-at) {
		return nil, 0, fmt.Errorf("%w: invalid nested BSON length", errPlayniteCorrupt)
	}
	end := at + int(length)
	if data[end-1] != 0 {
		return nil, 0, fmt.Errorf("%w: BSON document is not terminated", errPlayniteCorrupt)
	}
	doc := &bsonDocument{fields: make([]bsonField, 0, 8)}
	position := at + 4
	for position < end-1 {
		if len(doc.fields) >= maxBSONElements {
			return nil, 0, fmt.Errorf("%w: too many BSON elements", errPlayniteCorrupt)
		}
		kind := data[position]
		position++
		nameEnd := position
		for nameEnd < end-1 && data[nameEnd] != 0 {
			nameEnd++
		}
		if nameEnd >= end-1 {
			return nil, 0, fmt.Errorf("%w: unterminated BSON field name", errPlayniteCorrupt)
		}
		name := string(data[position:nameEnd])
		position = nameEnd + 1
		value, next, err := parseBSONValue(bsonKind(kind), data, position, end, depth+1)
		if err != nil {
			return nil, 0, err
		}
		doc.fields = append(doc.fields, bsonField{name: name, value: value})
		position = next
	}
	if position != end-1 {
		return nil, 0, fmt.Errorf("%w: BSON element overrun", errPlayniteCorrupt)
	}
	return doc, end, nil
}

func parseBSONValue(kind bsonKind, data []byte, at, end, depth int) (bsonValue, int, error) {
	value := bsonValue{kind: kind}
	if at < 0 || at > end {
		return value, 0, fmt.Errorf("%w: BSON value outside document", errPlayniteCorrupt)
	}
	need := func(n int) bool { return n >= 0 && at <= end-n }
	switch kind {
	case bsonString:
		if !need(4) {
			return value, 0, fmt.Errorf("%w: truncated BSON string", errPlayniteCorrupt)
		}
		n := int64(int32(binary.LittleEndian.Uint32(data[at:])))
		if n <= 0 || n > maxBSONString || n > int64(end-at-4) {
			return value, 0, fmt.Errorf("%w: invalid BSON string length", errPlayniteCorrupt)
		}
		start := at + 4
		last := start + int(n) - 1
		if data[last] != 0 {
			return value, 0, fmt.Errorf("%w: BSON string is not NUL terminated", errPlayniteCorrupt)
		}
		value.text = string(data[start:last])
		return value, last + 1, nil
	case bsonBinary:
		if !need(5) {
			return value, 0, fmt.Errorf("%w: truncated BSON binary", errPlayniteCorrupt)
		}
		n := int64(int32(binary.LittleEndian.Uint32(data[at:])))
		if n < 0 || n > maxBSONBytes || n > int64(end-at-5) {
			return value, 0, fmt.Errorf("%w: invalid BSON binary length", errPlayniteCorrupt)
		}
		value.subtype = data[at+4]
		value.bytes = append([]byte(nil), data[at+5:at+5+int(n)]...)
		return value, at + 5 + int(n), nil
	case bsonBool:
		if !need(1) || (data[at] != 0 && data[at] != 1) {
			return value, 0, fmt.Errorf("%w: invalid BSON boolean", errPlayniteCorrupt)
		}
		value.boolean = data[at] == 1
		return value, at + 1, nil
	case bsonDateTime:
		if !need(8) {
			return value, 0, fmt.Errorf("%w: truncated BSON datetime", errPlayniteCorrupt)
		}
		millis := int64(binary.LittleEndian.Uint64(data[at:]))
		value.date = time.Unix(0, millis*int64(time.Millisecond)).UTC()
		return value, at + 8, nil
	case bsonInt32:
		if !need(4) {
			return value, 0, fmt.Errorf("%w: truncated BSON int32", errPlayniteCorrupt)
		}
		value.number = int64(int32(binary.LittleEndian.Uint32(data[at:])))
		return value, at + 4, nil
	case bsonInt64:
		if !need(8) {
			return value, 0, fmt.Errorf("%w: truncated BSON int64", errPlayniteCorrupt)
		}
		value.number = int64(binary.LittleEndian.Uint64(data[at:]))
		return value, at + 8, nil
	case bsonDocumentKind, bsonArrayKind:
		nested, next, err := parseBSONDocumentAt(data, at, depth)
		if err != nil {
			return value, 0, err
		}
		if kind == bsonDocumentKind {
			value.kind = bsonDocumentKind
		} else {
			value.kind = bsonArrayKind
		}
		value.doc = nested
		return value, next, nil
	case 0x01: // double
		if !need(8) {
			return value, 0, fmt.Errorf("%w: truncated BSON double", errPlayniteCorrupt)
		}
		_ = math.Float64frombits(binary.LittleEndian.Uint64(data[at:]))
		return value, at + 8, nil
	case 0x06: // undefined
		value.kind = bsonOther
		return value, at, nil
	case bsonNull:
		return value, at, nil
	case 0x07: // ObjectId
		if !need(12) {
			return value, 0, fmt.Errorf("%w: truncated BSON ObjectId", errPlayniteCorrupt)
		}
		value.kind = bsonOther
		return value, at + 12, nil
	case 0x0b: // regular expression
		next, ok := readCString(data, at, end)
		if !ok {
			return value, 0, fmt.Errorf("%w: invalid BSON regular expression", errPlayniteCorrupt)
		}
		next, ok = readCString(data, next, end)
		if !ok {
			return value, 0, fmt.Errorf("%w: invalid BSON regular expression options", errPlayniteCorrupt)
		}
		value.kind = bsonOther
		return value, next, nil
	case 0x0c: // DBPointer: string plus ObjectId
		if !need(4) {
			return value, 0, fmt.Errorf("%w: truncated BSON DBPointer", errPlayniteCorrupt)
		}
		n := int64(int32(binary.LittleEndian.Uint32(data[at:])))
		if n <= 0 || n > maxBSONString || n > int64(end-at-4) || !need(4+int(n)+12) {
			return value, 0, fmt.Errorf("%w: invalid BSON DBPointer", errPlayniteCorrupt)
		}
		if data[at+4+int(n)-1] != 0 {
			return value, 0, fmt.Errorf("%w: invalid BSON DBPointer string", errPlayniteCorrupt)
		}
		value.kind = bsonOther
		return value, at + 4 + int(n) + 12, nil
	case 0x0d, 0x0e: // JavaScript and symbol
		if !need(4) {
			return value, 0, fmt.Errorf("%w: truncated BSON code", errPlayniteCorrupt)
		}
		n := int64(int32(binary.LittleEndian.Uint32(data[at:])))
		if n <= 0 || n > maxBSONString || n > int64(end-at-4) || data[at+4+int(n)-1] != 0 {
			return value, 0, fmt.Errorf("%w: invalid BSON code", errPlayniteCorrupt)
		}
		value.kind = bsonOther
		return value, at + 4 + int(n), nil
	case 0x0f: // JavaScript with scope
		if !need(4) {
			return value, 0, fmt.Errorf("%w: truncated BSON code-with-scope", errPlayniteCorrupt)
		}
		n := int64(int32(binary.LittleEndian.Uint32(data[at:])))
		if n < 5 || n > maxBSONBytes || n > int64(end-at) {
			return value, 0, fmt.Errorf("%w: invalid BSON code-with-scope length", errPlayniteCorrupt)
		}
		if _, _, err := parseBSONValue(bsonString, data, at+4, at+int(n), depth+1); err != nil {
			return value, 0, err
		}
		if _, _, err := parseBSONValue(bsonDocumentKind, data, at+4+4+int(binary.LittleEndian.Uint32(data[at+4:])), at+int(n), depth+1); err != nil {
			return value, 0, err
		}
		value.kind = bsonOther
		return value, at + int(n), nil
	case 0x11: // BSON timestamp
		if !need(8) {
			return value, 0, fmt.Errorf("%w: truncated BSON timestamp", errPlayniteCorrupt)
		}
		value.kind = bsonOther
		return value, at + 8, nil
	case 0x13: // Decimal
		if !need(16) {
			return value, 0, fmt.Errorf("%w: truncated BSON decimal", errPlayniteCorrupt)
		}
		value.kind = bsonOther
		return value, at + 16, nil
	case 0xff, 0x7f: // MinKey and MaxKey
		value.kind = bsonOther
		return value, at, nil
	default:
		return value, 0, fmt.Errorf("%w: unknown BSON type 0x%02x", errPlayniteCorrupt, byte(kind))
	}
}

func readCString(data []byte, at, end int) (int, bool) {
	if at < 0 || at >= end {
		return 0, false
	}
	for at < end {
		if data[at] == 0 {
			return at + 1, true
		}
		at++
	}
	return 0, false
}

func (d *bsonDocument) field(name string) (bsonValue, bool) {
	var found bsonValue
	ok := false
	for _, field := range d.fields {
		if field.name == name {
			if ok {
				return bsonValue{}, false
			}
			found, ok = field.value, true
		}
	}
	return found, ok
}

func (d *bsonDocument) hasDuplicate(name string) bool {
	count := 0
	for _, field := range d.fields {
		if field.name == name {
			count++
		}
	}
	return count > 1
}

func (v bsonValue) stringValue() (string, bool) {
	if v.kind != bsonString {
		return "", false
	}
	return strings.TrimSpace(v.text), true
}

func guidValue(v bsonValue) (string, bool) {
	if v.kind != bsonBinary || v.subtype != 0x04 || len(v.bytes) != 16 {
		return "", false
	}
	// Playnite/LiteDB stores Guid as BSON binary subtype 0x04 using the
	// .NET byte order (the subtype byte is consumed by the BSON decoder).
	b := append([]byte(nil), v.bytes...)
	for i, j := 0, 3; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	for i, j := 4, 5; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	for i, j := 6, 7; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return strings.ToLower(string(out[:])), true
}
