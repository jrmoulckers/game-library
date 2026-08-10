package metadata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	playnitePageSize = 4096
	playniteVersion  = 7
	maxPlaynitePages = 1 << 20
	maxPlayniteGames = 100000
	maxBSONBytes     = 16 << 20
	maxBSONElements  = 100000
	maxBSONDepth     = 64
	maxBSONString    = 4 << 20
)

const (
	playniteHeaderPageType     = 1
	playniteCollectionPageType = 2
	playniteIndexPageType      = 3
	playniteDataPageType       = 4
	playniteExtendPageType     = 5
	playniteBaseHeaderSize     = 25
)

var playniteHeaderSignature = []byte("** This is a LiteDB file **")

// PlayniteResult is deliberately a value, rather than an error-only API.
// Playnite is commonly running while a library is being inspected; callers
// can retain a calm status and diagnostic without treating that as a failure
// of the rest of a metadata import.
type PlayniteResult struct {
	Games       []PlayniteGame `json:"games,omitempty"`
	Status      string         `json:"status"`
	Message     string         `json:"message,omitempty"`
	Diagnostic  Diagnostic     `json:"diagnostic,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
}

type PlayniteGame struct {
	PlayniteGUID string                   `json:"playniteGuid"`
	Name         string                   `json:"name"`
	GameID       string                   `json:"gameId"`
	PluginID     string                   `json:"pluginId"`
	PlatformIDs  []string                 `json:"platformIds,omitempty"`
	SourceID     string                   `json:"sourceId"`
	Hidden       bool                     `json:"hidden"`
	Media        []PlayniteMediaReference `json:"media,omitempty"`
}

type PlayniteMediaReference struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

const (
	playniteStatusOK          = "ok"
	playniteStatusUnavailable = "unavailable"
	playniteStatusBusy        = "busy"
	playniteStatusUnsupported = "unsupported"
	playniteStatusCorrupt     = "corrupt"
	playniteStatusUnstable    = "unstable"
	playniteStatusEncrypted   = "encrypted"
	playniteStatusRecovering  = "recovering"
	playniteStatusJournal     = "journal"
)

var (
	errPlayniteUnsupported = errors.New("unsupported Playnite database")
	errPlayniteCorrupt     = errors.New("corrupt Playnite database")
	errPlayniteUnstable    = errors.New("Playnite database changed while reading")
	errPlayniteEncrypted   = errors.New("encrypted Playnite database")
	errPlayniteRecovering  = errors.New("Playnite database is recovering")
	errPlayniteJournal     = errors.New("Playnite journal tail is present")
)

// ReadPlaynite reads a Playnite library without taking a lock and without
// writing a journal, lock file, or recovery marker.  On any structural error
// Games is empty: a database is either a complete validated snapshot or is
// reported through Status and Diagnostic.
func ReadPlaynite(path string) PlayniteResult {
	file, err := openReadOnly(path)
	if err != nil {
		if isReadBusy(err) {
			return playniteFailure(playniteStatusBusy, "Playnite is running - titles are temporarily unavailable.", err)
		}
		return playniteFailure(playniteStatusUnavailable, "Playnite library is not available.", err)
	}
	defer file.Close()

	before, err := file.Stat()
	if err != nil {
		return playniteFailure(playniteStatusUnavailable, "Playnite library could not be inspected.", err)
	}
	afterOpen, err := file.Stat()
	if err != nil {
		return playniteFailure(playniteStatusUnavailable, "Playnite library could not be inspected.", err)
	}
	if !sameFileState(before, afterOpen) {
		return playniteFailure(playniteStatusUnstable, "Playnite library changed while it was opened.", errPlayniteUnstable)
	}

	reader := &playniteReader{file: file, physicalSize: before.Size()}
	header, err := reader.readHeader()
	if err != nil {
		return playniteFailure(playniteStatusForError(err), playniteMessageForError(err), err)
	}
	if header.logicalSize != before.Size() {
		if before.Size() > header.logicalSize {
			return playniteFailure(playniteStatusJournal, "Playnite library has an uncommitted journal tail.", errPlayniteJournal)
		}
		return playniteFailure(playniteStatusCorrupt, "Playnite library length is inconsistent.", errPlayniteCorrupt)
	}
	if header.logicalSize < playnitePageSize || header.logicalSize%playnitePageSize != 0 {
		return playniteFailure(playniteStatusCorrupt, "Playnite library length is invalid.", errPlayniteCorrupt)
	}
	if header.lastPage+1 != uint32(header.logicalSize/playnitePageSize) {
		return playniteFailure(playniteStatusCorrupt, "Playnite library page count is inconsistent.", errPlayniteCorrupt)
	}
	reader.logicalSize = header.logicalSize

	collections, err := reader.readCollections(header.collectionPage)
	if err != nil {
		return playniteFailure(playniteStatusForError(err), playniteMessageForError(err), err)
	}
	gamesCollection, ok := collections["games"]
	if !ok {
		return playniteFailure(playniteStatusUnsupported, "The Playnite games collection is unavailable.", errPlayniteUnsupported)
	}
	games, err := reader.readGames(gamesCollection)
	if err != nil {
		return playniteFailure(playniteStatusForError(err), playniteMessageForError(err), err)
	}

	after, err := file.Stat()
	if err != nil || !sameFileState(before, after) {
		return playniteFailure(playniteStatusUnstable, "Playnite library changed while it was read.", errPlayniteUnstable)
	}
	afterHeader, err := reader.readHeader()
	if err != nil {
		return playniteFailure(playniteStatusUnstable, "Playnite library changed while it was read.", errPlayniteUnstable)
	}
	if afterHeader.changeID != header.changeID ||
		afterHeader.logicalSize != header.logicalSize {
		return playniteFailure(playniteStatusUnstable, "Playnite library changed while it was read.", errPlayniteUnstable)
	}
	return PlayniteResult{Games: games, Status: playniteStatusOK}
}

func playniteFailure(status, message string, cause error) PlayniteResult {
	if status == "" {
		status = playniteStatusUnsupported
	}
	d := Diagnostic{Source: "playnite", Status: status, Message: message}
	if cause == nil {
		cause = errors.New(message)
	}
	return PlayniteResult{Status: status, Message: message, Diagnostic: d, Diagnostics: []Diagnostic{d}}
}

func playniteStatusForError(err error) string {
	switch {
	case errors.Is(err, errPlayniteUnstable):
		return playniteStatusUnstable
	case errors.Is(err, errPlayniteEncrypted):
		return playniteStatusEncrypted
	case errors.Is(err, errPlayniteRecovering):
		return playniteStatusRecovering
	case errors.Is(err, errPlayniteJournal):
		return playniteStatusJournal
	case errors.Is(err, errPlayniteUnsupported):
		return playniteStatusUnsupported
	default:
		return playniteStatusCorrupt
	}
}

func playniteMessageForError(err error) string {
	switch {
	case errors.Is(err, errPlayniteEncrypted):
		return "Playnite library encryption is not supported by the offline reader."
	case errors.Is(err, errPlayniteRecovering):
		return "Playnite library recovery is in progress."
	case errors.Is(err, errPlayniteJournal):
		return "Playnite library has an uncommitted journal tail."
	case errors.Is(err, errPlayniteUnsupported):
		return "Playnite library format is not supported."
	case errors.Is(err, errPlayniteUnstable):
		return "Playnite library changed while it was read."
	default:
		return "Playnite library is structurally inconsistent."
	}
}

func sameFileState(a, b os.FileInfo) bool {
	return a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

type playniteHeader struct {
	logicalSize    int64
	lastPage       uint32
	collectionPage pageAddress
	changeID       uint32
}

type playniteReader struct {
	file         io.ReaderAt
	physicalSize int64
	logicalSize  int64
}

type playnitePage struct {
	bytes     []byte
	id        uint32
	pageType  byte
	prev      uint32
	next      uint32
	itemCount uint16
	freeBytes uint16
}

func (r *playniteReader) readAt(p []byte, offset int64) error {
	if offset < 0 || int64(len(p)) > r.physicalSize-offset {
		return errPlayniteCorrupt
	}
	n, err := r.file.ReadAt(p, offset)
	if err != nil && !(err == io.EOF && n == len(p)) {
		return errPlayniteCorrupt
	}
	if n != len(p) {
		return errPlayniteCorrupt
	}
	return nil
}

func (r *playniteReader) page(id uint32) (playnitePage, error) {
	if id >= maxPlaynitePages || id == ^uint32(0) {
		return playnitePage{}, errPlayniteCorrupt
	}
	if r.logicalSize < playnitePageSize ||
		int64(id+1)*playnitePageSize > r.logicalSize {
		return playnitePage{}, errPlayniteCorrupt
	}
	page := make([]byte, playnitePageSize)
	if err := r.readAt(page, int64(id)*playnitePageSize); err != nil {
		return playnitePage{}, err
	}
	if got := binary.LittleEndian.Uint32(page); got != id {
		return playnitePage{}, fmt.Errorf("%w: page id mismatch", errPlayniteCorrupt)
	}
	if page[4] < playniteHeaderPageType || page[4] > playniteExtendPageType {
		return playnitePage{}, fmt.Errorf("%w: unknown page type", errPlayniteCorrupt)
	}
	for _, b := range page[17:25] {
		if b != 0 {
			return playnitePage{}, fmt.Errorf("%w: non-zero page header reserved bytes", errPlayniteCorrupt)
		}
	}
	return playnitePage{
		bytes: page, id: id, pageType: page[4],
		prev:      binary.LittleEndian.Uint32(page[5:]),
		next:      binary.LittleEndian.Uint32(page[9:]),
		itemCount: binary.LittleEndian.Uint16(page[13:]),
		freeBytes: binary.LittleEndian.Uint16(page[15:]),
	}, nil
}

func (r *playniteReader) readHeader() (playniteHeader, error) {
	page := make([]byte, playnitePageSize)
	if r.physicalSize < playnitePageSize {
		return playniteHeader{}, errPlayniteCorrupt
	}
	if err := r.readAt(page, 0); err != nil {
		return playniteHeader{}, err
	}

	if string(page[playniteBaseHeaderSize:playniteBaseHeaderSize+len(playniteHeaderSignature)]) != string(playniteHeaderSignature) ||
		page[52] != playniteVersion {
		return playniteHeader{}, errPlayniteUnsupported
	}
	if binary.LittleEndian.Uint32(page) != 0 || page[4] != playniteHeaderPageType {
		return playniteHeader{}, errPlayniteCorrupt
	}
	for _, b := range page[17:25] {
		if b != 0 {
			return playniteHeader{}, errPlayniteCorrupt
		}
	}
	for _, b := range page[65:85] {
		if b != 0 {
			return playniteHeader{}, errPlayniteEncrypted
		}
	}
	// Salt is deliberately ignored; it is not a password verifier.
	if page[4095] != 0 {
		return playniteHeader{}, errPlayniteRecovering
	}
	lastPage := binary.LittleEndian.Uint32(page[59:])
	if lastPage >= maxPlaynitePages {
		return playniteHeader{}, errPlayniteCorrupt
	}
	logicalSize := int64(lastPage+1) * playnitePageSize
	if logicalSize < playnitePageSize || logicalSize%playnitePageSize != 0 ||
		logicalSize > r.physicalSize {
		return playniteHeader{}, errPlayniteCorrupt
	}
	collectionPage := emptyPageAddress
	position := 102
	for i := 0; i < int(page[101]); i++ {
		name, next, ok := readHeaderCollectionName(page, position)
		if !ok || next+4 > len(page) {
			return playniteHeader{}, errPlayniteCorrupt
		}
		address := pageAddress{page: binary.LittleEndian.Uint32(page[next:]), slot: 0}
		if !address.valid(logicalSize) {
			return playniteHeader{}, errPlayniteCorrupt
		}
		if strings.EqualFold(name, "games") {
			if !collectionPage.empty() {
				return playniteHeader{}, errPlayniteCorrupt
			}
			collectionPage = address
		}
		position = next + 4
	}
	if collectionPage.empty() {
		return playniteHeader{}, errPlayniteUnsupported
	}
	return playniteHeader{
		logicalSize:    logicalSize,
		lastPage:       lastPage,
		collectionPage: collectionPage,
		changeID:       uint32(binary.LittleEndian.Uint16(page[53:])),
	}, nil
}

func readHeaderCollectionName(page []byte, at int) (string, int, bool) {
	if at < 0 || at+4 > len(page) {
		return "", 0, false
	}
	length := int64(int32(binary.LittleEndian.Uint32(page[at:])))
	if length <= 0 || length > 60 || length > int64(len(page)-at-4) {
		return "", 0, false
	}
	value := page[at+4 : at+4+int(length)]
	if !utf8.Valid(value) || strings.IndexByte(string(value), 0) >= 0 {
		return "", 0, false
	}
	return string(value), at + 4 + int(length), true
}

type pageAddress struct {
	page uint32
	slot uint16
}

var emptyPageAddress = pageAddress{page: ^uint32(0), slot: ^uint16(0)}

func readPageAddress(b []byte) pageAddress {
	if len(b) < 6 {
		return emptyPageAddress
	}
	return pageAddress{page: binary.LittleEndian.Uint32(b), slot: binary.LittleEndian.Uint16(b[4:])}
}

func (a pageAddress) empty() bool {
	return a.page == ^uint32(0) && a.slot == ^uint16(0)
}

func (a pageAddress) valid(logicalSize int64) bool {
	return !a.empty() && a.page < maxPlaynitePages &&
		int64(a.page+1)*playnitePageSize <= logicalSize &&
		int(a.slot) < playnitePageSize
}
