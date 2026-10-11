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

// walkServerFiles parses every non-test Go file under roots, skipping
// testdata and internal/cli (a user-facing terminal tool, not the server) plus
// any directory whose slash path ends in one of extraSkip, and calls visit for
// each. It returns the file count so callers can assert the walk was not vacuous.
func walkServerFiles(t *testing.T, roots []string, extraSkip []string, visit func(fset *token.FileSet, f *ast.File)) int {
	t.Helper()
	fset := token.NewFileSet()
	checked := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(path)
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "cli" && strings.Contains(slash, "internal/cli") {
					return filepath.SkipDir
				}
				for _, s := range extraSkip {
					if strings.HasSuffix(slash, s) {
						return filepath.SkipDir
					}
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
			checked++
			visit(fset, f)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return checked
}

// scanMsgKeys walks roots and returns the number of zerolog Msg/Msgf calls
// inspected plus a list of violations (file:line description).
func scanMsgKeys(t *testing.T, roots ...string) (int, []string) {
	t.Helper()
	var checked int
	var bad []string
	walkServerFiles(t, roots, nil, func(fset *token.FileSet, f *ast.File) {
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
	})
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

// scanStdoutWrites returns "file:line: call" for direct writes to
// stdout/stderr: fmt.Print*, builtin print/println, and fmt.Fprint* whose
// first argument is os.Stdout or os.Stderr. It matches on identifier names,
// so a local func named print or a package aliased to fmt would also trip it.
func scanStdoutWrites(fset *token.FileSet, f *ast.File) []string {
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "print" || fun.Name == "println" {
				name = fun.Name
			}
		case *ast.SelectorExpr:
			pkg, ok := fun.X.(*ast.Ident)
			if !ok || pkg.Name != "fmt" {
				return true
			}
			switch fun.Sel.Name {
			case "Print", "Printf", "Println":
				name = "fmt." + fun.Sel.Name
			case "Fprint", "Fprintf", "Fprintln":
				if len(call.Args) > 0 && isOSStdStream(call.Args[0]) {
					name = "fmt." + fun.Sel.Name
				}
			}
		}
		if name != "" {
			hits = append(hits, fset.Position(call.Pos()).String()+": "+name)
		}
		return true
	})
	return hits
}

func isOSStdStream(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr")
}

func TestServerCodeDoesNotWriteStdout(t *testing.T) {
	var hits []string
	// cmd/nomad-vars is an operator CLI that prints to the terminal on purpose.
	checked := walkServerFiles(t, []string{"../../../internal", "../../../cmd"}, []string{"cmd/nomad-vars"}, func(fset *token.FileSet, f *ast.File) {
		hits = append(hits, scanStdoutWrites(fset, f)...)
	})
	if checked < 40 {
		t.Fatalf("only %d files inspected; walk roots are wrong", checked)
	}
	for _, h := range hits {
		t.Errorf("direct stdout/stderr write in server code (use logger.FromContext / logger.Log): %s", h)
	}
}

func TestScanStdoutWrites(t *testing.T) {
	src := `package x
import ("fmt";"os";"bytes")
func f(b *bytes.Buffer) {
	fmt.Printf("a")
	fmt.Println("a")
	fmt.Print("a")
	print("a")
	println("a")
	fmt.Fprintf(os.Stderr, "a")
	fmt.Fprintln(os.Stdout, "a")
	fmt.Fprint(os.Stdout, "a")
	_ = fmt.Sprintf("ok")
	_ = fmt.Errorf("ok")
	fmt.Fprintf(b, "ok")
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := scanStdoutWrites(fset, file); len(got) != 8 {
		t.Fatalf("got %d hits, want 8: %v", len(got), got)
	}
}

// scanFatalPanic returns "file:line: call" for zero-argument .Fatal() and
// .Panic() calls, the shape of a zerolog event constructor. These log at
// FATAL/PANIC, outside DEBUG|INFO|WARN|ERROR, and Fatal exits without cleanup.
// Calls with arguments (stdlib log.Fatalf, t.Fatal("x")) are not matched.
func scanFatalPanic(fset *token.FileSet, f *ast.File) []string {
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (sel.Sel.Name == "Fatal" || sel.Sel.Name == "Panic") {
			hits = append(hits, fset.Position(call.Pos()).String()+": "+sel.Sel.Name)
		}
		return true
	})
	return hits
}

func TestServerCodeHasNoFatalOrPanicEvents(t *testing.T) {
	var hits []string
	checked := walkServerFiles(t, []string{"../..", "../../../cmd"}, nil, func(fset *token.FileSet, f *ast.File) {
		hits = append(hits, scanFatalPanic(fset, f)...)
	})
	if checked < 40 {
		t.Fatalf("only %d files inspected; walk roots are wrong", checked)
	}
	for _, h := range hits {
		t.Errorf("FATAL/PANIC event in server code (log at ERROR and return or exit): %s", h)
	}
}

func TestScanFatalPanic(t *testing.T) {
	src := `package x
func f(l L, t T) {
	l.Fatal().Msg("a.b")
	l.Panic().Msg("a.b")
	log.Fatalf("x")
	t.Fatal("x")
	l.Error().Msg("a.b")
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := scanFatalPanic(fset, file); len(got) != 2 {
		t.Fatalf("got %d hits, want 2: %v", len(got), got)
	}
}

// scanStdlibLog returns "file:line: call" for stdlib log.Print*, log.Fatal*
// and log.Panic* calls. The package is resolved by import path, so the local
// name may be an alias (stdlog), while github.com/rs/zerolog/log and a local
// variable named log do not match.
func scanStdlibLog(fset *token.FileSet, f *ast.File) []string {
	names := map[string]bool{}
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err != nil || p != "log" {
			continue
		}
		name := "log"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			names[name] = true
		}
	}
	if len(names) == 0 {
		return nil
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// Obj is nil for package names and non-nil for locals the parser resolved.
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || !names[pkg.Name] {
			return true
		}
		for _, prefix := range []string{"Print", "Fatal", "Panic"} {
			if strings.HasPrefix(sel.Sel.Name, prefix) {
				hits = append(hits, fset.Position(call.Pos()).String()+": log."+sel.Sel.Name)
				break
			}
		}
		return true
	})
	return hits
}

func TestServerCodeDoesNotUseStdlibLog(t *testing.T) {
	var hits []string
	// cmd/nomad-vars is an operator CLI that reports to the terminal on purpose.
	checked := walkServerFiles(t, []string{"../../../internal", "../../../cmd"}, []string{"cmd/nomad-vars"}, func(fset *token.FileSet, f *ast.File) {
		// test_helper.go is test-only scaffolding called from other packages'
		// tests; its log.Fatalf runs before any testing.T exists.
		if strings.HasSuffix(filepath.ToSlash(fset.Position(f.Pos()).Filename), "internal/database/test_helper.go") {
			return
		}
		hits = append(hits, scanStdlibLog(fset, f)...)
	})
	if checked < 40 {
		t.Fatalf("only %d files inspected; walk roots are wrong", checked)
	}
	for _, h := range hits {
		t.Errorf("stdlib log in server code (use logger.Log with an event key): %s", h)
	}
}

func TestScanStdlibLog(t *testing.T) {
	hit := `package x
import ("log";stdlog "log")
func f() {
	log.Printf("a")
	log.Fatal("x")
	log.Panicln("x")
	stdlog.Println("x")
}`
	miss := `package x
import ("github.com/rs/zerolog/log";stdlog "log")
type L struct{}
func (L) Printf(string) {}
func f() {
	log.Print("a")
	stdlog.SetFlags(0)
	stdlog.SetOutput(nil)
	var log L
	log.Printf("a")
}`
	for _, tc := range []struct {
		name, src string
		want      int
	}{{"hit", hit, 4}, {"miss", miss, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "x.go", tc.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := scanStdlibLog(fset, file); len(got) != tc.want {
				t.Fatalf("got %d hits, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}
