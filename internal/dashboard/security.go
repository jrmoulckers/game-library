package dashboard

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// maxBodyBytes bounds every request body the dashboard will read. It is
// generous enough for a policy/profile draft document but small enough to
// bound memory use from a misbehaving or hostile local process sharing the
// loopback trust boundary.
const maxBodyBytes = 1 << 20 // 1 MiB

// csrfHeader is the header clients must echo back on unsafe requests. The
// token is handed out over GET /api/bootstrap and never persisted to disk;
// it is regenerated every process start, matching ADR-0007's "no persistent
// bearer secret" requirement.
const csrfHeader = "X-Gamelib-Csrf"

func newCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate CSRF token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// securityChain wraps next with the loopback dashboard's defense-in-depth
// checks: strict Host validation (DNS-rebinding defense), Sec-Fetch-Site
// rejection of cross-site requests, same-origin Origin validation and CSRF
// token verification for unsafe methods, and uniform security response
// headers. Any failed check short-circuits with a sanitized JSON error and
// next is never invoked.
type securityChain struct {
	allowedHost string // exact expected Host header, e.g. "127.0.0.1:8787"
	csrfToken   string
	next        http.Handler
}

func (s *securityChain) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeSecurityHeaders(w)

	if r.Host != s.allowedHost {
		writeJSONError(w, http.StatusForbidden, "host_not_allowed", "request Host header does not match the loopback listener")
		return
	}

	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "none" && site != "same-origin" {
		writeJSONError(w, http.StatusForbidden, "cross_site_blocked", "cross-site requests are not permitted")
		return
	}

	if isUnsafeMethod(r.Method) {
		expectedOrigin := "http://" + s.allowedHost
		origin := r.Header.Get("Origin")
		if origin == "" || origin != expectedOrigin {
			writeJSONError(w, http.StatusForbidden, "origin_not_allowed", "request Origin header must match this server")
			return
		}
		token := r.Header.Get(csrfHeader)
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.csrfToken)) != 1 {
			writeJSONError(w, http.StatusForbidden, "csrf_token_invalid", "missing or invalid CSRF token")
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	s.next.ServeHTTP(w, r)
}

func writeSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-XSS-Protection", "0")
	h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), usb=()")
	h.Set("Content-Security-Policy", ""+
		"default-src 'self'; "+
		"script-src 'self'; "+
		"style-src 'self'; "+
		"img-src 'self' data:; "+
		"font-src 'self'; "+
		"connect-src 'self'; "+
		"object-src 'none'; "+
		"base-uri 'none'; "+
		"form-action 'self'; "+
		"frame-ancestors 'none'")
}

// apiError is the sanitized shape returned for every failed API request. It
// never includes filesystem paths, stack traces, or other internal detail —
// only a stable machine-readable code and a short human message.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(struct {
		Error apiError `json:"error"`
	}{apiError{Code: code, Message: message}}); err != nil {
		// The status line is already written, so the response cannot be
		// changed; the client disconnected or the write failed mid-body.
		return
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		// The status line is already written, so the response cannot be
		// changed; the client disconnected or the write failed mid-body.
		return
	}
}
