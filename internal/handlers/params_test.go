package handlers

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/models"
)

func TestParamID(t *testing.T) {
	tests := []struct {
		in      string
		want    uint
		wantErr bool
	}{
		{in: "1", want: 1},
		{in: "42", want: 42},
		{in: "0", wantErr: true},
		{in: "-1", wantErr: true},
		{in: "+1", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "1 or 1=1", wantErr: true},
		{in: "id>0", wantErr: true},
		{in: "18446744073709551616", wantErr: true}, // overflows uint64
		{in: "", wantErr: true},
		{in: " 1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseID(tt.in)
			if tt.wantErr {
				assert.ErrorIs(t, err, errInvalidID)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	// paramID reads the named route parameter. Drive fasthttp directly so the
	// payload reaches Params unescaped.
	for in, want := range map[string]uint{"7": 7, "id>0": 0} {
		app := fiber.New()
		var got uint
		called := false
		app.Get("/x/:id", func(c *fiber.Ctx) error {
			called = true
			got, _ = paramID(c, "id")
			return nil
		})
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.SetRequestURI("/x/" + in)
		app.Handler()(ctx)
		require.True(t, called, "handler not reached for %q", in)
		assert.Equal(t, want, got, "paramID for %q", in)
	}
	// An unset parameter name yields "" and must be rejected.
	app := fiber.New()
	var err error
	app.Get("/x", func(c *fiber.Ctx) error { _, err = paramID(c, "id"); return nil })
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/x")
	app.Handler()(ctx)
	assert.ErrorIs(t, err, errInvalidID)
}

// idParamName reports whether a route parameter name denotes a database id.
func idParamName(name string) bool {
	n := strings.ToLower(name)
	return n == "id" || strings.HasSuffix(n, "id")
}

// scanParamsCalls returns violations: calls to Params/ParamsInt/AllParams in
// non-test files under dirs whose name argument is an id-like literal or is not
// a literal at all, outside the function allowed. It also returns the number
// of Params-family calls inspected so callers can assert the walk was not vacuous.
func scanParamsCalls(t *testing.T, allowed string, dirs ...string) (int, []string) {
	t.Helper()
	fset := token.NewFileSet()
	var bad []string
	checked := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					switch sel.Sel.Name {
					case "Params", "ParamsInt", "AllParams":
					default:
						return true
					}
					checked++
					where := fmt.Sprintf("%s:%d", path, fset.Position(call.Pos()).Line)
					if sel.Sel.Name == "AllParams" {
						bad = append(bad, where+": AllParams hands every param to the caller; use paramID")
						return true
					}
					if len(call.Args) == 0 {
						bad = append(bad, where+": "+sel.Sel.Name+" without a name")
						return true
					}
					lit, ok := call.Args[0].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						if fd.Name.Name != allowed {
							bad = append(bad, where+": "+sel.Sel.Name+" with a non-literal name cannot be checked")
						}
						return true
					}
					name, _ := strconv.Unquote(lit.Value)
					if sel.Sel.Name == "ParamsInt" || idParamName(name) {
						if fd.Name.Name != allowed {
							bad = append(bad, where+": "+sel.Sel.Name+"(\""+name+"\") outside "+allowed+"; use paramID")
						}
					}
					return true
				})
			}
			return nil
		})
		require.NoError(t, err)
	}
	return checked, bad
}

// TestParamsIDOnlyInParamID fails when a handler reads an id-like path
// parameter without going through paramID.
func TestParamsIDOnlyInParamID(t *testing.T) {
	checked, bad := scanParamsCalls(t, "paramID", ".", "../server")
	assert.Greater(t, checked, 15, "scan looks vacuous: only %d Params calls seen", checked)
	assert.Empty(t, bad, strings.Join(bad, "\n"))
}

// TestScanParamsCalls_Detects proves the scanner flags each bypass form.
func TestScanParamsCalls_Detects(t *testing.T) {
	dir := t.TempDir()
	src := `package x
import "github.com/gofiber/fiber/v2"
func a(c *fiber.Ctx) { _ = c.Params("id") }
func b(c *fiber.Ctx) { _ = c.Params("member_id") }
func c2(c *fiber.Ctx) { _ = c.Params("OrgID") }
func d(c *fiber.Ctx, n string) { _ = c.Params(n) }
func e(c *fiber.Ctx) { _, _ = c.ParamsInt("page") }
func f(c *fiber.Ctx) { _ = c.AllParams() }
func g(c *fiber.Ctx) { _ = c.Params("username") }
func paramID(c *fiber.Ctx) { _ = c.Params("id") }
`
	require.NoError(t, writeFile(filepath.Join(dir, "x.go"), src))
	checked, bad := scanParamsCalls(t, "paramID", dir)
	assert.Equal(t, 8, checked)
	assert.Len(t, bad, 6, strings.Join(bad, "\n"))
}

// injectionPayloads are values that reach c.Params when route constraints are
// bypassed or accepted (-1, +1 pass Fiber's <int>).
var injectionPayloads = []string{
	"id>0",
	"1 or 1=1",
	"(select(pg_sleep(0)))is(null)",
	"-1",
	"+1",
	"abc",
}

// TestIDHandlers_RejectInjectionPayloads registers formerly vulnerable handlers
// on plain :id routes (no <int> constraint), seeds rows so the vulnerable code
// would find one, and asserts a 404 and that the payload never appears in any
// logged SQL. The GORM logger runs at a 1ns threshold: threshold 0 disables
// slow-query logging and a successful injected query would go unseen.
func TestIDHandlers_RejectInjectionPayloads(t *testing.T) {
	defer cleanupTestData(t)
	InitSession()

	admin := createTestUser(t, "injadmin")
	member := createTestUser(t, "injmember")
	org := models.Organization{Name: "injorg", Description: "d"}
	require.NoError(t, database.DB.Create(&org).Error)
	ms := models.Membership{UserID: member.ID, OrganizationID: org.ID, Role: "member"}
	require.NoError(t, database.DB.Create(&ms).Error)
	preq := models.PackRequest{Title: "inj", Description: "d", UserID: admin.ID}
	require.NoError(t, database.DB.Create(&preq).Error)
	res := createTestPack(t, admin.ID, "inj-pack")
	require.NoError(t, database.DB.Model(&res).Update("repository_url", failFastRepoURL).Error)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM memberships WHERE organization_id = ?", org.ID)
		database.DB.Exec("DELETE FROM organizations WHERE id = ?", org.ID)
		database.DB.Exec("DELETE FROM request_votes")
		database.DB.Exec("DELETE FROM pack_requests WHERE id = ?", preq.ID)
	})

	var buf lockedBuffer
	base := zerolog.New(&buf)
	orig := database.DB
	database.DB = orig.Session(&gorm.Session{Logger: database.NewGormLogger(&base, time.Nanosecond)})
	t.Cleanup(func() { database.DB = orig })

	adminUser := models.User{ID: admin.ID, Username: admin.Username, IsAdmin: true}
	idOf := func(id uint) string { return strconv.FormatUint(uint64(id), 10) }

	// bad is the payload; each case lists URLs with the payload in one param
	// position and valid ids elsewhere.
	cases := []struct {
		name    string
		method  string
		route   string
		handler fiber.Handler
		urls    func(bad string) []string
	}{
		{"GetRequest", "GET", "/requests/:id", GetRequest,
			func(b string) []string { return []string{"/requests/" + b} }},
		{"FetchReadme", "GET", "/resource/:id/fetch-readme", FetchReadme,
			func(b string) []string { return []string{"/resource/" + b + "/fetch-readme"} }},
		{"PostToggleAdmin", "POST", "/admin/users/:id/toggle-admin", PostToggleAdmin,
			func(b string) []string { return []string{"/admin/users/" + b + "/toggle-admin"} }},
		{"GetEditOrganization", "GET", "/admin/organizations/:id/edit", GetEditOrganization,
			func(b string) []string { return []string{"/admin/organizations/" + b + "/edit"} }},
		{"DeleteRequest", "DELETE", "/admin/requests/:id", DeleteRequest,
			func(b string) []string { return []string{"/admin/requests/" + b} }},
		{"PostAdminRemoveMember", "DELETE", "/admin/organizations/:id/members/:member_id", PostAdminRemoveMember,
			func(b string) []string {
				return []string{
					"/admin/organizations/" + b + "/members/" + idOf(ms.ID),
					"/admin/organizations/" + idOf(org.ID) + "/members/" + b,
				}
			}},
		{"PostAdminChangeMemberRole", "POST", "/admin/organizations/:id/members/:member_id/role", PostAdminChangeMemberRole,
			func(b string) []string {
				return []string{
					"/admin/organizations/" + b + "/members/" + idOf(ms.ID) + "/role",
					"/admin/organizations/" + idOf(org.ID) + "/members/" + b + "/role",
				}
			}},
		{"DeleteAdminUserMembership", "DELETE", "/admin/users/:id/memberships/:org_id", DeleteAdminUserMembership,
			func(b string) []string {
				return []string{
					"/admin/users/" + b + "/memberships/" + idOf(org.ID),
					"/admin/users/" + idOf(member.ID) + "/memberships/" + b,
				}
			}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			var seen []string
			app.Add(tc.method, tc.route, func(c *fiber.Ctx) error {
				c.Locals("User", adminUser)
				c.Locals("UserID", admin.ID)
				for _, k := range []string{"id", "member_id", "org_id"} {
					if v := c.Params(k); v != "" {
						seen = append(seen, v)
					}
				}
				return c.Next()
			}, tc.handler)

			for _, bad := range injectionPayloads {
				for _, u := range tc.urls(bad) {
					seen = nil
					buf.mu.Lock()
					buf.buf.Reset()
					buf.mu.Unlock()

					ctx := &fasthttp.RequestCtx{}
					ctx.Request.Header.SetMethod(tc.method)
					ctx.Request.SetRequestURI(u)
					app.Handler()(ctx)

					assert.Contains(t, seen, bad, "handler did not receive the raw payload for %s", u)
					assert.Equal(t, 404, ctx.Response.StatusCode(), "%s %s: %s", tc.method, u, ctx.Response.Body())
					for line := range strings.SplitSeq(buf.String(), "\n") {
						var m map[string]any
						if json.Unmarshal([]byte(line), &m) != nil {
							continue
						}
						msg, _ := m[zerolog.MessageFieldName].(string)
						sql, _ := m["sql"].(string)
						if strings.HasPrefix(msg, "db.") && strings.Contains(sql, bad) {
							t.Errorf("payload %q reached SQL for %s: %s", bad, u, line)
						}
					}
				}
			}
		})
	}

	// Positive control: the capture really sees queries.
	t.Run("control", func(t *testing.T) {
		app := fiber.New()
		app.Get("/requests/:id", GetRequest)
		buf.mu.Lock()
		buf.buf.Reset()
		buf.mu.Unlock()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/requests/" + idOf(preq.ID))
		app.Handler()(ctx)
		assert.NotNil(t, buf.findLogLine("db.slow_query", "pack_requests"), "logger captured nothing: %s", buf.String())
		assert.NotEqual(t, 404, ctx.Response.StatusCode())
	})
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// TestIDHandlers_BadIDIs404BeforeAnyWork covers handlers that used to ignore a
// parse error and continue with id 0 (PostNewVersion, PostEditResource,
// PostRemoveMember) plus the other resource handlers. No session or database
// state is set up: a bad id must 404 before either is touched.
func TestIDHandlers_BadIDIs404BeforeAnyWork(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		route   string
		urlFmt  string
		handler fiber.Handler
	}{
		{"GetNewVersion", "GET", "/resource/:id/new-version", "/resource/%s/new-version", GetNewVersion},
		{"PostNewVersion", "POST", "/resource/:id/version", "/resource/%s/version", PostNewVersion},
		{"PostEditResource", "POST", "/resource/:id/edit", "/resource/%s/edit", PostEditResource},
		{"DeleteResource", "DELETE", "/resource/:id", "/resource/%s", DeleteResource},
		{"PostRetryFetch", "POST", "/resource/:id/retry-fetch", "/resource/%s/retry-fetch", PostRetryFetch},
		{"PostResetWebhookSecret", "POST", "/resource/:id/webhook/reset", "/resource/%s/webhook/reset", PostResetWebhookSecret},
		{"ToggleStar", "POST", "/resource/:id/star", "/resource/%s/star", ToggleStar},
		{"HandleWebhook", "POST", "/resource/:id/webhook", "/resource/%s/webhook", HandleWebhook},
		{"PostRemoveMember", "POST", "/orgs/:orgname/members/:member_id/remove", "/orgs/o/members/%s/remove", PostRemoveMember},
	}
	for _, tc := range cases {
		for _, bad := range []string{"abc", "0", "-1", "id>0"} {
			t.Run(tc.name+"/"+bad, func(t *testing.T) {
				app := fiber.New()
				app.Add(tc.method, tc.route, func(c *fiber.Ctx) error {
					c.Locals("OrgID", uint(1))
					return c.Next()
				}, tc.handler)
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(tc.method)
				ctx.Request.SetRequestURI(fmt.Sprintf(tc.urlFmt, bad))
				app.Handler()(ctx)
				assert.Equal(t, 404, ctx.Response.StatusCode(), string(ctx.Response.Body()))
			})
		}
	}
}
