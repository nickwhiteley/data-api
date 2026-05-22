package dataapi

import (
	"io/fs"
	"strings"
	"testing"
)

func TestMigrationsFS_FilesPresent(t *testing.T) {
	t.Parallel()
	expected := []string{
		"migrations_sql/001_data_extraction.sql",
		"migrations_sql/002_deny_defaults.sql",
	}
	for _, name := range expected {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, err := MigrationsFS.Open(name)
			if err != nil {
				t.Fatalf("MigrationsFS missing file %q: %v", name, err)
			}
			t.Cleanup(func() {
				if err := f.Close(); err != nil {
					t.Errorf("close %q: %v", name, err)
				}
			})
			stat, err := f.Stat()
			if err != nil {
				t.Fatalf("stat %q: %v", name, err)
			}
			if stat.Size() == 0 {
				t.Errorf("file %q is empty", name)
			}
		})
	}
}

func TestMigrationsFS_NoHardcodedSchema(t *testing.T) {
	t.Parallel()
	// Migration files must not reference 'wd' as a schema — they are schema-agnostic.
	err := fs.WalkDir(MigrationsFS, "migrations_sql", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, readErr := fs.ReadFile(MigrationsFS, path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "wd.") {
			t.Errorf("file %q contains hardcoded 'wd.' schema reference", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk migrations_sql: %v", err)
	}
}

func TestMigrationsFS_NoGooseDirectives(t *testing.T) {
	t.Parallel()
	// Exported SQL must be plain — no Goose directives.
	err := fs.WalkDir(MigrationsFS, "migrations_sql", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, readErr := fs.ReadFile(MigrationsFS, path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "+goose") {
			t.Errorf("file %q contains Goose directive — exported SQL must be plain", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk migrations_sql: %v", err)
	}
}
