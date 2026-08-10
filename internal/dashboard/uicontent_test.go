package dashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestIndexHasCoreLandmarksAndSkipLink covers the accessibility contract
// for the rendered shell: a skip link, a banner header, a labeled
// navigation landmark, a focusable main landmark, and a footer, plus the
// two ARIA live regions every section reports through.
func TestIndexHasCoreLandmarksAndSkipLink(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()

	checks := []string{
		`class="skip-link" href="#main-content"`,
		"<header",
		`<nav class="stage-rail" aria-label="Organizer">`,
		`<main id="main-content"`,
		"<footer",
		`id="status-region"`,
		`role="status"`,
		`aria-live="polite"`,
		`id="error-region"`,
		`role="alert"`,
		`aria-live="assertive"`,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("index shell missing expected landmark/content %q", want)
		}
	}
}

// requiredSections lists every view this dashboard requires, keyed by
// its section id. TestIndexHasEverySectionWithHeadingAndCaption walks
// this list so a future removal of a required view fails loudly. The
// former review/audit stages were intentionally removed: the product is
// an artwork library, not an audit console.
var requiredSections = []string{
	"library", "platform-detail", "game-detail", "profile-library", "sources",
	"setup",
}

// TestIndexHasEverySectionWithHeadingAndCaption covers the "explicit state
// labels, dense but calm audit rails" requirement at the structural level:
// every required stage is a <section> with an aria-labelledby heading, and
// every <table> in the shell has a <caption> (a table with no caption is
// never an acceptable substitute for one, per the accessibility plan).
func TestIndexHasEverySectionWithHeadingAndCaption(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	body := rec.Body.String()

	for _, id := range requiredSections {
		sectionOpen := `id="` + id + `" aria-labelledby="` + id + `-heading"`
		if !strings.Contains(body, sectionOpen) {
			t.Fatalf("missing section %q with a matching aria-labelledby heading", id)
		}
		heading := `id="` + id + `-heading"`
		if !strings.Contains(body, heading) {
			t.Fatalf("missing heading element for section %q", id)
		}
	}

	tableOpens := strings.Count(body, "<table")
	captionOpens := strings.Count(body, "<caption")
	if tableOpens == 0 {
		t.Fatal("expected at least one <table> in the rendered shell")
	}
	if tableOpens != captionOpens {
		t.Fatalf("expected every <table> (%d) to have a matching <caption> (%d)", tableOpens, captionOpens)
	}
}

// TestIndexHasNoInlineScriptOrEventHandlers covers the CSP/no-inline-code
// constraint at the markup level: the only <script> element is the single
// same-origin module entry point, there is no inline "style=" attribute,
// and there is no inline "on<event>=" handler attribute anywhere in the
// shell (every interactive behavior is wired up from main.js via
// addEventListener instead).
func TestIndexHasNoInlineScriptOrEventHandlers(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	body := rec.Body.String()

	scriptTags := regexp.MustCompile(`<script[^>]*>`).FindAllString(body, -1)
	if len(scriptTags) != 1 {
		t.Fatalf("expected exactly one <script> element, found %d: %v", len(scriptTags), scriptTags)
	}
	if !strings.Contains(scriptTags[0], `type="module"`) || !strings.Contains(scriptTags[0], `src="/static/js/main.js"`) {
		t.Fatalf("expected the single script tag to be a same-origin module entry point, got %q", scriptTags[0])
	}

	if strings.Contains(body, "style=\"") {
		t.Fatal("index shell must not contain an inline style attribute")
	}
	if onHandler := regexp.MustCompile(`\son\w+="`).FindString(body); onHandler != "" {
		t.Fatalf("index shell must not contain an inline event handler attribute, found %q", onHandler)
	}
}

// TestStaticAssetsServeSelfOriginScriptAndStyle covers CSP static-asset
// wiring: the CSS and every JS module referenced by the shell round-trip
// through this server's own /static/ routes with a script/stylesheet MIME
// type, and the security headers on those responses still forbid
// unsafe-inline/unsafe-eval.
func TestStaticAssetsServeSelfOriginScriptAndStyle(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := fetch(t, handler, "/static/app.css")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("app.css: status = %d, content-type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "style-src 'self'") {
		t.Fatalf("expected a strict self-origin CSP on static responses, got %q", csp)
	}

	jsFilesToCheck := []string{
		"main.js", "api.js", "dom.js", "status.js", "setup.js", "organizer.js",
	}
	for _, name := range jsFilesToCheck {
		rec := fetch(t, handler, "/static/js/"+name)
		if rec.Code != 200 {
			t.Fatalf("/static/js/%s: status = %d", name, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
			t.Fatalf("/static/js/%s: content-type = %q, want a javascript type", name, rec.Header().Get("Content-Type"))
		}
	}
}

// TestStaticJSRejectsUnknownName covers path-safety for the single-segment
// static JS route: an unknown name is a clean 404. A traversal-shaped name
// such as ".." never reaches the handler at all — the standard library
// mux normalizes the request path first and issues a redirect instead —
// so neither outcome ever serves unexpected content.
func TestStaticJSRejectsUnknownName(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := fetch(t, handler, "/static/js/does-not-exist.js")
	if rec.Code != 404 {
		t.Fatalf("/static/js/does-not-exist.js: status = %d, want 404", rec.Code)
	}

	for _, name := range []string{"..", "%2e%2e"} {
		rec := fetch(t, handler, "/static/js/"+name)
		if rec.Code != 404 && rec.Code != 301 && rec.Code != 307 {
			t.Fatalf("/static/js/%s: status = %d, want 404 or a path-cleaning redirect", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "package dashboard") {
			t.Fatalf("/static/js/%s: response unexpectedly served repository source", name)
		}
	}
}

// requiredCopyBoundaries lists literal phrases the accepted plan requires
// to be visible in the rendered shell, expressed as substrings so wording
// tweaks that keep the meaning intact do not need to touch this test.
// requiredCopyBoundaries are the safety statements that must remain
// visible in the shell. The plan/draft/apply-gate vocabulary was
// intentionally dropped along with the audit console; what still matters
// is that the user is told this tool reads their libraries without
// changing them.
var requiredCopyBoundaries = []string{
	"Read-only",
	"no changes are made to your libraries",
	"canonical",
	"homelab",
}

// TestIndexStatesRequiredCopyBoundaries covers the product plan's
// "explicit state labels" and "never imply canonical promotion / apply
// availability" requirements at the copy level: every one of these
// boundary phrases must actually be present in the rendered shell, not
// just intended.
func TestIndexStatesRequiredCopyBoundaries(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	body := rec.Body.String()
	for _, phrase := range requiredCopyBoundaries {
		if !strings.Contains(body, phrase) {
			t.Fatalf("index shell missing required copy boundary %q", phrase)
		}
	}
}

// TestIndexHasNoApplyOrPublishControl covers the "Apply must be visibly
// unavailable and never wired" requirement at the markup level: no button,
// link, or form in the shell is labeled to apply, publish, execute,
// prune, delete, or roll back anything.
func TestIndexHasNoApplyOrPublishControl(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	body := strings.ToLower(rec.Body.String())
	forbidden := []string{
		">apply<", ">publish<", ">execute<", ">prune<", ">delete<", ">roll back<", ">rollback<",
	}
	for _, phrase := range forbidden {
		if strings.Contains(body, phrase) {
			t.Fatalf("index shell must not contain a control labeled %q", phrase)
		}
	}
}

// TestFormControlsHaveMatchingLabels spot-checks a representative sample
// of interactive controls across different sections to catch a
// label/for-id mismatch class of regression, without hand-maintaining an
// exhaustive list of every input on the page.
func TestFormControlsHaveMatchingLabels(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	body := rec.Body.String()

	pairs := []string{
		"setup-default-mode",
		"game-search",
		"game-coverage",
		"game-role",
		"game-source",
		"game-sort",
		"profile-new-name",
	}
	for _, id := range pairs {
		if !strings.Contains(body, `for="`+id+`"`) {
			t.Fatalf("expected a <label for=%q>", id)
		}
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Fatalf("expected a control with id=%q", id)
		}
	}
}

// TestJavaScriptModulesAreSyntacticallyValid provides the "lightweight JS
// syntax validation using an existing runtime" the plan calls for: if a
// `node` binary is available on the host, every embedded ES module is
// parsed (not executed) with `node --input-type=module --check`, which
// reliably surfaces a SyntaxError for import/export-bearing source (a
// bare `node --check file.js` does not: see the accompanying PR
// description). No new dependency is introduced; the test simply skips if
// node is not on PATH.
func TestJavaScriptModulesAreSyntacticallyValid(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available on PATH; skipping JS syntax validation")
	}

	entries, err := jsFiles.ReadDir("static/js")
	if err != nil {
		t.Fatalf("failed to list embedded JS files: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one embedded JS file")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		data, err := jsFiles.ReadFile("static/js/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		cmd := exec.Command(nodePath, "--input-type=module", "--check")
		cmd.Stdin = strings.NewReader(string(data))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("static/js/%s failed ES module syntax check: %v\n%s", entry.Name(), err, out)
		}
	}
}

// TestJavaScriptModulesAvoidUnsafeConstructs is a defense-in-depth scan
// (independent of the CSP, which already forbids unsafe-eval) ensuring no
// embedded module uses eval, the Function constructor, or innerHTML
// assignment, matching the "no eval" requirement and this project's
// practice of building DOM nodes with textContent rather than markup
// concatenation.
func TestJavaScriptModulesAvoidUnsafeConstructs(t *testing.T) {
	entries, err := jsFiles.ReadDir("static/js")
	if err != nil {
		t.Fatalf("failed to list embedded JS files: %v", err)
	}
	forbidden := []string{"eval(", "new Function(", ".innerHTML", "document.write("}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		data, err := jsFiles.ReadFile("static/js/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		content := string(data)
		for _, pattern := range forbidden {
			if strings.Contains(content, pattern) {
				t.Fatalf("static/js/%s contains forbidden construct %q", entry.Name(), pattern)
			}
		}
	}
}
