package logger

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var eventKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

// scanMsgKeys walks roots and returns the number of zerolog Msg/Msgf calls
// inspected plus a list of violations (file:line description).
func scanMsgKeys(t *testing.T, roots ...string) (int, []string) {
	t.Helper()
	var checked int
	var bad []string
	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(path)
			if d.IsDir() {
				// The CLI is a user-facing terminal tool, not the server.
				if d.Name() == "testdata" || d.Name() == "cli" && strings.Contains(slash, "internal/cli") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "Msg" && sel.Sel.Name != "Msgf") {
					return true
				}
				checked++
				pos := fset.Position(call.Pos())
				where := pos.Filename + ":" + strconv.Itoa(pos.Line)
				if sel.Sel.Name == "Msgf" {
					bad = append(bad, where+": Msgf is not allowed; use Msg with fields")
					return true
				}
				if len(call.Args) != 1 {
					bad = append(bad, where+": Msg needs exactly one argument")
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					bad = append(bad, where+": Msg argument must be a string literal")
					return true
				}
				s, uerr := strconv.Unquote(lit.Value)
				if uerr != nil || !eventKeyRe.MatchString(s) {
					bad = append(bad, where+": message "+lit.Value+" is not a dotted event key")
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return checked, bad
}

// TestServerLogMessagesAreEventKeys keeps msg a stable event key
// (area.event_name); variable data belongs in fields.
func TestServerLogMessagesAreEventKeys(t *testing.T) {
	checked, bad := scanMsgKeys(t, "../..", "../../../cmd")
	if checked < 40 {
		t.Fatalf("only %d Msg calls inspected; walk roots are probably wrong", checked)
	}
	for _, b := range bad {
		t.Error(b)
	}
}
