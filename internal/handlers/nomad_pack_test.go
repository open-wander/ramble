package handlers

import "testing"

func TestGetDownloadURLEscapesVersion(t *testing.T) {
	tests := []struct {
		name, repo, version, want string
	}{
		{"github plain", "https://github.com/o/r", "v1.0.0", "https://github.com/o/r/archive/refs/tags/v1.0.0.tar.gz"},
		{"github plus kept", "https://github.com/o/r.git", "1.0.0+b1", "https://github.com/o/r/archive/refs/tags/1.0.0+b1.tar.gz"},
		{"github fragment", "https://github.com/o/r", "v1#x", "https://github.com/o/r/archive/refs/tags/v1%23x.tar.gz"},
		{"github query", "https://github.com/o/r", "v1?x", "https://github.com/o/r/archive/refs/tags/v1%3Fx.tar.gz"},
		{"github slash", "https://github.com/o/r", "../x", "https://github.com/o/r/archive/refs/tags/..%2Fx.tar.gz"},
		{"gitlab plain", "https://gitlab.com/o/r", "v1.0.0", "https://gitlab.com/o/r/-/archive/v1.0.0/r-v1.0.0.tar.gz"},
		{"gitlab fragment", "https://gitlab.com/o/r", "v1#x", "https://gitlab.com/o/r/-/archive/v1%23x/r-v1%23x.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDownloadURL(tt.repo, tt.version); got != tt.want {
				t.Errorf("getDownloadURL = %q, want %q", got, tt.want)
			}
		})
	}
}
