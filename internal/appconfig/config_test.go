package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingDefaultIsOptional(t *testing.T) {
	cfg, found, err := Load(filepath.Join(t.TempDir(), "missing.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("found = true, cfg = %#v", cfg)
	}
}

func TestLoadMissingExplicitErrors(t *testing.T) {
	_, _, err := Load(filepath.Join(t.TempDir(), "missing.json"), true)
	if err == nil {
		t.Fatal("expected missing explicit config error")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conjur-onboard.json")
	want := Config{
		Version: Version,
		WorkDir: "work",
		Conjur: ConjurConfig{
			Target:   "saas",
			Tenant:   "my-tenant",
			Account:  "conjur",
			Username: "host/data/app",
		},
	}
	if err := Save(path, want, false); err != nil {
		t.Fatal(err)
	}

	got, found, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("config not found")
	}
	if got.Conjur.Tenant != want.Conjur.Tenant || got.WorkDir != want.WorkDir {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestSaveRefusesOverwriteWithoutForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conjur-onboard.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Save(path, Config{Version: Version}, false)
	if err == nil {
		t.Fatal("expected overwrite error")
	}
}

func TestValidateRejectsMismatchedTargetEndpoint(t *testing.T) {
	err := Config{
		Version: Version,
		Conjur: ConjurConfig{
			Target:    "saas",
			ConjurURL: "https://conjur.example.com",
		},
	}.Validate()
	if err == nil {
		t.Fatal("expected saas/conjur_url mismatch error")
	}
}
