package access

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var rowOp = regexp.MustCompile(`\bstore\.(Insert|Set|Update|Delete)\(`)

func TestRowsAreBuiltBesideTheirRules(t *testing.T) {
	checked := 0
	for _, root := range []string{"..", "../../tools"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			checked++
			if rowOp.Match(raw) && !strings.HasSuffix(name, "writes.go") && !strings.HasSuffix(name, "cache.go") {
				t.Errorf("%s builds rows; they belong in a writes.go or <thing>_writes.go, or a cache.go or <thing>_cache.go for a cascade", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no Go files found")
	}
}
