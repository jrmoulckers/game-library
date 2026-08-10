package metadata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SteamLocations identifies the Steam locations which should be read. The
// resolver never infers a user's home directory and never writes to any of
// these locations.
type SteamLocations struct {
	InstallRoots      []string
	AccountConfigDirs []string
}

// SteamResult is the in-memory result of a best-effort Steam scan.
type SteamResult struct {
	Titles      map[uint32]TitleRecord `json:"titles"`
	Diagnostics []Diagnostic           `json:"diagnostics,omitempty"`
}

const (
	steamMaxAppInfoFile  = 256 << 20
	steamMaxTextFile     = 16 << 20
	steamMaxShortcutFile = 32 << 20
	steamMaxRecords      = 500000
	steamMaxStrings      = 1000000
	steamMaxString       = 1 << 20
	steamMaxNodes        = 1000000
	steamMaxDepth        = 64
)

// ResolveSteam reads appinfo, library manifests, and account shortcuts. A
// bad individual source is reported and skipped; it does not prevent other
// sources from being resolved.
func ResolveSteam(locations SteamLocations) SteamResult {
	result := SteamResult{Titles: make(map[uint32]TitleRecord)}
	roots := uniqueSteamPaths(locations.InstallRoots)
	accounts := uniqueSteamPaths(locations.AccountConfigDirs)

	// appinfo is only owned by a Steam installation. Read every supplied
	// installation in stable order so duplicate installations are harmless.
	for _, root := range roots {
		path := filepath.Join(root, "appcache", "appinfo.vdf")
		if data, ok := readSteamOptional(path, steamMaxAppInfoFile, &result, "steam-appinfo"); ok {
			titles, err := parseSteamAppInfo(data)
			if err != nil {
				addSteamDiagnostic(&result, "steam-appinfo", "Steam titles unavailable - appinfo.vdf could not be read safely.")
				continue
			}
			if len(titles) == 0 {
				addSteamDiagnostic(&result, "steam-appinfo", "Steam titles unavailable - appinfo.vdf contained no readable names.")
			}
			addSteamTitles(result.Titles, titles, "steam-appinfo")
		}
	}

	libraryRoots := append([]string(nil), roots...)
	for _, root := range roots {
		path := filepath.Join(root, "steamapps", "libraryfolders.vdf")
		data, ok := readSteamOptional(path, steamMaxTextFile, &result, "steam-libraryfolders")
		if !ok {
			continue
		}
		paths, err := parseSteamLibraryFolders(data)
		if err != nil {
			addSteamDiagnostic(&result, "steam-libraryfolders", "invalid libraryfolders source")
			continue
		}
		libraryRoots = append(libraryRoots, paths...)
	}
	libraryRoots = uniqueSteamPaths(libraryRoots)

	for _, root := range libraryRoots {
		manifestDir := filepath.Join(root, "steamapps")
		entries, err := os.ReadDir(manifestDir)
		if err != nil {
			if !os.IsNotExist(err) {
				addSteamDiagnostic(&result, "steam-manifest", "unable to read manifest directory")
			}
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			lower := strings.ToLower(name)
			if strings.HasPrefix(lower, "appmanifest_") && strings.HasSuffix(lower, ".acf") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			data, ok := readSteamOptional(filepath.Join(manifestDir, name), steamMaxTextFile, &result, "steam-manifest")
			if !ok {
				continue
			}
			appID, title, err := parseSteamManifest(data)
			if err != nil {
				addSteamDiagnostic(&result, "steam-manifest", "invalid appmanifest source")
				continue
			}
			addSteamTitle(result.Titles, appID, title, "steam-manifest")
		}
	}

	for _, account := range accounts {
		candidates := []string{filepath.Join(account, "shortcuts.vdf")}
		// Accepting an account directory as well as its config directory is
		// useful for callers scanning userdata without making recursive scans.
		if !strings.EqualFold(filepath.Base(filepath.Clean(account)), "config") {
			candidates = append(candidates, filepath.Join(account, "config", "shortcuts.vdf"))
		}
		for _, path := range uniqueSteamPaths(candidates) {
			data, ok := readSteamOptional(path, steamMaxShortcutFile, &result, "steam-shortcut")
			if !ok {
				continue
			}
			titles, err := parseSteamShortcuts(data)
			if err != nil {
				addSteamDiagnostic(&result, "steam-shortcut", "Non-Steam shortcut titles unavailable - shortcuts.vdf could not be read safely.")
				continue
			}
			addSteamTitles(result.Titles, titles, "steam-shortcut")
		}
	}

	sort.Slice(result.Diagnostics, func(i, j int) bool {
		if result.Diagnostics[i].Source != result.Diagnostics[j].Source {
			return result.Diagnostics[i].Source < result.Diagnostics[j].Source
		}
		if result.Diagnostics[i].Status != result.Diagnostics[j].Status {
			return result.Diagnostics[i].Status < result.Diagnostics[j].Status
		}
		return result.Diagnostics[i].Message < result.Diagnostics[j].Message
	})
	return result
}

func addSteamTitle(titles map[uint32]TitleRecord, appID uint32, title, source string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	if _, exists := titles[appID]; !exists {
		titles[appID] = TitleRecord{Title: title, Source: source}
	}
}

func addSteamTitles(dst map[uint32]TitleRecord, src map[uint32]string, source string) {
	ids := make([]uint32, 0, len(src))
	for id := range src {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		addSteamTitle(dst, id, src[id], source)
	}
}

func addSteamDiagnostic(result *SteamResult, source, message string) {
	result.Diagnostics = append(result.Diagnostics, Diagnostic{
		Source: source, Status: "warning", Message: message,
	})
}

func readSteamOptional(path string, max int, result *SteamResult, source string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			addSteamDiagnostic(result, source, "unable to inspect source")
		}
		return nil, false
	}
	if !info.Mode().IsRegular() || info.Size() > int64(max) {
		addSteamDiagnostic(result, source, "source exceeds limits")
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		addSteamDiagnostic(result, source, "unable to read source")
		return nil, false
	}
	return data, true
}

func uniqueSteamPaths(paths []string) []string {
	type namedPath struct {
		key, path string
	}
	values := make([]namedPath, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		clean, err := filepath.Abs(filepath.Clean(value))
		if err != nil {
			continue
		}
		key := clean
		if filepath.Separator == '\\' {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, namedPath{key: key, path: clean})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].key != values[j].key {
			return values[i].key < values[j].key
		}
		return values[i].path < values[j].path
	})
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].path
	}
	return out
}

// --- appinfo.vdf ---------------------------------------------------------

type steamBinaryReader struct {
	data  []byte
	pos   int
	nodes int
}

func (r *steamBinaryReader) remaining() int { return len(r.data) - r.pos }

func (r *steamBinaryReader) bytes(n int) ([]byte, error) {
	if n < 0 || n > r.remaining() {
		return nil, errSteamFormat
	}
	value := r.data[r.pos : r.pos+n]
	r.pos += n
	return value, nil
}

func (r *steamBinaryReader) u32() (uint32, error) {
	value, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(value), nil
}

func (r *steamBinaryReader) u64() (uint64, error) {
	value, err := r.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(value), nil
}

func (r *steamBinaryReader) cString() (string, error) {
	remaining := r.data[r.pos:]
	end := bytes.IndexByte(remaining, 0)
	if end < 0 || end > steamMaxString {
		return "", errSteamFormat
	}
	value := remaining[:end]
	r.pos += end + 1
	if !utf8.Valid(value) {
		return "", errSteamFormat
	}
	return string(value), nil
}

type steamKV struct {
	key      string
	kind     byte
	text     string
	unsigned uint64
	children []steamKV
}

var errSteamFormat = errors.New("invalid steam source")

func parseIndexedKV(data []byte, stringsTable []string) (steamKV, error) {
	reader := &steamBinaryReader{data: data}
	children, err := parseIndexedObject(reader, stringsTable, 0)
	if err != nil || reader.pos != len(data) {
		return steamKV{}, errSteamFormat
	}
	return steamKV{kind: 0x00, children: children}, nil
}

func indexedString(table []string, index uint32) (string, error) {
	if uint64(index) >= uint64(len(table)) {
		return "", errSteamFormat
	}
	return table[index], nil
}

func parseIndexedValue(r *steamBinaryReader, table []string, depth int) (steamKV, error) {
	if depth > steamMaxDepth {
		return steamKV{}, errSteamFormat
	}
	r.nodes++
	if r.nodes > steamMaxNodes {
		return steamKV{}, errSteamFormat
	}
	tag, err := r.bytes(1)
	if err != nil || tag[0] == 0x08 {
		return steamKV{}, errSteamFormat
	}
	keyIndex, err := r.u32()
	if err != nil {
		return steamKV{}, errSteamFormat
	}
	key, err := indexedString(table, keyIndex)
	if err != nil {
		return steamKV{}, err
	}
	node := steamKV{key: key, kind: tag[0]}
	switch tag[0] {
	case 0x00:
		node.children, err = parseIndexedObject(r, table, depth+1)
	case 0x01:
		node.text, err = r.cString()
	case 0x02:
		_, err = r.u32()
	case 0x07, 0x0b:
		node.unsigned, err = r.u64()
	default:
		err = errSteamFormat
	}
	if err != nil {
		return steamKV{}, errSteamFormat
	}
	return node, nil
}

func parseIndexedObject(r *steamBinaryReader, table []string, depth int) ([]steamKV, error) {
	if depth > steamMaxDepth {
		return nil, errSteamFormat
	}
	children := make([]steamKV, 0, 4)
	for {
		if r.remaining() < 1 {
			return nil, errSteamFormat
		}
		if r.data[r.pos] == 0x08 {
			r.pos++
			return children, nil
		}
		child, err := parseIndexedValue(r, table, depth)
		if err != nil {
			return nil, errSteamFormat
		}
		children = append(children, child)
	}
}

func parseSteamAppInfo(data []byte) (map[uint32]string, error) {
	if len(data) < 8 {
		return nil, errSteamFormat
	}
	magic := binary.LittleEndian.Uint32(data)
	switch magic {
	case 0x07564429:
		return parseSteamAppInfoV29(data)
	case 0x07564428:
		return parseSteamAppInfoV28(data)
	default:
		return nil, errSteamFormat
	}
}

func parseSteamAppInfoV29(data []byte) (map[uint32]string, error) {
	if len(data) < 16 {
		return nil, errSteamFormat
	}
	offset := int64(binary.LittleEndian.Uint64(data[8:16]))
	if offset < 16 || offset > int64(len(data)) {
		return nil, errSteamFormat
	}
	tableReader := &steamBinaryReader{data: data[offset:]}
	count, err := tableReader.u32()
	if err != nil || count > steamMaxStrings {
		return nil, errSteamFormat
	}
	table := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		value, err := tableReader.cString()
		if err != nil {
			return nil, errSteamFormat
		}
		table = append(table, value)
	}
	if tableReader.pos != len(tableReader.data) {
		// The table is the final section. Bytes here would make record and
		// string-table boundaries ambiguous.
		return nil, errSteamFormat
	}
	return parseSteamAppRecords(data[16:offset], table, true)
}

func parseSteamAppInfoV28(data []byte) (map[uint32]string, error) {
	// v28 has only magic and universe before its app records.
	return parseSteamAppRecords(data[8:], nil, false)
}

func parseSteamAppRecords(data []byte, table []string, indexed bool) (map[uint32]string, error) {
	reader := &steamBinaryReader{data: data}
	titles := make(map[uint32]string)
	recordCount := 0
	foundSentinel := false
	for reader.pos < len(data) {
		if recordCount >= steamMaxRecords || reader.remaining() < 4 {
			return nil, errSteamFormat
		}
		appID, err := reader.u32()
		if err != nil {
			return nil, errSteamFormat
		}
		if appID == 0 {
			if reader.remaining() != 0 {
				return nil, errSteamFormat
			}
			foundSentinel = true
			break
		}
		if reader.remaining() < 4 {
			return nil, errSteamFormat
		}
		size, err := reader.u32()
		if err != nil || size > uint32(steamMaxAppInfoFile) {
			return nil, errSteamFormat
		}
		// Steam writers have used both interpretations of size: total app
		// payload and KV payload. Prefer the documented total-payload form,
		// but accept the latter without relaxing any bounds or KV checks.
		lengths := make([]int, 0, 2)
		if size >= 60 {
			lengths = append(lengths, int(size))
		}
		if uint64(size)+60 <= uint64(reader.remaining()) {
			lengths = append(lengths, int(uint64(size)+60))
		}
		parsed := false
		for _, length := range lengths {
			if length > reader.remaining() || length < 60 {
				continue
			}
			payload := data[reader.pos : reader.pos+length]
			kvData := payload[60:]
			var root steamKV
			if indexed {
				root, err = parseIndexedKV(kvData, table)
			} else {
				root, err = parseInlineKV(kvData)
			}
			if err != nil || root.kind != 0x00 {
				continue
			}
			reader.pos += length
			if title, ok := steamCommonName(root); ok {
				titles[appID] = title
			}
			parsed = true
			break
		}
		if !parsed {
			return nil, errSteamFormat
		}
		recordCount++
	}
	if !foundSentinel {
		return nil, errSteamFormat
	}
	return titles, nil
}

func steamCommonName(root steamKV) (string, bool) {
	for _, child := range root.children {
		if child.kind != 0x00 {
			continue
		}
		if strings.EqualFold(child.key, "common") {
			for _, value := range child.children {
				if value.kind == 0x01 && strings.EqualFold(value.key, "name") {
					title := strings.TrimSpace(value.text)
					if title != "" {
						return title, true
					}
				}
			}
		}
		if title, ok := steamCommonName(child); ok {
			return title, true
		}
	}
	return "", false
}

// --- shortcuts.vdf -------------------------------------------------------

func parseInlineKV(data []byte) (steamKV, error) {
	reader := &steamBinaryReader{data: data}
	children, err := parseInlineObject(reader, 0)
	if err != nil || reader.pos != len(data) {
		return steamKV{}, errSteamFormat
	}
	return steamKV{kind: 0x00, children: children}, nil
}

func parseInlineValue(r *steamBinaryReader, depth int) (steamKV, error) {
	if depth > steamMaxDepth {
		return steamKV{}, errSteamFormat
	}
	r.nodes++
	if r.nodes > steamMaxNodes {
		return steamKV{}, errSteamFormat
	}
	tag, err := r.bytes(1)
	if err != nil || tag[0] == 0x08 {
		return steamKV{}, errSteamFormat
	}
	key, err := r.cString()
	if err != nil {
		return steamKV{}, errSteamFormat
	}
	node := steamKV{key: key, kind: tag[0]}
	switch tag[0] {
	case 0x00:
		node.children, err = parseInlineObject(r, depth+1)
	case 0x01:
		node.text, err = r.cString()
	case 0x02:
		value, readErr := r.u32()
		node.unsigned = uint64(value)
		err = readErr
	case 0x07, 0x0b:
		node.unsigned, err = r.u64()
	default:
		err = errSteamFormat
	}
	if err != nil {
		return steamKV{}, errSteamFormat
	}
	return node, nil
}

func parseInlineObject(r *steamBinaryReader, depth int) ([]steamKV, error) {
	if depth > steamMaxDepth {
		return nil, errSteamFormat
	}
	children := make([]steamKV, 0, 8)
	for {
		if r.remaining() < 1 {
			return nil, errSteamFormat
		}
		if r.data[r.pos] == 0x08 {
			r.pos++
			return children, nil
		}
		child, err := parseInlineValue(r, depth)
		if err != nil {
			return nil, errSteamFormat
		}
		children = append(children, child)
	}
}

func parseSteamShortcuts(data []byte) (map[uint32]string, error) {
	root, err := parseInlineKV(data)
	if err != nil || root.kind != 0x00 {
		return nil, errSteamFormat
	}
	var shortcutEntries []steamKV
	for _, value := range root.children {
		if value.kind == 0x00 && strings.EqualFold(value.key, "shortcuts") {
			shortcutEntries = value.children
			break
		}
	}
	if shortcutEntries == nil {
		return nil, errSteamFormat
	}
	titles := make(map[uint32]string)
	for _, shortcut := range shortcutEntries {
		if shortcut.kind != 0x00 {
			continue
		}
		var appID uint32
		var haveID bool
		var title string
		for _, field := range shortcut.children {
			switch {
			case strings.EqualFold(field.key, "appid") &&
				(field.kind == 0x02 || field.kind == 0x07 || field.kind == 0x0b):
				if field.unsigned > uint64(^uint32(0)) {
					return nil, errSteamFormat
				}
				appID, haveID = uint32(field.unsigned), true
			case strings.EqualFold(field.key, "appname") && field.kind == 0x01:
				title = strings.TrimSpace(field.text)
			}
		}
		if haveID && title != "" {
			if _, exists := titles[appID]; !exists {
				titles[appID] = title
			}
		}
	}
	return titles, nil
}

// --- text VDF ------------------------------------------------------------

type steamVDFNode struct {
	key      string
	value    string
	children []steamVDFNode
}

type steamVDFLexer struct {
	data  []byte
	pos   int
	nodes int
}

func (l *steamVDFLexer) token() (string, byte, error) {
	for l.pos < len(l.data) {
		switch l.data[l.pos] {
		case ' ', '\t', '\r', '\n':
			l.pos++
		case '/':
			if l.pos+1 < len(l.data) && l.data[l.pos+1] == '/' {
				l.pos += 2
				for l.pos < len(l.data) && l.data[l.pos] != '\n' {
					l.pos++
				}
			} else {
				return "", 0, errSteamFormat
			}
		default:
			goto tokenStart
		}
	}
	return "", 'e', nil

tokenStart:
	if l.data[l.pos] == '{' || l.data[l.pos] == '}' {
		value := l.data[l.pos]
		l.pos++
		return "", value, nil
	}
	if l.data[l.pos] == '"' {
		l.pos++
		var out bytes.Buffer
		for l.pos < len(l.data) {
			char := l.data[l.pos]
			l.pos++
			if char == '"' {
				if !utf8.Valid(out.Bytes()) {
					return "", 0, errSteamFormat
				}
				return out.String(), 's', nil
			}
			if char == '\\' {
				if l.pos >= len(l.data) {
					return "", 0, errSteamFormat
				}
				escaped := l.data[l.pos]
				l.pos++
				switch escaped {
				case 'n':
					out.WriteByte('\n')
				case 'r':
					out.WriteByte('\r')
				case 't':
					out.WriteByte('\t')
				default:
					out.WriteByte(escaped)
				}
			} else {
				out.WriteByte(char)
			}
			if out.Len() > steamMaxString {
				return "", 0, errSteamFormat
			}
		}
		return "", 0, errSteamFormat
	}
	start := l.pos
	for l.pos < len(l.data) {
		char := l.data[l.pos]
		if char == ' ' || char == '\t' || char == '\r' || char == '\n' ||
			char == '{' || char == '}' {
			break
		}
		l.pos++
	}
	if start == l.pos {
		return "", 0, errSteamFormat
	}
	value := string(l.data[start:l.pos])
	if len(value) > steamMaxString || !utf8.ValidString(value) {
		return "", 0, errSteamFormat
	}
	return value, 's', nil
}

func parseSteamVDF(data []byte) ([]steamVDFNode, error) {
	lexer := &steamVDFLexer{data: data}
	return parseSteamVDFObject(lexer, false, 0)
}

func parseSteamVDFObject(lexer *steamVDFLexer, braced bool, depth int) ([]steamVDFNode, error) {
	if depth > steamMaxDepth {
		return nil, errSteamFormat
	}
	nodes := make([]steamVDFNode, 0, 8)
	for len(nodes) < steamMaxNodes {
		key, kind, err := lexer.token()
		if err != nil {
			return nil, errSteamFormat
		}
		if kind == 'e' {
			if braced {
				return nil, errSteamFormat
			}
			return nodes, nil
		}
		if kind == '}' {
			if !braced {
				return nil, errSteamFormat
			}
			return nodes, nil
		}
		if kind != 's' {
			return nil, errSteamFormat
		}
		value, valueKind, err := lexer.token()
		if err != nil || valueKind == 'e' || valueKind == '}' {
			return nil, errSteamFormat
		}
		node := steamVDFNode{key: key}
		lexer.nodes++
		if lexer.nodes > steamMaxNodes {
			return nil, errSteamFormat
		}
		if valueKind == '{' {
			node.children, err = parseSteamVDFObject(lexer, true, depth+1)
		} else if valueKind == 's' {
			node.value = value
		} else {
			return nil, errSteamFormat
		}
		if err != nil {
			return nil, errSteamFormat
		}
		nodes = append(nodes, node)
	}
	return nil, errSteamFormat
}

func parseSteamLibraryFolders(data []byte) ([]string, error) {
	nodes, err := parseSteamVDF(data)
	if err != nil {
		return nil, errSteamFormat
	}
	var paths []string
	var visit func([]steamVDFNode)
	visit = func(values []steamVDFNode) {
		for _, node := range values {
			if !strings.EqualFold(node.key, "libraryfolders") {
				continue
			}
			for _, entry := range node.children {
				if len(entry.children) == 0 && entry.value != "" {
					if path, ok := safeSteamLibraryPath(entry.value); ok {
						paths = append(paths, path)
					}
				}
				for _, field := range entry.children {
					if strings.EqualFold(field.key, "path") {
						if path, ok := safeSteamLibraryPath(field.value); ok {
							paths = append(paths, path)
						}
					}
				}
			}
		}
	}
	visit(nodes)
	return uniqueSteamPaths(paths), nil
}

func safeSteamLibraryPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "", false
		}
	}
	value = filepath.Clean(filepath.FromSlash(value))
	if !filepath.IsAbs(value) {
		return "", false
	}
	return value, true
}

func parseSteamManifest(data []byte) (uint32, string, error) {
	nodes, err := parseSteamVDF(data)
	if err != nil {
		return 0, "", errSteamFormat
	}
	for _, node := range nodes {
		if !strings.EqualFold(node.key, "appstate") {
			continue
		}
		var idText, title string
		for _, field := range node.children {
			switch {
			case strings.EqualFold(field.key, "appid"):
				idText = field.value
			case strings.EqualFold(field.key, "name"):
				title = strings.TrimSpace(field.value)
			}
		}
		id, err := strconv.ParseUint(strings.TrimSpace(idText), 10, 32)
		if err != nil || title == "" {
			return 0, "", errSteamFormat
		}
		return uint32(id), title, nil
	}
	return 0, "", errSteamFormat
}
