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
