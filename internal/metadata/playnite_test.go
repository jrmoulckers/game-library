package metadata

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayniteBSONUsesTopLevelFieldsOnly(t *testing.T) {
	id := testGUIDBytes(t, "12345678-1234-1234-9234-1234567890ab")
	nested := bsonDocumentBytes(bsonElement{typ: 0x02, name: "Name", value: bsonStringBytes("wrong")})
	doc := bsonDocumentBytes(
		bsonElement{typ: 0x05, name: "_id", value: bsonBinaryBytes(0x04, id)},
		bsonElement{typ: 0x03, name: "Metadata", value: nested},
		bsonElement{typ: 0x02, name: "Name", value: bsonStringBytes("Correct")},
	)
	parsed, _, err := parseBSONDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	game, err := gameFromDocument(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if game.Name != "Correct" || game.PlayniteGUID != "12345678-1234-1234-9234-1234567890ab" {
		t.Fatalf("game = %+v", game)
	}
}

func TestReadPlayniteSyntheticV7Fixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	writeSyntheticPlayniteFixture(
		t, path,
		"12345678-1234-1234-9234-1234567890ab",
		"Synthetic game", "synthetic-store-id", SteamPluginID,
	)
	result := ReadPlaynite(path)
	if result.Status != playniteStatusOK || len(result.Games) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Games[0].Name != "Synthetic game" || result.Games[0].PluginID != SteamPluginID {
		t.Fatalf("game = %+v", result.Games[0])
	}
}

func writeSyntheticPlayniteFixture(t *testing.T, filePath, guid, name, gameID, pluginID string) {
	t.Helper()
	data := make([]byte, playnitePageSize*4)
	setPageHeader(data, 0, playniteHeaderPageType, 0)
	copy(data[25:], playniteHeaderSignature)
	data[52] = playniteVersion
	binary.LittleEndian.PutUint16(data[53:], 9)
	binary.LittleEndian.PutUint32(data[55:], ^uint32(0))
	binary.LittleEndian.PutUint32(data[59:], 3)
	data[101] = 1
	binary.LittleEndian.PutUint32(data[102:], 5)
	copy(data[106:], "games")
	binary.LittleEndian.PutUint32(data[111:], 1)

	collection := data[playnitePageSize:]
	setPageHeader(collection, 1, playniteCollectionPageType, 1)
	position := 25
	position = putLiteString32(collection, position, "games")
	binary.LittleEndian.PutUint64(collection[position:], 1)
	position += 8
	binary.LittleEndian.PutUint32(collection[position:], ^uint32(0))
	position += 4
	for slot := 0; slot < 16; slot++ {
		field := ""
		if slot == 0 {
			field = "_id"
		}
		position = putLiteString32(collection, position, field)
		if slot == 0 {
			collection[position] = 1
		}
		position++
		head, tail := emptyPageAddress, emptyPageAddress
		if slot == 0 {
			head = pageAddress{page: 2, slot: 0}
			tail = pageAddress{page: 2, slot: 2}
		}
		putPageAddress(collection[position:], head)
		position += 6
		putPageAddress(collection[position:], tail)
		position += 6
		binary.LittleEndian.PutUint32(collection[position:], ^uint32(0))
		position += 4
	}

	index := data[2*playnitePageSize:]
	setPageHeader(index, 2, playniteIndexPageType, 3)
	id := testGUIDBytes(t, guid)
	position = 25
	position = putIndexNode(index, position, 0, 0, nil, emptyPageAddress, pageAddress{page: 2, slot: 1}, emptyPageAddress)
	position = putIndexNode(index, position, 1, 11, id, pageAddress{page: 2, slot: 0}, pageAddress{page: 2, slot: 2}, pageAddress{page: 3, slot: 0})
	putIndexNode(index, position, 2, 14, nil, pageAddress{page: 2, slot: 1}, emptyPageAddress, emptyPageAddress)

	key := bsonBinaryBytes(0x04, id)
	gameDoc := bsonDocumentBytes(
		bsonElement{typ: 0x05, name: "_id", value: key},
		bsonElement{typ: 0x02, name: "Name", value: bsonStringBytes(name)},
		bsonElement{typ: 0x02, name: "GameId", value: bsonStringBytes(gameID)},
		bsonElement{typ: 0x05, name: "PluginId", value: bsonBinaryBytes(0x04, testGUIDBytes(t, pluginID))},
		bsonElement{typ: 0x08, name: "Hidden", value: []byte{1}},
	)
	datapage := data[3*playnitePageSize:]
	setPageHeader(datapage, 3, playniteDataPageType, 1)
	position = 25
	binary.LittleEndian.PutUint16(datapage[position:], 0)
	position += 2
	if len(gameDoc) <= playnitePageSize-position-6 {
		binary.LittleEndian.PutUint32(datapage[position:], ^uint32(0))
		position += 4
		binary.LittleEndian.PutUint16(datapage[position:], uint16(len(gameDoc)))
		position += 2
		copy(datapage[position:], gameDoc)
	} else {
		extendedPages := (len(gameDoc) + (playnitePageSize - 26)) / (playnitePageSize - 25)
		grown := make([]byte, playnitePageSize*(4+extendedPages))
		copy(grown, data)
		data = grown
		binary.LittleEndian.PutUint32(data[59:], uint32(3+extendedPages))
		datapage = data[3*playnitePageSize:]
		position = 25
		binary.LittleEndian.PutUint16(datapage[position:], 0)
		position += 2
		binary.LittleEndian.PutUint32(datapage[position:], 4)
		position += 4
		binary.LittleEndian.PutUint16(datapage[position:], 0)
		offset := 0
		for pageIndex := 0; pageIndex < extendedPages; pageIndex++ {
			pageID := uint32(4 + pageIndex)
			page := data[int(pageID)*playnitePageSize:]
			remaining := len(gameDoc) - offset
			length := playnitePageSize - 25
			if remaining < length {
				length = remaining
			}
			setPageHeader(page, pageID, playniteExtendPageType, uint16(length))
			next := ^uint32(0)
			if pageIndex+1 < extendedPages {
				next = pageID + 1
			}
			binary.LittleEndian.PutUint32(page[9:], next)
			copy(page[25:], gameDoc[offset:offset+length])
			offset += length
		}
	}
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		t.Fatal(err)
	}

}

func TestReadPlaynitePageSpanningDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	name := strings.Repeat("S", 5000)
	writeSyntheticPlayniteFixture(
		t, path,
		"12345678-1234-1234-9234-1234567890ab",
		name, "42", SteamPluginID,
	)
	result := ReadPlaynite(path)
	if result.Status != playniteStatusOK || len(result.Games) != 1 || result.Games[0].Name != name {
		t.Fatalf("page-spanning result = status %q, games %d", result.Status, len(result.Games))
	}
}

func setPageHeader(page []byte, id uint32, pageType byte, itemCount uint16) {
	binary.LittleEndian.PutUint32(page, id)
	page[4] = pageType
	binary.LittleEndian.PutUint16(page[13:], itemCount)
}

func putLiteString32(target []byte, position int, value string) int {
	binary.LittleEndian.PutUint32(target[position:], uint32(len(value)))
	copy(target[position+4:], value)
	return position + 4 + len(value)
}

func putIndexNode(target []byte, position int, index uint16, keyType byte, key []byte, previous, next, data pageAddress) int {
	binary.LittleEndian.PutUint16(target[position:], index)
	position += 2
	target[position] = 1
	position++
	target[position] = 0
	position++
	putPageAddress(target[position:], emptyPageAddress)
	position += 6
	putPageAddress(target[position:], emptyPageAddress)
	position += 6
	binary.LittleEndian.PutUint16(target[position:], uint16(len(key)))
	position += 2
	target[position] = keyType
	position++
	copy(target[position:], key)
	position += len(key)
	putPageAddress(target[position:], data)
	position += 6
	putPageAddress(target[position:], previous)
	position += 6
	putPageAddress(target[position:], next)
	return position + 6
}

func TestPlayniteBSONRejectsNestedLengthAndUnknownType(t *testing.T) {
	doc := bsonDocumentBytes(bsonElement{typ: 0x02, name: "Name", value: bsonStringBytes("x")})
	binary.LittleEndian.PutUint32(doc, uint32(len(doc)+100))
	if _, _, err := parseBSONDocument(doc); err == nil {
		t.Fatal("truncated outer BSON length was accepted")
	}
	doc = bsonDocumentBytes(bsonElement{typ: 0x42, name: "x", value: nil})
	if _, _, err := parseBSONDocument(doc); err == nil {
		t.Fatal("unknown BSON type was accepted")
	}
	offByOne := bsonDocumentBytes(bsonElement{typ: 0x02, name: "Name", value: bsonStringBytes("synthetic")})
	stringLengthOffset := 4 + 1 + len("Name") + 1
	binary.LittleEndian.PutUint32(offByOne[stringLengthOffset:], uint32(len("synthetic")))
	if _, _, err := parseBSONDocument(offByOne); err == nil {
		t.Fatal("BSON string length excluding the trailing NUL was accepted")
	}
}

type bsonElement struct {
	typ   byte
	name  string
	value []byte
}

func bsonDocumentBytes(elements ...bsonElement) []byte {
	length := 5
	for _, element := range elements {
		length += 1 + len(element.name) + 1 + len(element.value)
	}
	out := make([]byte, length)
	binary.LittleEndian.PutUint32(out, uint32(length))
	position := 4
	for _, element := range elements {
		out[position] = element.typ
		position++
		copy(out[position:], element.name)
		position += len(element.name) + 1
		copy(out[position:], element.value)
		position += len(element.value)
	}
	out[position] = 0
	return out
}

func bsonStringBytes(value string) []byte {
	out := make([]byte, 4+len(value)+1)
	binary.LittleEndian.PutUint32(out, uint32(len(value)+1))
	copy(out[4:], value)
	return out
}

func bsonBinaryBytes(subtype byte, value []byte) []byte {
	out := make([]byte, 5+len(value))
	binary.LittleEndian.PutUint32(out, uint32(len(value)))
	out[4] = subtype
	copy(out[5:], value)
	return out
}

func testGUIDBytes(t *testing.T, value string) []byte {
	t.Helper()
	var out []byte
	for _, part := range []string{value[0:8], value[9:13], value[14:18], value[19:23], value[24:]} {
		b, err := hex.DecodeString(part)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b...)
	}
	for i, j := 0, 3; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	for i, j := 4, 5; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	for i, j := 6, 7; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func putLiteString(page []byte, position int, value string) int {
	binary.LittleEndian.PutUint32(page[position:], uint32(len(value)+1))
	copy(page[position+4:], value)
	page[position+4+len(value)] = 0
	return position + 4 + len(value) + 1
}

func putPageAddress(page []byte, address pageAddress) {
	binary.LittleEndian.PutUint32(page, address.page)
	binary.LittleEndian.PutUint16(page[4:], address.slot)
}
