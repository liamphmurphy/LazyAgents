package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

// Two branches can each add the next migration number and merge cleanly;
// goose then refuses to start. Catch that here, without a database.
func TestMigrationVersionsAreUnique(t *testing.T) {
	names, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, name := range names {
		version, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("%s has no version prefix", name)
			continue
		}
		if prev, dup := seen[version]; dup {
			t.Errorf("version %s is used by both %s and %s; renumber the newer one", version, prev, name)
		}
		seen[version] = name
	}
}
