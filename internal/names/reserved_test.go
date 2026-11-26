package names

import "testing"

func TestIsReserved(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"requests", true},
		{"Requests", true},
		{"ADMIN", true},
		{"  login  ", true},
		{"alice", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsReserved(tt.name); got != tt.want {
			t.Errorf("IsReserved(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
