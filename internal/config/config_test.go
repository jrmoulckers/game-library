package config

import "testing"

func TestIsSafeID(t *testing.T) {
	for _, value := range []string{"profile", "deck-default", "game.1"} {
		if !IsSafeID(value) {
			t.Fatalf("expected %q to be safe", value)
		}
	}
	for _, value := range []string{"", "../profile", "Has Space", "con", "COM1"} {
		if IsSafeID(value) {
			t.Fatalf("expected %q to be unsafe", value)
		}
	}
}
