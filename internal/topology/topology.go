// Package topology describes where a person's game artwork is meant to
// live: which hardware they own, which game platforms run on each piece of
// hardware, and which named artwork profiles exist for each platform.
//
// This is deliberately separate from the synced Decky catalog. The catalog
// stores artwork payloads and the small profile records the Deck plugin
// reads; it must keep working untouched. Topology is the owner's own
// description of their setup, so it lives in the gamelib workspace and is
// never written into the catalog.
package topology

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/config"
)

// Version is the schema version of a topology document.
const Version = 1

// Platform is a game library or frontend that presents artwork, such as
// Steam or Playnite. Platforms are the unit that profiles belong to: a
// profile named "Standard" means something different for Steam than it
// does for a retro frontend, so a profile is only ever identified
// alongside its platform.
type Platform struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Device is a piece of hardware the owner actually uses, and the
// platforms installed on it. A platform listed here is what makes its
// profiles reachable from that device.
type Device struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Platforms []string `json:"platforms"`
}

// Profile is a named artwork set for one platform, for example Steam's
// "Minimalist". Artwork names the catalog artwork set that backs it, and
// is empty for a profile that has been declared but not filled in yet.
// An unbacked profile is normal and is reported as empty rather than
// missing, because declaring the shape of a collection before populating
// it is the expected way to use this.
type Profile struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
	Artwork  string `json:"artwork,omitempty"`
}

// Key returns the stable identity of a profile: its platform and a slug
// of its name. Two profiles sharing a name on different platforms are
// different profiles and never collide.
func (p Profile) Key() string {
	return p.Platform + "/" + Slug(p.Name)
}

// Document is a complete description of an owner's setup.
type Document struct {
	Version   int        `json:"version"`
	Platforms []Platform `json:"platforms"`
	Devices   []Device   `json:"devices"`
	Profiles  []Profile  `json:"profiles"`
}

// Slug reduces a display name to a path-safe lowercase identifier.
func Slug(name string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Default returns the starting topology. It encodes the setup the owner
// described rather than an empty document, because an empty topology
// would render an empty coverage view and teach nobody anything. Every
// value here is editable.
func Default() Document {
	return Document{
		Version: Version,
		Platforms: []Platform{
			{ID: "steam", Name: "Steam"},
			{ID: "playnite", Name: "Playnite"},
			{ID: "retro", Name: "Retro gaming"},
		},
		Devices: []Device{
			{ID: "pc", Name: "Gaming PC", Platforms: []string{"steam", "playnite"}},
			{ID: "steam-deck", Name: "Steam Deck", Platforms: []string{"steam", "retro"}},
			{ID: "rog-ally-x", Name: "ROG Ally X", Platforms: []string{"steam"}},
		},
		Profiles: []Profile{
			{Platform: "steam", Name: "Standard"},
			{Platform: "steam", Name: "Retro"},
			{Platform: "steam", Name: "Minimalist"},
			{Platform: "steam", Name: "Tasteful"},
			{Platform: "playnite", Name: "Standard"},
			{Platform: "playnite", Name: "Minimalist"},
			{Platform: "retro", Name: "Retro"},
			{Platform: "retro", Name: "Minimalist"},
			{Platform: "retro", Name: "Tasteful"},
			{Platform: "retro", Name: "Canon"},
		},
	}
}

// Validate reports whether a document is internally consistent. It is
// strict about references and identity, because a dangling platform id
// would silently drop a profile out of every view.
func Validate(doc Document) error {
	if doc.Version != Version {
		return fmt.Errorf("topology version must be %d", Version)
	}
	platforms := make(map[string]struct{}, len(doc.Platforms))
	for _, platform := range doc.Platforms {
		if !config.IsSafeID(platform.ID) {
			return fmt.Errorf("platform id %q is not path-safe", platform.ID)
		}
		if _, ok := platforms[platform.ID]; ok {
			return fmt.Errorf("duplicate platform id %q", platform.ID)
		}
		if strings.TrimSpace(platform.Name) == "" {
			return fmt.Errorf("platform %q needs a name", platform.ID)
		}
		platforms[platform.ID] = struct{}{}
	}
	devices := make(map[string]struct{}, len(doc.Devices))
	for _, device := range doc.Devices {
		if !config.IsSafeID(device.ID) {
			return fmt.Errorf("device id %q is not path-safe", device.ID)
		}
		if _, ok := devices[device.ID]; ok {
			return fmt.Errorf("duplicate device id %q", device.ID)
		}
		if strings.TrimSpace(device.Name) == "" {
			return fmt.Errorf("device %q needs a name", device.ID)
		}
		devices[device.ID] = struct{}{}
		seen := make(map[string]struct{}, len(device.Platforms))
		for _, id := range device.Platforms {
			if _, ok := platforms[id]; !ok {
				return fmt.Errorf("device %q lists unknown platform %q", device.ID, id)
			}
			if _, ok := seen[id]; ok {
				return fmt.Errorf("device %q lists platform %q twice", device.ID, id)
			}
			seen[id] = struct{}{}
		}
	}
	keys := make(map[string]struct{}, len(doc.Profiles))
	for _, profile := range doc.Profiles {
		if _, ok := platforms[profile.Platform]; !ok {
			return fmt.Errorf("profile %q references unknown platform %q", profile.Name, profile.Platform)
		}
		if strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("profile on platform %q needs a name", profile.Platform)
		}
		if Slug(profile.Name) == "" {
			return fmt.Errorf("profile name %q has no usable identifier", profile.Name)
		}
		if profile.Artwork != "" && !config.IsSafeID(profile.Artwork) {
			return fmt.Errorf("profile %q artwork set %q is not path-safe", profile.Name, profile.Artwork)
		}
		key := profile.Key()
		if _, ok := keys[key]; ok {
			return fmt.Errorf("duplicate profile %q on platform %q", profile.Name, profile.Platform)
		}
		keys[key] = struct{}{}
	}
	return nil
}

// DevicesFor returns the devices that can present a platform, sorted by
// id. This is what turns "which profile" into "which hardware".
func (doc Document) DevicesFor(platformID string) []Device {
	var result []Device
	for _, device := range doc.Devices {
		for _, id := range device.Platforms {
			if id == platformID {
				result = append(result, device)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// PlatformName resolves a platform id to its display name, falling back
// to the id so an unknown platform is still legible.
func (doc Document) PlatformName(platformID string) string {
	for _, platform := range doc.Platforms {
		if platform.ID == platformID {
			return platform.Name
		}
	}
	return platformID
}

// Load reads the topology document at path. A file that has never been
// written is not an error: it reports the default document with
// found=false, so a first run shows the owner's declared setup instead of
// an empty screen.
func Load(path string) (doc Document, found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), false, nil
		}
		return Document{}, false, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, false, fmt.Errorf("parse topology: %w", err)
	}
	if err := Validate(doc); err != nil {
		return Document{}, false, err
	}
	return doc, true, nil
}

// Save validates and atomically writes a topology document.
func Save(path string, doc Document) error {
	if err := Validate(doc); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".topology-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
