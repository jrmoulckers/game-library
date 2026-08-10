// Package metadata reads optional local metadata from installed game
// launchers. Every provider is best-effort and read-only: a provider that is
// missing, locked, or malformed yields a diagnostic rather than an error, so
// one unavailable source never suppresses titles from another.
package metadata

import (
	"sort"
	"strings"
)

const SteamPluginID = "cb91dfc9-b977-43bf-8e70-55f46e410fab"

type TitleRecord struct {
	Title  string `json:"title"`
	Source string `json:"source"`
}

type Diagnostic struct {
	Source  string `json:"source"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type SourceState struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	ItemCount int    `json:"itemCount,omitempty"`
}

type Catalog struct {
	Titles      map[string]TitleRecord `json:"titles"`
	Aliases     map[string]string      `json:"aliases"`
	Stores      map[string]string      `json:"stores,omitempty"`
	Ambiguous   map[string]bool        `json:"ambiguous,omitempty"`
	Diagnostics []Diagnostic           `json:"diagnostics,omitempty"`
	Sources     []SourceState          `json:"sources,omitempty"`
}

func NewCatalog() Catalog {
	return Catalog{
		Titles:    make(map[string]TitleRecord),
		Aliases:   make(map[string]string),
		Stores:    make(map[string]string),
		Ambiguous: make(map[string]bool),
	}
}

func (c Catalog) Title(identity string) (TitleRecord, bool) {
	if c.Titles == nil {
		return TitleRecord{}, false
	}
	canonical := c.Canonical(identity)
	record, ok := c.Titles[canonical]
	if ok {
		return record, true
	}
	record, ok = c.Titles[identity]
	return record, ok
}

func (c Catalog) Canonical(identity string) string {
	seen := make(map[string]struct{})
	current := identity
	for current != "" {
		if _, exists := seen[current]; exists {
			return identity
		}
		seen[current] = struct{}{}
		next := c.Aliases[current]
		if next == "" {
			return current
		}
		current = next
	}
	return identity
}

type Builder struct {
	catalog   Catalog
	conflicts map[string]struct{}
}

func NewBuilder() *Builder {
	return &Builder{catalog: NewCatalog(), conflicts: make(map[string]struct{})}
}

func (b *Builder) AddTitle(identity, title, source string) {
	identity = strings.TrimSpace(identity)
	title = strings.TrimSpace(title)
	if identity == "" || title == "" {
		return
	}
	if current, exists := b.catalog.Titles[identity]; !exists || titlePriority(source) < titlePriority(current.Source) {
		b.catalog.Titles[identity] = TitleRecord{Title: title, Source: source}
	}
}

func titlePriority(source string) int {
	switch source {
	case "steam-appinfo":
		return 10
	case "steam-manifest":
		return 20
	case "steam-shortcut":
		return 30
	case "playnite":
		return 40
	case "esde-gamelist":
		return 10
	default:
		return 100
	}
}

func (b *Builder) AddAlias(identity, canonical string) {
	identity = strings.TrimSpace(identity)
	canonical = strings.TrimSpace(canonical)
	if identity == "" || canonical == "" || identity == canonical {
		return
	}

	if previous := b.catalog.Aliases[identity]; previous != "" && previous != canonical {
		delete(b.catalog.Aliases, identity)
		b.conflicts[identity] = struct{}{}
		return
	}
	if _, conflicted := b.conflicts[identity]; conflicted {
		return
	}
	b.catalog.Aliases[identity] = canonical
}

func (b *Builder) AddStore(identity, store string) {
	identity = strings.TrimSpace(identity)
	store = strings.TrimSpace(store)
	if identity == "" || store == "" {
		return
	}
	if _, exists := b.catalog.Stores[identity]; !exists {
		b.catalog.Stores[identity] = store
	}
}

func (b *Builder) AddDiagnostic(value Diagnostic) {
	b.catalog.Diagnostics = append(b.catalog.Diagnostics, value)
}

func (b *Builder) AddSource(value SourceState) {
	b.catalog.Sources = append(b.catalog.Sources, value)
}

func (b *Builder) Merge(catalog Catalog) {
	for identity, record := range catalog.Titles {
		b.AddTitle(identity, record.Title, record.Source)
	}
	for identity, canonical := range catalog.Aliases {
		b.AddAlias(identity, canonical)
	}
	for identity, store := range catalog.Stores {
		b.AddStore(identity, store)
	}
	for identity, ambiguous := range catalog.Ambiguous {
		if ambiguous {
			b.conflicts[identity] = struct{}{}
		}
	}
	for _, diagnostic := range catalog.Diagnostics {
		b.AddDiagnostic(diagnostic)
	}
	for _, source := range catalog.Sources {
		b.AddSource(source)
	}
}

func (b *Builder) Build() Catalog {
	for identity := range b.conflicts {
		b.catalog.Ambiguous[identity] = true
		b.catalog.Diagnostics = append(b.catalog.Diagnostics, Diagnostic{
			Source: "identity", Status: "needs-attention",
			Message: "Conflicting exact identities were kept separate.",
		})
		delete(b.catalog.Aliases, identity)
	}
	sort.Slice(b.catalog.Diagnostics, func(i, j int) bool {
		if b.catalog.Diagnostics[i].Source != b.catalog.Diagnostics[j].Source {
			return b.catalog.Diagnostics[i].Source < b.catalog.Diagnostics[j].Source
		}
		if b.catalog.Diagnostics[i].Status != b.catalog.Diagnostics[j].Status {
			return b.catalog.Diagnostics[i].Status < b.catalog.Diagnostics[j].Status
		}
		return b.catalog.Diagnostics[i].Message < b.catalog.Diagnostics[j].Message
	})
	sort.Slice(b.catalog.Sources, func(i, j int) bool {
		if b.catalog.Sources[i].Name != b.catalog.Sources[j].Name {
			return b.catalog.Sources[i].Name < b.catalog.Sources[j].Name
		}
		return b.catalog.Sources[i].ID < b.catalog.Sources[j].ID
	})
	return b.catalog
}

func StoreName(pluginID string) string {
	switch strings.ToLower(strings.TrimSpace(pluginID)) {
	case SteamPluginID:
		return "Steam"
	case "", "00000000-0000-0000-0000-000000000000":
		return "Manually added"
	default:
		return "Other library"
	}
}
