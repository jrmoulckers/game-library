package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/decky"
	"github.com/jrmoulckers/game-library/internal/identity"
	"github.com/jrmoulckers/game-library/internal/inventory"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/profile"
	"github.com/jrmoulckers/game-library/internal/report"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "inventory":
		return runInventory(args[1:])
	case "report":
		return runReport(args[1:])
	case "duplicates":
		return runDuplicates(args[1:])
	case "identity":
		return runIdentity(args[1:])
	case "import":
		return runImport(args[1:])
	case "profile":
		return runProfile(args[1:])
	case "bundle":
		return runBundle(args[1:])
	case "export":
		return runExport(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "manifest":
		return runManifest(args[1:])
	case "version":
		fmt.Println(model.ToolVersion)
		return nil
	default:
		return usageError()
	}
}

func runInventory(args []string) error {
	if len(args) == 0 || args[0] != "scan" {
		return fmt.Errorf("usage: gamelib inventory scan --config FILE --output FILE [--privacy private|sanitized]")
	}
	set := flag.NewFlagSet("inventory scan", flag.ContinueOnError)
	configPath := set.String("config", "", "host-local configuration")
	output := set.String("output", "", "JSON output")
	privacy := set.String("privacy", "sanitized", "private or sanitized")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *configPath == "" || *output == "" {
		return fmt.Errorf("--config and --output are required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	result, err := inventory.Scan(cfg.Roots)
	if err != nil {
		return err
	}
	switch *privacy {
	case "private":
	case "sanitized":
		result = inventory.Sanitize(result)
	default:
		return fmt.Errorf("--privacy must be private or sanitized")
	}
	return report.WriteJSON(*output, result)
}

func runReport(args []string) error {
	if len(args) == 0 || args[0] != "summary" {
		return fmt.Errorf("usage: gamelib report summary --inventory FILE --output FILE")
	}
	set := flag.NewFlagSet("report summary", flag.ContinueOnError)
	input := set.String("inventory", "", "private inventory JSON")
	output := set.String("output", "", "sanitized JSON output")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("--inventory and --output are required")
	}
	var value model.Inventory
	if err := report.ReadJSON(*input, &value); err != nil {
		return err
	}
	return report.WriteJSON(*output, inventory.Sanitize(value))
}

func runDuplicates(args []string) error {
	if len(args) == 0 || args[0] != "report" {
		return fmt.Errorf("usage: gamelib duplicates report --inventory FILE --output FILE")
	}
	input, output, err := twoFileFlags("duplicates report", args[1:], "inventory")
	if err != nil {
		return err
	}
	var value model.Inventory
	if err := report.ReadJSON(input, &value); err != nil {
		return err
	}
	if len(value.Observations) == 0 {
		return fmt.Errorf("duplicate reporting requires private inventory observations")
	}
	return report.WriteJSON(output, inventory.BuildDuplicateReport(value))
}

func runIdentity(args []string) error {
	if len(args) == 0 || args[0] != "propose" {
		return fmt.Errorf("usage: gamelib identity propose --inventory FILE --output FILE")
	}
	input, output, err := twoFileFlags("identity propose", args[1:], "inventory")
	if err != nil {
		return err
	}
	var value model.Inventory
	if err := report.ReadJSON(input, &value); err != nil {
		return err
	}
	if len(value.Observations) == 0 {
		return fmt.Errorf("identity proposals require private inventory observations")
	}
	return report.WriteJSON(output, identity.Propose(value))
}

func runImport(args []string) error {
	if len(args) == 0 || args[0] != "plan" {
		return fmt.Errorf("usage: gamelib import plan --inventory FILE --policy FILE --output FILE")
	}
	set := flag.NewFlagSet("import plan", flag.ContinueOnError)
	input := set.String("inventory", "", "private inventory JSON")
	policyPath := set.String("policy", "", "policy JSON")
	output := set.String("output", "", "manifest JSON")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *input == "" || *policyPath == "" || *output == "" {
		return fmt.Errorf("--inventory, --policy, and --output are required")
	}
	var inv model.Inventory
	var policies model.PolicyFile
	if err := report.ReadJSON(*input, &inv); err != nil {
		return err
	}
	if err := report.ReadJSON(*policyPath, &policies); err != nil {
		return err
	}
	plan, err := manifest.BuildImport(inv, policies)
	if err != nil {
		return err
	}
	return report.WriteJSON(*output, plan)
}

func runProfile(args []string) error {
	if len(args) == 0 || args[0] != "resolve" {
		return fmt.Errorf("usage: gamelib profile resolve --profile FILE --catalog DIR --output FILE")
	}
	set := flag.NewFlagSet("profile resolve", flag.ContinueOnError)
	profilePath := set.String("profile", "", "canonical profile JSON")
	catalog := set.String("catalog", "", "catalog root")
	output := set.String("output", "", "resolution JSON")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *profilePath == "" || *catalog == "" || *output == "" {
		return fmt.Errorf("--profile, --catalog, and --output are required")
	}
	value, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	resolution, err := profile.Resolve(value, *catalog)
	if err != nil {
		return err
	}
	return report.WriteJSON(*output, resolution)
}

func runBundle(args []string) error {
	if len(args) == 0 || args[0] != "plan" {
		return fmt.Errorf("usage: gamelib bundle plan --profile FILE --catalog DIR --output FILE")
	}
	set := flag.NewFlagSet("bundle plan", flag.ContinueOnError)
	profilePath := set.String("profile", "", "canonical profile JSON")
	catalog := set.String("catalog", "", "catalog root")
	output := set.String("output", "", "manifest JSON")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *profilePath == "" || *catalog == "" || *output == "" {
		return fmt.Errorf("--profile, --catalog, and --output are required")
	}
	value, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	plan, _, err := profile.BuildBundlePlan(value, *catalog)
	if err != nil {
		return err
	}
	return report.WriteJSON(*output, plan)
}

func runExport(args []string) error {
	if len(args) == 0 || args[0] != "plan" {
		return fmt.Errorf("usage: gamelib export plan --adapter steam|decky|playnite|esde|romm --profile FILE --output FILE")
	}
	set := flag.NewFlagSet("export plan", flag.ContinueOnError)
	adapter := set.String("adapter", "", "frontend adapter")
	profilePath := set.String("profile", "", "canonical profile JSON")
	output := set.String("output", "", "manifest JSON")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *adapter == "" || *profilePath == "" || *output == "" {
		return fmt.Errorf("--adapter, --profile, and --output are required")
	}
	value, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	plan, err := profile.BuildExportPlan(*adapter, value)
	if err != nil {
		return err
	}
	return report.WriteJSON(*output, plan)
}

func runValidate(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: gamelib validate profile|decky-v1|decky-catalog|inventory PATH")
	}
	switch args[0] {
	case "profile":
		_, err := profile.Load(args[1])
		return err
	case "decky-v1":
		_, err := decky.LoadAndValidate(args[1])
		return err
	case "decky-catalog":
		return decky.ValidateCatalog(args[1])
	case "inventory":
		var value model.Inventory
		if err := report.ReadJSON(args[1], &value); err != nil {
			return err
		}
		if value.Version != model.SchemaVersion {
			return fmt.Errorf("inventory version must be %d", model.SchemaVersion)
		}
		if value.Privacy != "private" && value.Privacy != "sanitized" {
			return fmt.Errorf("inventory privacy must be private or sanitized")
		}
		return nil
	default:
		return fmt.Errorf("unsupported validation target %q", args[0])
	}
}

func runManifest(args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return fmt.Errorf("usage: gamelib manifest verify --file FILE --sha256 HASH")
	}
	set := flag.NewFlagSet("manifest verify", flag.ContinueOnError)
	path := set.String("file", "", "manifest JSON")
	digest := set.String("sha256", "", "expected file SHA-256")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if *path == "" || *digest == "" {
		return fmt.Errorf("--file and --sha256 are required")
	}
	return manifest.VerifyFile(*path, *digest)
}

func twoFileFlags(name string, args []string, inputName string) (string, string, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	input := set.String(inputName, "", "input JSON")
	output := set.String("output", "", "output JSON")
	if err := set.Parse(args); err != nil {
		return "", "", err
	}
	if *input == "" || *output == "" {
		return "", "", fmt.Errorf("--%s and --output are required", inputName)
	}
	return *input, *output, nil
}

func usageError() error {
	return fmt.Errorf("usage: gamelib <inventory|report|duplicates|identity|import|profile|bundle|export|validate|manifest|version> ...")
}
