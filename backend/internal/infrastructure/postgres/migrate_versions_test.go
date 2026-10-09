package postgres_test

import (
	"io/fs"
	"regexp"
	"strconv"
	"testing"

	"github.com/socialos/backend/migrations"
)

// goose refuses to apply a migration numbered below one that is already applied, so a gap or a late merge of a lower
// number breaks every database that has run the higher one. Versions must be 1..N with no holes; merge migration PRs
// in number order instead of loosening goose (no WithAllowMissing).
func TestMigrationVersionsAreContiguous(t *testing.T) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^(\d+)_`)
	seen := map[int]bool{}
	top := 0
	for _, f := range files {
		m := re.FindStringSubmatch(f)
		if m == nil {
			t.Errorf("%s: migration files must start with a number and an underscore", f)
			continue
		}
		v, _ := strconv.Atoi(m[1])
		if seen[v] {
			t.Errorf("version %d is used twice", v)
		}
		seen[v], top = true, max(top, v)
	}
	for v := 1; v <= top; v++ {
		if !seen[v] {
			t.Errorf("migration %05d is missing: versions must be contiguous (is a lower-numbered migration PR not merged yet?)", v)
		}
	}
}
