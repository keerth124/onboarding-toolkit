package jenkins

import (
	"fmt"
	"testing"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

func TestFilterDiscoveryResourcesIncludesAndExcludes(t *testing.T) {
	disc := testPlatformDiscovery("api", []platform.Resource{
		{ID: "Payments", FullName: "Payments", Type: "folder"},
		{ID: "Payments/API", FullName: "Payments/API", Type: "folder"},
		{ID: "Payments/API/deploy", FullName: "Payments/API/deploy", Type: "pipeline"},
		{ID: "Payments/sandbox/test", FullName: "Payments/sandbox/test", Type: "pipeline"},
		{ID: "Platform/build", FullName: "Platform/build", Type: "pipeline"},
	})

	filtered, err := FilterDiscoveryResources(disc, Selection{
		IncludePatterns: []string{"Payments/**"},
		ExcludePatterns: []string{"Payments/sandbox/**"},
		IncludeTypes:    []string{"folder", "pipeline"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Resources) != 3 {
		t.Fatalf("len(Resources) = %d, want 3: %#v", len(filtered.Resources), filtered.Resources)
	}
}

func TestFilterDiscoveryResourcesRequiresSelectionForLargeAPIDiscovery(t *testing.T) {
	var resources []platform.Resource
	for i := 0; i < largeDiscoveryThreshold+1; i++ {
		name := fmt.Sprintf("Folder/job-%03d", i)
		resources = append(resources, platform.Resource{ID: name, FullName: name, Type: "pipeline"})
	}
	_, err := FilterDiscoveryResources(testPlatformDiscovery("api", resources), Selection{})
	if err == nil {
		t.Fatal("expected selection required error")
	}
}

func TestFilterDiscoveryResourcesAllowsJobsFromFileWithoutSelection(t *testing.T) {
	disc := testPlatformDiscovery("jobs-from-file", []platform.Resource{
		{ID: "Payments", FullName: "Payments", Type: "folder"},
	})
	filtered, err := FilterDiscoveryResources(disc, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Resources) != 1 {
		t.Fatalf("len(Resources) = %d, want 1", len(filtered.Resources))
	}
}

func testPlatformDiscovery(source string, resources []platform.Resource) *platform.Discovery {
	return &platform.Discovery{
		Platform:  NewAdapter().Descriptor(),
		Resources: resources,
		Metadata:  map[string]string{"source": source},
	}
}
