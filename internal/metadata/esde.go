package metadata

import (
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/media"
	"github.com/jrmoulckers/game-library/internal/model"
)

const (
	maxGamelistBytes   = 4 << 20
	maxGamelistEntries = 50000
	maxGamelistText    = 8192
)

type GamelistEntry struct {
	System  string
	RawStem string
	Name    string
}

type ESDEFileSystem struct {
	Open    func(string) (io.ReadCloser, error)
	ReadDir func(string) ([]os.DirEntry, error)
}

func ReadGamelist(reader io.Reader, system string) ([]GamelistEntry, error) {
	decoder := xml.NewDecoder(io.LimitReader(reader, maxGamelistBytes+1))
	decoder.Strict = true
	var entries []GamelistEntry
	var current struct {
		inGame bool
		path   string
		name   string
	}
	var stack []string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode gamelist XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			stack = append(stack, value.Name.Local)
			if len(stack) > 32 {
				return nil, fmt.Errorf("gamelist XML exceeds maximum depth")
			}
			if value.Name.Local == "game" {
				if current.inGame {
					return nil, fmt.Errorf("nested game element")
				}
				current.inGame = true
				current.path = ""
				current.name = ""
			}
		case xml.CharData:
			if !current.inGame || len(stack) == 0 {
				continue
			}
			text := string(value)
			if len(text) > maxGamelistText {
				return nil, fmt.Errorf("gamelist text exceeds maximum length")
			}
			switch stack[len(stack)-1] {
			case "path":
				current.path += text
			case "name":
				current.name += text
			}
			if len(current.path) > maxGamelistText || len(current.name) > maxGamelistText {
				return nil, fmt.Errorf("gamelist value exceeds maximum length")
			}
		case xml.EndElement:
			if value.Name.Local == "game" {
				entry, ok := finishGamelistEntry(system, current.path, current.name)
				if ok {
					entries = append(entries, entry)
					if len(entries) > maxGamelistEntries {
						return nil, fmt.Errorf("gamelist exceeds maximum entry count")
					}
				}
				current.inGame = false
			}
			if len(stack) == 0 || stack[len(stack)-1] != value.Name.Local {
				return nil, fmt.Errorf("mismatched gamelist XML element")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if current.inGame || len(stack) != 0 {
		return nil, fmt.Errorf("truncated gamelist XML")
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].System != entries[j].System {
			return entries[i].System < entries[j].System
		}
		return entries[i].RawStem < entries[j].RawStem
	})
	return entries, nil
}

func ResolveESDE(roots []model.Root, fileSystem ESDEFileSystem) Catalog {
	if fileSystem.Open == nil {
		fileSystem.Open = func(name string) (io.ReadCloser, error) { return os.Open(name) }
	}
	if fileSystem.ReadDir == nil {
		fileSystem.ReadDir = os.ReadDir
	}
	builder := NewBuilder()
	seenDirectories := make(map[string]struct{})
	for _, root := range roots {
		if root.Kind != "esde-media" {
			continue
		}
		gamelists := filepath.Join(filepath.Dir(filepath.Clean(root.Path)), "gamelists")
		key := filepath.Clean(gamelists)
		if _, exists := seenDirectories[key]; exists {
			continue
		}
		seenDirectories[key] = struct{}{}
		resolveESDEDirectory(builder, fileSystem, gamelists)
	}
	return builder.Build()
}

func resolveESDEDirectory(builder *Builder, fileSystem ESDEFileSystem, directory string) {
	systems, err := fileSystem.ReadDir(directory)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			builder.AddDiagnostic(Diagnostic{
				Source: "esde", Status: "unavailable",
				Message: "ES-DE game titles are temporarily unavailable.",
			})
		}
		return
	}
	for _, entry := range systems {
		if !entry.IsDir() || !safeSystemKey(entry.Name()) {
			continue
		}
		system := strings.ToLower(entry.Name())
		file, err := fileSystem.Open(filepath.Join(directory, entry.Name(), "gamelist.xml"))
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				builder.AddDiagnostic(Diagnostic{
					Source: "esde:" + system, Status: "unavailable",
					Message: "This system's ES-DE titles are temporarily unavailable.",
				})
			}
			continue
		}
		entries, parseErr := ReadGamelist(file, system)
		closeErr := file.Close()
		if parseErr != nil || closeErr != nil {
			builder.AddDiagnostic(Diagnostic{
				Source: "esde:" + system, Status: "unsupported",
				Message: "This system is using cleaned file names because its gamelist could not be read safely.",
			})
			continue
		}
		type titleValue struct {
			title      string
			conflicted bool
		}
		grouped := make(map[string]map[string]titleValue)
		for _, item := range entries {
			baseIdentity := "retro:" + system + ":" + media.IdentitySlug(item.RawStem)
			if grouped[baseIdentity] == nil {
				grouped[baseIdentity] = make(map[string]titleValue)
			}
			value := grouped[baseIdentity][item.RawStem]
			if value.title != "" && value.title != item.Name {
				value.conflicted = true
				builder.AddDiagnostic(Diagnostic{
					Source: "esde:" + system, Status: "needs-attention",
					Message: "Conflicting ES-DE names were kept separate.",
				})
			}
			if value.title == "" {
				value.title = item.Name
			}
			grouped[baseIdentity][item.RawStem] = value
		}
		for baseIdentity, values := range grouped {
			disambiguate := len(values) > 1
			if disambiguate {
				builder.AddDiagnostic(Diagnostic{
					Source: "esde:" + system, Status: "needs-attention",
					Message: "Similar ROM file names were kept as separate games.",
				})
			}
			for rawStem, value := range values {
				if value.conflicted {
					continue
				}
				identity := baseIdentity
				if disambiguate {
					identity = DisambiguatedRetroIdentity(system, rawStem)
				}
				builder.AddTitle(identity, value.title, "esde-gamelist")
			}
		}
	}
}

func DisambiguatedRetroIdentity(system, rawStem string) string {
	sum := sha256.Sum256([]byte(rawStem))
	return "retro:" + strings.ToLower(system) + ":" + media.IdentitySlug(rawStem) + "~" + fmt.Sprintf("%x", sum[:4])
}

func finishGamelistEntry(system, gamePath, name string) (GamelistEntry, bool) {
	system = strings.ToLower(strings.TrimSpace(system))
	gamePath = strings.TrimSpace(strings.ReplaceAll(gamePath, `\`, "/"))
	name = strings.TrimSpace(name)
	if !safeSystemKey(system) || gamePath == "" || name == "" {
		return GamelistEntry{}, false
	}
	base := path.Base(gamePath)
	if base == "." || base == "/" || base == "" {
		return GamelistEntry{}, false
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	if stem == "" || stem == "." || stem == ".." {
		return GamelistEntry{}, false
	}
	return GamelistEntry{System: system, RawStem: stem, Name: name}, true
}

func safeSystemKey(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
