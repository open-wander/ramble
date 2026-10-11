package server

import (
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
		for _, seg := range strings.Split(r.Path, "/") {
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
