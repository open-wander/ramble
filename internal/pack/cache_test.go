package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRegistry = "https://registry.example.org"

func TestPackPathRejectsUnsafeComponents(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "cache")}

	badVersions := []string{
		"v1.tar.gz#/../../../../../..",
		"../x",
		"/abs",
		"a/../../b",
		`..\x`,
		"v1\x00",
		"v1\x1b[31m",
		"%2e%2e",
		"..",
		"",
	}
	for _, v := range badVersions {
		t.Run("version "+Printable(v), func(t *testing.T) {
			p, err := c.PackPath(testRegistry, "alice", "tool", v)
			if err == nil {
				t.Fatalf("PackPath accepted version %q, returned %q", v, p)
			}
			if strings.ContainsAny(err.Error(), "\x00\x1b") {
				t.Errorf("error leaks control characters: %q", err.Error())
			}
			if !strings.Contains(err.Error(), "version") {
				t.Errorf("error does not name the field: %v", err)
			}
		})
	}

	badSegments := []struct {
		field, ns, name, registry string
	}{
		{"namespace", "..", "tool", testRegistry},
		{"namespace", "a/b", "tool", testRegistry},
		{"namespace", "", "tool", testRegistry},
		{"namespace", `a\b`, "tool", testRegistry},
		{"namespace", "a\x00", "tool", testRegistry},
		{"name", "alice", "../../x", testRegistry},
		{"name", "alice", "", testRegistry},
		{"name", "alice", ".", testRegistry},
		{"host", "alice", "tool", "https://.."},
		{"host", "alice", "tool", ""},
		{"namespace", strings.Repeat("a", 300), "tool", testRegistry},
	}
	for _, tt := range badSegments {
		t.Run(tt.field+" "+Printable(tt.ns+tt.name+tt.registry), func(t *testing.T) {
			p, err := c.PackPath(tt.registry, tt.ns, tt.name, "v1.0.0")
			if err == nil {
				t.Fatalf("PackPath accepted %+v, returned %q", tt, p)
			}
		})
	}
}

func TestPackPathGood(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "cache")}
	got, err := c.PackPath("http://localhost:8080", "alice", "tool", "v1.0.0-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(c.Dir, "localhost_8080", "alice", "tool", "v1.0.0-rc.1")
	if got != want {
		t.Errorf("PackPath = %q, want %q", got, want)
	}
}

func TestCacheReadersRejectBadVersion(t *testing.T) {
	base := t.TempDir()
	c := &Cache{Dir: filepath.Join(base, "cache")}
	// A directory outside the cache that looks like a pack.
	if err := os.WriteFile(filepath.Join(base, "metadata.hcl"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c.IsCached(testRegistry, "alice", "tool", "../../..") {
		t.Error("IsCached true for traversal version")
	}
	if _, err := c.Load(testRegistry, "alice", "tool", "../../.."); err == nil {
		t.Error("Load accepted traversal version")
	}
}

func TestStoreBadRefWritesNothing(t *testing.T) {
	base := t.TempDir()
	c := &Cache{Dir: filepath.Join(base, "cache")}
	_, err := c.Store(testRegistry, "alice", "tool", "v1.tar.gz#/../../../../../..", "https://example.org/x.tar.gz")
	if err == nil {
		t.Fatal("Store accepted a bad version")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Store created %d entries before validating", len(entries))
	}
}

type tarEntry struct {
	name     string
	typeflag byte
	body     string
	link     string
}

func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: 0o644, Linkname: e.link}
		if e.typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if e.typeflag == tar.TypeDir {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// extractInto runs extractTarGz into <base>/cache/<rel> and returns base.
func extractInto(t *testing.T, data []byte) (base string, err error) {
	t.Helper()
	base = t.TempDir()
	cacheDir := filepath.Join(base, "cache")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	rel := filepath.Join("h", "ns", "n", "v1")
	if err := root.MkdirAll(rel, 0o750); err != nil {
		t.Fatal(err)
	}
	return base, extractTarGz(bytes.NewReader(data), root, rel)
}

func TestExtractTarGzHappyPath(t *testing.T) {
	data := buildTarGz(t, []tarEntry{
		{name: "root/", typeflag: tar.TypeDir},
		{name: "root/metadata.hcl", typeflag: tar.TypeReg, body: "meta"},
		{name: "root/sub/", typeflag: tar.TypeDir},
		{name: "root/sub/f.txt", typeflag: tar.TypeReg, body: "hello"},
	})
	base, err := extractInto(t, data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(base, "cache", "h", "ns", "n", "v1", "sub", "f.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("read = %q, %v", got, err)
	}
}

func TestExtractTarGzRejectsUnsafeNames(t *testing.T) {
	tests := []struct {
		name  string
		entry tarEntry
		first bool
	}{
		{"parent traversal", tarEntry{name: "root/../evil", typeflag: tar.TypeReg, body: "x"}, false},
		{"deep traversal", tarEntry{name: "root/../../../evil", typeflag: tar.TypeReg, body: "x"}, false},
		{"absolute", tarEntry{name: "/abs/evil", typeflag: tar.TypeReg, body: "x"}, false},
		{"traversal as first entry", tarEntry{name: "../evil", typeflag: tar.TypeReg, body: "x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := []tarEntry{{name: "root/", typeflag: tar.TypeDir}, tt.entry}
			if tt.first {
				entries = []tarEntry{tt.entry}
			}
			data := buildTarGz(t, entries)
			base, err := extractInto(t, data)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, p := range []string{filepath.Join(base, "evil"), filepath.Join(base, "cache", "evil"), "/abs/evil"} {
				if _, statErr := os.Lstat(p); statErr == nil {
					t.Errorf("file written at %s", p)
				}
			}
		})
	}
}

func TestExtractTarGzSkipsLinks(t *testing.T) {
	outside := t.TempDir()
	data := buildTarGz(t, []tarEntry{
		{name: "root/", typeflag: tar.TypeDir},
		{name: "root/link", typeflag: tar.TypeSymlink, link: outside},
		{name: "root/link/x", typeflag: tar.TypeReg, body: "pwned"},
		{name: "root/hard", typeflag: tar.TypeLink, link: "root/metadata.hcl"},
		{name: "root/ok.txt", typeflag: tar.TypeReg, body: "ok"},
	})
	base, err := extractInto(t, data)
	// Either an error or a skip is acceptable; nothing may escape.
	_ = err
	if _, statErr := os.Lstat(filepath.Join(outside, "x")); statErr == nil {
		t.Error("file written through symlink")
	}
	dest := filepath.Join(base, "cache", "h", "ns", "n", "v1")
	if fi, statErr := os.Lstat(filepath.Join(dest, "link")); statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
		t.Error("symlink was created")
	}
	if _, statErr := os.Lstat(filepath.Join(dest, "hard")); statErr == nil {
		t.Error("hardlink was created")
	}
}
