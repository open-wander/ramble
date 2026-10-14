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

// slogMsgArg maps the slog Logger methods to the index of their msg argument.
var slogMsgArg = map[string]int{
	"Debug": 0, "Info": 0, "Warn": 0, "Error": 0,
	"DebugContext": 1, "InfoContext": 1, "WarnContext": 1, "ErrorContext": 1,
	"Log": 2, "LogAttrs": 2,
}

// scanFileMsgKeys inspects the log calls in f: slog Debug/Info/Warn/Error (msg
// is argument 1), their *Context variants (2) and Log/LogAttrs (3). Calls too short to carry a msg (a zero-argument Error()
// on an error value) are not log calls and are skipped. It returns the number
// of calls inspected and a list of violations (file:line description).
func scanFileMsgKeys(fset *token.FileSet, f *ast.File) (int, []string) {
	var checked int
	var bad []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pos := fset.Position(call.Pos())
		where := pos.Filename + ":" + strconv.Itoa(pos.Line)
		idx, isSlog := slogMsgArg[sel.Sel.Name]
		if !isSlog || len(call.Args) <= idx {
			return true
		}
		checked++
		msg := call.Args[idx]
		lit, ok := msg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			bad = append(bad, where+": "+sel.Sel.Name+" message must be a string literal")
			return true
		}
		s, uerr := strconv.Unquote(lit.Value)
		if uerr != nil || !eventKeyRe.MatchString(s) {
			bad = append(bad, where+": message "+lit.Value+" is not a dotted event key")
		}
		return true
	})
	return checked, bad
}

// scanMsgKeys walks roots and returns the number of log calls inspected plus
// a list of violations.
func scanMsgKeys(t *testing.T, roots ...string) (int, []string) {
	t.Helper()
	var checked int
	var bad []string
	walkServerFiles(t, roots, nil, func(fset *token.FileSet, f *ast.File) {
		c, b := scanFileMsgKeys(fset, f)
		checked += c
		bad = append(bad, b...)
	})
	return checked, bad
}

// TestServerLogMessagesAreEventKeys keeps msg a stable event key
// (area.event_name); variable data belongs in fields.
func TestServerLogMessagesAreEventKeys(t *testing.T) {
	checked, bad := scanMsgKeys(t, "../..", "../../../cmd")
	if checked < 40 {
		t.Fatalf("only %d log calls inspected; walk roots are probably wrong", checked)
	}
	for _, b := range bad {
		t.Error(b)
	}
}

func TestScanFileMsgKeys(t *testing.T) {
	src := `package x
func f(l L, ctx C, err error) {
	l.Info("a.b")
	l.Warn("a.b", "k", 1)
	l.Debug("a.b")
	l.Error("a.b", logger.Err(err))
	l.ErrorContext(ctx, "a.b")
	l.Log(ctx, 0, "a.b")
	l.LogAttrs(ctx, 0, "a.b")
	_ = err.Error()
	l.Info(msg)
	l.Info("nodot")
	l.Info("Has.Upper")
	l.InfoContext(ctx, msg)
	l.Log(ctx, 0, "nodot")
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	checked, bad := scanFileMsgKeys(fset, file)
	if checked != 12 || len(bad) != 5 {
		t.Fatalf("checked=%d bad=%d, want 12 and 5: %v", checked, len(bad), bad)
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

// zerologModule is the logging library this project moved away from; no
// server file may import it or any of its subpackages.
const zerologModule = "github.com/rs/zerolog"

// importNames returns the local names f gives the package imported from path
// (the base name unless aliased), skipping blank and dot imports.
func importNames(f *ast.File, path, base string) map[string]bool {
	names := map[string]bool{}
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err != nil || p != path {
			continue
		}
		name := base
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			names[name] = true
		}
	}
	return names
}

// pkgCalls returns "file:line: pkg.Func" for every call of a function in
// funcs on a package imported from path. The package is resolved by import
// path, so aliases match, while a local variable of the same name (Obj is
// non-nil for locals the parser resolved) does not.
func pkgCalls(fset *token.FileSet, f *ast.File, path, base string, match func(call *ast.CallExpr, fn string) bool) []string {
	names := importNames(f, path, base)
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
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || !names[pkg.Name] {
			return true
		}
		if match(call, sel.Sel.Name) {
			hits = append(hits, fset.Position(call.Pos()).String()+": "+base+"."+sel.Sel.Name)
		}
		return true
	})
	return hits
}

func parseFixture(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	return fset, file
}

// scanStdlibLog returns "file:line: call" for stdlib log.Print*, log.Fatal*
// and log.Panic* calls, plus an entry for each import of the zerolog module.
func scanStdlibLog(fset *token.FileSet, f *ast.File) []string {
	var hits []string
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil && (p == zerologModule || strings.HasPrefix(p, zerologModule+"/")) {
			hits = append(hits, fset.Position(imp.Pos()).String()+": import "+p)
		}
	}
	return append(hits, pkgCalls(fset, f, "log", "log", func(_ *ast.CallExpr, fn string) bool {
		return strings.HasPrefix(fn, "Print") || strings.HasPrefix(fn, "Fatal") || strings.HasPrefix(fn, "Panic")
	})...)
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
		t.Errorf("stdlib log or zerolog in server code (use logger.Log with an event key): %s", h)
	}
}

func TestScanStdlibLog(t *testing.T) {
	hit := `package x
import ("log";stdlog "log";"` + zerologModule + `/log")
func f() {
	log.Printf("a")
	log.Fatal("x")
	log.Panicln("x")
	stdlog.Println("x")
}`
	miss := `package x
import ("example.com/x/log";stdlog "log")
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
	}{{"hit", hit, 5}, {"miss", miss, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			fset, file := parseFixture(t, tc.src)
			if got := scanStdlibLog(fset, file); len(got) != tc.want {
				t.Fatalf("got %d hits, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

// scanOSExit returns "file:line: os.Exit" for calls of os.Exit. A reference
// such as `var exit = os.Exit` is a seam tests replace, not a call.
func scanOSExit(fset *token.FileSet, f *ast.File) []string {
	return pkgCalls(fset, f, "os", "os", func(_ *ast.CallExpr, fn string) bool { return fn == "Exit" })
}

// TestServerCodeHasNoOSExit keeps deferred cleanup and log flushing intact:
// server code returns errors. The allowlist is implicit: database's `exit`
// seam is a value, not a call, and internal/cli (the entry point, which does
// exit) is skipped by walkServerFiles. log.Fatal* and log.Panic* fall under
// TestServerCodeDoesNotUseStdlibLog.
func TestServerCodeHasNoOSExit(t *testing.T) {
	var hits []string
	checked := walkServerFiles(t, []string{"../../../internal", "../../../cmd"}, nil, func(fset *token.FileSet, f *ast.File) {
		hits = append(hits, scanOSExit(fset, f)...)
	})
	if checked < 40 {
		t.Fatalf("only %d files inspected; walk roots are wrong", checked)
	}
	for _, h := range hits {
		t.Errorf("os.Exit call in server code (return an error, or go through a seam): %s", h)
	}
}

func TestScanOSExit(t *testing.T) {
	hit := `package x
import ("os";o "os")
func f() {
	os.Exit(1)
	o.Exit(2)
}`
	miss := `package x
import "os"
type X struct{}
func (X) Exit() {}
var exit = os.Exit
func f(t T) {
	var x X
	x.Exit()
	t.Fatal("x")
	exit(1)
}`
	for _, tc := range []struct {
		name, src string
		want      int
	}{{"hit", hit, 2}, {"miss", miss, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			fset, file := parseFixture(t, tc.src)
			if got := scanOSExit(fset, file); len(got) != tc.want {
				t.Fatalf("got %d hits, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

// slogPkgLogFuncs are the package-level slog functions that log through the
// default logger, which has no request_id.
var slogPkgLogFuncs = map[string]bool{
	"Debug": true, "Info": true, "Warn": true, "Error": true, "Log": true, "LogAttrs": true,
	"DebugContext": true, "InfoContext": true, "WarnContext": true, "ErrorContext": true,
}

// scanSlogPackageCalls returns hits for slog.Info and friends called on the
// package rather than on a *slog.Logger.
func scanSlogPackageCalls(fset *token.FileSet, f *ast.File) []string {
	return pkgCalls(fset, f, "log/slog", "slog", func(_ *ast.CallExpr, fn string) bool { return slogPkgLogFuncs[fn] })
}

// TestServerCodeDoesNotCallPackageLevelSlog makes server code log through
// logger.Log or the request logger, so request_id is never lost by accident.
func TestServerCodeDoesNotCallPackageLevelSlog(t *testing.T) {
	var hits []string
	checked := walkServerFiles(t, []string{"../../../internal", "../../../cmd"}, nil, func(fset *token.FileSet, f *ast.File) {
		hits = append(hits, scanSlogPackageCalls(fset, f)...)
	})
	if checked < 40 {
		t.Fatalf("only %d files inspected; walk roots are wrong", checked)
	}
	for _, h := range hits {
		t.Errorf("package-level slog call (use logger.Log or logger.FromContext): %s", h)
	}
}

func TestScanSlogPackageCalls(t *testing.T) {
	hit := `package x
import ("log/slog";sl "log/slog")
func f(ctx C) {
	slog.Info("a.b")
	slog.WarnContext(ctx, "a.b")
	sl.Error("a.b")
	slog.LogAttrs(ctx, 0, "a.b")
}`
	miss := `package x
import "log/slog"
type L struct{}
func (L) Info(string) {}
func f(l *slog.Logger) {
	l.Info("a.b")
	_ = slog.String("k", "v")
	_ = slog.New(nil)
	slog.SetDefault(l)
	var slog L
	slog.Info("a.b")
}`
	for _, tc := range []struct {
		name, src string
		want      int
	}{{"hit", hit, 4}, {"miss", miss, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			fset, file := parseFixture(t, tc.src)
			if got := scanSlogPackageCalls(fset, file); len(got) != tc.want {
				t.Fatalf("got %d hits, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

// scanSlogAttrMisuse returns hits for slog.Duration (it writes integer
// nanoseconds; durations are float milliseconds) and slog.Any("error", ...)
// (use logger.Err). It cannot see a "error", err key/value pair or a
// time.Duration passed as a plain value.
func scanSlogAttrMisuse(fset *token.FileSet, f *ast.File) []string {
	return pkgCalls(fset, f, "log/slog", "slog", func(call *ast.CallExpr, fn string) bool {
		switch fn {
		case "Duration":
			return true
		case "Any":
			if len(call.Args) == 0 {
				return false
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			return ok && lit.Kind == token.STRING && lit.Value == `"error"`
		}
		return false
	})
}

func TestServerCodeUsesStandardAttrs(t *testing.T) {
	var hits []string
	checked := walkServerFiles(t, []string{"../../../internal", "../../../cmd"}, nil, func(fset *token.FileSet, f *ast.File) {
		hits = append(hits, scanSlogAttrMisuse(fset, f)...)
	})
	if checked < 40 {
		t.Fatalf("only %d files inspected; walk roots are wrong", checked)
	}
	for _, h := range hits {
		t.Errorf("slog.Duration (use float milliseconds) or slog.Any(\"error\", ...) (use logger.Err): %s", h)
	}
}

func TestScanSlogAttrMisuse(t *testing.T) {
	hit := `package x
import ("log/slog";sl "log/slog")
func f(err error, d D) {
	_ = slog.Duration("d", d)
	_ = sl.Any("error", err)
	_ = slog.Any("error", err)
}`
	miss := `package x
import "log/slog"
func f(err error, ms float64) {
	_ = slog.Any("details", err)
	_ = slog.Float64("duration_ms", ms)
	_ = slog.String("error", err.Error())
}`
	for _, tc := range []struct {
		name, src string
		want      int
	}{{"hit", hit, 3}, {"miss", miss, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			fset, file := parseFixture(t, tc.src)
			if got := scanSlogAttrMisuse(fset, file); len(got) != tc.want {
				t.Fatalf("got %d hits, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}
