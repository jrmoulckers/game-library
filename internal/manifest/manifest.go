package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/policy"
)

var fullSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var safeExtension = regexp.MustCompile(`^[a-z0-9]{1,8}$`)

func BuildImport(inventory model.Inventory, policies model.PolicyFile) (model.Manifest, error) {
	if inventory.Privacy != "private" || len(inventory.Observations) == 0 {
		return model.Manifest{}, fmt.Errorf("import planning requires a private inventory with observations")
	}
	if err := policy.Validate(policies); err != nil {
		return model.Manifest{}, err
	}
	var actions []model.Action
	for _, observation := range inventory.Observations {
		if !config.IsSafeID(observation.RootID) {
			return model.Manifest{}, fmt.Errorf("observation root id %q is not path-safe", observation.RootID)
		}
		if !safeRelativePath(observation.RelativePath) {
			return model.Manifest{}, fmt.Errorf("observation %s has unsafe relative path %q", observation.RootID, observation.RelativePath)
		}
		if !fullSHA256.MatchString(observation.SHA256) {
			return model.Manifest{}, fmt.Errorf("observation %s:%s has invalid SHA-256", observation.RootID, observation.RelativePath)
		}
		if observation.Media.Extension != "" && !safeExtension.MatchString(strings.ToLower(observation.Media.Extension)) {
			return model.Manifest{}, fmt.Errorf("observation %s:%s has unsafe extension", observation.RootID, observation.RelativePath)
		}
		mode := policy.Resolve(policies, observation)
		action := model.Action{
			SourceRoot:   observation.RootID,
			SourcePath:   observation.RelativePath,
			SourceSHA256: observation.SHA256,
			SourceSize:   observation.Size,
			Metadata: map[string]string{
				"identityHint": observation.IdentityHint,
				"role":         observation.Media.Role,
				"system":       observation.System,
			},
		}
		switch mode {
		case "managed":
			action.Action = "copy"
			action.DestinationRoot = "catalog"
			action.DestinationPath = assetPath(observation)
			action.ExpectedDestination = "absent-or-same-hash"
			action.Reason = "policy resolves asset to managed canonical storage"
		case "tracked-external":
			action.Action = "skip"
			action.Reason = "asset remains tracked at its external source"
		case "promote-on-approval":
			action.Action = "skip"
			action.Reason = "asset promotion requires separate explicit approval"
		case "quarantined":
			action.Action = "quarantine"
			action.DestinationRoot = "catalog"
			action.DestinationPath = filepath.ToSlash(filepath.Join("state", "quarantine", "policy", observation.SHA256))
			action.ExpectedDestination = "absent-or-same-hash"
			action.Reason = "policy requires quarantine"
		default:
			return model.Manifest{}, fmt.Errorf("unsupported policy mode %q", mode)
		}
		actions = append(actions, action)
	}
	sortActions(actions)
	operationID, sourceDigest, err := digestActions(actions)
	if err != nil {
		return model.Manifest{}, err
	}
	return model.Manifest{
		Version: model.SchemaVersion, ToolVersion: model.ToolVersion,
		OperationID: "import-" + operationID[:16], Kind: "import-plan",
		SourceDigest: sourceDigest, Actions: actions,
		Warnings: []string{"dry-run only; this manifest does not authorize filesystem mutation"},
	}, nil
}

func Digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func VerifyFile(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("manifest digest mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// OperationIDFor recomputes the deterministic operation id NewPlan/BuildImport
// would assign to kind/actions, without duplicating their private sorting
// and hashing logic. It lets a caller (for example the dashboard's history
// listing) verify that a persisted plan artifact's OperationID field still
// matches its own Actions content — i.e. that the artifact has not been
// tampered with since it was written — using only the plan's own recorded
// fields, never a filesystem path.
func OperationIDFor(kind string, actions []model.Action) (string, error) {
	sorted := append([]model.Action(nil), actions...)
	sortActions(sorted)
	operationID, _, err := digestActions(sorted)
	if err != nil {
		return "", err
	}
	return kind + "-" + operationID[:16], nil
}

func NewPlan(kind string, actions []model.Action, warnings ...string) (model.Manifest, error) {
	sortActions(actions)
	operationID, sourceDigest, err := digestActions(actions)
	if err != nil {
		return model.Manifest{}, err
	}
	return model.Manifest{
		Version: model.SchemaVersion, ToolVersion: model.ToolVersion,
		OperationID: kind + "-" + operationID[:16], Kind: kind,
		SourceDigest: sourceDigest, Actions: actions, Warnings: warnings,
	}, nil
}

func assetPath(observation model.Observation) string {
	ext := strings.TrimPrefix(strings.ToLower(observation.Media.Extension), ".")
	if ext == "" {
		ext = "bin"
	}
	return filepath.ToSlash(filepath.Join(
		"library", "assets", "sha256", observation.SHA256[:2], observation.SHA256, "content."+ext,
	))
}

func digestActions(actions []model.Action) (string, string, error) {
	digest, err := Digest(actions)
	if err != nil {
		return "", "", err
	}
	return digest, digest, nil
}

func sortActions(actions []model.Action) {
	sort.Slice(actions, func(i, j int) bool {
		left := actions[i].DestinationRoot + "\x00" + actions[i].DestinationPath + "\x00" + actions[i].SourceRoot + "\x00" + actions[i].SourcePath
		right := actions[j].DestinationRoot + "\x00" + actions[j].DestinationPath + "\x00" + actions[j].SourceRoot + "\x00" + actions[j].SourcePath
		return left < right
	})
}

func safeRelativePath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00") ||
		filepath.IsAbs(value) || filepath.VolumeName(value) != "" ||
		(len(value) >= 2 && value[1] == ':') {
		return false
	}
	clean := path.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}
