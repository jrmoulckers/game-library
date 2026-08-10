package media

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

var (
	steamName = regexp.MustCompile(`^([0-9]+)(p|_hero|_logo|_icon)?$`)
	uuidName  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

func Inspect(path, rootKind, relativePath string) (model.MediaFacts, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.MediaFacts{}, err
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return model.MediaFacts{}, err
	}
	head = head[:n]

	ext := mediaExtension(relativePath)
	mime := detectMIME(head, ext)
	facts := model.MediaFacts{
		Extension: strings.TrimPrefix(ext, "."),
		MIME:      mime,
		Role:      InferRole(rootKind, relativePath),
	}

	if strings.HasPrefix(mime, "image/") {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return facts, err
		}
		cfg, _, err := image.DecodeConfig(file)
		if err == nil {
			facts.Width = cfg.Width
			facts.Height = cfg.Height
		}
	}
	return facts, nil
}

func detectMIME(head []byte, ext string) string {
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return "application/pdf"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp"
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		return "video/mp4"
	case len(head) >= 4 && head[0] == 0 && head[1] == 0 && head[2] == 1 && head[3] == 0:
		return "image/x-icon"
	}
	mime := http.DetectContentType(head)
	if mime == "application/octet-stream" {
		switch ext {
		case ".mkv":
			return "video/x-matroska"
		case ".webm":
			return "video/webm"
		}
	}
	return mime
}

func InferRole(rootKind, relativePath string) string {
	normalized := strings.ToLower(filepath.ToSlash(relativePath))
	ext := mediaExtension(normalized)
	base := strings.TrimSuffix(filepath.Base(normalized), ext)
	parent := filepath.Base(filepath.ToSlash(filepath.Dir(normalized)))

	if role := roleFromDirectory(parent); role != "" {
		return role
	}
	switch base {
	case "logo":
		return "logo"
	case "videotrailer", "videomicrotrailer":
		return "video"
	}
	if rootKind == "playnite-library" && isImageExtension(ext) && uuidName.MatchString(base) {
		return "cover"
	}
	if isImageExtension(ext) &&
		(rootKind == "steam-grid" || rootKind == "decky-catalog" || strings.Contains(normalized, "/grid/")) {
		match := steamName.FindStringSubmatch(base)
		if len(match) == 3 {
			switch match[2] {
			case "p":
				return "portrait"
			case "_hero":
				return "hero"
			case "_logo":
				return "logo"
			case "_icon":
				return "icon"
			default:
				return "grid"
			}
		}
	}
	return ""
}

func mediaExtension(value string) string {
	name := filepath.Base(value)
	if strings.HasPrefix(name, ".") && strings.Count(name, ".") == 1 {
		return ""
	}
	return strings.ToLower(filepath.Ext(name))
}

func isImageExtension(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".ico":
		return true
	default:
		return false
	}
}

func roleFromDirectory(dir string) string {
	roles := map[string]string{
		"3dboxes":       "cover",
		"backcovers":    "cover",
		"covers":        "cover",
		"custom":        "custom",
		"fanart":        "fanart",
		"manuals":       "manual",
		"marquees":      "marquee",
		"miximages":     "miximage",
		"physicalmedia": "physicalmedia",
		"screenshots":   "screenshot",
		"titlescreens":  "titlescreen",
		"videos":        "video",
	}
	return roles[dir]
}

func InferSystem(root model.Root, relativePath string) string {
	if root.System != "" {
		return root.System
	}
	if root.Kind != "esde-media" {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	if len(parts) >= 3 {
		return strings.ToLower(parts[0])
	}
	return ""
}

func InferIdentityHint(rootKind, relativePath, system string) string {
	normalized := filepath.ToSlash(relativePath)
	base := strings.TrimSuffix(filepath.Base(normalized), filepath.Ext(normalized))
	if rootKind == "steam-grid" || rootKind == "decky-catalog" || strings.Contains(strings.ToLower(normalized), "/grid/") {
		if match := steamName.FindStringSubmatch(strings.ToLower(base)); len(match) == 3 {
			return "steam:" + match[1]
		}
	}
	parts := strings.Split(normalized, "/")
	if rootKind == "playnite-library" && uuidName.MatchString(base) {
		return "playnite:" + strings.ToLower(base)
	}
	for _, part := range parts {
		if uuidName.MatchString(part) {
			return "playnite:" + strings.ToLower(part)
		}
	}
	if rootKind == "esde-media" && system != "" {
		return "retro:" + system + ":" + slug(base)
	}
	return ""
}

func slug(value string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			dash = false
			continue
		}
		if !dash && out.Len() > 0 {
			out.WriteByte('-')
			dash = true
		}
	}
	result := strings.Trim(out.String(), "-")
	if result == "" {
		return "item"
	}
	return result
}

func DimensionKey(facts model.MediaFacts) string {
	if facts.Width == 0 || facts.Height == 0 {
		return ""
	}
	return strconv.Itoa(facts.Width) + "x" + strconv.Itoa(facts.Height)
}

func ValidateRole(facts model.MediaFacts) error {
	switch facts.Role {
	case "manual":
		if facts.MIME != "application/pdf" {
			return fmt.Errorf("manual must be PDF, got %s", facts.MIME)
		}
	case "video":
		if !strings.HasPrefix(facts.MIME, "video/") {
			return fmt.Errorf("video role has non-video MIME %s", facts.MIME)
		}
	case "":
		return nil
	default:
		if !strings.HasPrefix(facts.MIME, "image/") {
			return fmt.Errorf("%s role has non-image MIME %s", facts.Role, facts.MIME)
		}
	}
	return nil
}

func ValidateType(facts model.MediaFacts) error {
	expected := map[string]string{
		"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg",
		"gif": "image/gif", "webp": "image/webp", "ico": "image/x-icon",
		"pdf": "application/pdf", "mp4": "video/mp4",
	}
	want, ok := expected[facts.Extension]
	if !ok {
		return nil
	}
	if facts.MIME != want {
		return fmt.Errorf(".%s content has MIME %s, expected %s", facts.Extension, facts.MIME, want)
	}
	if strings.HasPrefix(want, "image/") && want != "image/webp" && want != "image/x-icon" &&
		(facts.Width < 1 || facts.Height < 1) {
		return fmt.Errorf(".%s image dimensions could not be decoded", facts.Extension)
	}
	return nil
}
