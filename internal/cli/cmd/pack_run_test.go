package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackRunRejectsUnsafeRegistryVersion(t *testing.T) {
	base := t.TempDir()
	cacheHome := filepath.Join(base, "xdg")
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "tool",
			"versions": []map[string]string{
				{"version": "v1.tar.gz#/../../../../../..\x1b[31m", "url": "https://example.org/x.tar.gz"},
			},
		})
	}))
	defer srv.Close()

	out, errOut, err := runCLI(t, "", "pack", "run", "attacker/tool", "--registry", srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
	assert.NotContains(t, out+errOut+err.Error(), "\x1b")

	// Nothing may be created: not the cache, not next to it.
	entries, readErr := os.ReadDir(base)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}
