package testenv_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/chadmayfield/skill-fizzy/"

// Every test package that can reach the config loader, directly or through
// another package of this module, must call testenv.Main from its TestMain.
// Without it a test could read the real config or reach a real server.
// The testenv package itself is exempt: its tests call Isolate directly.
func TestEveryPackageThatLoadsConfigUsesMain(t *testing.T) {
	root := filepath.Join("..", "..")
	imports := map[string][]string{} // package dir -> module packages it imports
	hasTests := map[string]bool{}
	callsMain := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) && path != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		dir, _ := filepath.Rel(root, filepath.Dir(path))
		dir = filepath.ToSlash(dir)
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if rest, ok := strings.CutPrefix(p, module); ok {
				imports[dir] = append(imports[dir], rest)
			}
		}
		if strings.HasSuffix(path, "_test.go") {
			hasTests[dir] = true
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(src), "testenv.Main(") {
				callsMain[dir] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reaches := map[string]bool{"internal/config": true}
	for changed := true; changed; {
		changed = false
		for dir, deps := range imports {
			for _, dep := range deps {
				if reaches[dep] && !reaches[dir] {
					reaches[dir], changed = true, true
				}
			}
		}
	}
	checked := 0
	for dir := range hasTests {
		if !reaches[dir] || dir == "internal/testenv" {
			continue
		}
		checked++
		if !callsMain[dir] {
			t.Errorf("%s reaches internal/config but no test file calls testenv.Main from TestMain", dir)
		}
	}
	if checked == 0 {
		t.Error("found no package to check; the walk is broken")
	}
}
