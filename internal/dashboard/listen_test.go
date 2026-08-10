package dashboard

import "testing"

func TestValidateListenAddrAcceptsExplicitLoopback(t *testing.T) {
	cases := []string{
		"127.0.0.1:8787",
		"127.0.0.1:0",
		"[::1]:8787",
		"[::1]:0",
	}
	for _, addr := range cases {
		if err := ValidateListenAddr(addr); err != nil {
			t.Fatalf("ValidateListenAddr(%q) = %v, want nil", addr, err)
		}
	}
}

func TestValidateListenAddrRejectsWildcard(t *testing.T) {
	cases := []string{
		"0.0.0.0:8787",
		"[::]:8787",
		":8787",
	}
	for _, addr := range cases {
		if err := ValidateListenAddr(addr); err == nil {
			t.Fatalf("ValidateListenAddr(%q) unexpectedly succeeded", addr)
		}
	}
}

func TestValidateListenAddrRejectsHostnames(t *testing.T) {
	cases := []string{
		"localhost:8787",
		"example.com:8787",
		"my-machine:8787",
	}
	for _, addr := range cases {
		if err := ValidateListenAddr(addr); err == nil {
			t.Fatalf("ValidateListenAddr(%q) unexpectedly succeeded", addr)
		}
	}
}

func TestValidateListenAddrRejectsLANAndPublicAddresses(t *testing.T) {
	cases := []string{
		"192.168.1.10:8787",
		"10.0.0.5:8787",
		"172.16.0.4:8787",
		"8.8.8.8:8787",
		"[2001:db8::1]:8787",
		"[fe80::1]:8787",
	}
	for _, addr := range cases {
		if err := ValidateListenAddr(addr); err == nil {
			t.Fatalf("ValidateListenAddr(%q) unexpectedly succeeded", addr)
		}
	}
}

func TestValidateListenAddrRejectsOtherLoopbackLiterals(t *testing.T) {
	// 127.0.0.2 is in the loopback /8 range but is not one of the two
	// explicit literals ADR-0007 allows.
	if err := ValidateListenAddr("127.0.0.2:8787"); err == nil {
		t.Fatal("ValidateListenAddr(127.0.0.2:8787) unexpectedly succeeded")
	}
}

func TestValidateListenAddrRejectsIPv4MappedIPv6Bypass(t *testing.T) {
	if err := ValidateListenAddr("[::ffff:127.0.0.1]:8787"); err == nil {
		t.Fatal("ValidateListenAddr should reject IPv4-mapped IPv6 spellings of loopback")
	}
}

func TestValidateListenAddrRejectsMissingPort(t *testing.T) {
	if err := ValidateListenAddr("127.0.0.1"); err == nil {
		t.Fatal("ValidateListenAddr should require an explicit port")
	}
}

func TestListenBindsAndRejectsNonLoopback(t *testing.T) {
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if listener.Addr().String() == "" {
		t.Fatal("expected a bound address")
	}

	if _, err := Listen("0.0.0.0:0"); err == nil {
		t.Fatal("Listen should reject a wildcard address")
	}
}
