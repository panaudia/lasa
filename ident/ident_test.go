package ident

import (
	"strings"
	"testing"
)

func TestValidIdentifier(t *testing.T) {
	valid := []string{
		"a", "0", "-", "abc-123", strings.Repeat("a", 128),
		"550e8400-e29b-41d4-a716-446655440000", // lowercase UUID
	}
	for _, s := range valid {
		if !ValidIdentifier(s) {
			t.Errorf("ValidIdentifier(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"", strings.Repeat("a", 129),
		"A", "a.b", "a_b", "a b", "a/b", "é", "a\x00b",
		"550E8400-E29B-41D4-A716-446655440000", // uppercase UUID
	}
	for _, s := range invalid {
		if ValidIdentifier(s) {
			t.Errorf("ValidIdentifier(%q) = true, want false", s)
		}
	}
}

func TestValidKey(t *testing.T) {
	valid := []string{
		"a", "lasa.entity.e1.name", "a.b-c.0", strings.Repeat("k", 512),
		"...", "-", // dots and hyphens are plain characters; structure is convention
	}
	for _, s := range valid {
		if !ValidKey(s) {
			t.Errorf("ValidKey(%q) = false, want true", s)
		}
	}
	invalid := []string{"", strings.Repeat("k", 513), "A.b", "a_b", "a b", "a/b"}
	for _, s := range invalid {
		if ValidKey(s) {
			t.Errorf("ValidKey(%q) = true, want false", s)
		}
	}
}

func TestValidKeyPrefix(t *testing.T) {
	if !ValidKeyPrefix("") {
		t.Error("empty prefix must be valid (subscribes to everything)")
	}
	if !ValidKeyPrefix("lasa.") {
		t.Error(`ValidKeyPrefix("lasa.") = false, want true`)
	}
	if ValidKeyPrefix(strings.Repeat("k", 513)) {
		t.Error("overlong prefix must be invalid")
	}
	if ValidKeyPrefix("A") {
		t.Error("uppercase prefix must be invalid")
	}
}
