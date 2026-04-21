package github

import (
	"reflect"
	"testing"
)

func TestParseClaimSelectionDefaultsAndSorts(t *testing.T) {
	got, err := ParseClaimSelection("", "workflow_ref, environment,environment")
	if err != nil {
		t.Fatal(err)
	}

	want := ClaimSelection{
		TokenAppProperty: "repository",
		EnforcedClaims:   []string{"environment", "workflow_ref"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseClaimSelection() = %#v, want %#v", got, want)
	}
}

func TestValidateGeneratorSupportedSelectionRejectsUnsupportedMVPClaims(t *testing.T) {
	selection := ClaimSelection{
		TokenAppProperty: "repository_owner",
	}

	if err := ValidateGeneratorSupportedSelection(selection); err == nil {
		t.Fatal("expected unsupported token_app_property error")
	}
}

func TestValidateGeneratorSupportedSelectionRejectsEnforcedClaims(t *testing.T) {
	selection := ClaimSelection{
		TokenAppProperty: "repository",
		EnforcedClaims:   []string{"environment"},
	}

	if err := ValidateGeneratorSupportedSelection(selection); err == nil {
		t.Fatal("expected unsupported enforced claims error")
	}
}
