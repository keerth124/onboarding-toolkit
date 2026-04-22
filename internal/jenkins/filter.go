package jenkins

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

const largeDiscoveryThreshold = 100

type Selection struct {
	IncludePatterns []string
	ExcludePatterns []string
	IncludeTypes    []string
	All             bool
}

func FilterDiscoveryResources(disc *platform.Discovery, selection Selection) (*platform.Discovery, error) {
	if disc == nil {
		return nil, fmt.Errorf("discovery is required")
	}
	if len(disc.Resources) > largeDiscoveryThreshold && isAPISource(disc) && !selection.All && !selection.hasFilters() {
		return nil, fmt.Errorf("Jenkins discovery contains %d resources; pass --include, --exclude, --include-type, --jobs-from-file during discover, or --all to generate every resource", len(disc.Resources))
	}
	typeSet := selection.typeSet()
	filtered := make([]platform.Resource, 0, len(disc.Resources))
	for _, resource := range disc.Resources {
		fullName := firstNonEmpty(resource.FullName, resource.ID, resource.Name)
		if fullName == "" {
			continue
		}
		if len(selection.IncludePatterns) > 0 && !matchesAny(fullName, selection.IncludePatterns) {
			continue
		}
		if len(selection.ExcludePatterns) > 0 && matchesAny(fullName, selection.ExcludePatterns) {
			continue
		}
		if len(typeSet) > 0 && !typeSet[strings.ToLower(resource.Type)] {
			continue
		}
		filtered = append(filtered, resource)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("Jenkins workload selection matched no resources")
	}
	copyDisc := *disc
	copyDisc.Resources = filtered
	sort.Slice(copyDisc.Resources, func(i, j int) bool {
		return copyDisc.Resources[i].FullName < copyDisc.Resources[j].FullName
	})
	return &copyDisc, nil
}

func (s Selection) hasFilters() bool {
	return len(s.IncludePatterns) > 0 || len(s.ExcludePatterns) > 0 || len(s.IncludeTypes) > 0
}

func (s Selection) typeSet() map[string]bool {
	if len(s.IncludeTypes) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, typ := range s.IncludeTypes {
		typ = strings.ToLower(strings.TrimSpace(typ))
		if typ != "" {
			set[typ] = true
		}
	}
	return set
}

func isAPISource(disc *platform.Discovery) bool {
	if disc.Metadata == nil {
		return false
	}
	return disc.Metadata["source"] == "api"
}

func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(value, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(value string, pattern string) bool {
	value = strings.Trim(value, "/")
	pattern = strings.Trim(strings.TrimSpace(pattern), "/")
	if pattern == "" {
		return false
	}
	if pattern == value {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return value == prefix || strings.HasPrefix(value, prefix+"/")
	}
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		return strings.HasPrefix(value, strings.Trim(parts[0], "/")) && strings.HasSuffix(value, strings.Trim(parts[len(parts)-1], "/"))
	}
	ok, err := path.Match(pattern, value)
	return err == nil && ok
}
