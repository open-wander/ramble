package names

import (
	"strings"
	"testing"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"simple", "alice", true},
		{"mixed case", "AliceCorp", true},
		{"digit first", "9lives", true},
		{"dots dashes underscores", "a.b-c_d", true},
		{"single char", "x", true},
		{"64 chars", strings.Repeat("a", 64), true},
		{"empty", "", false},
		{"65 chars", strings.Repeat("a", 65), false},
		{"leading dot", ".x", false},
		{"leading dash", "-x", false},
		{"leading underscore", "_x", false},
		{"space", "a b", false},
		{"slash", "a/b", false},
		{"unicode", "ü", false},
		{"unicode inside", "aüb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValid(tt.in); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"semver v", "v1.2.3", true},
		{"semver", "1.0.0", true},
		{"prerelease", "v1.0.0-rc.1", true},
		{"date release", "release-2024.01", true},
		{"build metadata", "1.0.0+build.5", true},
		{"underscore", "v1_0", true},
		{"single char", "1", true},
		{"64 chars", strings.Repeat("v", 64), true},
		{"empty", "", false},
		{"65 chars", strings.Repeat("v", 65), false},
		{"fragment traversal", "v1.tar.gz#/../../../../../..", false},
		{"dotdot slash", "../x", false},
		{"dotdot only", "..", false},
		{"dot only", ".", false},
		{"absolute", "/abs", false},
		{"embedded dotdot", "a/../../b", false},
		{"slash tag", "release/v1", false},
		{"double slash", "a//b", false},
		{"backslash", "..\\x", false},
		{"leading backslash", "\\x", false},
		{"nul", "v1\x00", false},
		{"escape", "v1\x1b[0m", false},
		{"newline", "v1\n", false},
		{"percent encoded", "%2e%2e", false},
		{"query", "v1?x=1", false},
		{"hash", "v1#x", false},
		{"space", "v 1", false},
		{"leading dot", ".hidden", false},
		{"leading dash", "-v1", false},
		{"unicode", "v1ü", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidVersion(tt.in); got != tt.want {
				t.Errorf("IsValidVersion(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
