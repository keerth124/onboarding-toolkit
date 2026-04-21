package github

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadRepoNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.txt")
	content := "# comment\napi-service\nacme/web\n\nworker\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadRepoNames(path)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"api-service", "acme/web", "worker"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadRepoNames() = %#v, want %#v", got, want)
	}
}

func TestLoadRepoNamesRejectsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.txt")
	if err := os.WriteFile(path, []byte("api service\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadRepoNames(path); err == nil {
		t.Fatal("expected whitespace validation error")
	}
}
