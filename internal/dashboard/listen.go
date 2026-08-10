package dashboard

import (
	"fmt"
	"net"
	"strings"
)

// loopbackLiterals is the exhaustive set of loopback host literals the
// server will bind to. ADR-0007 requires an explicit loopback IP listener;
// wildcard addresses, hostnames (including "localhost", which resolves
// through the OS and can be poisoned or reconfigured), and any other
// loopback-range address (127.0.0.2, etc.) are all rejected so the bind
// behavior is exact and auditable.
var loopbackLiterals = map[string]struct{}{
	"127.0.0.1": {},
	"::1":       {},
}

// ValidateListenAddr checks that addr is a "host:port" string whose host is
// exactly 127.0.0.1 or ::1. Port 0 (meaning "let the OS choose a free
// ephemeral port") is allowed so tests and local tooling can bind without
// coordinating a fixed port. Any other host — a wildcard ("", "0.0.0.0",
// "[::]"), a hostname ("localhost", "example.com"), or a non-loopback IP
// literal (LAN or public) — is rejected.
func ValidateListenAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address %q must be host:port: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("listen address %q must not use a wildcard host; use 127.0.0.1 or ::1", addr)
	}
	if port == "" {
		return fmt.Errorf("listen address %q must include a port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("listen host %q must be an explicit loopback IP literal (127.0.0.1 or ::1), not a hostname", host)
	}
	// Compare against the canonical string form, but reject IPv4-mapped
	// IPv6 spellings such as "::ffff:127.0.0.1": net.IP.String() would
	// otherwise normalize that to "127.0.0.1" and let a non-literal form
	// through as if it were the exact address it was compared against.
	isIPv6Literal := strings.Contains(host, ":")
	if isIPv6Literal && ip.To4() != nil {
		return fmt.Errorf("listen host %q is not an accepted loopback literal; use 127.0.0.1 or ::1", host)
	}
	if _, ok := loopbackLiterals[strings.ToLower(ip.String())]; !ok {
		return fmt.Errorf("listen host %q is not an accepted loopback literal; use 127.0.0.1 or ::1", host)
	}
	return nil
}

// Listen validates addr with ValidateListenAddr and opens a TCP listener on
// it. It never falls back to a different address.
func Listen(addr string) (net.Listener, error) {
	if err := ValidateListenAddr(addr); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", addr, err)
	}
	return listener, nil
}
