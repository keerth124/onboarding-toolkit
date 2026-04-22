package jenkins

import "testing"

func TestBuildSyntheticClaimAnalysisDefaultsToJenkinsFullName(t *testing.T) {
	analysis := BuildSyntheticClaimAnalysis("Payments/API/deploy", ClaimSelection{})

	if analysis.SelectedClaims.TokenAppProperty != DefaultTokenAppProperty {
		t.Fatalf("TokenAppProperty = %q, want %s", analysis.SelectedClaims.TokenAppProperty, DefaultTokenAppProperty)
	}
	if analysis.Recommended[0] != DefaultTokenAppProperty {
		t.Fatalf("Recommended = %#v, want %s first", analysis.Recommended, DefaultTokenAppProperty)
	}
	found := false
	for _, claim := range analysis.AvailableClaims {
		if claim.Name == "jenkins_full_name" && claim.ExampleValue == "Payments/API/deploy" && claim.Recommended {
			found = true
		}
	}
	if !found {
		t.Fatalf("jenkins_full_name claim was not marked recommended: %#v", analysis.AvailableClaims)
	}
}

func TestValidateGeneratorSupportedSelectionRejectsSub(t *testing.T) {
	err := ValidateGeneratorSupportedSelection(ClaimSelection{TokenAppProperty: "sub"})
	if err == nil {
		t.Fatal("expected unsupported generator selection error")
	}
}

func TestParseClaimSelectionNormalizesEnforcedClaims(t *testing.T) {
	selection, err := ParseClaimSelection("", "jenkins_name, jenkins_parent_full_name,jenkins_name")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"jenkins_name", "jenkins_parent_full_name"}
	if len(selection.EnforcedClaims) != len(want) {
		t.Fatalf("EnforcedClaims = %#v, want %#v", selection.EnforcedClaims, want)
	}
	for i := range want {
		if selection.EnforcedClaims[i] != want[i] {
			t.Fatalf("EnforcedClaims = %#v, want %#v", selection.EnforcedClaims, want)
		}
	}
}
