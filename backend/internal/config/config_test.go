package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetenv(t *testing.T) {
	t.Setenv("CFG_TEST_SET", "x")
	t.Setenv("CFG_TEST_EMPTY", "")
	if got := Getenv("CFG_TEST_SET", "d"); got != "x" {
		t.Errorf("set: got %q", got)
	}
	if got := Getenv("CFG_TEST_EMPTY", "d"); got != "d" {
		t.Errorf("empty: got %q", got)
	}
}

func TestLoadReportsMissingVars(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("# c\n\nCFG_A=1\nCFG_B=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	t.Setenv("CFG_A", "set")
	t.Setenv("CFG_B", "")

	if err := Load(); err == nil {
		t.Fatal("expected error for missing CFG_B")
	}
	t.Setenv("CFG_B", "set")
	if err := Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}
