package dashboard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jrmoulckers/game-library/internal/workspace"
)

// Options configures the dashboard's HTTP handler and server. It carries no
// authentication, telemetry, or cloud configuration by design (ADR-0007):
// the process trusts the local OS user and hardens the loopback surface
// instead.
type Options struct {
	// Workspace is the resolved host-local workspace paths (config, drafts,
	// artifacts) the handler is allowed to read and write. Required.
	Workspace workspace.Paths

	// InventoryReport, when set, is the path to an existing private
	// inventory report the review surface loads instead of scanning the
	// active configuration's roots fresh on every request. Optional.
	InventoryReport string

	// CatalogRoot, when set, is the canonical catalog root the review
	// surface uses for profile resolve/bundle previews and, when no
	// explicit destinationRoot is supplied, for manifest analysis.
	// Optional: profile/bundle preview and manifest-analysis endpoints
	// report a clear error instead of guessing a root when it is unset.
	CatalogRoot string

	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

func (o Options) withDefaults() Options {
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = 5 * time.Second
	}
	if o.ReadHeaderTimeout <= 0 {
		o.ReadHeaderTimeout = 5 * time.Second
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = 10 * time.Second
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 60 * time.Second
	}
	return o
}

// NewHandler builds the dashboard's http.Handler without binding any
// network listener or blocking, so unit and integration tests can drive it
// directly (with httptest.NewServer or httptest.NewRecorder) against an
// explicit allowedHost — the exact Host/Origin value the handler will
// accept, mirroring the Host validation a real bound listener enforces. It
// also returns the in-memory CSRF token so tests can exercise
// unsafe-method requests without scraping /api/bootstrap.
func NewHandler(allowedHost string, opts Options) (handler http.Handler, csrfToken string, err error) {
	if allowedHost == "" {
		return nil, "", fmt.Errorf("allowedHost is required")
	}
	if opts.Workspace.Root == "" {
		return nil, "", fmt.Errorf("workspace paths are required")
	}
	token, err := newCSRFToken()
	if err != nil {
		return nil, "", err
	}
	h := &handlers{
		opts: opts.withDefaults(), csrfToken: token,
		snapshots: &snapshotCache{}, scans: &scanManager{}, metadata: &metadataCache{},
	}
	chain := &securityChain{allowedHost: allowedHost, csrfToken: token, next: h.mux()}
	return chain, token, nil
}

// Server is a bound dashboard HTTP server: a validated loopback listener
// plus the request handler and its middleware and timeouts.
type Server struct {
	httpServer *http.Server
	listener   net.Listener
	csrfToken  string
}

// New validates addr as an explicit loopback listener (see
// ValidateListenAddr), binds it, and builds the dashboard handler around
// the bound address. It does not start serving; call Serve to block, or run
// it in a goroutine and call Shutdown/Close for graceful or immediate
// termination.
func New(addr string, opts Options) (*Server, error) {
	listener, err := Listen(addr)
	if err != nil {
		return nil, err
	}
	handler, token, err := NewHandler(listener.Addr().String(), opts)
	if err != nil {
		listener.Close()
		return nil, err
	}
	withDefaults := opts.withDefaults()
	httpServer := &http.Server{
		Handler:           handler,
		ReadTimeout:       withDefaults.ReadTimeout,
		ReadHeaderTimeout: withDefaults.ReadHeaderTimeout,
		WriteTimeout:      withDefaults.WriteTimeout,
		IdleTimeout:       withDefaults.IdleTimeout,
	}
	return &Server{httpServer: httpServer, listener: listener, csrfToken: token}, nil
}

// Addr returns the bound loopback address, e.g. "127.0.0.1:54321". Useful
// when the caller bound to port 0 and needs the OS-assigned port.
func (s *Server) Addr() string { return s.listener.Addr().String() }

// CSRFToken returns the in-memory per-process CSRF token issued to
// bootstrap responses. It lives only in memory for the life of the process
// and is never written to disk.
func (s *Server) CSRFToken() string { return s.csrfToken }

// Serve blocks, accepting connections on the bound listener until Shutdown
// or Close is called or an unrecoverable error occurs. It returns nil after
// a graceful Shutdown/Close rather than the sentinel http.ErrServerClosed.
func (s *Server) Serve() error {
	err := s.httpServer.Serve(s.listener)
	if err != nil && err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully stops the server, waiting for in-flight requests to
// finish or ctx to expire.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Close immediately closes the underlying listener and any active
// connections without waiting for in-flight requests. Use Shutdown for
// graceful termination.
func (s *Server) Close() error {
	err := s.httpServer.Close()
	_ = s.listener.Close()
	return err
}
