// Package model holds the Go types that mirror the JSON Schema contracts in
// schemas/v1. It is the single shared vocabulary for every other package.
package model

import "encoding/json"

const (
	SchemaVersion = 1
	ToolVersion   = "0.1.0"
)

type Config struct {
	Version int        `json:"version"`
	Roots   []Root     `json:"roots"`
	Policy  PolicyFile `json:"policy"`
}

type Root struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	System string `json:"system,omitempty"`
}

type PolicyFile struct {
	Version int          `json:"version"`
	Default string       `json:"default"`
	Rules   []PolicyRule `json:"rules,omitempty"`
}

type PolicyRule struct {
	Source      string `json:"source,omitempty"`
	System      string `json:"system,omitempty"`
	Role        string `json:"role,omitempty"`
	AssetSHA256 string `json:"assetSha256,omitempty"`
	Mode        string `json:"mode"`
}

type Inventory struct {
	Version          int               `json:"version"`
	ToolVersion      string            `json:"toolVersion"`
	CreatedAt        string            `json:"createdAt"`
	Privacy          string            `json:"privacy"`
	Roots            []RootSummary     `json:"roots"`
	Observations     []Observation     `json:"observations,omitempty"`
	DuplicateSummary DuplicateSummary  `json:"duplicateSummary"`
	Issues           []ValidationIssue `json:"issues,omitempty"`
}

type RootSummary struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	System     string         `json:"system,omitempty"`
	FileCount  int            `json:"fileCount"`
	TotalBytes int64          `json:"totalBytes"`
	MediaCount int            `json:"mediaCount"`
	ImageCount int            `json:"imageCount"`
	Extensions map[string]int `json:"extensions,omitempty"`
	Roles      map[string]int `json:"roles,omitempty"`
	Dimensions map[string]int `json:"dimensions,omitempty"`
}

type Observation struct {
	RootID       string     `json:"rootId"`
	RootKind     string     `json:"rootKind"`
	RelativePath string     `json:"relativePath"`
	Size         int64      `json:"size"`
	SHA256       string     `json:"sha256"`
	Media        MediaFacts `json:"media"`
	System       string     `json:"system,omitempty"`
	IdentityHint string     `json:"identityHint,omitempty"`
}

type MediaFacts struct {
	Extension string `json:"extension,omitempty"`
	MIME      string `json:"mime"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Role      string `json:"role,omitempty"`
}

type DuplicateSummary struct {
	Groups           int   `json:"groups"`
	Copies           int   `json:"copies"`
	ExcessBytes      int64 `json:"excessBytes"`
	CrossRootGroups  int   `json:"crossRootGroups"`
	UniqueFileHashes int   `json:"uniqueFileHashes"`
}

type DuplicateReport struct {
	Version     int              `json:"version"`
	ToolVersion string           `json:"toolVersion"`
	Summary     DuplicateSummary `json:"summary"`
	Groups      []DuplicateGroup `json:"groups"`
}

type DuplicateGroup struct {
	SHA256      string              `json:"sha256"`
	Size        int64               `json:"size"`
	Occurrences []DuplicateLocation `json:"occurrences"`
}

type DuplicateLocation struct {
	RootID       string `json:"rootId"`
	RelativePath string `json:"relativePath"`
}

type ValidationIssue struct {
	RootID       string `json:"rootId,omitempty"`
	RelativePath string `json:"relativePath,omitempty"`
	Code         string `json:"code"`
	Message      string `json:"message"`
}

type IdentityReport struct {
	Version     int                `json:"version"`
	ToolVersion string             `json:"toolVersion"`
	Proposals   []IdentityProposal `json:"proposals"`
	Unmapped    []UnmappedItem     `json:"unmapped,omitempty"`
}

type IdentityProposal struct {
	RootID       string `json:"rootId"`
	RelativePath string `json:"relativePath"`
	CanonicalID  string `json:"canonicalId"`
	MappingType  string `json:"mappingType"`
	Confidence   string `json:"confidence"`
	Reason       string `json:"reason"`
}

type UnmappedItem struct {
	RootID       string `json:"rootId"`
	RelativePath string `json:"relativePath"`
	Reason       string `json:"reason"`
}

type Manifest struct {
	Version      int      `json:"version"`
	ToolVersion  string   `json:"toolVersion"`
	OperationID  string   `json:"operationId"`
	Kind         string   `json:"kind"`
	SourceDigest string   `json:"sourceDigest,omitempty"`
	Actions      []Action `json:"actions"`
	Warnings     []string `json:"warnings,omitempty"`
}

type Action struct {
	Action              string            `json:"action"`
	SourceRoot          string            `json:"sourceRoot,omitempty"`
	SourcePath          string            `json:"sourcePath,omitempty"`
	SourceSHA256        string            `json:"sourceSha256,omitempty"`
	SourceSize          int64             `json:"sourceSize,omitempty"`
	DestinationRoot     string            `json:"destinationRoot,omitempty"`
	DestinationPath     string            `json:"destinationPath,omitempty"`
	ExpectedDestination string            `json:"expectedDestination,omitempty"`
	Reason              string            `json:"reason"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

type Profile struct {
	Version       int               `json:"version"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Theme         string            `json:"theme,omitempty"`
	Games         []ProfileGame     `json:"games"`
	Mods          []ProfileMod      `json:"mods,omitempty"`
	Compatibility map[string]string `json:"compatibility,omitempty"`
}

type ProfileGame struct {
	ID         string                    `json:"id"`
	Identities map[string]string         `json:"identities,omitempty"`
	Retro      *RetroTarget              `json:"retro,omitempty"`
	Assets     map[string]AssetSelection `json:"assets"`
}

type RetroTarget struct {
	System string `json:"system"`
	Stem   string `json:"stem"`
}

type AssetSelection struct {
	SHA256    string            `json:"sha256"`
	Extension string            `json:"extension"`
	Variant   map[string]string `json:"variant,omitempty"`
}

type ProfileMod struct {
	Game string `json:"game"`
	Set  string `json:"set"`
}

type ProfileResolution struct {
	Version     int                    `json:"version"`
	ToolVersion string                 `json:"toolVersion"`
	ProfileID   string                 `json:"profileId"`
	Complete    bool                   `json:"complete"`
	Revision    string                 `json:"revision"`
	Assets      []ResolvedProfileAsset `json:"assets"`
	Issues      []ValidationIssue      `json:"issues,omitempty"`
}

type ResolvedProfileAsset struct {
	GameID        string `json:"gameId"`
	Role          string `json:"role"`
	SHA256        string `json:"sha256"`
	Extension     string `json:"extension"`
	CanonicalPath string `json:"canonicalPath"`
	Available     bool   `json:"available"`
}

type DeckyProfileV1 struct {
	Version     int          `json:"version"`
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Artwork     *string      `json:"artwork"`
	Mods        []DeckyModV1 `json:"mods"`
}

type DeckyModV1 struct {
	Game string `json:"game"`
	Set  string `json:"set"`
}

func (profile DeckyProfileV1) MarshalJSON() ([]byte, error) {
	type deckyProfileAlias DeckyProfileV1
	if profile.Mods == nil {
		profile.Mods = []DeckyModV1{}
	}
	return json.Marshal(deckyProfileAlias(profile))
}
