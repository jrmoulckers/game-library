package dashboard

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersArePresentOnEveryResponse(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest("GET", "/healthz"))

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, want := range checks {
		if got := rec.Header().Get(header); got != want {
			t.Fatalf("header %s = %q, want %q", header, got, want)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("expected a Content-Security-Policy header")
	}
	for _, forbidden := range []string{"unsafe-inline", "unsafe-eval"} {
		if strings.Contains(csp, forbidden) {
			t.Fatalf("CSP must not contain %q: %s", forbidden, csp)
		}
	}
}

func TestAPIResponsesAreNoStore(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest("GET", "/api/bootstrap"))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected no-store, got %q", rec.Header().Get("Cache-Control"))
	}
}

func TestHostValidationRejectsMismatchedHost(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := newRequest("GET", "/healthz")
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for mismatched Host, got %d", rec.Code)
	}
}

func TestHostValidationAcceptsExactMatch(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest("GET", "/healthz"))
	if rec.Code != 200 {
		t.Fatalf("expected 200 for matching Host, got %d", rec.Code)
	}
}

func TestSecFetchSiteRejectsCrossSite(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := newRequest("GET", "/healthz")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for cross-site Sec-Fetch-Site, got %d", rec.Code)
	}
}

func TestSecFetchSiteRejectsSameSite(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := newRequest("GET", "/healthz")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for same-site Sec-Fetch-Site, got %d", rec.Code)
	}
}

func TestSecFetchSiteAllowsNoneAndSameOrigin(t *testing.T) {
	handler, _ := newTestHandler(t)
	for _, value := range []string{"", "none", "same-origin"} {
		req := newRequest("GET", "/healthz")
		if value != "" {
			req.Header.Set("Sec-Fetch-Site", value)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("Sec-Fetch-Site=%q: expected 200, got %d", value, rec.Code)
		}
	}
}

func TestUnsafeMethodRequiresOrigin(t *testing.T) {
	handler, token := newTestHandler(t)
	req := newRequest("PUT", "/api/config")
	req.Header.Set(csrfHeader, token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for a missing Origin header, got %d", rec.Code)
	}
}

func TestUnsafeMethodRejectsCrossOrigin(t *testing.T) {
	handler, token := newTestHandler(t)
	req := newRequest("PUT", "/api/config")
	req.Header.Set(csrfHeader, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://attacker.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for a cross-origin Origin header, got %d", rec.Code)
	}
}

func TestUnsafeMethodRequiresCSRFToken(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := newRequest("PUT", "/api/config")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for a missing CSRF token, got %d", rec.Code)
	}
}

func TestUnsafeMethodRejectsWrongCSRFToken(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := newRequest("PUT", "/api/config")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, "wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for a wrong CSRF token, got %d", rec.Code)
	}
}

func TestSafeGETNeverRequiresOriginOrCSRF(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest("GET", "/api/config"))
	if rec.Code != 200 {
		t.Fatalf("expected GET to succeed without Origin/CSRF, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBodyTooLargeIsRejected(t *testing.T) {
	handler, token := newTestHandler(t)
	huge := strings.Repeat("a", maxBodyBytes+1024)
	req := newRequest("PUT", "/api/drafts/profiles/example")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, token)
	req.Body = httpBody(`{"baseDigest":"","policy":` + `"` + huge + `"}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 413 {
		t.Fatalf("expected 413 for an oversized body, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnsupportedMediaTypeIsRejected(t *testing.T) {
	handler, token := newTestHandler(t)
	req := newRequest("PUT", "/api/config")
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, token)
	req.Body = httpBody(`{}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 415 {
		t.Fatalf("expected 415 for a non-JSON content type, got %d", rec.Code)
	}
}

func TestMalformedJSONIsRejected(t *testing.T) {
	handler, token := newTestHandler(t)
	req := newRequest("PUT", "/api/config")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, token)
	req.Body = httpBody(`{not json`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("expected 400 for malformed JSON, got %d", rec.Code)
	}
}

func TestMutationRoutesDoNotExist(t *testing.T) {
	handler, token := newTestHandler(t)
	// These endpoints must never exist at all: no path even resembling
	// apply/publish/plan/prune/rollback/gate-approve is registered.
	unregistered := []struct {
		method string
		path   string
	}{
		{"POST", "/api/apply"},
		{"POST", "/api/publish"},
		{"POST", "/api/plans"},
		{"POST", "/api/prune"},
		{"POST", "/api/rollback"},
		{"POST", "/api/gates/approve"},
		{"GET", "/api/apply"},
		{"GET", "/api/publish"},
	}
	for _, c := range unregistered {
		req := newRequest(c.method, c.path)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+testHost)
		req.Header.Set(csrfHeader, token)
		req.Body = httpBody(`{}`)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Fatalf("%s %s: expected 404 (route must not exist), got %d", c.method, c.path, rec.Code)
		}
	}

	// Existing config/draft resources must not accept DELETE: the path is
	// registered for GET/PUT only, so the standard library mux correctly
	// reports 405 Method Not Allowed rather than 404, but no delete
	// handler exists either way.
	noDelete := []string{"/api/config", "/api/drafts/profiles/example"}
	for _, path := range noDelete {
		req := newRequest("DELETE", path)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+testHost)
		req.Header.Set(csrfHeader, token)
		req.Body = httpBody(`{}`)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 405 {
			t.Fatalf("DELETE %s: expected 405 (no delete handler exists), got %d", path, rec.Code)
		}
	}
}

func TestMethodNotAllowedOnRegisteredPaths(t *testing.T) {
	handler, token := newTestHandler(t)
	req := newRequest("POST", "/api/config")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, token)
	req.Body = httpBody(`{}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405 for POST on a GET/PUT-only path, got %d", rec.Code)
	}
}
