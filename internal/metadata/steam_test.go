package metadata

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestResolveSteamFormatsPrecedenceAndDeterminism(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	library := t.TempDir()
	accountA := t.TempDir()
	accountB := t.TempDir()

	writeTestFile(t, filepath.Join(rootA, "appcache", "appinfo.vdf"),
		testAppInfoV29(map[uint32]string{7: "indexed title"}))
	writeTestFile(t, filepath.Join(rootB, "appcache", "appinfo.vdf"),
		testAppInfoV28(map[uint32]string{8: "inline title"}))
	writeTestFile(t, filepath.Join(rootA, "steamapps", "libraryfolders.vdf"),
		"\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\" "+strconv.Quote(library)+"\n\t}\n}\n")
	writeTestFile(t, filepath.Join(rootA, "steamapps", "appmanifest_7.acf"),
		testManifest(7, "manifest loses"))
	writeTestFile(t, filepath.Join(rootA, "steamapps", "appmanifest_9.acf"),
		testManifest(9, "manifest title"))
	writeTestFile(t, filepath.Join(rootB, "steamapps", "appmanifest_8.acf"),
		testManifest(8, "manifest loses"))
	writeTestFile(t, filepath.Join(library, "steamapps", "appmanifest_10.acf"),
		testManifest(10, "library title"))
	writeTestFile(t, filepath.Join(library, "steamapps", "appmanifest_bad.acf"),
		"not a manifest")

	writeTestFile(t, filepath.Join(accountA, "shortcuts.vdf"),
		testShortcuts([]testShortcut{{id: 7, name: "shortcut loses"}, {id: ^uint32(0), name: "large id"}}))
	writeTestFile(t, filepath.Join(accountB, "shortcuts.vdf"),
		testShortcuts([]testShortcut{{id: 11, name: "second account"}}))

	locations := SteamLocations{
		InstallRoots:      []string{rootB, rootA, rootA},
		AccountConfigDirs: []string{accountB, accountA, accountA},
	}
	first := ResolveSteam(locations)
	second := ResolveSteam(locations)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("resolution is not deterministic:\n%#v\n%#v", first, second)
	}
	want := map[uint32]TitleRecord{
		7:          {Title: "indexed title", Source: "steam-appinfo"},
		8:          {Title: "inline title", Source: "steam-appinfo"},
		9:          {Title: "manifest title", Source: "steam-manifest"},
		10:         {Title: "library title", Source: "steam-manifest"},
		11:         {Title: "second account", Source: "steam-shortcut"},
		^uint32(0): {Title: "large id", Source: "steam-shortcut"},
	}
	if !reflect.DeepEqual(first.Titles, want) {
		t.Fatalf("titles = %#v, want %#v", first.Titles, want)
	}
	if len(first.Diagnostics) != 1 || first.Diagnostics[0].Source != "steam-manifest" {
		t.Fatalf("diagnostics = %#v, want one malformed manifest diagnostic", first.Diagnostics)
	}
}

func TestSteamAppInfoRejectsUnknownTagsAndTruncation(t *testing.T) {
	valid := testAppInfoV29(map[uint32]string{31: "safe title"})
	if _, err := parseSteamAppInfo(append(append([]byte(nil), valid...), 0x99)); err == nil {
		t.Fatal("trailing bytes should reject the appinfo source")
	}
	// appinfo.vdf is a live cache that legitimately contains records this
	// reader does not model. Such a record is skipped rather than guessed at,
	// and it must never discard the rest of the file.
	unknown := testAppInfoV28WithKV(31, []byte{0x03, 0, 0, 0, 0})
	titles, err := parseSteamAppInfo(unknown)
	if err != nil {
		t.Fatalf("an unmodelled record should be skipped, not fatal: %v", err)
	}
	if len(titles) != 0 {
		t.Fatalf("titles = %#v, an unparsed record must not be guessed at", titles)
	}
	truncated := testAppInfoV28(map[uint32]string{31: "safe title"})
	truncated = truncated[:len(truncated)-1]
	if _, err := parseSteamAppInfo(truncated); err == nil {
		t.Fatal("truncated binary source should reject")
	}
	newer := make([]byte, 16)
	binary.LittleEndian.PutUint32(newer, 0x07564430)
	if _, err := parseSteamAppInfo(newer); err == nil {
		t.Fatal("newer appinfo magic should degrade instead of guessing")
	}
	badIndex := testAppInfoV29(map[uint32]string{31: "synthetic title"})
	binary.LittleEndian.PutUint32(badIndex[90:], 99)
	corrupt, err := parseSteamAppInfo(badIndex)
	if err != nil {
		t.Fatalf("a corrupt record should be skipped, not fatal: %v", err)
	}
	if len(corrupt) != 0 {
		t.Fatalf("titles = %#v, an out-of-range string index must not resolve a title", corrupt)
	}
}

// A single unreadable record must not cost the whole library its titles.
// appinfo.vdf is a live cache, so an unmodelled record is expected eventually.
func TestSteamAppInfoSkipsOnlyTheUnreadableRecord(t *testing.T) {
	good := indexedCommonNameKV("resolved title")
	bad := []byte{0x03, 0, 0, 0, 0, 0x08}
	titles, err := parseSteamAppInfo(testAppInfoV29Raw(map[uint32][]byte{11: bad, 12: good}))
	if err != nil {
		t.Fatal(err)
	}
	if titles[12] != "resolved title" {
		t.Fatalf("titles = %#v, a neighbouring bad record discarded a good one", titles)
	}
	if _, ok := titles[11]; ok {
		t.Fatalf("titles = %#v, the unreadable record must not be invented", titles)
	}
}

func TestSteamLibraryPathTraversalIsIgnored(t *testing.T) {
	data := []byte("\"libraryfolders\" { \"0\" { \"path\" \"../outside\" } }")
	paths, err := parseSteamLibraryFolders(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("paths = %#v, traversal path was accepted", paths)
	}
}

func TestSteamShortcutsAreStructuralAndCaseInsensitive(t *testing.T) {
	data := testShortcuts([]testShortcut{{id: ^uint32(0), name: "unsigned title"}})
	titles, err := parseSteamShortcuts(data)
	if err != nil {
		t.Fatal(err)
	}

	if titles[^uint32(0)] != "unsigned title" {
		t.Fatalf("titles = %#v", titles)
	}
	// A byte sequence resembling an appid outside a KV value is not a record.
	invalid := append([]byte{0x55, 0xff, 0xff, 0xff, 0xff}, data...)
	if _, err := parseSteamShortcuts(invalid); err == nil {
		t.Fatal("unstructured prefix should reject rather than be scanned")
	}
}

func TestSteamAppInfoZeroTitlesProducesDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "appcache", "appinfo.vdf"), testAppInfoV29(map[uint32]string{}))
	result := ResolveSteam(SteamLocations{InstallRoots: []string{root}})
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Message, "contained no readable names") {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

type testShortcut struct {
	id   uint32
	name string
}

func testAppInfoV29(titles map[uint32]string) []byte {
	table := []string{"common", "name"}
	var records bytes.Buffer
	ids := sortedTestIDs(titles)
	for _, id := range ids {
		kv := indexedCommonNameKV(titles[id])
		payload := append(make([]byte, 60), kv...)
		_ = binary.Write(&records, binary.LittleEndian, id)
		_ = binary.Write(&records, binary.LittleEndian, uint32(len(payload)))
		records.Write(payload)
	}
	_ = binary.Write(&records, binary.LittleEndian, uint32(0))
	tableBytes := []byte(strings.Join(table, "\x00") + "\x00")
	var tableSection bytes.Buffer
	_ = binary.Write(&tableSection, binary.LittleEndian, uint32(len(table)))
	tableSection.Write(tableBytes)
	offset := uint64(16 + records.Len())
	out := make([]byte, 16)
	binary.LittleEndian.PutUint32(out, 0x07564429)
	binary.LittleEndian.PutUint32(out[4:], 1)
	binary.LittleEndian.PutUint64(out[8:], offset)
	out = append(out, records.Bytes()...)
	return append(out, tableSection.Bytes()...)
}

// testAppInfoV29Raw builds a v29 file from caller-supplied record bodies so
// tests can reproduce byte shapes observed in real appinfo.vdf files.
func testAppInfoV29Raw(records map[uint32][]byte) []byte {
	table := []string{"common", "name"}
	var body bytes.Buffer
	ids := make([]uint32, 0, len(records))
	for id := range records {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		payload := append(make([]byte, 60), records[id]...)
		_ = binary.Write(&body, binary.LittleEndian, id)
		_ = binary.Write(&body, binary.LittleEndian, uint32(len(payload)))
		body.Write(payload)
	}
	_ = binary.Write(&body, binary.LittleEndian, uint32(0))
	var tableSection bytes.Buffer
	_ = binary.Write(&tableSection, binary.LittleEndian, uint32(len(table)))
	tableSection.WriteString(strings.Join(table, "\x00") + "\x00")
	out := make([]byte, 16)
	binary.LittleEndian.PutUint32(out, 0x07564429)
	binary.LittleEndian.PutUint32(out[4:], 1)
	binary.LittleEndian.PutUint64(out[8:], uint64(16+body.Len()))
	out = append(out, body.Bytes()...)
	return append(out, tableSection.Bytes()...)
}

func testAppInfoV28(titles map[uint32]string) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint32(0x07564428))
	_ = binary.Write(&out, binary.LittleEndian, uint32(1))
	for _, id := range sortedTestIDs(titles) {
		kv := inlineCommonNameKV(titles[id])
		payload := append(make([]byte, 60), kv...)
		_ = binary.Write(&out, binary.LittleEndian, id)
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(payload)))
		out.Write(payload)
	}
	_ = binary.Write(&out, binary.LittleEndian, uint32(0))
	return out.Bytes()
}

func testAppInfoV28WithKV(id uint32, kv []byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint32(0x07564428))
	_ = binary.Write(&out, binary.LittleEndian, uint32(1))
	payload := append(make([]byte, 60), kv...)
	_ = binary.Write(&out, binary.LittleEndian, id)
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(payload)))
	out.Write(payload)
	_ = binary.Write(&out, binary.LittleEndian, uint32(0))
	return out.Bytes()
}

func indexedCommonNameKV(title string) []byte {
	var out bytes.Buffer
	out.WriteByte(0x00)
	_ = binary.Write(&out, binary.LittleEndian, uint32(0))
	out.WriteByte(0x01)
	_ = binary.Write(&out, binary.LittleEndian, uint32(1))
	out.WriteString(title)
	out.WriteByte(0)
	out.WriteByte(0x08)
	out.WriteByte(0x08)
	return out.Bytes()
}

func inlineCommonNameKV(title string) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x00})
	out.WriteString("common\x00")
	out.WriteByte(0x01)
	out.WriteString("name\x00")
	out.WriteString(title)
	out.WriteByte(0)
	out.WriteByte(0x08)
	out.WriteByte(0x08)
	return out.Bytes()
}

func testShortcuts(shortcuts []testShortcut) []byte {
	var out bytes.Buffer
	out.WriteByte(0x00)
	out.WriteString("shortcuts\x00")
	for index, shortcut := range shortcuts {
		out.WriteByte(0x00)
		out.WriteString(strconv.Itoa(index))
		out.WriteByte(0)
		out.WriteByte(0x02)
		out.WriteString("ApPId\x00")
		var id [4]byte
		binary.LittleEndian.PutUint32(id[:], shortcut.id)
		out.Write(id[:])
		out.WriteByte(0x01)
		out.WriteString("APPname\x00")
		out.WriteString(shortcut.name)
		out.WriteByte(0)
		out.WriteByte(0x08)
	}
	out.WriteByte(0x08)
	out.WriteByte(0x08)
	return out.Bytes()
}

func testManifest(id uint32, title string) string {
	return "\"AppState\" {\n\"appid\" \"" + strconv.FormatUint(uint64(id), 10) +
		"\"\n\"name\" " + strconv.Quote(title) + "\n}\n"
}

func sortedTestIDs(values map[uint32]string) []uint32 {
	ids := make([]uint32, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	return ids
}

func writeTestFile(t *testing.T, path string, data interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var content []byte
	switch value := data.(type) {
	case string:
		content = []byte(value)
	case []byte:
		content = value
	default:
		t.Fatalf("unsupported fixture type %T", data)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
