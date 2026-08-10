package dashboard

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/jrmoulckers/game-library/internal/model"
)

//go:embed static/index.html.tmpl
var indexTemplateSource embed.FS

//go:embed static/app.css
var appCSS embed.FS

//go:embed static/js
var jsFiles embed.FS

var indexTemplate = template.Must(template.ParseFS(indexTemplateSource, "static/index.html.tmpl"))

type indexView struct {
	ToolVersion string
}

func (h *handlers) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, indexView{ToolVersion: model.ToolVersion}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to render the dashboard shell")
	}
}

func (h *handlers) staticCSS(w http.ResponseWriter, r *http.Request) {
	data, err := appCSS.ReadFile("static/app.css")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read a static asset")
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

// staticJS serves one embedded ES module file by its exact base name (the
// mux pattern "/static/js/{name}" only ever captures a single path
// segment, so a "name" containing a "/" cannot even reach this handler).
// jsFiles is an embed.FS containing only the files this package built into
// the binary; there is no way for a request to make it read anything from
// the real filesystem, so no path-traversal check is needed beyond what
// io/fs already enforces (fs.ValidPath rejects "." and ".." elements).
func (h *handlers) staticJS(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := jsFiles.ReadFile("static/js/" + name)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "no such static asset")
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}
