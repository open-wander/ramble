package server

import (
	"rmbl/internal/handlers"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// idLike reports whether a route parameter name denotes a database id.
func idLike(name string) bool {
	n := strings.ToLower(name)
	return n == "id" || strings.HasSuffix(n, "id")
}

// TestRegisterRoutes_IDParamsAreIntConstrained fails when any id-like path
// parameter lacks the <int> constraint, so non-numeric ids never reach a handler.
func TestRegisterRoutes_IDParamsAreIntConstrained(t *testing.T) {
	app := fiber.New()
	registerRoutes(app)

	constrained := 0
	for _, r := range app.GetRoutes() {
		for seg := range strings.SplitSeq(r.Path, "/") {
			if !strings.HasPrefix(seg, ":") {
				continue
			}
			name := strings.TrimPrefix(seg, ":")
			constraint := ""
			if i := strings.Index(name, "<"); i >= 0 {
				name, constraint = name[:i], name[i:]
			}
			name = strings.TrimRight(name, "?")
			if !idLike(name) {
				continue
			}
			if constraint != "<int>" {
				t.Errorf("%s %s: id param %q lacks <int> constraint (got %q)", r.Method, r.Path, name, constraint)
				continue
			}
			constrained++
		}
	}
	if constrained == 0 {
		t.Fatal("no <int>-constrained id routes seen; the guard is not inspecting anything")
	}
}

// TestRegisterRoutes_TopLevelSegmentsAreReserved fails when a top-level static
// route segment is not in the reserved-name list, so /:username pages cannot
// shadow or impersonate it.
func TestRegisterRoutes_TopLevelSegmentsAreReserved(t *testing.T) {
	// /api/dev is skipped in production; pin the environment.
	t.Setenv("ENV", "test")
	app := fiber.New()
	registerRoutes(app)

	seen := map[string]struct{}{}
	for _, r := range app.GetRoutes() {
		seg, _, _ := strings.Cut(strings.TrimPrefix(r.Path, "/"), "/")
		seg = strings.TrimRight(seg, "*")
		if seg == "" || strings.HasPrefix(seg, ":") {
			continue
		}
		seen[seg] = struct{}{}
		if !handlers.IsReservedName(seg) {
			t.Errorf("%s %s: top-level segment %q is not in the reserved names list", r.Method, r.Path, seg)
		}
	}
	if len(seen) < 20 {
		t.Fatalf("only %d distinct top-level segments seen; the guard is not inspecting the router", len(seen))
	}
}
