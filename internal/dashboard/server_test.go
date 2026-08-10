package dashboard

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/workspace"
)

func TestServerServesAndShutsDownGracefully(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	srv, err := New("127.0.0.1:0", Options{Workspace: paths})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()

	resp, err := http.Get("http://" + srv.Addr() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve returned an error after graceful shutdown: %v", err)
	}
}

func TestServerRejectsNonLoopbackListen(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	if _, err := New("0.0.0.0:0", Options{Workspace: paths}); err == nil {
		t.Fatal("expected an error for a wildcard listen address")
	}
}

func TestServerCloseWithoutServeDoesNotLeakListener(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	srv, err := New("127.0.0.1:0", Options{Workspace: paths})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("unexpected error closing an unserved server: %v", err)
	}
}
