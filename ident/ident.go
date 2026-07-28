package ident

import "fmt"

// MaxIdentifierLen is the maximum length of an identifier in bytes
// (lasa-core.md §2).
const MaxIdentifierLen = 128

// MaxKeyLen is the maximum length of a state key in bytes (lasa-core.md §2).
const MaxKeyLen = 512

// ValidIdentifier reports whether s matches the identifier grammar
// [a-z0-9-]{1,128} (lasa-core.md §2). The grammar covers every identifier
// that can be embedded in a state key or namespace tuple: entity ids,
// client ids, space ids, channel names, role names, attribute names.
func ValidIdentifier(s string) bool {
	if len(s) == 0 || len(s) > MaxIdentifierLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// ValidKey reports whether s is a well-formed state key: a sequence of
// [a-z0-9.-], at most 512 bytes (lasa-core.md §2). Keys may not be empty.
func ValidKey(s string) bool {
	if len(s) == 0 || len(s) > MaxKeyLen {
		return false
	}
	return validKeyChars(s)
}

// ValidKeyPrefix reports whether s is usable as a subscription key prefix:
// like a key, but the empty string is permitted (the empty prefix
// subscribes to everything, lasa-core.md §6.3).
func ValidKeyPrefix(s string) bool {
	if len(s) > MaxKeyLen {
		return false
	}
	return validKeyChars(s)
}

func validKeyChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// CheckIdentifier returns a descriptive error when s is not a valid
// identifier.
func CheckIdentifier(s string) error {
	if !ValidIdentifier(s) {
		return fmt.Errorf("invalid identifier %q: must match [a-z0-9-]{1,%d}", s, MaxIdentifierLen)
	}
	return nil
}
