package security

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// runtimeStage returns the final Dockerfile stage as one string.
func runtimeStage(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	var stage []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "FROM ") {
			stage = nil
		}
		stage = append(stage, l)
	}
	return strings.Join(stage, "\n")
}

// TestDockerfileRuntimeRunsNonRoot guards the hardening of the production image:
// fixed numeric unprivileged user, /app workdir, and root-owned (read-only) assets.
func TestDockerfileRuntimeRunsNonRoot(t *testing.T) {
	text := runtimeStage(t)

	if !regexp.MustCompile(`(?m)^USER 10001:10001\s*$`).MatchString(text) {
		t.Error("runtime stage must set USER 10001:10001")
	}
	if !regexp.MustCompile(`(?m)^WORKDIR /app\s*$`).MatchString(text) {
		t.Error("runtime stage must use WORKDIR /app")
	}
	if strings.Contains(text, "/root") {
		t.Error("runtime stage must not reference /root")
	}
	if !strings.Contains(text, "ca-certificates") {
		t.Error("runtime stage must keep ca-certificates")
	}
	if !strings.Contains(text, "adduser") || !strings.Contains(text, "-u 10001") {
		t.Error("runtime stage must create uid 10001")
	}
	if strings.Contains(text, "--chown") {
		t.Error("assets must stay root-owned; do not use --chown")
	}
}
