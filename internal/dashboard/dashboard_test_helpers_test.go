package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/workspace"
)

const testHost = "127.0.0.1:8787"

func newTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	paths := workspace.NewPaths(t.TempDir())
	handler, token, err := NewHandler(testHost, Options{Workspace: paths})
	if err != nil {
		t.Fatal(err)
	}
	return handler, token
}

// newTestHandlerWithOptions is like newTestHandler but lets a test supply
// additional Options fields (InventoryReport, CatalogRoot) while still
// getting a fresh temp workspace and the standard test host/token wiring.
func newTestHandlerWithOptions(t *testing.T, opts Options) (http.Handler, string, workspace.Paths) {
	t.Helper()
	paths := workspace.NewPaths(t.TempDir())
	opts.Workspace = paths
	handler, token, err := NewHandler(testHost, opts)
	if err != nil {
		t.Fatal(err)
	}
	return handler, token, paths
}

func newRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Host = testHost
	return req
}

func httpBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}
