// Package archtest holds repository-shape checks that run with `go test`: the dependency direction between
// layers and the file-length ceiling. Test files are exempt from both. See CONTRIBUTING.md to lower a ceiling.
package archtest

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	modulePath = "github.com/socialos/backend/internal/"
	// maxFileLines is the CI ceiling (the norm is 250). Ratchet: lower it, never raise it.
	maxFileLines = 400
)

// backendRoot is the directory holding go.mod (two levels above this package).
const backendRoot = "../.."

// productionFiles returns every non-test, non-generated .go file below dir (relative to backendRoot).
func productionFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	root := filepath.Join(backendRoot, dir)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func imports(t *testing.T, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("bad import in %s: %v", path, err)
		}
		out = append(out, p)
	}
	return out
}

// forbid fails for every file under dir that imports a package below one of the forbidden internal prefixes.
func forbid(t *testing.T, dir string, forbidden ...string) {
	t.Helper()
	for _, file := range productionFiles(t, dir) {
		for _, imp := range imports(t, file) {
			for _, bad := range forbidden {
				if strings.HasPrefix(imp, modulePath+bad) {
					t.Errorf("%s imports %s: %s must not depend on %s", file, imp, dir, bad)
				}
			}
		}
	}
}

func TestDomainDependsOnNothingOuter(t *testing.T) {
	forbid(t, "internal/domain", "application", "adapters", "infrastructure", "transport")
}

func TestTransportDoesNotImportPostgres(t *testing.T) {
	forbid(t, "internal/transport", "infrastructure/postgres")
}

func TestFileLengthCeiling(t *testing.T) {
	for _, file := range productionFiles(t, ".") {
		n := lineCount(t, file)
		if n > maxFileLines {
			t.Errorf("%s has %d lines, ceiling is %d: split it by topic", file, n, maxFileLines)
		}
	}
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return n
}
