// Package deps hosts the executable dependency-rule check defined by
// docs/architecture/dependency-map.md. It is a test-only runner: it builds
// no product behavior and exists so a violation of the documented import
// rules fails the build.
package deps

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/qira-dev/karyora"

// goListPackage mirrors the subset of `go list -json` output this check uses.
type goListPackage struct {
	ImportPath string
	Imports    []string
	Standard   bool
}

// allowedInternal mirrors the "Allowed internal imports" column of
// docs/architecture/dependency-map.md (paths relative to internal/). cmd/uawp
// is the composition root and is intentionally absent: it may import anything.
var allowedInternal = map[string]map[string]bool{
	"domain":          {},
	"events":          {"domain": true},
	"security":        {"domain": true},
	"config":          {"domain": true},
	"health":          {},
	"contractadapter": {"domain": true},
	"knowledgepack":   {"domain": true, "events": true},
	"aijobrelay":      {"domain": true, "events": true},
	"contextengine":   {"domain": true, "events": true},
	"workflow":        {"domain": true, "events": true},
	"app": {
		"domain": true, "events": true, "security": true,
		"knowledgepack": true, "aijobrelay": true, "contextengine": true, "workflow": true,
	},
	"storage": {
		"domain": true, "events": true,
		"knowledgepack": true, "aijobrelay": true, "contextengine": true, "workflow": true,
	},
	"providers": {"domain": true, "security": true},
	"plugins":   {"domain": true, "security": true},
	"runtime":   {"app": true, "security": true, "platform": true},
	"platform":  {},
}

// confinement lists imports that are allowed only in the named internal
// packages: database drivers only in storage; OS-specific APIs only in
// platform (and the cmd composition root).
var confinement = []struct {
	imported    string
	onlyIn      []string
	description string
}{
	{"database/sql", []string{"storage"}, "database driver imports are confined to internal/storage"},
	{"github.com/mattn/go-sqlite3", []string{"storage"}, "SQLite driver imports are confined to internal/storage"},
	{"modernc.org/sqlite", []string{"storage"}, "SQLite driver imports are confined to internal/storage"},
	{"syscall", []string{"platform"}, "OS-specific API imports are confined to internal/platform"},
	{"os/exec", []string{"platform"}, "OS-specific API imports are confined to internal/platform"},
	{"golang.org/x/sys/", []string{"platform"}, "OS-specific API imports are confined to internal/platform"},
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

func loadPackages(t *testing.T) []goListPackage {
	t.Helper()
	root := moduleRoot(t)
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -json ./...: %v", err)
	}
	var pkgs []goListPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p goListPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// internalRel converts an import path to its internal/-relative form, or
// returns ok=false for imports outside this module.
func internalRel(importPath string) (rel string, ok bool) {
	rest, found := strings.CutPrefix(importPath, modulePath+"/internal/")
	return rest, found
}

func TestDependencyRules(t *testing.T) {
	pkgs := loadPackages(t)
	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages")
	}
	for _, p := range pkgs {
		if p.Standard {
			continue
		}
		rel, isInternal := internalRel(p.ImportPath)
		if !isInternal {
			continue // cmd/uawp is the composition root; tests/deps is a runner.
		}
		allowed, ruled := allowedInternal[strings.SplitN(rel, "/", 2)[0]]
		if !ruled {
			t.Errorf("package %s is not covered by dependency-map.md: add it to the map and this check, or remove it", p.ImportPath)
			continue
		}
		for _, imp := range p.Imports {
			impRel, impInternal := internalRel(imp)
			if !impInternal {
				continue
			}
			if !allowed[strings.SplitN(impRel, "/", 2)[0]] {
				t.Errorf("%s imports internal/%s, which is not allowed by dependency-map.md", p.ImportPath, impRel)
			}
		}
		for _, c := range confinement {
			for _, imp := range p.Imports {
				if imp == c.imported || (strings.HasSuffix(c.imported, "/") && strings.HasPrefix(imp, c.imported)) {
					if !allowedIn(rel, c.onlyIn) {
						t.Errorf("%s imports %s: %s", p.ImportPath, imp, c.description)
					}
				}
			}
		}
	}
}

func allowedIn(rel string, onlyIn []string) bool {
	for _, allowed := range onlyIn {
		if rel == allowed || strings.HasPrefix(rel, allowed+"/") {
			return true
		}
	}
	return false
}
